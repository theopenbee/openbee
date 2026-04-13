import { describe, expect, it } from "vitest"
import { extractMessageContent } from "../format"

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
