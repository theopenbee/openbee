import { useState } from "react"
import { useTranslation } from "react-i18next"
import { Checkbox } from "@/components/arc/checkbox/checkbox"
import { SearchField } from "@/components/arc/search-field/search-field"
import { useFlatDepartments } from "@/hooks/use-departments"

const SEARCH_THRESHOLD = 6

interface WorkerDepartmentPickerProps {
  selected: Set<string>
  onChange: (next: Set<string>) => void
  disabled?: boolean
}

export function WorkerDepartmentPicker({ selected, onChange, disabled }: WorkerDepartmentPickerProps) {
  const { t } = useTranslation()
  const flatDepts = useFlatDepartments()
  const [query, setQuery] = useState("")

  const needle = query.trim().toLowerCase()
  const visible = needle
    ? flatDepts.filter(({ dept }) => dept.name.toLowerCase().includes(needle))
    : flatDepts

  const toggle = (id: string, on: boolean) => {
    const next = new Set(selected)
    if (on) next.add(id)
    else next.delete(id)
    onChange(next)
  }

  return (
    <div className="grid gap-3">
      {flatDepts.length > SEARCH_THRESHOLD && (
        <SearchField
          label={t("workers.form.searchDepartments")}
          placeholder={t("workers.form.searchDepartmentsPlaceholder")}
          value={query}
          onValueChange={setQuery}
        />
      )}
      <div className="relative max-h-96 overflow-y-auto overscroll-contain rounded-panel border border-border bg-surface px-2 py-1">
        {visible.length === 0 ? (
          <p className="px-3 py-4 text-sm text-secondary">{t("workers.form.noMatchingDepartments")}</p>
        ) : (
          <ul>
            {visible.map(({ dept, depth }) => (
              <li key={dept.id} style={{ paddingInlineStart: needle ? 0 : depth * 24 }}>
                <Checkbox
                  label={dept.name}
                  checked={selected.has(dept.id)}
                  onCheckedChange={(state) => toggle(dept.id, state === true)}
                  disabled={disabled}
                />
              </li>
            ))}
          </ul>
        )}
      </div>
    </div>
  )
}
