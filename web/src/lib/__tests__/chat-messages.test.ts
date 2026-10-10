import { describe, expect, it } from "vitest"
import type { ChatMessage } from "../types"
import { chatReducer, createMessageId, EMPTY_CHAT, type ChatState } from "../chat-messages"

const msg = (id: string, ts: number, extra: Partial<ChatMessage> = {}): ChatMessage => ({
  id,
  role: "user",
  content: id,
  ts,
  ...extra,
})

const live = (id: string, ts: number) => msg(id, ts, { pending: true })

const state = (messages: ChatMessage[], hasMore = false): ChatState => ({ messages, hasMore })

const ids = (s: ChatState) => s.messages.map((message) => message.id)

describe("chatReducer latest", () => {
  it("takes the first page as-is", () => {
    const next = chatReducer(EMPTY_CHAT, { type: "latest", messages: [msg("a", 10)], hasMore: true })

    expect(next).toEqual(state([msg("a", 10)], true))
  })

  it("keeps older history when the refreshed page overlaps it", () => {
    const prev = state([msg("a", 1), msg("b", 2), msg("c", 10), msg("d", 11)], true)

    const next = chatReducer(prev, {
      type: "latest",
      messages: [msg("c", 10), msg("d", 11), msg("e", 12)],
      hasMore: true,
    })

    expect(ids(next)).toEqual(["a", "b", "c", "d", "e"])
  })

  it("replaces an optimistic message with its stored copy despite clock skew", () => {
    const prev = state([msg("x", 1), msg("y", 2), live("u", 1000)])

    const next = chatReducer(prev, {
      type: "latest",
      messages: [msg("u", 1005), msg("r", 1010, { role: "bee" })],
      hasMore: true,
    })

    expect(next.messages).toEqual([msg("u", 1005), msg("r", 1010, { role: "bee" })])
  })

  it("drops disconnected older history instead of leaving a hidden gap", () => {
    const prev = state([msg("m1", 1), msg("m2", 2)])
    const latest = [msg("m101", 101), msg("m150", 150)]

    const next = chatReducer(prev, { type: "latest", messages: latest, hasMore: true })

    expect(next).toEqual(state(latest, true))
  })

  it("keeps live messages the snapshot has not stored yet", () => {
    const prev = state([msg("a", 10), msg("b", 11), live("reply", 12), live("sent", 13)])

    const next = chatReducer(prev, {
      type: "latest",
      messages: [msg("a", 10), msg("b", 11)],
      hasMore: false,
    })

    expect(ids(next)).toEqual(["a", "b", "reply", "sent"])
  })

  it("keeps live messages when the server has nothing stored yet", () => {
    const next = chatReducer(state([live("first", 5)]), { type: "latest", messages: [], hasMore: false })

    expect(ids(next)).toEqual(["first"])
  })

  it("drops stale live messages that precede a disconnected page", () => {
    const prev = state([msg("a", 1), live("lost", 2)])

    const next = chatReducer(prev, { type: "latest", messages: [msg("z", 50)], hasMore: true })

    expect(ids(next)).toEqual(["z"])
  })

  it("preserves hasMore from earlier pages when older history is kept", () => {
    const prev = state([msg("a", 1), msg("b", 2)], false)

    const next = chatReducer(prev, {
      type: "latest",
      messages: [msg("b", 2), msg("c", 3)],
      hasMore: true,
    })

    expect(next.hasMore).toBe(false)
  })

  it("adopts the server hasMore when the page starts the list", () => {
    const prev = state([msg("a", 1), msg("b", 2)], false)

    const next = chatReducer(prev, {
      type: "latest",
      messages: [msg("a", 1), msg("b", 2)],
      hasMore: true,
    })

    expect(next.hasMore).toBe(true)
  })
})

describe("chatReducer older", () => {
  it("prepends unseen messages and records hasMore", () => {
    const next = chatReducer(state([msg("c", 3)], true), {
      type: "older",
      messages: [msg("a", 1), msg("b", 2), msg("c", 3)],
      hasMore: false,
    })

    expect(next).toEqual(state([msg("a", 1), msg("b", 2), msg("c", 3)], false))
  })
})

describe("chatReducer live updates", () => {
  it("ignores a live message that is already shown", () => {
    const prev = state([msg("r", 1)])

    expect(chatReducer(prev, { type: "add", message: live("r", 1) })).toBe(prev)
  })

  it("adopts the server timestamp once a send is accepted", () => {
    const next = chatReducer(state([live("u", 1000)]), { type: "sent", id: "u", ts: 1005 })

    expect(next.messages).toEqual([live("u", 1005)])
  })

  it("removes a message whose send failed", () => {
    const next = chatReducer(state([msg("a", 1), live("u", 2)]), { type: "remove", id: "u" })

    expect(ids(next)).toEqual(["a"])
  })
})

describe("createMessageId", () => {
  it("returns distinct 32-character hex ids", () => {
    const a = createMessageId()

    expect(a).toMatch(/^[0-9a-f]{32}$/)
    expect(createMessageId()).not.toBe(a)
  })
})
