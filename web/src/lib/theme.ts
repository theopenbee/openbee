export type Theme = "dark" | "light"

const THEME_KEY = "theme"

// Light is the default theme (DESIGN.md › Overview); dark is opt-in.
export function getStoredTheme(): Theme {
  const stored = localStorage.getItem(THEME_KEY)
  return stored === "dark" ? "dark" : "light"
}

export function applyTheme(theme: Theme) {
  const root = document.documentElement
  root.classList.remove("dark", "light")
  root.classList.add(theme)
  localStorage.setItem(THEME_KEY, theme)
}
