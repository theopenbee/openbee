import { Link, useParams } from "react-router-dom"
import { useTranslation } from "react-i18next"
import { Activity, ArrowUpRight, Bot, Clock3, Logs } from "lucide-react"
import { useExecution } from "@/hooks/use-executions"
import { DetailField, DetailHero, DetailOverviewStat, DetailSection } from "@/components/detail-primitives"
import { LogViewer } from "@/components/log-viewer"
import { StatusBadge } from "@/components/status-badge"
import { PageHeader } from "@/components/page-header"
import { FadeIn } from "@/components/fade-in"
import { SkeletonPage } from "@/components/skeleton-loader"
import { cn } from "@/lib/utils"

function isActiveStatus(status: string) {
  return status === "running" || status === "pending"
}

function formatTimestamp(value: number | null | undefined) {
  if (!value) return "—"
  return new Date(value).toLocaleString()
}

function formatCompactTimestamp(value: number | null | undefined) {
  if (!value) return "—"
  return new Date(value).toLocaleString([], {
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  })
}

function formatDuration(startMs: number | null | undefined, endMs: number | null | undefined) {
  if (!startMs || !endMs) return "—"
  const diff = endMs - startMs
  if (diff < 0) return "—"

  const totalSec = Math.floor(diff / 1000)
  const h = Math.floor(totalSec / 3600)
  const m = Math.floor((totalSec % 3600) / 60)
  const s = totalSec % 60

  if (h > 0) return `${h}h ${m}m`
  if (m > 0) return `${m}m ${s}s`
  return `${s}s`
}

function statusTone(status: string) {
  switch (status) {
    case "running":
      return "text-status-working"
    case "completed":
      return "text-status-idle"
    case "failed":
      return "text-status-error"
    default:
      return "text-muted-foreground"
  }
}

