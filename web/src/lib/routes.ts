import type { SettingsSection } from "@/hooks/use-settings-dialog"
import { BEE_CONVERSATION, conversationPath, workerConversationId } from "@/lib/conversations"
import type { ConversationId } from "@/lib/types"

export type PanelSection = "overview" | "constraints" | "permissions" | "env"

export type PanelState =
  | { mode: "worker"; section: PanelSection }
  | { mode: "new"; copyFrom?: string }
  | null

export type PanelView =
  | { mode: "worker"; workerId: string; section: PanelSection }
  | { mode: "new"; copyFrom?: string }
  | null

export interface LegacyTarget {
  to: string
  settings?: SettingsSection
}

const PANEL_SECTIONS: readonly string[] = ["overview", "constraints", "permissions", "env"] satisfies PanelSection[]
const PANEL_PARAM = "panel"
const COPY_PARAM = "copy"
const NEW_PANEL = "new"

export function isPanelSection(value: string | null): value is PanelSection {
  return value !== null && PANEL_SECTIONS.includes(value)
}

export function panelFromSearch(search: URLSearchParams): PanelState {
  const panel = search.get(PANEL_PARAM)
  if (panel === NEW_PANEL) {
    const copyFrom = search.get(COPY_PARAM)
    return copyFrom ? { mode: "new", copyFrom } : { mode: "new" }
  }
  return isPanelSection(panel) ? { mode: "worker", section: panel } : null
}

export function panelToSearch(panel: PanelState, current: URLSearchParams): URLSearchParams {
  const next = new URLSearchParams(current)
  next.delete(PANEL_PARAM)
  next.delete(COPY_PARAM)
  if (panel?.mode === "worker") next.set(PANEL_PARAM, panel.section)
  if (panel?.mode === "new") {
    next.set(PANEL_PARAM, NEW_PANEL)
    if (panel.copyFrom) next.set(COPY_PARAM, panel.copyFrom)
  }
  return next
}

export function panelView(param: string | undefined, panel: PanelState): PanelView {
  if (panel?.mode === "new") return panel
  if (panel?.mode !== "worker" || !param || param === BEE_CONVERSATION) return null
  return { mode: "worker", workerId: param, section: panel.section }
}

export function panelReplacesHistory(current: PanelState, next: PanelState): boolean {
  return current?.mode === "worker" && next?.mode === "worker"
}

function withPanel(pathname: string, panel: PanelState): string {
  const query = panelToSearch(panel, new URLSearchParams()).toString()
  return query ? `${pathname}?${query}` : pathname
}

export function workerPanelPath(workerId: string): string {
  return withPanel(conversationPath(workerConversationId(workerId)), { mode: "worker", section: "overview" })
}

export function legacyRedirect(pathname: string, search: string): LegacyTarget | null {
  const segments = pathname.split("/").filter(Boolean)
  const params = new URLSearchParams(search)
  const beePath = conversationPath(BEE_CONVERSATION)

  if (segments.length === 1 && segments[0] === "departments") return { to: beePath, settings: "departments" }
  if (segments[0] !== "workers" || segments.length > 2) return null
  if (segments.length === 1) return { to: beePath }
  if (segments[1] === "create") {
    const copyFrom = params.get(COPY_PARAM)
    return { to: withPanel(beePath, copyFrom ? { mode: "new", copyFrom } : { mode: "new" }) }
  }
  const tab = params.get("tab")
  const section = isPanelSection(tab) ? tab : "overview"
  return { to: withPanel(conversationPath(workerConversationId(segments[1])), { mode: "worker", section }) }
}

export function resolveConversationRoute(param: string | undefined, workerIds: Set<string>): ConversationId | null {
  if (param === BEE_CONVERSATION) return BEE_CONVERSATION
  if (!param || !workerIds.has(param)) return null
  return workerConversationId(param)
}
