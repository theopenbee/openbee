import { Fragment, useMemo, useRef, useState, type CSSProperties, type KeyboardEvent, type ReactNode } from "react"
import { ChevronRightIcon, FolderIcon, FolderOpenIcon } from "lucide-react"
import { cn } from "@/lib/utils"
import type { DepartmentTree as DepartmentTreeNode } from "@/lib/types"

const INDENT = 18
const INSET = 12

interface TreeRow {
  id: string
  label: string
  depth: number
  parentId?: string
  position: number
  setSize: number
  hasChildren: boolean
  icon?: ReactNode
}

export interface DepartmentTreeLeadingItem {
  id: string
  label: string
  icon: ReactNode
}

interface DepartmentTreeProps {
  departments: DepartmentTreeNode[]
  selectedId: string | null
  onSelect: (id: string) => void
  label: string
  leading?: DepartmentTreeLeadingItem[]
  leadingLabel?: string
  className?: string
}

function flatten(nodes: DepartmentTreeNode[], collapsed: ReadonlySet<string>, depth = 1, parentId?: string): TreeRow[] {
  return nodes.flatMap((node, index) => [
    {
      id: node.id,
      label: node.name,
      depth,
      parentId,
      position: index + 1,
      setSize: nodes.length,
      hasChildren: node.children.length > 0,
    },
    ...(node.children.length > 0 && !collapsed.has(node.id) ? flatten(node.children, collapsed, depth + 1, node.id) : []),
  ])
}

function ancestorsOf(nodes: DepartmentTreeNode[], id: string): string[] | null {
  for (const node of nodes) {
    if (node.id === id) return []
    const below = ancestorsOf(node.children, id)
    if (below) return [node.id, ...below]
  }
  return null
}

