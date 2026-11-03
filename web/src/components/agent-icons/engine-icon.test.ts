import { createElement } from "react"
import { renderToStaticMarkup } from "react-dom/server"
import { describe, expect, it } from "vitest"
import { EngineIcon } from "./engine-icon"

const render = (engine?: string | null) => renderToStaticMarkup(createElement(EngineIcon, { engine }))

describe("EngineIcon", () => {
  it("renders nothing without an engine", () => {
    expect(render(undefined)).toBe("")
    expect(render(null)).toBe("")
    expect(render("")).toBe("")
  })

  it("renders a distinct brand icon for each built-in engine", () => {
    const icons = ["claude", "codex", "pi"].map(render)
    icons.forEach((markup) => expect(markup).toContain("<svg"))
    expect(new Set(icons).size).toBe(3)
  })

  it("renders a generic icon for any other engine id", () => {
    const generic = render("my-engine")
    expect(generic).toContain("<svg")
    expect(["claude", "codex", "pi"].map(render)).not.toContain(generic)
    expect(render("other-engine")).toBe(generic)
  })

  it("exposes a title as an accessible name", () => {
    const markup = renderToStaticMarkup(createElement(EngineIcon, { engine: "my-engine", title: "My engine" }))
    expect(markup).toContain('aria-label="My engine"')
    expect(markup).toContain('role="img"')
  })
})
