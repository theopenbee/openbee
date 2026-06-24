import { Sun, Moon } from "lucide-react"
import { useThemeToggle } from "@/hooks/use-theme-toggle"

export function ThemeSwitcher() {
  const { theme, toggle } = useThemeToggle()

  return (
    <button
      onClick={toggle}
      className="p-1.5 rounded-sm transition-colors text-muted-foreground hover:text-foreground"
      aria-label={theme === "dark" ? "Switch to light mode" : "Switch to dark mode"}
    >
      {theme === "dark" ? <Sun className="h-4 w-4" /> : <Moon className="h-4 w-4" />}
    </button>
  )
}
