import { createElement } from "react"
import { renderToStaticMarkup } from "react-dom/server"
import { describe, expect, it } from "vitest"
import { LogoMark, LogoMarkSmall } from "@/components/brand/logo"
import { BeeAvatar } from "./bee-avatar"

const markup = (size: "sm" | "default" | "lg" | "xl") => renderToStaticMarkup(createElement(BeeAvatar, { size }))
const inner = (component: typeof LogoMark) => renderToStaticMarkup(createElement(component)).match(/ d="([^"]+)"/)![1]

describe("BeeAvatar", () => {
  it("uses the wide-gap mark when the mark is drawn under 24px", () => {
    expect(markup("sm")).toContain(inner(LogoMarkSmall))
    expect(markup("default")).toContain(inner(LogoMarkSmall))
  })

  it("uses the standard mark at larger sizes", () => {
    expect(markup("lg")).toContain(inner(LogoMark))
    expect(markup("xl")).toContain(inner(LogoMark))
  })
})
