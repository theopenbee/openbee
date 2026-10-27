import { describe, expect, it } from "vitest"
import { availableEngine } from "@/lib/types"

describe("availableEngine", () => {
  it("keeps an enabled engine", () => {
    expect(availableEngine("pi", ["claude", "pi"])).toBe("pi")
  })

  it("falls back to the first enabled engine when the seeded one is disabled", () => {
    expect(availableEngine("claude", ["pi"])).toBe("pi")
  })

  it("keeps the engine when the enabled list is empty", () => {
    expect(availableEngine("codex", [])).toBe("codex")
  })
})
