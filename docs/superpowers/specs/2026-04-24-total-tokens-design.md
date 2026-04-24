# Design: Add `total_tokens` to `bee_token_stats`

**Date:** 2026-04-24
**Branch:** feat/token-usage-stats
**Status:** Approved

## Summary

Add a `total_tokens` stored column to the `bee_token_stats` table. The value is the sum of all four existing token fields: `input_tokens + output_tokens + cache_creation_tokens + cache_read_tokens`. This is computed once at write time and persisted as a plain column.

## Definition

```
total_tokens = input_tokens + output_tokens + cache_creation_tokens + cache_read_tokens
```

## Design Decisions

- **Stored column (not virtual/computed):** Persisted for fast reads and future indexing.
- **No backfill required:** Feature has not launched; no existing production data.
- **Migration strategy:** Modify the existing `CREATE TABLE` statement directly (migration 41 in `db.go`), not an `ALTER TABLE`.
- **Computation layer:** Calculated in `syncer.go` (not in parsers), keeping parsers unchanged.

## Files Changed

### 1. `internal/infra/store/db.go`

Add `total_tokens INTEGER NOT NULL DEFAULT 0` to the `CREATE TABLE IF NOT EXISTS bee_token_stats` statement in migration 41.

### 2. `internal/infra/model/token_stats.go`

Add field to `TokenStats` struct:

```go
TotalTokens int64 `json:"total_tokens" db:"total_tokens"`
```

### 3. `internal/infra/store/token_stats_store.go`

- Add `total_tokens` to the INSERT column list and values
- Add `total_tokens = excluded.total_tokens` to the `ON CONFLICT DO UPDATE` clause

### 4. `internal/tokenstat/syncer.go`

In `storeUsages`, compute and assign `TotalTokens` when constructing `model.TokenStats`:

```go
TotalTokens: usage.InputTokens + usage.OutputTokens + usage.CacheCreationTokens + usage.CacheReadTokens,
```

## Out of Scope

- Parser changes (claude, codex, pi, kimi): no changes needed
- `SessionTokenUsage` interface: no new fields needed
- Indexing on `total_tokens`: not required at this time
