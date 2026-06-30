import { describe, it, expect } from "vitest"
import { firstAccessiblePath, granted } from "../nav"
import { Perm } from "../permissions"

describe("firstAccessiblePath", () => {
  it("returns the dashboard for a super-admin (wildcard)", () => {
    expect(firstAccessiblePath(["*"])).toBe("/")
  })

  it("lands on chat when the user holds chat:write but not dashboard:read", () => {
    // /chat is gated on chat:write and sits above tasks/departments in nav order.
    expect(firstAccessiblePath([Perm.ChatWrite])).toBe("/chat")
  })

  it("prefers gated chat over a held deeper permission", () => {
    expect(firstAccessiblePath([Perm.ChatWrite, Perm.TasksRead])).toBe("/chat")
  })

  it("skips chat when chat:write is missing and lands on the next accessible page", () => {
    expect(firstAccessiblePath([Perm.TasksRead])).toBe("/tasks")
    expect(firstAccessiblePath([Perm.ContactsRead])).toBe("/departments")
  })

  it("returns undefined when no permissions grant any page", () => {
    expect(firstAccessiblePath([])).toBeUndefined()
    expect(firstAccessiblePath(undefined)).toBeUndefined()
  })

  it("resolves users/roles now that they live inside the System group", () => {
    // Moved from top-level leaves into the System group; group flattening must
    // still reach them via their sub-item perms.
    expect(firstAccessiblePath([Perm.UsersManage])).toBe("/users")
    expect(firstAccessiblePath([Perm.RolesManage])).toBe("/roles")
    expect(firstAccessiblePath([Perm.EnvRead])).toBe("/env")
  })
})

describe("granted", () => {
  it("always grants ungated entries", () => {
    expect(granted(undefined, undefined)).toBe(true)
    expect(granted([], undefined)).toBe(true)
  })

  it("honours the wildcard and explicit grants", () => {
    expect(granted(["*"], Perm.DashboardRead)).toBe(true)
    expect(granted([Perm.DashboardRead], Perm.DashboardRead)).toBe(true)
    expect(granted([Perm.TasksRead], Perm.DashboardRead)).toBe(false)
    expect(granted(undefined, Perm.DashboardRead)).toBe(false)
  })
})
