import { describe, expect, it } from "vitest"
import { extractMessageContent, formatTotalDuration } from "../format"

describe("extractMessageContent", () => {
  it("returns input unchanged when no known format is detected", () => {
    expect(extractMessageContent("hello world")).toBe("hello world")
  })

  it("returns empty string unchanged", () => {
    expect(extractMessageContent("")).toBe("")
  })

  it("strips old frontmatter format", () => {
    const input = "---\nmessage_id: abc123\n---\n\nThis is the content"
    expect(extractMessageContent(input)).toBe("This is the content")
  })

  it("strips old frontmatter format without trailing blank line", () => {
    const input = "---\nmessage_id: abc123\n---\nThis is the content"
    expect(extractMessageContent(input)).toBe("This is the content")
  })

  it("extracts content from new format with message_content tag", () => {
    const input = `<message_meta>{"from":"feishu","message_id":"abc"}</message_meta>\n<message_content>\nhello world\n</message_content>`
    expect(extractMessageContent(input)).toBe("hello world")
  })

  it("extracts content from new format with task_content tag", () => {
    const input = `<message_meta>{"from":"feishu","message_id":"abc"}</message_meta>\n<task_content>\ndo something\n</task_content>`
    expect(extractMessageContent(input)).toBe("do something")
  })

  it("preserves nested content inside task_content as-is", () => {
    const input = `<message_meta>{}</message_meta>\n<task_content>\n<worker_persona>X</worker_persona>\nsome task\n</task_content>`
    expect(extractMessageContent(input)).toBe("<worker_persona>X</worker_persona>\nsome task")
  })
})

describe("formatTotalDuration", () => {
  it("returns 0s for zero milliseconds", () => {
    expect(formatTotalDuration(0)).toBe("0s")
  })

  it("formats sub-minute durations as seconds", () => {
    expect(formatTotalDuration(45_000)).toBe("45s")
    expect(formatTotalDuration(1_000)).toBe("1s")
    expect(formatTotalDuration(59_999)).toBe("59s")
  })

  it("formats minute-range durations as m s", () => {
    expect(formatTotalDuration(90_000)).toBe("1m 30s")
    expect(formatTotalDuration(750_000)).toBe("12m 30s")
    expect(formatTotalDuration(3_599_999)).toBe("59m 59s")
  })

  it("formats hour-range durations as h m", () => {
    expect(formatTotalDuration(3_600_000)).toBe("1h 0m")
    expect(formatTotalDuration(8_100_000)).toBe("2h 15m")
    expect(formatTotalDuration(86_399_999)).toBe("23h 59m")
  })

  it("formats day-range durations as d h", () => {
    expect(formatTotalDuration(86_400_000)).toBe("1d 0h")
    expect(formatTotalDuration(97_200_000)).toBe("1d 3h")
  })
})
