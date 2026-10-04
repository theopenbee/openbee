import { useState } from "react"
import { useTranslation } from "react-i18next"
import { Search } from "lucide-react"
import { useFlatDepartments } from "@/hooks/use-departments"
import { Input } from "@/components/ui/input"
import { cn } from "@/lib/utils"

// Searchable department checklist shared by the create-worker form and the
// edit-worker sheet. `flush` lays it edge to edge inside a flush Panel (search
// strip over a padded list); otherwise it stacks inside an already padded
// section. Names wrap rather than truncate so similar names stay distinct.
export function DepartmentChecklist({
  selected,
  onChange,
  flush = false,
}: {
  selected: ReadonlySet<string>
  onChange: (next: Set<string>) => void
  flush?: boolean
}) {
  const { t } = useTranslation()
  const flatDepts = useFlatDepartments()
  const [search, setSearch] = useState("")
  const query = search.trim().toLowerCase()
  const filteredDepts = query
    ? flatDepts.filter(({ dept }) => dept.name.toLowerCase().includes(query))
    : flatDepts

  const toggle = (id: string, checked: boolean) => {
    const next = new Set(selected)
    if (checked) next.add(id)
    else next.delete(id)
    onChange(next)
  }

  return (
    <div className={cn(!flush && "space-y-3")}>
      <div className={cn(flush && "border-b border-hairline p-3")}>
        <div className="relative">
          <Search className="pointer-events-none absolute top-1/2 left-2.5 size-3.5 -translate-y-1/2 text-muted-foreground" />
          <Input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            // Both hosts render this inside a <form>; Enter here only filters.
            onKeyDown={(e) => {
              if (e.key === "Enter") e.preventDefault()
            }}
            placeholder={t("workers.form.searchDepartments")}
            aria-label={t("workers.form.searchDepartments")}
            className="pl-8"
          />
        </div>
      </div>

      <div className={cn("max-h-56 overflow-y-auto", flush ? "p-1.5" : "-mx-1.5")}>
        {filteredDepts.length === 0 ? (
          <p className="py-4 text-center text-body-sm text-muted-foreground">
            {t("workers.form.noMatchingDepartments")}
          </p>
        ) : (
          filteredDepts.map(({ dept, depth }) => (
            <label
              key={dept.id}
              className="flex min-h-8.5 cursor-pointer items-center gap-2.5 rounded-sm px-3 py-1.5 transition-colors hover:bg-accent"
              style={{ paddingLeft: `${12 + depth * 16}px` }}
            >
              <input
                type="checkbox"
                checked={selected.has(dept.id)}
                onChange={(e) => toggle(dept.id, e.target.checked)}
                className="size-4 shrink-0 cursor-pointer rounded-sm accent-primary"
              />
              <span className="min-w-0 text-sm leading-5 break-words text-foreground">{dept.name}</span>
            </label>
          ))
        )}
      </div>
    </div>
  )
}
