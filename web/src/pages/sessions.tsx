import { useMemo, useState } from "react"
import { Link } from "react-router-dom"
import { useTranslation } from "react-i18next"
import { Clock, Logs } from "lucide-react"
import { useExecutions } from "@/hooks/use-executions"
import type { WorkerExecution } from "@/lib/types"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { StatusBadge } from "@/components/status-badge"
import { EmptyState } from "@/components/empty-state"
import { PageHeader } from "@/components/page-header"
import { FadeIn } from "@/components/fade-in"
import { SkeletonTable } from "@/components/skeleton-loader"
import { PaginationControls } from "@/components/pagination-controls"
import { TokenStatsInfoButton } from "@/components/token-stats-tooltip"
import { cn } from "@/lib/utils"
import { ALERT_DESTRUCTIVE } from "@/lib/styles"
import { extractMessageContent, formatDuration, formatRelative, formatTokenCount, groupExecutionsBySession, isActiveStatus, STATUS_ROW_BORDER } from "@/lib/format"

const PAGE_SIZE = 20

const TURN_DOT: Record<string, string> = {
  running: "bg-status-working",
  completed: "bg-status-idle",
  failed: "bg-status-error",
  pending: "bg-muted-foreground/30",
}

function TurnPips({ executions }: { executions: WorkerExecution[] }) {
  const { t } = useTranslation()
  const ordered = [...executions].reverse()
  return (
    <div className="flex items-center gap-0.5 flex-wrap max-w-[120px]">
      {ordered.map((e, i) => (
        <div
          key={e.id}
          title={`${t("sessionDetail.turn", { index: i + 1 })}: ${t(`statuses.${e.status}`, e.status)}`}
          className={cn(
            "size-2 rounded-full shrink-0",
            TURN_DOT[e.status] ?? "bg-muted-foreground/30"
          )}
        />
      ))}
    </div>
  )
}

function SessionListItem({ group }: { group: WorkerExecution[] }) {
  const { t } = useTranslation()
  const latest = group[0]
  const oldest = group[group.length - 1]
  const intent = extractMessageContent(oldest.trigger_input)

  return (
    <li>
      <Link
        to={`/sessions/detail?session_id=${encodeURIComponent(latest.session_id)}`}
        aria-label={t("sessions.viewSession", { id: latest.session_id })}
        className="flex items-start gap-3 px-4 py-3 transition-colors hover:bg-primary/5 active:bg-primary/5 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring/50"
      >
        <div className="min-w-0 flex-1">
          <p className="truncate text-sm font-medium text-foreground">
            {intent || t("sessions.noTriggerContent")}
          </p>
          <div className="mt-1 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground">
            {latest.worker_id && (
              <span className="text-foreground/80">{latest.worker_name || latest.worker_id.slice(0, 8)}</span>
            )}
            <span className="inline-flex items-center gap-1">
              <Clock className="size-3.5 text-muted-foreground/70" aria-hidden="true" />
              {formatRelative(oldest.started_at, t)}
            </span>
            <span className="inline-flex items-center gap-1 font-mono">
              <Logs className="size-3.5 text-muted-foreground/70" aria-hidden="true" />
              {t("sessions.turnCount", { count: group.length })}
            </span>
          </div>
        </div>
        <StatusBadge status={latest.status} />
      </Link>
    </li>
  )
}

