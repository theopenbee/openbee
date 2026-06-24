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
    <div className="flex flex-col h-full">
      <div className="flex-1 overflow-y-auto py-2 space-y-0.5">
        <button
          onClick={() => onSelect(null)}
          className={cn(
            "w-full flex items-center gap-2 px-3 py-1.5 text-sm rounded-sm transition-colors",
            selectedId === null
              ? "bg-primary/10 text-primary font-medium"
              : "text-muted-foreground hover:bg-muted"
          )}
        >
          <UsersIcon className="size-4 shrink-0" />
          {t("departments.allWorkers")}
        </button>

        <button
          onClick={() => onSelect(UNGROUPED_FILTER)}
          className={cn(
            "w-full flex items-center gap-2 px-3 py-1.5 text-sm rounded-sm transition-colors",
            selectedId === UNGROUPED_FILTER
              ? "bg-primary/10 text-primary font-medium"
              : "text-muted-foreground hover:bg-muted"
          )}
        >
          <InboxIcon className="size-4 shrink-0" />
          {t("departments.ungrouped")}
        </button>

        {departments.length > 0 && (
          <div className="pt-2">
            <p className="px-3 pb-1 text-xs font-medium uppercase tracking-[0.1em] text-muted-foreground">
              {t("departments.title")}
            </p>
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
        className="w-full flex items-center gap-1.5 py-1.5 text-sm rounded-sm transition-colors"
        style={{ paddingLeft: `${depth * 16 + 12}px`, paddingRight: "12px" }}
      >
        {hasChildren ? (
          <button
            onClick={() => setExpanded(!expanded)}
            className="shrink-0 p-0.5 hover:bg-muted rounded"
          >
            <ChevronRightIcon
              className={cn("size-3.5 transition-transform", expanded && "rotate-90")}
            />
          </button>
        ) : (
          <span className="w-4.5 shrink-0" />
        )}
        <button
          onClick={() => onSelect(dept.id)}
          className={cn(
            "flex-1 flex items-center gap-1.5 text-left",
            selectedId === dept.id
              ? "text-primary font-medium"
              : "text-muted-foreground hover:text-foreground"
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
