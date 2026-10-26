import { describe, expect, it } from "vitest"
import {
  buildSettingsPayload,
  filterCatalog,
  formatElapsed,
  initialFormValues,
  invalidIntFields,
  isTransitional,
  latestLink,
  localize,
  marketAction,
  pendingInstalls,
} from "../plugins"
import type { CatalogPlugin, PluginConfigField, PluginOperation } from "../types"

const fields: PluginConfigField[] = [
  { key: "token", type: "secret", required: true },
  { key: "name", type: "string" },
  { key: "secret_path", type: "string", random_default: true },
  { key: "port", type: "int", default: 8080 },
  { key: "admins", type: "list" },
  { key: "debug", type: "bool", default: true },
]

describe("initialFormValues", () => {
  it("leaves secrets empty and fills saved values", () => {
    const values = initialFormValues(fields, {
      token: "****abcd",
      name: "bee",
      secret_path: "",
      port: 9000,
      admins: ["a", "b"],
      debug: false,
    })
    expect(values).toEqual({
      token: "",
      name: "bee",
      secret_path: "",
      port: "9000",
      admins: "a\nb",
      debug: false,
    })
  })

  it("prefills a random value for unsaved random_default fields only", () => {
    const values = initialFormValues(fields, {})
    expect(values.secret_path).toMatch(/^[0-9a-f]{16}$/)
    expect(values.name).toBe("")
    expect(values.port).toBe("")
    expect(values.debug).toBe(true)
  })
})

describe("buildSettingsPayload", () => {
  const saved = { token: "****abcd", name: "bee", port: 9000, admins: ["a"], debug: true, secret_path: "x" }

  it("omits untouched fields and a masked secret left empty", () => {
    const values = initialFormValues(fields, saved)
    expect(buildSettingsPayload(fields, saved, values)).toEqual({})
  })

  it("sends a new secret", () => {
    const values = { ...initialFormValues(fields, saved), token: " new-token " }
    expect(buildSettingsPayload(fields, saved, values)).toEqual({ token: "new-token" })
  })

  it("clears saved optional values by type", () => {
    const values = { ...initialFormValues(fields, saved), name: "", port: "", admins: "  " }
    expect(buildSettingsPayload(fields, saved, values)).toEqual({ name: "", port: null, admins: [] })
  })

  it("sends changed values with their types", () => {
    const values = { ...initialFormValues(fields, saved), port: "9100", admins: "a\n b \n\nc", debug: false }
    expect(buildSettingsPayload(fields, saved, values)).toEqual({ port: 9100, admins: ["a", "b", "c"], debug: false })
  })

  it("leaves unsaved empty fields to plugin defaults", () => {
    const values = { ...initialFormValues(fields, {}), secret_path: "keep" }
    expect(buildSettingsPayload(fields, {}, values)).toEqual({ secret_path: "keep" })
  })

  it("sends an emptied random_default field as an explicit empty string", () => {
    const values = { ...initialFormValues(fields, {}), secret_path: "" }
    expect(buildSettingsPayload(fields, {}, values)).toEqual({ secret_path: "" })
  })

  it("sends a bool that differs from the plugin default", () => {
    const values = { ...initialFormValues(fields, {}), debug: false }
    expect(buildSettingsPayload(fields, {}, values)).toMatchObject({ debug: false })
  })
})

describe("invalidIntFields", () => {
  it("reports non-integer input", () => {
    expect(invalidIntFields(fields, { port: "12a" })).toEqual(["port"])
    expect(invalidIntFields(fields, { port: " -3 " })).toEqual([])
    expect(invalidIntFields(fields, { port: "" })).toEqual([])
  })
})

const entry = (over: Partial<CatalogPlugin>): CatalogPlugin => ({
  category: "channel",
  id: "telegram",
  package: "@theopenbee/channel-telegram",
  version: "0.2.0",
  official: true,
  name: { en: "Telegram", zh: "Telegram" },
  description: { en: "Bot API channel", zh: "机器人渠道" },
  protocol_version: 1,
  license: "MIT",
  repository: "https://github.com/theopenbee/channel-telegram",
  maintainers: [],
  tags: ["global"],
  installed: false,
  update_available: false,
  installable: true,
  ...over,
})

