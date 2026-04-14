import { useState, useMemo, useCallback } from "react"
import { useTranslation } from "react-i18next"
import {
  LineChart,
  Line,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  Legend,
  ResponsiveContainer,
} from "recharts"
import { TrendLineCard, CHART_TOOLTIP_STYLE } from "@/components/trend-line-card"
import { useStatsTrend, useExecutionDurationTrend } from "@/hooks/use-stats"
import { formatTotalDuration } from "@/lib/format"

export function CombinedTrendChart() {
  const { t } = useTranslation()
  const [days, setDays] = useState<7 | 15 | 30>(7)

  const { data: activityData, isLoading: loadingActivity } = useStatsTrend(days)
  const { data: durationData, isLoading: loadingDuration } = useExecutionDurationTrend(days)

  const isLoading = loadingActivity || loadingDuration

  const chartData = useMemo(() => {
    const map = new Map<string, { date: string; active_workers: number; total_duration_ms: number }>()
    activityData?.data.forEach((p) =>
      map.set(p.date, { date: p.date, active_workers: p.active_workers, total_duration_ms: 0 }),
    )
    durationData?.data.forEach((p) => {
      const existing = map.get(p.date)
      if (existing) {
        existing.total_duration_ms = p.total_duration_ms
      } else {
        map.set(p.date, { date: p.date, active_workers: 0, total_duration_ms: p.total_duration_ms })
      }
    })
    return Array.from(map.values()).sort((a, b) => a.date.localeCompare(b.date))
  }, [activityData, durationData])

  const title = t("dashboard.activityTrend") + " & " + t("dashboard.executionDurationTrend")

  const tooltipFormatter = useCallback(
    (value: number | string, name: string, props: { dataKey?: string }) => {
      if (props.dataKey === "active_workers") {
        return [value, name]
      }
      return [formatTotalDuration(Number(value)), name]
    },
    [],
  )

  return (
    <TrendLineCard
      title={title}
      ariaLabel={t("dashboard.combinedTrendAriaLabel", { days })}
      emptyTitle={t("dashboard.noTrendData")}
      emptyDesc={t("dashboard.noTrendDataDesc")}
      chartData={chartData}
      isLoading={isLoading}
      days={days}
      onDaysChange={setDays}
    >
      <ResponsiveContainer width="100%" height={200}>
        <LineChart data={chartData} margin={{ top: 4, right: 8, left: -20, bottom: 0 }}>
          <CartesianGrid strokeDasharray="3 3" className="stroke-border" />
          <XAxis
            dataKey="date"
            tick={{ fontSize: 11 }}
            tickFormatter={(v: string) => v.slice(5)}
            className="text-muted-foreground"
          />
          <YAxis
            yAxisId="left"
            tick={{ fontSize: 11 }}
            allowDecimals={false}
            className="text-muted-foreground"
          />
          <YAxis
            yAxisId="right"
            orientation="right"
            tick={{ fontSize: 11 }}
            tickFormatter={(v: number) => formatTotalDuration(v)}
            className="text-muted-foreground"
            width={56}
          />
          <Tooltip
            labelFormatter={(label) => String(label)}
            formatter={tooltipFormatter}
            contentStyle={CHART_TOOLTIP_STYLE}
          />
          <Legend wrapperStyle={{ fontSize: 11, paddingTop: 8 }} />
          <Line
            yAxisId="left"
            type="monotone"
            dataKey="active_workers"
            name={t("dashboard.activeWorkers")}
            strokeWidth={2}
            dot={false}
            stroke="var(--primary)"
          />
          <Line
            yAxisId="right"
            type="monotone"
            dataKey="total_duration_ms"
            name={t("dashboard.executionDuration")}
            strokeWidth={2}
            dot={false}
            stroke="var(--chart-2, hsl(var(--chart-2, 160 60% 45%)))"
          />
        </LineChart>
      </ResponsiveContainer>
    </TrendLineCard>
  )
}
