# System Settings Page Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a `/settings` web page with a `default_engine` dropdown backed by `GET /api/system-configs` and `PUT /api/system-configs/:key` endpoints.

**Architecture:** New `SystemConfigHandler` in `internal/api` uses interface types for store and validator, keeping it testable without a full worker.Manager. Routes registered in the JWT-protected `/api` group. Frontend: new `settings.tsx` page with a `useQuery` fetch + `useMutation` save pattern, optimistic local state, and revert on error. Sidebar nav entry added.

**Tech Stack:** Go/Gin, `store.SystemConfigStore`, `enginecfg.Set()`, React 19, @tanstack/react-query v5, react-i18next, shadcn Select.

---

## File Map

| File | Change |
|------|--------|
| `internal/api/system_config_handler.go` | New — GET + PUT handler with inline interfaces |
| `internal/api/system_config_handler_test.go` | New — handler unit tests |
| `internal/routes/server.go` | Add `SystemConfigs *api.SystemConfigHandler` to `ServerParams` |
| `internal/routes/api.go` | Register GET `/system-configs` and PUT `/system-configs/:key` |
| `internal/app/app.go` | Construct `SystemConfigHandler` and pass to `buildAPIServer` |
| `web/src/lib/api.ts` | Add `api.systemConfigs.get()` and `api.systemConfigs.set()` |
| `web/src/locales/en.json` | Add `nav.systemSettings` + `systemSettings.*` keys |
| `web/src/locales/zh.json` | Add `nav.systemSettings` + `systemSettings.*` keys |
| `web/src/pages/settings.tsx` | New — System Settings page component |
| `web/src/app.tsx` | Lazy-import + `/settings` route |
| `web/src/components/app-sidebar.tsx` | Add "System Settings" entry to `navMain` |
| `web/src/lib/breadcrumb-config.ts` | Add `/settings` breadcrumb entry |

---

### Task 1: Backend handler + tests

**Files:**
- Create: `internal/api/system_config_handler.go`
- Create: `internal/api/system_config_handler_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/api/system_config_handler_test.go`:

```go
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/theopenbee/openbee/internal/infra/model"
)

// --- fakes ---

type fakeSysConfigStore struct {
	vals map[string]string
	err  error
}

func (f *fakeSysConfigStore) Get(_ context.Context, key string) (model.SystemConfig, bool, error) {
	if f.err != nil {
		return model.SystemConfig{}, false, f.err
	}
	v, ok := f.vals[key]
	if !ok {
		return model.SystemConfig{}, false, nil
	}
	return model.SystemConfig{Key: key, Value: v}, true, nil
}

func (f *fakeSysConfigStore) Set(_ context.Context, key, value string) error {
	if f.err != nil {
		return f.err
	}
	if f.vals == nil {
		f.vals = make(map[string]string)
	}
	f.vals[key] = value
	return nil
}

type fakeEngineValidatorForSys struct {
	valid map[string]bool
}

func (f *fakeEngineValidatorForSys) ValidateEngine(name string) error {
	if name == "" || f.valid[name] {
		return nil
	}
	return fmt.Errorf("engine %q not enabled", name)
}

func newSysConfigRouter(store sysConfigStore, validator engineValidatorForSys) *gin.Engine {
	gin.SetMode(gin.TestMode)
	h := NewSystemConfigHandler(store, validator)
	r := gin.New()
	api := r.Group("/api")
	api.GET("/system-configs", h.Get)
	api.PUT("/system-configs/:key", h.Set)
	return r
}

// --- tests ---

func TestSystemConfigHandler_Get_Empty(t *testing.T) {
	router := newSysConfigRouter(&fakeSysConfigStore{vals: map[string]string{}}, &fakeEngineValidatorForSys{})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/system-configs", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["default_engine"] != "" {
		t.Errorf("expected empty default_engine, got %q", resp["default_engine"])
	}
}

func TestSystemConfigHandler_Get_WithValue(t *testing.T) {
	store := &fakeSysConfigStore{vals: map[string]string{"default_engine": "claude"}}
	router := newSysConfigRouter(store, &fakeEngineValidatorForSys{})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/system-configs", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["default_engine"] != "claude" {
		t.Errorf("expected claude, got %q", resp["default_engine"])
	}
}

func TestSystemConfigHandler_Set_ValidEngine(t *testing.T) {
	store := &fakeSysConfigStore{vals: map[string]string{}}
	validator := &fakeEngineValidatorForSys{valid: map[string]bool{"claude": true}}
	router := newSysConfigRouter(store, validator)

	body, _ := json.Marshal(map[string]string{"value": "claude"})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/system-configs/default_engine", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if store.vals["default_engine"] != "claude" {
		t.Errorf("expected store to have claude, got %q", store.vals["default_engine"])
	}
}

func TestSystemConfigHandler_Set_InvalidEngine(t *testing.T) {
	store := &fakeSysConfigStore{vals: map[string]string{}}
	validator := &fakeEngineValidatorForSys{valid: map[string]bool{"claude": true}}
	router := newSysConfigRouter(store, validator)

	body, _ := json.Marshal(map[string]string{"value": "unknown-engine"})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/system-configs/default_engine", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSystemConfigHandler_Set_UnknownKey(t *testing.T) {
	router := newSysConfigRouter(&fakeSysConfigStore{vals: map[string]string{}}, &fakeEngineValidatorForSys{})

	body, _ := json.Marshal(map[string]string{"value": "something"})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/system-configs/unknown_key", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee2
go test ./internal/api/ -run TestSystemConfig -v
```

