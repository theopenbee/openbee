export type Theme = "dark" | "light"

export function getStoredTheme(): Theme {
  const stored = localStorage.getItem("theme")
  return stored === "light" ? "light" : "dark"
}

export function applyTheme(theme: Theme) {
  document.documentElement.classList.remove("dark", "light")
  document.documentElement.classList.add(theme)
  localStorage.setItem("theme", theme)
}
