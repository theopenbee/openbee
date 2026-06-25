import { useTranslation } from "react-i18next"
import { Hexagon } from "lucide-react"
import { LogoFull } from "@/components/brand/logo"
import { Skeleton } from "@/components/ui/skeleton"
import { useStatsOverview } from "@/hooks/use-stats"
import { formatNumber } from "@/lib/format"
import { EYEBROW_LABEL } from "@/lib/styles"

// The hero panel is the one place brand orange washes a whole surface: a faint
// tint (well under the visual weight of a fill) plus a brand-toned hairline, so
// the OpenBee mark and the org's headline counts read as the page's anchor
// without breaking the One-Orange restraint elsewhere. A hive watermark — a
// cluster of oversized hexagon outlines bleeding off the right edge, under a
// soft brand radial wash — gives the surface depth and grounds the metaphor
// without tipping into decoration that competes with the numbers.
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
      className="relative overflow-hidden rounded-sm border border-brand/25 bg-brand/[0.06] p-6"
      aria-label={t("dashboard.basicInfo")}
    >
      <div className="pointer-events-none absolute inset-0" aria-hidden>
        <div className="absolute inset-0 bg-[radial-gradient(120%_130%_at_92%_-15%,oklch(0.7_0.161_49/0.16),transparent_55%)]" />
        <Hexagon
          className="absolute -top-10 -right-6 size-44 -rotate-[18deg] text-brand/[0.10]"
          strokeWidth={1.25}
        />
        <Hexagon
          className="absolute top-16 right-28 size-24 rotate-[8deg] text-brand/[0.07]"
          strokeWidth={1.25}
        />
        <Hexagon
          className="absolute -bottom-8 right-16 size-20 rotate-[14deg] text-brand/[0.06]"
          strokeWidth={1.25}
        />
      </div>

      <div className="relative">
        <LogoFull className="h-7" />

        <dl className="mt-7 grid grid-cols-3">
          {stats.map(({ label, value }, i) => (
            <div
              key={label}
              className={i !== 0 ? "border-l border-brand/25 pl-5" : "pr-5"}
            >
              <dt className={EYEBROW_LABEL}>{label}</dt>
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
      </div>
    </section>
  )
}