Expected: compile errors — `sysConfigStore`, `engineValidatorForSys`, `NewSystemConfigHandler` not defined.

- [ ] **Step 3: Implement the handler**

Create `internal/api/system_config_handler.go`:

```go
package api

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/theopenbee/openbee/internal/domain/enginecfg"
	"github.com/theopenbee/openbee/internal/infra/model"
)

type sysConfigStore interface {
	Get(ctx context.Context, key string) (model.SystemConfig, bool, error)
	Set(ctx context.Context, key, value string) error
}

type engineValidatorForSys interface {
	ValidateEngine(name string) error
}

// SystemConfigHandler serves GET /system-configs and PUT /system-configs/:key.
type SystemConfigHandler struct {
	store     sysConfigStore
	validator engineValidatorForSys
}

func NewSystemConfigHandler(store sysConfigStore, validator engineValidatorForSys) *SystemConfigHandler {
	return &SystemConfigHandler{store: store, validator: validator}
}

// Get returns all known system config keys as a JSON object.
// Missing DB rows are returned as empty strings.
func (h *SystemConfigHandler) Get(c *gin.Context) {
	cfg, found, err := h.store.Get(c.Request.Context(), model.SystemConfigKeyDefaultEngine)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	value := ""
	if found {
		value = cfg.Value
	}
	c.JSON(http.StatusOK, gin.H{model.SystemConfigKeyDefaultEngine: value})
}

type setSystemConfigRequest struct {
	Value string `json:"value" binding:"required"`
}

// Set updates a single system config key.
// Only "default_engine" is accepted; unknown keys return 400.
func (h *SystemConfigHandler) Set(c *gin.Context) {
	key := c.Param("key")
	if key != model.SystemConfigKeyDefaultEngine {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unknown config key"})
		return
	}
	var req setSystemConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.validator.ValidateEngine(req.Value); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.store.Set(c.Request.Context(), key, req.Value); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	enginecfg.Set(req.Value)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee2
go test ./internal/api/ -run TestSystemConfig -v
```

Expected: all 4 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/api/system_config_handler.go internal/api/system_config_handler_test.go
git commit -m "feat: add SystemConfigHandler for GET/PUT /system-configs"
```

---

### Task 2: Wire handler into routes and app

**Files:**
- Modify: `internal/routes/server.go`
- Modify: `internal/routes/api.go`
- Modify: `internal/app/app.go`

- [ ] **Step 1: Add `SystemConfigs` to `ServerParams`**

In `internal/routes/server.go`, add the field after `Envs`:

```go
// existing:
Envs        *api.EnvHandler
// add:
SystemConfigs *api.SystemConfigHandler
```

Full updated `ServerParams` struct (replace the existing one):

```go
type ServerParams struct {
	Workers     *api.WorkerHandler
	Executions  *api.ExecutionHandler
	Messages    *api.MessageHandler
	Tasks       *api.TaskHandler
	Departments *api.DepartmentHandler
	Stats       *api.StatsHandler
	Config      *api.ConfigHandler
	LocalChat   *api.LocalChatHandler
	Auth        *auth.AuthHandler
	Envs        *api.EnvHandler
	SystemConfigs *api.SystemConfigHandler
	BeeMCP          *mcp.MCPServer
	MCPAuthMiddleware gin.HandlerFunc
	StaticFS          fs.FS
	JWTMiddleware     gin.HandlerFunc
}
```

- [ ] **Step 2: Register routes in `internal/routes/api.go`**

After the envs block, add:

```go
r.GET("/system-configs", s.SystemConfigs.Get)
r.PUT("/system-configs/:key", s.SystemConfigs.Set)
```

Full `registerAPIRoutes` after change (append before the closing `}`):

```go
r.GET("/system-configs", s.SystemConfigs.Get)
r.PUT("/system-configs/:key", s.SystemConfigs.Set)
```

- [ ] **Step 3: Construct handler in `internal/app/app.go`**

In `buildAPIServer`, find the `routes.NewServer(routes.ServerParams{...})` call (around line 318–330). Add `SystemConfigs` alongside the existing fields:

```go
SystemConfigs: api.NewSystemConfigHandler(s.systemConfigStore, mgr),
```

The function signature already receives `s appStores` (which has `systemConfigStore`) and `mgr *worker.Manager` (which implements `engineValidatorForSys`).

- [ ] **Step 4: Build to verify no compile errors**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee2
go build ./...
```

