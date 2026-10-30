import { localize } from "./plugins"
import type { AppConfig, EngineChoice } from "./types"

export interface EngineSelectOption {
  value: string
  label: string
  disabled?: boolean
}

export function listEnabledEngines(config: AppConfig | undefined, lang: string): EngineChoice[] {
  return (config?.engines ?? []).map((engine) => ({
    id: engine.id,
    name: localize(engine.name, lang) || engine.id,
  }))
}

export function findEngine(engines: readonly EngineChoice[], id: string): EngineChoice {
  return engines.find((engine) => engine.id === id) ?? { id, name: id }
}

export function engineSelectOptions(
  engines: readonly EngineChoice[],
  stored: string | undefined,
  unavailableLabel: (id: string) => string,
): EngineSelectOption[] {
  const options: EngineSelectOption[] = engines.map((engine) => ({ value: engine.id, label: engine.name }))
  if (stored && !engines.some((engine) => engine.id === stored)) {
    options.push({ value: stored, label: unavailableLabel(stored), disabled: true })
  }
  return options
}
