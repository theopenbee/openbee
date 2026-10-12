import { useState, type ComponentProps, type FormEvent } from "react"
import { useTranslation } from "react-i18next"
import type { TFunction } from "i18next"
import { CalendarClockIcon, PlusIcon, RepeatIcon } from "lucide-react"
import { useCreateTask, useTaskTargets, useUpdateTask } from "@/hooks/use-tasks"
import { useWorkers } from "@/hooks/use-workers"
import { useCan } from "@/hooks/use-can"
import { Perm } from "@/lib/permissions"
import { ALERT_DESTRUCTIVE } from "@/lib/styles"
import { cn, getErrorMessage } from "@/lib/utils"
import { CRON_PRESETS, initialTaskFormValues, toDateTimeLocalValue, toTaskInput, type TaskFormValues } from "@/lib/task-form"
import type { EditableTaskType, Task, TaskTarget } from "@/lib/types"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"

const LOCAL_PLATFORM = "local"

const TYPE_OPTIONS: { value: EditableTaskType; icon: typeof RepeatIcon }[] = [
  { value: "scheduled", icon: RepeatIcon },
  { value: "countdown", icon: CalendarClockIcon },
]

function targetLabel(target: TaskTarget, t: TFunction) {
  if (target.platform === LOCAL_PLATFORM) return t("tasks.form.webChat")
  return t(`tasks.platforms.${target.platform}`, { defaultValue: target.platform })
}

function TargetOption({ target }: { target: TaskTarget }) {
  const { t } = useTranslation()
  return (
    <span className="flex min-w-0 items-center gap-2">
      <span className="shrink-0">{targetLabel(target, t)}</span>
      {target.platform !== LOCAL_PLATFORM && (
        <span className="truncate font-mono text-xs text-muted-foreground">{target.session_key}</span>
      )}
    </span>
  )
}

interface TaskFormDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  task?: Task | null
  workerId?: string
}

export function TaskFormDialog({ open, onOpenChange, task, workerId }: TaskFormDialogProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[calc(100dvh-2rem)] overflow-y-auto sm:max-w-lg">
        <TaskForm
          key={task?.id ?? "new"}
          task={task ?? null}
          workerId={workerId}
          onDone={() => onOpenChange(false)}
        />
      </DialogContent>
    </Dialog>
  )
}

interface TaskFormProps {
  task: Task | null
  workerId?: string
  onDone: () => void
}

