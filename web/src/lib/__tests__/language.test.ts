import { describe, expect, it } from "vitest"
import { getStoredLanguage, normalizeLanguage, resolveLanguage, storeLanguage } from "../language"

function memoryStorage(initial: Record<string, string> = {}) {
  const values = new Map(Object.entries(initial))
  return {
    getItem: (key: string) => values.get(key) ?? null,
    setItem: (key: string, value: string) => {
      values.set(key, value)
    },
  }
}

describe("normalizeLanguage", () => {
  it.each([
    ["en", "en"],
    ["zh", "zh"],
    ["zh-CN", "zh"],
    ["ZH_cn", "zh"],
    ["en-US", "en"],
  ])("maps %s to %s", (input, expected) => {
    expect(normalizeLanguage(input)).toBe(expected)
  })

  it.each([["fr"], [""], [null], [undefined], [42]])("rejects %s", (input) => {
    expect(normalizeLanguage(input)).toBeNull()
  })
})

describe("stored language", () => {
  it("reads nothing before a choice is made", () => {
    expect(getStoredLanguage(memoryStorage())).toBeNull()
  })

  it("round-trips a choice", () => {
    const storage = memoryStorage()
    storeLanguage("zh", storage)

    expect(getStoredLanguage(storage)).toBe("zh")
  })

  it("ignores an unsupported stored value", () => {
    expect(getStoredLanguage(memoryStorage({ language: "fr" }))).toBeNull()
  })
})

describe("resolveLanguage", () => {
  it("keeps a supported server language", () => {
    expect(resolveLanguage("zh")).toBe("zh")
  })

  it.each([["fr"], [undefined]])("falls back to English for %s", (input) => {
    expect(resolveLanguage(input)).toBe("en")
  })
})
