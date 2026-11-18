import { afterEach, describe, expect, it, vi } from "vitest"
import {
  commandFor,
  hasCommand,
  neighborOf,
  registerCommand,
  requestComposerFocus,
  runCommand,
  SHORTCUTS,
  takeComposerFocus,
  type KeyContext,
} from "../shortcuts"

const pressed = (combo: string) => (shortcut: string) => shortcut === combo

const idle: KeyContext = { overlay: "none", editing: false, repeat: false }

describe("commandFor", () => {
  it("maps each shortcut to its command", () => {
    for (const [id, shortcut] of Object.entries(SHORTCUTS)) {
      expect(commandFor(pressed(shortcut), idle)).toBe(id)
    }
  })

  it("ignores keys without a command", () => {
    expect(commandFor(pressed("mod+j"), idle)).toBeNull()
  })

  it("only lets search and the shortcut list through their own dialogs", () => {
    const own: KeyContext = { ...idle, overlay: "own" }
    expect(commandFor(pressed(SHORTCUTS.quickSearch), own)).toBe("quickSearch")
    expect(commandFor(pressed(SHORTCUTS.showShortcuts), own)).toBe("showShortcuts")
    expect(commandFor(pressed(SHORTCUTS.nextConversation), own)).toBeNull()
    expect(commandFor(pressed(SHORTCUTS.newConversation), own)).toBeNull()
    expect(commandFor(pressed(SHORTCUTS.openSettings), own)).toBeNull()
  })

  it("lets nothing through other dialogs and menus", () => {
    const other: KeyContext = { ...idle, overlay: "other" }
    for (const shortcut of Object.values(SHORTCUTS)) {
      expect(commandFor(pressed(shortcut), other)).toBeNull()
    }
  })

  it("leaves Alt and the arrows to text fields other than the composer", () => {
    const editing: KeyContext = { ...idle, editing: true }
    expect(commandFor(pressed(SHORTCUTS.previousConversation), editing)).toBeNull()
    expect(commandFor(pressed(SHORTCUTS.nextConversation), editing)).toBeNull()
    expect(commandFor(pressed(SHORTCUTS.quickSearch), editing)).toBe("quickSearch")
    expect(commandFor(pressed(SHORTCUTS.newConversation), editing)).toBe("newConversation")
  })

  it("does not toggle dialogs on key repeat", () => {
    const repeat: KeyContext = { ...idle, repeat: true }
    expect(commandFor(pressed(SHORTCUTS.quickSearch), repeat)).toBeNull()
    expect(commandFor(pressed(SHORTCUTS.showShortcuts), repeat)).toBeNull()
    expect(commandFor(pressed(SHORTCUTS.nextConversation), repeat)).toBe("nextConversation")
  })

  it("uses distinct shortcuts", () => {
    const values = Object.values(SHORTCUTS)
    expect(new Set(values).size).toBe(values.length)
  })
})

describe("command registry", () => {
  const cleanups: (() => void)[] = []
  afterEach(() => cleanups.splice(0).forEach((cleanup) => cleanup()))

  it("runs the registered handler", () => {
    const run = vi.fn()
    cleanups.push(registerCommand("newConversation", run))
    expect(runCommand("newConversation")).toBe(true)
    expect(run).toHaveBeenCalledOnce()
  })

  it("reports commands nobody handles", () => {
    expect(hasCommand("nextConversation")).toBe(false)
    expect(runCommand("nextConversation")).toBe(false)
  })

  it("keeps a newer handler when an older one unregisters", () => {
    const older = vi.fn()
    const newer = vi.fn()
    const dropOlder = registerCommand("previousConversation", older)
    cleanups.push(registerCommand("previousConversation", newer))
    dropOlder()
    runCommand("previousConversation")
    expect(newer).toHaveBeenCalledOnce()
    expect(older).not.toHaveBeenCalled()
  })
})

describe("neighborOf", () => {
  const ids = ["bee", "worker:a", "worker:b"]

  it("steps through the list without wrapping", () => {
    expect(neighborOf(ids, "worker:a", 1)).toBe("worker:b")
    expect(neighborOf(ids, "worker:a", -1)).toBe("bee")
    expect(neighborOf(ids, "worker:b", 1)).toBeNull()
    expect(neighborOf(ids, "bee", -1)).toBeNull()
  })

  it("starts from an end when nothing is selected", () => {
    expect(neighborOf(ids, null, 1)).toBe("bee")
    expect(neighborOf(ids, null, -1)).toBe("worker:b")
    expect(neighborOf(ids, "worker:gone", 1)).toBe("bee")
  })

  it("returns null for an empty list", () => {
    expect(neighborOf([], null, 1)).toBeNull()
  })
})

describe("composer focus", () => {
  it("hands focus only to the requested conversation, once", () => {
    requestComposerFocus("worker:a")
    expect(takeComposerFocus("worker:b")).toBe(false)
    expect(takeComposerFocus("worker:a")).toBe(true)
    expect(takeComposerFocus("worker:a")).toBe(false)
  })
})
