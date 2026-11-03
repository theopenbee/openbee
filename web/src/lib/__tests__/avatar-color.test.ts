import { describe, expect, it } from "vitest"
import { avatarColorClass, avatarColorIndex } from "../avatar-color"

function seededHex(seed: number, length: number): string {
  let state = seed >>> 0
  let out = ""
  for (let i = 0; i < length; i++) {
    state = (Math.imul(state, 1664525) + 1013904223) >>> 0
    out += (state >>> 28).toString(16)
  }
  return out
}

function seededUuid(seed: number): string {
  const hex = seededHex(seed, 32)
  return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20)}`
}

describe("avatarColorIndex", () => {
  it("is stable and in range", () => {
    expect(avatarColorIndex("")).toBe(6)
    expect(avatarColorIndex("a")).toBe(5)
    expect(avatarColorIndex("foobar")).toBe(1)
    expect(avatarColorIndex("a")).toBe(avatarColorIndex("a"))
    for (let seed = 1; seed <= 200; seed++) {
      const index = avatarColorIndex(seededUuid(seed))
      expect(Number.isInteger(index)).toBe(true)
      expect(index).toBeGreaterThanOrEqual(1)
      expect(index).toBeLessThanOrEqual(8)
    }
  })

  it("hashes UTF-16 code units", () => {
    expect(avatarColorIndex("工")).toBe(1)
    expect(avatarColorIndex("🐝")).toBe(8)
  })

  it("spreads ids", () => {
    const seen = new Set<number>()
    for (let seed = 1; seed <= 50; seed++) seen.add(avatarColorIndex(seededUuid(seed)))
    expect(seen.size).toBeGreaterThanOrEqual(6)
  })
})

describe("avatarColorClass", () => {
  it("maps the index to a bg-avatar class", () => {
    expect(avatarColorClass("a")).toBe("bg-avatar-5")
    expect(avatarColorClass("foobar")).toBe("bg-avatar-1")
    expect(avatarColorClass("")).toBe("bg-avatar-6")
  })
})
