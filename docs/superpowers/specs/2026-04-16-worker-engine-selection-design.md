# Worker Per-Engine Selection Design

**Date:** 2026-04-16  
**Status:** Approved

## Overview

Allow each Worker agent to independently select its AI engine (Claude Code, Codex, or Pi). Currently all workers share the single global engine configured in `config.yaml`. This feature adds an `engine` field to the Worker model so each worker can run on a different engine.

## Decisions

- **Scope:** Engine type per worker (`claude`/`codex`/`pi`). Engine binary paths, timeouts, and env vars remain global in `config.yaml`.
- **Default:** Workers with no engine set fall back to the global `bee.engine` (backward compatibility for existing workers).
- **Mutability:** Engine can be changed after worker creation.
- **Validation:** Engine is required when creating or editing a worker via the UI/API.
- **Architecture:** Pre-built engine map (Approach A) — all engine adapters instantiated at startup and shared across workers.

## Section 1: Data Layer

### `internal/infra/model/worker.go`

Add field to `Worker` struct:

```go
Engine string `json:"engine" db:"engine"`
```

### `internal/infra/store/db.go`

Append migration:

```go
{
    name: "add_engine_to_bee_workers",
    sql:  `ALTER TABLE bee_workers ADD COLUMN engine TEXT NOT NULL DEFAULT ''`,
}
```

### `internal/infra/store/worker_store.go`

- Add `engine` to `workerColumns` and `workerColumnsAliased` constants
- Update `Create` INSERT to include `engine`
- Update `Update` SET clause to include `engine = ?`
- Update `scanWorker` to scan `&w.Engine`

## Section 2: Engine Wiring (`internal/app/app.go`)

### New helper: `EngineConfigRawFor`

Add to `config.BeeConfig`:

```go
func (b BeeConfig) EngineConfigRawFor(name string) map[string]any {
    switch name {
    case "claude":
        return map[string]any{"path": b.Claude.Path}
    case "codex":
        return map[string]any{"path": b.Codex.Path}
    case "pi":
        return map[string]any{"path": b.Pi.Path, "env": b.Pi.Env}
    default:
        return nil
    }
}
```

### Replace `buildEngine` → `buildAllEngines`

```go
func buildAllEngines(cfg config.BeeConfig) (map[string]ai.EngineAdapter, error) {
    names := []string{ai.EngineClaude, ai.EngineCodex, ai.EnginePi}
    result := make(map[string]ai.EngineAdapter, len(names))
    for _, name := range names {
        adapter, err := ai.New(name, ai.EngineConfig{
            OpenbeeURL: cfg.MCPBaseURL,
            Raw:        cfg.EngineConfigRawFor(name),
        })
        if err != nil {
            return nil, fmt.Errorf("init engine %q: %w", name, err)
        }
        result[name] = adapter
    }
    return result, nil
}
```

### `BuildApp` wiring

- Call `buildAllEngines(cfg.Bee)` → `engines map[string]ai.EngineAdapter`
- `buildBee` receives `engines[cfg.Bee.EffectiveEngine()]` (Bee coordinator unchanged)
- `buildWorkerManager` receives full `engines` map

## Section 3: Worker Manager (`internal/domain/worker/manager.go`)

### Struct change

```go
type Manager struct {
    engines       map[string]ai.EngineAdapter
    defaultEngine string
    // ... rest unchanged
}
```

### `resolveEngine` helper

```go
func (m *Manager) resolveEngine(w model.Worker) ai.EngineAdapter {
    if w.Engine != "" {
        if e, ok := m.engines[w.Engine]; ok {
            return e
        }
        log.Warn("unknown engine on worker, falling back to default",
            zap.String("worker_id", w.ID), zap.String("engine", w.Engine))
    }
    return m.engines[m.defaultEngine]
}
```

### `CreateWorkerParams`

Add `Engine string` field.

### `CreateWorker`

- Store `Engine` from params into `model.Worker`
- Use `resolveEngine(worker)` when calling `Prepare`

### `ExecuteWorker`

- Replace `m.engine.Prepare(...)` and `m.engine.Run(...)` calls with `m.resolveEngine(worker).Prepare(...)` / `m.resolveEngine(worker).Run(...)`

## Section 4: API Layer (`internal/api/worker_handler.go`)

### Engine validation helper

```go
var validEngines = map[string]bool{
    ai.EngineClaude: true,
    ai.EngineCodex:  true,
    ai.EnginePi:     true,
}

func validateEngine(name string) error {
    if !validEngines[name] {
        return fmt.Errorf("unknown engine %q, valid values: claude, codex, pi", name)
    }
    return nil
}
```

### `createWorkerRequest`

```go
type createWorkerRequest struct {
    Name             string `json:"name" binding:"required"`
    Engine           string `json:"engine" binding:"required"`  // new, required
    Description      string `json:"description"`
    Memory           string `json:"memory"`
    WorkDir          string `json:"work_dir"`
    PermissionScopes string `json:"permission_scopes"`
}
```

Handler validates engine name via `validateEngine` before calling manager.

### Update handler

Patch request adds:

```go
Engine *string `json:"engine"`
```

If provided, must be non-empty and pass `validateEngine`. Existing `engine` value is preserved if not included in patch.

## Section 5: Frontend

### `web/src/components/create-worker-sheet.tsx`

- Add `engine` state (default: `"claude"`)
- Add `<Select>` dropdown with options: `claude / codex / pi`
- Mark as required field
- Include `engine` in the create/update request body

### `web/src/pages/worker-detail.tsx`

- Display engine field in worker detail view
- Allow editing engine via same dropdown (required)
- Submit engine in PATCH request

### i18n keys

Add to locale files:
- `workers.form.engine` — label
- `workers.form.engineHelper` — helper text
- Engine name labels: `workers.engine.claude`, `workers.engine.codex`, `workers.engine.pi`

## Backward Compatibility

- Existing workers with `engine = ''` in the DB continue to use the global default engine at runtime.
- The `resolveEngine` method handles the empty-string fallback transparently.
- No data migration needed beyond adding the column with `DEFAULT ''`.

## Error Handling

- Create/Update with invalid engine name → `400 Bad Request`
- Worker with unknown engine at execute time → warn log + fallback to default (no crash)

## Testing

- Unit test `resolveEngine` with valid engine, empty engine (fallback), unknown engine (fallback + warn)
- Unit test `validateEngine`
- Integration test: create worker with engine, execute, verify correct engine adapter used
- Frontend: engine dropdown shown and required in create/edit forms
