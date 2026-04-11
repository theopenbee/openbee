import { describe, expect, it } from "vitest"
import { detectEngine } from "../detect-engine"

describe("detectEngine", () => {
  it("returns 'codex' when first line is thread.started", () => {
    const line = JSON.stringify({ type: "thread.started", thread_id: "abc-123" })
    expect(detectEngine(line)).toBe("codex")
  })

  it("returns 'claude' for a Claude assistant event", () => {
    const line = JSON.stringify({ type: "assistant", message: { content: [] } })
    expect(detectEngine(line)).toBe("claude")
  })

  it("returns 'claude' for any non-Codex type", () => {
    const line = JSON.stringify({ type: "system" })
    expect(detectEngine(line)).toBe("claude")
  })

  it("returns 'claude' when JSON is malformed", () => {
    expect(detectEngine("not json at all")).toBe("claude")
  })

  it("returns 'claude' for an empty string", () => {
    expect(detectEngine("")).toBe("claude")
  })
})
