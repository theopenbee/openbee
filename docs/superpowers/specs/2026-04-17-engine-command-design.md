# `/engine` Command & Default Engine Config — Design Spec

**Date:** 2026-04-17
**Branch:** feat/worker-engine-selection

---

## Overview

Add a `/engine` slash command that lets users switch the default bee engine and per-worker engine at runtime via chat messages. The command is intercepted in the ingest layer before reaching the bee AI, validated, applied immediately via a global in-memory cache backed by a new `bee_system_configs` table, and replied to inline.

---

## Requirements

```
/engine {engine}              # Set default bee engine (updates system config + global cache)
/engine {engine} {workerName} # Set a specific worker's engine (updates bee_workers.engine)
```

Valid engine names: `claude`, `codex`, `pi`, `kimi` (sourced from `ai.AllEngines`).

---

## Data Model

### New table: `bee_system_configs` (Migration v37)

```sql
CREATE TABLE IF NOT EXISTS bee_system_configs (
    key        TEXT PRIMARY KEY,
    value      TEXT NOT NULL,
    updated_at INTEGER NOT NULL
);
```

- Simple key/value; no encryption (engine names are not sensitive).
- No scope column — this is system-wide global configuration.
- Initial key used: `default_engine`.

### Model & Store

**`internal/infra/model/system_config.go`**
```go
type SystemConfig struct {
    Key       string
    Value     string
    UpdatedAt int64
}

const SystemConfigKeyDefaultEngine = "default_engine"
```

**`internal/infra/store/system_config_store.go`**
- `Get(ctx, key) (SystemConfig, bool, error)` — returns (config, found, err)
- `Set(ctx, key, value string) error` — upsert by key

---

## Global Engine Cache

**`internal/domain/enginecfg/default.go`**

A package-level, RWMutex-protected variable holding the active default engine name. Updated immediately when `/engine {engine}` is executed, so all in-flight goroutines see the new value on their next read.

```go
func Init(engine string)   // called once at app startup
func Get() string          // read current default engine
func Set(engine string)    // write new default engine
```

### Startup Initialization

```
1. Query bee_system_configs WHERE key = 'default_engine'
2. Row found  → enginecfg.Init(row.Value)
3. Row absent → enginecfg.Init(cfg.Bee.EffectiveEngine())
```

The DB value takes precedence over the config file, so a `/engine` change persists across restarts.

---

## Dynamic Engine Routing

### `ai.DynamicAdapter` — `internal/ai/dynamic.go`

Wraps `map[string]ai.EngineAdapter`. On each `Run()` / `ExtractResult()` call it reads `enginecfg.Get()` to select the active adapter. `Prepare()` calls `Prepare()` on every available adapter once at startup.

```go
type DynamicAdapter struct {
    engines map[string]ai.EngineAdapter
}

func NewDynamicAdapter(engines map[string]ai.EngineAdapter) *DynamicAdapter

func (d *DynamicAdapter) Prepare(workDir string, opts ai.PrepareOptions) error   // calls all engines
func (d *DynamicAdapter) Run(...) (ai.Process, <-chan ai.Output, error)           // routes via enginecfg.Get()
func (d *DynamicAdapter) ExtractResult(logPath string) string                    // routes via enginecfg.Get()
```

`DynamicAdapter` is passed to `bee.NewBeeProcess` in `app.go` instead of the fixed single-engine adapter.

---

## Command Interception

### `CommandHandler` interface — `internal/domain/msgingest/command.go`

```go
type CommandHandler interface {
    HandleCommand(ctx context.Context, content string, replyTo platform.InboundMessage) (handled bool)
}
```

### `EngineCommandHandler` — `internal/domain/command/engine.go`

Implements `CommandHandler`. Dependencies injected at construction:

| Dependency | Purpose |
|---|---|
| `WorkerStore` | Look up worker by name; update `engine` field |
| `SystemConfigStore` | Persist `default_engine` to DB |
| `map[string]platform.Sender` | Reply to the originating platform |

**Parse logic:**

| Input tokens | Action |
|---|---|
| `["/engine"]` | Reply with usage help |
| `["/engine", engine]` | Validate engine → update DB + `enginecfg.Set()` → reply |
| `["/engine", engine, workerName]` | Validate engine → find worker → update `bee_workers.engine` → reply |
| Other | `handled = false` (let message pass through to bee) |

**Reply messages (locale: zh-CN):**

| Scenario | Reply |
|---|---|
| Bee engine switched | `已将默认 engine 切换为 {engine}` |
| Worker engine switched | `已将 Worker "{name}" 的 engine 切换为 {engine}` |
| Unknown engine | `未知的 engine: {name}，支持的 engine：claude / codex / pi / kimi` |
| Worker not found | `Worker "{name}" 不存在` |
| Bad format | `用法：\n/engine {engine} — 切换默认 engine\n/engine {engine} {workerName} — 切换指定 worker 的 engine` |

### `msgingest.Gateway` changes

Add optional `CommandHandler` field. New functional option `WithCommandHandler(h CommandHandler)`.

In `onDebounce`, **before** the `CreateBatch` DB write:

```go
if g.commandHandler != nil {
    if handled := g.commandHandler.HandleCommand(ctx, content, msgs[n-1]); handled {
        return  // skip DB write and Out() emit
    }
}
// existing batch-write + emit logic unchanged
```

Command messages are never stored to the message DB and never reach bee.

---

## Components Updated for Dynamic Engine

### `bee/feeder.go`

Replace all 5 calls to `f.cfg.EffectiveEngine()` with `enginecfg.Get()`. No other changes.

### `task/dispatcher.go`

Replace `d.engineName` fallback (2 sites — `resolveWorkerEngine` and `ClearSession`) with `enginecfg.Get()`. The `WithEngine` option and `engineName` field can be removed.

---

## App Wiring (`app/app.go`)

```
1. Open DB, run migrations (v37 creates bee_system_configs)
2. Create SystemConfigStore
3. Load default engine from DB → enginecfg.Init(...)
4. Create ai.DynamicAdapter(engines)
5. Create EngineCommandHandler(workerStore, systemConfigStore, senders)
6. Create msgingest.Gateway(..., msgingest.WithCommandHandler(handler))
7. Create localIngest with the same CommandHandler injected (local chat also supports /engine)
8. buildBee: pass DynamicAdapter to bee.NewBeeProcess (remove engines[cfg.Bee.EffectiveEngine()] call)
9. buildPipeline: remove engineName param (TaskDispatcher no longer needs it)
```

---

## Testing

- Unit test for command parsing: valid/invalid engine, valid/invalid worker, format errors
- Unit test for `enginecfg`: concurrent Get/Set safety
- Unit test for `DynamicAdapter.Run`: routes to correct engine based on cache value
- Integration: `onDebounce` with command handler returning `handled=true` — no DB write, no emit

---

## Out of Scope

- Permission/role checks on `/engine` command
- Hot-reload of engine-specific config (API keys, etc.) — engine selection only
