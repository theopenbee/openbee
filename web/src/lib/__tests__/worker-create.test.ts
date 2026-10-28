import { describe, expect, it } from "vitest"
import { countAdvancedSettings } from "../worker-create"

const empty = { constraints: "", workDir: "", engineArgs: "", scopes: [] }

describe("countAdvancedSettings", () => {
  it("returns zero when nothing is set", () => {
    expect(countAdvancedSettings(empty)).toBe(0)
  })

  it("ignores whitespace-only text", () => {
    expect(countAdvancedSettings({ ...empty, constraints: "  ", workDir: "\n", engineArgs: " " })).toBe(0)
  })

  it("counts each filled text setting", () => {
    expect(countAdvancedSettings({ ...empty, constraints: "be brief", workDir: "/tmp/w" })).toBe(2)
    expect(countAdvancedSettings({ ...empty, engineArgs: "--effort high" })).toBe(1)
  })

  it("counts permissions once however many scopes are selected", () => {
    expect(countAdvancedSettings({ ...empty, scopes: ["read:workers", "read:tasks"] })).toBe(1)
  })

  it("counts every setting when all are filled", () => {
    expect(
      countAdvancedSettings({ constraints: "c", workDir: "/w", engineArgs: "--x", scopes: ["read:tasks"] }),
    ).toBe(4)
  })
})
