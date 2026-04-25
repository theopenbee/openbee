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
        className="flex items-center text-muted-foreground/40 hover:text-muted-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring transition-colors"
      >
        <Info className="size-3" />
      </TooltipTrigger>
      <TooltipContent side={side} align={align}>
        <TokenStatsTooltip stats={stats} />
      </TooltipContent>
    </Tooltip>
  )
}

export function TokenStatsTooltip({ stats }: { stats: SessionTokenStats }) {
  const sorted = [...stats.by_model].sort((a, b) => b.total_tokens - a.total_tokens)
  return (
    <div className="flex flex-col gap-1.5 font-mono text-xs min-w-[160px]">
      <div className="flex justify-between gap-4 font-semibold">
        <span>Total</span>
        <span>{stats.total_tokens.toLocaleString()}</span>
      </div>
      <div className="border-t border-background/20 pt-1 flex flex-col gap-2">
        {sorted.length === 0 ? (
          <span className="opacity-60">No model data</span>
        ) : sorted.map((m) => (
          <div key={m.model} className="flex flex-col gap-0.5">
            <div className="flex justify-between gap-4">
              <span className="opacity-90">{m.model}</span>
              <span>{m.total_tokens.toLocaleString()}</span>
            </div>
            <div className="flex gap-3 opacity-60 pl-1">
              <span>In {m.input_tokens.toLocaleString()}</span>
              <span>Out {m.output_tokens.toLocaleString()}</span>
            </div>
            {(m.cache_creation_tokens > 0 || m.cache_read_tokens > 0) && (
              <div className="flex gap-3 opacity-60 pl-1">
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
