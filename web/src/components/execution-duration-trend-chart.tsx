import { useState } from "react"
import { useTranslation } from "react-i18next"
import { TrendLineCard } from "@/components/trend-line-card"
import { useExecutionDurationTrend } from "@/hooks/use-stats"
import { formatTotalDuration } from "@/lib/format"

export function ExecutionDurationTrendChart() {
  const { t } = useTranslation()
  const [days, setDays] = useState<7 | 15 | 30>(7)
  const { data, isLoading } = useExecutionDurationTrend(days)

  return (
    <TrendLineCard
      title={t("dashboard.executionDurationTrend")}
      ariaLabel={t("dashboard.executionDurationTrendAriaLabel", { days })}
      emptyTitle={t("dashboard.noExecDurationData")}
      emptyDesc={t("dashboard.noExecDurationDataDesc")}
      dataKey="total_duration_ms"
      tooltipLabel={t("dashboard.executionDuration")}
      chartData={data?.data ?? []}
      isLoading={isLoading}
      days={days}
      onDaysChange={setDays}
      yAxisFormatter={formatTotalDuration}
      tooltipFormatter={(v) => formatTotalDuration(Number(v))}
    />
  )
}
