export type ThemePreference = "light" | "dark" | "system"
export type Theme = "light" | "dark"

const THEME_KEY = "theme"
const ACCENT = "orange"
const darkQuery = "(prefers-color-scheme: dark)"

export function getStoredPreference(): ThemePreference {
  const stored = localStorage.getItem(THEME_KEY)
  return stored === "light" || stored === "system" ? stored : "dark"
}

export function resolveTheme(preference: ThemePreference): Theme {
  if (preference !== "system") return preference
  return window.matchMedia(darkQuery).matches ? "dark" : "light"
}

export function applyPreference(preference: ThemePreference) {
  const root = document.documentElement
  root.dataset.theme = resolveTheme(preference)
  root.dataset.accent = ACCENT
  localStorage.setItem(THEME_KEY, preference)
}

export function subscribeSystemTheme(onChange: () => void) {
  const media = window.matchMedia(darkQuery)
  media.addEventListener("change", onChange)
  return () => media.removeEventListener("change", onChange)
}
