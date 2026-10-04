import { describe, it, expect } from "vitest"
import { resolveCrumbs } from "../breadcrumb-config"

describe("resolveCrumbs", () => {
  it("names a top-level page on its own", () => {
    expect(resolveCrumbs("/")).toEqual([{ labelKey: "nav.dashboard", to: undefined }])
    expect(resolveCrumbs("/sessions")).toEqual([{ labelKey: "nav.sessions", to: undefined }])
  })

  it("leads a grouped page with its sidebar group", () => {
    expect(resolveCrumbs("/workers")).toEqual([
      { labelKey: "nav.digitalEmployees" },
      { labelKey: "nav.workers", to: undefined },
    ])
    expect(resolveCrumbs("/settings")).toEqual([
      { labelKey: "nav.systemConfig" },
      { labelKey: "nav.systemSettings", to: undefined },
    ])
  })

  it("links the parent trail for detail pages", () => {
    expect(resolveCrumbs("/workers/abc")).toEqual([
      { labelKey: "nav.digitalEmployees" },
      { labelKey: "nav.workers", to: "/workers" },
      { labelKey: "breadcrumb.detail" },
    ])
    expect(resolveCrumbs("/sessions/detail", "?session_id=s1")).toEqual([
      { labelKey: "nav.sessions", to: "/sessions" },
      { labelKey: "breadcrumb.detail" },
    ])
  })

  it("treats a trailing slash as the same page", () => {
    expect(resolveCrumbs("/workers/")).toEqual(resolveCrumbs("/workers"))
    expect(resolveCrumbs("/sessions/")).toEqual(resolveCrumbs("/sessions"))
    expect(resolveCrumbs("/workers/abc/")).toEqual(resolveCrumbs("/workers/abc"))
  })

  it("names the create route after the copy flow when ?copy is set", () => {
    expect(resolveCrumbs("/workers/create")[2]).toEqual({ labelKey: "workers.createWorker" })
    expect(resolveCrumbs("/workers/create", "?copy=w1")[2]).toEqual({ labelKey: "workers.copyWorker" })
  })

  it("returns no crumbs for unknown paths", () => {
    expect(resolveCrumbs("/nope")).toEqual([])
    expect(resolveCrumbs("/nope/deeper")).toEqual([])
  })
})
