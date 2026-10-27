import { useTranslation } from "react-i18next"
import { FadeIn } from "@/components/fade-in"
import { PageHeader } from "@/components/page-header"
import { DashboardMetrics } from "@/components/dashboard-metrics"
import { QuickLinks } from "@/components/quick-links"
import { TokenUsageCard } from "@/components/token-usage-card"
import { SupportedAgentsCard } from "@/components/supported-agents-card"
import { SystemInfoCard } from "@/components/system-info-card"

export function Dashboard() {
  const { t } = useTranslation()

  return (
    <FadeIn className="@container">
      <PageHeader title={t("dashboard.title")} />

      <div className="grid gap-6">
        <DashboardMetrics />

        <div className="grid items-stretch gap-6 @5xl:grid-cols-3">
          <TokenUsageCard className="@5xl:col-span-2" />
          <QuickLinks />
        </div>

        <div className="grid items-stretch gap-6 @3xl:grid-cols-2">
          <SupportedAgentsCard />
          <SystemInfoCard />
        </div>
      </div>
    </FadeIn>
  )
}
