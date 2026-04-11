import type { ParsedEntry, StreamParser } from "./types"
import { appendRawEntry, parseJsonEvent } from "./types"

interface PiContentBlock {
  type: string
  text?: string
  thinking?: string
  id?: string
  name?: string
  input?: unknown
}

interface PiMessage {
  role?: string
  content?: PiContentBlock[]
}

interface PiToolResult {
  content?: Array<{ type: string; text?: string }>
}

interface PiEvent {
  type: string
  message?: PiMessage
  toolCallId?: string
  toolName?: string
  args?: unknown
  result?: PiToolResult
  isError?: boolean
  attempt?: number
  maxAttempts?: number
  finalError?: string
  success?: boolean
}

export class PiParser implements StreamParser {
  private messageCount = 0

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

    const event = parseJsonEvent<PiEvent>(line)

    if (!event) {
      appendRawEntry(line, logType, entries)
      return
    }

    switch (event.type) {
      case "agent_start":
      case "agent_end":
      case "turn_start":
      case "turn_end":
      case "message_start":
      case "message_update":
      case "queue_update":
      case "compaction_start":
      case "compaction_end":
        return

      case "message_end": {
        const messageIndex = this.messageCount++
        const content = event.message?.content
        if (!Array.isArray(content)) return
        content.forEach((block, blockIndex) => {
          if (block.type === "thinking" && block.thinking) {
            entries.push({
              kind: "pi-thinking",
              id: `thinking-${messageIndex}-${blockIndex}`,
              thinking: block.thinking,
            })
          } else if (block.type === "text" && block.text) {
            entries.push({ kind: "text", text: block.text })
          }
          // tool_use blocks are ignored — handled by tool_execution events
        })
        return
      }

      case "tool_execution_start": {
        const { toolCallId, toolName, args } = event
        if (!toolCallId || !toolName) {
          appendRawEntry(line, logType, entries)
          return
        }
        itemMap.set(toolCallId, entries.length)
        entries.push({ kind: "tool", id: toolCallId, name: toolName, input: args ?? {}, result: undefined })
        return
      }

      case "tool_execution_end": {
        const { toolCallId, result, isError } = event
        if (!toolCallId) {
          appendRawEntry(line, logType, entries)
          return
        }
        const idx = itemMap.get(toolCallId)
        if (idx === undefined) return
        const existing = entries[idx]
        if (existing?.kind !== "tool") return
        const textBlock = result?.content?.find((b) => b.type === "text")
        entries[idx] = {
          ...existing,
          result: textBlock?.text ?? "",
          isError: Boolean(isError),
        }
        itemMap.delete(toolCallId)
        return
      }

      case "auto_retry_start": {
        const attempt = event.attempt ?? "?"
        const max = event.maxAttempts ?? "?"
        appendRawEntry(`Retrying (attempt ${attempt}/${max})...`, logType, entries)
        return
      }

      case "auto_retry_end": {
        if (!event.success && event.finalError) {
          appendRawEntry(`Retry failed: ${event.finalError}`, logType, entries)
        }
        return
      }

      default:
        appendRawEntry(line, logType, entries)
    }
  }
}
