import { useMemo, useState } from "react"
import { Link } from "react-router-dom"
import { useTranslation } from "react-i18next"
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
import { cn } from "@/lib/utils"
import { formatDuration, formatRelative, groupExecutionsBySession, isActiveStatus, STATUS_ROW_BORDER } from "@/lib/format"

const PAGE_SIZE = 20

const TURN_DOT: Record<string, string> = {
  running: "bg-status-working",
  completed: "bg-status-idle",
  failed: "bg-status-error",
  pending: "bg-muted-foreground/30",
}

function TurnPips({ executions }: { executions: WorkerExecution[] }) {
  const ordered = [...executions].reverse()
  return (
    <div className="flex items-center gap-0.5 flex-wrap max-w-[120px]">
      {ordered.map((e, i) => (
        <div
          key={e.id}
          title={`Turn ${i + 1}: ${e.status}`}
          className={cn(
            "size-2 rounded-full shrink-0",
            TURN_DOT[e.status] ?? "bg-muted-foreground/30"
          )}
        />
      ))}
    </div>
  )
}

export function Executions() {
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
        ? t("executions.summaryWithActive", { count: totalSessions, active: activeCount })
        : t("executions.summary", { count: totalSessions })
      : undefined

  return (
    <FadeIn>
      <PageHeader title={t("executions.title")} subtitle={subtitle} />

      {error && (
        <div role="alert" className="mb-4 rounded-2xl border border-destructive/20 bg-destructive/10 px-4 py-3 text-sm text-destructive">
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
          <div className="rounded-2xl border border-border/70 bg-card overflow-hidden">
            <Table>
              <TableHeader>
                <TableRow className="bg-secondary/50 hover:bg-secondary/50">
                  <TableHead className="pl-5 w-28">{t("executions.columns.session")}</TableHead>
                  <TableHead className="w-36">{t("executions.columns.worker")}</TableHead>
                  <TableHead>{t("executions.columns.turns")}</TableHead>
                  <TableHead className="w-28">{t("executions.columns.latestStatus")}</TableHead>
                  <TableHead className="w-24">{t("executions.columns.started")}</TableHead>
                  <TableHead className="w-20">{t("executions.columns.duration")}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {sessionGroups.map((group) => {
                  const latest = group[0]
                  const oldest = group[group.length - 1]
                  const lastCompleted = group.find((e) => e.completed_at)
                  const isActive = latest.status === "running" || latest.status === "pending"
                  const duration = formatDuration(oldest.started_at, lastCompleted?.completed_at ?? null)

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
                          aria-label={t("executions.viewSession", { id: latest.session_id })}
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
                            {t("executions.turnCount", { count: group.length })}
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
                        {formatRelative(oldest.started_at)}
                      </TableCell>

                      <TableCell className="text-xs font-mono">
                        {isActive ? (
                          <span className="text-status-working animate-pulse-amber">live</span>
                        ) : (
                          <span className="text-muted-foreground">{duration}</span>
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
