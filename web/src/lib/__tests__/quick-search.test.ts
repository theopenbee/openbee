import { describe, expect, it } from "vitest"
import { quickSearchEntries, type QuickSearchInput } from "../quick-search"
import type { CatalogPlugin, ConversationSummary, PluginStatus, Worker } from "../types"

const summary = (id: ConversationSummary["id"]): ConversationSummary => ({
  id,
  kind: id === "bee" ? "bee" : "worker",
  worker_id: id === "bee" ? undefined : id.slice("worker:".length),
  unread: 0,
  activity: { state: "idle" },
})

const worker = (id: string, name: string, description = ""): Worker => ({
  id,
  name,
  description,
  constraints: "",
  work_dir: "",
  engine: "claude",
  status: "idle",
  created_at: 0,
  updated_at: 0,
})

const plugin = (id: string, name: Record<string, string>): PluginStatus => ({
  id,
  type: "channel",
  source: "npm",
  path: "",
  name,
  enabled: true,
  bot_name: "",
  state: "running",
  has_setup: false,
})

const workers = new Map([
  ["w1", worker("w1", "Qiao", "Builds openbee")],
  ["w2", worker("w2", "Lin")],
])

const input = (overrides: Partial<QuickSearchInput> = {}): QuickSearchInput => ({
  conversations: [summary("bee"), summary("worker:w1"), summary("worker:w2")],
  workers,
  current: null,
  plugins: [],
  commands: new Set(),
  lang: "en",
  t: (key) => key,
  ...overrides,
})

describe("quickSearchEntries", () => {
  it("lists conversations in the given order before everything else", () => {
    const entries = quickSearchEntries(input())
    expect(entries.slice(0, 3).map((entry) => entry.label)).toEqual(["chat.beeLabel", "Qiao", "Lin"])
    expect(entries[1]).toMatchObject({
      group: "conversations",
      description: "Builds openbee",
      keywords: ["claude"],
      target: { kind: "conversation", conversation: "worker:w1" },
    })
    expect(entries[2].description).toBeUndefined()
  })

  it("skips conversations whose worker is gone", () => {
    const entries = quickSearchEntries(input({ conversations: [summary("worker:gone")] }))
    expect(entries.some((entry) => entry.group === "conversations")).toBe(false)
  })

  it("offers a new conversation only when the chat can start one", () => {
    const ids = (commands: QuickSearchInput["commands"]) =>
      quickSearchEntries(input({ commands }))
        .filter((entry) => entry.group === "actions")
        .map((entry) => entry.id)
    expect(ids(new Set())).toEqual(["action:newWorker", "action:showShortcuts"])
    expect(ids(new Set(["newConversation"]))).toEqual([
      "action:newConversation",
      "action:newWorker",
      "action:showShortcuts",
    ])
  })

  it("adds the open worker's panel sections", () => {
    const panels = (current: QuickSearchInput["current"]) =>
      quickSearchEntries(input({ current })).filter((entry) => entry.group === "worker")
    expect(panels("bee")).toEqual([])
    expect(panels(null)).toEqual([])
    expect(panels("worker:w1").map((entry) => entry.target)).toEqual([
      { kind: "panel", section: "constraints" },
      { kind: "panel", section: "permissions" },
      { kind: "panel", section: "env" },
    ])
    expect(panels("worker:w1")[0].keywords).toEqual(["Qiao"])
  })

  it("lists every settings section and installed plugin", () => {
    const entries = quickSearchEntries(
      input({ lang: "zh", plugins: [plugin("lark", { en: "Lark", zh: "飞书" }), plugin("bare", {})] }),
    )
    expect(entries.filter((entry) => entry.group === "settings").map((entry) => entry.target)).toEqual([
      { kind: "settings", section: "env" },
      { kind: "settings", section: "system" },
      { kind: "settings", section: "plugins" },
      { kind: "settings", section: "about" },
    ])
    expect(entries.filter((entry) => entry.group === "plugins").map((entry) => entry.label)).toEqual(["飞书", "bare"])
  })

  it("carries the plugin logo when the plugin has one", () => {
    const svg = `<svg xmlns="http://www.w3.org/2000/svg"/>`
    const entries = quickSearchEntries(input({ plugins: [{ ...plugin("lark", { en: "Lark" }), icon: svg }, plugin("bare", {})] }))
    expect(entries.filter((entry) => entry.group === "plugins").map((entry) => entry.icon)).toEqual([svg, undefined])
  })

  it("falls back to the catalog logo of an installed plugin", () => {
    const svg = `<svg xmlns="http://www.w3.org/2000/svg"/>`
    const catalog = [{ id: "bare", icon: svg }] as CatalogPlugin[]
    const entries = quickSearchEntries(input({ plugins: [plugin("lark", { en: "Lark" }), plugin("bare", {})], catalog }))
    expect(entries.filter((entry) => entry.group === "plugins").map((entry) => entry.icon)).toEqual([undefined, svg])
  })

  it("gives every entry a unique id", () => {
    const entries = quickSearchEntries(
      input({ current: "worker:w1", commands: new Set(["newConversation"]), plugins: [plugin("lark", { en: "Lark" })] }),
    )
    expect(new Set(entries.map((entry) => entry.id)).size).toBe(entries.length)
  })
})
