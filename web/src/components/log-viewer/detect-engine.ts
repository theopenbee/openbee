import { parseJsonEvent } from "./types"

export function detectEngine(firstLine: string): "claude" | "codex" | "pi" {
  const event = parseJsonEvent<{ type: string }>(firstLine)
  if (event?.type === "thread.started") return "codex"
  if (event?.type === "agent_start") return "pi"
  return "claude"
}
