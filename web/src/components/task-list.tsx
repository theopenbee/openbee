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
import { useCan } from "@/hooks/use-can"
import { Perm } from "@/lib/permissions"
import { StatusBadge } from "@/components/status-badge"
import { cn } from "@/lib/utils"
import { ALERT_DESTRUCTIVE, SURFACE } from "@/lib/styles"

export const TASK_PAGE_SIZE = 20

interface TaskListProps {
  workerId?: string
  page?: number
  pageSize?: number
  onPageChange?: (page: number) => void
}

function CronCell({ task }: { task: Task }) {
  if (task.type === "scheduled" && task.cron_expr) {
    return <span className="font-mono text-body-sm text-foreground">{task.cron_expr}</span>
  }
  return <span className="text-muted-foreground">—</span>
}

function NextRunCell({ task }: { task: Task }) {
  const timestamp = task.type === "countdown" ? task.scheduled_at : task.next_run_at
  if (timestamp) {
    return (
      <span className="text-body-sm text-foreground tabular-nums">
        {new Date(timestamp).toLocaleString()}
      </span>
    )
  }
  return <span className="text-muted-foreground">—</span>
}

export function TaskList({
  workerId,
  page: controlledPage,
  pageSize = TASK_PAGE_SIZE,
  onPageChange,
}: TaskListProps) {
  const { t } = useTranslation()
  const canWrite = useCan(Perm.TasksWrite)
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
      {workerId && canWrite && !isLoading && tasks.length > 0 && (
        <div className="mb-3 flex justify-end">
          <Button
            variant="outline"
            size="sm"
            onClick={() => setConfirmCancelAll(true)}
            disabled={cancelAll.isPending}
            className="text-destructive-foreground hover:text-destructive-foreground"
          >
            {t("tasks.cancelAll")}
          </Button>
        </div>
      )}

      {(error || mutationError) && (
        <div role="alert" className={cn(ALERT_DESTRUCTIVE, "mb-4")}>
          {(error || mutationError)?.message}
        </div>
      )}

      {isLoading ? (
        <SkeletonTable />
      ) : tasks.length === 0 && !error ? (
        <EmptyState framed title={t("emptyState.noTasks")} />
      ) : (
        <>
          <div className={SURFACE}>
            <Table className="md:min-w-[960px]">
              <TableHeader>
                <TableRow>
                  {!workerId && <TableHead className="pl-4 md:w-40">{t("tasks.columns.worker")}</TableHead>}
                  <TableHead className={cn("md:min-w-[20rem]", workerId && "pl-4")}>{t("tasks.columns.instruction")}</TableHead>
                  <TableHead className="hidden w-28 md:table-cell">{t("tasks.columns.status")}</TableHead>
                  <TableHead className="hidden w-40 md:table-cell">{t("tasks.columns.cron")}</TableHead>
                  <TableHead className="hidden w-48 md:table-cell">{t("tasks.columns.nextRunAt")}</TableHead>
                  <TableHead className="pr-4 text-right md:w-24">{t("tasks.columns.actions")}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {tasks.map((task) => (
                  <TableRow key={task.id}>
                    {!workerId && (
                      <TableCell className="pl-4">
                        {task.worker_id ? (
                          <Link
                            to={`/workers/${task.worker_id}`}
                            className="text-sm text-foreground underline-offset-4 transition-colors hover:text-link hover:underline"
                          >
                            {task.worker_name || task.worker_id.slice(0, 8) + "..."}
                          </Link>
                        ) : (
                          <span className="text-muted-foreground">—</span>
                        )}
                      </TableCell>
                    )}
                    <TableCell className={cn("max-w-[32rem] whitespace-normal", workerId && "pl-4")}>
                      <p
                        className="line-clamp-2 text-sm leading-5 break-words text-foreground"
                        title={task.instruction}
                      >
                        {task.instruction}
                      </p>
                      {/* Below md the status and next-run columns fold in
                          under the instruction so state stays in view. */}
                      <div className="mt-1.5 flex flex-wrap items-center gap-x-2 gap-y-1 md:hidden">
                        <StatusBadge status={task.status} />
                        <span className="text-body-sm">
                          <span className="text-muted-foreground">{t("tasks.columns.nextRunAt")} </span>
                          <NextRunCell task={task} />
                        </span>
                      </div>
                    </TableCell>
                    <TableCell className="hidden md:table-cell">
                      <StatusBadge status={task.status} />
                    </TableCell>
                    <TableCell className="hidden md:table-cell">
                      <CronCell task={task} />
                    </TableCell>
                    <TableCell className="hidden md:table-cell">
                      <NextRunCell task={task} />
                    </TableCell>
                    <TableCell className="pr-4 text-right">
                      {task.status === "pending" && canWrite ? (
                        <Button
                          variant="ghost"
                          size="sm"
                          onClick={() => setConfirmCancelId(task.id)}
                          disabled={cancelTask.isPending}
                          className="text-destructive-foreground hover:bg-danger-tint hover:text-destructive-foreground"
                        >
                          {t("tasks.cancel")}
                        </Button>
                      ) : (
                        <span className="text-muted-foreground">—</span>
                      )}
                    </TableCell>
                  </TableRow>
                ))}
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
