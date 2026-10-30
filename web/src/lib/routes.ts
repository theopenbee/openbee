import { matchPath, parsePath } from "react-router-dom"
import { BEE_CONVERSATION, conversationPath, workerConversationId, workerIdOf } from "@/lib/conversations"
import type { ConversationId } from "@/lib/types"

export type PanelSection = "constraints" | "permissions" | "env"

export type PanelState =
  | { mode: "worker"; section: PanelSection }
  | { mode: "new" }
  | null

export type PanelView =
  | { mode: "worker"; workerId: string; section: PanelSection }
  | { mode: "new" }
  | null

const PANEL_SECTIONS: readonly string[] = ["constraints", "permissions", "env"] satisfies PanelSection[]
export const DEFAULT_PANEL_SECTION: PanelSection = "constraints"
const PANEL_PARAM = "panel"
const NEW_PANEL = "new"

export function isPanelSection(value: string | null): value is PanelSection {
  return value !== null && PANEL_SECTIONS.includes(value)
}

export function panelFromSearch(search: URLSearchParams): PanelState {
  const panel = search.get(PANEL_PARAM)
  if (panel === NEW_PANEL) return { mode: "new" }
  return isPanelSection(panel) ? { mode: "worker", section: panel } : null
}

export function panelToSearch(panel: PanelState, current: URLSearchParams): URLSearchParams {
  const next = new URLSearchParams(current)
  next.delete(PANEL_PARAM)
  if (panel?.mode === "worker") next.set(PANEL_PARAM, panel.section)
  if (panel?.mode === "new") next.set(PANEL_PARAM, NEW_PANEL)
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
  return withPanel(conversationPath(workerConversationId(workerId)), { mode: "worker", section: DEFAULT_PANEL_SECTION })
}

export function legacyRedirect(pathname: string, search: string): string | null {
  const segments = pathname.split("/").filter(Boolean)
  const params = new URLSearchParams(search)
  const beePath = conversationPath(BEE_CONVERSATION)

  if (segments.length === 1 && segments[0] === "departments") return beePath
  if (segments[0] !== "workers" || segments.length > 2) return null
  if (segments.length === 1) return beePath
  if (segments[1] === "create") return withPanel(beePath, { mode: "new" })
  const tab = params.get("tab")
  const section = isPanelSection(tab) ? tab : DEFAULT_PANEL_SECTION
  return withPanel(conversationPath(workerConversationId(segments[1])), { mode: "worker", section })
}

export function resolveConversationRoute(param: string | undefined, workerIds: Set<string>): ConversationId | null {
  if (param === BEE_CONVERSATION) return BEE_CONVERSATION
  if (!param || !workerIds.has(param)) return null
  return workerConversationId(param)
}

export function hashShowsConversation(hash: string, conversation: ConversationId): boolean {
  const { pathname = "" } = parsePath(hash.replace(/^#/, ""))
  const match = matchPath("/chat/:conversation", pathname.startsWith("/") ? pathname : `/${pathname}`)
  return match?.params.conversation === (workerIdOf(conversation) ?? BEE_CONVERSATION)
}

export function isRoutedConversation(conversation: ConversationId): boolean {
  return hashShowsConversation(window.location.hash, conversation)
}
