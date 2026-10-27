import { useCallback, useEffect, useSyncExternalStore } from "react"
import {
  type ThemePreference,
  applyPreference,
  getStoredPreference,
  resolveTheme,
  subscribeSystemTheme,
} from "@/lib/theme"

const listeners = new Set<() => void>()
let current: ThemePreference = getStoredPreference()

function subscribe(listener: () => void) {
  listeners.add(listener)
  return () => listeners.delete(listener)
}

function setThemePreference(next: ThemePreference) {
  current = next
  applyPreference(next)
  listeners.forEach((listener) => listener())
}

export function useThemePreference() {
  const preference = useSyncExternalStore(subscribe, () => current)
  const theme = useSyncExternalStore(
    useCallback((onChange: () => void) => {
      const offStore = subscribe(onChange)
      const offSystem = subscribeSystemTheme(() => {
        if (current === "system") applyPreference("system")
        onChange()
      })
      return () => {
        offStore()
        offSystem()
      }
    }, []),
    () => resolveTheme(current),
  )

  useEffect(() => {
    applyPreference(preference)
  }, [preference])

  const toggle = useCallback(() => setThemePreference(theme === "dark" ? "light" : "dark"), [theme])

  return { preference, theme, setPreference: setThemePreference, toggle }
}
