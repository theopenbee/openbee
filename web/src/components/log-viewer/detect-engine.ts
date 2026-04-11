import { parseJsonEvent } from "./types"

export function detectEngine(lines: string[]): "claude" | "codex" | "pi" {
  for (const line of lines) {
    const event = parseJsonEvent<{ type: string }>(line)
    if (event?.type === "thread.started") return "codex"
    if (event?.type === "agent_start") return "pi"
  }
  return "claude"
}
