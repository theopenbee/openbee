# System Settings Page Design

**Date:** 2026-04-17
**Status:** Approved

---

## Background

The `default_engine` system config (stored in `bee_system_configs`) is currently only changeable via the `/engine` slash command in chat. There is no Web UI for system-level configuration. This feature adds a new standalone `/settings` page that lets admins view and update `default_engine` through the web interface.

---

## Goal

- Add `GET /api/system-configs` and `PUT /api/system-configs/:key` API endpoints
- Add a new `/settings` page in the web UI with a sidebar nav entry
- Wire up `default_engine` as the first configurable item on the page

---

## Backend Design

### New handler: `internal/api/system_config_handler.go`

```go
type SystemConfigHandler struct {
    store     SystemConfigStore  // Get(ctx, key) / Set(ctx, key, value)
    validator EngineValidator    // ValidateEngine(name) / EnabledEngines()
}
```

Both interfaces already exist (`SystemConfigStore` subset and `EngineValidator`) and can be reused as-is.

### API Endpoints

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/api/system-configs` | JWT | Returns all known system config keys as a JSON object |
| PUT | `/api/system-configs/:key` | JWT | Updates a single key; validates value for known keys |

**GET `/api/system-configs` response:**
```json
{
  "default_engine": "claude"
}
```
If a key has no DB row yet, its value is returned as `""` (frontend treats empty string as "use system default").

**PUT `/api/system-configs/:key` request body:**
```json
{ "value": "claude" }
```

**Validation rules:**
- `default_engine`: value must pass `EngineValidator.ValidateEngine()`; on success also calls `enginecfg.Set(value)` to sync the in-memory cache (same as `/engine` slash command)
- Unknown keys: return 400

**Route registration:** Added to the JWT-protected `/api` group in `internal/routes/api.go`.

**`routes.ServerParams`** gains a `SystemConfigs *api.SystemConfigHandler` field.

**`buildAPIServer` in `internal/app/app.go`** constructs the handler with `store.NewSystemConfigStore(db)` and the existing `mgr` (which implements `EngineValidator`).

---

## Frontend Design

### New page: `web/src/pages/settings.tsx`

Route: `/settings`

The page uses the same `FadeIn` + `PageHeader` + `DetailSection` layout pattern as the existing pages.

**Page structure:**
```
System Settings          ← PageHeader
└── Engine Configuration ← DetailSection
    ├── Title label: "Default Engine"
    ├── Hint text: "The AI engine Bee uses by default when no per-worker engine is set."
    └── Select dropdown
        ├── Placeholder option: "System Default (claude)" (shown when value is "")
        ├── Options: one per entry in AppConfig.enabled_engines
        ├── Option labels: reuse existing workers.engines.* i18n keys
        └── Current value fetched from GET /api/system-configs
```

**Interaction:**
- On mount: fetch `GET /api/system-configs`, populate dropdown
- On change: immediately call `PUT /api/system-configs/default_engine`
  - Success: toast "Default engine updated"
  - Error: toast with error message
- Dropdown is disabled during loading and mutation

### API client additions (`web/src/lib/api.ts`)

```ts
systemConfigs: {
  get: () => fetchAPI<Record<string, string>>("/system-configs"),
  set: (key: string, value: string) =>
    fetchAPI(`/system-configs/${key}`, {
      method: "PUT",
      body: JSON.stringify({ value }),
    }),
}
```

No changes to the `AppConfig` type — system configs are fetched via their own endpoint.

### Sidebar (`web/src/components/app-sidebar.tsx`)

Add a new entry to `navMain`:
```ts
{ title: t("nav.systemSettings"), url: "/settings", icon: <SettingsIcon /> }
```

The existing `/env` entry (Env Variables) is preserved unchanged.

### Router (`web/src/app.tsx`)

Add lazy import and route:
```tsx
const SystemSettings = lazy(() => import("@/pages/settings").then(m => ({ default: m.SystemSettings })))
// ...
<Route path="/settings" element={<SystemSettings />} />
```

### Breadcrumb (`web/src/lib/breadcrumb-config.ts`)

Add route entry:
```ts
{ test: /^\/settings$/, crumbs: [{ labelKey: "nav.systemSettings" }] }
```

---

## i18n Keys

### `web/src/locales/en.json`

```json
"nav": {
  "systemSettings": "System Settings"
},
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

### `web/src/locales/zh.json`

```json
"nav": {
  "systemSettings": "系统设置"
},
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

---

## File Map

| File | Change |
|------|--------|
| `internal/api/system_config_handler.go` | New — GET + PUT handler |
| `internal/routes/server.go` | Add `SystemConfigs` field to `ServerParams` |
| `internal/routes/api.go` | Register GET/PUT system-configs routes |
| `internal/app/app.go` | Construct and wire `SystemConfigHandler` |
| `web/src/pages/settings.tsx` | New — System Settings page |
| `web/src/app.tsx` | Add lazy import + `/settings` route |
| `web/src/components/app-sidebar.tsx` | Add "System Settings" nav entry |
| `web/src/lib/breadcrumb-config.ts` | Add `/settings` breadcrumb |
| `web/src/lib/api.ts` | Add `systemConfigs.get` and `systemConfigs.set` |
| `web/src/locales/en.json` | Add `nav.systemSettings` + `systemSettings.*` keys |
| `web/src/locales/zh.json` | Add `nav.systemSettings` + `systemSettings.*` keys |

---

## Constraints

- Only `default_engine` is supported in this iteration; other keys return 400
- Empty string value in GET response means "no override set; system falls back to config.yaml default (claude)"
- PUT `default_engine` calls `enginecfg.Set()` to keep in-memory cache in sync — same behavior as `/engine` slash command
