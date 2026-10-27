import { useTranslation } from "react-i18next"
import { Input } from "@/components/arc/input/input"
import { formatEngineLabel } from "@/lib/format"
import type { Engine } from "@/lib/types"

interface EngineArgsSectionProps {
  engines: readonly Engine[]
  value: Record<string, string>
  onChange: (args: Record<string, string>) => void
  idPrefix?: string
  disabled?: boolean
}

export function EngineArgsSection({ engines, value, onChange, idPrefix = "engine-args", disabled }: EngineArgsSectionProps) {
  const { t } = useTranslation()
  const single = engines.length === 1

  return (
    <div className="grid gap-4">
      {engines.map((engine) => {
        const engineLabel = formatEngineLabel(engine, t)
        return (
          <Input
            key={engine}
            id={`${idPrefix}-${engine}`}
            label={single ? t("workers.form.engineArgs") : engineLabel}
            description={single ? t("workers.form.engineArgsHelper", { engine: engineLabel }) : undefined}
            value={value[engine] ?? ""}
            onChange={(e) => onChange({ ...value, [engine]: e.target.value })}
            placeholder={t("workers.form.engineArgsPlaceholder")}
            autoComplete="off"
            spellCheck={false}
            disabled={disabled}
          />
        )
      })}
    </div>
  )
}
