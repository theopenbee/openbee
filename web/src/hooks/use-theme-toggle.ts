import { useCallback, useState } from "react"
import { type Theme, getStoredTheme, applyTheme } from "@/lib/theme"

export function useThemeToggle() {
  const [theme, setTheme] = useState<Theme>(getStoredTheme)
  const toggle = useCallback(() => {
    setTheme((current) => {
      const next: Theme = current === "dark" ? "light" : "dark"
      applyTheme(next)
      return next
    })
  }, [])
  return { theme, toggle }
}
