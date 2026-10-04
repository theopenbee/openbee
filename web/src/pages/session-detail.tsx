import { useEffect, useState } from "react"
import { Link, useSearchParams } from "react-router-dom"
import { useTranslation } from "react-i18next"
import { useQueryClient } from "@tanstack/react-query"
import type { ReactNode } from "react"
import { useSessionDetail } from "@/hooks/use-session-detail"
import { LogViewer } from "@/components/log-viewer"
import { StatusBadge } from "@/components/status-badge"
import { PageHeader } from "@/components/page-header"
import { Panel } from "@/components/panel"
import { CopyButton } from "@/components/copy-button"
import { Badge } from "@/components/ui/badge"
import { FadeIn } from "@/components/fade-in"
import { SkeletonPage } from "@/components/skeleton-loader"
import { EmptyState } from "@/components/empty-state"
import { TokenStatsInfoButton } from "@/components/token-stats-tooltip"
import { cn } from "@/lib/utils"
import { ALERT_DESTRUCTIVE, FIELD_LABEL } from "@/lib/styles"
import { formatTimestamp, formatCompactTimestamp, formatDuration, formatTokenCount, isActiveStatus, extractMessageContent } from "@/lib/format"

// One cell of the overview metric strip: 12px caption over a 16px value.
// Cells share hairline gutters (gap-px over bg-hairline) instead of carrying
// their own outlines, so the strip reads as one surface inside the Panel.
function OverviewCell({
  label,
  value,
  hint,
  className,
}: {
  label: string
  value: ReactNode
  hint?: ReactNode
  className?: string
}) {
  return (
    <div className={cn("min-w-0 bg-card px-4 py-3.5", className)}>
      <dt className={FIELD_LABEL}>{label}</dt>
      <dd className="mt-1.5 text-base leading-6 font-semibold break-words text-strong tabular-nums">{value}</dd>
      {hint ? <dd className="mt-0.5 text-[13px] text-muted-foreground tabular-nums">{hint}</dd> : null}
    </div>
  )
}

// Key/value cell for the execution metadata grid: caption above a 13px value,
// cells separated by hairline gutters like the overview strip.
function MetaCell({ label, value, mono = false }: { label: string; value: ReactNode; mono?: boolean }) {
  return (
    <div className="min-w-0 bg-card px-4 py-2.5">
      <dt className={FIELD_LABEL}>{label}</dt>
      <dd className={cn("mt-0.5 text-[13px] text-foreground", mono ? "font-mono break-all" : "break-words tabular-nums")}>{value}</dd>
    </div>
  )
}

