import { useState, useMemo } from "react"
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

const ACTIVE_WORKERS_KEY = "active_workers"
const TOTAL_DURATION_KEY = "total_duration_ms"

// eslint-disable-next-line @typescript-eslint/no-explicit-any
function tooltipFormatter(value: any, _name: any, props: any): [string, string] {
  if (props.dataKey === ACTIVE_WORKERS_KEY) {
    return [value != null ? String(value) : "", _name]
  }
  return [formatTotalDuration(Number(Array.isArray(value) ? 0 : (value ?? 0))), _name]
}

export function CombinedTrendChart() {
  const { t } = useTranslation()
  const [days, setDays] = useState<7 | 15 | 30>(7)

  const { data: activityData, isLoading: loadingActivity } = useStatsTrend(days)
  const { data: durationData, isLoading: loadingDuration } = useExecutionDurationTrend(days)

  const isLoading = loadingActivity || loadingDuration

  // Both datasets cover the same date window (server zero-fills all days), so zip by index.
  const chartData = useMemo(
    () =>
      (activityData?.data ?? []).map((p, i) => ({
        date: p.date,
        [ACTIVE_WORKERS_KEY]: p.active_workers,
        [TOTAL_DURATION_KEY]: durationData?.data[i]?.total_duration_ms ?? 0,
      })),
    [activityData, durationData],
  )

  return (
    <TrendLineCard
      title={t("dashboard.combinedTrendTitle")}
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
            dataKey={ACTIVE_WORKERS_KEY}
            name={t("dashboard.activeWorkers")}
            strokeWidth={2}
            dot={false}
            stroke="var(--primary)"
          />
          <Line
            yAxisId="right"
            type="monotone"
            dataKey={TOTAL_DURATION_KEY}
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
