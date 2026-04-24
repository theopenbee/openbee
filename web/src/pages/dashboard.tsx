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
import { formatChange, formatTotalDuration, formatTokenCount } from "@/lib/format"
import type { StatsOverview } from "@/lib/types"

const EMPTY: StatsOverview = {
  departments: 0,
  workers: 0,
  active_workers_today: 0,
  active_workers_yesterday: 0,
  active_workers_change: null,
  messages_received_today: 0,
  messages_sent_today: 0,
  messages_total_today: 0,
  messages_total_global: 0,
  executions_today: { total: 0, success: 0, failed: 0 },
  exec_duration_today_ms: 0,
  exec_duration_yesterday_ms: 0,
  exec_duration_total_ms: 0,
  scheduled_tasks: 0,
  tokens_total: 0,
  tokens_total_input: 0,
  tokens_total_output: 0,
  tokens_today_total: 0,
  tokens_today_input: 0,
  tokens_today_output: 0,
  tokens_yesterday_total: 0,
  tokens_yesterday_input: 0,
  tokens_yesterday_output: 0,
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

  const change = ov.active_workers_change
  const changeLabel = formatChange(change)
  const ChangeIcon =
    change === null ? null : change > 0 ? TrendingUp : change < 0 ? TrendingDown : Minus
  const changeColor =
    change === null
      ? ""
      : change > 0
        ? "text-status-idle"
        : change < 0
          ? "text-status-error"
          : "text-muted-foreground"

  const durationDiff = ov.exec_duration_today_ms - ov.exec_duration_yesterday_ms
  const durationRatio = ov.exec_duration_yesterday_ms > 0 ? durationDiff / ov.exec_duration_yesterday_ms : null
  const durationChangeLabel = formatChange(durationRatio)
  const durationChangeColor =
    durationDiff > 0 ? "text-status-idle" : durationDiff < 0 ? "text-status-error" : "text-muted-foreground"

  const tokenDiff = ov.tokens_today_total - ov.tokens_yesterday_total
  const tokenRatio =
    ov.tokens_yesterday_total > 0 ? tokenDiff / ov.tokens_yesterday_total : null
  const tokenChangeLabel = formatChange(tokenRatio)
  const tokenChangeColor =
    tokenDiff > 0
      ? "text-status-idle"
      : tokenDiff < 0
        ? "text-status-error"
        : "text-muted-foreground"

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
                { label: t("dashboard.totalMessages"), value: ov.messages_total_global },
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
                    i < 4 ? "pb-6 lg:pb-0" : "",
                  ].join(" ")}
                  aria-label={label}
                >
                  <p className="text-[11px] font-medium uppercase tracking-[0.18em] text-muted-foreground mb-2.5">
                    {label}
                  </p>
                  <p className="text-3xl font-semibold tabular-nums leading-none">{value}</p>
                </div>
              ))}
              {/* Total Tokens — 6th item with hover tooltip */}
              <div
                className="pl-6 border-l border-border/70 sm:pl-8 sm:border-l sm:border-border/70"
                aria-label={t("dashboard.totalTokens")}
              >
                <p className="text-[11px] font-medium uppercase tracking-[0.18em] text-muted-foreground mb-2.5">
                  {t("dashboard.totalTokens")}
                </p>
                <Tooltip>
                  <TooltipTrigger asChild>
                    <p className="text-3xl font-semibold tabular-nums leading-none cursor-default">
                      {formatTokenCount(ov.tokens_total)}
                    </p>
                  </TooltipTrigger>
                  <TooltipContent>
                    <p>{t("dashboard.tokensTodayInput")}: {ov.tokens_total_input.toLocaleString()}</p>
                    <p>{t("dashboard.tokensTodayOutput")}: {ov.tokens_total_output.toLocaleString()}</p>
                  </TooltipContent>
                </Tooltip>
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
                    {changeLabel !== null && ChangeIcon && (
                      <div
                        className={`flex items-center gap-1 ${changeColor}`}
                        aria-label={changeLabel}
                      >
                        <ChangeIcon className="h-3 w-3" aria-hidden />
                        <span className="text-xs font-semibold tabular-nums">{changeLabel}</span>
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
                  <div className="flex gap-6">
                    <StatSkeleton />
                    <StatSkeleton />
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
                  <div className="flex gap-6">
                    <div>
                      <p className="text-[11px] font-medium uppercase tracking-[0.18em] text-muted-foreground mb-2">
                        {t("dashboard.messagesReceived")}
                      </p>
                      <p
                        className="text-xl font-semibold tabular-nums leading-none"
                        aria-live="polite"
                      >
                        {ov.messages_received_today}
                      </p>
                    </div>
                    <div>
                      <p className="text-[11px] font-medium uppercase tracking-[0.18em] text-muted-foreground mb-2">
                        {t("dashboard.messagesSent")}
                      </p>
                      <p
                        className="text-xl font-semibold tabular-nums leading-none"
                        aria-live="polite"
                      >
                        {ov.messages_sent_today}
                      </p>
                    </div>
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
                  <div className="flex gap-6">
                    <StatSkeleton />
                    <StatSkeleton />
                  </div>
                </div>
              ) : (
                <div>
                  <p
                    className="text-5xl font-semibold tabular-nums leading-none mb-4"
                    aria-label={`${t("dashboard.executionsTotal")}: ${ov.executions_today.total}`}
                    aria-live="polite"
                  >
                    {ov.executions_today.total}
                  </p>
                  <div className="flex gap-6">
                    <div>
                      <p className="text-[11px] font-medium uppercase tracking-[0.18em] text-muted-foreground mb-2">
                        {t("dashboard.executionsSuccess")}
                      </p>
                      <p
                        className="text-xl font-semibold tabular-nums leading-none text-status-idle"
                        aria-label={`${t("dashboard.executionsSuccess")}: ${ov.executions_today.success}`}
                        aria-live="polite"
                      >
                        {ov.executions_today.success}
                      </p>
                    </div>
                    <div>
                      <p className="text-[11px] font-medium uppercase tracking-[0.18em] text-muted-foreground mb-2">
                        {t("dashboard.executionsFailed")}
                      </p>
                      <p
                        className="text-xl font-semibold tabular-nums leading-none text-status-error"
                        aria-label={`${t("dashboard.executionsFailed")}: ${ov.executions_today.failed}`}
                        aria-live="polite"
                      >
                        {ov.executions_today.failed}
                      </p>
                    </div>
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
                  <div className="flex gap-6">
                    <StatSkeleton />
                    <StatSkeleton />
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
                  <div className="flex gap-6">
                    <div>
                      <p className="text-[11px] font-medium uppercase tracking-[0.18em] text-muted-foreground mb-2">
                        {t("dashboard.execDurationYesterday")}
                      </p>
                      <p
                        className="text-xl font-semibold tabular-nums leading-none text-muted-foreground"
                        aria-live="polite"
                      >
                        {formatTotalDuration(ov.exec_duration_yesterday_ms)}
                      </p>
                    </div>
                    <div>
                      <p className="text-[11px] font-medium uppercase tracking-[0.18em] text-muted-foreground mb-2">
                        {t("dashboard.execDurationDayOverDay")}
                      </p>
                      <p
                        className="text-xl font-semibold tabular-nums leading-none"
                        aria-live="polite"
                      >
                        <span className={durationChangeColor}>
                          {durationDiff >= 0 ? "+" : "−"}{formatTotalDuration(Math.abs(durationDiff))}
                          {durationChangeLabel !== null && ` (${durationChangeLabel})`}
                        </span>
                      </p>
                    </div>
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
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <p
                        className="text-5xl font-semibold tabular-nums leading-none mb-4 cursor-default"
                        aria-label={`${t("dashboard.tokensToday")}: ${formatTokenCount(ov.tokens_today_total)}`}
                        aria-live="polite"
                      >
                        {formatTokenCount(ov.tokens_today_total)}
                      </p>
                    </TooltipTrigger>
                    <TooltipContent>
                      <p>{t("dashboard.tokensTodayInput")}: {ov.tokens_today_input.toLocaleString()}</p>
                      <p>{t("dashboard.tokensTodayOutput")}: {ov.tokens_today_output.toLocaleString()}</p>
                    </TooltipContent>
                  </Tooltip>
                  <div className="flex flex-wrap items-center gap-x-5 gap-y-1.5">
                    <div className="flex items-baseline gap-2">
                      <span className="text-[11px] font-medium uppercase tracking-[0.18em] text-muted-foreground">
                        {t("dashboard.tokensYesterday")}
                      </span>
                      <span className="text-lg font-medium tabular-nums text-muted-foreground leading-none">
                        {formatTokenCount(ov.tokens_yesterday_total)}
                      </span>
                    </div>
                    {tokenChangeLabel !== null && (
                      <div
                        className={`flex items-center gap-1 ${tokenChangeColor}`}
                        aria-label={tokenChangeLabel}
                      >
                        {tokenDiff > 0 ? (
                          <TrendingUp className="h-3 w-3" aria-hidden />
                        ) : tokenDiff < 0 ? (
                          <TrendingDown className="h-3 w-3" aria-hidden />
                        ) : (
                          <Minus className="h-3 w-3" aria-hidden />
                        )}
                        <span className="text-xs font-semibold tabular-nums">{tokenChangeLabel}</span>
                      </div>
                    )}
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <button
                          type="button"
                          aria-label={t("dashboard.tokensCrossDayNote")}
                          className="ml-auto text-muted-foreground/50 hover:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring rounded"
                        >
                          <Info className="h-3.5 w-3.5" aria-hidden />
                        </button>
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