export function SessionDetail() {
  const { t } = useTranslation()
  const [searchParams] = useSearchParams()
  const currentSessionId = searchParams.get("session_id") ?? ""
  const queryClient = useQueryClient()
  const { data, error, isLoading } = useSessionDetail(currentSessionId)
  const executions = data?.executions ?? []
  const tokenStats = data?.token_stats ?? null
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
          actions={<StatusBadge status={latestExecution.status} />}
        />

        {error && (
          <div role="alert" className={ALERT_DESTRUCTIVE}>
            {error.message}
          </div>
        )}

        <Panel
          title={t("sessionDetail.overview")}
          description={t("sessionDetail.inspectTurnHint")}
          flush
        >
          <div className="flex items-start gap-2 border-b border-hairline px-4 py-3">
            <div className="min-w-0 flex-1">
              <p className={FIELD_LABEL}>{t("executionDetail.session")}</p>
              <p className="mt-0.5 font-mono text-[13px] break-all text-foreground">{currentSessionId}</p>
            </div>
            <CopyButton value={currentSessionId} className="mt-4 p-1" />
          </div>

          <dl className="grid grid-cols-2 gap-px overflow-hidden rounded-b-sm bg-hairline xl:grid-cols-5">
            <OverviewCell
              label={t("sessions.columns.turns")}
              value={t("sessions.turnCount", { count: executions.length })}
            />
            <OverviewCell
              label={t("sessionDetail.worker")}
              value={
                hasWorker ? (
                  <Link
                    to={`/workers/${workerExecution.worker_id}`}
                    className="text-link underline-offset-4 hover:underline"
                  >
                    {workerLabel}
                  </Link>
                ) : (
                  workerLabel
                )
              }
            />
            <OverviewCell
              label={t("sessions.columns.started")}
              value={formatTimestamp(firstExecution.started_at)}
            />
            <OverviewCell
              label={t("sessions.columns.duration")}
              value={sessionDuration}
              hint={
                isActiveStatus(latestExecution.status)
                  ? t("sessionDetail.live")
                  : formatCompactTimestamp(latestExecution.completed_at)
              }
            />
            <OverviewCell
              className="col-span-2 xl:col-span-1"
              label={t("sessionDetail.tokens")}
              value={
                tokenStats ? (
                  <span className="inline-flex items-center gap-1">
                    <span>{formatTokenCount(tokenStats.total_tokens)}</span>
                    <TokenStatsInfoButton stats={tokenStats} side="bottom" align="start" />
                  </span>
                ) : (
                  "—"
                )
              }
            />
          </dl>
        </Panel>

        <div className="grid items-start gap-6 xl:grid-cols-[minmax(18rem,22rem)_minmax(0,1fr)]">
          <Panel
            title={t("sessionDetail.turnNavigator")}
            description={t("sessionDetail.turnNavigatorHint")}
            flush
            className="xl:sticky xl:top-6"
            bodyClassName="xl:max-h-[calc(100dvh-12rem)] xl:overflow-y-auto"
          >
            <ul className="divide-y divide-hairline">
              {[...executions].reverse().map((exec, reverseIndex) => {
                const turnNumber = executions.length - reverseIndex
                const isSelected = exec.id === selectedExecution.id

                return (
                  <li key={exec.id}>
                    <button
                      type="button"
                      aria-pressed={isSelected}
                      onClick={() => setSelectedExecutionId(exec.id)}
                      className={cn(
                        "block w-full px-4 py-3 text-left transition-colors outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-inset",
                        isSelected ? "bg-accent" : "hover:bg-elevated"
                      )}
                    >
                      <div className="flex items-center justify-between gap-2">
                        <div className="flex min-w-0 items-center gap-2">
                          <span
                            className={cn(
                              "text-sm text-strong",
                              isSelected ? "font-semibold" : "font-medium"
                            )}
                          >
                            {t("sessionDetail.turn", { index: turnNumber })}
                          </span>
                          {reverseIndex === 0 && (
                            <Badge variant="outline">{t("sessionDetail.latest")}</Badge>
                          )}
                        </div>
                        <StatusBadge status={exec.status} />
                      </div>

                      <p className="mt-1 truncate text-[13px] text-foreground">
                        {extractMessageContent(exec.trigger_input) || t("sessionDetail.noTriggerInput")}
                      </p>

                      <div className="mt-1 flex flex-wrap items-center gap-x-2 text-xs text-muted-foreground tabular-nums">
                        <span>{formatCompactTimestamp(exec.started_at)}</span>
                        <span aria-hidden="true">·</span>
                        <span>
                          {formatDuration(
                            exec.started_at,
                            exec.completed_at ??
                              (isActiveStatus(exec.status) ? Date.now() : exec.started_at)
                          )}
                        </span>
                      </div>
                    </button>
                  </li>
                )
              })}
            </ul>
          </Panel>

          <div className="min-w-0 space-y-6">
            <Panel
              title={t("sessionDetail.turn", { index: selectedTurnIndex })}
              action={<StatusBadge status={selectedExecution.status} />}
              flush
            >
              <dl
                aria-label={t("sessionDetail.metadata")}
                className="grid grid-cols-2 gap-px border-b border-hairline bg-hairline lg:grid-cols-3"
              >
                <MetaCell label={t("sessionDetail.execution")} value={selectedExecution.id} mono />
                <MetaCell
                  label={t("sessionDetail.worker")}
                  value={
                    selectedExecution.worker_id ? (
                      <Link
                        to={`/workers/${selectedExecution.worker_id}`}
                        className="text-link underline-offset-4 hover:underline"
                      >
                        {selectedExecution.worker_name || selectedExecution.worker_id}
                      </Link>
                    ) : (
                      t("sessionDetail.bee")
                    )
                  }
                />
                <MetaCell label={t("executionDetail.pid")} value={selectedExecution.ai_process_pid || "—"} mono />
                <MetaCell label={t("executionDetail.started")} value={formatTimestamp(selectedExecution.started_at)} />
                <MetaCell label={t("executionDetail.completed")} value={formatTimestamp(selectedExecution.completed_at)} />
                <MetaCell label={t("sessions.columns.duration")} value={selectedDuration} />
              </dl>

              <div className="space-y-4 p-4">
                <section>
                  <h3 className={FIELD_LABEL}>{t("executionDetail.triggerInput")}</h3>
                  <div className="mt-1.5 rounded-sm bg-recessed px-3 py-2.5">
                    {selectedExecution.trigger_input ? (
                      <pre className="font-sans text-sm leading-6 break-words whitespace-pre-wrap text-foreground">
                        {extractMessageContent(selectedExecution.trigger_input)}
                      </pre>
                    ) : (
                      <p className="text-sm text-muted-foreground">{t("sessionDetail.noTriggerInput")}</p>
                    )}
                  </div>
                </section>

                <section>
                  <h3 className={FIELD_LABEL}>{t("executionDetail.result")}</h3>
                  <pre className="mt-1.5 max-h-72 overflow-auto rounded-sm bg-recessed px-3 py-2.5 font-mono text-[13px] leading-6 break-words whitespace-pre-wrap text-foreground">
                    {selectedExecution.result || t("executionDetail.noResult")}
                  </pre>
                </section>
              </div>
            </Panel>

            <Panel
              title={t("executionDetail.logs")}
              description={t("sessionDetail.logsHint")}
              flush
            >
              <LogViewer
                executionId={selectedExecution.id}
                status={selectedExecution.status}
                variant="embedded"
                autoScroll={selectedExecution.id === latestExecution.id}
                onComplete={
                  isActiveStatus(selectedExecution.status)
                    ? () =>
                        queryClient.invalidateQueries({
                          queryKey: ["sessions", currentSessionId],
                        })
                    : undefined
                }
              />
            </Panel>
          </div>
        </div>
      </div>
    </FadeIn>
  )
}
