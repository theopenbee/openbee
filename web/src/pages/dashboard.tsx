import type { ReactNode } from "react"
import { useTranslation } from "react-i18next"
import { TrendingUp, TrendingDown, Minus } from "lucide-react"
import { FadeIn } from "@/components/fade-in"
import { PageHeader } from "@/components/page-header"
import { Skeleton } from "@/components/ui/skeleton"
import { ActivityTrendChart } from "@/components/activity-trend-chart"
import { useStatsOverview } from "@/hooks/use-stats"
import { formatChange, formatTotalDuration } from "@/lib/format"
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
  executions_today: { total: 0, success: 0, failed: 0 },
  exec_duration_today_ms: 0,
  exec_duration_yesterday_ms: 0,
  exec_duration_total_ms: 0,
  scheduled_tasks: 0,
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

  return (
    <FadeIn>
      <PageHeader title={t("dashboard.title")} />

      {/* ── System Status ─────────────────────────────────────────── */}
      <div className="mb-10">
        <SectionRule>{t("dashboard.systemStatus")}</SectionRule>
        <div className="grid grid-cols-2 sm:grid-cols-3">
          {isLoading ? (
            Array.from({ length: 3 }).map((_, i) => (
              <div
                key={i}
                className={
                  i % 2 !== 0
                    ? "pl-6 border-l border-border/70 sm:pl-8"
                    : i > 0
                      ? "pl-6 sm:pl-8 sm:border-l sm:border-border/70"
                      : ""
                }
              >
                <StatSkeleton />
              </div>
            ))
          ) : (
            [
              { label: t("dashboard.departments"), value: ov.departments },
              { label: t("dashboard.workers"), value: ov.workers },
              { label: t("dashboard.scheduledTasks"), value: ov.scheduled_tasks },
            ].map(({ label, value }, i) => (
              <div
                key={i}
                className={[
                  i % 2 !== 0 ? "pl-6 border-l border-border/70" : "",
                  i > 0 ? "sm:pl-8 sm:border-l sm:border-border/70" : "",
                  i < 2 ? "pb-6 sm:pb-0" : "",
                ].join(" ")}
                aria-label={label}
              >
                <p className="text-[11px] font-medium uppercase tracking-[0.18em] text-muted-foreground mb-2.5">
                  {label}
                </p>
                <p className="text-3xl font-semibold tabular-nums leading-none">{value}</p>
              </div>
            ))
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
          <div className="grid grid-cols-1 sm:grid-cols-3 divide-y sm:divide-y-0 sm:divide-x divide-border">

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

          </div>
        </div>
      </div>

      {/* ── Activity Trend ─────────────────────────────────────────── */}
      <ActivityTrendChart />
    </FadeIn>
  )
}
