import { describe, expect, it } from "vitest"
import type { ChatMessage } from "../types"
import { mergeLatestPage, messageKeys } from "../chat-messages"

const msg = (ts: number, role: ChatMessage["role"] = "user", content = String(ts)): ChatMessage => ({
  role,
  content,
  ts,
})

describe("messageKeys", () => {
  it("keeps existing keys stable when older messages are prepended", () => {
    const latest = [msg(30), msg(40, "bee")]
    const before = messageKeys(latest)
    const after = messageKeys([msg(10), msg(20, "bee"), ...latest])

    expect(after.slice(2)).toEqual(before)
  })

  it("disambiguates messages that share role and timestamp", () => {
    expect(messageKeys([msg(5, "bee"), msg(5, "bee"), msg(5, "user")])).toEqual([
      "bee-5",
      "bee-5-1",
      "user-5",
    ])
  })
})

describe("mergeLatestPage", () => {
  it("keeps previously loaded older messages ahead of the refreshed page", () => {
    const older = [msg(1), msg(2, "bee")]
    const prev = [...older, msg(10), msg(11, "bee")]
    const latest = [msg(10), msg(11, "bee"), msg(12)]

    expect(mergeLatestPage(prev, latest)).toEqual([...older, ...latest])
  })

  it("keeps messages that scrolled out of the latest page window", () => {
    const prev = [msg(1), msg(10), msg(11)]
    const latest = [msg(11), msg(12)]

    expect(mergeLatestPage(prev, latest)).toEqual([msg(1), msg(10), msg(11), msg(12)])
  })

  it("drops optimistic messages that the refreshed page replaces", () => {
    const prev = [msg(10), msg(11), msg(9, "user", "optimistic")]
    const latest = [msg(10), msg(11), msg(12)]

    expect(mergeLatestPage(prev, latest)).toEqual(latest)
  })

  it("returns the latest page as-is on first load", () => {
    const latest = [msg(10)]

    expect(mergeLatestPage([], latest)).toBe(latest)
  })

  it("clears messages when the server returns an empty page", () => {
    expect(mergeLatestPage([msg(1)], [])).toEqual([])
  })
})
