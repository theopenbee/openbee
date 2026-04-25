import { useTranslation } from "react-i18next"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import type { Engine } from "@/lib/types"

interface EngineArgsSectionProps {
  engines: Engine[]
  value: Record<string, string>
  onChange: (args: Record<string, string>) => void
}

export function EngineArgsSection({ engines, value, onChange }: EngineArgsSectionProps) {
  const { t } = useTranslation()

  if (engines.length === 0) return null

  return (
    <div className="space-y-2">
      <Label>{t("workers.form.engineArgs")}</Label>
      <div className="space-y-2">
        {engines.map((eng) => (
          <div key={eng} className="space-y-1">
            <span className="text-xs font-medium text-muted-foreground capitalize">{eng}</span>
            <Input
              value={value[eng] ?? ""}
              onChange={(e) =>
                onChange({ ...value, [eng]: e.target.value })
              }
              placeholder={t("workers.form.engineArgsPlaceholder")}
              className="font-mono text-xs"
            />
          </div>
        ))}
      </div>
      <p className="text-xs text-muted-foreground">{t("workers.form.engineArgsHelper")}</p>
    </div>
  )
}
