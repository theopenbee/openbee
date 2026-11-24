import { ApiError } from "@/lib/api"
import { workerIdOf } from "@/lib/conversations"
import { findEngine } from "@/lib/engines"
import { profileEngine } from "@/lib/worker-edit"
import type { ActiveTask, ChatMessage, ConversationActivity, ConversationId, EngineChoice, Worker } from "@/lib/types"

export const GROUP_GAP_MS = 5 * 60_000

const INSTRUCTION_PREVIEW_LINES = 3

const SUBTITLE_JOIN = " · "

type Translate = (key: string, options?: Record<string, unknown>) => string

export interface SenderDisplay {
  kind: ChatMessage["sender"]["type"]
  name: string
  workerId?: string
  via?: "bee"
}

export function senderDisplay(
  message: ChatMessage,
  conversation: ConversationId,
  workers: Map<string, Worker>,
  t: Translate,
): SenderDisplay {
  const { sender } = message
  switch (sender.type) {
    case "user":
      return { kind: "user", name: t("chat.you") }
    case "bee":
      return workerIdOf(conversation)
        ? { kind: "bee", name: t("chat.beeLabel"), via: "bee" }
        : { kind: "bee", name: t("chat.beeLabel") }
    case "worker": {
      const workerId = sender.id ?? workerIdOf(conversation) ?? ""
      return { kind: "worker", name: workers.get(workerId)?.name ?? t("chat.deletedWorker"), workerId }
    }
    case "system":
      return { kind: "system", name: t("chat.system") }
  }
}

function sameSender(a: ChatMessage, b: ChatMessage): boolean {
  return a.sender.type === b.sender.type && (a.sender.id ?? "") === (b.sender.id ?? "")
}

export function groupStarts(messages: ChatMessage[]): boolean[] {
  return messages.map((message, index) => {
    const prev = messages[index - 1]
    return (
      !prev ||
      prev.kind === "reset" ||
      message.kind === "reset" ||
      !sameSender(prev, message) ||
      message.ts - prev.ts >= GROUP_GAP_MS
    )
  })
}

const BEE_BUSY: ConversationActivity = { state: "running" }

export function beeActivity(server: ConversationActivity, awaitingBee: boolean): ConversationActivity {
  return awaitingBee && server.state === "idle" ? BEE_BUSY : server
}

export function composerDisabled(activity: ConversationActivity): boolean {
  return false
}

export function statusText(activity: ConversationActivity, worker: Worker | undefined, t: Translate): string {
  if (activity.state === "queued") return t("chat.status.queued", { count: activity.queued_ahead ?? 0 })
  if (activity.state === "running") return t("chat.status.running")
  return worker?.status === "error" ? t("statuses.error") : t("statuses.idle")
}

export function headerSubtitle(
  worker: Worker | undefined,
  engines: readonly EngineChoice[],
  systemDefault?: string,
  defaultModels?: Record<string, string>,
): string {
  if (!worker) return ""
  const description = worker.description.split(/\s+/).filter(Boolean).join(" ")
  const shown = profileEngine(worker, systemDefault, engines.map((engine) => engine.id), defaultModels)
  const parts = shown ? [findEngine(engines, shown.engine).name, shown.model, description] : [description]
  return parts.filter(Boolean).join(SUBTITLE_JOIN)
}

export function taskTypeLabelKey(type: string): string | null {
  if (type === "countdown") return "chat.taskType.countdown"
  if (type === "scheduled") return "chat.taskType.scheduled"
  return null
}

function isActiveTask(value: unknown): value is ActiveTask {
  if (typeof value !== "object" || value === null) return false
  const task = value as Record<string, unknown>
  return typeof task.id === "string" && typeof task.instruction === "string" && typeof task.type === "string"
}

export function activeTasksOf(err: unknown): ActiveTask[] | null {
  if (!(err instanceof ApiError) || err.status !== 409) return null
  const tasks = err.details?.active_tasks
  return Array.isArray(tasks) ? tasks.filter(isActiveTask) : null
}

export function isChoiceTaken(err: unknown): boolean {
  return err instanceof ApiError && err.status === 409
}

export type ChoiceState = "open" | "loading" | "chosen" | "disabled"

export function choiceStateOf(index: number, choice: number | undefined, pending: number | null): ChoiceState {
  if (typeof choice === "number") return index === choice ? "chosen" : "disabled"
  if (pending !== null) return index === pending ? "loading" : "disabled"
  return "open"
}

export function instructionPreview(content: string): string {
  return content
    .split("\n")
    .map((line) => line.trim())
    .filter(Boolean)
    .slice(0, INSTRUCTION_PREVIEW_LINES)
    .join("\n")
}

export function showsHandoffTask(messages: readonly ChatMessage[], taskId: string): boolean {
  return messages.some((message) => message.handoff?.task_id === taskId)
}