export function ExecutionDetail() {
  const { t } = useTranslation()
  const { id } = useParams<{ id: string }>()
  const { data: execution, error: fetchError, isLoading, refetch } = useExecution(id!)

  if (isLoading) return <SkeletonPage />

  if (fetchError || !execution) {
    return (
      <p role="alert" className="text-destructive">
        {fetchError?.message ?? t("executionDetail.notFound")}
      </p>
    )
  }

  const isLive = isActiveStatus(execution.status)
  const duration = formatDuration(
    execution.started_at,
    execution.completed_at ?? (isLive ? Date.now() : execution.started_at)
  )
  const workerLabel = execution.worker_id
    ? execution.worker_name || `${execution.worker_id.slice(0, 8)}...`
    : t("sessionDetail.bee")

  return (
    <FadeIn>
      <div className="space-y-6">
        <PageHeader
          title={t("executionDetail.title")}
          subtitle={t("executionDetail.summary")}
          actions={
            <div className="flex flex-wrap items-center justify-end gap-2">
              {isLive && (
                <span className="rounded-full bg-status-working/10 px-2 py-0.5 text-[11px] font-medium text-status-working">
                  {t("sessionDetail.live")}
                </span>
              )}
              <StatusBadge status={execution.status} />
            </div>
          }
        />

        <DetailHero>
          <div className="flex flex-col gap-6 p-5 sm:p-6">
            <div className="flex flex-col gap-5 lg:flex-row lg:items-end lg:justify-between">
              <div className="space-y-3">
                <div className="flex flex-wrap items-center gap-2">
                  <p className="text-xs font-medium uppercase tracking-[0.24em] text-muted-foreground">
                    {t("executionDetail.overview")}
                  </p>
                  {isLive && (
                    <span className="rounded-full bg-status-working/10 px-2 py-0.5 text-[11px] font-medium text-status-working">
                      {t("sessionDetail.live")}
                    </span>
                  )}
                </div>

                <div className="space-y-2">
                  <h2 className="max-w-4xl break-all font-mono text-sm leading-7 text-foreground sm:text-base">
                    {execution.id}
                  </h2>
                  <p className="max-w-2xl text-sm leading-6 text-muted-foreground">
                    {t("executionDetail.inspectHint")}
                  </p>
                </div>
              </div>

              <div className="flex flex-wrap items-center gap-2">
                {execution.worker_id ? (
                  <Link
                    to={`/workers/${execution.worker_id}`}
                    className="inline-flex max-w-full items-center gap-2 rounded-full border border-border bg-background px-3 py-1.5 text-xs text-muted-foreground transition-colors hover:border-primary/20 hover:bg-primary/5"
                  >
                    <Bot className="size-3.5 shrink-0" />
                    <span>{t("executionDetail.worker")}</span>
                    <span className="truncate font-medium text-foreground">{workerLabel}</span>
                    <ArrowUpRight className="size-3.5 shrink-0 text-muted-foreground" />
                  </Link>
                ) : (
                  <div className="inline-flex items-center gap-2 rounded-full border border-border bg-background px-3 py-1.5 text-xs text-muted-foreground">
                    <Bot className="size-3.5" />
                    <span>{t("executionDetail.worker")}</span>
                    <span className="font-medium text-foreground">{workerLabel}</span>
                  </div>
                )}

                <Link
                  to={`/sessions/${execution.session_id}`}
                  className="inline-flex max-w-full items-center gap-2 rounded-full border border-border bg-background px-3 py-1.5 text-xs text-muted-foreground transition-colors hover:border-primary/20 hover:bg-primary/5"
                >
                  <Logs className="size-3.5 shrink-0" />
                  <span>{t("executionDetail.session")}</span>
                  <span className="max-w-48 truncate font-medium text-foreground">
                    {execution.session_id}
                  </span>
                  <ArrowUpRight className="size-3.5 shrink-0 text-muted-foreground" />
                </Link>
              </div>
            </div>

            <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
              <DetailOverviewStat
                icon={Activity}
                label={t("executions.columns.latestStatus")}
                value={
                  <span className={cn("inline-flex items-center gap-2", statusTone(execution.status))}>
                    <span className="size-2 rounded-full bg-current" />
                    <span className="font-medium">{execution.status}</span>
                  </span>
                }
              />
              <DetailOverviewStat
                icon={Clock3}
                label={t("executions.columns.started")}
                value={<span className="font-mono text-sm sm:text-base">{formatTimestamp(execution.started_at)}</span>}
                hint={formatCompactTimestamp(execution.started_at)}
              />
              <DetailOverviewStat
                icon={Clock3}
                label={t("executionDetail.completed")}
                value={
                  <span className="font-mono text-sm sm:text-base">
                    {formatTimestamp(execution.completed_at)}
                  </span>
                }
                hint={isLive ? t("sessionDetail.live") : formatCompactTimestamp(execution.completed_at)}
              />
              <DetailOverviewStat
                icon={Activity}
                label={t("executions.columns.duration")}
                value={<span className="font-mono text-sm sm:text-base">{duration}</span>}
                hint={isLive ? t("executionDetail.liveDurationHint") : undefined}
              />
            </div>
          </div>
        </DetailHero>

        <DetailSection>
          <div className="border-b border-border/70 px-5 py-4 sm:px-6">
            <div className="flex flex-col gap-2 sm:flex-row sm:items-end sm:justify-between">
              <div>
                <p className="text-xs font-medium uppercase tracking-[0.18em] text-muted-foreground">
                  {t("executionDetail.logs")}
                </p>
                <p className="mt-1 text-sm leading-6 text-muted-foreground">
                  {t("executionDetail.logsHint")}
                </p>
              </div>

              <span className="font-mono text-xs text-muted-foreground">{execution.id}</span>
            </div>
          </div>

          <div className="px-5 py-5 sm:px-6">
            <LogViewer
              executionId={execution.id}
              status={execution.status}
              variant="embedded"
              autoScroll={isLive}
              onComplete={isLive ? refetch : undefined}
            />
          </div>
        </DetailSection>

        <div className="grid gap-6 xl:grid-cols-[minmax(0,1.15fr)_minmax(18rem,0.85fr)]">
          <DetailSection>
            <div className="border-b border-border/70 px-5 py-4 sm:px-6">
              <p className="text-xs font-medium uppercase tracking-[0.18em] text-muted-foreground">
                {t("executionDetail.context")}
              </p>
              <p className="mt-2 text-sm leading-6 text-muted-foreground">
                {t("executionDetail.contextHint")}
              </p>
            </div>

            <div className="grid gap-6 px-5 py-5 sm:px-6 xl:grid-cols-[minmax(0,0.95fr)_minmax(0,1.05fr)]">
              <section className="space-y-3">
                <p className="text-xs font-medium uppercase tracking-[0.18em] text-muted-foreground">
                  {t("executionDetail.triggerInput")}
                </p>
                <div className="rounded-2xl border border-border/70 bg-background/80 p-4">
                  {execution.trigger_input ? (
                    <pre className="max-h-[26rem] overflow-auto whitespace-pre-wrap break-words text-sm leading-6 text-foreground">
                      {execution.trigger_input}
                    </pre>
                  ) : (
                    <p className="text-sm text-muted-foreground">{t("sessionDetail.noTriggerInput")}</p>
                  )}
                </div>
              </section>

              <section className="space-y-3">
                <p className="text-xs font-medium uppercase tracking-[0.18em] text-muted-foreground">
                  {t("executionDetail.result")}
                </p>
                <div className="rounded-2xl border border-border/70 bg-background/80 p-4">
                  <pre className="max-h-[26rem] overflow-auto whitespace-pre-wrap break-words font-mono text-xs leading-6 text-foreground">
                    {execution.result || t("executionDetail.noResult")}
                  </pre>
                </div>
              </section>
            </div>
          </DetailSection>

          <aside className="xl:sticky xl:top-6 xl:self-start">
            <DetailSection className="p-5 sm:p-6">
              <p className="text-xs font-medium uppercase tracking-[0.18em] text-muted-foreground">
                {t("sessionDetail.metadata")}
              </p>
              <p className="mt-2 text-sm leading-6 text-muted-foreground">
                {t("executionDetail.metadataHint")}
              </p>

              <div className="mt-6 space-y-4">
                <DetailField label={t("sessionDetail.execution")} value={execution.id} mono />
                <DetailField
                  label={t("executionDetail.worker")}
                  value={
                    execution.worker_id ? (
                      <Link
                        to={`/workers/${execution.worker_id}`}
                        className="transition-colors hover:text-primary hover:underline"
                      >
                        {execution.worker_name || execution.worker_id}
                      </Link>
                    ) : (
                      t("sessionDetail.bee")
                    )
                  }
                />
                <DetailField
                  label={t("executionDetail.session")}
                  value={
                    <Link
                      to={`/sessions/${execution.session_id}`}
                      className="font-mono transition-colors hover:text-primary hover:underline"
                    >
                      {execution.session_id}
                    </Link>
                  }
                  mono
                />
                <DetailField label={t("executions.columns.duration")} value={duration} mono />
                <DetailField label={t("executionDetail.started")} value={formatTimestamp(execution.started_at)} />
                <DetailField
                  label={t("executionDetail.completed")}
                  value={formatTimestamp(execution.completed_at)}
                />
                <DetailField
                  label={t("executionDetail.pid")}
                  value={execution.ai_process_pid || "—"}
                  mono
                />
              </div>
            </DetailSection>
          </aside>
        </div>
      </div>
    </FadeIn>
  )
}
