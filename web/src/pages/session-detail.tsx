import { useEffect, useState, type ReactNode } from "react"
import { useNavigate, useSearchParams } from "react-router-dom"
import { useTranslation } from "react-i18next"
import { useQueryClient } from "@tanstack/react-query"
import { Alert } from "@/components/arc/alert/alert"
import { Badge } from "@/components/arc/badge/badge"
import { Button } from "@/components/arc/button/button"
import { TextShimmer } from "@/components/arc/text-shimmer/text-shimmer"
import { useSessionDetail } from "@/hooks/use-session-detail"
import { DetailSection } from "@/components/detail-primitives"
import { LogViewer } from "@/components/log-viewer"
import { StatusBadge } from "@/components/status-badge"
import { PageHeader } from "@/components/page-header"
import { Panel } from "@/components/panel"
import { FadeIn } from "@/components/fade-in"
import { SkeletonCard } from "@/components/skeleton-loader"
import { EmptyState } from "@/components/empty-state"
import { CopyButton } from "@/components/copy-button"
import { TokenStatsInfoButton } from "@/components/token-stats-tooltip"
import { WorkerLink } from "@/components/worker-link"
import { cn } from "@/lib/utils"
import {
  extractMessageContent,
  formatCompactTimestamp,
  formatDuration,
  formatTimestamp,
  formatTokenCount,
  isActiveStatus,
} from "@/lib/format"
import type { WorkerExecution } from "@/lib/types"

function runDuration(execution: WorkerExecution) {
  return formatDuration(
    execution.started_at,
    execution.completed_at ?? (isActiveStatus(execution.status) ? Date.now() : execution.started_at)
  )
}

function OverviewStat({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="min-w-0">
      <dt className="text-xs text-secondary">{label}</dt>
      <dd className="mt-1 flex min-w-0 items-center gap-1.5 text-base font-medium text-foreground tabular">{children}</dd>
    </div>
  )
}

function MetaField({ label, children, mono = false }: { label: string; children: ReactNode; mono?: boolean }) {
  return (
    <div className="min-w-0">
      <dt className="text-xs text-secondary">{label}</dt>
      <dd className={cn("mt-1 text-sm text-foreground", mono ? "break-all font-mono" : "tabular")}>{children}</dd>
    </div>
  )
}

function TextBlock({ label, children }: { label: string; children: ReactNode }) {
  return (
    <section className="min-w-0">
      <h3 className="text-xs text-secondary">{label}</h3>
      <div className="mt-2 max-h-72 overflow-y-auto whitespace-pre-wrap break-words rounded-inset bg-surface-muted px-4 py-3 text-sm leading-relaxed text-foreground">
        {children}
      </div>
    </section>
  )
}

function SessionDetailSkeleton() {
  return (
    <div className="grid gap-6" aria-busy="true">
      <div className="grid gap-3">
        <div className="skeleton h-9 w-40" />
        <div className="skeleton h-4 w-72 max-w-full" />
      </div>
      <div className="grid grid-cols-2 gap-6 rounded-surface border border-border bg-surface p-6 sm:grid-cols-3 xl:grid-cols-6">
        {Array.from({ length: 6 }).map((_, index) => (
          <div key={index}>
            <div className="skeleton h-3 w-14" />
            <div className="skeleton mt-2 h-5 w-24 max-w-full" />
          </div>
        ))}
      </div>
      <SkeletonCard className="min-h-64" />
      <SkeletonCard className="min-h-96" />
    </div>
  )
}