function TaskForm({ task, workerId, onDone }: TaskFormProps) {
  const { t } = useTranslation()
  const isEdit = task !== null
  const [values, setValues] = useState<TaskFormValues>(() => initialTaskFormValues(task, workerId, Date.now()))
  const [sessionKey, setSessionKey] = useState<string | null>(null)
  const [error, setError] = useState("")
  const [minRunAt] = useState(() => toDateTimeLocalValue(Date.now()))

  const { data: workers = [] } = useWorkers()
  const { data: targets = [], isLoading: targetsLoading } = useTaskTargets({ enabled: !isEdit })
  const createTask = useCreateTask()
  const updateTask = useUpdateTask()

  const workerLocked = Boolean(workerId)
  const workerNameById = Object.fromEntries(workers.map((w) => [w.id, w.name]))
  if (task) workerNameById[task.worker_id] ??= task.worker_name
  const targetByKey = Object.fromEntries(targets.map((target) => [target.session_key, target]))
  const defaultTarget = targets.find((target) => target.platform === LOCAL_PLATFORM) ?? targets[0]
  const selectedSessionKey = sessionKey ?? defaultTarget?.session_key ?? ""
  const noTargets = !isEdit && !targetsLoading && targets.length === 0
  const pending = createTask.isPending || updateTask.isPending

  const update = (patch: Partial<TaskFormValues>) => {
    setValues((prev) => ({ ...prev, ...patch }))
    setError("")
  }

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault()
    const result = toTaskInput(values, Date.now())
    if (result.error) {
      setError(t(`tasks.form.errors.${result.error}`))
      return
    }
    try {
      if (task) {
        await updateTask.mutateAsync({ id: task.id, data: result.input })
      } else {
        await createTask.mutateAsync({ ...result.input, session_key: selectedSessionKey })
      }
      onDone()
    } catch (err) {
      setError(getErrorMessage(err))
    }
  }

  return (
    <form onSubmit={handleSubmit} className="grid gap-4">
      <DialogHeader>
        <DialogTitle>{isEdit ? t("tasks.form.editTitle") : t("tasks.form.createTitle")}</DialogTitle>
        <DialogDescription>
          {isEdit ? t("tasks.form.editDescription") : t("tasks.form.createDescription")}
        </DialogDescription>
      </DialogHeader>

      {error && <div role="alert" className={ALERT_DESTRUCTIVE}>{error}</div>}

      <div className="space-y-1.5">
        <Label htmlFor="task-worker">{t("tasks.form.worker")}</Label>
        <Select
          value={values.workerId || null}
          onValueChange={(v) => v && update({ workerId: v })}
          disabled={workerLocked}
        >
          <SelectTrigger id="task-worker" className="w-full">
            <SelectValue placeholder={t("tasks.form.workerPlaceholder")}>
              {(value: string | null) => (value ? workerNameById[value] ?? value : t("tasks.form.workerPlaceholder"))}
            </SelectValue>
          </SelectTrigger>
          <SelectContent>
            {workers.map((w) => (
              <SelectItem key={w.id} value={w.id}>{w.name}</SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      {!isEdit && (
        <div className="space-y-1.5">
          <Label htmlFor="task-target">{t("tasks.form.target")}</Label>
          {noTargets ? (
            <p className="rounded-sm border border-dashed border-border px-3 py-2 text-sm text-muted-foreground">
              {t("tasks.form.targetEmpty")}
            </p>
          ) : (
            <Select
              value={selectedSessionKey || null}
              onValueChange={(v) => v && setSessionKey(v)}
              disabled={targetsLoading}
            >
              <SelectTrigger id="task-target" className="w-full">
                <SelectValue placeholder={t("tasks.form.targetPlaceholder")}>
                  {(value: string | null) => {
                    const target = value ? targetByKey[value] : undefined
                    return target ? <TargetOption target={target} /> : t("tasks.form.targetPlaceholder")
                  }}
                </SelectValue>
              </SelectTrigger>
              <SelectContent>
                {targets.map((target) => (
                  <SelectItem key={target.session_key} value={target.session_key}>
                    <TargetOption target={target} />
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          )}
          <p className="text-xs text-muted-foreground">{t("tasks.form.targetHelper")}</p>
        </div>
      )}

      <div className="space-y-1.5">
        <Label id="task-type-label">{t("tasks.form.type")}</Label>
        <div role="group" aria-labelledby="task-type-label" className="grid grid-cols-2 gap-1 rounded-sm border border-border p-1">
          {TYPE_OPTIONS.map(({ value, icon: Icon }) => {
            const active = values.type === value
            return (
              <button
                key={value}
                type="button"
                aria-pressed={active}
                onClick={() => update({ type: value })}
                className={cn(
                  "inline-flex h-7 items-center justify-center gap-1.5 rounded-sm text-sm font-medium transition-colors outline-none focus-visible:ring-3 focus-visible:ring-ring/50",
                  active ? "bg-primary text-primary-foreground" : "text-muted-foreground hover:bg-muted hover:text-foreground"
                )}
              >
                <Icon className="size-3.5" />
                {t(`tasks.form.types.${value}`)}
              </button>
            )
          })}
        </div>
      </div>

      {values.type === "scheduled" ? (
        <div className="space-y-1.5">
          <Label htmlFor="task-cron">{t("tasks.form.cron")}</Label>
          <Input
            id="task-cron"
            value={values.cronExpr}
            onChange={(e) => update({ cronExpr: e.target.value })}
            placeholder="0 9 * * *"
            className="font-mono"
            spellCheck={false}
            autoComplete="off"
          />
          <div className="flex flex-wrap gap-1.5">
            {CRON_PRESETS.map((preset) => {
              const active = values.cronExpr.trim() === preset.expr
              return (
                <Button
                  key={preset.key}
                  type="button"
                  variant="outline"
                  size="xs"
                  className={cn("rounded-sm", active && "border-primary/60 bg-primary/10 text-primary hover:bg-primary/15 hover:text-primary")}
                  aria-pressed={active}
                  onClick={() => update({ cronExpr: preset.expr })}
                >
                  {t(`tasks.form.presets.${preset.key}`)}
                </Button>
              )
            })}
          </div>
          <p className="text-xs text-muted-foreground">{t("tasks.form.cronHelper")}</p>
        </div>
      ) : (
        <div className="space-y-1.5">
          <Label htmlFor="task-run-at">{t("tasks.form.runAt")}</Label>
          <Input
            id="task-run-at"
            type="datetime-local"
            value={values.runAt}
            min={minRunAt}
            onChange={(e) => update({ runAt: e.target.value })}
            className="font-mono"
          />
        </div>
      )}

      <div className="space-y-1.5">
        <Label htmlFor="task-instruction">{t("tasks.form.instruction")}</Label>
        <Textarea
          id="task-instruction"
          value={values.instruction}
          onChange={(e) => update({ instruction: e.target.value })}
          placeholder={t("tasks.form.instructionPlaceholder")}
          rows={4}
          className="max-h-60"
        />
      </div>

      <DialogFooter>
        <Button type="button" variant="outline" onClick={onDone}>
          {t("common.cancel")}
        </Button>
        <Button type="submit" disabled={pending || (!isEdit && !selectedSessionKey)}>
          {isEdit ? t("common.save") : t("common.create")}
        </Button>
      </DialogFooter>
    </form>
  )
}

export function NewTaskButton({ workerId, ...props }: { workerId?: string } & ComponentProps<typeof Button>) {
  const { t } = useTranslation()
  const canWrite = useCan(Perm.TasksWrite)
  const [open, setOpen] = useState(false)
  if (!canWrite) return null
  return (
    <>
      <Button onClick={() => setOpen(true)} {...props}>
        <PlusIcon className="size-4" />
        {t("tasks.create")}
      </Button>
      <TaskFormDialog open={open} onOpenChange={setOpen} workerId={workerId} />
    </>
  )
}
