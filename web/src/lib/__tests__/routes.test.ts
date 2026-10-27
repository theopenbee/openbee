import { describe, expect, it } from "vitest"
import {
  legacyRedirect,
  panelFromSearch,
  panelToSearch,
  resolveConversationRoute,
  type PanelState,
} from "../routes"

describe("legacyRedirect", () => {
  it("legacyRedirect maps worker detail tabs", () => {
    expect(legacyRedirect("/workers/w1", "?tab=env")).toEqual({ to: "/chat/w1?panel=env" })
    expect(legacyRedirect("/workers/w1", "?tab=constraints")).toEqual({ to: "/chat/w1?panel=constraints" })
    expect(legacyRedirect("/workers/w1", "?tab=permissions")).toEqual({ to: "/chat/w1?panel=permissions" })
    expect(legacyRedirect("/workers/w1", "")).toEqual({ to: "/chat/w1?panel=overview" })
    expect(legacyRedirect("/workers/w1", "?tab=sessions")).toEqual({ to: "/chat/w1?panel=overview" })
    expect(legacyRedirect("/workers/w1/", "?tab=env")).toEqual({ to: "/chat/w1?panel=env" })
  })

  it("legacyRedirect maps create and departments", () => {
    expect(legacyRedirect("/workers/create", "")).toEqual({ to: "/chat/bee?panel=new" })
    expect(legacyRedirect("/workers/create", "?copy=w1")).toEqual({ to: "/chat/bee?panel=new&copy=w1" })
    expect(legacyRedirect("/departments", "")).toEqual({ to: "/chat/bee", settings: "departments" })
  })

  it("legacyRedirect maps the worker list to the bee chat", () => {
    expect(legacyRedirect("/workers", "")).toEqual({ to: "/chat/bee" })
    expect(legacyRedirect("/workers/", "?q=x")).toEqual({ to: "/chat/bee" })
  })

  it("legacyRedirect ignores other paths", () => {
    expect(legacyRedirect("/", "")).toBeNull()
    expect(legacyRedirect("/chat", "")).toBeNull()
    expect(legacyRedirect("/chat/bee", "")).toBeNull()
    expect(legacyRedirect("/workers/w1/edit", "")).toBeNull()
    expect(legacyRedirect("/departments/d1", "")).toBeNull()
  })
})

describe("resolveConversationRoute", () => {
  it("resolveConversationRoute rejects unknown workers", () => {
    const workerIds = new Set(["w1"])

    expect(resolveConversationRoute("bee", workerIds)).toBe("bee")
    expect(resolveConversationRoute("bee", new Set())).toBe("bee")
    expect(resolveConversationRoute("w1", workerIds)).toBe("worker:w1")
    expect(resolveConversationRoute("w2", workerIds)).toBeNull()
    expect(resolveConversationRoute("worker:w1", workerIds)).toBeNull()
    expect(resolveConversationRoute("", workerIds)).toBeNull()
    expect(resolveConversationRoute(undefined, workerIds)).toBeNull()
  })
})

describe("panel search", () => {
  it("panel search round trip", () => {
    const states: PanelState[] = [
      null,
      { mode: "worker", section: "overview" },
      { mode: "worker", section: "constraints" },
      { mode: "worker", section: "permissions" },
      { mode: "worker", section: "env" },
      { mode: "new" },
      { mode: "new", copyFrom: "w1" },
    ]

    for (const state of states) {
      const current = new URLSearchParams("q=x&panel=new&copy=old")

      const next = panelToSearch(state, current)

      expect(panelFromSearch(next)).toEqual(state)
      expect(next.get("q")).toBe("x")
      expect(next.has("copy")).toBe(state?.mode === "new" && state.copyFrom !== undefined)
      expect(current.toString()).toBe("q=x&panel=new&copy=old")
    }
  })

  it("reads an unknown or missing panel as closed", () => {
    expect(panelFromSearch(new URLSearchParams(""))).toBeNull()
    expect(panelFromSearch(new URLSearchParams("panel=tasks"))).toBeNull()
    expect(panelFromSearch(new URLSearchParams("copy=w1"))).toBeNull()
  })

  it("ignores an empty copy source", () => {
    expect(panelFromSearch(new URLSearchParams("panel=new&copy="))).toEqual({ mode: "new" })
  })
})
