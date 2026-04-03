import { useState } from "react"
import { type Theme, getStoredTheme, applyTheme } from "@/lib/theme"

export function useThemeToggle() {
  const [theme, setTheme] = useState<Theme>(getStoredTheme)
  const toggle = () => {
    const next: Theme = theme === "dark" ? "light" : "dark"
    applyTheme(next)
    setTheme(next)
  }
  return { theme, toggle }
}
