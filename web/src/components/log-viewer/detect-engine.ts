import { parseJsonEvent } from "./types"

export function detectEngine(firstLine: string): "claude" | "codex" {
  const event = parseJsonEvent<{ type: string }>(firstLine)
  return event?.type === "thread.started" ? "codex" : "claude"
}
