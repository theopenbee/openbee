import { createElement } from "react"
import { renderToStaticMarkup } from "react-dom/server"
import { I18nextProvider } from "react-i18next"
import { describe, expect, it } from "vitest"
import { testI18n } from "@/test/i18n"
import { ActivityIndicator } from "./activity-indicator"

const render = (label: string | null) =>
  renderToStaticMarkup(
    createElement(
      I18nextProvider,
      { i18n: testI18n("en") },
      createElement(ActivityIndicator, { label, avatar: null, onShowExecution: () => {} }),
    ),
  )

const statusRegion = (html: string) => {
  const match = html.match(/<(\w+)[^>]*role="status"[^>]*>([\s\S]*?)<\/\1>/)
  if (!match) throw new Error(`no status region in ${html}`)
  return match[2]
}

describe("ActivityIndicator", () => {
  it("announces the status without the process button", () => {
    const html = render("Alice is working")
    expect(statusRegion(html)).toContain("Alice is working")
    expect(statusRegion(html)).not.toContain("View process")
    expect(html).toContain("View process")
  })

  it("keeps an empty status region while idle", () => {
    const html = render(null)
    expect(statusRegion(html)).toBe("")
    expect(html).not.toContain("View process")
  })
})
