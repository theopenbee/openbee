import { QueryClient } from "@tanstack/react-query"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"
import type { ChatMessage, ConversationSummary, Worker } from "../types"
import {
  CONVERSATIONS_KEY,
  applyEventToCaches,
  applyEventToConversations,
  applyWorkerStatus,
  detectGap,
  ingestChatEvent,
  previewOf,
  workerCacheUpdater,
  type ChatEvent,
} from "../chat-events"

const summary = (id: ConversationSummary["id"], extra: Partial<ConversationSummary> = {}): ConversationSummary => ({
  id,
  kind: id === "bee" ? "bee" : "worker",
  worker_id: id === "bee" ? undefined : id.slice("worker:".length),
  unread: 0,
  activity: { state: "idle" },
  ...extra,
})

const message = (content: string, extra: Partial<ChatMessage> = {}): ChatMessage => ({
  id: "m1",
  sender: { type: "worker", id: "w1" },
  kind: "message",
  content,
  ts: 100,
  ...extra,
})

const messageEvent = (conversationId: ConversationSummary["id"], msg: ChatMessage, seq = 1): ChatEvent => ({
  type: "message",
  seq,
  conversation_id: conversationId,
  message: msg,
})

const taskEvent = (
  state: Extract<ChatEvent, { type: "task" }>["state"],
  queuedAhead?: number,
): ChatEvent => ({
  type: "task",
  seq: 1,
  conversation_id: "worker:w1",
  task_id: "t1",
  worker_id: "w1",
  state,
  queued_ahead: queuedAhead,
})

const idle = { active: null, visible: true }

const worker = (id: string, status: Worker["status"]): Worker => ({
  id,
  name: id,
  description: "",
  constraints: "",
  work_dir: "",
  engine: "claude",
  status,
  created_at: 0,
  updated_at: 0,
})

describe("applyEventToConversations", () => {
  it("message event updates preview and unread for inactive", () => {
    const list = [summary("bee"), summary("worker:w1", { unread: 2 })]

    const next = applyEventToConversations(list, messageEvent("worker:w1", message("  hello\n there ", { ts: 150 })), idle)

    expect(next[1].last_message).toEqual({
      sender: { type: "worker", id: "w1" },
      kind: "message",
      preview: "hello there",
      ts: 150,
    })
    expect(next[1].unread).toBe(3)
    expect(next[0]).toBe(list[0])
    expect(list[1].unread).toBe(2)
  })

  it("message event does not bump unread for active visible conversation", () => {
    const list = [summary("bee"), summary("worker:w1")]

    const next = applyEventToConversations(list, messageEvent("worker:w1", message("hi")), {
      active: "worker:w1",
      visible: true,
    })

    expect(next[1].unread).toBe(0)
    expect(next[1].last_message?.preview).toBe("hi")
  })

  it("message event bumps unread for the active conversation while the tab is hidden", () => {
    const list = [summary("worker:w1")]

    const next = applyEventToConversations(list, messageEvent("worker:w1", message("hi")), {
      active: "worker:w1",
      visible: false,
    })

    expect(next[0].unread).toBe(1)
  })

  it("message event bumps unread when another conversation is active", () => {
    const list = [summary("bee"), summary("worker:w1")]

    const next = applyEventToConversations(list, messageEvent("worker:w1", message("hi")), {
      active: "bee",
      visible: true,
    })

    expect(next[1].unread).toBe(1)
  })

  it("own message does not bump unread", () => {
    const list = [summary("worker:w1", { unread: 1 })]

    const next = applyEventToConversations(
      list,
      messageEvent("worker:w1", message("mine", { sender: { type: "user" } })),
      idle,
    )

    expect(next[0].unread).toBe(1)
    expect(next[0].last_message?.sender).toEqual({ type: "user" })
    expect(next[0].last_message?.preview).toBe("mine")
  })

  it("keeps the message kind in the last message", () => {
    const list = [summary("bee")]

    const next = applyEventToConversations(
      list,
      messageEvent("bee", message("delegated", { kind: "handoff", sender: { type: "bee" } })),
      idle,
    )

    expect(next[0].last_message?.kind).toBe("handoff")
    expect(next[0].unread).toBe(1)
  })

  it("task queued then completed toggles activity", () => {
    const list = [summary("bee"), summary("worker:w1")]

    const queued = applyEventToConversations(list, taskEvent("queued", 2), idle)
    expect(queued[1].activity).toEqual({ state: "queued", queued_ahead: 2 })

    const running = applyEventToConversations(queued, taskEvent("running"), idle)
    expect(running[1].activity).toEqual({ state: "running" })
    expect(running[1].activity).not.toHaveProperty("queued_ahead")

    const completed = applyEventToConversations(running, taskEvent("completed"), idle)
    expect(completed[1].activity).toEqual({ state: "idle" })
    expect(completed[0]).toBe(list[0])
  })

  it.each(["failed", "cancelled"] as const)("task %s sets activity to idle", (state) => {
    const list = [summary("worker:w1", { activity: { state: "running" } })]

    const next = applyEventToConversations(list, taskEvent(state), idle)

    expect(next[0].activity).toEqual({ state: "idle" })
  })

  it("returns the list unchanged for worker_status events", () => {
    const list = [summary("worker:w1")]

    const next = applyEventToConversations(
      list,
      { type: "worker_status", seq: 1, worker_id: "w1", status: "working" },
      idle,
    )

    expect(next).toBe(list)
  })

  it("returns the list unchanged for an unknown conversation", () => {
    const list = [summary("bee")]

    expect(applyEventToConversations(list, messageEvent("worker:gone", message("hi")), idle)).toBe(list)
    expect(applyEventToConversations(list, taskEvent("running"), idle)).toBe(list)
  })
})

