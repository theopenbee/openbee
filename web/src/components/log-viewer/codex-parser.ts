import type { ParsedEntry, StreamParser } from "./types"
import { appendTextEntry, appendRawEntry, parseJsonEvent } from "./types"

interface CodexEvent {
  type: string
  thread_id?: string
  item?: {
    id?: string
    type?: string
    command?: string
    text?: string
    status?: string
  }
  usage?: {
    input_tokens?: number
    cached_input_tokens?: number
    output_tokens?: number
  }
}

export class CodexParser implements StreamParser {
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

    const event = parseJsonEvent<CodexEvent>(line)

    if (!event) {
      appendRawEntry(line, logType, entries)
      return
    }

    if (event.type === "thread.started" || event.type === "turn.started") {
      return
    }

    if (event.type === "item.started") {
      const item = event.item
      if (item?.type === "command_execution" && item.id && item.command) {
        itemMap.set(item.id, entries.length)
        entries.push({
          kind: "codex-command",
          id: item.id,
          command: item.command,
          inProgress: true,
        })
      }
      return
    }

    if (event.type === "item.completed") {
      const item = event.item
      if (!item) {
        appendRawEntry(line, logType, entries)
        return
      }

      if (item.type === "agent_message" && item.text) {
        appendTextEntry(item.text, entries)
        return
      }

      if (item.type === "command_execution" && item.id) {
        const idx = itemMap.get(item.id)
        if (idx !== undefined) {
          const existing = entries[idx]
          if (existing?.kind === "codex-command") {
            entries[idx] = { ...existing, inProgress: false, output: item.text ?? "" }
            itemMap.delete(item.id)
            return
          }
        }
        // id not found in map — fall through to raw
      }

      appendRawEntry(line, logType, entries)
      return
    }

    if (event.type === "turn.completed") {
      const usage = event.usage
      if (
        usage &&
        typeof usage.input_tokens === "number" &&
        typeof usage.cached_input_tokens === "number" &&
        typeof usage.output_tokens === "number"
      ) {
        entries.push({
          kind: "codex-turn",
          inputTokens: usage.input_tokens,
          cachedInputTokens: usage.cached_input_tokens,
          outputTokens: usage.output_tokens,
        })
      }
      return
    }

    appendRawEntry(line, logType, entries)
  }
}
