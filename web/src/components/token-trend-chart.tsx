import { useState } from "react"
import { useTranslation } from "react-i18next"
import { TrendLineCard } from "@/components/trend-line-card"
import { useTokenTrend } from "@/hooks/use-stats"
import { formatTokenCount } from "@/lib/format"

export function TokenTrendChart() {
  const { t } = useTranslation()
  const [days, setDays] = useState<7 | 15 | 30>(7)
  const { data, isLoading } = useTokenTrend(days)

  return (
    <TrendLineCard
      title={t("dashboard.tokensTrend")}
      ariaLabel={t("dashboard.tokensTrendAriaLabel", { days })}
      emptyTitle={t("dashboard.noTokenData")}
      emptyDesc={t("dashboard.noTokenDataDesc")}
      dataKey="total_tokens"
      tooltipLabel={t("dashboard.tokens")}
      chartData={data?.data ?? []}
      isLoading={isLoading}
      days={days}
      onDaysChange={setDays}
      yAxisFormatter={(v) => formatTokenCount(v)}
      tooltipFormatter={(v) => formatTokenCount(v)}
    />
  )
}
