import { useTranslation } from "react-i18next"
import type { TFunction } from "i18next"
import { MetricCard } from "@/components/arc/metric-card/metric-card"
import { LoadErrorAlert } from "@/components/dashboard-load-error"
import { useStatsOverview } from "@/hooks/use-stats"
import { formatTokenCount } from "@/lib/format"

function tokenMetric(n: number): { value: number; suffix?: string } {
  if (n >= 10_000_000_000) return { value: Math.round(n / 1_000_000_000), suffix: "B" }
  if (n >= 10_000_000) return { value: Math.round(n / 1_000_000), suffix: "M" }
  return { value: n }
}

function tokenComparison(today: number, yesterday: number, t: TFunction): string {
  if (yesterday <= 0) return t("dashboard.noComparison")
  const value = formatTokenCount(yesterday)
  const ratio = (today - yesterday) / yesterday
  const pct = `${Math.abs(ratio * 100).toFixed(1)}%`
  if (ratio > 0) return t("dashboard.tokensUp", "Up {{pct}} from {{value}} yesterday", { pct, value })
  if (ratio < 0) return t("dashboard.tokensDown", "Down {{pct}} from {{value}} yesterday", { pct, value })
  return t("dashboard.tokensFlat", "Same as yesterday ({{value}})", { value })
}

function MetricSkeleton() {
  return (
    <div className="rounded-surface border border-border bg-surface p-6" aria-hidden="true">
      <div className="flex h-5 items-center">
        <div className="skeleton h-4 w-24" />
      </div>
      <div className="skeleton mt-8 h-9 w-28" />
      <div className="mt-4 flex h-5 items-center">
        <div className="skeleton h-3.5 w-40" />
      </div>
    </div>
  )
}

export function DashboardMetrics() {
  const { t } = useTranslation()
  const { data, isLoading, error, refetch, isRefetching } = useStatsOverview()

  if (error && !data) {
    return (
      <LoadErrorAlert
        title={t("dashboard.loadError", "Overview numbers could not load")}
        error={error}
        retrying={isRefetching}
        onRetry={() => refetch()}
      />
    )
  }

  const tokens = data ? tokenMetric(data.tokens_today_total) : null

  return (
    <section
      aria-label={t("dashboard.metricsLabel", "Key numbers")}
      aria-busy={isLoading}
      className="grid items-stretch gap-4 @lg:grid-cols-2 @5xl:grid-cols-4"
    >
      {data && tokens ? (
        <>
          <MetricCard
            label={t("dashboard.workers")}
            value={data.workers}
            context={t("dashboard.workersContext", "Digital workers on this server")}
          />
          <MetricCard
            label={t("dashboard.departments")}
            value={data.departments}
            context={t("dashboard.departmentsContext", "Teams that organize your workers")}
          />
          <MetricCard
            label={t("dashboard.scheduledTasks")}
            value={data.scheduled_tasks}
            context={t("dashboard.scheduledTasksContext", "Recurring tasks waiting to run")}
          />
          <MetricCard
            label={t("dashboard.tokensToday")}
            value={tokens.value}
            suffix={tokens.suffix}
            context={tokenComparison(data.tokens_today_total, data.tokens_yesterday_total, t)}
          />
        </>
      ) : (
        Array.from({ length: 4 }, (_, i) => <MetricSkeleton key={i} />)
      )}
    </section>
  )
}
