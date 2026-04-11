import { describe, expect, it } from "vitest"
import { ClaudeParser } from "../claude-parser"
import type { ParsedEntry } from "../types"

function run(lines: string[]): ParsedEntry[] {
  const parser = new ClaudeParser()
  const entries: ParsedEntry[] = []
  const itemMap = new Map<string, number>()
  for (const line of lines) parser.parseLine(line, "stdout", entries, itemMap)
  return entries
}

describe("ClaudeParser", () => {
  it("parses assistant text block", () => {
    const line = JSON.stringify({
      type: "assistant",
      message: { content: [{ type: "text", text: "Hello world" }] },
    })
    const entries = run([line])
    expect(entries).toHaveLength(1)
    expect(entries[0]).toMatchObject({ kind: "text", text: "Hello world" })
  })

  it("parses tool_use block", () => {
    const line = JSON.stringify({
      type: "assistant",
      message: {
        content: [{ type: "tool_use", id: "t1", name: "Bash", input: { command: "ls" } }],
      },
    })
    const entries = run([line])
    expect(entries).toHaveLength(1)
    expect(entries[0]).toMatchObject({ kind: "tool", id: "t1", name: "Bash" })
  })

  it("fills tool result via user event", () => {
    const assistantLine = JSON.stringify({
      type: "assistant",
      message: { content: [{ type: "tool_use", id: "t1", name: "Bash", input: {} }] },
    })
    const userLine = JSON.stringify({
      type: "user",
      message: {
        content: [{ type: "tool_result", tool_use_id: "t1", content: "output text" }],
      },
    })
    const entries = run([assistantLine, userLine])
    expect(entries).toHaveLength(1)
    expect(entries[0]).toMatchObject({ kind: "tool", id: "t1", result: "output text" })
  })

  it("parses result event", () => {
    const line = JSON.stringify({ type: "result", result: "done", subtype: "success" })
    const entries = run([line])
    expect(entries).toHaveLength(1)
    expect(entries[0]).toMatchObject({ kind: "result", text: "done", subtype: "success" })
  })

  it("ignores system events", () => {
    const line = JSON.stringify({ type: "system", data: "init" })
    const entries = run([line])
    expect(entries).toHaveLength(0)
  })

  it("ignores rate_limit_event", () => {
    const line = JSON.stringify({ type: "rate_limit_event" })
    const entries = run([line])
    expect(entries).toHaveLength(0)
  })

  it("falls back to raw for unknown events", () => {
    const line = JSON.stringify({ type: "unknown_type" })
    const entries = run([line])
    expect(entries).toHaveLength(1)
    expect(entries[0]).toMatchObject({ kind: "raw", logType: "stdout" })
  })
})
