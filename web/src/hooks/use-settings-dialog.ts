import { createContext, useContext } from "react"

export type SettingsSection = "env" | "system" | "plugins" | "departments"
export type PluginsTab = "market" | "installed"
export type SettingsTarget = { section: SettingsSection; plugin?: string; tab?: PluginsTab }

export const SettingsDialogContext = createContext<((target: SettingsTarget) => void) | null>(null)

export function useSettingsDialog() {
  const show = useContext(SettingsDialogContext)
  if (!show) throw new Error("useSettingsDialog must be used within SettingsDialogProvider")
  return show
}
