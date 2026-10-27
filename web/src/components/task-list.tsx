import { useEffect, useState } from "react"
import { useNavigate } from "react-router-dom"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"
import { Button } from "@/components/arc/button/button"
import { EmptyState } from "@/components/empty-state"
import { ExpandableContent } from "@/components/expandable-content"
import { LoadErrorAlert } from "@/components/dashboard-load-error"
import { PaginationControls } from "@/components/pagination-controls"
import { StatusBadge } from "@/components/status-badge"
import { WorkerLink } from "@/components/worker-link"
import { CancelAllTasksDialog, CancelTaskDialog } from "@/components/task-cancel-dialogs"
import { formatRunAt, formatRunAtFull, formatRunIn, taskRunAt } from "@/components/task-time"
import { useTasks, useCancelTask, useCancelWorkerTasks } from "@/hooks/use-tasks"
import { useIsMobile } from "@/hooks/use-mobile"
import type { Task } from "@/lib/types"
import { cn } from "@/lib/utils"

export const TASK_PAGE_SIZE = 20

interface TaskListProps {
  workerId?: string
  page?: number
  pageSize?: number
  onPageChange?: (page: number) => void
}

const HEAD_CELL = "h-11 px-6 text-left align-middle text-xs font-normal whitespace-nowrap text-secondary"
const BODY_CELL = "px-6 py-4 align-top"
const CARD = "overflow-hidden rounded-surface border border-border bg-surface"
const WORKER_LINK = "text-foreground hover:text-accent-strong"

function useNow(intervalMs: number) {
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => {
    const id = window.setInterval(() => setNow(Date.now()), intervalMs)
    return () => window.clearInterval(id)
  }, [intervalMs])
  return now
}

function canCancel(task: Task) {
  return task.status === "pending"
}

function TaskSchedule({ task }: { task: Task }) {
  const { t } = useTranslation()
  const recurring = task.type === "scheduled"
  return (
    <div className="min-w-0">
      <p className="text-sm text-foreground">{recurring ? t("tasks.recurring") : t("tasks.oneTime")}</p>
      {recurring && task.cron_expr && (
        <p className="mt-0.5 truncate font-mono text-xs text-secondary" title={task.cron_expr}>
          {task.cron_expr}
        </p>
      )}
    </div>
  )
}

function TaskNextRun({ task, now }: { task: Task; now: number }) {
  const { t, i18n } = useTranslation()
  const ts = taskRunAt(task)
  if (!ts) return <p className="text-sm text-secondary">{t("tasks.notScheduled")}</p>
  return (
    <div className="min-w-0" title={formatRunAtFull(ts, i18n.language)}>
      <p className="text-sm text-foreground tabular">{formatRunIn(ts, i18n.language, now)}</p>
      <p className="mt-0.5 text-xs text-secondary tabular">{formatRunAt(ts, i18n.language, now)}</p>
    </div>
  )
}

function Instruction({ text, collapsedHeight }: { text: string; collapsedHeight: number }) {
  return (
    <ExpandableContent
      maxHeight={collapsedHeight}
      fadeClassName="h-5 from-surface group-hover/row:from-surface-muted"
      className="min-w-0"
    >
      <p className="text-sm leading-5 break-words whitespace-pre-wrap text-foreground">{text}</p>
    </ExpandableContent>
  )
}

interface RowsProps {
  tasks: Task[]
  showWorker: boolean
  now: number
  onCancel: (task: Task) => void
}

