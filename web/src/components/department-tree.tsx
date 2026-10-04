import { Fragment } from "react"
import { useTranslation } from "react-i18next"
import { ChevronRightIcon, FolderIcon, FolderOpenIcon, UsersIcon, InboxIcon } from "lucide-react"
import { cn } from "@/lib/utils"
import { FIELD_LABEL, RAIL_ITEM, RAIL_ITEM_IDLE, RAIL_ITEM_SELECTED } from "@/lib/styles"
import type { DepartmentTree as DepartmentTreeType } from "@/lib/types"

export const UNGROUPED_FILTER = "ungrouped" as const

// Expansion is controlled by the caller (a set of collapsed ids, so nodes start
// expanded), which lets several mounted copies of the tree stay in sync.
interface DepartmentTreeProps {
  departments: DepartmentTreeType[]
  selectedId: string | null
  onSelect: (id: string | null) => void
  collapsedIds: ReadonlySet<string>
  onToggle: (id: string) => void
}

export function DepartmentTreeSidebar({ departments, selectedId, onSelect, collapsedIds, onToggle }: DepartmentTreeProps) {
  const { t } = useTranslation()

  return (
    <div className="flex h-full flex-col">
      <div className="flex-1 space-y-0.5 overflow-y-auto px-2 py-3">
        <button
          type="button"
          onClick={() => onSelect(null)}
          aria-pressed={selectedId === null}
          className={cn(RAIL_ITEM, selectedId === null ? RAIL_ITEM_SELECTED : RAIL_ITEM_IDLE)}
        >
          <UsersIcon className="size-4 shrink-0" />
          <span className="truncate">{t("departments.allWorkers")}</span>
        </button>

        <button
          type="button"
          onClick={() => onSelect(UNGROUPED_FILTER)}
          aria-pressed={selectedId === UNGROUPED_FILTER}
          className={cn(RAIL_ITEM, selectedId === UNGROUPED_FILTER ? RAIL_ITEM_SELECTED : RAIL_ITEM_IDLE)}
        >
          <InboxIcon className="size-4 shrink-0" />
          <span className="truncate">{t("departments.ungrouped")}</span>
        </button>

        {departments.length > 0 && (
          <div className="pt-4">
            <p className={cn(FIELD_LABEL, "px-3 pb-1.5")}>
              {t("departments.title")}
            </p>
            <div className="space-y-0.5">
              {departments.map((dept) => (
                <DepartmentNode
                  key={dept.id}
                  dept={dept}
                  selectedId={selectedId}
                  onSelect={onSelect}
                  collapsedIds={collapsedIds}
                  onToggle={onToggle}
                  depth={0}
                />
              ))}
            </div>
          </div>
        )}
      </div>
    </div>
  )
}

function DepartmentNode({
  dept,
  selectedId,
  onSelect,
  collapsedIds,
  onToggle,
  depth,
}: {
  dept: DepartmentTreeType
  selectedId: string | null
  onSelect: (id: string) => void
  collapsedIds: ReadonlySet<string>
  onToggle: (id: string) => void
  depth: number
}) {
  const expanded = !collapsedIds.has(dept.id)
  const hasChildren = dept.children.length > 0
  const isSelected = selectedId === dept.id

  return (
    <Fragment>
      <div
        className="flex w-full items-center gap-0.5"
        style={{ paddingLeft: `${depth * 16}px` }}
      >
        {hasChildren ? (
          <button
            type="button"
            onClick={() => onToggle(dept.id)}
            aria-label={dept.name}
            aria-expanded={expanded}
            className="flex size-6 shrink-0 items-center justify-center rounded-sm text-muted-foreground transition-colors hover:bg-accent hover:text-strong focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
          >
            <ChevronRightIcon
              className={cn("size-3.5 transition-transform", expanded && "rotate-90")}
            />
          </button>
        ) : (
          <span className="w-6 shrink-0" />
        )}
        <button
          type="button"
          onClick={() => onSelect(dept.id)}
          aria-pressed={isSelected}
          className={cn(RAIL_ITEM, "min-w-0 flex-1 px-2", isSelected ? RAIL_ITEM_SELECTED : RAIL_ITEM_IDLE)}
        >
          {expanded && hasChildren ? (
            <FolderOpenIcon className="size-4 shrink-0" />
          ) : (
            <FolderIcon className="size-4 shrink-0" />
          )}
          <span className="truncate">{dept.name}</span>
        </button>
      </div>

      {expanded && hasChildren && dept.children.map((child) => (
        <DepartmentNode
          key={child.id}
          dept={child}
          selectedId={selectedId}
          onSelect={onSelect}
          collapsedIds={collapsedIds}
          onToggle={onToggle}
          depth={depth + 1}
        />
      ))}
    </Fragment>
  )
}
