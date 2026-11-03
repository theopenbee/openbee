export const LANGUAGES = [
  { value: "en", label: "English", short: "EN" },
  { value: "zh", label: "简体中文", short: "中文" },
] as const

export type Language = (typeof LANGUAGES)[number]["value"]

export const DEFAULT_LANGUAGE: Language = "en"

const LANGUAGE_KEY = "language"

type LanguageStorage = Pick<Storage, "getItem" | "setItem">

export function normalizeLanguage(value: unknown): Language | null {
  if (typeof value !== "string") return null
  const base = value.toLowerCase().split(/[-_]/)[0]
  return LANGUAGES.find((language) => language.value === base)?.value ?? null
}

export function getStoredLanguage(storage: LanguageStorage = localStorage): Language | null {
  return normalizeLanguage(storage.getItem(LANGUAGE_KEY))
}

export function storeLanguage(language: Language, storage: LanguageStorage = localStorage) {
  storage.setItem(LANGUAGE_KEY, language)
}

export function resolveLanguage(value: unknown): Language {
  return normalizeLanguage(value) ?? DEFAULT_LANGUAGE
}