describe("applyWorkerStatus", () => {
  it("updates only the matching worker", () => {
    const workers = [worker("w1", "idle"), worker("w2", "idle")]

    const next = applyWorkerStatus(workers, { type: "worker_status", seq: 1, worker_id: "w2", status: "working" })

    expect(next.map((w) => w.status)).toEqual(["idle", "working"])
    expect(next[0]).toBe(workers[0])
    expect(workers[1].status).toBe("idle")
  })

  it("returns the same array when nothing changes", () => {
    const workers = [worker("w1", "working")]

    expect(applyWorkerStatus(workers, { type: "worker_status", seq: 1, worker_id: "w1", status: "working" })).toBe(workers)
    expect(applyWorkerStatus(workers, { type: "worker_status", seq: 1, worker_id: "other", status: "error" })).toBe(workers)
  })

  it("ignores other event types", () => {
    const workers = [worker("w1", "idle")]

    expect(applyWorkerStatus(workers, taskEvent("running"))).toBe(workers)
  })
})

describe("detectGap", () => {
  it("detectGap", () => {
    expect(detectGap(null, 1)).toBe(false)
    expect(detectGap(null, 40)).toBe(false)
    expect(detectGap(1, 2)).toBe(false)
    expect(detectGap(5, 7)).toBe(true)
    expect(detectGap(5, 5)).toBe(true)
    expect(detectGap(5, 1)).toBe(true)
  })
})

describe("previewOf", () => {
  it("previewOf strips markers and truncates", () => {
    expect(previewOf(message("\u0000[file] a.png\n\u0000[file] b.pdf\nlook\n\n  at   this\tpic"))).toBe("look at this pic")
    expect(previewOf(message("[file] a.png\n[file] b.pdf\ntwo files"))).toBe("two files")
    expect(previewOf(message("\u0000[file] a.png\n[file] b.pdf\nmixed"))).toBe("mixed")
    expect(previewOf(message("字".repeat(130)))).toBe("字".repeat(120))
    expect(previewOf(message("😀".repeat(130)))).toBe("😀".repeat(120))
    expect(previewOf(message("a".repeat(120)))).toBe("a".repeat(120))
  })

  it("keeps text that only looks like a marker", () => {
    expect(previewOf(message("see [file] a.png\nnow"))).toBe("see [file] a.png now")
    expect(previewOf(message("\u0000[file] unterminated"))).toBe("\u0000[file] unterminated")
    expect(previewOf(message("[file]nospace\nhello"))).toBe("[file]nospace hello")
  })

  it("returns an empty preview for blank or media-only content", () => {
    expect(previewOf(message(""))).toBe("")
    expect(previewOf(message("\u0000[file] a.png\n "))).toBe("")
    expect(previewOf(message(" \n\t\xa0\u{3000}\u{2028} "))).toBe("")
    expect(previewOf(message("a\xa0\u{3000}b\x85c"))).toBe("a b c")
  })
})

