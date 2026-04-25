import type { ReactNode } from "react"
import { useTranslation } from "react-i18next"
import { TrendingUp, TrendingDown, Minus, Info } from "lucide-react"
import { FadeIn } from "@/components/fade-in"
import { PageHeader } from "@/components/page-header"
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

function changeIndicator(ratio: number | null) {
  const label = formatChange(ratio)
  const Icon = ratio === null ? null : ratio > 0 ? TrendingUp : ratio < 0 ? TrendingDown : Minus
  const color =
    ratio === null
      ? ""
      : ratio > 0
        ? "text-status-idle"
        : ratio < 0
          ? "text-status-error"
          : "text-muted-foreground"
  return { label, Icon, color }
}

function SectionRule({ children }: { children: ReactNode }) {
  return (
    <div className="flex items-center gap-3 mb-5">
      <span className="text-[11px] font-medium uppercase tracking-[0.18em] text-muted-foreground whitespace-nowrap select-none">
        {children}
      </span>
      <div className="flex-1 h-px bg-border" />
    </div>
  )
}

function StatSkeleton() {
  return (
    <div>
      <Skeleton className="h-2.5 w-20 mb-3" />
      <Skeleton className="h-8 w-14" />
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

  return (
    <FadeIn>
      <PageHeader title={t("dashboard.title")} />

      {/* ── System Status ─────────────────────────────────────────── */}
      <div className="mb-10">
        <SectionRule>{t("dashboard.systemStatus")}</SectionRule>
        <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-6">
          {isLoading ? (
            Array.from({ length: 6 }).map((_, i) => (
              <div
                key={i}
                className={[
                  i % 2 !== 0 ? "pl-6 border-l border-border/70" : "",
                  i > 0 ? "sm:pl-8 sm:border-l sm:border-border/70" : "",
                  i < 5 ? "pb-6 lg:pb-0" : "",
                ].join(" ")}
              >
                <StatSkeleton />
              </div>
            ))
          ) : (
            <>
              {[
                { label: t("dashboard.departments"), value: ov.departments },
                { label: t("dashboard.workers"), value: ov.workers },
                { label: t("dashboard.scheduledTasks"), value: ov.scheduled_tasks },
                { label: t("dashboard.totalMessages"), value: formatNumber(ov.messages_total_global) },
                {
                  label: t("dashboard.totalWorkDuration"),
                  value: formatTotalDuration(ov.exec_duration_total_ms),
                },
              ].map(({ label, value }, i) => (
                <div
                  key={i}
                  className={[
                    i % 2 !== 0 ? "pl-6 border-l border-border/70" : "",
                    i > 0 ? "sm:pl-8 sm:border-l sm:border-border/70" : "",
                    i < 5 ? "pb-6 lg:pb-0" : "",
                  ].join(" ")}
                  aria-label={label}
                >
                  <p className="text-[11px] font-medium uppercase tracking-[0.18em] text-muted-foreground mb-2.5">
                    {label}
                  </p>
                  <p className="text-3xl font-semibold tabular-nums leading-none">{value}</p>
                </div>
              ))}
              <div
                className="pl-6 border-l border-border/70 sm:pl-8 sm:border-l sm:border-border/70 pb-6 lg:pb-0"
                aria-label={t("dashboard.totalTokens")}
              >
                <p className="text-[11px] font-medium uppercase tracking-[0.18em] text-muted-foreground mb-2.5">
                  {t("dashboard.totalTokens")}
                </p>
                <p className="text-3xl font-semibold tabular-nums leading-none">
                  {formatTokenCount(ov.tokens_total)}
                </p>
              </div>
            </>
          )}
        </div>
      </div>

      {/* ── Today ─────────────────────────────────────────────────── */}
      <div className="mb-10">
        <SectionRule>{t("dashboard.todayActivity")}</SectionRule>
        <div
          className="border border-border/70 rounded-3xl overflow-hidden"
          role="region"
          aria-label={t("dashboard.todayActivity")}
        >
          <div className="grid grid-cols-1 sm:grid-cols-5 divide-y sm:divide-y-0 sm:divide-x divide-border">

            {/* Active Workers */}
            <div className="p-6">
              <p className="text-[11px] font-medium uppercase tracking-[0.18em] text-muted-foreground mb-5">
                {t("dashboard.activeWorkers")}
              </p>
              {isLoading ? (
                <div className="space-y-4">
                  <Skeleton className="h-12 w-16" />
                  <div className="flex gap-4">
                    <Skeleton className="h-5 w-20" />
                    <Skeleton className="h-5 w-12" />
                  </div>
                </div>
              ) : (
                <div>
                  <p
                    className="text-5xl font-semibold tabular-nums leading-none mb-4"
                    aria-label={`${t("dashboard.activeWorkers")}: ${ov.active_workers_today}`}
                    aria-live="polite"
                  >
                    {ov.active_workers_today}
                  </p>
                  <div className="flex flex-wrap items-center gap-x-5 gap-y-1.5">
                    <div className="flex items-baseline gap-2">
                      <span className="text-[11px] font-medium uppercase tracking-[0.18em] text-muted-foreground">
                        {t("dashboard.yesterday")}
                      </span>
                      <span className="text-lg font-medium tabular-nums text-muted-foreground leading-none">
                        {ov.active_workers_yesterday}
                      </span>
                    </div>
                    {activeWorkers.label !== null && activeWorkers.Icon && (
                      <div
                        className={`flex items-center gap-1 ${activeWorkers.color}`}
                        aria-label={activeWorkers.label}
                      >
                        <activeWorkers.Icon className="h-3 w-3" aria-hidden />
                        <span className="text-xs font-semibold tabular-nums">{activeWorkers.label}</span>
                      </div>
                    )}
                  </div>
                </div>
              )}
            </div>

            {/* Messages */}
            <div className="p-6">
              <p className="text-[11px] font-medium uppercase tracking-[0.18em] text-muted-foreground mb-5">
                {t("dashboard.messages")}
              </p>
              {isLoading ? (
                <div className="space-y-4">
                  <Skeleton className="h-12 w-16" />
                  <div className="flex gap-4">
                    <Skeleton className="h-5 w-20" />
                    <Skeleton className="h-5 w-12" />
                  </div>
                </div>
              ) : (
                <div>
                  <p
                    className="text-5xl font-semibold tabular-nums leading-none mb-4"
                    aria-label={`${t("dashboard.messages")}: ${ov.messages_total_today}`}
                    aria-live="polite"
                  >
                    {ov.messages_total_today}
                  </p>
                  <div className="flex flex-wrap items-center gap-x-5 gap-y-1.5">
                    <div className="flex items-baseline gap-2">
                      <span className="text-[11px] font-medium uppercase tracking-[0.18em] text-muted-foreground">
                        {t("dashboard.yesterday")}
                      </span>
                      <span className="text-lg font-medium tabular-nums text-muted-foreground leading-none">
                        {ov.messages_total_yesterday}
                      </span>
                    </div>
                    {messages.label !== null && messages.Icon && (
                      <div
                        className={`flex items-center gap-1 ${messages.color}`}
                        aria-label={messages.label}
                      >
                        <messages.Icon className="h-3 w-3" aria-hidden />
                        <span className="text-xs font-semibold tabular-nums">{messages.label}</span>
                      </div>
                    )}
                  </div>
                </div>
              )}
            </div>

            {/* Executions */}
            <div className="p-6">
              <p className="text-[11px] font-medium uppercase tracking-[0.18em] text-muted-foreground mb-5">
                {t("dashboard.executions")}
              </p>
              {isLoading ? (
                <div className="space-y-4">
                  <Skeleton className="h-12 w-16" />
                  <div className="flex gap-4">
                    <Skeleton className="h-5 w-20" />
                    <Skeleton className="h-5 w-12" />
                  </div>
                </div>
              ) : (
                <div>
                  <p
                    className="text-5xl font-semibold tabular-nums leading-none mb-4"
                    aria-label={`${t("dashboard.executions")}: ${ov.executions_today}`}
                    aria-live="polite"
                  >
                    {ov.executions_today}
                  </p>
                  <div className="flex flex-wrap items-center gap-x-5 gap-y-1.5">
                    <div className="flex items-baseline gap-2">
                      <span className="text-[11px] font-medium uppercase tracking-[0.18em] text-muted-foreground">
                        {t("dashboard.yesterday")}
                      </span>
                      <span className="text-lg font-medium tabular-nums text-muted-foreground leading-none">
                        {ov.executions_yesterday}
                      </span>
                    </div>
                    {executions.label !== null && executions.Icon && (
                      <div
                        className={`flex items-center gap-1 ${executions.color}`}
                        aria-label={executions.label}
                      >
                        <executions.Icon className="h-3 w-3" aria-hidden />
                        <span className="text-xs font-semibold tabular-nums">{executions.label}</span>
                      </div>
                    )}
                  </div>
                </div>
              )}
            </div>

            {/* Execution Duration */}
            <div className="p-6">
              <p className="text-[11px] font-medium uppercase tracking-[0.18em] text-muted-foreground mb-5">
                {t("dashboard.executionDuration")}
              </p>
              {isLoading ? (
                <div className="space-y-4">
                  <Skeleton className="h-12 w-28" />
                  <div className="flex gap-4">
                    <Skeleton className="h-5 w-20" />
                    <Skeleton className="h-5 w-12" />
                  </div>
                </div>
              ) : (
                <div>
                  <p
                    className="text-5xl font-semibold tabular-nums leading-none mb-4"
                    aria-label={`${t("dashboard.execDurationToday")}: ${formatTotalDuration(ov.exec_duration_today_ms)}`}
                    aria-live="polite"
                  >
                    {formatTotalDuration(ov.exec_duration_today_ms)}
                  </p>
                  <div className="flex flex-wrap items-center gap-x-5 gap-y-1.5">
                    <div className="flex items-baseline gap-2">
                      <span className="text-[11px] font-medium uppercase tracking-[0.18em] text-muted-foreground">
                        {t("dashboard.yesterday")}
                      </span>
                      <span className="text-lg font-medium tabular-nums text-muted-foreground leading-none">
                        {formatTotalDuration(ov.exec_duration_yesterday_ms)}
                      </span>
                    </div>
                    {duration.label !== null && duration.Icon && (
                      <div
                        className={`flex items-center gap-1 ${duration.color}`}
                        aria-label={duration.label}
                      >
                        <duration.Icon className="h-3 w-3" aria-hidden />
                        <span className="text-xs font-semibold tabular-nums">{duration.label}</span>
                      </div>
                    )}
                  </div>
                </div>
              )}
            </div>

            {/* Tokens */}
            <div className="p-6">
              <p className="text-[11px] font-medium uppercase tracking-[0.18em] text-muted-foreground mb-5">
                {t("dashboard.tokensToday")}
              </p>
              {isLoading ? (
                <div className="space-y-4">
                  <Skeleton className="h-12 w-16" />
                  <div className="flex gap-4">
                    <Skeleton className="h-5 w-20" />
                    <Skeleton className="h-5 w-12" />
                  </div>
                </div>
              ) : (
                <div>
                  <p
                    aria-live="polite"
                    className="text-5xl font-semibold tabular-nums leading-none mb-4"
                  >
                    {formatTokenCount(ov.tokens_today_total)}
                  </p>
                  <div className="flex flex-wrap items-center gap-x-5 gap-y-1.5">
                    <div className="flex items-baseline gap-2">
                      <span className="text-[11px] font-medium uppercase tracking-[0.18em] text-muted-foreground">
                        {t("dashboard.tokensYesterday")}
                      </span>
                      <span className="text-lg font-medium tabular-nums text-muted-foreground leading-none">
                        {formatTokenCount(ov.tokens_yesterday_total)}
                      </span>
                    </div>
                    {tokens.label !== null && tokens.Icon && (
                      <div
                        className={`flex items-center gap-1 ${tokens.color}`}
                        aria-label={tokens.label}
                      >
                        <tokens.Icon className="h-3 w-3" aria-hidden />
                        <span className="text-xs font-semibold tabular-nums">{tokens.label}</span>
                      </div>
                    )}
                    <Tooltip>
                      <TooltipTrigger
                        type="button"
                        aria-label={t("dashboard.tokensCrossDayNote")}
                        className="ml-auto text-muted-foreground/50 hover:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring rounded"
                      >
                        <Info className="h-3.5 w-3.5" aria-hidden />
                      </TooltipTrigger>
                      <TooltipContent>
                        <p>{t("dashboard.tokensCrossDayNote")}</p>
                      </TooltipContent>
                    </Tooltip>
                  </div>
                </div>
              )}
            </div>

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
