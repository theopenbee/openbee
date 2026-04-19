import { describe, expect, it } from "vitest"
import { detectEngine } from "../detect-engine"

describe("detectEngine", () => {
  it("returns 'codex' when first line is thread.started", () => {
    const line = JSON.stringify({ type: "thread.started", thread_id: "abc-123" })
    expect(detectEngine([line])).toBe("codex")
  })

  it("returns 'pi' when first line is agent_start", () => {
    const line = JSON.stringify({ type: "agent_start" })
    expect(detectEngine([line])).toBe("pi")
  })

  it("returns 'pi' when agent_start appears on the second line", () => {
    const firstLine = JSON.stringify({ type: "some_metadata" })
    const secondLine = JSON.stringify({ type: "agent_start" })
    expect(detectEngine([firstLine, secondLine])).toBe("pi")
  })

  it("returns 'claude' for a Claude assistant event", () => {
    const line = JSON.stringify({ type: "assistant", message: { content: [] } })
    expect(detectEngine([line])).toBe("claude")
  })

  it("returns 'claude' for any non-Codex non-pi type", () => {
    const line = JSON.stringify({ type: "system" })
    expect(detectEngine([line])).toBe("claude")
  })

  it("returns 'claude' when JSON is malformed", () => {
    expect(detectEngine(["not json at all"])).toBe("claude")
  })

  it("returns 'claude' for an empty array", () => {
    expect(detectEngine([])).toBe("claude")
  })

  it("returns 'kimi' when first line has role but no type", () => {
    const line = JSON.stringify({ role: "user", content: "hello" })
    expect(detectEngine([line])).toBe("kimi")
  })

  it("returns 'kimi' when assistant role line appears first", () => {
    const line = JSON.stringify({ role: "assistant", content: "hi" })
    expect(detectEngine([line])).toBe("kimi")
  })

  it("does not mistake Claude assistant event for kimi", () => {
    // Claude lines have a top-level "type" field
    const line = JSON.stringify({ type: "assistant", message: { content: [] } })
    expect(detectEngine([line])).toBe("claude")
  })

  it("returns 'kimi' when kimi line appears after an unknown line", () => {
    const unknown = "not json"
    const kimiLine = JSON.stringify({ role: "user", content: "hello" })
    expect(detectEngine([unknown, kimiLine])).toBe("kimi")
  })
})
