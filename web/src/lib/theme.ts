export type Theme = "dark" | "light"

const THEME_KEY = "theme"

export function getStoredTheme(): Theme {
  const stored = localStorage.getItem(THEME_KEY)
  return stored === "light" ? "light" : "dark"
}

export function applyTheme(theme: Theme) {
  const root = document.documentElement
  root.classList.remove("dark", "light")
  root.classList.add(theme)
  localStorage.setItem(THEME_KEY, theme)
}
