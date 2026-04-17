import { describe, expect, it } from "vitest"
import { KimiParser } from "../kimi-parser"
import type { ParsedEntry } from "../types"

function run(lines: string[], logType = "stdout"): ParsedEntry[] {
  const parser = new KimiParser()
  const entries: ParsedEntry[] = []
  const itemMap = new Map<string, number>()
  for (const line of lines) parser.parseLine(line, logType, entries, itemMap)
  return entries
}

describe("KimiParser", () => {
  it("skips role=user lines", () => {
    const line = JSON.stringify({ role: "user", content: "hello" })
    expect(run([line])).toHaveLength(0)
  })

  it("creates text entry for assistant with string content", () => {
    const line = JSON.stringify({ role: "assistant", content: "world" })
    const entries = run([line])
    expect(entries).toHaveLength(1)
    expect(entries[0]).toEqual({ kind: "text", text: "world" })
  })

  it("merges consecutive assistant text into one entry", () => {
    const l1 = JSON.stringify({ role: "assistant", content: "first" })
    const l2 = JSON.stringify({ role: "assistant", content: "second" })
    const entries = run([l1, l2])
    expect(entries).toHaveLength(1)
    expect(entries[0]).toEqual({ kind: "text", text: "first\n\nsecond" })
  })

  it("creates text entry for assistant with array content (text block)", () => {
    const line = JSON.stringify({
      role: "assistant",
      content: [{ type: "text", text: "array answer" }],
    })
    const entries = run([line])
    expect(entries).toHaveLength(1)
    expect(entries[0]).toEqual({ kind: "text", text: "array answer" })
  })

  it("skips non-text blocks in array content", () => {
    const line = JSON.stringify({
      role: "assistant",
      content: [{ type: "tool_use", id: "tc_1" }, { type: "text", text: "after tool" }],
    })
    const entries = run([line])
    expect(entries).toHaveLength(1)
    expect(entries[0]).toEqual({ kind: "text", text: "after tool" })
  })

  it("creates in-progress tool entry for each tool_call", () => {
    const line = JSON.stringify({
      role: "assistant",
      content: "calling",
      tool_calls: [
        { type: "function", id: "tc_1", function: { name: "Shell", arguments: '{"command":"ls"}' } },
      ],
    })
    const entries = run([line])
    expect(entries).toHaveLength(2)
    expect(entries[0]).toEqual({ kind: "text", text: "calling" })
    expect(entries[1]).toMatchObject({ kind: "tool", id: "tc_1", name: "Shell", result: undefined })
    expect((entries[1] as Extract<ParsedEntry, { kind: "tool" }>).input).toEqual({ command: "ls" })
  })

  it("parses tool_call input as JSON when possible", () => {
    const line = JSON.stringify({
      role: "assistant",
      content: "",
      tool_calls: [
        { type: "function", id: "tc_2", function: { name: "Read", arguments: '{"path":"/tmp/x"}' } },
      ],
    })
    const entries = run([line])
    const tool = entries.find((e) => e.kind === "tool") as Extract<ParsedEntry, { kind: "tool" }>
    expect(tool.input).toEqual({ path: "/tmp/x" })
  })

  it("uses raw string input when tool_call arguments are not valid JSON", () => {
    const line = JSON.stringify({
      role: "assistant",
      content: "",
      tool_calls: [
        { type: "function", id: "tc_3", function: { name: "Shell", arguments: "not-json" } },
      ],
    })
    const entries = run([line])
    const tool = entries.find((e) => e.kind === "tool") as Extract<ParsedEntry, { kind: "tool" }>
    expect(tool.input).toBe("not-json")
  })

  it("updates tool entry with result when role=tool arrives (string content)", () => {
    const assistantLine = JSON.stringify({
      role: "assistant",
      content: "doing it",
      tool_calls: [
        { type: "function", id: "tc_1", function: { name: "Shell", arguments: '{"command":"ls"}' } },
      ],
    })
    const toolLine = JSON.stringify({ role: "tool", tool_call_id: "tc_1", content: "file1\nfile2" })
    const entries = run([assistantLine, toolLine])
    const tool = entries.find((e) => e.kind === "tool") as Extract<ParsedEntry, { kind: "tool" }>
    expect(tool.result).toBe("file1\nfile2")
  })

  it("updates tool entry with result when role=tool arrives (array content blocks)", () => {
    const assistantLine = JSON.stringify({
      role: "assistant",
      content: "",
      tool_calls: [
        { type: "function", id: "tc_arr", function: { name: "Shell", arguments: '{"command":"free -h"}' } },
      ],
    })
    const toolLine = JSON.stringify({
      role: "tool",
      tool_call_id: "tc_arr",
      content: [
        { type: "text", text: "<system>Command executed successfully.</system>" },
        { type: "text", text: '{"total":"16G","used":"8G"}' },
      ],
    })
    const entries = run([assistantLine, toolLine])
    const tool = entries.find((e) => e.kind === "tool") as Extract<ParsedEntry, { kind: "tool" }>
    expect(tool.result).toBe('<system>Command executed successfully.</system>\n{"total":"16G","used":"8G"}')
  })

  it("ignores role=tool when tool_call_id not in itemMap", () => {
    const toolLine = JSON.stringify({ role: "tool", tool_call_id: "unknown", content: "result" })
    expect(run([toolLine])).toHaveLength(0)
  })

  it("emits raw entry for non-JSON lines", () => {
    const entries = run(["not json at all"])
    expect(entries).toHaveLength(1)
    expect(entries[0]).toMatchObject({ kind: "raw", content: "not json at all" })
  })

  it("emits raw entry for stderr lines", () => {
    const line = JSON.stringify({ role: "assistant", content: "x" })
    const entries = run([line], "stderr")
    expect(entries).toHaveLength(1)
    expect(entries[0]).toMatchObject({ kind: "raw", logType: "stderr" })
  })
})
