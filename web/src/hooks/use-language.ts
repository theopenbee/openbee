import { useCallback } from "react"
import { useTranslation } from "react-i18next"
import { resolveLanguage, storeLanguage } from "@/lib/language"

export function useLanguage() {
  const { i18n } = useTranslation()
  const language = resolveLanguage(i18n.resolvedLanguage ?? i18n.language)

  const setLanguage = useCallback(
    (value: string) => {
      const next = resolveLanguage(value)
      storeLanguage(next)
      void i18n.changeLanguage(next)
    },
    [i18n],
  )

  return { language, setLanguage }
}
