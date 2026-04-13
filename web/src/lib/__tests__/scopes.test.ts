import { describe, expect, it } from "vitest"
import { parseScopes, serializeScopes, KNOWN_SCOPES } from "../scopes"

describe("parseScopes", () => {
  it("returns empty array for empty string", () => {
    expect(parseScopes("")).toEqual([])
  })

  it("splits comma-separated scopes", () => {
    expect(parseScopes("read:workers,read:tasks")).toEqual(["read:workers", "read:tasks"])
  })

  it("trims whitespace around entries", () => {
    expect(parseScopes("read:workers, read:tasks")).toEqual(["read:workers", "read:tasks"])
  })

  it("filters out blank entries", () => {
    expect(parseScopes(",read:workers,")).toEqual(["read:workers"])
  })
})

describe("serializeScopes", () => {
  it("joins scopes with comma", () => {
    expect(serializeScopes(["read:workers", "read:tasks"])).toBe("read:workers,read:tasks")
  })

  it("returns empty string for empty array", () => {
    expect(serializeScopes([])).toBe("")
  })

  it("round-trips through serialize then parse", () => {
    const scopes = ["read:workers", "read:tasks"]
    expect(parseScopes(serializeScopes(scopes))).toEqual(scopes)
  })
})

describe("KNOWN_SCOPES", () => {
  it("contains exactly 3 entries", () => {
    expect(KNOWN_SCOPES).toHaveLength(3)
  })

  it("all entries have id, titleKey, descriptionKey", () => {
    for (const s of KNOWN_SCOPES) {
      expect(s.id).toBeTruthy()
      expect(s.titleKey).toBeTruthy()
      expect(s.descriptionKey).toBeTruthy()
    }
  })
})