export function DepartmentTree({ departments, selectedId, onSelect, label, leading = [], leadingLabel, className }: DepartmentTreeProps) {
  const [collapsed, setCollapsed] = useState<Set<string>>(() => new Set())
  const [focusedId, setFocusedId] = useState<string | null>(null)
  const [revealed, setRevealed] = useState<string | null>(null)
  const refs = useRef(new Map<string, HTMLButtonElement>())

  if (selectedId !== revealed) {
    setRevealed(selectedId)
    const ancestors = selectedId ? ancestorsOf(departments, selectedId) : null
    if (ancestors?.some((id) => collapsed.has(id))) {
      const next = new Set(collapsed)
      ancestors.forEach((id) => next.delete(id))
      setCollapsed(next)
    }
  }

  const rows = useMemo<TreeRow[]>(
    () => [
      ...leading.map((item, index) => ({
        id: item.id,
        label: item.label,
        depth: 1,
        position: index + 1,
        setSize: leading.length + departments.length,
        hasChildren: false,
        icon: item.icon,
      })),
      ...flatten(departments, collapsed).map((row) =>
        row.depth === 1 && leading.length > 0
          ? { ...row, position: row.position + leading.length, setSize: leading.length + departments.length }
          : row,
      ),
    ],
    [leading, departments, collapsed],
  )

  const has = (id: string | null) => id !== null && rows.some((row) => row.id === id)
  const tabStop = has(focusedId) ? focusedId : has(selectedId) ? selectedId : (rows[0]?.id ?? null)
  const firstDepartmentId = departments[0]?.id

  const toggle = (row: TreeRow) => {
    if (!row.hasChildren) return
    setCollapsed((prev) => {
      const next = new Set(prev)
      if (next.has(row.id)) next.delete(row.id)
      else next.add(row.id)
      return next
    })
  }

  const focus = (id: string) => {
    setFocusedId(id)
    const node = refs.current.get(id)
    if (node) node.focus()
    else requestAnimationFrame(() => refs.current.get(id)?.focus())
  }

  const onKeyDown = (event: KeyboardEvent<HTMLButtonElement>, row: TreeRow, index: number) => {
    const expanded = row.hasChildren && !collapsed.has(row.id)
    switch (event.key) {
      case "ArrowDown":
        event.preventDefault()
        focus(rows[Math.min(index + 1, rows.length - 1)].id)
        return
      case "ArrowUp":
        event.preventDefault()
        focus(rows[Math.max(index - 1, 0)].id)
        return
      case "Home":
        event.preventDefault()
        focus(rows[0].id)
        return
      case "End":
        event.preventDefault()
        focus(rows[rows.length - 1].id)
        return
      case "ArrowRight":
        if (!row.hasChildren) return
        event.preventDefault()
        if (!expanded) toggle(row)
        else if (rows[index + 1]?.parentId === row.id) focus(rows[index + 1].id)
        return
      case "ArrowLeft":
        event.preventDefault()
        if (expanded) toggle(row)
        else if (row.parentId) focus(row.parentId)
        return
      case "Enter":
      case " ":
        event.preventDefault()
        onSelect(row.id)
        return
    }
  }

  return (
    <div role="tree" aria-label={label} className={cn("grid gap-0.5", className)}>
      {rows.map((row, index) => {
        const expanded = row.hasChildren && !collapsed.has(row.id)
        const selected = row.id === selectedId
        const Folder = expanded ? FolderOpenIcon : FolderIcon
        return (
          <Fragment key={row.id}>
            {leadingLabel && row.id === firstDepartmentId && leading.length > 0 && (
              <div role="none" aria-hidden="true" className="px-3 pt-4 pb-1.5 text-xs text-secondary">
                {leadingLabel}
              </div>
            )}
            <button
              ref={(node) => {
                if (node) refs.current.set(row.id, node)
                else refs.current.delete(row.id)
              }}
              type="button"
              role="treeitem"
              aria-level={row.depth}
              aria-posinset={row.position}
              aria-setsize={row.setSize}
              aria-expanded={row.hasChildren ? expanded : undefined}
              aria-selected={selected}
              tabIndex={row.id === tabStop ? 0 : -1}
              title={row.label}
              onFocus={() => setFocusedId(row.id)}
              onKeyDown={(event) => onKeyDown(event, row, index)}
              onClick={() => {
                setFocusedId(row.id)
                onSelect(row.id)
              }}
              style={{ paddingLeft: INSET + (row.depth - 1) * INDENT } as CSSProperties}
              className={cn(
                "relative flex h-10 w-full min-w-0 items-center gap-2 rounded-control pr-3 text-left text-sm transition-colors duration-150",
                selected
                  ? "bg-accent-subtle text-foreground"
                  : "text-secondary hover:bg-surface-muted hover:text-foreground",
              )}
            >
              {row.depth > 1 && (
                <span
                  aria-hidden="true"
                  className="absolute inset-y-1 w-px bg-border-subtle"
                  style={{ left: INSET + (row.depth - 2) * INDENT + 8 }}
                />
              )}
              <span
                aria-hidden="true"
                onClick={(event) => {
                  if (!row.hasChildren) return
                  event.stopPropagation()
                  setFocusedId(row.id)
                  toggle(row)
                }}
                className={cn(
                  "-ml-1 grid w-6 shrink-0 self-stretch place-items-center text-muted",
                  row.hasChildren && "cursor-pointer hover:text-foreground",
                )}
              >
                {row.hasChildren && (
                  <ChevronRightIcon
                    size={14}
                    strokeWidth={1.75}
                    className={cn("transition-transform duration-200", expanded && "rotate-90")}
                  />
                )}
              </span>
              <span aria-hidden="true" className={cn("shrink-0", selected ? "text-accent-strong" : "text-muted")}>
                {row.icon ?? <Folder size={16} strokeWidth={1.75} />}
              </span>
              <span className={cn("min-w-0 flex-1 truncate", selected && "font-medium")}>{row.label}</span>
            </button>
          </Fragment>
        )
      })}
    </div>
  )
}
