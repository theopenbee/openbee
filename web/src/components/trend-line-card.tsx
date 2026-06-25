import type { ReactNode } from "react"
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
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { EmptyState } from "@/components/empty-state"

export const DAY_OPTIONS = [7, 15, 30] as const
export type DayOption = typeof DAY_OPTIONS[number]

export const CHART_TOOLTIP_STYLE = {
  background: "var(--card)",
  border: "1px solid var(--border)",
  borderRadius: "var(--radius-sm)",
  color: "var(--card-foreground)",
  fontSize: 12,
} as const

type TrendLineCardBase = {
  title: string
  ariaLabel: string
  emptyTitle: string
  emptyDesc: string
  chartData: object[]
  isLoading: boolean
  days: DayOption
  onDaysChange: (d: DayOption) => void
  yAxisFormatter?: (v: number) => string
  tooltipFormatter?: (value: number) => string | number
}

type TrendLineCardProps =
  | (TrendLineCardBase & { children: ReactNode; dataKey?: never; tooltipLabel?: never })
  | (TrendLineCardBase & { children?: never; dataKey: string; tooltipLabel: string })

export function TrendLineCard({
  title,
  ariaLabel,
  emptyTitle,
  emptyDesc,
  dataKey,
  tooltipLabel,
  chartData,
  isLoading,
  days,
  onDaysChange,
  yAxisFormatter,
  tooltipFormatter,
  children,
}: TrendLineCardProps) {
  const { t } = useTranslation()

  // Bare block (no card wrapper): embeds beneath a Panel's own title and
  // hairline divider. The subtitle reads as a quiet section label, with the
  // day-range toggle aligned to its right.
  return (
    <div>
      <div className="mb-3 flex items-center justify-between gap-3">
        <h3 className="min-w-0 truncate text-[11px] font-medium uppercase tracking-[0.18em] text-muted-foreground">
          {title}
        </h3>
        <div className="flex gap-1 shrink-0" role="group" aria-label={title}>
          {DAY_OPTIONS.map((d) => (
            <Button
              key={d}
              variant={days === d ? "default" : "ghost"}
              size="sm"
              className="h-7 px-2 text-xs"
              aria-pressed={days === d}
              aria-label={t("dashboard.daysLabel", { count: d })}
              onClick={() => onDaysChange(d)}
            >
              {d}{t("dashboard.days")}
            </Button>
          ))}
        </div>
      </div>
      <div>
        {isLoading ? (
          <Skeleton className="h-48 w-full" />
        ) : chartData.length === 0 ? (
          <EmptyState title={emptyTitle} description={emptyDesc} />
        ) : (
          // left: -20 offsets the YAxis tick label width so the chart aligns flush with card edges
          <div role="img" aria-label={ariaLabel}>
            {children ?? (
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
                    tick={{ fontSize: 11 }}
                    allowDecimals={false}
                    tickFormatter={yAxisFormatter}
                    className="text-muted-foreground"
                  />
                  <Tooltip
                    labelFormatter={(label) => String(label)}
                    formatter={(value) => [
                      tooltipFormatter ? tooltipFormatter(Number(value)) : value,
                      tooltipLabel,
                    ]}
                    contentStyle={CHART_TOOLTIP_STYLE}
                  />
                  <Legend wrapperStyle={{ fontSize: 11, paddingTop: 8 }} />
                  <Line
                    type="monotone"
                    dataKey={dataKey}
                    name={tooltipLabel}
                    strokeWidth={2}
                    dot={false}
                    stroke="var(--primary)"
                  />
                </LineChart>
              </ResponsiveContainer>
            )}
          </div>
        )}
      </div>
    </div>
  )
}
