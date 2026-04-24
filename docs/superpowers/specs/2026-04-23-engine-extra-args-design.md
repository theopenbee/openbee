# Engine Extra Args Configuration Design

**Date:** 2026-04-23  
**Status:** Approved

## Overview

Add support for configuring extra CLI arguments for AI engines (claude/codex/pi/kimi) at multiple scopes: global (bee + all workers), bee-specific, and per-worker. This allows operators to pass arguments like `--model <model>` and `--effort <level>` without modifying code.

## Background

Currently, CLI arguments passed to each engine invoker are hardcoded. The only runtime-configurable mechanism is environment variables via `config.yaml`. There is no way to dynamically set engine-specific flags like `--model` or `--effort` at runtime.

## Design Decisions

1. **Engine-specific**: Extra args are grouped by engine name (claude/codex/pi/kimi), so each engine has its own independent set of args.
2. **Smart merge (by key)**: When multiple config levels define args for the same engine, they are merged by argument key — deeper/more-specific levels override the same key from higher levels. Non-conflicting keys from all levels are preserved.
3. **Input format**: API accepts raw CLI strings per engine (e.g. `"--model claude-sonnet-4-5 --effort high"`); backend parses into a map for storage.
4. **Storage**: Reuse `bee_system_configs` table for global/bee scopes; add `engine_extra_args` column to `bee_workers` for per-worker scope.
5. **Naming**: All new fields/keys carry the `engine_` prefix for consistency.

## Configuration Scopes

| Scope | Storage Key / Field | Applies To |
|-------|---------------------|------------|
| Global | `engine_extra_args_global` in `bee_system_configs` | Bee process + all workers |
| Bee-specific | `engine_extra_args_bee` in `bee_system_configs` | Bee process only |
| Worker-specific | `engine_extra_args` column in `bee_workers` | That worker only |

## Data Format

### Stored in DB (JSON map of maps)

```json
{
  "claude": { "model": "claude-sonnet-4-5", "effort": "high" },
  "codex":  { "model": "o3" },
  "kimi":   { "model": "kimi-k2" }
}
```

Boolean flags (no value) use an empty string as value:
```json
{ "verbose": "" }
```

### API Input (raw CLI string per engine, parsed by backend)

```json
{
  "claude": "--model claude-sonnet-4-5 --effort high",
  "codex":  "--model o3"
}
```

Parsing rules:
- Split on whitespace
- `--key value` pairs: key = `key`, value = `value`
- `--flag` alone (next token starts with `--` or end of string): key = `flag`, value = `""`

## Merge Logic at Runtime

### Launching Bee (using engine X)

```
final_args[X] = merge(global[X], bee[X])
```
`bee[X]` overrides same-named keys from `global[X]`; non-conflicting keys from both are kept.

### Launching Worker (using engine X)

```
final_args[X] = merge(global[X], worker.engine_extra_args[X])
```
`worker[X]` overrides same-named keys from `global[X]`.

### Merge Function

```
func mergeArgs(base, override map[string]string) map[string]string:
    result = copy(base)
    for k, v in override:
        result[k] = v
    return result
```

### Building CLI Args from Map

```
for key, value in merged_args:
    if value == "":
        append "--{key}"
    else:
        append "--{key}", "{value}"
```

## API Endpoints

### System Config

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/system-configs/engine_extra_args_global` | Get global extra args |
| `PUT` | `/api/system-configs/engine_extra_args_global` | Set global extra args |
| `GET` | `/api/system-configs/engine_extra_args_bee` | Get bee extra args |
| `PUT` | `/api/system-configs/engine_extra_args_bee` | Set bee extra args |

Request/response body for PUT:
```json
{
  "value": "{\"claude\": \"--model claude-sonnet-4-5 --effort high\"}"
}
```
(value is a JSON-encoded string; backend parses the inner string map)

### Worker API

`POST /api/workers` and `PUT /api/workers/:id` accept a new field:
```json
{
  "engine_extra_args": {
    "claude": "--model claude-opus-4-7",
    "codex":  "--model o3-mini"
  }
}
```

## Schema Changes

### `bee_system_configs` table
No schema change. Two new keys are inserted/upserted:
- `engine_extra_args_global`
- `engine_extra_args_bee`

New Go constants:
```go
const (
    SystemConfigKeyEngineExtraArgsGlobal = "engine_extra_args_global"
    SystemConfigKeyEngineExtraArgsBee    = "engine_extra_args_bee"
)
```

### `bee_workers` table
Add new column:
```sql
ALTER TABLE bee_workers ADD COLUMN engine_extra_args TEXT NOT NULL DEFAULT '{}';
```

Go model addition:
```go
type Worker struct {
    // ... existing fields ...
    EngineExtraArgs string `db:"engine_extra_args"` // JSON: map[engine]map[arg]value
}
```

## Go Types

```go
// EngineExtraArgsMap maps engine name -> arg key -> arg value
type EngineExtraArgsMap map[string]map[string]string

// Parse raw CLI string per engine into structured map
func ParseEngineExtraArgs(raw map[string]string) (EngineExtraArgsMap, error)

// Merge two EngineExtraArgsMaps (override wins on key conflict)
func MergeEngineExtraArgs(base, override EngineExtraArgsMap) EngineExtraArgsMap

// Build CLI arg slice from a single engine's args map
func BuildExtraArgSlice(args map[string]string) []string
```

## Files to Change

| File | Change |
|------|--------|
| `internal/infra/model/system_config.go` | Add two new key constants |
| `internal/infra/model/worker.go` | Add `EngineExtraArgs` field |
| `internal/infra/store/worker_store.go` | Include `engine_extra_args` in queries |
| `internal/ai/engine.go` | Add `ExtraArgs map[string]string` to `RunOptions` |
| `internal/ai/extra_args.go` (new) | `ParseEngineExtraArgs`, `MergeEngineExtraArgs`, `BuildExtraArgSlice` |
| `internal/ai/claude/invoker.go` | Append `RunOptions.ExtraArgs` to CLI args |
| `internal/ai/codex/invoker.go` | Same |
| `internal/ai/pi/invoker.go` | Same |
| `internal/ai/kimi/invoker.go` | Same |
| `internal/domain/worker/manager.go` | Load global config, merge with worker args, pass in `RunOptions` |
| `internal/domain/bee/bee_process.go` | Load global + bee config, merge, pass in `RunOptions` |
| `internal/api/system_config_handler.go` | Support GET/PUT for the two new keys |
| `internal/api/worker_handler.go` | Accept and persist `engine_extra_args` |
| DB migration | Add `engine_extra_args` column to `bee_workers` |

## Out of Scope

- Frontend UI changes (follow-up task)
- Per-department extra args
- Validating that a given arg is supported by the target engine
