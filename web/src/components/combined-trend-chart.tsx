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
import { Card, CardContent, CardHeader } from "@/components/ui/card"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { EmptyState } from "@/components/empty-state"
import { useStatsTrend, useExecutionDurationTrend } from "@/hooks/use-stats"
import { formatTotalDuration } from "@/lib/format"

const DAY_OPTIONS = [7, 15, 30] as const

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
  const ariaLabel = t("dashboard.combinedTrendAriaLabel", { days })
  const isEmpty = !isLoading && chartData.length === 0

  return (
    <Card>
      <CardHeader className="pb-2">
        <div className="flex items-center justify-between gap-3">
          <div className="flex items-center gap-3 min-w-0 flex-1">
            <span className="text-[10px] font-semibold uppercase tracking-[0.16em] text-muted-foreground/70 whitespace-nowrap select-none">
              {title}
            </span>
            <div className="flex-1 h-px bg-border" />
          </div>
          <div className="flex gap-1 shrink-0" role="group" aria-label={title}>
            {DAY_OPTIONS.map((d) => (
              <Button
                key={d}
                variant={days === d ? "default" : "ghost"}
                size="sm"
                className="h-7 px-2 text-xs"
                aria-pressed={days === d}
                aria-label={t("dashboard.daysLabel", { count: d })}
                onClick={() => setDays(d)}
              >
                {d}{t("dashboard.days")}
              </Button>
            ))}
          </div>
        </div>
      </CardHeader>
      <CardContent>
        {isLoading ? (
          <Skeleton className="h-48 w-full" />
        ) : isEmpty ? (
          <EmptyState
            title={t("dashboard.noTrendData")}
            description={t("dashboard.noTrendDataDesc")}
          />
        ) : (
          <div role="img" aria-label={ariaLabel}>
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
                  formatter={(value, name) => {
                    if (name === t("dashboard.activeWorkers")) {
                      return [value, name]
                    }
                    return [formatTotalDuration(Number(value)), name]
                  }}
                  contentStyle={{
                    background: "var(--card)",
                    border: "1px solid var(--border)",
                    borderRadius: "var(--radius-md)",
                    color: "var(--card-foreground)",
                    fontSize: 12,
                  }}
                />
                <Legend
                  wrapperStyle={{ fontSize: 11, paddingTop: 8 }}
                />
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
          </div>
        )}
      </CardContent>
    </Card>
  )
}
