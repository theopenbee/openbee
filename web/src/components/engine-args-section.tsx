import { useTranslation } from "react-i18next"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import type { Engine } from "@/lib/types"

interface EngineArgsSectionProps {
  engine: Engine
  value: Record<string, string>
  onChange: (args: Record<string, string>) => void
}

export function EngineArgsSection({ engine, value, onChange }: EngineArgsSectionProps) {
  const { t } = useTranslation()

  return (
    <div className="space-y-1.5">
      <Label htmlFor={`engine-args-${engine}`}>
        {t("workers.form.engineArgs")}
        <span className="ml-1 text-xs font-normal text-muted-foreground capitalize">
          ({engine})
        </span>
      </Label>
      <Input
        id={`engine-args-${engine}`}
        value={value[engine] ?? ""}
        onChange={(e) =>
          onChange({ ...value, [engine]: e.target.value })
        }
        placeholder={t("workers.form.engineArgsPlaceholder")}
        className="font-mono text-xs"
      />
      <p className="text-xs text-muted-foreground">{t("workers.form.engineArgsHelper")}</p>
    </div>
  )
}
