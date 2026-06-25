import { useTranslation } from "react-i18next"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { EngineIcon } from "@/components/agent-icons/engine-icon"
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
          <span className="flex w-16 shrink-0 items-center gap-2 text-xs font-medium text-muted-foreground">
            <EngineIcon engine={engine} className="size-4 text-foreground/70" />
            <span className="capitalize">{engine}</span>
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
    </div>
  )
}