const statusEvent = (workerId: string, status: Worker["status"]): ChatEvent => ({
  type: "worker_status",
  seq: 1,
  worker_id: workerId,
  status,
})

describe("workerCacheUpdater", () => {
  it("returns undefined when nothing is cached", () => {
    expect(workerCacheUpdater(statusEvent("w1", "working"))(undefined)).toBeUndefined()
  })

  it("returns undefined for an unknown worker", () => {
    const update = workerCacheUpdater(statusEvent("gone", "working"))

    expect(update([worker("w1", "idle")])).toBeUndefined()
    expect(update(worker("w1", "idle"))).toBeUndefined()
  })

  it("returns undefined for an unchanged status", () => {
    const update = workerCacheUpdater(statusEvent("w1", "working"))

    expect(update([worker("w1", "working"), worker("w2", "idle")])).toBeUndefined()
    expect(update(worker("w1", "working"))).toBeUndefined()
  })

  it("returns undefined for events other than worker_status", () => {
    expect(workerCacheUpdater(taskEvent("running"))([worker("w1", "idle")])).toBeUndefined()
  })

  it("returns a new list when the status changes", () => {
    const workers = [worker("w1", "idle"), worker("w2", "idle")]

    const next = workerCacheUpdater(statusEvent("w1", "working"))(workers)

    expect(next).not.toBe(workers)
    expect(next).toEqual([worker("w1", "working"), worker("w2", "idle")])
    expect((next as Worker[])[1]).toBe(workers[1])
  })

  it("returns a new worker when the status changes for a single worker", () => {
    const current = worker("w1", "idle")

    const next = workerCacheUpdater(statusEvent("w1", "error"))(current)

    expect(next).not.toBe(current)
    expect(next).toEqual(worker("w1", "error"))
  })
})

