import i18next, { type i18n } from "i18next"
import en from "@/locales/en.json"
import zh from "@/locales/zh.json"

export function testI18n(lng: "en" | "zh" = "en"): i18n {
  const instance = i18next.createInstance()
  void instance.init({
    lng,
    fallbackLng: "en",
    resources: { en: { translation: en }, zh: { translation: zh } },
    interpolation: { escapeValue: false },
    initAsync: false,
  })
  return instance
}
