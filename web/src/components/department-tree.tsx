import { Fragment, useState } from "react"
import { useTranslation } from "react-i18next"
import { ChevronRightIcon, FolderIcon, FolderOpenIcon, UsersIcon, InboxIcon } from "lucide-react"
import { cn } from "@/lib/utils"
import type { DepartmentTree as DepartmentTreeType } from "@/lib/types"

export const UNGROUPED_FILTER = "ungrouped" as const

interface DepartmentTreeProps {
  departments: DepartmentTreeType[]
  selectedId: string | null
  onSelect: (id: string | null) => void
}

export function DepartmentTreeSidebar({ departments, selectedId, onSelect }: DepartmentTreeProps) {
  const { t } = useTranslation()

  return (
    <div className="flex h-full flex-col">
      <div className="flex-1 space-y-0.5 overflow-y-auto px-2 py-3">
        <button
          onClick={() => onSelect(null)}
          className={cn(
            "flex w-full items-center gap-2.5 rounded-sm px-2.5 py-2 text-sm transition-colors",
            selectedId === null
              ? "bg-primary/10 font-medium text-primary"
              : "text-muted-foreground hover:bg-muted hover:text-foreground"
          )}
        >
          <UsersIcon className="size-4 shrink-0" />
          {t("departments.allWorkers")}
        </button>

        <button
          onClick={() => onSelect(UNGROUPED_FILTER)}
          className={cn(
            "flex w-full items-center gap-2.5 rounded-sm px-2.5 py-2 text-sm transition-colors",
            selectedId === UNGROUPED_FILTER
              ? "bg-primary/10 font-medium text-primary"
              : "text-muted-foreground hover:bg-muted hover:text-foreground"
          )}
        >
          <InboxIcon className="size-4 shrink-0" />
          {t("departments.ungrouped")}
        </button>

        {departments.length > 0 && (
          <div className="pt-4">
            <p className="px-2.5 pb-2 text-xs font-medium uppercase tracking-[0.1em] text-muted-foreground">
              {t("departments.title")}
            </p>
            <div className="space-y-0.5">
              {departments.map((dept) => (
                <DepartmentNode
                  key={dept.id}
                  dept={dept}
                  selectedId={selectedId}
                  onSelect={onSelect}
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
  depth,
}: {
  dept: DepartmentTreeType
  selectedId: string | null
  onSelect: (id: string) => void
  depth: number
}) {
  const [expanded, setExpanded] = useState(true)
  const hasChildren = dept.children.length > 0

  return (
    <Fragment>
      <div
        className="flex w-full items-center gap-1 pr-2 text-sm"
        style={{ paddingLeft: `${depth * 16 + 8}px` }}
      >
        {hasChildren ? (
          <button
            onClick={() => setExpanded(!expanded)}
            aria-label={dept.name}
            className="shrink-0 rounded-sm p-1 text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
          >
            <ChevronRightIcon
              className={cn("size-3.5 transition-transform", expanded && "rotate-90")}
            />
          </button>
        ) : (
          <span className="w-6 shrink-0" />
        )}
        <button
          onClick={() => onSelect(dept.id)}
          className={cn(
            "flex flex-1 items-center gap-2 rounded-sm px-2 py-2 text-left transition-colors",
            selectedId === dept.id
              ? "bg-primary/10 font-medium text-primary"
              : "text-muted-foreground hover:bg-muted hover:text-foreground"
          )}
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
          depth={depth + 1}
        />
      ))}
    </Fragment>
  )
}
