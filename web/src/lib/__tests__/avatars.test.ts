import { beforeEach, describe, expect, it, vi } from "vitest"
import { avatarLabel, avatarUrl, pickRandomAvatar } from "../avatars"
import type { AvatarPack } from "../types"

const packs: AvatarPack[] = [
  { id: "mfers", name: { en: "mfers" }, avatars: ["mfers/00001", "mfers/00002"] },
  { id: "nouns", name: { en: "Nouns" }, avatars: ["nouns/7"] },
]

describe("avatarUrl", () => {
  beforeEach(() => {
    const store = new Map<string, string>()
    vi.stubGlobal("localStorage", {
      getItem: (key: string) => store.get(key) ?? null,
      setItem: (key: string, value: string) => store.set(key, value),
      removeItem: (key: string) => store.delete(key),
    })
  })

  it("points at the avatar endpoint", () => {
    expect(avatarUrl("mfers/00042")).toBe("/api/avatars/mfers/00042")
  })

  it("adds the access token for image requests", () => {
    localStorage.setItem("openbee_access_token", "tok en")
    expect(avatarUrl("mfers/00042")).toBe("/api/avatars/mfers/00042?token=tok%20en")
  })
})

describe("avatarLabel", () => {
  it("drops the plugin id", () => {
    expect(avatarLabel("mfers/00042")).toBe("00042")
  })
})

describe("pickRandomAvatar", () => {
  it("prefers avatars nobody uses", () => {
    const used = new Set(["mfers/00001", "nouns/7"])
    for (const r of [0, 0.5, 0.99]) expect(pickRandomAvatar(packs, used, () => r)).toBe("mfers/00002")
  })

  it("falls back to any avatar when all are taken", () => {
    const used = new Set(["mfers/00001", "mfers/00002", "nouns/7"])
    expect(pickRandomAvatar(packs, used, () => 0)).toBe("mfers/00001")
    expect(pickRandomAvatar(packs, used, () => 0.999)).toBe("nouns/7")
  })

  it("returns nothing without packs", () => {
    expect(pickRandomAvatar([], new Set())).toBe("")
  })
})
