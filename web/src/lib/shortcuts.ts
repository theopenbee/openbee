export type CommandId =
  | "quickSearch"
  | "showShortcuts"
  | "openSettings"
  | "previousConversation"
  | "nextConversation"
  | "newConversation"

export const SHORTCUTS: Record<CommandId, string> = {
  quickSearch: "mod+k",
  showShortcuts: "mod+/",
  openSettings: "mod+,",
  previousConversation: "alt+up",
  nextConversation: "alt+down",
  newConversation: "mod+shift+o",
}

const TOGGLES: ReadonlySet<CommandId> = new Set(["quickSearch", "showShortcuts"])
const ARROWS: ReadonlySet<CommandId> = new Set(["previousConversation", "nextConversation"])

export type Overlay = "none" | "own" | "other"

export interface KeyContext {
  overlay: Overlay
  editing: boolean
  repeat: boolean
}

type Handler = () => void

const handlers = new Map<CommandId, Handler>()
const listeners = new Set<() => void>()
let version = 0

function changed() {
  version++
  listeners.forEach((listener) => listener())
}

export function registerCommand(id: CommandId, run: Handler): () => void {
  handlers.set(id, run)
  changed()
  return () => {
    if (handlers.get(id) !== run) return
    handlers.delete(id)
    changed()
  }
}

export function runCommand(id: CommandId): boolean {
  const run = handlers.get(id)
  if (!run) return false
  run()
  return true
}

export function hasCommand(id: CommandId): boolean {
  return handlers.has(id)
}

export function subscribeCommands(listener: () => void): () => void {
  listeners.add(listener)
  return () => {
    listeners.delete(listener)
  }
}

export function commandsVersion(): number {
  return version
}

function allowed(id: CommandId, { overlay, editing, repeat }: KeyContext): boolean {
  if (overlay === "other") return false
  if (TOGGLES.has(id)) return !repeat
  return overlay === "none" && !(editing && ARROWS.has(id))
}

export function commandFor(matches: (shortcut: string) => boolean, context: KeyContext): CommandId | null {
  for (const [id, shortcut] of Object.entries(SHORTCUTS) as [CommandId, string][]) {
    if (matches(shortcut)) return allowed(id, context) ? id : null
  }
  return null
}

export function neighborOf<T>(items: readonly T[], current: T | null, step: 1 | -1): T | null {
  if (items.length === 0) return null
  const index = current === null ? -1 : items.indexOf(current)
  if (index === -1) return step === 1 ? items[0] : items[items.length - 1]
  return items[index + step] ?? null
}

let composerFocusTarget: string | null = null
const composerListeners = new Set<() => void>()

export function requestComposerFocus(target: string) {
  composerFocusTarget = target
  composerListeners.forEach((listener) => listener())
}

export function takeComposerFocus(target: string): boolean {
  if (composerFocusTarget !== target) return false
  composerFocusTarget = null
  return true
}

export function subscribeComposerFocus(listener: () => void): () => void {
  composerListeners.add(listener)
  return () => {
    composerListeners.delete(listener)
  }
}

export const COMPOSER_ATTR = "data-composer"

const OVERLAYS = '[role="dialog"], [role="alertdialog"], [role="menu"], [role="listbox"], [data-streamdown="link-safety-modal"]'

export function isEditableTarget(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false
  return target.isContentEditable || target.tagName === "INPUT" || target.tagName === "TEXTAREA" || target.tagName === "SELECT"
}

export function isEditingOutsideComposer(target: EventTarget | null): boolean {
  return isEditableTarget(target) && !(target as HTMLElement).closest(`[${COMPOSER_ATTR}]`)
}

export function isOverlayOpen(): boolean {
  return document.querySelector(OVERLAYS) !== null
}
