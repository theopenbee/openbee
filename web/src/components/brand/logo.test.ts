import { readFileSync } from "node:fs"
import { resolve } from "node:path"
import { createElement, type ComponentType } from "react"
import { renderToStaticMarkup } from "react-dom/server"
import { describe, expect, it } from "vitest"
import { LogoFull, LogoMark, LogoMarkSmall } from "./logo"

const pathsOf = (markup: string) => [...markup.matchAll(/ d="([^"]+)"/g)].map((m) => m[1])
const rendered = (component: ComponentType) => pathsOf(renderToStaticMarkup(createElement(component)))
const publicFile = (name: string) => pathsOf(readFileSync(resolve(__dirname, "../../../public", name), "utf8"))

describe("brand logo components", () => {
  it("draw the same mark as public/logo-mark.svg", () => {
    expect(rendered(LogoMark)).toEqual(publicFile("logo-mark.svg"))
  })

  it("draw the small mark from public/favicon.svg", () => {
    expect(rendered(LogoMarkSmall)).toEqual(publicFile("favicon.svg"))
    expect(rendered(LogoMarkSmall)).not.toEqual(rendered(LogoMark))
  })

  it("draw the same lockup as public/logo-full.svg", () => {
    expect(rendered(LogoFull)).toEqual(publicFile("logo-full.svg"))
  })

  it("size the lockup mark to the wordmark and centre it on the wordmark", () => {
    const [markPath] = rendered(LogoFull)
    const points = [...markPath.matchAll(/(-?[\d.]+) (-?[\d.]+)(?=[LAMZ])/g)].map((m) => [Number(m[1]), Number(m[2])])
    const ys = points.map(([, y]) => y)
    expect(Math.max(...points.map(([x]) => x))).toBeLessThanOrEqual(340)
    expect((Math.min(...ys) + Math.max(...ys)) / 2).toBeCloseTo(210, 0)
  })

  it("paint every shape with the brand token", () => {
    for (const component of [LogoMark, LogoMarkSmall, LogoFull]) {
      const fills = [...renderToStaticMarkup(createElement(component)).matchAll(/ fill="([^"]+)"/g)].map((m) => m[1])
      expect(new Set(fills)).toEqual(new Set(["var(--brand)"]))
    }
  })
})