function TaskTable({ tasks, showWorker, now, onCancel }: RowsProps) {
  const { t } = useTranslation()
  return (
    <div className={CARD}>
      <div className="overflow-x-auto overscroll-x-contain">
        <table className={cn("w-full border-collapse text-sm", showWorker ? "min-w-[60rem]" : "min-w-[50rem]")}>
          <caption className="sr-only">{t("tasks.title")}</caption>
          <thead>
            <tr>
              {showWorker && <th scope="col" className={cn(HEAD_CELL, "w-36")}>{t("tasks.columns.worker")}</th>}
              <th scope="col" className={HEAD_CELL}>{t("tasks.columns.instruction")}</th>
              <th scope="col" className={cn(HEAD_CELL, "w-32")}>{t("tasks.columns.status")}</th>
              <th scope="col" className={cn(HEAD_CELL, "w-40")}>{t("tasks.columns.schedule")}</th>
              <th scope="col" className={cn(HEAD_CELL, "w-44")}>{t("tasks.columns.nextRunAt")}</th>
              <th scope="col" className={cn(HEAD_CELL, "w-24")}>
                <span className="sr-only">{t("tasks.columns.actions")}</span>
              </th>
            </tr>
          </thead>
          <tbody>
            {tasks.map((task) => (
              <tr key={task.id} className="group/row border-t border-border-subtle transition-colors hover:bg-surface-muted">
                {showWorker && (
                  <td className={BODY_CELL}>
                    {task.worker_id ? (
                      <WorkerLink id={task.worker_id} name={task.worker_name} className={cn("block truncate text-sm", WORKER_LINK)} />
                    ) : null}
                  </td>
                )}
                <td className={BODY_CELL}>
                  <Instruction text={task.instruction} collapsedHeight={40} />
                </td>
                <td className={BODY_CELL}>
                  <StatusBadge status={task.status} />
                </td>
                <td className={BODY_CELL}>
                  <TaskSchedule task={task} />
                </td>
                <td className={BODY_CELL}>
                  <TaskNextRun task={task} now={now} />
                </td>
                <td className="px-4 py-2.5 text-right align-top">
                  {canCancel(task) && (
                    <Button variant="ghost" size="sm" onClick={() => onCancel(task)}>
                      {t("tasks.cancel")}
                    </Button>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  )
}

function TaskCards({ tasks, showWorker, now, onCancel }: RowsProps) {
  const { t, i18n } = useTranslation()
  return (
    <ul className={cn(CARD, "divide-y divide-border-subtle")}>
      {tasks.map((task) => {
        const ts = taskRunAt(task)
        const recurring = task.type === "scheduled"
        return (
          <li key={task.id} className="grid gap-3 px-5 py-4">
            <div className="flex items-start justify-between gap-3">
              <div className="min-w-0 flex-1">
                <Instruction text={task.instruction} collapsedHeight={60} />
              </div>
              <StatusBadge status={task.status} />
            </div>
            <div className="flex items-end justify-between gap-3">
              <dl className="grid min-w-0 flex-1 grid-cols-[max-content_minmax(0,1fr)] gap-x-3 gap-y-1 text-xs">
                {showWorker && task.worker_id && (
                  <div className="contents">
                    <dt className="shrink-0 text-secondary">{t("tasks.columns.worker")}</dt>
                    <dd className="min-w-0 truncate">
                      <WorkerLink id={task.worker_id} name={task.worker_name} className={WORKER_LINK} />
                    </dd>
                  </div>
                )}
                <div className="contents">
                  <dt className="shrink-0 text-secondary">{t("tasks.columns.schedule")}</dt>
                  <dd className="min-w-0 break-words text-foreground">
                    <span className="mr-2">{recurring ? t("tasks.recurring") : t("tasks.oneTime")}</span>
                    {recurring && task.cron_expr && (
                      <span className="inline-block font-mono text-secondary">{task.cron_expr}</span>
                    )}
                  </dd>
                </div>
                <div className="contents">
                  <dt className="shrink-0 text-secondary">{t("tasks.columns.nextRunAt")}</dt>
                  <dd className="min-w-0 break-words text-foreground tabular">
                    {ts ? (
                      <>
                        <span className="mr-2">{formatRunIn(ts, i18n.language, now)}</span>
                        <span className="inline-block text-secondary">{formatRunAt(ts, i18n.language, now)}</span>
                      </>
                    ) : (
                      <span className="text-secondary">{t("tasks.notScheduled")}</span>
                    )}
                  </dd>
                </div>
              </dl>
              {canCancel(task) && (
                <Button variant="secondary" size="md" className="shrink-0" onClick={() => onCancel(task)}>
                  {t("tasks.cancel")}
                </Button>
              )}
            </div>
          </li>
        )
      })}
    </ul>
  )
}

function TaskListSkeleton({ mobile, showWorker }: { mobile: boolean; showWorker: boolean }) {
  const rows = Array.from({ length: 5 }, (_, i) => i)
  if (mobile) {
    return (
      <ul className={cn(CARD, "divide-y divide-border-subtle")} aria-hidden="true">
        {rows.map((i) => (
          <li key={i} className="grid gap-3 px-5 py-4">
            <div className="flex items-start justify-between gap-3">
              <div className="grid flex-1 gap-2">
                <div className="skeleton h-4 w-full" />
                <div className="skeleton h-4 w-2/3" />
              </div>
              <div className="skeleton h-6 w-16 rounded-full" />
            </div>
            <div className="grid gap-1.5">
              <div className="skeleton h-3 w-32" />
              <div className="skeleton h-3 w-40" />
            </div>
          </li>
        ))}
      </ul>
    )
  }
  return (
    <div className={CARD} aria-hidden="true">
      <div className="flex h-11 items-center gap-6 px-6">
        {showWorker && <div className="skeleton h-3 w-20" />}
        <div className="skeleton h-3 w-24" />
      </div>
      {rows.map((i) => (
        <div key={i} className="flex items-start gap-6 border-t border-border-subtle px-6 py-4">
          {showWorker && <div className="skeleton h-4 w-32 shrink-0" />}
          <div className="grid min-w-0 flex-1 gap-2">
            <div className="skeleton h-4 w-full max-w-md" />
            <div className="skeleton h-4 w-1/2 max-w-xs" />
          </div>
          <div className="skeleton h-6 w-16 shrink-0 rounded-full" />
          <div className="skeleton h-4 w-28 shrink-0" />
          <div className="skeleton h-4 w-32 shrink-0" />
          <div className="w-16 shrink-0" />
        </div>
      ))}
    </div>
  )
}

export function TaskList({
  workerId,
  page: controlledPage,
  pageSize = TASK_PAGE_SIZE,
  onPageChange,
}: TaskListProps) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const isMobile = useIsMobile()
  const now = useNow(60_000)
  const [internalPage, setInternalPage] = useState(1)
  const page = controlledPage ?? internalPage
  const setPage = onPageChange ?? setInternalPage

  const { data, error, isLoading, refetch, isRefetching } = useTasks({ workerID: workerId, page, pageSize })
  const cancelTask = useCancelTask()
  const cancelAll = useCancelWorkerTasks()
  const [cancelTarget, setCancelTarget] = useState<Task | null>(null)
  const [cancelOpen, setCancelOpen] = useState(false)
  const [cancelAllOpen, setCancelAllOpen] = useState(false)

  const tasks = data?.items ?? []
  const total = data?.total ?? 0
  const totalPages = Math.max(1, Math.ceil(total / pageSize))
  const showWorker = !workerId

  useEffect(() => {
    if (data && page > totalPages) setPage(totalPages)
  }, [data, page, totalPages, setPage])

  const openCancel = (task: Task) => {
    cancelTask.reset()
    setCancelTarget(task)
    setCancelOpen(true)
  }

  const confirmCancel = () => {
    if (!cancelTarget) return
    cancelTask.mutate(cancelTarget.id, {
      onSuccess: () => {
        setCancelOpen(false)
        toast.success(t("tasks.cancelled"))
      },
    })
  }

  const openCancelAll = () => {
    cancelAll.reset()
    setCancelAllOpen(true)
  }

  const confirmCancelAll = () => {
    if (!workerId) return
    cancelAll.mutate(workerId, {
      onSuccess: () => {
        setCancelAllOpen(false)
        toast.success(t("tasks.allCancelled"))
      },
    })
  }

  const body = () => {
    if (isLoading) return <TaskListSkeleton mobile={isMobile} showWorker={showWorker} />
    if (error && !data) {
      return (
        <LoadErrorAlert
          title={t("tasks.loadError")}
          error={error}
          retrying={isRefetching}
          onRetry={() => refetch()}
        />
      )
    }
    if (tasks.length === 0) {
      return (
        <div className={CARD}>
          <EmptyState
            title={t("emptyState.noTasks")}
            description={t("emptyState.noTasksDesc")}
            action={
              <Button variant="secondary" onClick={() => navigate("/chat")}>
                {t("tasks.openChat")}
              </Button>
            }
          />
        </div>
      )
    }
    const rows = { tasks, showWorker, now, onCancel: openCancel }
    return (
      <div>
        {isMobile ? <TaskCards {...rows} /> : <TaskTable {...rows} />}
        <PaginationControls
          page={page}
          totalPages={totalPages}
          onPageChange={setPage}
          leadingLabel={t("tasks.summary", { count: total })}
        />
      </div>
    )
  }

  return (
    <div className="grid min-w-0 grid-cols-[minmax(0,1fr)] gap-4" aria-busy={isLoading}>
      {workerId && tasks.length > 0 && (
        <div className="flex justify-end">
          <Button variant="secondary" size="sm" onClick={openCancelAll}>
            {t("tasks.cancelAll")}
          </Button>
        </div>
      )}

      {error && data && (
        <LoadErrorAlert
          title={t("tasks.refreshError")}
          error={error}
          retrying={isRefetching}
          onRetry={() => refetch()}
        />
      )}

      {body()}

      <CancelTaskDialog
        open={cancelOpen}
        task={cancelTarget}
        pending={cancelTask.isPending}
        error={cancelTask.error}
        onOpenChange={setCancelOpen}
        onConfirm={confirmCancel}
      />

      {workerId && (
        <CancelAllTasksDialog
          open={cancelAllOpen}
          workerName={tasks[0]?.worker_name || workerId.slice(0, 8)}
          count={total}
          pending={cancelAll.isPending}
          error={cancelAll.error}
          onOpenChange={setCancelAllOpen}
          onConfirm={confirmCancelAll}
        />
      )}
    </div>
  )
}
