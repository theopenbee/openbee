import { describe, expect, it } from "vitest"
import { PiParser } from "../pi-parser"
import type { ParsedEntry } from "../types"

function run(lines: string[], logType = "stdout"): ParsedEntry[] {
  const parser = new PiParser()
  const entries: ParsedEntry[] = []
  const itemMap = new Map<string, number>()
  for (const line of lines) parser.parseLine(line, logType, entries, itemMap)
  return entries
}

describe("PiParser", () => {
  // --- ignored events ---

  it("ignores agent_start", () => {
    expect(run([JSON.stringify({ type: "agent_start" })])).toHaveLength(0)
  })

  it("ignores agent_end", () => {
    expect(run([JSON.stringify({ type: "agent_end", messages: [] })])).toHaveLength(0)
  })

  it("ignores turn_start", () => {
    expect(run([JSON.stringify({ type: "turn_start" })])).toHaveLength(0)
  })

  it("ignores turn_end", () => {
    expect(run([JSON.stringify({ type: "turn_end", message: {}, toolResults: [] })])).toHaveLength(0)
  })

  it("ignores message_start", () => {
    expect(run([JSON.stringify({ type: "message_start", message: {} })])).toHaveLength(0)
  })

  it("ignores message_update", () => {
    expect(run([JSON.stringify({ type: "message_update", message: {}, assistantMessageEvent: { type: "text_delta", delta: "hi" } })])).toHaveLength(0)
  })

  it("ignores tool_execution_update", () => {
    expect(run([JSON.stringify({ type: "tool_execution_update", toolCallId: "call_abc", toolName: "bash", args: { command: "ls" }, partialResult: { content: [] } })])).toHaveLength(0)
  })

  it("ignores queue_update", () => {
    expect(run([JSON.stringify({ type: "queue_update", steering: [], followUp: [] })])).toHaveLength(0)
  })

  it("ignores compaction_start", () => {
    expect(run([JSON.stringify({ type: "compaction_start", reason: "threshold" })])).toHaveLength(0)
  })

  it("ignores compaction_end", () => {
    expect(run([JSON.stringify({ type: "compaction_end", reason: "threshold", aborted: false, willRetry: false })])).toHaveLength(0)
  })

  // --- message_end ---

  it("extracts text block from message_end as text entry", () => {
    const line = JSON.stringify({
      type: "message_end",
      message: {
        role: "assistant",
        content: [{ type: "text", text: "Hello world" }],
      },
    })
    const entries = run([line])
    expect(entries).toHaveLength(1)
    expect(entries[0]).toMatchObject({ kind: "text", text: "Hello world" })
  })

  it("extracts thinking block from message_end as pi-thinking entry", () => {
    const line = JSON.stringify({
      type: "message_end",
      message: {
        role: "assistant",
        content: [{ type: "thinking", thinking: "let me reason..." }],
      },
    })
    const entries = run([line])
    expect(entries).toHaveLength(1)
    expect(entries[0]).toMatchObject({ kind: "pi-thinking", thinking: "let me reason..." })
    expect((entries[0] as Extract<typeof entries[0], { kind: "pi-thinking" }>).id).toBe("thinking-0-0")
  })

  it("extracts both thinking and text blocks from the same message_end in order", () => {
    const line = JSON.stringify({
      type: "message_end",
      message: {
        role: "assistant",
        content: [
          { type: "thinking", thinking: "step by step..." },
          { type: "text", text: "Done." },
        ],
      },
    })
    const entries = run([line])
    expect(entries).toHaveLength(2)
    expect(entries[0]).toMatchObject({ kind: "pi-thinking", thinking: "step by step..." })
    expect(entries[1]).toMatchObject({ kind: "text", text: "Done." })
  })

  it("ignores tool_use blocks in message_end (handled by tool_execution events)", () => {
    const line = JSON.stringify({
      type: "message_end",
      message: {
        role: "assistant",
        content: [
          { type: "tool_use", id: "call_1", name: "bash", input: { command: "ls" } },
        ],
      },
    })
    expect(run([line])).toHaveLength(0)
  })

  it("assigns sequential thinking IDs across multiple message_end events", () => {
    const msg0 = JSON.stringify({
      type: "message_end",
      message: { role: "assistant", content: [{ type: "thinking", thinking: "first" }] },
    })
    const msg1 = JSON.stringify({
      type: "message_end",
      message: { role: "assistant", content: [{ type: "thinking", thinking: "second" }] },
    })
    const entries = run([msg0, msg1])
    expect(entries).toHaveLength(2)
    const ids = entries.map((e) => (e as Extract<typeof e, { kind: "pi-thinking" }>).id)
    expect(ids[0]).toBe("thinking-0-0")
    expect(ids[1]).toBe("thinking-1-0")
  })

  it("skips message_end when message has no content array", () => {
    const line = JSON.stringify({ type: "message_end", message: { role: "assistant" } })
    expect(run([line])).toHaveLength(0)
  })

  it("skips message_end for user role (avoids showing raw message_meta XML)", () => {
    const line = JSON.stringify({
      type: "message_end",
      message: {
        role: "user",
        content: [{ type: "text", text: "<message_meta>{}</message_meta>\n<message_content>\nhello\n</message_content>\n" }],
      },
    })
    expect(run([line])).toHaveLength(0)
  })

  it("skips message_end for toolResult role", () => {
    const line = JSON.stringify({
      type: "message_end",
      message: {
        role: "toolResult",
        toolCallId: "call_abc",
        content: [{ type: "text", text: "some result" }],
      },
    })
    expect(run([line])).toHaveLength(0)
  })

  // --- tool_execution_start ---

  it("creates in-progress tool entry for tool_execution_start", () => {
    const line = JSON.stringify({
      type: "tool_execution_start",
      toolCallId: "call_abc",
      toolName: "bash",
      args: { command: "ls -la" },
    })
    const entries = run([line])
    expect(entries).toHaveLength(1)
    expect(entries[0]).toMatchObject({
      kind: "tool",
      id: "call_abc",
      name: "bash",
      input: { command: "ls -la" },
      result: undefined,
    })
  })

  // --- tool_execution_end ---

  it("updates tool entry with result on tool_execution_end", () => {
    const startLine = JSON.stringify({
      type: "tool_execution_start",
      toolCallId: "call_abc",
      toolName: "bash",
      args: { command: "ls" },
    })
    const endLine = JSON.stringify({
      type: "tool_execution_end",
      toolCallId: "call_abc",
      toolName: "bash",
      result: { content: [{ type: "text", text: "file1\nfile2" }] },
      isError: false,
    })
    const entries = run([startLine, endLine])
    expect(entries).toHaveLength(1)
    expect(entries[0]).toMatchObject({
      kind: "tool",
      id: "call_abc",
      result: "file1\nfile2",
      isError: false,
    })
  })

  it("marks tool entry as error when isError is true", () => {
    const startLine = JSON.stringify({
      type: "tool_execution_start",
      toolCallId: "call_err",
      toolName: "bash",
      args: { command: "bad" },
    })
    const endLine = JSON.stringify({
      type: "tool_execution_end",
      toolCallId: "call_err",
      toolName: "bash",
      result: { content: [{ type: "text", text: "error output" }] },
      isError: true,
    })
    const entries = run([startLine, endLine])
    expect(entries[0]).toMatchObject({ kind: "tool", isError: true, result: "error output" })
  })

  it("silently ignores tool_execution_end for unknown toolCallId", () => {
    const line = JSON.stringify({
      type: "tool_execution_end",
      toolCallId: "call_unknown",
      toolName: "bash",
      result: { content: [{ type: "text", text: "out" }] },
      isError: false,
    })
    expect(run([line])).toHaveLength(0)
  })

  it("falls back to empty string result when content array is empty", () => {
    const startLine = JSON.stringify({
      type: "tool_execution_start",
      toolCallId: "call_empty",
      toolName: "bash",
      args: {},
    })
    const endLine = JSON.stringify({
      type: "tool_execution_end",
      toolCallId: "call_empty",
      toolName: "bash",
      result: { content: [] },
      isError: false,
    })
    const entries = run([startLine, endLine])
    expect(entries[0]).toMatchObject({ kind: "tool", result: "" })
  })

  // --- auto_retry ---

  it("creates raw entry for auto_retry_start", () => {
    const line = JSON.stringify({
      type: "auto_retry_start",
      attempt: 1,
      maxAttempts: 3,
      delayMs: 2000,
      errorMessage: "overloaded",
    })
    const entries = run([line])
    expect(entries).toHaveLength(1)
    expect(entries[0]).toMatchObject({ kind: "raw" })
    expect((entries[0] as Extract<typeof entries[0], { kind: "raw" }>).content).toContain("Retrying")
    expect((entries[0] as Extract<typeof entries[0], { kind: "raw" }>).content).toContain("1/3")
  })

  it("ignores auto_retry_end when success is true", () => {
    const line = JSON.stringify({ type: "auto_retry_end", success: true, attempt: 2 })
    expect(run([line])).toHaveLength(0)
  })

  it("creates raw entry for auto_retry_end when success is false", () => {
    const line = JSON.stringify({
      type: "auto_retry_end",
      success: false,
      attempt: 3,
      finalError: "overloaded_error: Overloaded",
    })
    const entries = run([line])
    expect(entries).toHaveLength(1)
    expect(entries[0]).toMatchObject({ kind: "raw" })
    expect((entries[0] as Extract<typeof entries[0], { kind: "raw" }>).content).toContain("overloaded_error")
  })

  // --- extension_error ---

  it("creates raw entry for extension_error", () => {
    const line = JSON.stringify({
      type: "extension_error",
      extensionPath: "/path/ext.ts",
      event: "tool_call",
      error: "boom",
    })
    const entries = run([line])
    expect(entries).toHaveLength(1)
    expect(entries[0]).toMatchObject({ kind: "raw" })
  })

  // --- fallbacks ---

  it("creates raw entry for unknown event type", () => {
    const line = JSON.stringify({ type: "unknown.event" })
    const entries = run([line])
    expect(entries).toHaveLength(1)
    expect(entries[0]).toMatchObject({ kind: "raw", logType: "stdout" })
  })

  it("creates raw entry for malformed JSON", () => {
    const entries = run(["not valid json"])
    expect(entries).toHaveLength(1)
    expect(entries[0]).toMatchObject({ kind: "raw", content: "not valid json" })
  })

  it("passes non-stdout lines through as raw", () => {
    const entries = run([JSON.stringify({ type: "agent_start" })], "stderr")
    expect(entries).toHaveLength(1)
    expect(entries[0]).toMatchObject({ kind: "raw", logType: "stderr" })
  })
})
