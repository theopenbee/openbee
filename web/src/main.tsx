import "@fontsource-variable/inter"
import "@fontsource-variable/jetbrains-mono"
// Bundled CJK fallback — only the simplified-Chinese subset: 400 for body, and
// one medium face that also serves 600 titles (see cjk-font.css).
import "@fontsource/noto-sans-sc/chinese-simplified-400.css"
import "./cjk-font.css"
import "./i18n"
import { getStoredTheme, applyTheme, dropLegacyTheme } from "./lib/theme"
dropLegacyTheme()
applyTheme(getStoredTheme())
import i18n from "./i18n"
import { StrictMode } from "react"
import { createRoot } from "react-dom/client"
import { App } from "./app"
import "./globals.css"

async function bootstrap() {
  try {
    const res = await fetch("/api/config")
    if (res.ok) {
      const data = await res.json()
      if (data.language) {
        await i18n.changeLanguage(data.language)
      }
    }
  } catch {
    // network failure — stay on fallback "en"
  }
  createRoot(document.getElementById("root")!).render(
    <StrictMode>
      <App />
    </StrictMode>
  )
}

bootstrap()
