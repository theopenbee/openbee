import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

type Entry = { target: Element }

class FakeResizeObserver {
  static instances: FakeResizeObserver[] = []
  observed = new Set<Element>()

  constructor(public callback: (entries: Entry[]) => void) {
    FakeResizeObserver.instances.push(this)
  }

  observe(el: Element) {
    this.observed.add(el)
  }

  unobserve(el: Element) {
    this.observed.delete(el)
  }

  disconnect() {
    this.observed.clear()
  }

  trigger(...targets: Element[]) {
    this.callback(targets.map((target) => ({ target })))
  }
}

const el = () => ({}) as Element

describe("observeResize", () => {
  beforeEach(() => {
    FakeResizeObserver.instances = []
    vi.stubGlobal("ResizeObserver", FakeResizeObserver)
    vi.resetModules()
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it("shares one observer across elements", async () => {
    const { observeResize } = await import("../resize-observer")
    const a = el()
    const b = el()
    observeResize(a, () => {})
    observeResize(b, () => {})

    expect(FakeResizeObserver.instances).toHaveLength(1)
    expect(FakeResizeObserver.instances[0].observed).toEqual(new Set([a, b]))
  })

  it("routes each entry to the callback registered for its element", async () => {
    const { observeResize } = await import("../resize-observer")
    const a = el()
    const b = el()
    const onA = vi.fn()
    const onB = vi.fn()
    observeResize(a, onA)
    observeResize(b, onB)

    FakeResizeObserver.instances[0].trigger(b)

    expect(onA).not.toHaveBeenCalled()
    expect(onB).toHaveBeenCalledTimes(1)
  })

  it("stops observing and notifying after cleanup", async () => {
    const { observeResize } = await import("../resize-observer")
    const a = el()
    const onA = vi.fn()
    const stop = observeResize(a, onA)

    stop()
    FakeResizeObserver.instances[0].trigger(a)

    expect(FakeResizeObserver.instances[0].observed.has(a)).toBe(false)
    expect(onA).not.toHaveBeenCalled()
  })
})
