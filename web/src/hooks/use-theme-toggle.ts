import { useCallback, useState } from "react"
import { type Theme, getStoredTheme, applyTheme, persistTheme } from "@/lib/theme"

export function useThemeToggle() {
  const [theme, setTheme] = useState<Theme>(() => {
    const t = getStoredTheme()
    applyTheme(t)
    return t
  })
  const toggle = useCallback(() => {
    setTheme((current) => {
      const next: Theme = current === "dark" ? "light" : "dark"
      applyTheme(next)
      persistTheme(next)
      return next
    })
  }, [])
  return { theme, toggle }
}
