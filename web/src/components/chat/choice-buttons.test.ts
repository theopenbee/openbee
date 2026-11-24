import { createElement } from "react"
import { renderToStaticMarkup } from "react-dom/server"
import { I18nextProvider } from "react-i18next"
import { describe, expect, it } from "vitest"
import type { ChatButton } from "@/lib/types"
import { testI18n } from "@/test/i18n"
import { ChoiceButtons } from "./choice-buttons"

const buttons: ChatButton[] = [{ label: "Confirm", style: "danger" }, { label: "Cancel" }]

const render = (choice?: number, lng: "en" | "zh" = "en") =>
  renderToStaticMarkup(
    createElement(
      I18nextProvider,
      { i18n: testI18n(lng) },
      createElement(ChoiceButtons, { buttons, choice, onChoose: async () => {} }),
    ),
  )

const buttonTags = (html: string) => html.match(/<button[^>]*>/g) ?? []

describe("ChoiceButtons", () => {
  it("shows every button enabled before a choice", () => {
    const html = render()
    const tags = buttonTags(html)

    expect(tags).toHaveLength(2)
    expect(tags.every((tag) => !tag.includes("disabled"))).toBe(true)
    expect(html).toContain("Confirm")
    expect(html).toContain("Cancel")
    expect(html).not.toContain("lucide-check")
  })

  it("disables every button and marks the chosen one after a choice", () => {
    const html = render(1)
    const tags = buttonTags(html)

    expect(tags.every((tag) => tag.includes("disabled"))).toBe(true)
    expect(tags[1]).toContain('aria-label="Cancel, Chosen"')
    expect(tags[0]).not.toContain("aria-label")
    expect(html.match(/lucide-check/g)).toHaveLength(1)
    expect(html.indexOf("lucide-check")).toBeGreaterThan(html.indexOf(tags[1]))
  })

  it("names the chosen button in the current language", () => {
    expect(buttonTags(render(0, "zh"))[0]).toContain('aria-label="Confirm, 已选择"')
  })
})
