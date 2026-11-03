import { describe, expect, it } from "vitest"
import { engineSelectOptions, findEngine, listEnabledEngines } from "../engines"
import type { AppConfig } from "../types"

const config = (over: Partial<AppConfig>): AppConfig => ({
  language: "en",
  enabled_engines: [],
  engines: [],
  ...over,
})

describe("listEnabledEngines", () => {
  it("returns an empty list when no config is loaded or no engine is installed", () => {
    expect(listEnabledEngines(undefined, "en")).toEqual([])
    expect(listEnabledEngines(config({}), "en")).toEqual([])
    expect(listEnabledEngines({ language: "en" } as AppConfig, "en")).toEqual([])
  })

  it("localizes names for the UI language", () => {
    const data = config({
      enabled_engines: ["claude", "pi"],
      engines: [
        { id: "claude", name: { en: "Claude Code", zh: "Claude 代码" } },
        { id: "pi", name: { en: "Pi" } },
      ],
    })
    expect(listEnabledEngines(data, "zh")).toEqual([
      { id: "claude", name: "Claude 代码" },
      { id: "pi", name: "Pi" },
    ])
    expect(listEnabledEngines(data, "en")).toEqual([
      { id: "claude", name: "Claude Code" },
      { id: "pi", name: "Pi" },
    ])
  })

  it("falls back to the id when a name is missing", () => {
    const data = config({ engines: [{ id: "custom", name: {} }, { id: "other" } as never] })
    expect(listEnabledEngines(data, "en")).toEqual([
      { id: "custom", name: "custom" },
      { id: "other", name: "other" },
    ])
  })

  it("keeps the order sent by the server", () => {
    const data = config({
      engines: [
        { id: "pi", name: { en: "Pi" } },
        { id: "codex", name: { en: "Codex" } },
      ],
    })
    expect(listEnabledEngines(data, "en").map((e) => e.id)).toEqual(["pi", "codex"])
  })
})

describe("findEngine", () => {
  const engines = [{ id: "pi", name: "Pi" }]

  it("returns the listed engine", () => {
    expect(findEngine(engines, "pi")).toEqual({ id: "pi", name: "Pi" })
  })

  it("falls back to the id for an engine that is not installed", () => {
    expect(findEngine(engines, "codex")).toEqual({ id: "codex", name: "codex" })
  })
})

describe("engineSelectOptions", () => {
  const engines = [
    { id: "claude", name: "Claude Code" },
    { id: "pi", name: "Pi" },
  ]
  const unavailable = (id: string) => `${id} (not installed)`

  it("lists the installed engines", () => {
    expect(engineSelectOptions(engines, "pi", unavailable)).toEqual([
      { value: "claude", label: "Claude Code" },
      { value: "pi", label: "Pi" },
    ])
  })

  it("adds the stored engine as a disabled option when it is no longer installed", () => {
    expect(engineSelectOptions(engines, "codex", unavailable)).toEqual([
      { value: "claude", label: "Claude Code" },
      { value: "pi", label: "Pi" },
      { value: "codex", label: "codex (not installed)", disabled: true },
    ])
    expect(engineSelectOptions([], "codex", unavailable)).toEqual([
      { value: "codex", label: "codex (not installed)", disabled: true },
    ])
  })

  it("adds nothing for an unset stored engine", () => {
    expect(engineSelectOptions(engines, "", unavailable)).toHaveLength(2)
    expect(engineSelectOptions(engines, undefined, unavailable)).toHaveLength(2)
    expect(engineSelectOptions([], undefined, unavailable)).toEqual([])
  })
})
