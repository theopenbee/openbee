import { useTranslation } from "react-i18next"
import { Input } from "@/components/arc/input/input"
import type { EngineChoice } from "@/lib/types"

interface EngineArgsSectionProps {
  engines: readonly EngineChoice[]
  value: Record<string, string>
  onChange: (args: Record<string, string>) => void
  idPrefix?: string
  disabled?: boolean
}

export function EngineArgsSection({ engines, value, onChange, idPrefix = "engine-args", disabled }: EngineArgsSectionProps) {
  const { t } = useTranslation()
  const single = engines.length === 1

  if (engines.length === 0) {
    return <p className="text-sm text-secondary">{t("workers.form.noEngine")}</p>
  }

  return (
    <div className="grid gap-4">
      {engines.map((engine) => (
        <Input
          key={engine.id}
          id={`${idPrefix}-${engine.id}`}
          label={single ? t("workers.form.engineArgs") : engine.name}
          description={single ? t("workers.form.engineArgsHelper", { engine: engine.name }) : undefined}
          value={value[engine.id] ?? ""}
          onChange={(e) => onChange({ ...value, [engine.id]: e.target.value })}
          placeholder={t("workers.form.engineArgsPlaceholder")}
          autoComplete="off"
          spellCheck={false}
          disabled={disabled}
        />
      ))}
    </div>
  )
}
