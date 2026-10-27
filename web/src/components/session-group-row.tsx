import type { ReactNode } from "react"
import { Link } from "react-router-dom"
import { useTranslation } from "react-i18next"
import { TextShimmer } from "@/components/arc/text-shimmer/text-shimmer"
import { StatusBadge } from "@/components/status-badge"
import { CopyButton } from "@/components/copy-button"
import { TokenStatsInfoButton } from "@/components/token-stats-tooltip"
import { WorkerLink } from "@/components/worker-link"
import { cn, sessionDetailPath } from "@/lib/utils"
import {
  extractMessageContent,
  formatDuration,
  formatRelative,
  formatTimestamp,
  formatTokenCount,
  isActiveStatus,
  statusLabel,
} from "@/lib/format"
import type { SessionTokenStats, WorkerExecution } from "@/lib/types"

const TURN_DOT: Record<string, string> = {
  running: "bg-accent animate-pulse-soft",
  completed: "bg-success",
  failed: "bg-danger",
}

function MetaItem({ children }: { children: ReactNode }) {
  return (
    <span className="inline-flex min-w-0 items-center before:inline-block before:w-4 before:shrink-0 before:text-center before:text-muted before:content-['·']">
      {children}
    </span>
  )
}

export function SessionTurnPips({ executions }: { executions: WorkerExecution[] }) {
  const { t } = useTranslation()
  const ordered = [...executions].reverse()
  const summary = ordered
    .map((execution, index) => `${t("sessionDetail.turn", { index: index + 1 })}: ${statusLabel(execution.status, t)}`)
    .join(", ")

  return (
    <span className="flex max-w-32 flex-wrap items-center justify-end gap-1">
      <span className="sr-only">{summary}</span>
      {ordered.map((execution, index) => (
        <span
          key={execution.id}
          aria-hidden="true"
          title={`${t("sessionDetail.turn", { index: index + 1 })}: ${statusLabel(execution.status, t)}`}
          className={cn("size-2 shrink-0 rounded-full", TURN_DOT[execution.status] ?? "bg-border-strong")}
        />
      ))}
    </span>
  )
}

export function SessionGroupRow({
  group,
  tokenStats,
  showWorker = false,
}: {
  group: WorkerExecution[]
  tokenStats?: SessionTokenStats | null
  showWorker?: boolean
}) {
  const { t } = useTranslation()
  const latest = group[0]
  const oldest = group[group.length - 1]
  const lastCompleted = group.find((execution) => execution.completed_at)
  const active = isActiveStatus(latest.status)
  const intent = extractMessageContent(oldest.trigger_input) || t("sessions.noTriggerContent")
  const duration = formatDuration(oldest.started_at, lastCompleted?.completed_at ?? null)

  return (
    <div className="group relative flex items-start gap-4 px-4 py-4 transition-colors hover:bg-surface-muted sm:items-center sm:gap-6 sm:px-6">
      <Link
        to={sessionDetailPath(latest.session_id)}
        aria-label={t("sessions.viewSession", { id: latest.session_id })}
        className="absolute inset-0"
      />

      <div className="min-w-0 flex-1">
        <p className="truncate text-sm font-medium text-foreground">{intent}</p>

        <div className="mt-1 overflow-x-clip">
          <div className="-ml-4 flex flex-wrap items-center gap-y-1 text-xs text-secondary">
            {showWorker && (
              <MetaItem>
                {latest.worker_id ? (
                  <WorkerLink
                    id={latest.worker_id}
                    name={latest.worker_name}
                    className="relative z-10 text-foreground hover:text-accent-strong"
                  />
                ) : (
                  <span className="text-foreground">{t("sessionDetail.bee")}</span>
                )}
              </MetaItem>
            )}
            <MetaItem>
              <span title={oldest.started_at ? formatTimestamp(oldest.started_at) : undefined}>
                {formatRelative(oldest.started_at, t)}
              </span>
            </MetaItem>
            <MetaItem>
              <span className="tabular">{t("sessions.turnCount", { count: group.length })}</span>
            </MetaItem>
            <MetaItem>
              {active ? (
                <TextShimmer className="text-accent-strong">{t("sessionDetail.live")}</TextShimmer>
              ) : (
                <span className="tabular">{duration}</span>
              )}
            </MetaItem>
            {tokenStats && (
              <MetaItem>
                <span className="inline-flex items-center gap-1 tabular">
                  {t("sessions.tokenTotal", { value: formatTokenCount(tokenStats.total_tokens) })}
                  <span className="relative z-10 inline-flex">
                    <TokenStatsInfoButton stats={tokenStats} />
                  </span>
                </span>
              </MetaItem>
            )}
            <MetaItem>
              <span className="inline-flex items-center gap-0.5">
                <span className="font-mono">{latest.session_id.slice(0, 8)}</span>
                <span className="relative z-10 -my-2.5 inline-flex transition-opacity pointer-fine:opacity-0 pointer-fine:group-hover:opacity-100 pointer-fine:focus-within:opacity-100">
                  <CopyButton value={latest.session_id} label={t("sessions.copySessionId")} />
                </span>
              </span>
            </MetaItem>
          </div>
        </div>
      </div>

      <div className="flex shrink-0 items-center gap-4">
        <span className="hidden md:flex">
          <SessionTurnPips executions={group} />
        </span>
        <StatusBadge status={latest.status} />
      </div>
    </div>
  )
}

export function SessionGroupRowSkeleton() {
  return (
    <div className="flex items-center gap-6 px-4 py-4 sm:px-6" aria-hidden="true">
      <div className="min-w-0 flex-1">
        <div className="skeleton h-4 w-3/5" />
        <div className="skeleton mt-2 h-3 w-2/5" />
      </div>
      <div className="skeleton h-6 w-16 rounded-full" />
    </div>
  )
}
