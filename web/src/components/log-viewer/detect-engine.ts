export function detectEngine(lines: string[]): "claude" | "codex" | "pi" | "kimi" {
  for (const line of lines) {
    let obj: Record<string, unknown>
    try {
      const parsed = JSON.parse(line)
      if (!parsed || typeof parsed !== "object") continue
      obj = parsed as Record<string, unknown>
    } catch {
      continue
    }
    if (obj.type === "thread.started") return "codex"
    if (obj.type === "agent_start") return "pi"
    if (typeof obj.role === "string" && obj.type === undefined) return "kimi"
  }
  return "claude"
}
