import { useState } from "react"
import { useTranslation } from "react-i18next"
import { ChevronRightIcon, FolderIcon, FolderOpenIcon, UsersIcon, InboxIcon } from "lucide-react"
import { cn } from "@/lib/utils"
import type { DepartmentTree as DepartmentTreeType } from "@/lib/types"

export const UNGROUPED_FILTER = "ungrouped" as const

interface DepartmentTreeProps {
  departments: DepartmentTreeType[]
  selectedId: string | null
  onSelect: (id: string | null) => void
  onManage: () => void
}

export function DepartmentTreeSidebar({ departments, selectedId, onSelect, onManage }: DepartmentTreeProps) {
  const { t } = useTranslation()

  return (
    <div className="flex flex-col h-full">
      <div className="flex-1 overflow-y-auto py-2 space-y-0.5">
        {/* All Workers */}
        <button
          onClick={() => onSelect(null)}
          className={cn(
            "w-full flex items-center gap-2 px-3 py-1.5 text-sm rounded-md transition-colors",
            selectedId === null
              ? "bg-primary/10 text-primary font-medium"
              : "text-muted-foreground hover:bg-muted"
          )}
        >
          <UsersIcon className="size-4 shrink-0" />
          {t("departments.allWorkers")}
        </button>

        {/* Ungrouped */}
        <button
          onClick={() => onSelect(UNGROUPED_FILTER)}
          className={cn(
            "w-full flex items-center gap-2 px-3 py-1.5 text-sm rounded-md transition-colors",
            selectedId === UNGROUPED_FILTER
              ? "bg-primary/10 text-primary font-medium"
              : "text-muted-foreground hover:bg-muted"
          )}
        >
          <InboxIcon className="size-4 shrink-0" />
          {t("departments.ungrouped")}
        </button>

        {/* Department tree */}
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

      <div className="border-t px-3 py-2">
        <button
          onClick={onManage}
          className="w-full text-xs text-muted-foreground hover:text-foreground transition-colors text-center py-1"
        >
          {t("departments.manage")}
        </button>
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
    <div>
      <button
        onClick={() => onSelect(dept.id)}
        className={cn(
          "w-full flex items-center gap-1.5 py-1.5 text-sm rounded-md transition-colors",
          selectedId === dept.id
            ? "bg-primary/10 text-primary font-medium"
            : "text-muted-foreground hover:bg-muted"
        )}
        style={{ paddingLeft: `${depth * 16 + 12}px`, paddingRight: "12px" }}
      >
        {hasChildren ? (
          <button
            onClick={(e) => {
              e.stopPropagation()
              setExpanded(!expanded)
            }}
            className="shrink-0 p-0.5 hover:bg-muted rounded"
          >
            <ChevronRightIcon
              className={cn("size-3.5 transition-transform", expanded && "rotate-90")}
            />
          </button>
        ) : (
          <span className="w-4.5 shrink-0" />
        )}
        {expanded && hasChildren ? (
          <FolderOpenIcon className="size-4 shrink-0" />
        ) : (
          <FolderIcon className="size-4 shrink-0" />
        )}
        <span className="truncate">{dept.name}</span>
      </button>

      {expanded && hasChildren && (
        <div>
          {dept.children.map((child) => (
            <DepartmentNode
              key={child.id}
              dept={child}
              selectedId={selectedId}
              onSelect={onSelect}
              depth={depth + 1}
            />
          ))}
        </div>
      )}
    </div>
  )
}
