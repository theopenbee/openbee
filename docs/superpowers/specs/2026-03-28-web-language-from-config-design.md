# Design: Web Language Driven by Backend Config

**Date:** 2026-03-28
**Branch:** feat/i18n
**Status:** Approved

---

## Overview

The project now has a `language` field in `config.yaml` (written by the CLI config wizard). The Web UI should use this configured language instead of providing its own language switcher. The `LanguageSwitcher` component is removed entirely; the Web fetches the language from a new public backend endpoint at startup.

---

## Requirements

1. Remove the `LanguageSwitcher` component from the Web UI — users can no longer switch language in the browser.
2. On startup, the Web fetches `GET /api/config` (unauthenticated) to read the `language` field.
3. The fetched language is applied before React mounts, so no language flash occurs.
4. If the fetch fails or the field is absent, the Web falls back to `"en"`.
5. `localStorage` is no longer used to persist or read language preference.

---

## Architecture

```
Browser startup
  └─ main.tsx: bootstrap()
       ├─ fetch GET /api/config  ──→  gin router (no JWT)
       │                               └─ configHandler.GetConfig()
       │                                    └─ returns {"language": cfg.Language || "en"}
       ├─ i18n.changeLanguage(lang)
       └─ ReactDOM.createRoot(...).render(<App />)
```

---

## Backend Changes

### 1. `internal/api/config_handler.go` — new file

```go
package api

import (
    "net/http"
    "github.com/gin-gonic/gin"
)

type configResponse struct {
    Language string `json:"language"`
}

func (s *Server) getConfig(c *gin.Context) {
    lang := s.Language
    if lang == "" {
        lang = "en"
    }
    c.JSON(http.StatusOK, configResponse{Language: lang})
}
```

### 2. `internal/api/router.go` — register route outside JWT group

```go
func (s *Server) setupRoutes() error {
    s.registerAuthRoutes()
    s.router.GET("/api/config", s.getConfig)   // ← new, public

    api := s.router.Group("/api")
    api.Use(s.JWTMiddleware)
    // ... existing routes unchanged
}
```

### 3. `internal/api/router.go` — add `Language` to `ServerParams`

```go
type ServerParams struct {
    // ... existing fields unchanged
    Language string
}
```

### 4. `internal/app/app.go` — pass `cfg.Language` to `buildAPIServer` and `ServerParams`

`buildAPIServer` receives `language string` as an additional parameter and sets `ServerParams.Language`.

---

## Frontend Changes

### 1. `web/src/i18n.ts` — remove `localStorage`

- Remove `localStorage.getItem("language")` and `localStorage.setItem("language", code)`.
- Initialize `i18n` with `lng: "en"` as a static placeholder; `main.tsx` overrides this before mount.
- Remove the `languageChanged` event listener for `document.documentElement.lang` — keep the `updateHtmlLang` call but drive it from `main.tsx` instead.

```ts
import i18n from "i18next"
import { initReactI18next } from "react-i18next"
import en from "./locales/en.json"
import zh from "./locales/zh.json"

i18n.use(initReactI18next).init({
  resources: {
    en: { translation: en },
    zh: { translation: zh },
  },
  lng: "en",
  fallbackLng: "en",
  interpolation: { escapeValue: false },
})

i18n.on("languageChanged", (lng) => {
  document.documentElement.lang = lng === "zh" ? "zh-CN" : lng
})

export default i18n
```

### 2. `web/src/main.tsx` — async bootstrap

```ts
import "./i18n"
import i18n from "./i18n"

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
  ReactDOM.createRoot(document.getElementById("root")!).render(
    <React.StrictMode>
      <App />
    </React.StrictMode>
  )
}

bootstrap()
```

### 3. `web/src/components/language-switcher.tsx` — delete

File is removed entirely.

### 4. `web/src/components/nav.tsx` — remove LanguageSwitcher

Remove the import and the `<LanguageSwitcher />` JSX element.

---

## Files Changed

| File | Change |
|------|--------|
| `internal/api/config_handler.go` | New — `getConfig` handler returning `{"language": "..."}` |
| `internal/api/router.go` | Add `Language` to `ServerParams`; register `GET /api/config` outside JWT group |
| `internal/app/app.go` | Pass `cfg.Language` through `buildAPIServer` → `ServerParams.Language` |
| `web/src/i18n.ts` | Remove `localStorage` reads/writes; use static `lng: "en"` placeholder |
| `web/src/main.tsx` | Add async `bootstrap()` that fetches language before mounting React |
| `web/src/components/language-switcher.tsx` | Delete |
| `web/src/components/nav.tsx` | Remove `LanguageSwitcher` import and usage |

---

## Edge Cases

| Scenario | Behavior |
|----------|----------|
| `config.yaml` has `language: zh` | Web renders in Chinese from first paint |
| `config.yaml` has `language: en` or field absent | Web renders in English (default) |
| `/api/config` fetch fails (network error) | Falls back to `"en"` |
| `/api/config` returns `language: ""` | Falls back to `"en"` (handler normalizes empty string) |

---

## Out of Scope

- Allowing per-user language override in the Web UI
- Adding new languages beyond `zh` and `en`
- Syncing language changes back to `config.yaml` from the Web
