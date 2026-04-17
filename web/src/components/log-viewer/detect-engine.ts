import { parseJsonObject } from "./types"

export function detectEngine(lines: string[]): "claude" | "codex" | "pi" | "kimi" {
  for (const line of lines) {
    const obj = parseJsonObject(line)
    if (!obj) continue
    if (obj.type === "thread.started") return "codex"
    if (obj.type === "agent_start") return "pi"
    if (typeof obj.role === "string" && obj.type === undefined) return "kimi"
  }
  return "claude"
}
