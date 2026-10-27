import { useMemo, useState } from "react"
import { useTranslation } from "react-i18next"
import { LineChart, type LineChartDatum } from "@/components/arc/line-chart/line-chart"
import { EmptyState } from "@/components/empty-state"
import { LoadErrorAlert } from "@/components/dashboard-load-error"
import { useTokenTrend } from "@/hooks/use-stats"
import { formatTokenCountAxis } from "@/lib/format"

export type TrendRange = 7 | 15 | 30

function dayFormatter(locale: string, options: Intl.DateTimeFormatOptions) {
  const format = new Intl.DateTimeFormat(locale, { ...options, timeZone: "UTC" })
  return (date: string) => {
    const parsed = new Date(`${date}T00:00:00Z`)
    return Number.isNaN(parsed.getTime()) ? date : format.format(parsed)
  }
}

export function TokenTrendChart({ days }: { days: TrendRange }) {
  const { t, i18n } = useTranslation()
  const { data, isLoading, error, refetch, isRefetching } = useTokenTrend(days)
  const [lastData, setLastData] = useState(data)
  if (data && data !== lastData) setLastData(data)
  const trend = data ?? lastData

  const points = useMemo<LineChartDatum[]>(() => {
    const long = dayFormatter(i18n.language, { month: "short", day: "numeric", weekday: "short" })
    const short = dayFormatter(i18n.language, { month: "numeric", day: "numeric" })
    return (trend?.data ?? []).map((point) => ({
      key: point.date,
      label: long(point.date),
      axisLabel: short(point.date),
      values: { total_tokens: point.total_tokens },
    }))
  }, [trend, i18n.language])

  if (error && !trend) {
    return (
      <LoadErrorAlert
        title={t("dashboard.trendLoadError", "Token trend could not load")}
        error={error}
        retrying={isRefetching}
        onRetry={() => refetch()}
      />
    )
  }

  if (!isLoading && points.length === 0) {
    return <EmptyState title={t("dashboard.noTokenData")} description={t("dashboard.noTokenDataDesc")} />
  }

  return (
    <LineChart
      label={t("dashboard.tokensTrendAriaLabel", { days })}
      data={points}
      series={[{ key: "total_tokens", label: t("dashboard.tokens") }]}
      height={232}
      loading={isLoading}
      formatTick={formatTokenCountAxis}
      categoryLabel={t("dashboard.dateLabel", "Date")}
      emptyLabel={t("dashboard.noTokenData")}
    />
  )
}
