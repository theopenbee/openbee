import { describe, expect, it } from "vitest"
import { availableEngine, newWorkerEngine } from "@/lib/types"

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

describe("newWorkerEngine", () => {
  it("preselects the system default engine when it is enabled", () => {
    expect(newWorkerEngine("pi", ["claude", "codex", "pi"])).toBe("pi")
  })

  it("falls back to the first enabled engine when the default is disabled or unset", () => {
    expect(newWorkerEngine("claude", ["codex", "pi"])).toBe("codex")
    expect(newWorkerEngine(undefined, ["codex", "pi"])).toBe("codex")
    expect(newWorkerEngine("", ["pi"])).toBe("pi")
  })

  it("uses the built-in default when no engine is enabled", () => {
    expect(newWorkerEngine("pi", [])).toBe("claude")
  })
})
