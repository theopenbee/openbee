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
  | { kind: "raw"; content: string; logType: string }
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
    last.text = `${last.text}\n\n${text}`
    return
  }
  entries.push({ kind: "text", text })
}

export function appendRawEntry(content: string, logType: string, entries: ParsedEntry[]): void {
  const last = entries[entries.length - 1]
  if (last?.kind === "raw" && last.logType === logType) {
    last.content = `${last.content}\n${content}`
    return
  }
  entries.push({ kind: "raw", content, logType })
}
