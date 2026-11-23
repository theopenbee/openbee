import { useTranslation } from "react-i18next"
import { ModelField } from "@/components/model-field"
import type { EngineChoice } from "@/lib/types"

interface EngineModelsSectionProps {
  engines: readonly EngineChoice[]
  value: Record<string, string>
  onChange: (models: Record<string, string>) => void
  defaults?: Record<string, string>
  idPrefix: string
  disabled?: boolean
}

export function EngineModelsSection({ engines, value, onChange, defaults, idPrefix, disabled }: EngineModelsSectionProps) {
  const { t } = useTranslation()
  const single = engines.length === 1

  if (engines.length === 0) {
    return <p className="text-sm text-secondary">{t("workers.form.noEngine")}</p>
  }

  return (
    <div className="grid gap-4">
      {engines.map((engine) => (
        <ModelField
          key={engine.id}
          id={`${idPrefix}-${engine.id}`}
          engine={engine}
          label={single ? undefined : engine.name}
          description={single ? undefined : ""}
          defaultModel={defaults?.[engine.id]}
          value={value[engine.id] ?? ""}
          onChange={(model) => onChange({ ...value, [engine.id]: model })}
          disabled={disabled}
        />
      ))}
    </div>
  )
}
