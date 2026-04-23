# Design: Usage Sync Debug Logging & engine Field

**Date:** 2026-04-23  
**Branch:** feat/token-usage-statistics

## Overview

Two improvements to the token usage tracking feature:

1. Add debug-level logging to the usage sync process so execution flow and results are visible.
2. Add an `engine` column to `bee_usage_records` to record which AI engine (claude / pi / codex / kimi) produced the usage.

## Requirement 1: Usage Sync Debug Logging

### Current State

`syncBatch()` in `internal/domain/usage/syncer.go` only emits error-level logs. Normal operation is completely silent.

### Design

Add three debug log points to `syncBatch()`:

| Point | Message | Fields |
|-------|---------|--------|
| After `ListUnsynced` returns | `"sync batch start"` | `unsynced_count` |
| After each `ParseUsage` call | `"parsed usage"` | `execution_id`, `engine`, `total_tokens`, `cost_usd` |
| After `InsertBatch` succeeds | `"sync batch done"` | `count`, `duration_ms` |

Log level: **Debug** throughout. Zero-value results (parse failures that fall back to empty `UsageData`) are still logged so parse failures are visible at debug level.

### Files Changed

- `internal/domain/usage/syncer.go` — add debug log calls

---

## Requirement 2: engine Field in bee_usage_records

### Current State

`bee_usage_records` has no engine information. The parser internally detects the engine (claude / pi / codex) but does not expose it.

### Design

Expose the detected engine through the data pipeline and persist it.

**Engine values:**

| Engine | String value |
|--------|-------------|
| Claude Code | `"claude"` |
| Pi | `"pi"` |
| Codex | `"codex"` |
| Kimi | `"kimi"` |
| Unknown | `""` (empty string) |

### Files Changed

**① `internal/ai/usage/parser.go`**
- Add `Engine string` field to `UsageData`
- In `ParseUsage`, set `data.Engine` in each engine branch: `"claude"`, `"pi"`, `"codex"`

**② `internal/infra/model/usage.go`**
- Add `Engine string \`json:"engine" db:"engine"\`` to `UsageRecord`

**③ `internal/infra/store/db.go`**
- Add `engine TEXT NOT NULL DEFAULT ''` directly to the `CREATE TABLE IF NOT EXISTS bee_usage_records` statement (no ALTER TABLE migration needed — feature not yet released)

**④ `internal/infra/store/usage_store.go`**
- Add `engine` to `usageSelect` constant
- Add `engine` to `scanUsageRecord` scan call
- Add `engine` to `Insert` and `InsertBatch` SQL and parameter lists

**⑤ `internal/domain/usage/syncer.go`**
- Set `Engine: data.Engine` when constructing `UsageRecord`
- (Also adds debug logs per Requirement 1)

### Data Flow

```
ParseUsage() detects engine
  → sets UsageData.Engine
    → syncer builds UsageRecord with Engine field
      → InsertBatch persists to bee_usage_records.engine
```

## Out of Scope

- No API changes to expose engine in query responses (separate concern)
- No backfill for existing records (field defaults to `""`)
- Kimi parser implementation (engine value reserved but parser not added here)