export function SessionDetail() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()
  const currentSessionId = searchParams.get("session_id") ?? ""
  const queryClient = useQueryClient()
  const { data, error, isLoading, refetch, isRefetching } = useSessionDetail(currentSessionId)
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

  if (isLoading) return <SessionDetailSkeleton />

  const retryButton = (
    <Button variant="secondary" size="sm" loading={isRefetching} onClick={() => void refetch()}>
      {t("common.retry")}
    </Button>
  )

  if (error && executions.length === 0) {
    return (
      <FadeIn>
        <PageHeader title={t("sessionDetail.session")} />
        <Alert tone="danger" title={t("sessionDetail.loadError")}>
          {error.message}
          <span className="mt-3 flex">{retryButton}</span>
        </Alert>
      </FadeIn>
    )
  }

  const selectedExecution =
    executions.find((exec) => exec.id === selectedExecutionId) ?? latestExecution

  if (!selectedExecution || !firstExecution || !latestExecution) {
    return (
      <EmptyState
        title={t("sessionDetail.noExecutions")}
        description={t("sessionDetail.noExecutionsDesc")}
        action={
          <Button variant="secondary" onClick={() => navigate("/sessions")}>
            {t("sessionDetail.backToSessions")}
          </Button>
        }
      />
    )
  }

  const selectedTurnIndex = executions.findIndex((exec) => exec.id === selectedExecution.id) + 1
  const sessionLive = isActiveStatus(latestExecution.status)
  const sessionDuration = formatDuration(
    firstExecution.started_at,
    latestExecution.completed_at ?? (sessionLive ? Date.now() : latestExecution.started_at)
  )
  const selectedLive = isActiveStatus(selectedExecution.status)

  let workerExecution = latestExecution
  if (!workerExecution.worker_id) {
    for (let i = executions.length - 1; i >= 0; i -= 1) {
      if (executions[i].worker_id) {
        workerExecution = executions[i]
        break
      }
    }
  }

  const showNavigator = executions.length > 1
  const triggerText = extractMessageContent(selectedExecution.trigger_input)

  return (
    <FadeIn>
      <PageHeader
        title={t("sessionDetail.session")}
        subtitle={
          <span className="inline-flex max-w-full items-center gap-1">
            <span className="min-w-0 break-all font-mono">{currentSessionId}</span>
            <span className="-my-2.5 inline-flex shrink-0">
              <CopyButton value={currentSessionId} label={t("sessions.copySessionId")} />
            </span>
          </span>
        }
        actions={<StatusBadge status={latestExecution.status} size="md" />}
      />

      {error && (
        <div className="mb-6">
          <Alert tone="danger" title={t("sessionDetail.refreshError")}>
            {error.message}
            <span className="mt-3 flex">{retryButton}</span>
          </Alert>
        </div>
      )}

      <DetailSection>
        <dl className="grid grid-cols-2 gap-x-6 gap-y-5 p-6 sm:grid-cols-3 xl:grid-cols-6">
          <OverviewStat label={t("sessionDetail.worker")}>
            {workerExecution.worker_id ? (
              <WorkerLink
                id={workerExecution.worker_id}
                name={workerExecution.worker_name}
                className="truncate hover:text-accent-strong"
              />
            ) : (
              <span className="truncate">{t("sessionDetail.bee")}</span>
            )}
          </OverviewStat>
          <OverviewStat label={t("sessions.columns.turns")}>
            {t("sessions.turnCount", { count: executions.length })}
          </OverviewStat>
          <OverviewStat label={t("sessions.columns.started")}>
            <span className="truncate">{formatCompactTimestamp(firstExecution.started_at)}</span>
          </OverviewStat>
          <OverviewStat label={t("sessionDetail.lastActivity")}>
            <span className="truncate">
              {formatCompactTimestamp(latestExecution.completed_at ?? latestExecution.started_at)}
            </span>
          </OverviewStat>
          <OverviewStat label={t("sessions.columns.duration")}>
            <span>{sessionDuration}</span>
            {sessionLive && (
              <TextShimmer className="text-xs font-normal text-accent-strong">{t("sessionDetail.live")}</TextShimmer>
            )}
          </OverviewStat>
          <OverviewStat label={t("sessionDetail.tokens")}>
            {tokenStats ? (
              <>
                <span>{formatTokenCount(tokenStats.total_tokens)}</span>
                <TokenStatsInfoButton stats={tokenStats} side="bottom" align="end" />
              </>
            ) : (
              <span className="font-normal text-secondary">{t("sessionDetail.noTokens")}</span>
            )}
          </OverviewStat>
        </dl>
      </DetailSection>

      <div className={cn("mt-6 grid gap-6", showNavigator && "xl:grid-cols-[18rem_minmax(0,1fr)] xl:items-start")}>
        {showNavigator && (
          <aside className="min-w-0 xl:sticky xl:top-8">
            <Panel title={t("sessionDetail.turnNavigator")} flush>
              <ul className="max-h-80 overflow-y-auto border-t border-border-subtle xl:max-h-[calc(100dvh-12rem)]">
                {[...executions].reverse().map((exec, reverseIndex) => {
                  const turnNumber = executions.length - reverseIndex
                  const isSelected = exec.id === selectedExecution.id

                  return (
                    <li key={exec.id} className="border-t border-border-subtle first:border-t-0">
                      <button
                        type="button"
                        aria-current={isSelected ? "true" : undefined}
                        onClick={() => setSelectedExecutionId(exec.id)}
                        className={cn(
                          "flex w-full flex-col gap-1 px-6 py-3 text-left transition-colors",
                          isSelected ? "bg-accent-subtle dark:bg-surface-muted" : "hover:bg-surface-muted"
                        )}
                      >
                        <span className="flex items-center justify-between gap-3">
                          <span className="flex min-w-0 items-center gap-2">
                            <span className={cn("text-sm font-medium tabular", isSelected ? "text-accent-strong" : "text-foreground")}>
                              {t("sessionDetail.turn", { index: turnNumber })}
                            </span>
                            {reverseIndex === 0 && (
                              <Badge size="sm">{t("sessionDetail.latest")}</Badge>
                            )}
                          </span>
                          <StatusBadge status={exec.status} />
                        </span>
                        <span className="truncate text-sm text-secondary">
                          {extractMessageContent(exec.trigger_input) || t("sessionDetail.noTriggerInput")}
                        </span>
                        <span className="text-xs text-secondary tabular">
                          {formatCompactTimestamp(exec.started_at)} · {runDuration(exec)}
                        </span>
                      </button>
                    </li>
                  )
                })}
              </ul>
            </Panel>
          </aside>
        )}

        <div className="flex min-w-0 flex-col gap-6">
          <Panel
            title={
              <span className="tabular">
                {t("sessionDetail.turn", { index: selectedTurnIndex })}
              </span>
            }
            description={
              <span className="tabular">
                {formatCompactTimestamp(selectedExecution.started_at)} · {runDuration(selectedExecution)}
              </span>
            }
            action={<StatusBadge status={selectedExecution.status} />}
          >
            <div className="grid gap-6">
              <TextBlock label={t("sessionDetail.prompt")}>
                {triggerText || <span className="text-secondary">{t("sessionDetail.noTriggerInput")}</span>}
              </TextBlock>

              <TextBlock label={t("sessionDetail.result")}>
                {selectedExecution.result ? (
                  selectedExecution.result
                ) : selectedLive ? (
                  <TextShimmer className="text-secondary">{t("sessionDetail.resultPending")}</TextShimmer>
                ) : (
                  <span className="text-secondary">{t("executionDetail.noResult")}</span>
                )}
              </TextBlock>

              <dl className="grid grid-cols-2 gap-x-6 gap-y-4 border-t border-border-subtle pt-6 sm:grid-cols-3">
                <MetaField label={t("sessionDetail.execution")} mono>
                  {selectedExecution.id}
                </MetaField>
                <MetaField label={t("sessionDetail.worker")}>
                  {selectedExecution.worker_id ? (
                    <WorkerLink
                      id={selectedExecution.worker_id}
                      name={selectedExecution.worker_name}
                      className="hover:text-accent-strong"
                    />
                  ) : (
                    t("sessionDetail.bee")
                  )}
                </MetaField>
                <MetaField label={t("executionDetail.pid")} mono>
                  {selectedExecution.ai_process_pid || t("sessionDetail.notAvailable")}
                </MetaField>
                <MetaField label={t("executionDetail.started")}>
                  {formatTimestamp(selectedExecution.started_at)}
                </MetaField>
                <MetaField label={t("executionDetail.completed")}>
                  {selectedExecution.completed_at
                    ? formatTimestamp(selectedExecution.completed_at)
                    : t("sessionDetail.notAvailable")}
                </MetaField>
                <MetaField label={t("sessions.columns.duration")}>{runDuration(selectedExecution)}</MetaField>
              </dl>
            </div>
          </Panel>

          <Panel title={t("executionDetail.logs")} description={t("sessionDetail.logsHint")} flush>
            <LogViewer
              executionId={selectedExecution.id}
              status={selectedExecution.status}
              variant="embedded"
              autoScroll={selectedExecution.id === latestExecution.id}
              onComplete={
                selectedLive
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
    </FadeIn>
  )
}