Expected: exits 0 with no output.

- [ ] **Step 5: Commit**

```bash
git add internal/routes/server.go internal/routes/api.go internal/app/app.go
git commit -m "feat: wire SystemConfigHandler into routes and app"
```

---

### Task 3: Frontend API client

**Files:**
- Modify: `web/src/lib/api.ts`

- [ ] **Step 1: Add `systemConfigs` namespace to the `api` object**

In `web/src/lib/api.ts`, after the `envs:` block inside the `api` object, add:

```ts
systemConfigs: {
  get: () => fetchAPI<Record<string, string>>("/system-configs"),
  set: (key: string, value: string) =>
    fetchAPI<{ ok: boolean }>(`/system-configs/${key}`, {
      method: "PUT",
      body: JSON.stringify({ value }),
    }),
},
```

- [ ] **Step 2: Verify TypeScript compiles**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee2/web
pnpm tsc --noEmit
```

Expected: exits 0 with no errors.

- [ ] **Step 3: Commit**

```bash
git add web/src/lib/api.ts
git commit -m "feat(web): add systemConfigs API client methods"
```

---

### Task 4: i18n translation keys

**Files:**
- Modify: `web/src/locales/en.json`
- Modify: `web/src/locales/zh.json`

- [ ] **Step 1: Add keys to `en.json`**

In `web/src/locales/en.json`, add `"systemSettings"` to the `"nav"` object:

```json
"nav": {
  "dashboard": "Dashboard",
  "workers": "Workers",
  "executions": "Sessions",
  "tasks": "Scheduled Tasks",
  "departments": "Departments",
  "directory": "Directory",
  "settings": "Env Variables",
  "systemSettings": "System Settings"
},
```

Then add a new top-level `"systemSettings"` object (e.g., after the `"envConfig"` block):

```json
"systemSettings": {
  "title": "System Settings",
  "engineSection": {
    "title": "Default Engine",
    "hint": "The AI engine Bee uses by default when no per-worker engine is set.",
    "systemDefault": "System Default (claude)"
  },
  "updated": "Default engine updated"
}
```

- [ ] **Step 2: Add keys to `zh.json`**

In `web/src/locales/zh.json`, add `"systemSettings"` to the `"nav"` object:

```json
"systemSettings": "系统设置"
```

Then add a new top-level `"systemSettings"` object:

```json
"systemSettings": {
  "title": "系统设置",
  "engineSection": {
    "title": "默认引擎",
    "hint": "未为员工单独设置引擎时，Bee 调度时使用的默认 AI 引擎。",
    "systemDefault": "系统默认 (claude)"
  },
  "updated": "默认引擎已更新"
}
```

- [ ] **Step 3: Commit**

```bash
git add web/src/locales/en.json web/src/locales/zh.json
git commit -m "feat(web): add i18n keys for system settings page"
```

---

### Task 5: System Settings page component

**Files:**
- Create: `web/src/pages/settings.tsx`

- [ ] **Step 1: Create the page**

Create `web/src/pages/settings.tsx`:

```tsx
import { useEffect, useState } from "react"
import { useTranslation } from "react-i18next"
import { useQuery, useMutation } from "@tanstack/react-query"
import { FadeIn } from "@/components/fade-in"
import { PageHeader } from "@/components/page-header"
import { DetailSection } from "@/components/detail-primitives"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { EngineSelectItems } from "@/components/engine-select-items"
import { useEnabledEngines } from "@/hooks/use-config"
import { api } from "@/lib/api"

