import { useId, type ReactNode } from "react"
import { useTranslation } from "react-i18next"
import { Building2, CalendarIcon, History } from "lucide-react"
import { Badge } from "@/components/arc/badge/badge"
import { EngineIcon } from "@/components/agent-icons/engine-icon"
import { CopyButton } from "@/components/copy-button"
import { DetailOverviewStat } from "@/components/detail-primitives"
import { formatEngineLabel, formatRelative, formatTimestamp } from "@/lib/format"
import type { Worker } from "@/lib/types"

function InfoRow({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="grid grid-cols-[6.5rem_minmax(0,1fr)] items-baseline gap-3 border-t border-border-subtle py-3 first:border-t-0">
      <dt className="text-sm text-secondary">{label}</dt>
      <dd className="min-w-0 text-sm text-foreground">{children}</dd>
    </div>
  )
}

function MonoValue({ value, copyLabel }: { value: string; copyLabel: string }) {
  return (
    <span className="flex min-w-0 items-center gap-1">
      <span className="min-w-0 break-all font-mono text-sm text-foreground">{value}</span>
      <CopyButton value={value} label={copyLabel} className="shrink-0" />
    </span>
  )
}

export function WorkerOverview({ worker }: { worker: Worker }) {
  const { t } = useTranslation()
  const headingId = useId()
  const departments = worker.departments ?? []

  return (
    <div className="grid gap-6">
      <div className="grid grid-cols-2 gap-3">
        <DetailOverviewStat
          icon={CalendarIcon}
          label={t("workerDetail.created")}
          value={formatRelative(worker.created_at, t)}
          hint={formatTimestamp(worker.created_at)}
          valueClassName="tabular"
        />
        <DetailOverviewStat
          icon={History}
          label={t("workerDetail.updated")}
          value={formatRelative(worker.updated_at, t)}
          hint={formatTimestamp(worker.updated_at)}
          valueClassName="tabular"
        />
      </div>

      <section className="grid gap-1" aria-labelledby={headingId}>
        <h4 id={headingId} className="text-sm font-medium text-foreground">
          {t("workerDetail.workerInfo")}
        </h4>
        <dl>
          <InfoRow label={t("workerDetail.id")}>
            <MonoValue value={worker.id} copyLabel={t("workerDetail.copyId")} />
          </InfoRow>
          <InfoRow label={t("workers.form.engine")}>
            {worker.engine ? (
              <span className="inline-flex items-center gap-2">
                <EngineIcon engine={worker.engine} className="size-4" />
                {formatEngineLabel(worker.engine, t)}
              </span>
            ) : (
              <span className="text-secondary">{t("workers.noEngine")}</span>
            )}
          </InfoRow>
          <InfoRow label={t("departments.title")}>
            {departments.length > 0 ? (
              <span className="flex flex-wrap items-center gap-1.5">
                {departments.map((d) => (
                  <Badge key={d.id} size="sm" icon={<Building2 size={12} strokeWidth={1.75} />}>
                    {d.name}
                  </Badge>
                ))}
              </span>
            ) : (
              <span className="text-secondary">{t("departments.ungrouped")}</span>
            )}
          </InfoRow>
          <InfoRow label={t("workerDetail.workDir")}>
            {worker.work_dir ? (
              <MonoValue value={worker.work_dir} copyLabel={t("workerDetail.copyWorkDir")} />
            ) : (
              <span className="text-secondary">{t("workerDetail.defaultWorkDir")}</span>
            )}
          </InfoRow>
        </dl>
      </section>
    </div>
  )
}
