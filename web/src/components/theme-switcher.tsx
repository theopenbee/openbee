import { useState } from "react"
import { Sun, Moon } from "lucide-react"

type Theme = "dark" | "light"

function getStoredTheme(): Theme {
  const stored = localStorage.getItem("theme")
  return stored === "light" ? "light" : "dark"
}

function applyTheme(theme: Theme) {
  const root = document.documentElement
  root.classList.remove("dark", "light")
  root.classList.add(theme)
  localStorage.setItem("theme", theme)
}

export function ThemeSwitcher() {
  const [theme, setTheme] = useState<Theme>(getStoredTheme)

  const toggle = () => {
    const next: Theme = theme === "dark" ? "light" : "dark"
    applyTheme(next)
    setTheme(next)
  }

  return (
    <button
      onClick={toggle}
      className="p-1.5 rounded-md transition-colors text-muted-foreground hover:text-foreground"
      aria-label={theme === "dark" ? "Switch to light mode" : "Switch to dark mode"}
    >
      {theme === "dark" ? <Sun className="h-4 w-4" /> : <Moon className="h-4 w-4" />}
    </button>
  )
}
