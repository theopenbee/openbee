import { useTranslation } from "react-i18next"
import { Skeleton } from "@/components/ui/skeleton"
import { useStatsOverview } from "@/hooks/use-stats"
import { formatNumber } from "@/lib/format"
import { FIELD_LABEL } from "@/lib/styles"

// The org's headline counts as a row of plain stat tiles, the way a Cloudflare
// account home opens: white surfaces outlined by the border ring, a 12px muted
// caption, and a 24px tabular value. The mark already lives in the top bar, so
// the content carries no logo, tagline, or brand wash.
export function DashboardStats() {
  const { t } = useTranslation()
  const { data, isLoading } = useStatsOverview()

  const stats = [
    { label: t("dashboard.departments"), value: data?.departments },
    { label: t("dashboard.workers"), value: data?.workers },
    { label: t("dashboard.scheduledTasks"), value: data?.scheduled_tasks },
  ]

  return (
    <section aria-label={t("dashboard.basicInfo")}>
      <dl className="grid grid-cols-3 gap-3 sm:gap-4">
        {stats.map(({ label, value }) => (
          <div
            key={label}
            className="flex min-w-0 flex-col justify-between gap-2 rounded-sm bg-card p-3 ring-1 ring-border sm:p-4"
          >
            <dt className={FIELD_LABEL}>{label}</dt>
            <dd className="flex h-8 items-center text-xl font-semibold tabular-nums text-strong sm:text-2xl">
              {isLoading || value === undefined ? (
                <Skeleton className="h-6 w-10" />
              ) : (
                formatNumber(value)
              )}
            </dd>
          </div>
        ))}
      </dl>
    </section>
  )
}
