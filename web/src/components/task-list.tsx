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
import { cn } from "@/lib/utils"

export const TASK_PAGE_SIZE = 20

const STATUS_ROW_BORDER: Record<string, string> = {
  pending: "border-l-transparent",
  running: "border-l-status-working",
  completed: "border-l-status-idle",
  failed: "border-l-status-error",
  cancelled: "border-l-transparent",
}

interface TaskListProps {
  workerId?: string
  page?: number
  pageSize?: number
  onPageChange?: (page: number) => void
}

function TimeInfo({ task }: { task: Task }) {
  const { t } = useTranslation()
  if (task.type === "countdown" && task.scheduled_at) {
    return (
      <div className="space-y-1">
        <p className="text-xs font-medium text-muted-foreground">
          {t("tasks.triggerAt")}
        </p>
        <p className="font-mono text-xs text-foreground/80">
          {new Date(task.scheduled_at).toLocaleString()}
        </p>
      </div>
    )
  }
  if (task.type === "scheduled" && (task.next_run_at || task.cron_expr)) {
    return (
      <div className="space-y-1">
        {task.next_run_at && (
          <div className="space-y-1">
            <p className="text-xs font-medium text-muted-foreground">{t("tasks.nextRun")}</p>
            <p className="font-mono text-xs text-foreground/80">
              {new Date(task.next_run_at).toLocaleString()}
            </p>
          </div>
        )}
        {task.cron_expr && (
          <p className="font-mono text-xs text-muted-foreground">{task.cron_expr}</p>
        )}
      </div>
    )
  }
  return <span className="text-sm text-muted-foreground">—</span>
}

export function TaskList({
  workerId,
  page: controlledPage,
  pageSize = TASK_PAGE_SIZE,
  onPageChange,
}: TaskListProps) {
  const { t } = useTranslation()
  const [internalPage, setInternalPage] = useState(1)
  const page = controlledPage ?? internalPage
  const setPage = onPageChange ?? setInternalPage

  const { data, error, isLoading } = useTasks({ workerID: workerId, page, pageSize })
  const cancelTask = useCancelTask()
  const cancelAll = useCancelWorkerTasks()

  const tasks = data?.items ?? []
  const totalPages = Math.max(1, Math.ceil((data?.total ?? 0) / pageSize))

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
            <Table className="min-w-[920px]">
              <TableHeader>
                <TableRow className="bg-secondary/50 hover:bg-secondary/50">
                  <TableHead className="pl-5 w-28">{t("tasks.columns.type")}</TableHead>
                  {!workerId && <TableHead className="w-40">{t("tasks.columns.worker")}</TableHead>}
                  <TableHead className="min-w-[24rem]">{t("tasks.columns.instruction")}</TableHead>
                  <TableHead className="w-28">{t("tasks.columns.status")}</TableHead>
                  <TableHead className="w-56">{t("tasks.columns.timeInfo")}</TableHead>
                  <TableHead className="w-28 text-right">{t("tasks.columns.actions")}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {tasks.map((task) => (
                  <TableRow key={task.id} className="hover:bg-primary/5 transition-colors">
                    <TableCell
                      className={cn(
                        "pl-4 border-l-2",
                        STATUS_ROW_BORDER[task.status] ?? "border-l-transparent"
                      )}
                    >
                      <Badge variant={task.type === "scheduled" ? "secondary" : "outline"}>
                        {t(`tasks.types.${task.type}`)}
                      </Badge>
                    </TableCell>
                    {!workerId && (
                      <TableCell>
                        {task.worker_id ? (
                          <Link
                            to={`/workers/${task.worker_id}`}
                            className="text-sm text-muted-foreground hover:text-foreground transition-colors"
                          >
                            {task.worker_name || task.worker_id.slice(0, 8) + "..."}
                          </Link>
                        ) : (
                          <span className="text-muted-foreground">—</span>
                        )}
                      </TableCell>
                    )}
                    <TableCell className="max-w-[32rem] whitespace-normal">
                      <p
                        className="line-clamp-2 break-words text-sm leading-5 text-foreground"
                        title={task.instruction}
                      >
                        {task.instruction}
                      </p>
                    </TableCell>
                    <TableCell>
                      <StatusBadge status={task.status} />
                    </TableCell>
                    <TableCell>
                      <TimeInfo task={task} />
                    </TableCell>
                    <TableCell className="text-right">
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
                      {task.status !== "pending" && (
                        <span className="text-sm text-muted-foreground">—</span>
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
