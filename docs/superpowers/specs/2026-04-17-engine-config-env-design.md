# Design: Engine config.yaml env Configuration Support

**Date:** 2026-04-17
**Branch:** feat/worker-engine-selection

## Overview

Add support for configuring static environment variables per engine in `config.yaml`. These env vars are injected into the engine process at runtime with the lowest priority — they are overridden by all DB-backed scopes (global, department, worker, bee).

## Background

The system already supports a multi-layer env injection chain for worker/bee execution:

```
baseEnv (OPENBEE_URL)
  → extraEnv (DB: global → dept → worker / bee)
  → OPENBEE_API_KEY  (system-reserved, always last)
```

Pi and Kimi adapters already read `cfg.Raw["env"]` and merge it into `baseEnv`, but `EngineConfigRawFor` never populates that key, so the feature is silently a no-op. Claude and Codex have no extraEnv support at all.

## Goal

Enable operators to set static env vars per engine in `config.yaml`, e.g.:

```yaml
bee:
  engines:
    claude:
      enabled: true
      path: claude
      env:
        ANTHROPIC_BASE_URL: https://my-proxy.example.com
    kimi:
      enabled: true
      path: kimi
      env:
        MOONSHOT_API_KEY: sk-xxx
    pi:
      enabled: true
      env:
        PI_CUSTOM_VAR: value
    codex:
      enabled: true
```

## Priority Semantics

Config env is merged into the invoker's `baseEnv` at startup. The final env assembled by `BuildRunEnv` at execution time is:

```
baseEnv (OPENBEE_URL + config.yaml env vars)   ← lowest priority
  ↓ appended, later keys win
extraEnv (DB: global → dept → worker / bee)
  ↓
OPENBEE_API_KEY                                 ← system-reserved, never overridable
```

A key set in config.yaml will be overridden by the same key set in any DB-backed scope. `OPENBEE_API_KEY` is always last and cannot be overridden by anything. This behavior is identical for both worker and bee execution paths.

## Changes

### 1. `internal/infra/config/config.go`

Add `Env` field to `EngineItemConfig`:

```go
type EngineItemConfig struct {
    Enabled bool              `yaml:"enabled"`
    Path    string            `yaml:"path"`
    Env     map[string]string `yaml:"env"`  // static env vars, lowest priority
}
```

Update `EngineConfigRawFor` to include env:

```go
func (b BeeConfig) EngineConfigRawFor(name string) map[string]any {
    item := b.Engines.itemFor(name)
    if item.Path == "" {
        return nil
    }
    return map[string]any{
        "path": item.Path,
        "env":  item.Env,
    }
}
```

Note: when `item.Env` is nil, the type assertion `cfg.Raw["env"].(map[string]string)` in adapters returns a zero-value empty map, which is safe.

### 2. `internal/ai/claude/adapter.go`

Read `env` from Raw and pass to `NewAdapter` (mirroring pi/kimi pattern):

```go
func init() {
    ai.Register(ai.EngineClaude, func(cfg ai.EngineConfig) (ai.EngineAdapter, error) {
        path, _ := cfg.Raw["path"].(string)
        if path == "" {
            path = ai.EngineClaude
        }
        extraEnv, _ := cfg.Raw["env"].(map[string]string)
        return NewAdapter(path, cfg.OpenbeeURL, extraEnv)
    })
}
```

### 3. `internal/ai/claude/invoker.go`

`NewInvoker` accepts `extraEnv` and merges into `baseEnv`:

```go
func NewInvoker(binary, openbeeURL string, extraEnv map[string]string) (*Invoker, error) {
    base := ai.BuildBaseEnv(openbeeURL)
    for k, v := range extraEnv {
        if v != "" {
            base = append(base, k+"="+v)
        }
    }
    return &Invoker{binary: binary, baseEnv: base}, nil
}
```

### 4. `internal/ai/codex/adapter.go` and `internal/ai/codex/invoker.go`

Same changes as claude — identical pattern.

### 5. Pi and Kimi

No adapter/invoker changes needed. The config-side fix (sections 1 above) is sufficient to complete the already-wired path.

## What Is Not Changed

- `BuildRunEnv` signature and logic — no changes
- `RunOptions` — no changes
- `ResolveWorkerEnv` / `ResolveBeeEnv` — no changes
- DB-backed env scope system — no changes
- `OPENBEE_API_KEY` handling — no changes

## Testing

- Unit tests for `EngineConfigRawFor` with and without `Env` set
- Unit tests for claude/codex invoker: verify `extraEnv` keys appear in `baseEnv`
- Integration-level smoke: start a worker with a config-env key and confirm it appears in the process env (lower priority than a same-named DB env var)
