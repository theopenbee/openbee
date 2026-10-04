import { useMemo, useState } from "react"
import { Link } from "react-router-dom"
import { useTranslation } from "react-i18next"
import { useExecutions } from "@/hooks/use-executions"
import type { ExecutionStatus, WorkerExecution } from "@/lib/types"
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
import { ALERT_DESTRUCTIVE, SURFACE } from "@/lib/styles"
import { formatDuration, formatRelative, formatTokenCount, groupExecutionsBySession, isActiveStatus } from "@/lib/format"

const PAGE_SIZE = 20

const TURN_DOT: Record<ExecutionStatus, string> = {
  running: "bg-status-working",
  completed: "bg-status-idle",
  failed: "bg-status-error",
  pending: "bg-muted-foreground/40",
}

// One dot per turn, oldest first, so a failure earlier in the session stays
// visible even when the latest turn (the status column) succeeded.
function TurnPips({ executions }: { executions: WorkerExecution[] }) {
  const { t } = useTranslation()
  const ordered = [...executions].reverse()
  return (
    <div className="flex max-w-full flex-wrap items-center gap-0.5">
      {ordered.map((e, i) => {
        const label = t("sessions.turnStatus", { n: i + 1, status: t(`statuses.${e.status}`, e.status) })
        return (
          <span
            key={e.id}
            role="img"
            aria-label={label}
            title={label}
            className={cn("size-2 shrink-0 rounded-full", TURN_DOT[e.status] ?? "bg-muted-foreground/40")}
          />
        )
      })}
    </div>
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
          framed
          title={t("emptyState.noExecutions")}
          description={t("emptyState.noExecutionsDesc")}
        />
      ) : (
        <>
          <div className={SURFACE}>
            <Table className="md:min-w-[760px]">
              <TableHeader>
                <TableRow>
                  <TableHead className="w-32 pl-4">{t("sessions.columns.session")}</TableHead>
                  <TableHead>{t("sessions.columns.worker")}</TableHead>
                  <TableHead className="hidden w-28 md:table-cell">{t("sessions.columns.turns")}</TableHead>
                  <TableHead className="hidden w-32 md:table-cell">{t("sessions.columns.latestStatus")}</TableHead>
                  <TableHead className="w-32">{t("sessions.columns.started")}</TableHead>
                  <TableHead className="hidden w-28 md:table-cell">{t("sessions.columns.duration")}</TableHead>
                  <TableHead className="w-32 pr-4 text-right">{t("sessions.columns.tokens")}</TableHead>
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
                    <TableRow key={latest.session_id}>
                      <TableCell className="pl-4">
                        <Link
                          to={`/sessions/detail?session_id=${encodeURIComponent(latest.session_id)}`}
                          aria-label={t("sessions.viewSession", { id: latest.session_id })}
                          className="rounded-sm font-mono text-body-sm font-medium text-link underline-offset-4 outline-none hover:underline focus-visible:ring-2 focus-visible:ring-ring"
                        >
                          {latest.session_id.slice(0, 8)}
                        </Link>
                        {/* Below md the status column folds in here, so state
                            stays visible without scrolling. */}
                        <div className="mt-1.5 md:hidden">
                          <StatusBadge status={latest.status} />
                        </div>
                      </TableCell>

                      <TableCell>
                        {latest.worker_id ? (
                          <Link
                            to={`/workers/${latest.worker_id}`}
                            className="text-sm text-foreground underline-offset-4 transition-colors hover:text-link hover:underline"
                          >
                            {latest.worker_name || latest.worker_id.slice(0, 8)}
                          </Link>
                        ) : (
                          <span className="text-sm text-muted-foreground">—</span>
                        )}
                      </TableCell>

                      <TableCell className="hidden md:table-cell">
                        <div className="flex flex-col gap-1.5">
                          <span className="text-body-sm text-muted-foreground tabular-nums">
                            {t("sessions.turnCount", { count: group.length })}
                          </span>
                          <TurnPips executions={group} />
                        </div>
                      </TableCell>

                      <TableCell className="hidden md:table-cell">
                        <StatusBadge status={latest.status} />
                      </TableCell>

                      <TableCell
                        className="text-body-sm text-muted-foreground tabular-nums"
                        title={
                          oldest.started_at
                            ? new Date(oldest.started_at).toLocaleString()
                            : undefined
                        }
                      >
                        {formatRelative(oldest.started_at, t)}
                      </TableCell>

                      <TableCell className="hidden text-body-sm text-muted-foreground tabular-nums md:table-cell">
                        {isActive ? t("sessionDetail.live") : duration}
                      </TableCell>

                      <TableCell className="pr-4 text-right">
                        {tokenStats ? (
                          <div className="inline-flex items-center justify-end gap-1 text-body-sm text-foreground tabular-nums">
                            <span>{formatTokenCount(tokenStats.total_tokens)}</span>
                            <TokenStatsInfoButton stats={tokenStats} side="left" align="center" />
                          </div>
                        ) : (
                          <span className="text-body-sm text-muted-foreground">—</span>
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
