# Design: Remove Dead Error Path from resolveEngine

**Date:** 2026-04-24
**Branch:** feat/token-usage-stats

## Problem

`resolveEngine` in `internal/domain/worker/manager.go` currently returns `(string, ai.EngineAdapter, error)`. The error path is dead code — it can never be reached in production — but callers are forced to handle it, and `ExecuteWorker` has a 12-line error-handling block that creates a failed execution record with `engine = ""`. This adds noise and misleads readers into thinking engine resolution can fail at runtime.

## Why the Error Is Unreachable

Three invariants together make the error path impossible:

1. **Startup check** (`app.go:112-114`): `BuildApp` returns an error if the configured default engine is not in the enabled engines map. The process does not start.

2. **Runtime validation** (`ValidateEngine`): The `/engine` switch command validates the engine name via `ValidateEngine` before calling `engineCfg.Set`. Only enabled engines are accepted.

3. **DB recovery guard** (`app.go:122-127`): When loading the default engine from DB at startup, the code only applies the DB value if it is a currently-enabled engine; otherwise it falls back to the config default (which already passed invariant 1).

Therefore, `m.engineCfg.Get()` always returns a name present in `m.engines`, and `resolveEngine` can never reach its error return.

## Solution

### 1. Change `resolveEngine` signature

Remove the `error` return. The function either returns a valid worker-specific engine or the always-valid default.

```go
func (m *Manager) resolveEngine(w model.Worker) (string, ai.EngineAdapter)
```

Internal logic: if the worker's engine is not found, log a warning and fall through to the default. Access `m.engines[defaultEngine]` directly — startup guarantees it is non-nil.

### 2. Update `CreateWorker` call site

```go
// Before
_, engine, err := m.resolveEngine(workerModel)
if err != nil {
    return model.Worker{}, err
}

// After
_, engine := m.resolveEngine(workerModel)
```

### 3. Update `ExecuteWorker` call site

Remove the entire 12-line error-handling block. The Create call moves up immediately after resolution.

```go
// Before
engineName, engine, err := m.resolveEngine(worker)
if err != nil {
    exec, createErr := m.executionStore.Create(workerID, triggerInput, sessionID, "")
    // ... 10 lines of error handling
    return exec, err
}
exec, err := m.executionStore.Create(workerID, triggerInput, sessionID, engineName)

// After
engineName, engine := m.resolveEngine(worker)
exec, err := m.executionStore.Create(workerID, triggerInput, sessionID, engineName)
```

### 4. Test cleanup

Delete `TestExecuteWorker_resolveEngineFails` in `manager_test.go`. This test exercises a scenario that can no longer happen. The remaining tests (worker with explicit engine, worker with empty engine falling to default, worker with unknown engine falling to default) are preserved.

### 5. tokenstat syncer — no change

The SQL condition `e.engine = ''` is intentionally kept. Historical execution records written before engine tracking was added may still have empty engine values. Removing this condition is a separate data-migration concern.

## Files Changed

| File | Change |
|------|--------|
| `internal/domain/worker/manager.go` | `resolveEngine` returns `(string, ai.EngineAdapter)`; remove error path; update both call sites |
| `internal/domain/worker/manager_test.go` | Delete `TestExecuteWorker_resolveEngineFails`; update `resolveEngine` call sites in other tests if any |

## Non-Goals

- No change to startup validation logic — existing checks are sufficient.
- No change to the tokenstat syncer SQL.
- No change to how `engineCfg` is stored or updated.
