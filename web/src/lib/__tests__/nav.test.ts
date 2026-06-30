import { describe, it, expect } from "vitest"
import { firstAccessiblePath, granted } from "../nav"
import { Perm } from "../permissions"

describe("firstAccessiblePath", () => {
  it("returns the dashboard for a super-admin (wildcard)", () => {
    expect(firstAccessiblePath(["*"])).toBe("/")
  })

  it("skips the stats-gated dashboard and lands on chat when stats:read is missing", () => {
    // /chat carries no perm, so it is the universal fallback landing page.
    expect(firstAccessiblePath([])).toBe("/chat")
  })

  it("still prefers the ungated chat over a held deeper permission", () => {
    // chat sits above tasks/departments in nav order and is always reachable.
    expect(firstAccessiblePath([Perm.TasksRead])).toBe("/chat")
    expect(firstAccessiblePath([Perm.DepartmentsRead])).toBe("/chat")
  })

  it("treats undefined perms the same as none — chat remains reachable", () => {
    expect(firstAccessiblePath(undefined)).toBe("/chat")
  })
})

describe("granted", () => {
  it("always grants ungated entries", () => {
    expect(granted(undefined, undefined)).toBe(true)
    expect(granted([], undefined)).toBe(true)
  })

  it("honours the wildcard and explicit grants", () => {
    expect(granted(["*"], Perm.StatsRead)).toBe(true)
    expect(granted([Perm.StatsRead], Perm.StatsRead)).toBe(true)
    expect(granted([Perm.TasksRead], Perm.StatsRead)).toBe(false)
    expect(granted(undefined, Perm.StatsRead)).toBe(false)
  })
})
