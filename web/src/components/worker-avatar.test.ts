import { createElement } from "react"
import { renderToStaticMarkup } from "react-dom/server"
import { describe, expect, it } from "vitest"
import { WorkerAvatar } from "./worker-avatar"

const markup = (status?: string) => renderToStaticMarkup(createElement(WorkerAvatar, { workerId: "w1", name: "Lin", status }))

describe("WorkerAvatar", () => {
  it("draws a status dot when a status is given", () => {
    expect(markup("idle")).toContain("bg-success")
  })

  it("draws no status dot without a status", () => {
    const html = markup()
    expect(html).not.toContain("ring-surface")
    expect(html).not.toContain("title=")
  })
})
