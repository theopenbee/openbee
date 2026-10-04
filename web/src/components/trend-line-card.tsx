import type { ReactNode } from "react"
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
import { Skeleton } from "@/components/ui/skeleton"
import { EmptyState } from "@/components/empty-state"
import { SegmentedControl } from "@/components/segmented-control"

export const DAY_OPTIONS = [7, 15, 30] as const
export type DayOption = typeof DAY_OPTIONS[number]

// Tooltip as a popover surface: white (popover) ground, the popover edge+drop
// shadow token, squared corners, 12px text.
export const CHART_TOOLTIP_STYLE = {
  background: "var(--popover)",
  border: "none",
  borderRadius: "var(--radius-sm)",
  boxShadow: "var(--shadow-popover)",
  color: "var(--popover-foreground)",
  fontSize: 12,
  padding: "6px 10px",
} as const

const CHART_TOOLTIP_LABEL_STYLE = {
  color: "var(--muted-foreground)",
  marginBottom: 2,
} as const

const CHART_TOOLTIP_ITEM_STYLE = {
  color: "var(--popover-foreground)",
  padding: 0,
} as const

const AXIS_TICK = { fontSize: 12, fill: "var(--muted-foreground)" } as const

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
  // hairline divider. A quiet 13px label on the left, the Kumo segmented
  // day-range control on the right.
  return (
    <div>
      <div className="mb-3 flex items-center justify-between gap-3">
        <h3 className="min-w-0 truncate text-body-sm font-medium text-muted-foreground">{title}</h3>
        <SegmentedControl
          ariaLabel={title}
          value={days}
          onChange={onDaysChange}
          options={DAY_OPTIONS.map((d) => ({
            value: d,
            label: `${d}${t("dashboard.days")}`,
            ariaLabel: t("dashboard.daysLabel", { count: d }),
          }))}
        />
      </div>
      <div>
        {isLoading ? (
          <Skeleton className="h-[200px] w-full" />
        ) : chartData.length === 0 ? (
          <EmptyState title={emptyTitle} description={emptyDesc} />
        ) : (
          <div role="img" aria-label={ariaLabel}>
            {children ?? (
              <ResponsiveContainer width="100%" height={200}>
                <LineChart data={chartData} margin={{ top: 8, right: 8, left: 0, bottom: 0 }}>
                  <CartesianGrid vertical={false} stroke="var(--hairline)" />
                  <XAxis
                    dataKey="date"
                    tick={AXIS_TICK}
                    tickFormatter={(v: string) => v.slice(5)}
                    tickLine={false}
                    axisLine={{ stroke: "var(--border)" }}
                    tickMargin={8}
                    minTickGap={16}
                    interval="equidistantPreserveStart"
                  />
                  <YAxis
                    tick={AXIS_TICK}
                    allowDecimals={false}
                    tickFormatter={yAxisFormatter}
                    tickLine={false}
                    axisLine={false}
                    tickMargin={4}
                    width={48}
                  />
                  <Tooltip
                    labelFormatter={(label) => String(label)}
                    formatter={(value) => [
                      tooltipFormatter ? tooltipFormatter(Number(value)) : value,
                      tooltipLabel,
                    ]}
                    contentStyle={CHART_TOOLTIP_STYLE}
                    labelStyle={CHART_TOOLTIP_LABEL_STYLE}
                    itemStyle={CHART_TOOLTIP_ITEM_STYLE}
                    cursor={{ stroke: "var(--border)", strokeWidth: 1 }}
                  />
                  <Line
                    type="monotone"
                    dataKey={dataKey}
                    name={tooltipLabel}
                    strokeWidth={2}
                    dot={false}
                    activeDot={{ r: 3.5, strokeWidth: 2, stroke: "var(--background)", fill: "var(--chart-1)" }}
                    stroke="var(--chart-1)"
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
