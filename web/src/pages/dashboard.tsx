import { useTranslation } from "react-i18next"
import { FadeIn } from "@/components/fade-in"
import { PageHeader } from "@/components/page-header"
import { DashboardStats } from "@/components/dashboard-hero-card"
import { QuickLinks } from "@/components/quick-links"
import { TokenUsageCard } from "@/components/token-usage-card"
import { SupportedAgentsCard } from "@/components/supported-agents-card"
import { SystemInfoCard } from "@/components/system-info-card"

export function Dashboard() {
  const { t } = useTranslation()

  return (
    <FadeIn>
      <PageHeader title={t("dashboard.title")} />

      {/* Main column (headline counts, usage, launchers) beside a fixed side
          column (engines, build info); one column under lg. The stat row lives
          in the main column so its edges line up with the panels below it. */}
      <div className="grid grid-cols-1 items-start gap-4 lg:grid-cols-[minmax(0,1fr)_360px]">
        <div className="flex min-w-0 flex-col gap-4">
          <DashboardStats />
          <TokenUsageCard />
          <QuickLinks />
        </div>
        <div className="flex min-w-0 flex-col gap-4">
          <SupportedAgentsCard />
          <SystemInfoCard />
        </div>
      </div>
    </FadeIn>
  )
}
