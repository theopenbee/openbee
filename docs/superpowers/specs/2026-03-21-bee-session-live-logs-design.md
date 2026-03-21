# Design: Bee Session Real-Time Log Viewing

**Date:** 2026-03-21
**Status:** Approved

## Problem Statement

Non-worker sessions (bee's own executions — those where `worker_id IS NULL`) cannot have their logs viewed on the session detail page (`/sessions/:sessionId`). Three distinct bugs contribute to this:

1. **No live logs for bee executions**: While a bee execution is running, `GET /api/executions/:id/logs` returns empty content because bee executions are never registered in the `worker.Manager`'s `activeLogs` map. Only worker executions register there.

2. **Cache-Control caches empty log responses**: When the log API falls through to `ReadLog` for a running bee execution, `log_path` is not yet set, so `ReadLog` returns an empty string. The handler then adds `Cache-Control: public, max-age=3600` to this empty response, causing browsers to cache it for up to one hour. Even after the execution completes and logs are written to disk, the browser serves the cached empty response.

3. **Session detail page stuck in "Loading…"**: `session-detail.tsx` has `if (executions.length === 0 && !error) return <p>Loading...</p>`, which displays a permanent loading state instead of a proper empty state when no executions exist for a session.

## Design: Shared ActiveLogRegistry (Approach B)

### Architecture Overview

**Before (broken)**
```
Feeder ─────────────────────────────────────────────────── (no live logs)
Manager ── activeLogs[execID] ── GetActiveLog(id) ──→ API Server
```

**After (target)**
```
                    ActiveLogRegistry
                    ┌──────────────────────────────────┐
Manager ────────────┤  Register / Unregister / Get     ├──→ API Server
Feeder  ────────────┤  (shared, thread-safe)           │
                    └──────────────────────────────────┘
```

A new `ActiveLogRegistry` is the single source of truth for all in-flight execution logs. Both `Manager` (worker executions) and `Feeder` (bee executions) register executions with it. The API Server reads from it directly.

---

## Component Changes

### 1. `internal/worker/log_registry.go` (new file)

Extract the `executionLog` type and `writeLine`/`string` methods from `manager.go` into this new file. Add the registry wrapper around it.

```go
// executionLog is a thread-safe in-memory log buffer for a single in-flight execution.
type executionLog struct {
    mu  sync.Mutex
    buf strings.Builder
}

func (l *executionLog) writeLine(s string) { ... }
func (l *executionLog) string() string     { ... }

// ActiveLogRegistry manages live log buffers for all running executions.
// It is safe for concurrent use by multiple goroutines.
type ActiveLogRegistry struct {
    mu   sync.RWMutex
    logs map[string]*executionLog
}

func NewActiveLogRegistry() *ActiveLogRegistry

// Register creates a new log buffer for the given execution ID and returns
// a function that callers use to append lines. Panics if id already registered.
func (r *ActiveLogRegistry) Register(id string) func(line string)

// Get returns the current accumulated content for id.
// Returns ("", false) if no active buffer exists for that id.
func (r *ActiveLogRegistry) Get(id string) (string, bool)

// Unregister removes the log buffer for id. Safe to call even if id is absent.
func (r *ActiveLogRegistry) Unregister(id string)
```

**Key invariant:** Callers must call `Unregister` only *after* the final log has been persisted to disk. This prevents a race where the API falls through to `ReadLog` before the file is written.

---

### 2. `internal/worker/manager.go` (modified)

- Remove `executionLog` type definition (moved to `log_registry.go`).
- Replace `activeLogs map[string]*executionLog` field with `logRegistry *ActiveLogRegistry`.
- Remove `GetActiveLog(id string)` public method (API Server will use the registry directly).
- Update `NewManager` to accept `*ActiveLogRegistry` as a parameter.
- Update `launchRuntime`: replace direct `activeLogs` map writes with `m.logRegistry.Register(exec.ID)`.
- Update `monitorExecution`: use the returned `writeLine` func for output; call `m.logRegistry.Unregister(exec.ID)` at the end of the goroutine (after `WriteLog` and `UpdateResult`).

Execution lifecycle in `monitorExecution` (updated order):
```
OutputDone/OutputError received:
  1. WriteLog(exec.ID, ...)          ← persist to disk first
  2. UpdateResult(exec.ID, ...)      ← update status
  3. logRegistry.Unregister(exec.ID) ← remove from live registry last
```

---

### 3. `internal/bee/feeder.go` (modified)

Add an optional `logRegistry *worker.ActiveLogRegistry` field, injected via a new `Option`:

```go
// WithLogRegistry injects a shared log registry so bee executions provide live logs.
func WithLogRegistry(r *worker.ActiveLogRegistry) Option {
    return func(f *Feeder) { f.logRegistry = r }
}
```

Update `processBeeGroup` to register/unregister with the registry when available:

```go
// After CreateBeeExecution succeeds:
var writeLine func(string)
if f.logRegistry != nil && execErr == nil {
    writeLine = f.logRegistry.Register(exec.ID)
}

// After WriteLog + UpdateResult:
if f.logRegistry != nil && execErr == nil {
    f.logRegistry.Unregister(exec.ID)
}
```

Update `drainBeeOutput` signature to accept a `writeLine` callback:

```go
func (f *Feeder) drainBeeOutput(ch <-chan claude.Output, writeLine func(string)) (string, error)
```

Inside the loop, each line is written to both the `strings.Builder` (for final disk write) and the live registry (for real-time API reads):

```go
case claude.OutputStdout, claude.OutputStderr:
    line := out.Content + "\n"
    sb.WriteString(line)
    if writeLine != nil {
        writeLine(out.Content)  // same signature as executionLog.writeLine
    }
```

When no registry is configured (`writeLine == nil`), behavior is identical to today.

**Order of operations in `processBeeGroup` (complete):**
```
1. CreateBeeExecution(sessionID, prompt)
2. logRegistry.Register(exec.ID)         ← start live log
3. UpdatePID(exec.ID, proc.PID())
4. drainBeeOutput(outputCh, writeLine)   ← stream output to registry + buffer
5. WriteLog(exec.ID, ...)                ← persist to disk
6. UpdateResult(exec.ID, ...)            ← mark complete/failed
7. logRegistry.Unregister(exec.ID)       ← stop live log AFTER disk write
8. UpsertSessionContext(...)
9. MarkBeeProcessed(...)
```

---

### 4. `internal/api/execution_handler.go` (modified)

`getExecutionLogs` is updated to use the registry instead of `manager.GetActiveLog`:

```go
func (s *Server) getExecutionLogs(c *gin.Context) {
    id := c.Param("id")

    // Live log path: execution is still running (worker or bee).
    if content, ok := s.logRegistry.Get(id); ok {
        c.String(http.StatusOK, content)
        return
    }

    // Completed path: read from persisted log file.
    content, err := s.executionStore.ReadLog(id)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    // Cache-Control fix: only cache when log content actually exists.
    // Previously this header was set unconditionally, causing browsers to cache
    // empty responses for running bee executions, hiding logs even after completion.
    if content != "" {
        c.Header("Cache-Control", "public, max-age=3600")
    }
    c.String(http.StatusOK, content)
}
```

`Server` struct and `NewServer` gain a `logRegistry *worker.ActiveLogRegistry` parameter. `manager.GetActiveLog` call is removed.

---

### 5. `internal/api/router.go` (modified)

```go
type Server struct {
    // existing fields ...
    logRegistry *worker.ActiveLogRegistry  // ← new
}

func NewServer(
    ws *store.WorkerStore,
    es *store.ExecutionStore,
    mgr *worker.Manager,
    logRegistry *worker.ActiveLogRegistry, // ← new
    // ... rest unchanged
) *Server
```

---

### 6. `web/src/pages/session-detail.tsx` (modified)

Fix the permanent loading state bug. The `useSessionExecutions` hook exposes `isLoading`; use it to distinguish loading from empty:

```tsx
// Before (buggy):
if (executions.length === 0 && !error) return <p>Loading...</p>

// After:
const { data: executions = [], error, isLoading } = useSessionExecutions(sessionId!)
if (isLoading) return <p>Loading...</p>
if (!error && executions.length === 0) return <p>{t("sessionDetail.noExecutions")}</p>
```

Add `sessionDetail.noExecutions` to i18n translation files.

---

### 7. `main.go` (modified)

```go
// Create shared registry — single instance for the process lifetime.
logRegistry := worker.NewActiveLogRegistry()

mgr := worker.NewManager(workerBaseDir, beeCfg, workerStore, execStore, logRegistry)

feeder := bee.NewFeeder(
    msgStore, taskStore, sessionStore, execStore, beeProcess, beeWorkDir, beeCfg,
    bee.WithLogRegistry(logRegistry),
    // ... other options
)

srv := api.NewServer(workerStore, execStore, mgr, logRegistry, mcpServer, mcpAPIKey, staticFS, localChat)
```

---

## Bug Fix Summary

| Bug | Root Cause | Fix |
|-----|-----------|-----|
| Bee live logs not visible | Feeder never registers executions in Manager's `activeLogs` | Feeder registers with shared `ActiveLogRegistry` |
| Cache-Control caches empty logs | `Cache-Control: max-age=3600` added unconditionally even for empty responses | Only add header when `content != ""` |
| Session detail stuck at "Loading…" | `length === 0 && !error` conflates loading with empty state | Use `isLoading` flag from React Query |

---

## Non-Goals

- **Worker log streaming to frontend (SSE/WebSocket)**: The existing poll-based approach is retained. Real-time delivery improvement is out of scope.
- **Log rotation or size limits**: Not addressed in this change.
- **Bee execution stop/cancel via UI**: Out of scope.

---

## Files Changed

| File | Change |
|------|--------|
| `internal/worker/log_registry.go` | **New** — `executionLog` type + `ActiveLogRegistry` |
| `internal/worker/manager.go` | Use registry; remove `activeLogs` map and `GetActiveLog` |
| `internal/bee/feeder.go` | `WithLogRegistry` option; update `drainBeeOutput` |
| `internal/api/router.go` | Add `logRegistry` to `Server` struct and `NewServer` |
| `internal/api/execution_handler.go` | Use `logRegistry.Get`; fix Cache-Control |
| `main.go` | Wire registry into Manager, Feeder, and Server |
| `web/src/pages/session-detail.tsx` | Fix loading/empty state |
| `web/src/locales/*.json` | Add `sessionDetail.noExecutions` key |
