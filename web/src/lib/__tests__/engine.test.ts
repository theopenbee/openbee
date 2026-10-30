import { describe, expect, it } from "vitest"
import { availableEngine, newWorkerEngine, pickDefaultEngine } from "@/lib/types"

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

  it("is empty when no engine is enabled", () => {
    expect(newWorkerEngine("pi", [])).toBe("")
  })
})

describe("pickDefaultEngine", () => {
  it("keeps the worker's engine even when it is no longer enabled", () => {
    expect(pickDefaultEngine("codex", ["pi"])).toBe("codex")
    expect(pickDefaultEngine("codex", [])).toBe("codex")
  })

  it("seeds an unset engine from the first enabled engine", () => {
    expect(pickDefaultEngine("", ["pi", "claude"])).toBe("pi")
    expect(pickDefaultEngine(undefined, ["pi"])).toBe("pi")
    expect(pickDefaultEngine(undefined, [])).toBe("")
  })
})
