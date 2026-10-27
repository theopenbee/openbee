import { useTranslation } from "react-i18next"
import { Panel } from "@/components/panel"
import { LoadErrorAlert } from "@/components/dashboard-load-error"
import { useVersion } from "@/hooks/use-version"
import { cn } from "@/lib/utils"

function shortCommit(commit: string): string {
  return /^[0-9a-f]{7,}$/i.test(commit) ? commit.slice(0, 7) : commit
}

function formatBuildDate(value: string, locale: string): string {
  const parsed = new Date(value)
  if (!value || Number.isNaN(parsed.getTime())) return value
  return new Intl.DateTimeFormat(locale, { dateStyle: "medium", timeStyle: "short" }).format(parsed)
}

type InfoRow = { label: string; value: string; full?: string; mono?: boolean }

const ROW = "flex min-h-11 items-center justify-between gap-4 border-b border-border-subtle px-6 py-2 last:border-b-0"

export function SystemInfoCard({ className }: { className?: string }) {
  const { t, i18n } = useTranslation()
  const { data, isLoading, error, refetch, isRefetching } = useVersion()

  const rows: InfoRow[] = data
    ? [
        { label: t("dashboard.version"), value: data.version },
        { label: t("dashboard.commit"), value: shortCommit(data.commit), full: data.commit, mono: true },
        { label: t("dashboard.buildDate"), value: formatBuildDate(data.date, i18n.language), full: data.date },
        { label: t("dashboard.runtime"), value: data.go_version },
        { label: t("dashboard.platform"), value: `${data.os}/${data.arch}` },
      ]
    : []

  return (
    <Panel title={t("dashboard.systemInfo")} ariaLabel={t("dashboard.systemInfo")} className={className} flush>
      {error && !data ? (
        <div className="px-6 pb-4">
          <LoadErrorAlert
            title={t("dashboard.systemInfoError", "System info could not load")}
            error={error}
            retrying={isRefetching}
            onRetry={() => refetch()}
          />
        </div>
      ) : (
        <dl className="border-t border-border-subtle" aria-busy={isLoading}>
          {isLoading
            ? Array.from({ length: 5 }, (_, i) => (
                <div key={i} className={ROW} aria-hidden="true">
                  <div className="skeleton h-3.5 w-16" />
                  <div className="skeleton h-3.5 w-24" />
                </div>
              ))
            : rows.map(({ label, value, full, mono }) => (
                <div key={label} className={ROW}>
                  <dt className="shrink-0 text-sm text-secondary">{label}</dt>
                  <dd
                    className={cn("min-w-0 truncate text-right text-sm text-foreground tabular", mono && "font-mono")}
                    title={full ?? value}
                  >
                    {value}
                  </dd>
                </div>
              ))}
        </dl>
      )}
    </Panel>
  )
}
