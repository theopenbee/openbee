import type { ConversationId, ConversationSummary, Worker } from "@/lib/types"

export const BEE_CONVERSATION: ConversationId = "bee"

const WORKER_PREFIX = "worker:"

export function workerConversationId(workerId: string): ConversationId {
  return `${WORKER_PREFIX}${workerId}`
}

export function workerIdOf(id: ConversationId): string | null {
  return id.startsWith(WORKER_PREFIX) ? id.slice(WORKER_PREFIX.length) : null
}

export function conversationPath(id: ConversationId): string {
  return `/chat/${workerIdOf(id) ?? BEE_CONVERSATION}`
}

function nameOf(item: ConversationSummary, names: Map<string, string>): string {
  return (item.worker_id && names.get(item.worker_id)) || ""
}

export function sortConversations(
  list: ConversationSummary[],
  names: Map<string, string>,
): ConversationSummary[] {
  return [...list].sort((a, b) => {
    if (a.id === BEE_CONVERSATION) return b.id === BEE_CONVERSATION ? 0 : -1
    if (b.id === BEE_CONVERSATION) return 1
    const aTs = a.last_message?.ts
    const bTs = b.last_message?.ts
    if (aTs !== undefined && bTs !== undefined) return bTs - aTs
    if (aTs !== undefined) return -1
    if (bTs !== undefined) return 1
    return nameOf(a, names).localeCompare(nameOf(b, names))
  })
}

export function filterConversations(
  list: ConversationSummary[],
  query: string,
  workers: Map<string, Worker>,
): ConversationSummary[] {
  const needle = query.trim().toLowerCase()
  if (!needle) return list
  return list.filter((item) => {
    if (item.id === BEE_CONVERSATION) return true
    const worker = item.worker_id ? workers.get(item.worker_id) : undefined
    if (!worker) return false
    return worker.name.toLowerCase().includes(needle) || worker.description.toLowerCase().includes(needle)
  })
}

type LastMessage = NonNullable<ConversationSummary["last_message"]>
type Translate = (key: string, options?: Record<string, string>) => string

export function previewText(last: LastMessage, workers: Map<string, Worker>, t: Translate): string {
  if (last.kind === "handoff" && last.handoff_worker_id) {
    const name = workers.get(last.handoff_worker_id)?.name ?? t("chat.deletedWorker")
    return t("chat.handedOffTo", { name })
  }
  return last.sender.type === "user" ? `${t("chat.youPrefix")}${last.preview}` : last.preview
}
