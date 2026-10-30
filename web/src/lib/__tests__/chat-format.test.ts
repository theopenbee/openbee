import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"
import { formatMessageTimestamp } from "../chat-format"

describe("formatMessageTimestamp", () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date(2026, 9, 27, 15, 30))
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it("returns an empty string without a timestamp", () => {
    expect(formatMessageTimestamp(undefined, "en")).toBe("")
    expect(formatMessageTimestamp(null, "en")).toBe("")
    expect(formatMessageTimestamp(0, "en")).toBe("")
  })

  it("shows only the time for today", () => {
    const today = new Date(2026, 9, 27, 9, 5).getTime()
    expect(formatMessageTimestamp(today, "en")).toBe("09:05")
  })

  it("uses a 24-hour clock in every language", () => {
    const afternoon = new Date(2026, 9, 27, 14, 32).getTime()
    expect(formatMessageTimestamp(afternoon, "en")).toBe("14:32")
    expect(formatMessageTimestamp(afternoon, "zh")).toBe("14:32")
  })

  it("adds the date for other days", () => {
    const earlier = new Date(2026, 9, 20, 9, 5).getTime()
    expect(formatMessageTimestamp(earlier, "en")).toBe("Oct 20, 09:05")
  })
})