describe("applyEventToCaches", () => {
  let queryClient: QueryClient
  const listKey = ["workers", { departmentId: undefined }]
  const departmentKey = ["workers", { departmentId: "d1" }]

  beforeEach(() => {
    vi.useFakeTimers()
    queryClient = new QueryClient()
  })

  afterEach(() => {
    queryClient.clear()
    vi.useRealTimers()
  })

  const seedWorkers = () => {
    queryClient.setQueryData(listKey, [worker("w1", "working"), worker("w2", "idle")])
    queryClient.setQueryData(departmentKey, [worker("w2", "idle")])
    queryClient.setQueryData(["workers", "w1"], worker("w1", "working"))
    queryClient.setQueryData(["workers", "w2"], worker("w2", "idle"))
  }

  const stamps = () =>
    [listKey, departmentKey, ["workers", "w1"], ["workers", "w2"]].map((key) => {
      const state = queryClient.getQueryState(key)
      return { invalidated: state?.isInvalidated, updatedAt: state?.dataUpdatedAt }
    })

  it.each([
    ["an unchanged status", statusEvent("w1", "working")],
    ["an unknown worker", statusEvent("gone", "error")],
  ])("worker_status with %s keeps invalidation and update time", (_, event) => {
    seedWorkers()
    queryClient.invalidateQueries({ queryKey: ["workers"] })
    vi.advanceTimersByTime(5000)
    const before = stamps()

    applyEventToCaches(queryClient, event, idle)

    expect(stamps()).toEqual(before)
    expect(before.every((stamp) => stamp.invalidated)).toBe(true)
  })

  it("worker_status updates matching list and single caches in place", () => {
    seedWorkers()
    const other = queryClient.getQueryData<Worker>(["workers", "w2"])
    const sibling = queryClient.getQueryData<Worker[]>(listKey)![1]

    applyEventToCaches(queryClient, statusEvent("w1", "error"), idle)

    expect(queryClient.getQueryData<Worker[]>(listKey)!.map((w) => w.status)).toEqual(["error", "idle"])
    expect(queryClient.getQueryData<Worker[]>(listKey)![1]).toBe(sibling)
    expect(queryClient.getQueryData<Worker>(["workers", "w1"])?.status).toBe("error")
    expect(queryClient.getQueryData<Worker>(["workers", "w2"])).toBe(other)
    expect(queryClient.getQueryData<Worker[]>(departmentKey)!.map((w) => w.status)).toEqual(["idle"])
  })

  it("worker_status creates no cache entries", () => {
    applyEventToCaches(queryClient, statusEvent("w1", "error"), idle)

    expect(queryClient.getQueryCache().getAll()).toHaveLength(0)
  })

  it("message and task events update the cached conversations", () => {
    queryClient.setQueryData(CONVERSATIONS_KEY, [summary("bee"), summary("worker:w1")])

    applyEventToCaches(queryClient, messageEvent("worker:w1", message("hi")), idle)
    applyEventToCaches(queryClient, taskEvent("running"), idle)

    const list = queryClient.getQueryData<ConversationSummary[]>(CONVERSATIONS_KEY)!
    expect(list[1].unread).toBe(1)
    expect(list[1].last_message?.preview).toBe("hi")
    expect(list[1].activity).toEqual({ state: "running" })
  })

  it("conversation events write nothing when the list is not cached", () => {
    applyEventToCaches(queryClient, messageEvent("worker:w1", message("hi")), idle)

    expect(queryClient.getQueryCache().getAll()).toHaveLength(0)
  })

  it("invalidates the list for a conversation it does not know", () => {
    const list = [summary("bee")]
    queryClient.setQueryData(CONVERSATIONS_KEY, list)

    applyEventToCaches(queryClient, messageEvent("worker:new", message("hi")), idle)

    expect(queryClient.getQueryData(CONVERSATIONS_KEY)).toBe(list)
    expect(queryClient.getQueryState(CONVERSATIONS_KEY)?.isInvalidated).toBe(true)
  })
})

describe("ingestChatEvent", () => {
  let queryClient: QueryClient

  beforeEach(() => {
    vi.useFakeTimers()
    queryClient = new QueryClient()
    queryClient.setQueryData(CONVERSATIONS_KEY, [summary("bee"), summary("worker:w1")])
  })

  afterEach(() => {
    queryClient.clear()
    vi.useRealTimers()
  })

  it("returns the event seq without a gap callback for the first and the next event", () => {
    const onGap = vi.fn()

    expect(ingestChatEvent(queryClient, messageEvent("worker:w1", message("a"), 7), null, idle, onGap)).toBe(7)
    expect(ingestChatEvent(queryClient, messageEvent("worker:w1", message("b"), 8), 7, idle, onGap)).toBe(8)

    expect(onGap).not.toHaveBeenCalled()
    expect(queryClient.getQueryData<ConversationSummary[]>(CONVERSATIONS_KEY)![1].unread).toBe(2)
  })

  it.each([
    ["a skipped seq", 3, 9],
    ["a seq reset after a server restart", 40, 1],
  ])("reports %s as a gap", (_, lastSeq, seq) => {
    const onGap = vi.fn()

    const next = ingestChatEvent(queryClient, messageEvent("worker:w1", message("a"), seq), lastSeq, idle, onGap)

    expect(onGap).toHaveBeenCalledTimes(1)
    expect(next).toBe(seq)
  })

  it("applies the event before the gap callback so the invalidation survives", () => {
    const unreadWhenResynced: number[] = []
    const onGap = () => {
      unreadWhenResynced.push(queryClient.getQueryData<ConversationSummary[]>(CONVERSATIONS_KEY)![1].unread)
      queryClient.invalidateQueries({ queryKey: CONVERSATIONS_KEY })
    }

    ingestChatEvent(queryClient, messageEvent("worker:w1", message("a"), 9), 3, idle, onGap)

    expect(unreadWhenResynced).toEqual([1])
    expect(queryClient.getQueryState(CONVERSATIONS_KEY)?.isInvalidated).toBe(true)
  })
})
