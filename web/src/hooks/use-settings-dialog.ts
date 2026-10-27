import { createContext, useContext } from "react"

export type SettingsSection = "env" | "system" | "plugins" | "departments"
export type PluginsTab = "market" | "installed"
export type SettingsTarget = { section: SettingsSection; plugin?: string; tab?: PluginsTab }

export type SettingsDialogApi = { show: (target: SettingsTarget) => void; close: () => void }

export const SettingsDialogContext = createContext<SettingsDialogApi | null>(null)

function useSettingsDialogApi() {
  const api = useContext(SettingsDialogContext)
  if (!api) throw new Error("useSettingsDialog must be used within SettingsDialogProvider")
  return api
}

export function useSettingsDialog() {
  return useSettingsDialogApi().show
}

export function useCloseSettingsDialog() {
  return useSettingsDialogApi().close
}
