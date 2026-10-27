import { useState } from "react"
import { useTranslation } from "react-i18next"
import { InfoIcon } from "lucide-react"
import { Popover, PopoverContent, PopoverTrigger } from "@/components/arc/popover/popover"
import type { SessionTokenStats } from "@/lib/types"

export function TokenStatsInfoButton({
  stats,
  side = "bottom",
  align = "start",
}: {
  stats: SessionTokenStats
  side?: "bottom" | "left" | "right" | "top"
  align?: "start" | "center" | "end"
}) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger
        aria-label={t("tokenStats.breakdown", "Token breakdown")}
        onPointerEnter={(event) => event.pointerType === "mouse" && setOpen(true)}
        onPointerLeave={(event) => event.pointerType === "mouse" && setOpen(false)}
        className="inline-flex items-center rounded-full text-muted transition-colors hover:text-foreground pointer-coarse:-m-2.5 pointer-coarse:p-2.5"
      >
        <InfoIcon size={14} strokeWidth={1.75} aria-hidden="true" />
      </PopoverTrigger>
      <PopoverContent side={side} align={align} onOpenAutoFocus={(event) => event.preventDefault()}>
        <TokenStatsTooltip stats={stats} />
      </PopoverContent>
    </Popover>
  )
}

export function TokenStatsTooltip({ stats }: { stats: SessionTokenStats }) {
  const { t } = useTranslation()
  const sorted = [...stats.by_model].sort((a, b) => b.total_tokens - a.total_tokens)
  return (
    <div className="flex min-w-44 flex-col gap-2 text-xs tabular">
      <div className="flex justify-between gap-4 font-medium text-foreground">
        <span>{t("tokenStats.total", "Total")}</span>
        <span>{stats.total_tokens.toLocaleString()}</span>
      </div>
      <div className="flex flex-col gap-2 border-t border-border-subtle pt-2">
        {sorted.length === 0 ? (
          <span className="text-muted">{t("tokenStats.noModels", "No model data")}</span>
        ) : sorted.map((m) => (
          <div key={m.model} className="flex flex-col gap-0.5">
            <div className="flex justify-between gap-4 text-foreground">
              <span className="font-mono">{m.model}</span>
              <span>{m.total_tokens.toLocaleString()}</span>
            </div>
            <div className="flex gap-3 text-secondary">
              <span>{t("tokenStats.input", "In")} {m.input_tokens.toLocaleString()}</span>
              <span>{t("tokenStats.output", "Out")} {m.output_tokens.toLocaleString()}</span>
            </div>
            {(m.cache_creation_tokens > 0 || m.cache_read_tokens > 0) && (
              <div className="flex gap-3 text-secondary">
                <span>{t("tokenStats.cacheWrite", "Cache write")} {m.cache_creation_tokens.toLocaleString()}</span>
                <span>{t("tokenStats.cacheRead", "Cache read")} {m.cache_read_tokens.toLocaleString()}</span>
              </div>
            )}
          </div>
        ))}
      </div>
    </div>
  )
}
