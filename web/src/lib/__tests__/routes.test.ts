import { describe, expect, it } from "vitest"
import {
  hashShowsConversation,
  isPanelSection,
  legacyRedirect,
  panelFromSearch,
  panelReplacesHistory,
  panelToSearch,
  panelView,
  resolveConversationRoute,
  settingsPath,
  settingsReturnPath,
  settingsTarget,
  workerPanelPath,
  type PanelState,
} from "../routes"

describe("legacyRedirect", () => {
  it("legacyRedirect maps worker detail tabs", () => {
    expect(legacyRedirect("/workers/w1", "?tab=env")).toBe("/chat/w1?panel=env")
    expect(legacyRedirect("/workers/w1", "?tab=constraints")).toBe("/chat/w1?panel=constraints")
    expect(legacyRedirect("/workers/w1", "?tab=permissions")).toBe("/chat/w1?panel=permissions")
    expect(legacyRedirect("/workers/w1", "")).toBe("/chat/w1?panel=constraints")
    expect(legacyRedirect("/workers/w1", "?tab=sessions")).toBe("/chat/w1?panel=constraints")
    expect(legacyRedirect("/workers/w1/", "?tab=env")).toBe("/chat/w1?panel=env")
  })

  it("legacyRedirect maps create and the old departments page", () => {
    expect(legacyRedirect("/workers/create", "")).toBe("/chat/bee?panel=new")
    expect(legacyRedirect("/workers/create", "?copy=w1")).toBe("/chat/bee?panel=new")
    expect(legacyRedirect("/departments", "")).toBe("/chat/bee")
  })

  it("legacyRedirect maps the worker list to the bee chat", () => {
    expect(legacyRedirect("/workers", "")).toBe("/chat/bee")
    expect(legacyRedirect("/workers/", "?q=x")).toBe("/chat/bee")
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
      { mode: "worker", section: "constraints" },
      { mode: "worker", section: "permissions" },
      { mode: "worker", section: "env" },
      { mode: "new" },
    ]

    for (const state of states) {
      const current = new URLSearchParams("q=x&panel=new")

      const next = panelToSearch(state, current)

      expect(panelFromSearch(next)).toEqual(state)
      expect(next.get("q")).toBe("x")
      expect(current.toString()).toBe("q=x&panel=new")
    }
  })

  it("reads an unknown or missing panel as closed", () => {
    expect(panelFromSearch(new URLSearchParams(""))).toBeNull()
    expect(panelFromSearch(new URLSearchParams("panel=tasks"))).toBeNull()
  })

  it("ignores a legacy copy source", () => {
    expect(panelFromSearch(new URLSearchParams("panel=new&copy=w1"))).toEqual({ mode: "new" })
  })
})

describe("panelView", () => {
  it("shows the new worker form on any chat", () => {
    expect(panelView("bee", { mode: "new" })).toEqual({ mode: "new" })
    expect(panelView("w1", { mode: "new" })).toEqual({ mode: "new" })
    expect(panelView(undefined, { mode: "new" })).toEqual({ mode: "new" })
  })

  it("shows worker sections only in worker chats", () => {
    expect(panelView("w1", { mode: "worker", section: "env" })).toEqual({ mode: "worker", workerId: "w1", section: "env" })
    expect(panelView("bee", { mode: "worker", section: "constraints" })).toBeNull()
    expect(panelView(undefined, { mode: "worker", section: "constraints" })).toBeNull()
    expect(panelView("", { mode: "worker", section: "constraints" })).toBeNull()
  })

  it("stays closed without a panel", () => {
    expect(panelView("w1", null)).toBeNull()
    expect(panelView("bee", null)).toBeNull()
  })
})

describe("workerPanelPath", () => {
  it("opens the new worker chat on its constraints", () => {
    expect(workerPanelPath("w9")).toBe("/chat/w9?panel=constraints")
  })
})

describe("isPanelSection", () => {
  it("accepts only the three worker sections", () => {
    expect(["constraints", "permissions", "env"].every(isPanelSection)).toBe(true)
    expect(isPanelSection("overview")).toBe(false)
    expect(isPanelSection("new")).toBe(false)
    expect(isPanelSection("sessions")).toBe(false)
    expect(isPanelSection(null)).toBe(false)
  })
})

describe("panelReplacesHistory", () => {
  it("replaces history only when switching worker sections", () => {
    const constraints: PanelState = { mode: "worker", section: "constraints" }
    const env: PanelState = { mode: "worker", section: "env" }

    expect(panelReplacesHistory(constraints, env)).toBe(true)
    expect(panelReplacesHistory(null, constraints)).toBe(false)
    expect(panelReplacesHistory(constraints, null)).toBe(false)
    expect(panelReplacesHistory(constraints, { mode: "new" })).toBe(false)
    expect(panelReplacesHistory({ mode: "new" }, constraints)).toBe(false)
  })
})

