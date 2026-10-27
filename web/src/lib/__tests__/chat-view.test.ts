import { describe, expect, it } from "vitest"
import { ApiError } from "../api"
import type { ChatMessage, Worker } from "../types"
import {
  activeTasksOf,
  composerDisabled,
  groupStarts,
  instructionPreview,
  senderDisplay,
  statusText,
  taskTypeLabelKey,
} from "../chat-view"

const MIN = 60_000

const t = (key: string, options?: Record<string, unknown>) =>
  options && "count" in options ? `${key}:${options.count}` : key

const worker = (id: string, name: string, status: Worker["status"] = "idle"): Worker => ({
  id,
  name,
  description: "",
  constraints: "",
  work_dir: "",
  engine: "claude",
  status,
  created_at: 0,
  updated_at: 0,
})

const workers = new Map([["w1", worker("w1", "Lin")]])

const message = (sender: ChatMessage["sender"], ts = 0, extra: Partial<ChatMessage> = {}): ChatMessage => ({
  id: `${sender.type}-${sender.id ?? ""}-${ts}`,
  sender,
  kind: "message",
  content: "hi",
  ts,
  ...extra,
})

describe("senderDisplay", () => {
  it("senderDisplay labels bee handoff inside worker chat", () => {
    expect(senderDisplay(message({ type: "bee" }), "worker:w1", workers, t)).toEqual({
      kind: "bee",
      name: "chat.beeLabel",
      via: "bee",
    })
    expect(senderDisplay(message({ type: "bee" }), "bee", workers, t)).toEqual({
      kind: "bee",
      name: "chat.beeLabel",
    })
  })

  it("senderDisplay marks deleted worker", () => {
    expect(senderDisplay(message({ type: "worker", id: "gone" }), "bee", workers, t)).toEqual({
      kind: "worker",
      name: "chat.deletedWorker",
      workerId: "gone",
    })
    expect(senderDisplay(message({ type: "worker", id: "w1" }), "bee", workers, t)).toEqual({
      kind: "worker",
      name: "Lin",
      workerId: "w1",
    })
  })

  it("falls back to the chat's worker when a worker message has no id", () => {
    expect(senderDisplay(message({ type: "worker" }), "worker:w1", workers, t)).toEqual({
      kind: "worker",
      name: "Lin",
      workerId: "w1",
    })
  })

  it("names the user and system senders", () => {
    expect(senderDisplay(message({ type: "user" }), "worker:w1", workers, t)).toEqual({ kind: "user", name: "chat.you" })
    expect(senderDisplay(message({ type: "system" }), "worker:w1", workers, t)).toEqual({
      kind: "system",
      name: "chat.system",
    })
  })
})

describe("groupStarts", () => {
  it("groupStarts splits on sender and five minute gap", () => {
    const messages = [
      message({ type: "user" }, 0),
      message({ type: "user" }, 1 * MIN),
      message({ type: "bee" }, 2 * MIN),
      message({ type: "bee" }, 6 * MIN + 59_999),
      message({ type: "bee" }, 11 * MIN + 59_999),
      message({ type: "worker", id: "w1" }, 12 * MIN),
      message({ type: "worker", id: "w2" }, 12 * MIN),
      message({ type: "worker", id: "w2" }, 13 * MIN),
    ]

    expect(groupStarts(messages)).toEqual([true, false, true, false, true, true, true, false])
  })

  it("returns an empty list for no messages", () => {
    expect(groupStarts([])).toEqual([])
  })
})

describe("composerDisabled", () => {
  it("composerDisabled is false while busy", () => {
    expect(composerDisabled({ state: "running" })).toBe(false)
    expect(composerDisabled({ state: "queued", queued_ahead: 3 })).toBe(false)
    expect(composerDisabled({ state: "idle" })).toBe(false)
  })
})

describe("statusText", () => {
  it("prefers the conversation activity over the worker status", () => {
    expect(statusText({ state: "queued", queued_ahead: 2 }, worker("w1", "Lin", "error"), t)).toBe("chat.status.queued:2")
    expect(statusText({ state: "running" }, worker("w1", "Lin", "error"), t)).toBe("chat.status.running")
  })

  it("counts zero ahead when the position is unknown", () => {
    expect(statusText({ state: "queued" }, undefined, t)).toBe("chat.status.queued:0")
  })

  it("falls back to error or idle when nothing runs", () => {
    expect(statusText({ state: "idle" }, worker("w1", "Lin", "error"), t)).toBe("statuses.error")
    expect(statusText({ state: "idle" }, worker("w1", "Lin", "working"), t)).toBe("statuses.idle")
    expect(statusText({ state: "idle" }, undefined, t)).toBe("statuses.idle")
  })
})

describe("activeTasksOf", () => {
  const tasks = [{ id: "t1", instruction: "ship it", type: "immediate" }]

  it("reads the active tasks of a 409 clear error", () => {
    const error = new ApiError(409, "active tasks", undefined, undefined, { error: "active tasks", active_tasks: tasks })

    expect(activeTasksOf(error)).toEqual(tasks)
  })

  it("returns null for other errors", () => {
    expect(activeTasksOf(new ApiError(500, "boom", undefined, undefined, { active_tasks: tasks }))).toBeNull()
    expect(activeTasksOf(new ApiError(409, "conflict"))).toBeNull()
    expect(activeTasksOf(new ApiError(409, "conflict", undefined, undefined, { active_tasks: "x" }))).toBeNull()
    expect(activeTasksOf(new Error("network"))).toBeNull()
    expect(activeTasksOf(undefined)).toBeNull()
  })

  it("drops malformed entries", () => {
    const error = new ApiError(409, "active tasks", undefined, undefined, {
      active_tasks: [...tasks, { id: 2 }, null, "t3"],
    })

    expect(activeTasksOf(error)).toEqual(tasks)
  })
})

describe("taskTypeLabelKey", () => {
  it("labels only countdown and scheduled tasks", () => {
    expect(taskTypeLabelKey("countdown")).toBe("chat.taskType.countdown")
    expect(taskTypeLabelKey("scheduled")).toBe("chat.taskType.scheduled")
    expect(taskTypeLabelKey("immediate")).toBeNull()
  })
})

describe("instructionPreview", () => {
  it("keeps the first three non-blank lines", () => {
    expect(instructionPreview("  one\n\n two \nthree\nfour")).toBe("one\ntwo\nthree")
  })
})
