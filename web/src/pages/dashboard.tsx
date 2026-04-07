import { useTranslation } from "react-i18next"
import { PageHeader } from "@/components/page-header"
import { FadeIn } from "@/components/fade-in"
import { StatCard } from "@/components/stat-card"
import { ActiveWorkersCard } from "@/components/active-workers-card"
import { MessagesCard } from "@/components/messages-card"
import { ExecutionsCard } from "@/components/executions-card"
import { ActivityTrendChart } from "@/components/activity-trend-chart"
import { useStatsOverview } from "@/hooks/use-stats"

export function Dashboard() {
  const { t } = useTranslation()
  const { data, isLoading } = useStatsOverview()

  const empty: import("@/lib/types").StatsOverview = {
    departments: 0,
    workers: 0,
    active_workers_today: 0,
    active_workers_yesterday: 0,
    active_workers_change: null,
    messages_received_today: 0,
    messages_sent_today: 0,
    sessions_new_today: 0,
    executions_today: { total: 0, success: 0, failed: 0 },
    scheduled_tasks: 0,
  }
  const ov = data ?? empty

  return (
    <FadeIn>
      <PageHeader title={t("dashboard.title")} />

      {/* Row 1: 4 stat cards */}
      <div className="grid grid-cols-2 sm:grid-cols-4 gap-4 mb-4">
        <StatCard title={t("dashboard.departments")} value={ov.departments} loading={isLoading} />
        <StatCard title={t("dashboard.workers")} value={ov.workers} loading={isLoading} />
        <StatCard title={t("dashboard.scheduledTasks")} value={ov.scheduled_tasks} loading={isLoading} />
        <StatCard title={t("dashboard.sessionsToday")} value={ov.sessions_new_today} loading={isLoading} />
      </div>

      {/* Row 2: active workers full width */}
      <div className="mb-4">
        <ActiveWorkersCard
          today={ov.active_workers_today}
          yesterday={ov.active_workers_yesterday}
          change={ov.active_workers_change}
          loading={isLoading}
        />
      </div>

      {/* Row 3: messages + executions */}
      <div className="grid grid-cols-1 sm:grid-cols-2 gap-4 mb-4">
        <MessagesCard
          received={ov.messages_received_today}
          sent={ov.messages_sent_today}
          loading={isLoading}
        />
        <ExecutionsCard stats={ov.executions_today} loading={isLoading} />
      </div>

      {/* Row 4: trend chart */}
      <ActivityTrendChart />
    </FadeIn>
  )
}
