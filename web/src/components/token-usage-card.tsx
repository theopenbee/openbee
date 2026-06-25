import { useTranslation } from "react-i18next"
import { TrendingUp, TrendingDown, Minus } from "lucide-react"
import { SectionRule } from "@/components/section-rule"
import { Skeleton } from "@/components/ui/skeleton"
import { TokenTrendChart } from "@/components/token-trend-chart"
import { useStatsOverview } from "@/hooks/use-stats"
import { formatChange, formatTokenCount } from "@/lib/format"

const STAT_LABEL =
  "text-[11px] font-medium uppercase tracking-[0.18em] text-muted-foreground"

// Direction is carried by the arrow alone; the delta text stays neutral so the
// presence palette (green/purple/red) is never co-opted as a trend valence.
function changeMeta(ratio: number | null) {
  const Icon = ratio === null ? null : ratio > 0 ? TrendingUp : ratio < 0 ? TrendingDown : Minus
  return { label: formatChange(ratio), Icon }
}

export function TokenUsageCard() {
  const { t } = useTranslation()
  const { data, isLoading } = useStatsOverview()

  const today = data?.tokens_today_total ?? 0
  const yesterday = data?.tokens_yesterday_total ?? 0
  const ratio = yesterday > 0 ? (today - yesterday) / yesterday : null
  const change = changeMeta(ratio)

  return (
    <section aria-label={t("dashboard.tokenUsage")}>
      <SectionRule className="mb-4">{t("dashboard.tokenUsage")}</SectionRule>

      <div className="grid grid-cols-3 rounded-sm border border-border/70">
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
        <div className="border-l border-border/70 p-5">
          <p className={STAT_LABEL}>{t("dashboard.dayOverDay")}</p>
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

      <div className="mt-4">
        <TokenTrendChart />
      </div>
    </section>
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
    <div className={divider ? "border-l border-border/70 p-5" : "p-5"}>
      <p className={STAT_LABEL}>{label}</p>
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
