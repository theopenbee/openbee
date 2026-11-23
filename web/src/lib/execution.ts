import type { ExecutionStep, ExecutionSummary, ToolKind } from "@/lib/types"

type Translate = (key: string, options?: Record<string, unknown>) => string

export const EXECUTION_POLL_MS = 2000

const TOOL_KINDS: readonly string[] = [
  "command",
  "read",
  "edit",
  "search",
  "web",
  "message",
  "other",
] satisfies ToolKind[]

const STEP_TYPES: readonly string[] = ["text", "thinking", "error", "tool"] satisfies ExecutionStep["type"][]

export function knownSteps(steps: readonly ExecutionStep[]): ExecutionStep[] {
  return steps.filter((step) => STEP_TYPES.includes(step.type))
}

export function isToolKind(kind: string): kind is ToolKind {
  return TOOL_KINDS.includes(kind)
}

export function formatDuration(ms: number, t: Translate): string {
  const seconds = Math.max(1, Math.round(ms / 1000))
  if (seconds < 60) return t("execution.seconds", { count: seconds })
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) {
    const rest = seconds % 60
    return rest ? t("execution.minutesSeconds", { minutes, seconds: rest }) : t("execution.minutes", { count: minutes })
  }
  const hours = Math.floor(minutes / 60)
  const rest = minutes % 60
  return rest ? t("execution.hoursMinutes", { hours, minutes: rest }) : t("execution.hours", { count: hours })
}

export function toolDuration(ms: number | undefined, t: Translate): string | null {
  return ms !== undefined && ms >= 1000 ? formatDuration(ms, t) : null
}

export function handoffRunLabel(run: ExecutionSummary, latest: boolean, t: Translate): string {
  const prefix = latest ? t("execution.latest") : ""
  const status = t(`execution.status.${run.status}`)
  if (!run.completed_at) return prefix + status
  const took = t("execution.took", { duration: formatDuration(run.completed_at - run.started_at, t) })
  return `${prefix}${status} · ${took}`
}

export function stepStats(steps: readonly ExecutionStep[]): { steps: number; tools: number } {
  return { steps: steps.length, tools: steps.filter((step) => step.type === "tool").length }
}