describe("hashShowsConversation", () => {
  it("matches the conversation the hash routes to", () => {
    expect(hashShowsConversation("#/chat/w1", "worker:w1")).toBe(true)
    expect(hashShowsConversation("#/chat/bee", "bee")).toBe(true)
  })

  it("ignores the panel query", () => {
    expect(hashShowsConversation("#/chat/w1?panel=env", "worker:w1")).toBe(true)
  })

  it("matches the same loose paths the router accepts", () => {
    expect(hashShowsConversation("#/chat/w1/", "worker:w1")).toBe(true)
    expect(hashShowsConversation("#/CHAT/w1", "worker:w1")).toBe(true)
    expect(hashShowsConversation("#chat/w1", "worker:w1")).toBe(true)
    expect(hashShowsConversation("#/chat/w1#x", "worker:w1")).toBe(true)
  })

  it("rejects another conversation the hash has already moved to", () => {
    expect(hashShowsConversation("#/chat/w2", "worker:w1")).toBe(false)
    expect(hashShowsConversation("#/chat/w1", "bee")).toBe(false)
  })

  it("rejects routes that show no conversation", () => {
    expect(hashShowsConversation("#/chat", "bee")).toBe(false)
    expect(hashShowsConversation("", "bee")).toBe(false)
    expect(hashShowsConversation("#/chat/w1/extra", "worker:w1")).toBe(false)
  })
})

describe("settingsPath", () => {
  it("puts each section on its own page", () => {
    expect(settingsPath({ section: "env" })).toBe("/settings/env")
    expect(settingsPath({ section: "system" })).toBe("/settings/system")
    expect(settingsPath({ section: "plugins" })).toBe("/settings/plugins")
  })

  it("routes plugin pages and the installed tab", () => {
    expect(settingsPath({ section: "plugins", plugin: "telegram" })).toBe("/settings/plugins/telegram")
    expect(settingsPath({ section: "plugins", tab: "installed" })).toBe("/settings/plugins?tab=installed")
    expect(settingsPath({ section: "plugins", tab: "market" })).toBe("/settings/plugins")
  })

  it("ignores plugin fields outside the plugins section", () => {
    expect(settingsPath({ section: "env", plugin: "telegram", tab: "installed" })).toBe("/settings/env")
  })
})

describe("settingsTarget", () => {
  const none = new URLSearchParams()

  it("reads the section, plugin and tab back", () => {
    expect(settingsTarget("env", undefined, none)).toEqual({ section: "env" })
    expect(settingsTarget("plugins", "telegram", none)).toEqual({ section: "plugins", plugin: "telegram" })
    expect(settingsTarget("plugins", undefined, new URLSearchParams("tab=installed"))).toEqual({
      section: "plugins",
      tab: "installed",
    })
    expect(settingsTarget("plugins", undefined, new URLSearchParams("tab=other"))).toEqual({ section: "plugins" })
  })

  it("rejects unknown sections", () => {
    expect(settingsTarget("billing", undefined, none)).toBeNull()
    expect(settingsTarget(undefined, undefined, none)).toBeNull()
  })

  it("round-trips every path settingsPath builds", () => {
    const targets = [
      { section: "env" },
      { section: "system" },
      { section: "plugins" },
      { section: "plugins", tab: "installed" },
      { section: "plugins", plugin: "lark" },
    ] as const
    for (const target of targets) {
      const url = new URL(settingsPath(target), "http://x")
      const [, , section, plugin] = url.pathname.split("/")
      expect(settingsTarget(section, plugin, url.searchParams)).toEqual(target)
    }
  })
})

describe("settingsReturnPath", () => {
  it("returns to the chat settings opened from", () => {
    expect(settingsReturnPath("/chat/w1", "?panel=env", null)).toBe("/chat/w1?panel=env")
    expect(settingsReturnPath("/chat", "", null)).toBe("/chat")
  })

  it("keeps the chat to return to while moving between settings pages", () => {
    expect(settingsReturnPath("/settings/env", "", { returnTo: "/chat/w1" })).toBe("/chat/w1")
    expect(settingsReturnPath("/settings/plugins/lark", "", { returnTo: "/chat/bee" })).toBe("/chat/bee")
  })

  it("falls back to the chat list without a chat to return to", () => {
    expect(settingsReturnPath("/settings/env", "", null)).toBe("/chat")
    expect(settingsReturnPath("/settings/env", "", { returnTo: "/settings/system" })).toBe("/chat")
    expect(settingsReturnPath("/settings/env", "", { returnTo: 42 })).toBe("/chat")
    expect(settingsReturnPath("/settings/env", "", { returnTo: "https://example.com/chat" })).toBe("/chat")
    expect(settingsReturnPath("/login", "", null)).toBe("/chat")
  })
})
