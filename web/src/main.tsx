import "./i18n"
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
