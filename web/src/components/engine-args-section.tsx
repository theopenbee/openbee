import { useTranslation } from "react-i18next"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { EngineIcon } from "@/components/agent-icons/engine-icon"
import { formatEngineLabel } from "@/lib/format"
import { cn } from "@/lib/utils"
import type { Engine } from "@/lib/types"

interface EngineArgsSectionProps {
  engines: readonly Engine[]
  value: Record<string, string>
  onChange: (args: Record<string, string>) => void
  showLabel?: boolean
}

export function EngineArgsSection({ engines, value, onChange, showLabel = true }: EngineArgsSectionProps) {
  const { t } = useTranslation()

  // Label above; one row per engine with the engine named beside its input.
  // Several engines share a fixed name column so their inputs line up.
  // The visible label can only point at one input, so whenever it doesn't
  // (several engines, or no label) each input names its own engine.
  const labelTargetsInput = showLabel && engines.length === 1

  return (
    <div className="space-y-2">
      {showLabel && (
        <Label htmlFor={labelTargetsInput ? `engine-args-${engines[0]}` : undefined}>
          {t("workers.form.engineArgs")}
        </Label>
      )}
      {engines.map((engine) => (
        <div key={engine} className="flex items-center gap-3">
          <span
            className={cn(
              "flex shrink-0 items-center gap-2 text-body-sm text-foreground",
              engines.length > 1 && "w-28"
            )}
          >
            <EngineIcon engine={engine} className="size-4" />
            <span className="truncate">{formatEngineLabel(engine, t)}</span>
          </span>
          <Input
            id={`engine-args-${engine}`}
            aria-label={
              labelTargetsInput
                ? undefined
                : t("workers.form.engineArgsFor", { engine: formatEngineLabel(engine, t) })
            }
            value={value[engine] ?? ""}
            onChange={(e) =>
              onChange({ ...value, [engine]: e.target.value })
            }
            placeholder={t("workers.form.engineArgsPlaceholder")}
            className="min-w-0 flex-1 font-mono text-base md:text-body-sm"
          />
        </div>
      ))}
    </div>
  )
}
