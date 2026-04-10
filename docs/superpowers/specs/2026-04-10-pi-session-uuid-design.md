# PI Session UUID Design

**Date**: 2026-04-10
**Branch**: feat/engine-plugin
**Status**: Approved

## Background

The PI agent's session management has reliability and coupling issues in the current implementation. The flow involves:

1. openbee generates a UUID placeholder session_id
2. PI CLI runs and internally creates a session file with a random hex name
3. PI CLI emits the real session file path via `OutputSessionID`
4. openbee overwrites the UUID with the file path
5. The file path is persisted to `bee_session_contexts`

This "generate-then-overwrite" pattern creates two problems:
- **Split identity**: openbee uses UUIDs for orchestration but stores file paths — two different ID systems for one concept
- **Tight coupling**: PI's internal file path leaks into openbee's storage layer; upper layers must parse and relay PI's file path

## New Design

**Core principle**: openbee owns the session_id (UUID format) throughout its lifecycle. The PI adapter internally maps that UUID to a session file path, with no path ever surfacing to openbee.

## Architecture

### PI Invoker (`internal/ai/pi/invoker.go`)

**Remove**: `newSessionPath()` — generates random hex-named session files

**Add**: `sessionPath(sessionID string) (string, error)` — constructs a deterministic path:

```
~/.openbee/.pi/sessions/{sessionID}.jsonl
```

The UUID passed in `RunOptions.SessionID` becomes the session file name directly. No mapping table needed.

**Remove**: emission of `OutputSessionID` output event. Since the path is deterministic from the UUID, there is nothing to report back.

PI CLI invocation remains unchanged: `pi --mode json --session <path> -p <prompt>`. PI CLI handles resume vs. new session based on whether the file exists.

### Manager (`internal/domain/worker/manager.go`)

**Remove**: `OutputSessionID` case in the output monitoring loop and the `UpdateSessionID` call it triggers.

The execution record's `session_id` is set at creation time with the UUID and never needs overwriting.

### Feeder (`internal/domain/bee/feeder.go`)

**Remove**: `waitBeeOutput` logic that extracts `engineSessionID` and overwrites `sessionID`.

**Replace with**: a simple drain of the output channel. The `sessionID` variable holds the correct UUID from the start and is persisted directly.

### Engine Interface (`internal/ai/engine.go`)

**Keep**: `OutputSessionID` constant — the Codex adapter (`internal/ai/codex/invoker.go`) still emits it. Only PI stops emitting it.

### Dispatcher (`internal/domain/task/dispatcher.go`)

**No changes**. The resume-failure fallback (clear session + retry fresh) is preserved as a defensive layer and remains correct under the new scheme.

## Data Flow (New)

```
openbee generates UUID
    │
    ▼
RunOptions{SessionID: uuid, Resume: bool}
    │
    ▼
pi/invoker constructs ~/.openbee/.pi/sessions/{uuid}.jsonl
    │
    ▼
PI CLI runs (file exists → resume, file absent → new session)
    │
    ▼
openbee persists UUID to bee_session_contexts
    │
    ▼
Next invocation: DB returns UUID, Resume=true, same path resolved
```

UUID is stable across the entire lifecycle — no placeholder, no overwrite.

## Migration

No data migration. Existing `bee_session_contexts` rows store PI file paths (not UUIDs). On the next resume attempt, the PI adapter will construct a non-existent path from the stored value, and PI CLI will create a fresh session file. This is equivalent to a session reset — acceptable.

## Files Changed

| File | Change |
|------|--------|
| `internal/ai/pi/invoker.go` | Remove `newSessionPath()`; add `sessionPath(uuid)`; stop emitting `OutputSessionID` |
| `internal/domain/worker/manager.go` | Remove `OutputSessionID` monitoring and `UpdateSessionID` call |
| `internal/domain/bee/feeder.go` | Remove `engineSessionID` overwrite logic; simple output drain |
| `internal/ai/pi/invoker_test.go` | Update tests to reflect new session path behavior |
| `internal/ai/engine.go` | No change (constant kept for Codex) |
| `internal/domain/task/dispatcher.go` | No change |
