import { useState } from "react"
import { useTranslation } from "react-i18next"
import SegmentedControl from "@/components/arc/segmented-control/segmented-control"
import { Panel } from "@/components/panel"
import { TokenTrendChart, type TrendRange } from "@/components/token-trend-chart"

const RANGES: TrendRange[] = [7, 15, 30]

export function TokenUsageCard({ className }: { className?: string }) {
  const { t } = useTranslation()
  const [days, setDays] = useState<TrendRange>(7)

  return (
    <Panel
      title={<span className="inline-flex min-h-11 items-center">{t("dashboard.tokenUsage")}</span>}
      ariaLabel={t("dashboard.tokenUsage")}
      className={className}
      action={
        <SegmentedControl
          label={t("dashboard.rangeLabel", "Range")}
          options={RANGES.map((d) => ({ value: String(d), label: t("dashboard.rangeDays", { count: d }) }))}
          value={String(days)}
          onValueChange={(value) => setDays(Number(value) as TrendRange)}
        />
      }
    >
      <TokenTrendChart days={days} />
    </Panel>
  )
}
