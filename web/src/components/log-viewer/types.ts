export type ParsedEntry =
  | { kind: "text"; text: string }
  | {
      kind: "tool"
      id: string
      name: string
      input: unknown
      result?: string
      isError?: boolean
    }
  | { kind: "result"; text: string; subtype: string }
  | { kind: "raw"; content: string; logType: string; lineCount: number }
  | {
      kind: "codex-command"
      id: string
      command: string
      output?: string
      inProgress: boolean
    }
  | {
      kind: "codex-turn"
      inputTokens: number
      cachedInputTokens: number
      outputTokens: number
    }
  | { kind: "pi-thinking"; id: string; thinking: string }

export interface StreamParser {
  parseLine(
    line: string,
    logType: string,
    entries: ParsedEntry[],
    itemMap: Map<string, number>
  ): void
}

export function appendTextEntry(text: string, entries: ParsedEntry[]): void {
  const last = entries[entries.length - 1]
  if (last?.kind === "text") {
    entries[entries.length - 1] = { ...last, text: `${last.text}\n\n${text}` }
    return
  }
  entries.push({ kind: "text", text })
}

export function appendRawEntry(content: string, logType: string, entries: ParsedEntry[]): void {
  const last = entries[entries.length - 1]
  if (last?.kind === "raw" && last.logType === logType) {
    entries[entries.length - 1] = { ...last, content: `${last.content}\n${content}`, lineCount: last.lineCount + 1 }
    return
  }
  entries.push({ kind: "raw", content, logType, lineCount: 1 })
}

export function parseJsonEvent<T extends { type: string }>(line: string): T | null {
  try {
    const obj = JSON.parse(line)
    if (obj && typeof obj.type === "string") return obj as T
    return null
  } catch {
    return null
  }
}
