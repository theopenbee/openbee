# Web Language Driven by Backend Config — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove the Web language switcher and drive the Web UI language from `config.yaml` via a new public `GET /api/config` endpoint.

**Architecture:** A new unauthenticated `GET /api/config` route returns `{"language": "en"|"zh"}` from the loaded config. The frontend fetches this in an async `bootstrap()` before mounting React, so the correct language is applied from the very first paint. The `LanguageSwitcher` component and all `localStorage` language logic are deleted.

**Tech Stack:** Go / Gin (backend), React / react-i18next / TypeScript (frontend)

---

## File Map

| File | Action | Responsibility |
|------|--------|----------------|
| `internal/api/config_handler.go` | Create | `getConfig` handler — returns `{"language": "..."}` |
| `internal/api/router.go` | Modify | Add `Language string` to `ServerParams`; register `GET /api/config` outside JWT group |
| `internal/app/app.go` | Modify | Add `language string` param to `buildAPIServer`; pass `cfg.Language` at call site |
| `web/src/i18n.ts` | Modify | Remove `localStorage` reads/writes; use static `lng: "en"` placeholder |
| `web/src/main.tsx` | Modify | Replace synchronous mount with async `bootstrap()` that fetches language first |
| `web/src/components/language-switcher.tsx` | Delete | Removed entirely |
| `web/src/components/nav.tsx` | Modify | Remove `LanguageSwitcher` import and JSX |

---

## Task 1: Add `Language` field to `ServerParams` and wire it through `app.go`

**Files:**
- Modify: `internal/api/router.go`
- Modify: `internal/app/app.go`

- [ ] **Step 1: Add `Language` to `ServerParams` in `router.go`**

Open `internal/api/router.go`. Add `Language string` as the last field of `ServerParams`:

```go
type ServerParams struct {
	WorkerStore      *store.WorkerStore
	ExecutionStore   *store.ExecutionStore
	Manager          *worker.Manager
	BeeMCPServer     *mcp.MCPServer
	WorkerMCPServer  *mcp.MCPServer
	BeeAPIKey        string
	WorkerAPIKey     string
	StaticFS         fs.FS
	LocalChatHandler *LocalChatHandler
	AuthHandler      *auth.AuthHandler
	JWTMiddleware    gin.HandlerFunc
	Language         string
}
```

- [ ] **Step 2: Add `language string` parameter to `buildAPIServer` in `app.go`**

Change the function signature at line 246 of `internal/app/app.go`:

```go
func buildAPIServer(serverCfg config.ServerConfig, mcpCfg config.MCPConfig, s appStores, mgr *worker.Manager, beeMCPSrv *mcp.MCPServer, workerMCPSrv *mcp.MCPServer, localChat *api.LocalChatHandler, language string) (*api.Server, error) {
```

- [ ] **Step 3: Pass `Language` into `ServerParams` inside `buildAPIServer`**

Add `Language: language` to the `api.ServerParams` literal at the end of `buildAPIServer` (around line 254):

```go
return api.NewServer(api.ServerParams{
	WorkerStore:      s.workerStore,
	ExecutionStore:   s.execStore,
	Manager:          mgr,
	BeeMCPServer:     beeMCPSrv,
	WorkerMCPServer:  workerMCPSrv,
	BeeAPIKey:        mcpCfg.APIKey,
	WorkerAPIKey:     mcpCfg.WorkerAPIKey,
	StaticFS:         webui.DistFS,
	LocalChatHandler: localChat,
	AuthHandler:      authHandler,
	JWTMiddleware:    jwtMiddleware,
	Language:         language,
})
```

- [ ] **Step 4: Pass `cfg.Language` at the call site in `BuildApp`**

At line 162 of `internal/app/app.go`, add `cfg.Language` as the last argument:

```go
srv, err := buildAPIServer(cfg.Server, cfg.Bee.MCP, s, mgr, beeMCPSrv, workerMCPSrv, localChatHandler, cfg.Language)
```

- [ ] **Step 5: Verify the project compiles**

```bash
go build ./...
```

Expected: no errors.

- [ ] **Step 6: Commit**

```bash
git add internal/api/router.go internal/app/app.go
git commit -m "feat: add Language field to ServerParams and wire cfg.Language"
```

---

## Task 2: Create the `GET /api/config` handler

**Files:**
- Create: `internal/api/config_handler.go`
- Modify: `internal/api/router.go`

- [ ] **Step 1: Create `internal/api/config_handler.go`**

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

- [ ] **Step 2: Register the route in `setupRoutes` — outside the JWT group**

In `internal/api/router.go`, add one line after `s.registerAuthRoutes()` and before the `api` group:

