import { parseJsonEvent } from "./types"

function hasTopLevelRole(line: string): boolean {
  try {
    const obj = JSON.parse(line)
    return obj !== null && typeof obj === "object" && typeof obj.role === "string" && typeof obj.type === "undefined"
  } catch {
    return false
  }
}

export function detectEngine(lines: string[]): "claude" | "codex" | "pi" | "kimi" {
  for (const line of lines) {
    const event = parseJsonEvent<{ type: string }>(line)
    if (event?.type === "thread.started") return "codex"
    if (event?.type === "agent_start") return "pi"
    if (hasTopLevelRole(line)) return "kimi"
  }
  return "claude"
}
