import type { ReactNode } from "react"
import { useTranslation } from "react-i18next"
import { TrendingUp, TrendingDown, Minus, Info } from "lucide-react"
import { FadeIn } from "@/components/fade-in"
import { PageHeader } from "@/components/page-header"
import { SectionRule } from "@/components/section-rule"
import { Skeleton } from "@/components/ui/skeleton"
import { CombinedTrendChart } from "@/components/combined-trend-chart"
import { TokenTrendChart } from "@/components/token-trend-chart"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import { useStatsOverview } from "@/hooks/use-stats"
import { formatChange, formatTotalDuration, formatTokenCount, formatNumber } from "@/lib/format"
import type { StatsOverview } from "@/lib/types"

const EMPTY: StatsOverview = {
  departments: 0,
  workers: 0,
  active_workers_today: 0,
  active_workers_yesterday: 0,
  active_workers_change: null,
  messages_total_today: 0,
  messages_total_yesterday: 0,
  messages_change: null,
  messages_total_global: 0,
  executions_today: 0,
  executions_yesterday: 0,
  executions_change: null,
  exec_duration_today_ms: 0,
  exec_duration_yesterday_ms: 0,
  exec_duration_total_ms: 0,
  scheduled_tasks: 0,
  tokens_total: 0,
  tokens_today_total: 0,
  tokens_yesterday_total: 0,
}

type ChangeIndicator = ReturnType<typeof changeIndicator>

function changeIndicator(ratio: number | null) {
  const label = formatChange(ratio)
  const Icon = ratio === null ? null : ratio > 0 ? TrendingUp : ratio < 0 ? TrendingDown : Minus
  // Direction is carried entirely by the arrow icon. The delta color stays
  // neutral so the presence palette (green idle / purple working / red error)
  // remains reserved for worker status and never reads as a trend valence.
  return { label, Icon, color: "text-muted-foreground" }
}

const STAT_LABEL = "text-[11px] font-medium uppercase tracking-[0.18em] text-muted-foreground"

// The System Status row is a wrapping grid (2 → 3 → 6 columns). Column dividers
// must only sit between cells in the same visual row, so the left border and its
// padding are recomputed per breakpoint from the column count. Bottom padding is
// dropped on the final row at each breakpoint.
function statusCellClass(i: number): string {
  return [
    "border-border/70",
    i % 2 !== 0 ? "border-l pl-6" : "",
    i % 3 !== 0 ? "sm:border-l sm:pl-8" : "sm:border-l-0 sm:pl-0",
    i !== 0 ? "lg:border-l lg:pl-8" : "lg:border-l-0 lg:pl-0",
    i < 4 ? "pb-6" : "",
    i < 3 ? "sm:pb-6" : "sm:pb-0",
    "lg:pb-0",
  ].join(" ")
}

// Skeleton dimensions mirror the loaded stat (label line box, mb-2.5, 3xl value)
// so the System Status row does not shift vertically when data resolves.
function StatSkeleton() {
  return (
    <div>
      <Skeleton className="h-3.5 w-20 mb-2.5" />
      <Skeleton className="h-8 w-14" />
    </div>
  )
}

function TodayMetric({
  label,
  value,
  valueAriaLabel,
  pastLabel,
  pastValue,
  change,
  isLoading,
  valueSkeletonClass = "w-16",
  note,
}: {
  label: string
  value: ReactNode
  valueAriaLabel: string
  pastLabel: string
  pastValue: ReactNode
  change: ChangeIndicator
  isLoading: boolean
  valueSkeletonClass?: string
  note?: ReactNode
}) {
  return (
    <div className="p-6">
      <p className={`${STAT_LABEL} mb-5`}>{label}</p>
      {isLoading ? (
        <div className="space-y-4">
          <Skeleton className={`h-12 ${valueSkeletonClass}`} />
          <div className="flex gap-4">
            <Skeleton className="h-5 w-20" />
            <Skeleton className="h-5 w-12" />
          </div>
        </div>
      ) : (
        <div>
          <p
            className="text-5xl font-semibold tabular-nums leading-none mb-4"
            aria-label={valueAriaLabel}
            aria-live="polite"
          >
            {value}
          </p>
          <div className="flex flex-wrap items-center gap-x-5 gap-y-1.5">
            <div className="flex items-baseline gap-2">
              <span className={STAT_LABEL}>{pastLabel}</span>
              <span className="text-lg font-medium tabular-nums text-muted-foreground leading-none">
                {pastValue}
              </span>
            </div>
            {change.label !== null && change.Icon && (
              <div className={`flex items-center gap-1 ${change.color}`} aria-label={change.label}>
                <change.Icon className="h-3 w-3" aria-hidden />
                <span className="text-xs font-semibold tabular-nums">{change.label}</span>
              </div>
            )}
            {note}
          </div>
        </div>
      )}
    </div>
  )
}

