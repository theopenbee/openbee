import { createElement } from "react"
import { renderToStaticMarkup } from "react-dom/server"
import { describe, expect, it } from "vitest"
import { PluginAvatar } from "./plugin-ui"

const svg = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><path fill="currentColor" d="M0 0h24v24H0z"/></svg>`

const render = (icon?: string, size?: "sm" | "md" | "lg") => renderToStaticMarkup(createElement(PluginAvatar, { name: "Telegram Bot", icon, size }))

describe("PluginAvatar", () => {
  it("shows the initials without an icon", () => {
    const markup = render()
    expect(markup).toContain(">TB<")
    expect(markup).not.toContain("mask-image")
    expect(render("")).toBe(markup)
  })

  it("draws the logo as a mask inside the avatar circle", () => {
    const markup = render(svg)
    expect(markup).toContain("mask-image:url(")
    expect(markup).toContain(encodeURIComponent(svg))
    expect(markup).toContain("bg-current")
    expect(markup).not.toContain(">TB<")
  })

  it("keeps the avatar size and stays hidden from assistive tech", () => {
    for (const size of ["sm", "md", "lg"] as const) {
      const initials = render(undefined, size)
      const logo = render(svg, size)
      const sizeClass = (markup: string) => markup.match(/class="([^"]*)"/)?.[1].split(" ").slice(0, 2).join(" ")
      expect(sizeClass(logo)).toBe(sizeClass(initials))
      expect(logo).toMatch(/^<span[^>]*aria-hidden="true"/)
      expect(logo).not.toContain('role="img"')
    }
  })
})
