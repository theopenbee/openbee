import { ApiError } from "@/lib/api"
import { workerIdOf } from "@/lib/conversations"
import type { ActiveTask, ChatMessage, ConversationActivity, ConversationId, Worker } from "@/lib/types"

export const GROUP_GAP_MS = 5 * 60_000

const INSTRUCTION_PREVIEW_LINES = 3

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

export function composerDisabled(activity: ConversationActivity): boolean {
  return false
}

export function statusText(activity: ConversationActivity, worker: Worker | undefined, t: Translate): string {
  if (activity.state === "queued") return t("chat.status.queued", { count: activity.queued_ahead ?? 0 })
  if (activity.state === "running") return t("chat.status.running")
  return worker?.status === "error" ? t("statuses.error") : t("statuses.idle")
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

export function instructionPreview(content: string): string {
  return content
    .split("\n")
    .map((line) => line.trim())
    .filter(Boolean)
    .slice(0, INSTRUCTION_PREVIEW_LINES)
    .join("\n")
}
