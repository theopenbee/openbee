import type { ParsedEntry, StreamParser } from "./types"
import { appendTextEntry, appendRawEntry, parseJsonEvent } from "./types"

interface ClaudeStreamEvent {
  type: string
  subtype?: string
  message?: {
    content: Array<{
      type: string
      text?: string
      id?: string
      name?: string
      input?: unknown
      tool_use_id?: string
      content?: unknown
      is_error?: boolean
    }>
  }
  result?: string
}

export function stringify(value: unknown): string {
  if (typeof value === "string") return value
  try {
    return JSON.stringify(value, null, 2)
  } catch {
    return String(value)
  }
}

export function getToolMeta(name: string): {
  label: string
  summary: (input: unknown) => string
} {
  switch (name) {
    case "Bash":
      return {
        label: "SH",
        summary: (input: unknown) =>
          (input as { command?: string; cmd?: string })?.command ??
          (input as { command?: string; cmd?: string })?.cmd ??
          stringify(input),
      }
    case "Read":
    case "Write":
    case "Edit":
    case "Glob":
    case "Grep":
      return {
        label: "FS",
        summary: (input: unknown) => {
          const record = input as Record<string, string>
          return record?.file_path ?? record?.path ?? record?.pattern ?? record?.glob ?? stringify(input)
        },
      }
    case "WebSearch":
    case "WebFetch":
      return {
        label: "WEB",
        summary: (input: unknown) => {
          const record = input as Record<string, string>
          return record?.query ?? record?.url ?? stringify(input)
        },
      }
    default:
      return {
        label: "TOOL",
        summary: (input: unknown) => stringify(input),
      }
  }
}

export function extractToolResultText(content: unknown): string {
  if (typeof content === "string") return content
  if (Array.isArray(content)) {
    const texts = content
      .filter((chunk): chunk is { type: string; text: string } => {
        return (
          typeof chunk === "object" &&
          chunk !== null &&
          "text" in chunk &&
          typeof (chunk as Record<string, unknown>).text === "string"
        )
      })
      .map((chunk) => chunk.text)
    if (texts.length > 0) return texts.join("\n")
  }
  return stringify(content)
}

export class ClaudeParser implements StreamParser {
  parseLine(
    line: string,
    logType: string,
    entries: ParsedEntry[],
    itemMap: Map<string, number>
  ): void {
    if (logType === "stdout") {
      const event = parseJsonEvent<ClaudeStreamEvent>(line)
      if (event) {
        if (event.type === "assistant" && event.message?.content) {
          for (const block of event.message.content) {
            if (block.type === "text" && block.text) {
              appendTextEntry(block.text, entries)
            } else if (block.type === "tool_use" && block.id && block.name) {
              itemMap.set(block.id, entries.length)
              entries.push({ kind: "tool", id: block.id, name: block.name, input: block.input })
            }
          }
          return
        }

        if (event.type === "user" && event.message?.content) {
          for (const block of event.message.content) {
            if (block.type === "tool_result" && block.tool_use_id) {
              const idx = itemMap.get(block.tool_use_id)
              if (idx === undefined) continue
              const existing = entries[idx]
              if (existing?.kind === "tool") {
                entries[idx] = {
                  ...existing,
                  result: extractToolResultText(block.content),
                  isError: block.is_error,
                }
              }
            }
          }
          return
        }

        if (event.type === "result") {
          entries.push({ kind: "result", text: event.result ?? "", subtype: event.subtype ?? "" })
          return
        }

        if (event.type === "system" || event.type === "rate_limit_event") {
          return
        }
      }
    }

    appendRawEntry(line, logType, entries)
  }
}
