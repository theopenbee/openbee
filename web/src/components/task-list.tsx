import { useState } from "react"
import { Link } from "react-router-dom"
import { useTranslation } from "react-i18next"
import { useTasks, useCancelTask, useCancelWorkerTasks } from "@/hooks/use-tasks"
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
import { SkeletonTable } from "@/components/skeleton-loader"
import { PaginationControls } from "@/components/pagination-controls"
import { Badge } from "@/components/ui/badge"
import type { Task } from "@/lib/types"

const PAGE_SIZE = 20

interface TaskListProps {
  workerId?: string
}

function TimeInfo({ task }: { task: Task }) {
  const { t } = useTranslation()
  if (task.type === "countdown" && task.scheduled_at) {
    return (
      <div className="text-sm text-muted-foreground">
        <span className="text-xs font-medium text-foreground">{t("tasks.triggerAt")}</span>
        <br />
        {new Date(task.scheduled_at).toLocaleString()}
      </div>
    )
  }
  if (task.type === "scheduled" && (task.next_run_at || task.cron_expr)) {
    return (
      <div className="text-sm text-muted-foreground">
        {task.next_run_at && (
          <>
            <span className="text-xs font-medium text-foreground">{t("tasks.nextRun")}</span>
            <br />
            {new Date(task.next_run_at).toLocaleString()}
          </>
        )}
        {task.cron_expr && (
          <div className="font-mono text-xs mt-1">{task.cron_expr}</div>
        )}
      </div>
    )
  }
  return <span className="text-muted-foreground">—</span>
}

export function TaskList({ workerId }: TaskListProps) {
  const { t } = useTranslation()
  const [page, setPage] = useState(1)
  const { data, error, isLoading } = useTasks({ workerID: workerId, page, pageSize: PAGE_SIZE })
  const cancelTask = useCancelTask()
  const cancelAll = useCancelWorkerTasks()

  const tasks = data?.items ?? []
  const totalPages = Math.max(1, Math.ceil((data?.total ?? 0) / PAGE_SIZE))

  const mutationError = cancelTask.error || cancelAll.error

  return (
    <div>
      {workerId && (
        <div className="flex justify-end mb-4">
          <Button
            variant="outline"
            size="sm"
            onClick={() => cancelAll.mutate(workerId)}
            disabled={cancelAll.isPending}
          >
            {t("tasks.cancelAll")}
          </Button>
        </div>
      )}

      {(error || mutationError) && (
        <p className="text-destructive mb-4">{(error || mutationError)?.message}</p>
      )}

      {isLoading ? (
        <SkeletonTable />
      ) : tasks.length === 0 && !error ? (
        <EmptyState
          title={t("emptyState.noTasks")}
          description={t("emptyState.noTasksDesc")}
        />
      ) : (
        <>
          <div className="rounded-xl bg-card ring-1 ring-foreground/5 overflow-hidden">
            <Table>
              <TableHeader>
                <TableRow className="bg-secondary/50 hover:bg-secondary/50">
                  <TableHead>{t("tasks.columns.type")}</TableHead>
                  {!workerId && <TableHead>{t("tasks.columns.worker")}</TableHead>}
                  <TableHead>{t("tasks.columns.instruction")}</TableHead>
                  <TableHead>{t("tasks.columns.status")}</TableHead>
                  <TableHead>{t("tasks.columns.timeInfo")}</TableHead>
                  <TableHead>{t("tasks.columns.actions")}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {tasks.map((task) => (
                  <TableRow key={task.id} className="hover:bg-primary/5 transition-colors">
                    <TableCell>
                      <Badge variant={task.type === "scheduled" ? "secondary" : "outline"}>
                        {t(`tasks.types.${task.type}`)}
                      </Badge>
                    </TableCell>
                    {!workerId && (
                      <TableCell>
                        {task.worker_id ? (
                          <Link
                            to={`/workers/${task.worker_id}`}
                            className="text-sm hover:text-primary transition-colors"
                          >
                            {task.worker_name || task.worker_id.slice(0, 8) + "..."}
                          </Link>
                        ) : (
                          <span className="text-muted-foreground">—</span>
                        )}
                      </TableCell>
                    )}
                    <TableCell className="max-w-xs">
                      <p className="text-sm truncate" title={task.instruction}>
                        {task.instruction}
                      </p>
                    </TableCell>
                    <TableCell>
                      <StatusBadge status={task.status} />
                    </TableCell>
                    <TableCell>
                      <TimeInfo task={task} />
                    </TableCell>
                    <TableCell>
                      {task.status === "pending" && (
                        <Button
                          variant="ghost"
                          size="sm"
                          onClick={() => cancelTask.mutate(task.id)}
                          disabled={cancelTask.isPending}
                          className="text-destructive hover:text-destructive"
                        >
                          {t("tasks.cancelTask")}
                        </Button>
                      )}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
          <PaginationControls page={page} totalPages={totalPages} onPageChange={setPage} />
        </>
      )}
    </div>
  )
}
