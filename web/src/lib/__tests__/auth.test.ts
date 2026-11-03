import { describe, expect, it } from "vitest"
import { tokenExpiresSoon } from "../auth"

const NOW = 1_800_000_000_000

const base64url = (value: string) => btoa(value).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "")

const jwt = (payload: Record<string, unknown>) => `${base64url('{"alg":"HS256"}')}.${base64url(JSON.stringify(payload))}.sig`

describe("tokenExpiresSoon", () => {
  it("keeps a token that is valid well past now", () => {
    expect(tokenExpiresSoon(jwt({ exp: NOW / 1000 + 3600 }), NOW)).toBe(false)
  })

  it("decodes base64url payloads", () => {
    const token = jwt({ exp: NOW / 1000 + 3600, sub: "??>???" })

    expect(token).toMatch(/[-_]/)
    expect(tokenExpiresSoon(token, NOW)).toBe(false)
  })

  it.each([
    ["an expired token", jwt({ exp: NOW / 1000 - 1 })],
    ["a token expiring within a minute", jwt({ exp: NOW / 1000 + 30 })],
    ["a token without exp", jwt({ sub: "u1" })],
    ["a token with a malformed payload", "a.not-json.c"],
    ["a value that is not a JWT", "opaque"],
    ["a missing token", null],
  ])("refreshes %s", (_, token) => {
    expect(tokenExpiresSoon(token, NOW)).toBe(true)
  })
})
