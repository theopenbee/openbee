import { describe, expect, it } from "vitest"
import { createElement } from "react"
import { renderToStaticMarkup } from "react-dom/server"
import { Streamdown } from "streamdown"
import { CHAT_REMARK_PLUGINS } from "../markdown"

function render(markdown: string) {
  return renderToStaticMarkup(
    createElement(Streamdown, { mode: "static", remarkPlugins: CHAT_REMARK_PLUGINS }, markdown)
  )
}

describe("CHAT_REMARK_PLUGINS", () => {
  it("renders single newlines as line breaks", () => {
    expect(render("first\nsecond")).toContain("first<br/>")
  })

  it("renders tables with leading pipes", () => {
    expect(render("| Page | Status |\n| --- | --- |\n| Chat | Done |")).toContain("<table")
  })

  it("renders tables without leading pipes", () => {
    expect(render("a | b\n--- | ---\n1 | 2")).toContain("<table")
  })

  it("renders tables inside blockquotes", () => {
    const html = render("> | a |\n> |---|\n> | 1 |")
    expect(html).toContain("<blockquote")
    expect(html).toContain("<table")
  })

  it("keeps tight lists tight", () => {
    const html = render("- a\n- b")
    expect(html).toContain("<ul")
    expect(html).not.toContain("<p>")
  })
})
