import { useTranslation } from "react-i18next"
import { cn } from "@/lib/utils"

const LANGUAGES = [
  { code: "en", label: "EN" },
  { code: "zh", label: "中文" },
]

export function LanguageSwitcher() {
  const { i18n } = useTranslation()

  const handleChange = (code: string) => {
    i18n.changeLanguage(code)
    localStorage.setItem("language", code)
  }

  return (
    <div className="flex items-center gap-1">
      {LANGUAGES.map((lang) => (
        <button
          key={lang.code}
          onClick={() => handleChange(lang.code)}
          className={cn(
            "px-2 py-1 text-sm rounded-md transition-colors",
            i18n.language === lang.code
              ? "text-primary font-medium"
              : "text-muted-foreground hover:text-foreground"
          )}
        >
          {lang.label}
        </button>
      ))}
    </div>
  )
}
