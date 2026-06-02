import type { Engine } from "@/lib/types"
import { parseJsonObject } from "./types"

export function detectEngine(lines: string[]): Engine {
  for (const line of lines) {
    const obj = parseJsonObject(line)
    if (!obj) continue
    if (obj.type === "thread.started") return "codex"
    if (obj.type === "agent_start") return "pi"
  }
  return "claude"
}