export function Dashboard() {
  const { t } = useTranslation()
  const { data, isLoading } = useStatsOverview()
  const ov = data ?? EMPTY

  const activeWorkers = changeIndicator(ov.active_workers_change)
  const messages = changeIndicator(ov.messages_change)
  const executions = changeIndicator(ov.executions_change)

  const durationDiff = ov.exec_duration_today_ms - ov.exec_duration_yesterday_ms
  const durationRatio = ov.exec_duration_yesterday_ms > 0 ? durationDiff / ov.exec_duration_yesterday_ms : null
  const duration = changeIndicator(durationRatio)

  const tokenDiff = ov.tokens_today_total - ov.tokens_yesterday_total
  const tokenRatio = ov.tokens_yesterday_total > 0 ? tokenDiff / ov.tokens_yesterday_total : null
  const tokens = changeIndicator(tokenRatio)

  const stats = [
    { label: t("dashboard.departments"), value: ov.departments },
    { label: t("dashboard.workers"), value: ov.workers },
    { label: t("dashboard.scheduledTasks"), value: ov.scheduled_tasks },
    { label: t("dashboard.totalMessages"), value: formatNumber(ov.messages_total_global) },
    { label: t("dashboard.totalWorkDuration"), value: formatTotalDuration(ov.exec_duration_total_ms) },
    { label: t("dashboard.totalTokens"), value: formatTokenCount(ov.tokens_total) },
  ]

  return (
    <FadeIn>
      <PageHeader title={t("dashboard.title")} />

      {/* ── System Status ─────────────────────────────────────────── */}
      <div className="mb-10">
        <SectionRule className="mb-5">{t("dashboard.systemStatus")}</SectionRule>
        <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-6">
          {isLoading
            ? Array.from({ length: 6 }).map((_, i) => (
                <div key={i} className={statusCellClass(i)}>
                  <StatSkeleton />
                </div>
              ))
            : stats.map(({ label, value }, i) => (
                <div key={label} className={statusCellClass(i)} aria-label={label}>
                  <p className={`${STAT_LABEL} mb-2.5`}>{label}</p>
                  <p className="text-3xl font-semibold tabular-nums leading-none">{value}</p>
                </div>
              ))}
        </div>
      </div>

      {/* ── Today ─────────────────────────────────────────────────── */}
      <div className="mb-10">
        <SectionRule className="mb-5">{t("dashboard.todayActivity")}</SectionRule>
        <div
          className="border border-border/70 rounded-sm overflow-hidden"
          role="region"
          aria-label={t("dashboard.todayActivity")}
        >
          <div className="grid grid-cols-1 sm:grid-cols-5 divide-y sm:divide-y-0 sm:divide-x divide-border/70">
            <TodayMetric
              label={t("dashboard.activeWorkers")}
              value={ov.active_workers_today}
              valueAriaLabel={`${t("dashboard.activeWorkers")}: ${ov.active_workers_today}`}
              pastLabel={t("dashboard.yesterday")}
              pastValue={ov.active_workers_yesterday}
              change={activeWorkers}
              isLoading={isLoading}
            />
            <TodayMetric
              label={t("dashboard.messages")}
              value={ov.messages_total_today}
              valueAriaLabel={`${t("dashboard.messages")}: ${ov.messages_total_today}`}
              pastLabel={t("dashboard.yesterday")}
              pastValue={ov.messages_total_yesterday}
              change={messages}
              isLoading={isLoading}
            />
            <TodayMetric
              label={t("dashboard.executions")}
              value={ov.executions_today}
              valueAriaLabel={`${t("dashboard.executions")}: ${ov.executions_today}`}
              pastLabel={t("dashboard.yesterday")}
              pastValue={ov.executions_yesterday}
              change={executions}
              isLoading={isLoading}
            />
            <TodayMetric
              label={t("dashboard.executionDuration")}
              value={formatTotalDuration(ov.exec_duration_today_ms)}
              valueAriaLabel={`${t("dashboard.execDurationToday")}: ${formatTotalDuration(ov.exec_duration_today_ms)}`}
              pastLabel={t("dashboard.yesterday")}
              pastValue={formatTotalDuration(ov.exec_duration_yesterday_ms)}
              change={duration}
              isLoading={isLoading}
              valueSkeletonClass="w-28"
            />
            <TodayMetric
              label={t("dashboard.tokensToday")}
              value={formatTokenCount(ov.tokens_today_total)}
              valueAriaLabel={`${t("dashboard.tokensToday")}: ${formatTokenCount(ov.tokens_today_total)}`}
              pastLabel={t("dashboard.tokensYesterday")}
              pastValue={formatTokenCount(ov.tokens_yesterday_total)}
              change={tokens}
              isLoading={isLoading}
              note={
                <Tooltip>
                  <TooltipTrigger
                    type="button"
                    aria-label={t("dashboard.tokensCrossDayNote")}
                    className="ml-auto text-muted-foreground/60 hover:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring rounded transition-colors"
                  >
                    <Info className="h-3.5 w-3.5" aria-hidden />
                  </TooltipTrigger>
                  <TooltipContent>
                    <p>{t("dashboard.tokensCrossDayNote")}</p>
                  </TooltipContent>
                </Tooltip>
              }
            />
          </div>
        </div>
      </div>

      {/* ── Charts ─────────────────────────────────────────────────── */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        <TokenTrendChart />
        <CombinedTrendChart />
      </div>
    </FadeIn>
  )
}
