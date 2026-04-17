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

/**
 * Extracts the heredoc stdin content from an `openbee ctl message send --stdin` shell command.
 * Returns null if the command is not a message-send call or has no heredoc.
 */
function extractSentMessage(command: string): string | null {
  if (!command.includes("openbee ctl message send") || !command.includes("--stdin")) return null
  const match = command.match(/<<\s*['"]?EOF['"]?\n([\s\S]*?)\nEOF/)
  return match ? match[1] : null
}

export class KimiParser implements StreamParser {
  /** Stdin content from the last `openbee ctl message send --stdin` Shell call, if any. */
  private pendingSentMessage: string | null = null

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
            if (block.type !== "text" || !block.text?.trim()) continue
            if (block.text.startsWith("(Empty response:")) {
              // The actual response was already sent via `openbee ctl message send --stdin`.
              // Render that content instead of the placeholder.
              if (this.pendingSentMessage) {
                appendTextEntry(this.pendingSentMessage, entries)
                this.pendingSentMessage = null
              }
            } else {
              appendTextEntry(block.text, entries)
            }
          }
        }
        if (Array.isArray(msg.tool_calls)) {
          for (const tc of msg.tool_calls) {
            if (!tc.id || !tc.function?.name) continue
            const input = parseToolInput(tc.function.arguments ?? "")
            if (tc.function.name === "Shell") {
              const command = (input as Record<string, string>)?.command ?? ""
              const sent = extractSentMessage(command)
              if (sent) this.pendingSentMessage = sent
            }
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
