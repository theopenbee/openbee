export type Theme = "dark" | "light"

// Older builds wrote the implicit default to "theme" on every boot, so a value
// there doesn't mean the user picked it. Only explicit toggles are persisted,
// under a fresh key; the legacy key is dropped on boot.
const THEME_KEY = "openbee.theme"
const LEGACY_THEME_KEY = "theme"

// Light is the default theme (DESIGN.md › Overview); dark is opt-in.
export function getStoredTheme(): Theme {
  return localStorage.getItem(THEME_KEY) === "dark" ? "dark" : "light"
}

export function applyTheme(theme: Theme) {
  const root = document.documentElement
  root.classList.remove("dark", "light")
  root.classList.add(theme)
}

export function persistTheme(theme: Theme) {
  localStorage.setItem(THEME_KEY, theme)
}

export function dropLegacyTheme() {
  localStorage.removeItem(LEGACY_THEME_KEY)
}
