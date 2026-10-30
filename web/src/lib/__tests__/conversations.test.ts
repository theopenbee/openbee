import { describe, expect, it } from "vitest"
import type { ConversationSummary, Worker } from "../types"
import {
  BEE_CONVERSATION,
  conversationPath,
  filterConversations,
  previewText,
  sortConversations,
  workerConversationId,
  workerIdOf,
} from "../conversations"

const summary = (id: ConversationSummary["id"], ts?: number): ConversationSummary => ({
  id,
  kind: id === "bee" ? "bee" : "worker",
  worker_id: id === "bee" ? undefined : id.slice("worker:".length),
  last_message: ts === undefined ? undefined : { sender: { type: "user" }, kind: "message", preview: "hi", ts },
  unread: 0,
  activity: { state: "idle" },
})

const worker = (id: string, name: string, description: string): Worker => ({
  id,
  name,
  description,
  constraints: "",
  work_dir: "",
  engine: "claude",
  status: "idle",
  created_at: 0,
  updated_at: 0,
})

describe("sortConversations", () => {
  it("puts bee first then recent then by name", () => {
    const list = [
      summary("worker:w-zed"),
      summary("worker:w-old", 10),
      summary("bee"),
      summary("worker:w-alpha"),
      summary("worker:w-new", 20),
    ]
    const names = new Map([
      ["w-zed", "Zed"],
      ["w-old", "Old"],
      ["w-alpha", "Alpha"],
      ["w-new", "New"],
    ])

    const sorted = sortConversations(list, names).map((item) => item.id)

    expect(sorted).toEqual(["bee", "worker:w-new", "worker:w-old", "worker:w-alpha", "worker:w-zed"])
  })

  it("puts bee first even when a worker has a newer message", () => {
    const sorted = sortConversations([summary("worker:w1", 99), summary("bee", 1)], new Map())

    expect(sorted.map((item) => item.id)).toEqual(["bee", "worker:w1"])
  })

  it("does not mutate the input list", () => {
    const list = [summary("worker:w1"), summary("bee")]

    sortConversations(list, new Map())

    expect(list.map((item) => item.id)).toEqual(["worker:w1", "bee"])
  })
})

describe("conversation ids", () => {
  it("builds worker ids and reads them back", () => {
    expect(BEE_CONVERSATION).toBe("bee")
    expect(workerConversationId("w1")).toBe("worker:w1")
    expect(workerIdOf("worker:w1")).toBe("w1")
    expect(workerIdOf("bee")).toBeNull()
  })

  it("builds the chat path", () => {
    expect(conversationPath("bee")).toBe("/chat/bee")
    expect(conversationPath("worker:w1")).toBe("/chat/w1")
  })
})

describe("filterConversations", () => {
  const workers = new Map([
    ["w1", worker("w1", "Researcher", "Finds papers")],
    ["w2", worker("w2", "Coder", "Writes Go services")],
  ])
  const list = [summary("bee"), summary("worker:w1"), summary("worker:w2")]

  it("keeps bee and matches description", () => {
    const filtered = filterConversations(list, "papers", workers).map((item) => item.id)

    expect(filtered).toEqual(["bee", "worker:w1"])
  })

  it("matches the name case-insensitively", () => {
    const filtered = filterConversations(list, "CODER", workers).map((item) => item.id)

    expect(filtered).toEqual(["bee", "worker:w2"])
  })

  it("returns the list unchanged for an empty query", () => {
    expect(filterConversations(list, "", workers)).toBe(list)
    expect(filterConversations(list, "   ", workers)).toBe(list)
  })

  it("drops workers that are not loaded", () => {
    expect(filterConversations(list, "papers", new Map()).map((item) => item.id)).toEqual(["bee"])
  })
})

describe("previewText", () => {
  const t = (key: string, options?: Record<string, string>) =>
    options ? `${key}(${Object.values(options).join(",")})` : key
  const workers = new Map([["w1", worker("w1", "Researcher", "")]])
  const last = (extra: Partial<NonNullable<ConversationSummary["last_message"]>>) => ({
    sender: { type: "bee" as const },
    kind: "message" as const,
    preview: "write the report",
    ts: 1,
    ...extra,
  })

  it("prefixes the user's own messages", () => {
    expect(previewText(last({ sender: { type: "user" } }), workers, t)).toBe("chat.youPrefixwrite the report")
  })

  it("shows other senders' preview as is", () => {
    expect(previewText(last({ sender: { type: "worker", id: "w1" } }), workers, t)).toBe("write the report")
  })

  it("names the handoff target", () => {
    expect(previewText(last({ kind: "handoff", handoff_worker_id: "w1" }), workers, t)).toBe(
      "chat.handedOffTo(Researcher)",
    )
  })

  it("marks a handoff to a deleted worker", () => {
    expect(previewText(last({ kind: "handoff", handoff_worker_id: "w9" }), workers, t)).toBe(
      "chat.handedOffTo(chat.deletedWorker)",
    )
  })

  it("falls back to the preview when the handoff target is missing", () => {
    expect(previewText(last({ kind: "handoff" }), workers, t)).toBe("write the report")
  })

  it("labels a media-only message by its attachments", () => {
    expect(previewText(last({ preview: "", media_paths: ["a.png", "b.JPG"] }), workers, t)).toBe("chat.imagePreview")
    expect(previewText(last({ preview: "", media_paths: ["a.png", "report.pdf"] }), workers, t)).toBe(
      "chat.filePreview",
    )
    expect(previewText(last({ preview: "", media_paths: ["a.png"], sender: { type: "user" } }), workers, t)).toBe(
      "chat.youPrefixchat.imagePreview",
    )
  })

  it("keeps the text when a message has both text and media", () => {
    expect(previewText(last({ media_paths: ["a.png"] }), workers, t)).toBe("write the report")
  })
})