describe("filterCatalog", () => {
  const list = [
    entry({}),
    entry({ id: "weixin", package: "@theopenbee/channel-weixin", name: { en: "WeChat", zh: "微信" }, description: { zh: "扫码登录" }, tags: ["china"] }),
  ]

  it("matches id, any localized name and description", () => {
    expect(filterCatalog(list, "", null).map((p) => p.id)).toEqual(["telegram", "weixin"])
    expect(filterCatalog(list, "微信", null).map((p) => p.id)).toEqual(["weixin"])
    expect(filterCatalog(list, "bot api", null).map((p) => p.id)).toEqual(["telegram"])
    expect(filterCatalog(list, "WEIX", null).map((p) => p.id)).toEqual(["weixin"])
  })

  it("filters by tag", () => {
    expect(filterCatalog(list, "", "china").map((p) => p.id)).toEqual(["weixin"])
  })
})

describe("marketAction", () => {
  it("covers every button state", () => {
    expect(marketAction(entry({}), false)).toBe("install")
    expect(marketAction(entry({}), true)).toBe("installing")
    expect(marketAction(entry({ installable: false }), false)).toBe("incompatible")
    expect(marketAction(entry({ installed: true }), false)).toBe("configure")
    expect(marketAction(entry({ installed: true, update_available: true }), false)).toBe("update")
    expect(marketAction(entry({ installed: true, update_available: true, installable: false }), false)).toBe("configure")
  })
})

describe("pendingInstalls", () => {
  const op = (over: Partial<PluginOperation>): PluginOperation => ({
    id: "op",
    kind: "install",
    spec: "@theopenbee/channel-telegram@0.2.0",
    status: "running",
    created_at: "2026-10-26T00:00:00Z",
    ...over,
  })

  it("maps queued and running installs to their package", () => {
    const got = pendingInstalls([
      op({ id: "a" }),
      op({ id: "b", spec: "@theopenbee/channel-weixin", status: "queued" }),
      op({ id: "c", spec: "@theopenbee/channel-slack@1.0.0", status: "succeeded" }),
      op({ id: "d", kind: "uninstall", spec: "", plugin_id: "line" }),
    ])
    expect(Object.keys(got).sort()).toEqual(["@theopenbee/channel-telegram", "@theopenbee/channel-weixin"])
    expect(got["@theopenbee/channel-telegram"].id).toBe("a")
  })
})

describe("latestLink", () => {
  it("returns the last line that is a link", () => {
    expect(latestLink(["scan", "https://a.example/1", "waiting", "https://a.example/2 ", "scanned"])).toBe("https://a.example/2")
  })

  it("ignores links inside sentences", () => {
    expect(latestLink(["see https://docs.example for help"])).toBeNull()
  })
})

describe("localize", () => {
  it("prefers the language, then its base, then English", () => {
    expect(localize({ en: "Hi", zh: "你好" }, "zh-CN")).toBe("你好")
    expect(localize({ en: "Hi" }, "zh")).toBe("Hi")
    expect(localize({ fr: "Salut" }, "zh")).toBe("Salut")
    expect(localize(undefined, "en")).toBe("")
  })
})

describe("isTransitional", () => {
  it("is true while starting or retrying", () => {
    expect(isTransitional("starting")).toBe(true)
    expect(isTransitional("retrying")).toBe(true)
    expect(isTransitional("running")).toBe(false)
  })
})

describe("formatElapsed", () => {
  it("formats minutes and seconds", () => {
    expect(formatElapsed(0)).toBe("0:00")
    expect(formatElapsed(12_400)).toBe("0:12")
    expect(formatElapsed(125_000)).toBe("2:05")
    expect(formatElapsed(-5)).toBe("0:00")
  })
})
