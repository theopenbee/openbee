export function detectEngine(firstLine: string): "claude" | "codex" {
  try {
    const obj = JSON.parse(firstLine)
    if (obj && obj.type === "thread.started") return "codex"
  } catch {
    // fall through
  }
  return "claude"
}
