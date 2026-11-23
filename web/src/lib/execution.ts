import type { ExecutionDetail, ExecutionStep, ExecutionSummary, ToolKind, ToolStep } from "@/lib/types"

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

export function toolKind(step: ToolStep): ToolKind {
  return isToolKind(step.kind) ? step.kind : "other"
}

export function toolTitle(step: ToolStep, t: Translate): string {
  const kind = toolKind(step)
  if (kind === "other") return t("execution.tool.other", { name: step.name })
  const summary = step.summary?.replace(/\s+/g, " ").trim()
  return summary ? t(`execution.toolTitle.${kind}`, { summary }) : t(`execution.tool.${kind}`)
}

export function splitThinking(text: string): { title: string; rest: string } {
  const [first, ...rest] = text.trim().split("\n")
  return { title: first.trim().replace(/^(\*\*|__)((?:(?!\*\*|__).)+)\1$/, "$2"), rest: rest.join("\n").trim() }
}

export type DiffLineType = "add" | "remove" | "context" | "note"

export interface DiffLine {
  type: DiffLineType
  text: string
}

const DIFF_MARKERS: Record<string, DiffLineType> = { "+": "add", "-": "remove", " ": "context" }

export function diffLines(diff: string): DiffLine[] {
  return diff
    .replace(/\n$/, "")
    .split("\n")
    .map((line): DiffLine => {
      const type = DIFF_MARKERS[line.charAt(0)]
      return type ? { type, text: line.slice(1) } : { type: "note", text: line }
    })
}

export function diffStats(lines: readonly DiffLine[]): { added: number; removed: number } {
  return {
    added: lines.filter((line) => line.type === "add").length,
    removed: lines.filter((line) => line.type === "remove").length,
  }
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

export function switcherRuns(execution: ExecutionDetail): ExecutionSummary[] {
  const runs = execution.runs ?? []
  if (runs.length === 0 || runs.some((run) => run.id === execution.id)) return runs
  const { id, status, started_at, completed_at } = execution
  return [...runs, { id, status, started_at, completed_at }]
}
