import { useState } from "react"
import { useTranslation } from "react-i18next"
import {
  LineChart,
  Line,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ResponsiveContainer,
} from "recharts"
import { Card, CardContent, CardHeader } from "@/components/ui/card"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { EmptyState } from "@/components/empty-state"
import { useStatsTrend } from "@/hooks/use-stats"

const DAY_OPTIONS = [7, 15, 30] as const

export function ActivityTrendChart() {
  const { t } = useTranslation()
  const [days, setDays] = useState<7 | 15 | 30>(7)
  const { data, isLoading } = useStatsTrend(days)

  const chartData = data?.data ?? []

  return (
    <Card>
      <CardHeader className="pb-2">
        <div className="flex items-center justify-between gap-3">
          <div className="flex items-center gap-3 min-w-0 flex-1">
            <span className="text-[10px] font-semibold uppercase tracking-[0.16em] text-muted-foreground/70 whitespace-nowrap select-none">
              {t("dashboard.activityTrend")}
            </span>
            <div className="flex-1 h-px bg-border" />
          </div>
          <div className="flex gap-1 shrink-0" role="group" aria-label={t("dashboard.activityTrend")}>
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
        ) : chartData.length === 0 ? (
          <EmptyState
            title={t("dashboard.noTrendData")}
            description={t("dashboard.noTrendDataDesc")}
          />
        ) : (
          // left: -20 offsets the YAxis tick label width so the chart aligns flush with card edges
          <div
            role="img"
            aria-label={t("dashboard.activityTrendAriaLabel", { days })}
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
                <YAxis tick={{ fontSize: 11 }} allowDecimals={false} className="text-muted-foreground" />
                <Tooltip
                  labelFormatter={(label) => String(label)}
                  formatter={(value) => [value, t("dashboard.activeWorkers")]}
                  contentStyle={{
                    background: "var(--card)",
                    border: "1px solid var(--border)",
                    borderRadius: "var(--radius-md)",
                    color: "var(--card-foreground)",
                    fontSize: 12,
                  }}
                />
                <Line
                  type="monotone"
                  dataKey="active_workers"
                  strokeWidth={2}
                  dot={false}
                  stroke="var(--primary)"
                />
              </LineChart>
            </ResponsiveContainer>
          </div>
        )}
      </CardContent>
    </Card>
  )
}
