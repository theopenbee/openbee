import { useTranslation } from "react-i18next"
import { Panel } from "@/components/panel"
import { Skeleton } from "@/components/ui/skeleton"
import { useVersion } from "@/hooks/use-version"
import { KV_KEY, KV_ROW } from "@/lib/styles"
import { cn } from "@/lib/utils"

// A short commit SHA reads better than the full 40 chars; build tooling injects
// either, so trim defensively.
function shortCommit(commit: string): string {
  return /^[0-9a-f]{7,}$/i.test(commit) ? commit.slice(0, 7) : commit
}

const ROW = cn(KV_ROW, "flex items-center justify-between gap-4")

// Key/value rows: muted key left, mono value right (every value here is a
// version, hash, date, or platform string), divided by hairlines.
export function SystemInfoCard() {
  const { t } = useTranslation()
  const { data, isLoading } = useVersion()

  const rows: { label: string; value: string }[] = data
    ? [
        { label: t("dashboard.version"), value: data.version },
        { label: t("dashboard.commit"), value: shortCommit(data.commit) },
        { label: t("dashboard.buildDate"), value: data.date },
        { label: t("dashboard.runtime"), value: data.go_version },
        { label: t("dashboard.platform"), value: `${data.os}/${data.arch}` },
      ]
    : []

  return (
    <Panel title={t("dashboard.systemInfo")} ariaLabel={t("dashboard.systemInfo")} flush>
      <dl className="divide-y divide-hairline">
        {isLoading
          ? Array.from({ length: 5 }).map((_, i) => (
              <div key={i} className={ROW}>
                <Skeleton className="h-4 w-16" />
                <Skeleton className="h-4 w-24" />
              </div>
            ))
          : rows.map(({ label, value }) => (
              <div key={label} className={ROW}>
                <dt className={KV_KEY}>{label}</dt>
                <dd className="min-w-0 truncate font-mono text-xs text-foreground" title={value}>
                  {value}
                </dd>
              </div>
            ))}
      </dl>
    </Panel>
  )
}
