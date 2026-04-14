import { useState } from "react"
import { useTranslation } from "react-i18next"
import { TrendLineCard } from "@/components/trend-line-card"
import { useStatsTrend } from "@/hooks/use-stats"

export function ActivityTrendChart() {
  const { t } = useTranslation()
  const [days, setDays] = useState<7 | 15 | 30>(7)
  const { data, isLoading } = useStatsTrend(days)

  return (
    <TrendLineCard
      title={t("dashboard.activityTrend")}
      ariaLabel={t("dashboard.activityTrendAriaLabel", { days })}
      emptyTitle={t("dashboard.noTrendData")}
      emptyDesc={t("dashboard.noTrendDataDesc")}
      dataKey="active_workers"
      tooltipLabel={t("dashboard.activeWorkers")}
      chartData={data?.data ?? []}
      isLoading={isLoading}
      days={days}
      onDaysChange={setDays}
    />
  )
}
