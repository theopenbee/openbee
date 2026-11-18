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

const OVER_DIALOGS: ReadonlySet<CommandId> = new Set(["quickSearch", "showShortcuts"])

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

export function commandFor(
  matches: (shortcut: string) => boolean,
  dialogOpen: boolean,
): CommandId | null {
  for (const [id, shortcut] of Object.entries(SHORTCUTS) as [CommandId, string][]) {
    if (!matches(shortcut)) continue
    return dialogOpen && !OVER_DIALOGS.has(id) ? null : id
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

export function isEditableTarget(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false
  return target.isContentEditable || target.tagName === "INPUT" || target.tagName === "TEXTAREA" || target.tagName === "SELECT"
}

export function isDialogOpen(): boolean {
  return document.querySelector('[role="dialog"], [role="alertdialog"]') !== null
}
