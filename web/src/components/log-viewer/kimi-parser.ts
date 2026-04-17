import type { ParsedEntry, StreamParser } from "./types"
import { appendRawEntry, appendTextEntry } from "./types"
import { extractToolResultText } from "./claude-parser"

interface KimiContentBlock {
  type: string
  text?: string
}

interface KimiToolCall {
  type: string
  id: string
  function: {
    name: string
    arguments: string
  }
}

interface KimiMessage {
  role: string
  content?: string | KimiContentBlock[]
  tool_call_id?: string
  tool_calls?: KimiToolCall[]
}

function parseToolInput(args: string): unknown {
  try {
    return JSON.parse(args)
  } catch {
    return args
  }
}

export class KimiParser implements StreamParser {
  parseLine(
    line: string,
    logType: string,
    entries: ParsedEntry[],
    itemMap: Map<string, number>
  ): void {
    if (logType !== "stdout") {
      appendRawEntry(line, logType, entries)
      return
    }

    let msg: KimiMessage
    try {
      const parsed = JSON.parse(line)
      if (!parsed || typeof parsed.role !== "string") {
        appendRawEntry(line, logType, entries)
        return
      }
      msg = parsed as KimiMessage
    } catch {
      appendRawEntry(line, logType, entries)
      return
    }

    switch (msg.role) {
      case "user":
        return

      case "assistant": {
        if (typeof msg.content === "string" && msg.content !== "") {
          appendTextEntry(msg.content, entries)
        } else if (Array.isArray(msg.content)) {
          for (const block of msg.content) {
            if (block.type === "text" && block.text?.trim()) {
              appendTextEntry(block.text, entries)
            }
          }
        }
        if (Array.isArray(msg.tool_calls)) {
          for (const tc of msg.tool_calls) {
            if (!tc.id || !tc.function?.name) continue
            const input = parseToolInput(tc.function.arguments ?? "")
            itemMap.set(tc.id, entries.length)
            entries.push({ kind: "tool", id: tc.id, name: tc.function.name, input, result: undefined })
          }
        }
        return
      }

      case "tool": {
        const { tool_call_id, content } = msg
        if (!tool_call_id) return
        const idx = itemMap.get(tool_call_id)
        if (idx === undefined) return
        const existing = entries[idx]
        if (existing?.kind !== "tool") return
        entries[idx] = { ...existing, result: extractToolResultText(content) }
        itemMap.delete(tool_call_id)
        return
      }

      default:
        appendRawEntry(line, logType, entries)
    }
  }
}
