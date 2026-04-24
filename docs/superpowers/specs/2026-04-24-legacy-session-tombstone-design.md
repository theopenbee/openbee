# Legacy Session Tombstone Design

**Date:** 2026-04-24
**Status:** Approved

## Problem

164 legacy `bee_executions` rows (where `engine = ""`) are collected on every sync cycle but never produce a `bee_token_stats` record:

1. `collectSessions` returns them because `COALESCE(MAX(ts.synced_at), 0)` is 0 (no record exists).
2. `syncSession` tries all four parsers; all return `ErrSessionDataNotFound`.
3. Because `engine == ""`, the function returns `nil` without writing anything.
4. Next cycle: same 164 sessions are collected again — infinite loop.

Additionally, sessions with a known engine (`engine != ""`) where all parsers return `ErrSessionDataNotFound` currently return an error and are retried indefinitely, which is also wasteful.

## Goal

For any session where all parsers return `ErrSessionDataNotFound` and there are no real parser errors:
- Write a tombstone record (`model="unknown"`, all token counts `0`) to `bee_token_stats`.
- This updates `synced_at`, so the session is excluded from future sync rounds.
- The tombstone is visible in the session detail UI (`model=unknown | 0 tokens`).

## Non-Goals

- No DB schema changes.
- No changes to `TokenStatsStore`, `storeUsages`, or the UI layer.
- No filtering of `unknown` records from API responses (user wants them visible).

## Design

### `internal/tokenstat/syncer.go` — `syncSession`

Remove the `engine == ""` early-return branch. Replace it and the `fmt.Errorf("no token session data found…")` fallthrough with a unified tombstone write.

**Before (lines 145–151):**
```go
if firstErr != nil {
    return firstErr
}
if engine == "" { // legacy execution without engine hint; missing data is expected
    return nil
}
return fmt.Errorf("no token session data found for %s", sessionID)
```

**After:**
```go
if firstErr != nil {
    return firstErr
}
if engine == "" {
    logger.Debug("tokenstat: legacy session has no data, writing tombstone",
        zap.String("session_id", sessionID))
} else {
    logger.Warn("tokenstat: no token data found for session, writing tombstone",
        zap.String("session_id", sessionID),
        zap.String("engine", engine))
}
return s.storeUsages([]SessionTokenUsage{{SessionID: sessionID, Model: "unknown"}})
```

### Tombstone Record

| Field | Value |
|---|---|
| `session_id` | actual session ID |
| `agent_type` | `""` |
| `model` | `"unknown"` |
| `input_tokens` | `0` |
| `output_tokens` | `0` |
| `cache_creation_tokens` | `0` |
| `cache_read_tokens` | `0` |
| `total_tokens` | `0` |
| `synced_at` | current Unix milliseconds |

### Why This Fixes the Loop

After the tombstone is written, `collectSessions`'s HAVING clause:
```sql
HAVING MAX(e.completed_at) > COALESCE(MAX(ts.synced_at), 0)
```
resolves to `MAX(e.completed_at) > <now>`, which is false. The session is excluded from all future sync rounds unless a new execution with `completed_at > synced_at` is inserted.

## Tests

Two new test cases in `internal/tokenstat/syncer_test.go`:

### `TestSyncer_SyncOnce_LegacyExecutionNoDataWritesTombstone`
- Insert an execution with `engine=""`, provide no session files.
- Assert: `bee_token_stats` contains exactly one record with `model="unknown"` and `total_tokens=0`.
- Assert: a second `SyncOnce` does not produce a second record (idempotent via upsert, session not re-collected).

### `TestSyncer_SyncOnce_KnownEngineNoDataWritesTombstone`
- Insert an execution with `engine="claude"`, provide no session files.
- Assert: `bee_token_stats` contains exactly one record with `model="unknown"` and `total_tokens=0`.

## Impact Summary

| Area | Change |
|---|---|
| `syncer.go` | Remove `engine==""` branch, unified tombstone write |
| `syncer_test.go` | Two new test cases |
| DB schema | None |
| Store layer | None |
| UI / API | None (tombstone visible in session detail by design) |