```go
func (s *Server) setupRoutes() error {
	s.registerAuthRoutes()
	s.router.GET("/api/config", s.getConfig) // public — no JWT required

	api := s.router.Group("/api")
	api.Use(s.JWTMiddleware)
	{
		s.registerWorkerRoutes(api)
		s.registerExecutionRoutes(api)
		s.registerLocalChatRoutes(api)
	}

	s.registerMCPRoutes()

	return s.registerStaticRoutes()
}
```

- [ ] **Step 3: Verify the project compiles**

```bash
go build ./...
```

Expected: no errors.

- [ ] **Step 4: Smoke-test the endpoint manually**

Start the server locally (`openbee start`) with a `config.yaml` that has `language: zh`, then:

```bash
curl -s http://localhost:<port>/api/config
```

Expected output: `{"language":"zh"}`

Without a `language` field in config.yaml:

```bash
curl -s http://localhost:<port>/api/config
```

Expected output: `{"language":"en"}`

- [ ] **Step 5: Commit**

```bash
git add internal/api/config_handler.go internal/api/router.go
git commit -m "feat: add public GET /api/config endpoint returning language"
```

---

## Task 3: Update `web/src/i18n.ts` — remove `localStorage`

**Files:**
- Modify: `web/src/i18n.ts`

Current file uses `localStorage.getItem("language") || "en"` as the initial language. We replace the entire file with a version that uses a static `"en"` placeholder (overridden by `main.tsx` before mount) and keeps the `languageChanged` listener for `document.documentElement.lang`.

- [ ] **Step 1: Replace the contents of `web/src/i18n.ts`**

```ts
import i18n from "i18next"
import { initReactI18next } from "react-i18next"
import en from "./locales/en.json"
import zh from "./locales/zh.json"

i18n
  .use(initReactI18next)
  .init({
    resources: {
      en: { translation: en },
      zh: { translation: zh },
    },
    lng: "en",
    fallbackLng: "en",
    interpolation: {
      escapeValue: false,
    },
  })

i18n.on("languageChanged", (lng) => {
  document.documentElement.lang = lng === "zh" ? "zh-CN" : lng
})

export default i18n
```

- [ ] **Step 2: Verify TypeScript compiles**

```bash
cd web && npx tsc --noEmit
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add web/src/i18n.ts
git commit -m "feat: remove localStorage from i18n init — language driven by backend"
```

---

## Task 4: Update `web/src/main.tsx` — async bootstrap

**Files:**
- Modify: `web/src/main.tsx`

Current `main.tsx` synchronously mounts React. We wrap the mount in an async `bootstrap()` that fetches language from `/api/config` first.

- [ ] **Step 1: Replace the contents of `web/src/main.tsx`**

```tsx
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
```

- [ ] **Step 2: Verify TypeScript compiles**

```bash
cd web && npx tsc --noEmit
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add web/src/main.tsx
git commit -m "feat: fetch language from /api/config before React mount"
```

---

## Task 5: Remove `LanguageSwitcher` from the Web UI

**Files:**
- Delete: `web/src/components/language-switcher.tsx`
- Modify: `web/src/components/nav.tsx`

- [ ] **Step 1: Delete `language-switcher.tsx`**

```bash
git rm web/src/components/language-switcher.tsx
```

- [ ] **Step 2: Remove `LanguageSwitcher` from `nav.tsx`**

Open `web/src/components/nav.tsx`. Remove the import line:

```ts
import { LanguageSwitcher } from "./language-switcher"
```

And remove the JSX element inside the `<nav>` return:

```tsx
<LanguageSwitcher />
```

The `nav.tsx` closing section should now look like:

```tsx
        <ThemeSwitcher />
      </div>
    </nav>
```

- [ ] **Step 3: Verify TypeScript compiles and the dev build succeeds**

```bash
cd web && npx tsc --noEmit && npx vite build
```

Expected: no TypeScript errors, build succeeds.

- [ ] **Step 4: Commit**

```bash
git add web/src/components/nav.tsx
git commit -m "feat: remove LanguageSwitcher — language now driven by backend config"
```

---

## Self-Review

**Spec coverage check:**

| Requirement | Covered by |
|-------------|-----------|
| Remove `LanguageSwitcher` component | Task 5 |
| `GET /api/config` unauthenticated endpoint | Task 2 |
| Language applied before React mount (no flash) | Task 4 |
| Fallback to `"en"` on fetch failure or missing field | Task 4 (try/catch) + Task 2 (handler normalizes `""`) |
| Remove `localStorage` usage | Task 3 |

All requirements covered. No placeholders. Types consistent across tasks (`configResponse.Language`, `ServerParams.Language`, `data.language`).
