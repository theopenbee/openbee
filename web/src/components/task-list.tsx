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
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from "@/components/ui/dialog"
import { EmptyState } from "@/components/empty-state"
import { SkeletonTable } from "@/components/skeleton-loader"
import { PaginationControls } from "@/components/pagination-controls"
import type { Task } from "@/lib/types"
import { cn } from "@/lib/utils"
import { STATUS_ROW_BORDER } from "@/lib/format"

export const TASK_PAGE_SIZE = 20

interface TaskListProps {
  workerId?: string
  page?: number
  pageSize?: number
  onPageChange?: (page: number) => void
}

function CronCell({ task }: { task: Task }) {
  if (task.type === "scheduled" && task.cron_expr) {
    return <p className="font-mono text-xs text-foreground/80">{task.cron_expr}</p>
  }
  return <span className="text-sm text-muted-foreground">—</span>
}

function NextRunCell({ task }: { task: Task }) {
  const timestamp = task.type === "countdown" ? task.scheduled_at : task.next_run_at
  if (timestamp) {
    return (
      <p className="font-mono text-xs text-foreground/80">
        {new Date(timestamp).toLocaleString()}
      </p>
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
  const [confirmCancelId, setConfirmCancelId] = useState<string | null>(null)
  const [confirmCancelAll, setConfirmCancelAll] = useState(false)

  const tasks = data?.items ?? []
  const totalPages = Math.max(1, Math.ceil((data?.total ?? 0) / pageSize))

  const mutationError = cancelTask.error || cancelAll.error

  return (
    <div>
      {workerId && !isLoading && tasks.length > 0 && (
        <div className="flex justify-end mb-4">
          <Button
            variant="outline"
            size="sm"
            onClick={() => setConfirmCancelAll(true)}
            disabled={cancelAll.isPending}
          >
            {t("tasks.cancelAll")}
          </Button>
        </div>
      )}

      {(error || mutationError) && (
        <div role="alert" className="mb-4 rounded-sm border border-destructive/20 bg-destructive/10 px-4 py-3 text-sm text-destructive">
          {(error || mutationError)?.message}
        </div>
      )}

      {isLoading ? (
        <SkeletonTable />
      ) : tasks.length === 0 && !error ? (
        <EmptyState title={t("emptyState.noTasks")} />
      ) : (
        <>
          <div className="rounded-sm border border-border/70 bg-card overflow-hidden">
            <Table className="min-w-[920px]">
              <TableHeader>
                <TableRow className="bg-secondary/50 hover:bg-secondary/50">
                  {!workerId && <TableHead className="pl-5 w-40">{t("tasks.columns.worker")}</TableHead>}
                  <TableHead className={cn("min-w-[24rem]", workerId && "pl-5")}>{t("tasks.columns.instruction")}</TableHead>
                  <TableHead className="w-44">{t("tasks.columns.cron")}</TableHead>
                  <TableHead className="w-48">{t("tasks.columns.nextRunAt")}</TableHead>
                  <TableHead className="w-28 text-right">{t("tasks.columns.actions")}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {tasks.map((task) => {
                  const borderClass = cn(
                    "pl-4 border-l-2",
                    STATUS_ROW_BORDER[task.status] ?? "border-l-transparent"
                  )
                  return (
                  <TableRow key={task.id} className="hover:bg-primary/5 transition-colors">
                    {!workerId && (
                      <TableCell className={borderClass}>
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
                    <TableCell className={cn("max-w-[32rem] whitespace-normal", workerId && borderClass)}>
                      <p
                        className="line-clamp-2 break-words text-sm leading-5 text-foreground"
                        title={task.instruction}
                      >
                        {task.instruction}
                      </p>
                    </TableCell>
                    <TableCell>
                      <CronCell task={task} />
                    </TableCell>
                    <TableCell>
                      <NextRunCell task={task} />
                    </TableCell>
                    <TableCell className="text-right">
                      {task.status === "pending" && (
                        <Button
                          variant="ghost"
                          size="sm"
                          onClick={() => setConfirmCancelId(task.id)}
                          disabled={cancelTask.isPending}
                          className="text-destructive hover:text-destructive"
                        >
                          {t("tasks.cancel")}
                        </Button>
                      )}
                      {task.status !== "pending" && (
                        <span className="text-sm text-muted-foreground">—</span>
                      )}
                    </TableCell>
                  </TableRow>
                  )
                })}
              </TableBody>
            </Table>
          </div>
          <PaginationControls
            page={page}
            totalPages={totalPages}
            onPageChange={setPage}
            leadingLabel={t("tasks.summary", { count: data?.total ?? 0 })}
          />
        </>
      )}

      <Dialog open={confirmCancelId !== null} onOpenChange={(open) => { if (!open) setConfirmCancelId(null) }}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("tasks.cancelConfirmTitle")}</DialogTitle>
            <DialogDescription>{t("tasks.cancelConfirmDescription")}</DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setConfirmCancelId(null)}>
              {t("common.cancel")}
            </Button>
            <Button
              variant="destructive"
              onClick={() => {
                cancelTask.mutate(confirmCancelId!)
                setConfirmCancelId(null)
              }}
            >
              {t("tasks.cancelTask")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={confirmCancelAll} onOpenChange={setConfirmCancelAll}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("tasks.cancelAllConfirmTitle")}</DialogTitle>
            <DialogDescription>{t("tasks.cancelAllConfirmDescription")}</DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setConfirmCancelAll(false)}>
              {t("common.cancel")}
            </Button>
            <Button
              variant="destructive"
              onClick={() => {
                if (workerId) cancelAll.mutate(workerId)
                setConfirmCancelAll(false)
              }}
            >
              {t("tasks.cancelAll")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
