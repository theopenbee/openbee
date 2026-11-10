import { describe, expect, it } from "vitest"
import {
  containScale,
  fitScale,
  isLongImage,
  MAX_ZOOM,
  stepZoom,
  toggleZoom,
  wheelZoomFactor,
  zoomPercent,
  zoomRange,
} from "../image-zoom"

const box = { width: 1000, height: 800 }

describe("fitScale", () => {
  it("never enlarges a small image", () => {
    expect(fitScale({ width: 400, height: 300 }, box)).toBe(1)
  })

  it("shrinks a large image to fit the box", () => {
    expect(fitScale({ width: 2000, height: 1000 }, box)).toBe(0.5)
    expect(fitScale({ width: 1000, height: 1600 }, box)).toBe(0.5)
  })

  it("fits a long image to the width only", () => {
    expect(isLongImage({ width: 1170, height: 8000 })).toBe(true)
    expect(fitScale({ width: 2000, height: 8000 }, box)).toBe(0.5)
    expect(fitScale({ width: 500, height: 8000 }, box)).toBe(1)
  })

  it("treats exactly twice as tall as a normal image", () => {
    expect(isLongImage({ width: 500, height: 1000 })).toBe(false)
    expect(fitScale({ width: 500, height: 1000 }, box)).toBe(0.8)
  })
})

describe("zoomRange", () => {
  it("goes from 10% to 800%", () => {
    expect(zoomRange({ width: 400, height: 300 }, box)).toEqual([0.1, MAX_ZOOM])
  })

  it("lets a huge image zoom out until it fits", () => {
    const [min] = zoomRange({ width: 1000, height: 16000 }, box)
    expect(min).toBe(containScale({ width: 1000, height: 16000 }, box))
    expect(min).toBe(0.05)
  })
})

describe("stepZoom", () => {
  const range: [number, number] = [0.1, 8]

  it("moves to the next preset", () => {
    expect(stepZoom(1, 1, range)).toBe(1.25)
    expect(stepZoom(1, -1, range)).toBe(0.75)
    expect(stepZoom(0.37, 1, range)).toBe(0.5)
    expect(stepZoom(0.37, -1, range)).toBe(0.25)
  })

  it("stays within the range", () => {
    expect(stepZoom(8, 1, range)).toBe(8)
    expect(stepZoom(0.1, -1, range)).toBe(0.1)
    expect(stepZoom(0.1, -1, [0.05, 8])).toBe(0.05)
    expect(stepZoom(0.05, 1, [0.05, 8])).toBe(0.1)
  })
})

describe("toggleZoom", () => {
  it("goes to 100% from the fit and back", () => {
    expect(toggleZoom(0.4, 0.4)).toBe(1)
    expect(toggleZoom(1, 0.4)).toBe(0.4)
    expect(toggleZoom(3, 0.4)).toBe(0.4)
  })

  it("doubles an image that already shows at full size", () => {
    expect(toggleZoom(1, 1)).toBe(2)
    expect(toggleZoom(2, 1)).toBe(1)
  })
})

describe("wheelZoomFactor", () => {
  it("follows a touchpad pinch smoothly", () => {
    expect(wheelZoomFactor(-10, 0)).toBeCloseTo(Math.exp(0.1))
    expect(wheelZoomFactor(10, 0)).toBeCloseTo(Math.exp(-0.1))
  })

  it("zooms a mouse wheel notch by a fixed step", () => {
    expect(wheelZoomFactor(-100, 0)).toBe(1.25)
    expect(wheelZoomFactor(120, 0)).toBe(0.8)
    expect(wheelZoomFactor(-3, 1)).toBe(1.25)
  })
})

describe("zoomPercent", () => {
  it("rounds to a whole percent", () => {
    expect(zoomPercent(1)).toBe("100%")
    expect(zoomPercent(0.3333)).toBe("33%")
  })
})
