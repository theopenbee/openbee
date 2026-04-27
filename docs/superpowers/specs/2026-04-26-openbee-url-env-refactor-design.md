# Design: Remove OpenbeeURL from EngineConfig Parameter Chain

**Date:** 2026-04-26  
**Branch:** feat/tokenstat-engine-cohesion  
**Status:** Approved

---

## Problem

`OPENBEE_URL` is conceptually an environment variable — optional, like `OPENBEE_API_KEY` — but it is threaded explicitly through the entire engine construction chain:

```
cfg.MCPBaseURL
  → EngineConfig.OpenbeeURL
  → NewAdapter(binaryPath, openbeeURL, extraEnv)
  → NewInvoker(binary, openbeeURL, extraEnv)
  → BuildBaseEnv(openbeeURL)
  → env = append(env, "OPENBEE_URL="+openbeeURL)
```

This is inconsistent with `OPENBEE_API_KEY`, which is injected per-run via `BuildRunEnv` without appearing in any constructor signature. The explicit chain adds noise to every engine package and makes `EngineConfig` carry a field that is infrastructural rather than engine-specific.

---

## Solution

Inject `OPENBEE_URL` into the server process environment once at startup via `os.Setenv`. Since `BuildBaseEnv` already snapshots `os.Environ()` to build the subprocess base environment, child processes inherit the URL with no further plumbing required.

```
os.Setenv("OPENBEE_URL", cfg.MCPBaseURL)   ← once, in buildAllEngines
os.Environ() already contains it
  → BuildBaseEnv() inherits it
  → subprocess gets OPENBEE_URL set
```

---

## Resulting Symmetry

| Variable | Injection point | Mechanism |
|----------|----------------|-----------|
| `OPENBEE_URL` | Server startup (fixed per process) | `os.Setenv` once; inherited by all subprocesses |
| `OPENBEE_API_KEY` | Each `Run` call (dynamic per execution) | `BuildRunEnv` injects fresh value each time |

Both are pure env-var semantics. Neither appears in engine constructor signatures.

---

## Changes

### `internal/app/app.go`

In `buildAllEngines`, add `os.Setenv` before the engine loop and remove the `OpenbeeURL` assignment from `EngineConfig{}`:

```go
func buildAllEngines(cfg config.BeeConfig) (map[string]ai.EngineAdapter, error) {
    os.Setenv("OPENBEE_URL", cfg.MCPBaseURL) // inject once for all subprocesses

    result := make(map[string]ai.EngineAdapter)
    for _, name := range ai.AllEngines() {
        if !cfg.Engines.IsEnabled(name) {
            continue
        }
        adapter, err := ai.New(name, ai.EngineConfig{
            Raw: cfg.EngineConfigRawFor(name), // OpenbeeURL removed
        })
        ...
    }
    ...
}
```

### `internal/ai/registry.go`

Remove `OpenbeeURL string` from `EngineConfig`:

```go
type EngineConfig struct {
    // Raw holds engine-specific configuration (parsed from config.yaml).
    Raw map[string]any
}
```

### `internal/ai/process.go`

Remove `openbeeURL string` parameter from `BuildBaseEnv`; the URL is already present in `os.Environ()`:

```go
// BuildBaseEnv constructs the base environment for engine subprocesses.
// It prepends the current executable's directory to PATH.
// OPENBEE_URL is inherited from the server process environment.
func BuildBaseEnv() []string {
    sysEnv := os.Environ()
    env := make([]string, 0, len(sysEnv)+1)
    if exePath, err := os.Executable(); err == nil {
        oldPath := os.Getenv("PATH")
        for _, e := range sysEnv {
            if !strings.HasPrefix(e, "PATH=") {
                env = append(env, e)
            }
        }
        env = append(env, "PATH="+filepath.Dir(exePath)+string(os.PathListSeparator)+oldPath)
    } else {
        env = append(env, sysEnv...)
    }
    return env[:len(env):len(env)]
}
```

### Engine packages (claude, kimi, codex, pi)

Each engine's `adapter.go` and `invoker.go` loses the `openbeeURL string` parameter:

**adapter.go** (same pattern for all four engines):
```go
func init() {
    ai.Register(ai.EngineClaude, func(cfg ai.EngineConfig) (ai.EngineAdapter, error) {
        return NewAdapter(cfg.PathOrDefault(ai.EngineClaude), cfg.ExtraEnv()), nil
    })
}

func NewAdapter(binaryPath string, extraEnv map[string]string) ai.EngineAdapter { ... }
```

**invoker.go** (same pattern for all four engines):
```go
func NewInvoker(binary string, extraEnv map[string]string) *Invoker {
    base := ai.BuildBaseEnv()
    return &Invoker{binary: binary, baseEnv: ai.AppendExtraEnv(base, extraEnv)}
}
```

`codex/invoker.go` also has a `store *SessionStore` param that stays:
```go
func NewInvoker(binary string, store *SessionStore, extraEnv map[string]string) *Invoker { ... }
```

`pi/invoker.go` returns an error (for `MkdirAll`) so its signature is:
```go
func NewInvoker(binary string, extraEnv map[string]string) (*Invoker, error) { ... }
```

### `internal/ai/claude/invoker_test.go`

`TestNewInvoker` sets `OPENBEE_URL` via `t.Setenv` (which auto-restores after the test) and removes the parameter from the `NewInvoker` call. The assertion that `OPENBEE_URL=http://localhost:8080` appears in `baseEnv` remains valid because `BuildBaseEnv()` snapshots `os.Environ()`:

```go
func TestNewInvoker(t *testing.T) {
    t.Setenv("OPENBEE_URL", "http://localhost:8080")
    inv := NewInvoker("/usr/bin/claude", nil)
    ...
    // assertion unchanged: baseEnv must contain OPENBEE_URL=http://localhost:8080
}
```

`internal/ai/pi/invoker_test.go` also calls `NewInvoker("true", "http://localhost:8080", nil)` in two tests — apply the same `t.Setenv` + signature update. Kimi and codex invoker tests have no direct `NewInvoker` calls with `openbeeURL`, so they need no test changes.

---

## Scope

- No behavioral change: subprocesses continue to receive `OPENBEE_URL` exactly as before.
- No config schema change: `MCPBaseURL` is still derived from `server.host:port`.
- Tests remain meaningful — `t.Setenv` restores the original value after each test, so there is no global state leak.
- `os.Setenv` is called once at server startup before any engine goroutine is created, so there is no race condition.
