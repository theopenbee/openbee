import { describe, expect, it } from "vitest"
import { CodexParser } from "../codex-parser"
import type { ParsedEntry } from "../types"

function run(lines: string[]): ParsedEntry[] {
  const parser = new CodexParser()
  const entries: ParsedEntry[] = []
  const itemMap = new Map<string, number>()
  for (const line of lines) parser.parseLine(line, "stdout", entries, itemMap)
  return entries
}

describe("CodexParser", () => {
  it("ignores thread.started", () => {
    const line = JSON.stringify({ type: "thread.started", thread_id: "abc" })
    expect(run([line])).toHaveLength(0)
  })

  it("ignores turn.started", () => {
    const line = JSON.stringify({ type: "turn.started" })
    expect(run([line])).toHaveLength(0)
  })

  it("creates in-progress codex-command entry for item.started command_execution", () => {
    const line = JSON.stringify({
      type: "item.started",
      item: { id: "item_1", type: "command_execution", command: "ls -la", status: "in_progress" },
    })
    const entries = run([line])
    expect(entries).toHaveLength(1)
    expect(entries[0]).toMatchObject({
      kind: "codex-command",
      id: "item_1",
      command: "ls -la",
      inProgress: true,
    })
  })

  it("updates codex-command entry when item.completed command_execution arrives", () => {
    const startLine = JSON.stringify({
      type: "item.started",
      item: { id: "item_1", type: "command_execution", command: "ls -la", status: "in_progress" },
    })
    const completeLine = JSON.stringify({
      type: "item.completed",
      item: { id: "item_1", type: "command_execution", text: "file1\nfile2" },
    })
    const entries = run([startLine, completeLine])
    expect(entries).toHaveLength(1)
    expect(entries[0]).toMatchObject({
      kind: "codex-command",
      id: "item_1",
      command: "ls -la",
      inProgress: false,
      output: "file1\nfile2",
    })
  })

  it("creates text entry for item.completed agent_message", () => {
    const line = JSON.stringify({
      type: "item.completed",
      item: { id: "item_3", type: "agent_message", text: "Task done." },
    })
    const entries = run([line])
    expect(entries).toHaveLength(1)
    expect(entries[0]).toMatchObject({ kind: "text", text: "Task done." })
  })

  it("falls back to raw for item.completed with unknown item type", () => {
    const line = JSON.stringify({
      type: "item.completed",
      item: { id: "item_5", type: "function_call", text: "result" },
    })
    const entries = run([line])
    expect(entries).toHaveLength(1)
    expect(entries[0]).toMatchObject({ kind: "raw", logType: "stdout" })
  })

  it("creates codex-turn entry for turn.completed with usage", () => {
    const line = JSON.stringify({
      type: "turn.completed",
      usage: { input_tokens: 100, cached_input_tokens: 50, output_tokens: 20 },
    })
    const entries = run([line])
    expect(entries).toHaveLength(1)
    expect(entries[0]).toMatchObject({
      kind: "codex-turn",
      inputTokens: 100,
      cachedInputTokens: 50,
      outputTokens: 20,
    })
  })

  it("skips codex-turn when turn.completed has no usage", () => {
    const line = JSON.stringify({ type: "turn.completed" })
    expect(run([line])).toHaveLength(0)
  })

  it("falls back to raw for completely unknown event type", () => {
    const line = JSON.stringify({ type: "weird.event" })
    const entries = run([line])
    expect(entries).toHaveLength(1)
    expect(entries[0]).toMatchObject({ kind: "raw", logType: "stdout" })
  })

  it("falls back to raw when JSON parse fails", () => {
    const entries = run(["not valid json"])
    expect(entries).toHaveLength(1)
    expect(entries[0]).toMatchObject({ kind: "raw", content: "not valid json" })
  })
})
