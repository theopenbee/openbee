import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"
import { attachmentName, formatMessageTimestamp } from "../chat-format"

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

describe("attachmentName", () => {
  it("drops the stored id prefix", () => {
    expect(attachmentName("26d63ea6-656b-433d-9553-6ffa5da48555_markdown-demo.md")).toBe("markdown-demo.md")
    expect(attachmentName("uploads/26d63ea6-656b-433d-9553-6ffa5da48555_chart.png")).toBe("chart.png")
  })

  it("drops only the first prefix", () => {
    const id = "26d63ea6-656b-433d-9553-6ffa5da48555"
    expect(attachmentName(`${id}_${id}_notes.txt`)).toBe(`${id}_notes.txt`)
  })

  it("keeps names without a stored id prefix", () => {
    expect(attachmentName("report.pdf")).toBe("report.pdf")
    expect(attachmentName("2026-11-10_report.pdf")).toBe("2026-11-10_report.pdf")
    expect(attachmentName("out-1_chart.png")).toBe("out-1_chart.png")
  })

  it("keeps the stored name when nothing follows the prefix", () => {
    expect(attachmentName("26d63ea6-656b-433d-9553-6ffa5da48555_")).toBe("26d63ea6-656b-433d-9553-6ffa5da48555_")
  })
})
