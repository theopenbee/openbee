import "@fontsource-variable/inter"
import "@fontsource-variable/geist"
import "@fontsource-variable/jetbrains-mono"
// Bundled CJK fallback — only the simplified-Chinese subset at the weights we use (400 body, 500 headings/labels).
import "@fontsource/noto-sans-sc/chinese-simplified-400.css"
import "@fontsource/noto-sans-sc/chinese-simplified-500.css"
import "./i18n"
import { getStoredPreference, applyPreference } from "./lib/theme"
applyPreference(getStoredPreference())
import i18n from "./i18n"
import { getStoredLanguage, resolveLanguage } from "./lib/language"
import { StrictMode } from "react"
import { createRoot } from "react-dom/client"
import { App } from "./app"
import "./globals.css"

async function fetchServerLanguage(): Promise<unknown> {
  try {
    const res = await fetch("/api/config")
    return res.ok ? (await res.json()).language : undefined
  } catch {
    return undefined
  }
}

async function bootstrap() {
  await i18n.changeLanguage(getStoredLanguage() ?? resolveLanguage(await fetchServerLanguage()))
  createRoot(document.getElementById("root")!).render(
    <StrictMode>
      <App />
    </StrictMode>
  )
}

bootstrap()
