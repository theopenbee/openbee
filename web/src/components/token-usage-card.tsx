import type { ReactNode } from "react"
import { useTranslation } from "react-i18next"
import { ArrowUp, ArrowDown, Minus } from "lucide-react"
import { Panel } from "@/components/panel"
import { Skeleton } from "@/components/ui/skeleton"
import { TokenTrendChart } from "@/components/token-trend-chart"
import { useStatsOverview } from "@/hooks/use-stats"
import { formatChange, formatTokenCount } from "@/lib/format"
import { FIELD_LABEL } from "@/lib/styles"
import { cn } from "@/lib/utils"

const VALUE_CLASS = "text-2xl font-semibold tabular-nums leading-none"

// Token usage and its trend live in one panel: today / yesterday / day-over-day
// in a hairline-divided row, then the trend chart beneath. The delta is neutral
// (arrow + strong text): more tokens is neither good nor bad on its own, so it
// carries no success/danger color.
export function TokenUsageCard() {
  const { t } = useTranslation()
  const { data, isLoading } = useStatsOverview()

  const today = data?.tokens_today_total ?? 0
  const yesterday = data?.tokens_yesterday_total ?? 0
  const ratio = yesterday > 0 ? (today - yesterday) / yesterday : null
  const changeLabel = formatChange(ratio)
  const ChangeIcon = ratio === null ? null : ratio > 0 ? ArrowUp : ratio < 0 ? ArrowDown : Minus
  const changeTone = ratio === null || ratio === 0 ? "text-muted-foreground" : "text-strong"

  return (
    <Panel title={t("dashboard.tokenUsage")} ariaLabel={t("dashboard.tokenUsage")} flush>
      {/* Three across from sm; under it the delta drops to its own full-width
          row so a long percentage never collides with the panel edge. */}
      <div className="grid grid-cols-2 sm:grid-cols-3">
        <Metric label={t("dashboard.tokensToday")} isLoading={isLoading}>
          <p className={cn(VALUE_CLASS, "text-strong")}>{formatTokenCount(today)}</p>
        </Metric>
        <Metric label={t("dashboard.tokensYesterday")} isLoading={isLoading} className="border-l border-hairline">
          <p className={cn(VALUE_CLASS, "text-foreground")}>{formatTokenCount(yesterday)}</p>
        </Metric>
        <Metric
          label={t("dashboard.dayOverDay")}
          isLoading={isLoading}
          className="col-span-2 border-t border-hairline sm:col-span-1 sm:border-t-0 sm:border-l"
        >
          {changeLabel !== null && ChangeIcon ? (
            <span className={cn("flex items-center gap-1", changeTone)} aria-label={changeLabel}>
              <ChangeIcon className="size-5 shrink-0" aria-hidden />
              <span className={VALUE_CLASS}>{changeLabel}</span>
            </span>
          ) : (
            <span className={cn(VALUE_CLASS, "text-muted-foreground")} aria-label={t("dashboard.noComparison")}>
              —
            </span>
          )}
        </Metric>
      </div>

      <div className="border-t border-hairline px-4 pt-3 pb-2">
        <TokenTrendChart />
      </div>
    </Panel>
  )
}

function Metric({
  label,
  isLoading,
  className,
  children,
}: {
  label: string
  isLoading: boolean
  className?: string
  children: ReactNode
}) {
  return (
    <div className={cn("min-w-0 px-4 py-4", className)}>
      <p className={FIELD_LABEL}>{label}</p>
      <div className="mt-2 flex h-8 items-center">
        {isLoading ? <Skeleton className="h-6 w-16" /> : children}
      </div>
    </div>
  )
}
