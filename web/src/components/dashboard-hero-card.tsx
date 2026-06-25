import { useTranslation } from "react-i18next"
import { LogoFull } from "@/components/brand/logo"
import { Skeleton } from "@/components/ui/skeleton"
import { useStatsOverview } from "@/hooks/use-stats"
import { formatNumber } from "@/lib/format"

const STAT_LABEL =
  "text-[11px] font-medium uppercase tracking-[0.18em] text-muted-foreground"

// The hero panel is the one place brand orange washes a whole surface: a faint
// tint (well under the visual weight of a fill) plus a brand-toned hairline, so
// the OpenBee mark and the org's headline counts read as the page's anchor
// without breaking the One-Orange restraint elsewhere.
export function DashboardHeroCard() {
  const { t } = useTranslation()
  const { data, isLoading } = useStatsOverview()

  const stats = [
    { label: t("dashboard.workers"), value: data?.workers },
    { label: t("dashboard.departments"), value: data?.departments },
    { label: t("dashboard.scheduledTasks"), value: data?.scheduled_tasks },
  ]

  return (
    <section
      className="rounded-sm border border-brand/25 bg-brand/[0.06] p-6"
      aria-label={t("dashboard.basicInfo")}
    >
      <LogoFull className="h-7" />

      <dl className="mt-7 grid grid-cols-3">
        {stats.map(({ label, value }, i) => (
          <div
            key={label}
            className={i !== 0 ? "border-l border-brand/15 pl-5" : "pr-5"}
          >
            <dt className={STAT_LABEL}>{label}</dt>
            <dd className="mt-2 text-3xl font-semibold tabular-nums leading-none">
              {isLoading || value === undefined ? (
                <Skeleton className="h-8 w-12" />
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
