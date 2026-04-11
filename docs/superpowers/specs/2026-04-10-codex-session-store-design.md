# Codex Session Store Design

**Date:** 2026-04-10  
**Branch:** feat/engine-plugin  
**Status:** Approved

## Background

The current engine plugin system supports three engines: claude, codex, and pi. Claude and pi both accept a pre-allocated openbee UUID as their session ID — the caller generates a UUID before the run and that UUID remains stable throughout the session's lifetime.

Codex is different: it generates its own internal `thread_id` upon first run (emitted via a `thread.started` JSON event). Openbee currently extracts this `thread_id` from the output stream and emits it as an `OutputSessionID` event back to the caller (feeder/manager), which then updates the database. This creates coupling: feeder and manager must contain codex-specific logic to handle `OutputSessionID`.

## Goal

Decouple codex session management from the rest of the system. After this change, codex's external behavior (from feeder/manager's perspective) must be identical to claude and pi: the caller passes a UUID, and the engine handles resume internally. No `OutputSessionID` events are emitted.

## Design

### Storage

Codex maintains a per-session mapping directory:

```
~/.openbee/.codex/sessions/
├── <openbee-uuid-1>    # file content: codex-thread-id-1
├── <openbee-uuid-2>    # file content: codex-thread-id-2
└── ...
```

Each file is named by the openbee UUID and contains the corresponding codex `thread_id` as plain text. This avoids concurrent write conflicts between sessions — different sessions write different files.

Writes use an atomic pattern (write temp file + `os.Rename`) to protect against partial writes on crash.

### New Component: SessionStore

New file: `internal/ai/codex/session_store.go`

```go
type SessionStore struct {
    dir string  // ~/.openbee/.codex/sessions/
}

func NewSessionStore() (*SessionStore, error)
func (s *SessionStore) Get(openbeeUUID string) (threadID string, ok bool)
func (s *SessionStore) Set(openbeeUUID, threadID string) error
```

- `NewSessionStore`: resolves `~/.openbee/.codex/sessions/`, creates the directory if it does not exist.
- `Get`: reads `dir/<openbeeUUID>`, returns `("", false)` if the file does not exist.
- `Set`: writes `threadID` to a temp file in the same directory, then atomically renames it to `dir/<openbeeUUID>`.

### Invoker Behavior Changes

| Scenario | Before | After |
|---|---|---|
| New session (`Resume=false`) | Run codex → parse `thread.started` → emit `OutputSessionID` | Run codex → parse `thread.started` → `store.Set(uuid, threadID)` |
| Resume (`Resume=true`) | Use `sessionID` directly as codex thread_id for `codex exec resume` | `store.Get(uuid)` → use `threadID` for `codex exec resume <threadID>` |
| Resume but mapping missing | N/A | Fall back to new session, log warning |

The `SessionStore` is created once in `NewAdapter` and shared across all invocations.

### Cleanup: Remove OutputSessionID

The following changes remove all `OutputSessionID` infrastructure that is no longer needed:

| File | Change |
|---|---|
| `internal/ai/codex/invoker.go` | Remove the `ch <- ai.Output{Type: ai.OutputSessionID, ...}` emit |
| `internal/domain/bee/feeder.go:301` | Remove the `case ai.OutputSessionID` handler |
| `internal/domain/worker/manager.go:162` | Remove the `case ai.OutputSessionID` handler |
| `internal/ai/engine.go:39` | Remove the `OutputSessionID OutputType = "session_id"` constant |
| `internal/ai/pi/invoker_test.go` | Remove the test assertions guarding against `OutputSessionID` |

### Error Handling

- **sessions dir creation fails**: `NewSessionStore` returns error; adapter initialization fails with a clear message.
- **`Get` file not found**: returns `("", false)` — caller falls back to new session.
- **`Get` read error** (e.g., permissions): returns `("", false)` with a logged warning; run continues as new session.
- **`Set` write error**: log error, do not fail the run. The session ran successfully; it simply won't be resumable next time.
- **Resume but `Get` returns false**: fall back to new session, log a warning.

## Files Changed

| File | Action |
|---|---|
| `internal/ai/codex/session_store.go` | New |
| `internal/ai/codex/session_store_test.go` | New |
| `internal/ai/codex/adapter.go` | Initialize `SessionStore`, pass to invoker |
| `internal/ai/codex/invoker.go` | Use `SessionStore` for get/set; remove `OutputSessionID` emit |
| `internal/ai/codex/invoker_test.go` | Update tests |
| `internal/ai/engine.go` | Remove `OutputSessionID` constant |
| `internal/domain/bee/feeder.go` | Remove `OutputSessionID` handler |
| `internal/domain/worker/manager.go` | Remove `OutputSessionID` handler |
| `internal/ai/pi/invoker_test.go` | Remove `OutputSessionID` guard assertions |
