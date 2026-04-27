import { useTranslation } from "react-i18next"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import type { Engine } from "@/lib/types"

interface EngineArgsSectionProps {
  engines: readonly Engine[]
  value: Record<string, string>
  onChange: (args: Record<string, string>) => void
}

export function EngineArgsSection({ engines, value, onChange }: EngineArgsSectionProps) {
  const { t } = useTranslation()

  return (
    <div className="space-y-2">
      <Label>{t("workers.form.engineArgs")}</Label>
      {engines.map((engine) => (
        <div key={engine} className="flex items-center gap-3">
          <span className="w-16 shrink-0 text-xs font-medium text-muted-foreground capitalize">
            {engine}
          </span>
          <Input
            id={`engine-args-${engine}`}
            value={value[engine] ?? ""}
            onChange={(e) =>
              onChange({ ...value, [engine]: e.target.value })
            }
            placeholder={t("workers.form.engineArgsPlaceholder")}
            className="flex-1 font-mono text-xs"
          />
        </div>
      ))}
      <p className="text-xs text-muted-foreground">{t("workers.form.engineArgsHelper")}</p>
    </div>
  )
}
