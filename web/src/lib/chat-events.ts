import type { QueryClient } from "@tanstack/react-query"
import type {
  ChatMessage,
  ConversationActivity,
  ConversationId,
  ConversationSummary,
  Worker,
  WorkerStatus,
} from "@/lib/types"

export type TaskState = "queued" | "running" | "completed" | "failed" | "cancelled"

export interface ChatMessageEvent {
  type: "message"
  seq: number
  conversation_id: ConversationId
  message: ChatMessage
}

export interface WorkerStatusEvent {
  type: "worker_status"
  seq: number
  worker_id: string
  status: WorkerStatus
}

export interface TaskEvent {
  type: "task"
  seq: number
  conversation_id: ConversationId
  task_id: string
  worker_id: string
  state: TaskState
  queued_ahead?: number
}

export type ChatEvent = ChatMessageEvent | WorkerStatusEvent | TaskEvent

export interface ConversationView {
  active: ConversationId | null
  visible: boolean
}

export const CONVERSATIONS_KEY = ["conversations"] as const

const PREVIEW_MAX_CODE_POINTS = 120
const MEDIA_MARKERS = ["\u0000[file] ", "[file] "]
const WHITESPACE = /[\t\n\v\f\r \x85\xa0\u{1680}\u{2000}-\u{200a}\u{2028}\u{2029}\u{202f}\u{205f}\u{3000}]+/u

function stripMediaMarkers(content: string): string {
  let rest = content
  while (MEDIA_MARKERS.some((marker) => rest.startsWith(marker))) {
    const end = rest.indexOf("\n")
    if (end === -1) break
    rest = rest.slice(end + 1)
  }
  return rest
}

export function previewOf(message: ChatMessage): string {
  const text = stripMediaMarkers(message.content).split(WHITESPACE).filter(Boolean).join(" ")
  return Array.from(text).slice(0, PREVIEW_MAX_CODE_POINTS).join("")
}

export function detectGap(lastSeq: number | null, seq: number): boolean {
  return lastSeq !== null && seq !== lastSeq + 1
}

function updateConversation(
  list: ConversationSummary[],
  id: ConversationId,
  update: (item: ConversationSummary) => ConversationSummary,
): ConversationSummary[] {
  const index = list.findIndex((item) => item.id === id)
  if (index === -1) return list
  return list.map((item, i) => (i === index ? update(item) : item))
}

function activityOf(event: TaskEvent): ConversationActivity {
  if (event.state !== "queued" && event.state !== "running") return { state: "idle" }
  return event.queued_ahead === undefined
    ? { state: event.state }
    : { state: event.state, queued_ahead: event.queued_ahead }
}

export function applyEventToConversations(
  list: ConversationSummary[],
  event: ChatEvent,
  view: ConversationView,
): ConversationSummary[] {
  switch (event.type) {
    case "message": {
      const { message } = event
      const seen = message.sender.type === "user" || (view.active === event.conversation_id && view.visible)
      return updateConversation(list, event.conversation_id, (item) => ({
        ...item,
        last_message: {
          sender: message.sender,
          kind: message.kind,
          preview: previewOf(message),
          ...(message.handoff ? { handoff_worker_id: message.handoff.worker_id } : {}),
          ts: message.ts,
        },
        unread: seen ? item.unread : item.unread + 1,
      }))
    }
    case "task":
      return updateConversation(list, event.conversation_id, (item) => ({ ...item, activity: activityOf(event) }))
    case "worker_status":
      return list
  }
}

export function applyWorkerStatus(workers: Worker[], event: ChatEvent): Worker[] {
  if (event.type !== "worker_status") return workers
  const index = workers.findIndex((worker) => worker.id === event.worker_id)
  if (index === -1 || workers[index].status === event.status) return workers
  return workers.map((worker, i) => (i === index ? { ...worker, status: event.status } : worker))
}

type WorkerCacheData = Worker | Worker[] | undefined

export function workerCacheUpdater(event: ChatEvent) {
  return (data: WorkerCacheData): WorkerCacheData => {
    if (!data) return undefined
    if (Array.isArray(data)) {
      const next = applyWorkerStatus(data, event)
      return next === data ? undefined : next
    }
    const [next] = applyWorkerStatus([data], event)
    return next === data ? undefined : next
  }
}

function updateConversationCache(
  queryClient: QueryClient,
  event: ChatMessageEvent | TaskEvent,
  view: ConversationView,
) {
  const list = queryClient.getQueryData<ConversationSummary[]>(CONVERSATIONS_KEY)
  if (!list) return
  if (!list.some((item) => item.id === event.conversation_id)) {
    queryClient.invalidateQueries({ queryKey: CONVERSATIONS_KEY })
    return
  }
  const next = applyEventToConversations(list, event, view)
  if (next !== list) queryClient.setQueryData<ConversationSummary[]>(CONVERSATIONS_KEY, next)
}

export function applyEventToCaches(queryClient: QueryClient, event: ChatEvent, view: ConversationView) {
  switch (event.type) {
    case "message":
    case "task":
      updateConversationCache(queryClient, event, view)
      return
    case "worker_status":
      queryClient.setQueriesData<Worker | Worker[]>(
        { predicate: (query) => query.queryKey[0] === "workers" },
        workerCacheUpdater(event),
      )
      return
  }
}

export function ingestChatEvent(
  queryClient: QueryClient,
  event: ChatEvent,
  lastSeq: number | null,
  view: ConversationView,
  onGap: () => void,
): number {
  const gap = detectGap(lastSeq, event.seq)
  applyEventToCaches(queryClient, event, view)
  if (gap) onGap()
  return event.seq
}
