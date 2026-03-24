import { useMemo, useState } from "react"
import { Link } from "react-router-dom"
import { useTranslation } from "react-i18next"
import { ChevronLeft, ChevronRight } from "lucide-react"
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
import { Button } from "@/components/ui/button"
import { StatusBadge } from "@/components/status-badge"
import { EmptyState } from "@/components/empty-state"
import { PageHeader } from "@/components/page-header"
import { FadeIn } from "@/components/fade-in"
import { SkeletonTable } from "@/components/skeleton-loader"

const PAGE_SIZE = 20

export function Executions() {
  const { t } = useTranslation()
  const [page, setPage] = useState(1)
  const { data, error, isLoading } = useExecutions(page, PAGE_SIZE)

  const executions = data?.items ?? []
  const totalPages = Math.max(1, Math.ceil((data?.total ?? 0) / PAGE_SIZE))

  const sessionGroups = useMemo(() => {
    const map = new Map<string, WorkerExecution[]>()
    for (const e of executions) {
      const group = map.get(e.session_id) ?? []
      group.push(e)
      map.set(e.session_id, group)
    }
    return Array.from(map.values()).sort((a, b) => {
      return (b[0].started_at ?? 0) - (a[0].started_at ?? 0)
    })
  }, [executions])

  return (
    <FadeIn>
      <PageHeader title={t("executions.title")} />

      {error && <p className="text-destructive mb-4">{error.message}</p>}

      {isLoading ? (
        <SkeletonTable />
      ) : sessionGroups.length === 0 && !error ? (
        <EmptyState
          title={t("emptyState.noExecutions")}
          description={t("emptyState.noExecutionsDesc")}
        />
      ) : (
        <>
          <div className="rounded-xl bg-card ring-1 ring-foreground/5 overflow-hidden">
            <Table>
              <TableHeader>
                <TableRow className="bg-secondary/50 hover:bg-secondary/50">
                  <TableHead>{t("executions.columns.session")}</TableHead>
                  <TableHead>{t("executions.columns.worker")}</TableHead>
                  <TableHead>{t("executions.columns.turns")}</TableHead>
                  <TableHead>{t("executions.columns.latestStatus")}</TableHead>
                  <TableHead>{t("executions.columns.started")}</TableHead>
                  <TableHead>{t("executions.columns.lastCompleted")}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {sessionGroups.map((group) => {
                  const latest = group[0]
                  const oldest = group[group.length - 1]
                  const lastCompleted = group.find((e) => e.completed_at)
                  return (
                    <TableRow key={latest.session_id} className="hover:bg-primary/5 transition-colors">
                      <TableCell>
                        <Link
                          to={`/sessions/${latest.session_id}`}
                          className="font-mono text-sm text-primary hover:underline"
                        >
                          {latest.session_id.slice(0, 8)}...
                        </Link>
                      </TableCell>
                      <TableCell>
                        {latest.worker_id ? (
                          <Link
                            to={`/workers/${latest.worker_id}`}
                            className="text-sm hover:text-primary transition-colors"
                          >
                            {latest.worker_name || latest.worker_id.slice(0, 8) + "..."}
                          </Link>
                        ) : (
                          <span className="text-sm text-muted-foreground">—</span>
                        )}
                      </TableCell>
                      <TableCell className="text-sm font-mono">{t("executions.turnCount", { count: group.length })}</TableCell>
                      <TableCell>
                        <StatusBadge status={latest.status} />
                      </TableCell>
                      <TableCell className="text-sm font-mono text-muted-foreground">
                        {oldest.started_at ? new Date(oldest.started_at).toLocaleString() : "-"}
                      </TableCell>
                      <TableCell className="text-sm font-mono text-muted-foreground">
                        {lastCompleted?.completed_at
                          ? new Date(lastCompleted.completed_at).toLocaleString()
                          : "-"}
                      </TableCell>
                    </TableRow>
                  )
                })}
              </TableBody>
            </Table>
          </div>

          {totalPages > 1 && (
            <div className="flex items-center justify-between mt-4">
              <span className="text-sm text-muted-foreground">
                {t("executions.pagination.page", { page, totalPages })}
              </span>
              <div className="flex gap-2">
                <Button
                  variant="outline"
                  size="sm"
                  disabled={page <= 1}
                  onClick={() => setPage((p) => Math.max(1, p - 1))}
                >
                  <ChevronLeft className="size-4" />
                  {t("executions.pagination.previous")}
                </Button>
                <Button
                  variant="outline"
                  size="sm"
                  disabled={page >= totalPages}
                  onClick={() => setPage((p) => Math.min(totalPages, p + 1))}
                >
                  {t("executions.pagination.next")}
                  <ChevronRight className="size-4" />
                </Button>
              </div>
            </div>
          )}
        </>
      )}
    </FadeIn>
  )
}