export function Sessions() {
  const { t } = useTranslation()
  const [page, setPage] = useState(1)
  const { data, error, isLoading } = useExecutions(page, PAGE_SIZE)

  const executions = data?.items ?? []
  const totalPages = Math.max(1, Math.ceil((data?.total ?? 0) / PAGE_SIZE))
  const totalSessions = data?.total ?? 0

  const sessionGroups = useMemo(() => groupExecutionsBySession(executions), [executions])

  const activeCount = sessionGroups.filter((g) => isActiveStatus(g[0].status)).length

  const subtitle =
    totalSessions > 0
      ? activeCount > 0
        ? t("sessions.summaryWithActive", { count: totalSessions, active: activeCount })
        : t("sessions.summary", { count: totalSessions })
      : undefined

  return (
    <FadeIn>
      <PageHeader title={t("sessions.title")} subtitle={subtitle} />

      {error && (
        <div role="alert" className={cn(ALERT_DESTRUCTIVE, "mb-4")}>
          {error.message}
        </div>
      )}

      {isLoading ? (
        <SkeletonTable />
      ) : sessionGroups.length === 0 && !error ? (
        <EmptyState
          title={t("emptyState.noExecutions")}
          description={t("emptyState.noExecutionsDesc")}
        />
      ) : (
        <>
          <ul className="divide-y divide-border/70 overflow-hidden rounded-sm border border-border/70 bg-card md:hidden">
            {sessionGroups.map((group) => (
              <SessionListItem key={group[0].session_id} group={group} />
            ))}
          </ul>

          <div className="hidden overflow-hidden rounded-sm border border-border/70 bg-card md:block">
            <Table>
              <TableHeader>
                <TableRow className="bg-secondary/50 hover:bg-secondary/50">
                  <TableHead className="pl-5 w-28">{t("sessions.columns.session")}</TableHead>
                  <TableHead className="w-36">{t("sessions.columns.worker")}</TableHead>
                  <TableHead>{t("sessions.columns.turns")}</TableHead>
                  <TableHead className="w-28">{t("sessions.columns.latestStatus")}</TableHead>
                  <TableHead className="w-24">{t("sessions.columns.started")}</TableHead>
                  <TableHead className="w-20">{t("sessions.columns.duration")}</TableHead>
                  <TableHead className="w-24">{t("sessions.columns.tokens")}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {sessionGroups.map((group) => {
                  const latest = group[0]
                  const oldest = group[group.length - 1]
                  const lastCompleted = group.find((e) => e.completed_at)
                  const isActive = latest.status === "running" || latest.status === "pending"
                  const duration = formatDuration(oldest.started_at, lastCompleted?.completed_at ?? null)
                  const tokenStats = data?.token_stats?.[latest.session_id] ?? null

                  return (
                    <TableRow
                      key={latest.session_id}
                      className="hover:bg-primary/5 transition-colors"
                    >
                      <TableCell
                        className={cn(
                          "pl-4 border-l-2",
                          STATUS_ROW_BORDER[latest.status] ?? "border-l-transparent"
                        )}
                      >
                        <Link
                          to={`/sessions/detail?session_id=${encodeURIComponent(latest.session_id)}`}
                          aria-label={t("sessions.viewSession", { id: latest.session_id })}
                          className="font-mono text-sm font-medium text-foreground hover:text-primary transition-colors"
                        >
                          {latest.session_id.slice(0, 8)}
                        </Link>
                      </TableCell>

                      <TableCell>
                        {latest.worker_id ? (
                          <Link
                            to={`/workers/${latest.worker_id}`}
                            className="text-sm text-muted-foreground hover:text-foreground transition-colors"
                          >
                            {latest.worker_name || latest.worker_id.slice(0, 8)}
                          </Link>
                        ) : (
                          <span className="text-sm text-muted-foreground/50">—</span>
                        )}
                      </TableCell>

                      <TableCell>
                        <div className="flex flex-col gap-1.5">
                          <span className="text-xs font-mono text-muted-foreground">
                            {t("sessions.turnCount", { count: group.length })}
                          </span>
                          <TurnPips executions={group} />
                        </div>
                      </TableCell>

                      <TableCell>
                        <StatusBadge status={latest.status} />
                      </TableCell>

                      <TableCell
                        className="text-xs font-mono text-muted-foreground"
                        title={
                          oldest.started_at
                            ? new Date(oldest.started_at).toLocaleString()
                            : undefined
                        }
                      >
                        {formatRelative(oldest.started_at, t)}
                      </TableCell>

                      <TableCell className="text-xs font-mono">
                        {isActive ? (
                          <span className="text-status-working animate-pulse-amber">live</span>
                        ) : (
                          <span className="text-muted-foreground">{duration}</span>
                        )}
                      </TableCell>

                      <TableCell>
                        {tokenStats ? (
                          <div className="flex items-center gap-1 text-xs font-mono">
                            <span className="text-muted-foreground">{formatTokenCount(tokenStats.total_tokens)}</span>
                            <TokenStatsInfoButton stats={tokenStats} side="left" align="center" />
                          </div>
                        ) : (
                          <span className="text-xs text-muted-foreground/40">—</span>
                        )}
                      </TableCell>
                    </TableRow>
                  )
                })}
              </TableBody>
            </Table>
          </div>

          <PaginationControls page={page} totalPages={totalPages} onPageChange={setPage} />
        </>
      )}
    </FadeIn>
  )
}
