import { useTranslation } from "react-i18next"
import { TrendingUp, TrendingDown, Minus } from "lucide-react"
import { Panel } from "@/components/panel"
import { Skeleton } from "@/components/ui/skeleton"
import { TokenTrendChart } from "@/components/token-trend-chart"
import { useStatsOverview } from "@/hooks/use-stats"
import { formatChange, formatTokenCount } from "@/lib/format"
import { EYEBROW_LABEL } from "@/lib/styles"

// Direction is carried by the arrow alone; the delta text stays neutral so the
// presence palette (green/purple/red) is never co-opted as a trend valence.
function changeMeta(ratio: number | null) {
  const Icon = ratio === null ? null : ratio > 0 ? TrendingUp : ratio < 0 ? TrendingDown : Minus
  return { label: formatChange(ratio), Icon }
}

// Token usage and its trend live in one board: the headline metrics sit directly
// under the title, then a single hairline divides them from the trend chart
// below. One surface, two reads.
export function TokenUsageCard() {
  const { t } = useTranslation()
  const { data, isLoading } = useStatsOverview()

  const today = data?.tokens_today_total ?? 0
  const yesterday = data?.tokens_yesterday_total ?? 0
  const ratio = yesterday > 0 ? (today - yesterday) / yesterday : null
  const change = changeMeta(ratio)

  return (
    <Panel title={t("dashboard.tokenUsage")} ariaLabel={t("dashboard.tokenUsage")} flush>
      <div className="grid grid-cols-3">
        <Metric
          label={t("dashboard.tokensToday")}
          isLoading={isLoading}
          value={formatTokenCount(today)}
          valueClass="text-2xl font-semibold"
        />
        <Metric
          label={t("dashboard.tokensYesterday")}
          isLoading={isLoading}
          value={formatTokenCount(yesterday)}
          valueClass="text-2xl font-medium text-muted-foreground"
          divider
        />
        <div className="border-l border-border/70 px-5 py-4">
          <p className={EYEBROW_LABEL}>{t("dashboard.dayOverDay")}</p>
          <div className="mt-2.5 flex h-7 items-center">
            {isLoading ? (
              <Skeleton className="h-6 w-16" />
            ) : change.label !== null && change.Icon ? (
              <span className="flex items-center gap-1.5 text-muted-foreground" aria-label={change.label}>
                <change.Icon className="size-4" aria-hidden />
                <span className="text-xl font-semibold tabular-nums">{change.label}</span>
              </span>
            ) : (
              <span className="text-xl font-medium text-muted-foreground" aria-label={t("dashboard.noComparison")}>
                —
              </span>
            )}
          </div>
        </div>
      </div>

      <div className="border-t border-border/70 px-5 pt-4 pb-1">
        <TokenTrendChart />
      </div>
    </Panel>
  )
}

function Metric({
  label,
  value,
  valueClass,
  isLoading,
  divider,
}: {
  label: string
  value: string
  valueClass: string
  isLoading: boolean
  divider?: boolean
}) {
  return (
    <div className={divider ? "border-l border-border/70 px-5 py-4" : "px-5 py-4"}>
      <p className={EYEBROW_LABEL}>{label}</p>
      <div className="mt-2.5 flex h-7 items-center">
        {isLoading ? (
          <Skeleton className="h-6 w-20" />
        ) : (
          <p className={`${valueClass} tabular-nums leading-none`}>{value}</p>
        )}
      </div>
    </div>
  )
}
