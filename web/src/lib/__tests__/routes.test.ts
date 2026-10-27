import { describe, expect, it } from "vitest"
import {
  isPanelSection,
  legacyRedirect,
  panelFromSearch,
  panelReplacesHistory,
  panelToSearch,
  panelView,
  resolveConversationRoute,
  workerPanelPath,
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

describe("panelView", () => {
  it("shows the new worker form on any chat", () => {
    expect(panelView("bee", { mode: "new" })).toEqual({ mode: "new" })
    expect(panelView("w1", { mode: "new", copyFrom: "w1" })).toEqual({ mode: "new", copyFrom: "w1" })
    expect(panelView(undefined, { mode: "new" })).toEqual({ mode: "new" })
  })

  it("shows worker sections only in worker chats", () => {
    expect(panelView("w1", { mode: "worker", section: "env" })).toEqual({ mode: "worker", workerId: "w1", section: "env" })
    expect(panelView("bee", { mode: "worker", section: "overview" })).toBeNull()
    expect(panelView(undefined, { mode: "worker", section: "overview" })).toBeNull()
    expect(panelView("", { mode: "worker", section: "overview" })).toBeNull()
  })

  it("stays closed without a panel", () => {
    expect(panelView("w1", null)).toBeNull()
    expect(panelView("bee", null)).toBeNull()
  })
})

describe("workerPanelPath", () => {
  it("opens the new worker chat on its overview", () => {
    expect(workerPanelPath("w9")).toBe("/chat/w9?panel=overview")
  })
})

describe("isPanelSection", () => {
  it("accepts only the four worker sections", () => {
    expect(["overview", "constraints", "permissions", "env"].every(isPanelSection)).toBe(true)
    expect(isPanelSection("new")).toBe(false)
    expect(isPanelSection("sessions")).toBe(false)
    expect(isPanelSection(null)).toBe(false)
  })
})

describe("panelReplacesHistory", () => {
  it("replaces history only when switching worker sections", () => {
    const overview: PanelState = { mode: "worker", section: "overview" }
    const env: PanelState = { mode: "worker", section: "env" }

    expect(panelReplacesHistory(overview, env)).toBe(true)
    expect(panelReplacesHistory(null, overview)).toBe(false)
    expect(panelReplacesHistory(overview, null)).toBe(false)
    expect(panelReplacesHistory(overview, { mode: "new", copyFrom: "w1" })).toBe(false)
    expect(panelReplacesHistory({ mode: "new" }, overview)).toBe(false)
  })
})
