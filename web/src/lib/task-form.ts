import type { EditableTaskType, Task, TaskInput } from "./types"

export const MIN_COUNTDOWN_LEAD_MS = 5_000

export const CRON_PRESETS = [
  { key: "hourly", expr: "0 * * * *" },
  { key: "daily", expr: "0 9 * * *" },
  { key: "weekdays", expr: "0 9 * * 1-5" },
  { key: "weekly", expr: "0 9 * * 1" },
] as const

const DEFAULT_CRON = "0 9 * * *"

export interface TaskFormValues {
  workerId: string
  instruction: string
  type: EditableTaskType
  cronExpr: string
  runAt: string
}

export type TaskFormError = "worker" | "instruction" | "cron" | "runAt"

export type TaskInputResult = { input: TaskInput; error?: never } | { input?: never; error: TaskFormError }

const pad = (n: number) => String(n).padStart(2, "0")

export function toDateTimeLocalValue(ms: number): string {
  const d = new Date(ms)
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`
}

export function nextFullHour(now: number): number {
  const d = new Date(now)
  d.setMinutes(0, 0, 0)
  d.setHours(d.getHours() + 1)
  return d.getTime()
}

export function initialTaskFormValues(task: Task | null | undefined, workerId: string | undefined, now: number): TaskFormValues {
  return {
    workerId: task?.worker_id ?? workerId ?? "",
    instruction: task?.instruction ?? "",
    type: task?.type === "countdown" ? "countdown" : "scheduled",
    cronExpr: task?.cron_expr || DEFAULT_CRON,
    runAt: toDateTimeLocalValue(task?.scheduled_at ?? nextFullHour(now)),
  }
}

export function toTaskInput(values: TaskFormValues, now: number): TaskInputResult {
  const instruction = values.instruction.trim()
  if (!values.workerId) return { error: "worker" }
  if (!instruction) return { error: "instruction" }

  if (values.type === "scheduled") {
    const cronExpr = values.cronExpr.trim().replace(/\s+/g, " ")
    if (!cronExpr) return { error: "cron" }
    return { input: { worker_id: values.workerId, instruction, type: "scheduled", cron_expr: cronExpr } }
  }

  const scheduledAt = values.runAt ? new Date(values.runAt).getTime() : Number.NaN
  if (Number.isNaN(scheduledAt) || scheduledAt < now + MIN_COUNTDOWN_LEAD_MS) return { error: "runAt" }
  return { input: { worker_id: values.workerId, instruction, type: "countdown", scheduled_at: scheduledAt } }
}
