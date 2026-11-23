import type { EngineModel } from "./types"

export function normalizeModelList(raw: unknown): EngineModel[] {
  if (!Array.isArray(raw)) return []
  const seen = new Set<string>()
  const models: EngineModel[] = []
  for (const item of raw) {
    if (typeof item !== "object" || item === null) continue
    const { id, name } = item as { id?: unknown; name?: unknown }
    if (typeof id !== "string") continue
    const trimmed = id.trim()
    if (trimmed === "" || seen.has(trimmed)) continue
    seen.add(trimmed)
    const label = typeof name === "string" ? name.trim() : ""
    models.push(label && label !== trimmed ? { id: trimmed, name: label } : { id: trimmed })
  }
  return models
}

export function buildCreateEngineModels(
  engine: string,
  models: Record<string, string>,
): Record<string, string> | undefined {
  const model = engineModel(models, engine)
  return model ? { [engine]: model } : undefined
}

export function changedEngineModels(
  stored: Record<string, string>,
  edited: Record<string, string>,
): Record<string, string> | null {
  const changes: Record<string, string> = {}
  for (const [engine, raw] of Object.entries(edited)) {
    const next = raw.trim()
    if (next !== (stored[engine] ?? "").trim()) changes[engine] = next
  }
  return Object.keys(changes).length > 0 ? changes : null
}

export function engineModel(models: Record<string, string> | undefined, engine: string): string {
  return engine ? (models?.[engine] ?? "").trim() : ""
}

export function parseEngineModels(raw: string | undefined): Record<string, string> {
  if (!raw) return {}
  try {
    const parsed: unknown = JSON.parse(raw)
    if (typeof parsed !== "object" || parsed === null || Array.isArray(parsed)) return {}
    const models: Record<string, string> = {}
    for (const [engine, value] of Object.entries(parsed)) {
      if (typeof value === "string" && value.trim() !== "") models[engine] = value.trim()
    }
    return models
  } catch {
    return {}
  }
}

export function cleanEngineModels(models: Record<string, string>): Record<string, string> {
  const result: Record<string, string> = {}
  for (const [engine, value] of Object.entries(models)) {
    if (value.trim() !== "") result[engine] = value.trim()
  }
  return result
}

export function filterModels(models: readonly EngineModel[], query: string): EngineModel[] {
  const needle = query.trim().toLocaleLowerCase()
  if (!needle) return [...models]
  return models.filter(
    (model) =>
      model.id.toLocaleLowerCase().includes(needle) || (model.name?.toLocaleLowerCase().includes(needle) ?? false),
  )
}

export type ModelListStatus = "loading" | "error" | "ready"

export type ModelListNotice = "loading" | "error" | "noMatches"

export function modelListNotice(status: ModelListStatus, total: number, matches: number): ModelListNotice | null {
  if (status === "loading") return "loading"
  if (status === "error") return "error"
  return total > 0 && matches === 0 ? "noMatches" : null
}

export interface SuggestionState {
  open: boolean
  query: string | null
  active: number
}

export const CLOSED_SUGGESTIONS: SuggestionState = { open: false, query: null, active: -1 }

export type SuggestionAction =
  | { type: "open" }
  | { type: "close" }
  | { type: "input"; text: string }
  | { type: "move"; step: 1 | -1; count: number }
  | { type: "hover"; index: number }

export function suggestionReducer(state: SuggestionState, action: SuggestionAction): SuggestionState {
  switch (action.type) {
    case "open":
      return state.open ? state : { open: true, query: null, active: -1 }
    case "close":
      return state.open ? CLOSED_SUGGESTIONS : state
    case "input":
      return { open: true, query: action.text, active: -1 }
    case "move": {
      if (!state.open) return { open: true, query: null, active: -1 }
      if (action.count <= 0) return state.active === -1 ? state : { ...state, active: -1 }
      const from = state.active < 0 || state.active >= action.count ? (action.step === 1 ? -1 : 0) : state.active
      return { ...state, active: (from + action.step + action.count) % action.count }
    }
    case "hover":
      return state.open && state.active !== action.index ? { ...state, active: action.index } : state
  }
}

export function visibleModels(models: readonly EngineModel[], state: SuggestionState): EngineModel[] {
  return filterModels(models, state.query ?? "")
}

export function highlightedModel(visible: readonly EngineModel[], state: SuggestionState): EngineModel | undefined {
  return state.open && state.active >= 0 ? visible[state.active] : undefined
}
