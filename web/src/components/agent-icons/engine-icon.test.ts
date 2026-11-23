import { createElement } from "react"
import { renderToStaticMarkup } from "react-dom/server"
import { describe, expect, it } from "vitest"
import { EngineIcon, engineIconMask } from "./engine-icon"

const svg = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><path fill="currentColor" d="M0 0h24v24H0z"/></svg>`

const render = (icon?: string, title?: string) => renderToStaticMarkup(createElement(EngineIcon, { icon, title }))

describe("engineIconMask", () => {
  it("wraps the SVG in an encoded data URL", () => {
    const mask = engineIconMask(svg)
    expect(mask.startsWith('url("data:image/svg+xml;charset=utf-8,')).toBe(true)
    expect(mask).not.toContain("<")
    expect(decodeURIComponent(mask.slice(mask.indexOf(",") + 1, -2))).toBe(svg)
  })

  it("keeps quotes inside the SVG from closing the URL", () => {
    const mask = engineIconMask(`<svg xmlns="http://www.w3.org/2000/svg"><path d='M0 0")'/></svg>`)
    expect(mask.slice(5, -2)).not.toContain('"')
  })
})

describe("EngineIcon", () => {
  it("draws the plugin icon as a mask in the text color", () => {
    const markup = render(svg)
    expect(markup).toMatch(/^<span /)
    expect(markup).toContain("bg-current")
    expect(markup).toContain("mask-image:url(")
    expect(markup).toContain(encodeURIComponent(svg))
    expect(markup).not.toContain("<svg xmlns")
  })

  it("falls back to a generic icon without a plugin icon", () => {
    const generic = render(undefined)
    expect(generic).toContain("<svg")
    expect(generic).not.toContain("mask-image")
    expect(render("")).toBe(generic)
  })

  it("exposes a title as an accessible name", () => {
    for (const markup of [render(svg, "My engine"), render(undefined, "My engine")]) {
      expect(markup).toContain('aria-label="My engine"')
      expect(markup).toContain('role="img"')
    }
    expect(render(svg)).toContain('aria-hidden="true"')
  })
})
