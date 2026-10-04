import { Info } from "lucide-react"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
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
  return (
    <Tooltip>
      <TooltipTrigger
        type="button"
        aria-label="Token breakdown"
        className="inline-flex size-5 shrink-0 items-center justify-center rounded-sm text-muted-foreground transition-colors outline-none hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring"
      >
        <Info className="size-3.5" />
      </TooltipTrigger>
      <TooltipContent side={side} align={align}>
        <TokenStatsTooltip stats={stats} />
      </TooltipContent>
    </Tooltip>
  )
}

// Rendered inside the inverted tooltip surface (bg-strong / text-background),
// so secondary text steps down via the background tone's opacity.
export function TokenStatsTooltip({ stats }: { stats: SessionTokenStats }) {
  const sorted = [...stats.by_model].sort((a, b) => b.total_tokens - a.total_tokens)
  return (
    <div className="flex min-w-44 flex-col gap-2 py-0.5 text-xs">
      <div className="flex justify-between gap-6 font-semibold">
        <span>Total</span>
        <span className="tabular-nums">{stats.total_tokens.toLocaleString()}</span>
      </div>
      <div className="flex flex-col gap-2 border-t border-background/20 pt-2">
        {sorted.length === 0 ? (
          <span className="text-background/70">No model data</span>
        ) : sorted.map((m) => (
          <div key={m.model} className="flex flex-col gap-0.5">
            <div className="flex justify-between gap-6">
              <span className="font-mono">{m.model}</span>
              <span className="font-medium tabular-nums">{m.total_tokens.toLocaleString()}</span>
            </div>
            <div className="flex gap-3 text-background/70 tabular-nums">
              <span>In {m.input_tokens.toLocaleString()}</span>
              <span>Out {m.output_tokens.toLocaleString()}</span>
            </div>
            {(m.cache_creation_tokens > 0 || m.cache_read_tokens > 0) && (
              <div className="flex gap-3 text-background/70 tabular-nums">
                <span>Cache↑ {m.cache_creation_tokens.toLocaleString()}</span>
                <span>Cache↓ {m.cache_read_tokens.toLocaleString()}</span>
              </div>
            )}
          </div>
        ))}
      </div>
    </div>
  )
}
