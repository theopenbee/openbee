import { useEffect, useState } from "react"
import { Link, useParams } from "react-router-dom"
import { useTranslation } from "react-i18next"
import { useQueryClient } from "@tanstack/react-query"
import { Activity, Bot, Clock3, Logs } from "lucide-react"
import { useSessionExecutions } from "@/hooks/use-executions"
import { DetailField, DetailHero, DetailOverviewStat, DetailSection } from "@/components/detail-primitives"
import { LogViewer } from "@/components/log-viewer"
import { StatusBadge } from "@/components/status-badge"
import { PageHeader } from "@/components/page-header"
import { FadeIn } from "@/components/fade-in"
import { SkeletonPage } from "@/components/skeleton-loader"
import { EmptyState } from "@/components/empty-state"
import { cn } from "@/lib/utils"
import { formatTimestamp, formatCompactTimestamp, formatDuration, statusTone, isActiveStatus } from "@/lib/format"

function stripMetadataPrefix(input: string): string {
  const match = input.match(/^---\n[\s\S]*?\n---\n\n?/)
  return match ? input.slice(match[0].length) : input
}

export function SessionDetail() {
  const { t } = useTranslation()
  const { sessionId } = useParams<{ sessionId: string }>()
  const currentSessionId = sessionId ?? ""
  const queryClient = useQueryClient()
  const { data: executions = [], error, isLoading } = useSessionExecutions(currentSessionId)
  const [selectedExecutionId, setSelectedExecutionId] = useState<string | null>(null)

  const firstExecution = executions[0]
  const latestExecution = executions[executions.length - 1]
  const activeExecution = executions.find((exec) => isActiveStatus(exec.status))
  const preferredExecutionId = activeExecution?.id ?? latestExecution?.id ?? null

  useEffect(() => {
    if (!preferredExecutionId) {
      setSelectedExecutionId(null)
      return
    }

    setSelectedExecutionId((current) => {
      if (current && executions.some((exec) => exec.id === current)) {
        return current
      }
      return preferredExecutionId
    })
  }, [executions, preferredExecutionId])

  if (isLoading) return <SkeletonPage />

  if (!error && executions.length === 0) {
    return <EmptyState title={t("sessionDetail.noExecutions")} />
  }

  const selectedExecution =
    executions.find((exec) => exec.id === selectedExecutionId) ?? latestExecution

  if (!selectedExecution || !firstExecution || !latestExecution) {
    return <EmptyState title={t("sessionDetail.noExecutions")} />
  }

  const selectedTurnIndex = executions.findIndex((exec) => exec.id === selectedExecution.id) + 1
  const sessionDuration = formatDuration(
    firstExecution.started_at,
    latestExecution.completed_at ??
      (isActiveStatus(latestExecution.status) ? Date.now() : latestExecution.started_at)
  )

  const selectedDuration = formatDuration(
    selectedExecution.started_at,
    selectedExecution.completed_at ??
      (isActiveStatus(selectedExecution.status) ? Date.now() : selectedExecution.started_at)
  )

  let workerExecution = latestExecution
  if (!workerExecution.worker_id) {
    for (let i = executions.length - 1; i >= 0; i -= 1) {
      if (executions[i].worker_id) {
        workerExecution = executions[i]
        break
      }
    }
  }

  const hasWorker = !!workerExecution.worker_id
  const workerLabel = hasWorker
    ? workerExecution.worker_name || `${workerExecution.worker_id!.slice(0, 8)}...`
    : t("sessionDetail.bee")

  return (
    <FadeIn>
      <div className="space-y-6">
        <PageHeader
          title={t("sessionDetail.session")}
          subtitle={t("sessionDetail.summary", { count: executions.length })}
          actions={
            <div className="flex flex-wrap items-center justify-end gap-2">
              <div className="inline-flex items-center gap-2 rounded-full border border-border bg-background px-3 py-1.5 text-xs text-muted-foreground">
                <Logs className="size-3.5" />
                <span>{t("sessionDetail.latestTurn")}</span>
                <span className="font-mono text-foreground">{executions.length}</span>
              </div>
              <StatusBadge status={latestExecution.status} />
            </div>
          }
        />

        {error && (
          <p role="alert" className="text-destructive">
            {error.message}
          </p>
        )}

        <DetailHero>
          <div className="flex flex-col gap-6 p-5 sm:p-6">
            <div className="flex flex-col gap-5 lg:flex-row lg:items-end lg:justify-between">
              <div className="space-y-3">
                <p className="text-xs font-medium uppercase tracking-[0.24em] text-muted-foreground">
                  {t("sessionDetail.overview")}
                </p>
                <div className="space-y-2">
                  <h2 className="max-w-4xl break-all font-mono text-sm leading-7 text-foreground sm:text-base">
                    {currentSessionId}
                  </h2>
                  <p className="max-w-2xl text-sm leading-6 text-muted-foreground">
                    {t("sessionDetail.inspectTurnHint")}
                  </p>
                </div>
              </div>

              <div className="flex flex-wrap items-center gap-2">
                <div className="inline-flex items-center gap-2 rounded-full border border-border bg-background px-3 py-1.5 text-xs text-muted-foreground">
                  <Bot className="size-3.5" />
                  {hasWorker ? (
                    <Link
                      to={`/workers/${workerExecution.worker_id}`}
                      className="font-medium text-foreground transition-colors hover:text-primary"
                    >
                      {workerLabel}
                    </Link>
                  ) : (
                    <span className="font-medium text-foreground">{workerLabel}</span>
                  )}
                </div>

                <div className="inline-flex items-center gap-2 rounded-full border border-border bg-background px-3 py-1.5 text-xs text-muted-foreground">
                  <Activity className={cn("size-3.5", statusTone(latestExecution.status))} />
                  <span>{t("executions.columns.latestStatus")}</span>
                  <span className={cn("font-medium", statusTone(latestExecution.status))}>
                    {latestExecution.status}
                  </span>
                </div>
              </div>
            </div>

            <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
              <DetailOverviewStat
                icon={Logs}
                label={t("executions.columns.turns")}
                value={t("executions.turnCount", { count: executions.length })}
                hint={
                  <span>
                    {t("sessionDetail.latestTurn")}{" "}
                    <span className="font-mono text-foreground">{executions.length}</span>
                  </span>
                }
              />
              <DetailOverviewStat
                icon={Bot}
                label={t("sessionDetail.worker")}
                value={workerLabel}
                hint={
                  <span>
                    {t("executions.columns.latestStatus")}{" "}
                    <span className={cn("font-medium", statusTone(latestExecution.status))}>
                      {latestExecution.status}
                    </span>
                  </span>
                }
              />
              <DetailOverviewStat
                icon={Clock3}
                label={t("executions.columns.started")}
                value={formatTimestamp(firstExecution.started_at)}
                hint={formatTimestamp(latestExecution.completed_at ?? latestExecution.started_at)}
              />
              <DetailOverviewStat
                icon={Activity}
                label={t("executions.columns.duration")}
                value={sessionDuration}
                hint={
                  isActiveStatus(latestExecution.status)
                    ? t("sessionDetail.live")
                    : formatCompactTimestamp(latestExecution.completed_at)
                }
              />
            </div>
          </div>
        </DetailHero>

        <div className="grid gap-6 xl:grid-cols-[minmax(18rem,22rem)_minmax(0,1fr)]">
          <aside className="xl:sticky xl:top-6 xl:self-start">
            <DetailSection>
              <div className="border-b border-border/70 px-4 py-4 sm:px-5">
                <p className="text-xs font-medium uppercase tracking-[0.18em] text-muted-foreground">
                  {t("sessionDetail.turnNavigator")}
                </p>
                <p className="mt-2 text-sm leading-6 text-muted-foreground">
                  {t("sessionDetail.turnNavigatorHint")}
                </p>
              </div>

              <div className="max-h-[70vh] space-y-2 overflow-y-auto p-3 sm:p-4">
                {[...executions].reverse().map((exec, reverseIndex) => {
                  const turnNumber = executions.length - reverseIndex
                  const isSelected = exec.id === selectedExecution.id

                  return (
                    <button
                      key={exec.id}
                      type="button"
                      aria-pressed={isSelected}
                      onClick={() => setSelectedExecutionId(exec.id)}
                      className={cn(
                        "w-full rounded-2xl border px-3 py-3 text-left transition-all",
                        isSelected
                          ? "border-primary/20 bg-primary/5 ring-1 ring-primary/10"
                          : "border-border/60 bg-background/70 hover:border-border hover:bg-muted/40"
                      )}
                    >
                      <div className="flex items-start gap-3">
                        <div
                          className={cn(
                            "flex size-10 shrink-0 items-center justify-center rounded-2xl border text-xs font-medium",
                            isSelected
                              ? "border-primary/20 bg-background text-foreground"
                              : "border-border/70 bg-background text-muted-foreground"
                          )}
                        >
                          {turnNumber}
                        </div>

                        <div className="min-w-0 flex-1">
                          <div className="flex flex-wrap items-center gap-2">
                            <p className="text-sm font-medium text-foreground">
                              {t("sessionDetail.turn", { index: turnNumber })}
                            </p>
                            {reverseIndex === 0 && (
                              <span className="rounded-full bg-secondary px-2 py-0.5 text-[11px] font-medium text-secondary-foreground">
                                {t("sessionDetail.latest")}
                              </span>
                            )}
                          </div>

                          <p className="mt-1 truncate text-sm text-muted-foreground">
                            {stripMetadataPrefix(exec.trigger_input) || t("sessionDetail.noTriggerInput")}
                          </p>

                          <div className="mt-3 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground">
                            <span className="font-mono">{formatCompactTimestamp(exec.started_at)}</span>
                            <span className="font-mono">
                              {formatDuration(
                                exec.started_at,
                                exec.completed_at ??
                                  (isActiveStatus(exec.status) ? Date.now() : exec.started_at)
                              )}
                            </span>
                            <span className={cn("inline-flex items-center gap-1.5 font-medium", statusTone(exec.status))}>
                              <span className="size-1.5 rounded-full bg-current" />
                              {exec.status}
                            </span>
                          </div>
                        </div>
                      </div>
                    </button>
                  )
                })}
              </div>
            </DetailSection>
          </aside>

          <div className="space-y-6">
            <DetailHero>
              <div className="flex flex-col gap-4 border-b border-border/70 px-5 py-5 sm:px-6">
                <div className="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
                  <div className="space-y-3">
                    <div className="flex flex-wrap items-center gap-2">
                      <p className="text-xs font-medium uppercase tracking-[0.18em] text-muted-foreground">
                        {t("sessionDetail.inspectTurn")}
                      </p>
                      {isActiveStatus(selectedExecution.status) && (
                        <span className="rounded-full bg-status-working/10 px-2 py-0.5 text-[11px] font-medium text-status-working">
                          {t("sessionDetail.live")}
                        </span>
                      )}
                    </div>

                    <h3 className="text-2xl font-semibold tracking-tight text-foreground">
                      {t("sessionDetail.turn", { index: selectedTurnIndex })}
                    </h3>
                  </div>

                  <StatusBadge status={selectedExecution.status} />
                </div>
              </div>

              <div className="grid gap-6 px-5 py-5 sm:px-6 xl:grid-cols-[minmax(0,1.3fr)_minmax(17rem,0.9fr)]">
                <div className="space-y-5">
                  <section className="space-y-3">
                    <p className="text-xs font-medium uppercase tracking-[0.18em] text-muted-foreground">
                      {t("executionDetail.triggerInput")}
                    </p>
                    <div className="rounded-2xl border border-border/70 bg-background/80 p-4">
                      {selectedExecution.trigger_input ? (
                        <pre className="whitespace-pre-wrap break-words text-sm leading-6 text-foreground">
                          {selectedExecution.trigger_input}
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
                      <pre className="max-h-72 overflow-auto whitespace-pre-wrap break-words font-mono text-xs leading-6 text-foreground">
                        {selectedExecution.result || t("executionDetail.noResult")}
                      </pre>
                    </div>
                  </section>
                </div>

                <section className="rounded-2xl border border-border/70 bg-background/80 p-4">
                  <div className="space-y-4">
                    <p className="text-xs font-medium uppercase tracking-[0.18em] text-muted-foreground">
                      {t("sessionDetail.metadata")}
                    </p>

                    <div className="space-y-4">
                      <DetailField
                        label={t("sessionDetail.execution")}
                        value={selectedExecution.id}
                        mono
                      />

                      <DetailField
                        label={t("sessionDetail.worker")}
                        value={
                          selectedExecution.worker_id ? (
                            <Link
                              to={`/workers/${selectedExecution.worker_id}`}
                              className="transition-colors hover:text-primary"
                            >
                              {selectedExecution.worker_name || selectedExecution.worker_id}
                            </Link>
                          ) : (
                            t("sessionDetail.bee")
                          )
                        }
                      />

                      <DetailField label={t("executionDetail.session")} value={currentSessionId} mono />
                      <DetailField label={t("executions.columns.duration")} value={selectedDuration} />
                      <DetailField label={t("executionDetail.started")} value={formatTimestamp(selectedExecution.started_at)} />
                      <DetailField label={t("executionDetail.completed")} value={formatTimestamp(selectedExecution.completed_at)} />
                      <DetailField label={t("executionDetail.pid")} value={selectedExecution.ai_process_pid || "—"} mono />
                    </div>
                  </div>
                </section>
              </div>
            </DetailHero>

            <DetailSection>
              <div className="border-b border-border/70 px-5 py-4 sm:px-6">
                <div>
                  <p className="text-xs font-medium uppercase tracking-[0.18em] text-muted-foreground">
                    {t("executionDetail.logs")}
                  </p>
                  <p className="mt-1 text-sm leading-6 text-muted-foreground">
                    {t("sessionDetail.logsHint")}
                  </p>
                </div>
              </div>

              <div className="px-5 py-5 sm:px-6">
                <LogViewer
                  executionId={selectedExecution.id}
                  status={selectedExecution.status}
                  variant="embedded"
                  autoScroll={selectedExecution.id === latestExecution.id}
                  onComplete={
                    isActiveStatus(selectedExecution.status)
                      ? () =>
                          queryClient.invalidateQueries({
                            queryKey: ["sessions", currentSessionId, "executions"],
                          })
                      : undefined
                  }
                />
              </div>
            </DetailSection>
          </div>
        </div>
      </div>
    </FadeIn>
  )
}