export function SystemSettings() {
  const { t } = useTranslation()
  const enabledEngines = useEnabledEngines()

  const { data: sysConfigs } = useQuery({
    queryKey: ["system-configs"],
    queryFn: () => api.systemConfigs.get(),
  })

  const [engine, setEngine] = useState<string>("")

  useEffect(() => {
    if (sysConfigs !== undefined) {
      setEngine(sysConfigs["default_engine"] ?? "")
    }
  }, [sysConfigs])

  const { mutate: saveEngine, isPending } = useMutation({
    mutationFn: (value: string) => api.systemConfigs.set("default_engine", value),
    onError: () => {
      setEngine(sysConfigs?.["default_engine"] ?? "")
    },
  })

  function handleEngineChange(value: string) {
    setEngine(value)
    saveEngine(value)
  }

  return (
    <FadeIn>
      <div className="space-y-6">
        <PageHeader title={t("systemSettings.title")} />

        <DetailSection className="p-5 sm:p-6 space-y-4">
          <div>
            <p className="text-xs font-medium uppercase tracking-[0.18em] text-muted-foreground">
              {t("systemSettings.engineSection.title")}
            </p>
            <p className="mt-1 text-sm leading-6 text-muted-foreground">
              {t("systemSettings.engineSection.hint")}
            </p>
          </div>
          <Select
            value={engine}
            onValueChange={handleEngineChange}
            disabled={isPending}
          >
            <SelectTrigger className="w-48">
              <SelectValue placeholder={t("systemSettings.engineSection.systemDefault")} />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="">
                {t("systemSettings.engineSection.systemDefault")}
              </SelectItem>
              <EngineSelectItems engines={enabledEngines} />
            </SelectContent>
          </Select>
        </DetailSection>
      </div>
    </FadeIn>
  )
}
```

- [ ] **Step 2: Verify TypeScript compiles**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee2/web
pnpm tsc --noEmit
```

Expected: exits 0 with no errors.

- [ ] **Step 3: Commit**

```bash
git add web/src/pages/settings.tsx
git commit -m "feat(web): add SystemSettings page with default_engine selector"
```

---

### Task 6: Route, navigation, and breadcrumb

**Files:**
- Modify: `web/src/app.tsx`
- Modify: `web/src/components/app-sidebar.tsx`
- Modify: `web/src/lib/breadcrumb-config.ts`

- [ ] **Step 1: Add lazy import and route in `app.tsx`**

Add the import after the `Env` lazy import line:

```tsx
const SystemSettings = lazy(() => import("@/pages/settings").then(m => ({ default: m.SystemSettings })))
```

Add the route inside the `<AuthGuard>` routes, after the `/env` route:

```tsx
<Route path="/settings" element={<SystemSettings />} />
```

- [ ] **Step 2: Add sidebar nav entry in `app-sidebar.tsx`**

In the `navMain` array (inside `React.useMemo`), add the System Settings entry after the Env Variables entry:

```ts
{ title: t("nav.settings"), url: "/env", icon: <SettingsIcon /> },
{ title: t("nav.systemSettings"), url: "/settings", icon: <SettingsIcon /> },
```

The `SettingsIcon` import is already present in the file.

- [ ] **Step 3: Add breadcrumb entry in `breadcrumb-config.ts`**

Add a new entry to the `ROUTES` array (e.g., after the `/tasks` entry):

```ts
{
  test: /^\/settings$/,
  crumbs: [{ labelKey: "nav.systemSettings" }],
},
```

- [ ] **Step 4: Verify TypeScript compiles**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee2/web
pnpm tsc --noEmit
```

Expected: exits 0 with no errors.

- [ ] **Step 5: Commit**

```bash
git add web/src/app.tsx web/src/components/app-sidebar.tsx web/src/lib/breadcrumb-config.ts
git commit -m "feat(web): wire /settings route, sidebar nav, and breadcrumb"
```

---

## Post-Implementation Verification

After all tasks are done, run the full backend test suite and frontend type check:

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee2
go test ./... 2>&1 | tail -20

cd web
pnpm tsc --noEmit
```

Both should exit 0.
