import { useTranslation } from "react-i18next"
import { FadeIn } from "@/components/fade-in"
import { PageHeader } from "@/components/page-header"
import { DashboardHeroCard } from "@/components/dashboard-hero-card"
import { QuickLinks } from "@/components/quick-links"
import { TokenUsageCard } from "@/components/token-usage-card"
import { SupportedAgentsCard } from "@/components/supported-agents-card"
import { SystemInfoCard } from "@/components/system-info-card"

export function Dashboard() {
  const { t } = useTranslation()

  return (
    <FadeIn>
      <PageHeader title={t("dashboard.title")} />

      <div className="grid grid-cols-1 gap-x-6 gap-y-8 lg:grid-cols-3">
        {/* Left 2/3: identity, launchers, token usage */}
        <div className="space-y-8 lg:col-span-2">
          <DashboardHeroCard />
          <QuickLinks />
          <TokenUsageCard />
        </div>

        {/* Right 1/3: supported engines, system info */}
        <div className="space-y-8">
          <SupportedAgentsCard />
          <SystemInfoCard />
        </div>
      </div>
    </FadeIn>
  )
}
