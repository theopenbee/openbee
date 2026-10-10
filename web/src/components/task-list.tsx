import { useState, type ComponentProps } from "react"
import { useTranslation } from "react-i18next"
import { CalendarClockIcon, RepeatIcon } from "lucide-react"
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
import { ExpandableContent } from "@/components/expandable-content"
import { SkeletonTable } from "@/components/skeleton-loader"
import { PaginationControls } from "@/components/pagination-controls"
import { StatusBadge } from "@/components/status-badge"
import { WorkerLink } from "@/components/worker-link"
import type { Task } from "@/lib/types"
import { useCan } from "@/hooks/use-can"
import { useIsMobile } from "@/hooks/use-mobile"
import { Perm } from "@/lib/permissions"
import { cn } from "@/lib/utils"
import { ALERT_DESTRUCTIVE } from "@/lib/styles"
import { formatTimestamp, STATUS_ROW_BORDER } from "@/lib/format"

export const TASK_PAGE_SIZE = 20

interface TaskListProps {
  workerId?: string
  page?: number
  pageSize?: number
  onPageChange?: (page: number) => void
}

function cronLabel(task: Task) {
  return task.type === "scheduled" && task.cron_expr ? task.cron_expr : undefined
}

function nextRunLabel(task: Task) {
  const timestamp = task.type === "countdown" ? task.scheduled_at : task.next_run_at
  return timestamp ? formatTimestamp(timestamp) : undefined
}

const INSTRUCTION_COLLAPSED_HEIGHT = 60

function CancelTaskButton({ className, ...props }: ComponentProps<typeof Button>) {
  const { t } = useTranslation()
  return (
    <Button variant="ghost" className={cn("text-destructive hover:text-destructive", className)} {...props}>
      {t("tasks.cancel")}
    </Button>
  )
}

function ScheduleCell({ value }: { value?: string }) {
  if (value) {
    return <p className="font-mono text-xs text-foreground/80">{value}</p>
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
  const canWrite = useCan(Perm.TasksWrite)
  const isMobile = useIsMobile()
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
  const canCancel = (task: Task) => task.status === "pending" && canWrite

  return (
    <div>
      {workerId && canWrite && !isLoading && tasks.length > 0 && (
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
        <div role="alert" className={cn(ALERT_DESTRUCTIVE, "mb-4")}>
          {(error || mutationError)?.message}
        </div>
      )}

      {isLoading ? (
        <SkeletonTable />
      ) : tasks.length === 0 && !error ? (
        <EmptyState title={t("emptyState.noTasks")} />
      ) : (
        <>
          {isMobile ? (
            <ul className="divide-y divide-border/70 overflow-hidden rounded-sm border border-border/70 bg-card">
              {tasks.map((task) => {
                const cron = cronLabel(task)
                const nextRun = nextRunLabel(task)
                return (
                  <li key={task.id} className="space-y-2 px-4 py-3">
                    <div className="flex items-start justify-between gap-3">
                      <ExpandableContent
                        maxHeight={INSTRUCTION_COLLAPSED_HEIGHT}
                        fadeClassName="h-5 from-card"
                        className="min-w-0"
                      >
                        <p className="break-words text-sm leading-5 text-foreground">{task.instruction}</p>
                      </ExpandableContent>
                      <StatusBadge status={task.status} />
                    </div>
                    <div className="flex items-center gap-3">
                      <div className="flex min-w-0 flex-1 flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground">
                        {!workerId && task.worker_id && (
                          <WorkerLink
                            id={task.worker_id}
                            name={task.worker_name}
                            className="font-medium text-foreground/80"
                          />
                        )}
                        {cron && (
                          <span className="inline-flex items-center gap-1 font-mono">
                            <RepeatIcon className="size-3.5 shrink-0" aria-label={t("tasks.columns.cron")} />
                            {cron}
                          </span>
                        )}
                        {nextRun && (
                          <span className="inline-flex items-center gap-1 font-mono">
                            <CalendarClockIcon className="size-3.5 shrink-0" aria-label={t("tasks.columns.nextRunAt")} />
                            {nextRun}
                          </span>
                        )}
                      </div>
                      {canCancel(task) && (
                        <CancelTaskButton
                          size="lg"
                          onClick={() => setConfirmCancelId(task.id)}
                          disabled={cancelTask.isPending}
                          className="-mr-2"
                        />
                      )}
                    </div>
                  </li>
                )
              })}
            </ul>
          ) : (
            <div className="overflow-hidden rounded-sm border border-border/70 bg-card">
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
                            <WorkerLink
                              id={task.worker_id}
                              name={task.worker_name}
                              className="text-sm text-muted-foreground"
                            />
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
                        <ScheduleCell value={cronLabel(task)} />
                      </TableCell>
                      <TableCell>
                        <ScheduleCell value={nextRunLabel(task)} />
                      </TableCell>
                      <TableCell className="text-right">
                        {canCancel(task) ? (
                          <CancelTaskButton
                            size="sm"
                            onClick={() => setConfirmCancelId(task.id)}
                            disabled={cancelTask.isPending}
                          />
                        ) : (
                          <span className="text-sm text-muted-foreground">—</span>
                        )}
                      </TableCell>
                    </TableRow>
                    )
                  })}
                </TableBody>
              </Table>
            </div>
          )}
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
