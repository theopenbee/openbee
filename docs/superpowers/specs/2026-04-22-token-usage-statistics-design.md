# Token Usage Statistics — Design Spec

**Date**: 2026-04-22  
**Status**: Approved  
**Scope**: Data collection and storage only (no UI, no quota enforcement)

---

## Overview

Introduce token usage statistics to openbee by parsing existing execution log files and storing structured usage records in a new database table. A background sync job handles both new and historical executions.

---

## Goals

- Record `model`, `input_tokens`, `output_tokens`, `cache_creation_tokens`, `cache_read_tokens`, `total_tokens`, `cost_usd` for every completed execution
- Cover historical executions (already have log files) without any manual migration step
- Zero changes to the execution hot path (worker/manager.go, AI invokers)

**Out of scope (deferred)**: UI display, quota enforcement, API endpoints exposure.

---

## Data Model

### New table: `bee_usage_records`

```sql
CREATE TABLE bee_usage_records (
  id                    TEXT PRIMARY KEY,
  execution_id          TEXT NOT NULL UNIQUE,
  model                 TEXT NOT NULL DEFAULT '',
  input_tokens          INTEGER NOT NULL DEFAULT 0,
  output_tokens         INTEGER NOT NULL DEFAULT 0,
  cache_creation_tokens INTEGER NOT NULL DEFAULT 0,
  cache_read_tokens     INTEGER NOT NULL DEFAULT 0,
  total_tokens          INTEGER NOT NULL DEFAULT 0,
  cost_usd              REAL NOT NULL DEFAULT 0,
  synced_at             INTEGER NOT NULL
);

CREATE INDEX idx_usage_execution_id ON bee_usage_records(execution_id);
CREATE INDEX idx_usage_synced_at ON bee_usage_records(synced_at);
```

**Notes**:
- `execution_id UNIQUE` guarantees idempotency — the sync job can run multiple times safely
- `total_tokens` is stored redundantly (`input + output + cache_creation + cache_read`) to avoid per-query computation
- `bee_executions` table is **not modified**

---

## Background Sync Job

### Trigger

A background goroutine starts with the server and ticks every **60 seconds**.

### Query — find unsynced completed executions

```sql
SELECT e.id, e.log_path, e.engine
FROM bee_executions e
LEFT JOIN bee_usage_records u ON e.id = u.execution_id
WHERE e.status IN ('completed', 'failed')
  AND e.log_path != ''
  AND u.id IS NULL
LIMIT 50
```

`LIMIT 50` prevents disk I/O spikes. If a batch returns exactly 50 rows the job immediately re-queries to drain the backlog.

### Processing loop

```
tick (60s)
  → query unsynced executions (LIMIT 50)
  → for each execution:
      1. read log file at log_path
      2. select parser by engine type
      3. parse UsageData
      4. INSERT INTO bee_usage_records
  → if batch size == 50: re-query immediately
```

### Error handling

| Situation | Behavior |
|-----------|----------|
| Log file not found | Skip; will retry next tick |
| Parse failure (unexpected format) | Insert zero-value record (model="", tokens=0, cost=0) to prevent infinite retry |
| DB insert conflict (duplicate) | Ignore (`INSERT OR IGNORE`) |

---

## Log Parsers

Entry point: `ParseUsageFromLog(logPath string, engine string) (*UsageData, error)`

### Claude

Scan file lines in **reverse** (last line first) for `{"type":"result"}`. Extract:
- `usage.input_tokens`, `usage.output_tokens`, `usage.cache_creation_input_tokens`, `usage.cache_read_input_tokens`
- `total_cost_usd` (pre-calculated by Claude CLI — no external pricing API needed)
- `message.model` from the last `{"type":"assistant"}` event for model name

### Codex

Scan for `{"type":"item.completed"}` events with token data; accumulate deltas.

### Pi

Scan for `{"type":"agent_end"}` event; extract usage fields from message payload, mapping `cacheWrite` → `cache_creation_tokens`, `cacheRead` → `cache_read_tokens`.

### Unknown engine

Return a zero-value `UsageData` with `model="unknown"` and log a warning.

---

## Code Structure

```
internal/
├── infra/
│   ├── model/
│   │   └── usage.go                  # UsageRecord struct + DailyUsage + UsageSummary
│   └── store/
│       ├── usage_store.go            # Insert / GetByExecutionID / SumByWorker / SumByDay / SumTotal
│       └── db.go                     # +1 migration (CREATE TABLE bee_usage_records)
├── ai/
│   └── usage/
│       ├── parser.go                 # ParseUsageFromLog dispatcher + UsageData struct
│       ├── claude_parser.go          # Claude stream-json result event parser
│       ├── codex_parser.go           # Codex JSON event parser
│       └── pi_parser.go              # Pi agent_end event parser
└── domain/
    └── usage/
        └── syncer.go                 # UsageSyncer: Start(ctx) / Stop() / sync loop
```

**Server integration**: Register `syncer.Run(ctx)` as a background goroutine in `internal/app/app.go`, following the same pattern as `task.Scheduler`.

---

## Key Interfaces

```go
// internal/ai/usage/parser.go
type UsageData struct {
    Model                string
    InputTokens          int64
    OutputTokens         int64
    CacheCreationTokens  int64
    CacheReadTokens      int64
    TotalTokens          int64
    CostUSD              float64
}

func ParseUsageFromLog(logPath string, engine string) (*UsageData, error)

// internal/domain/usage/syncer.go
type UsageSyncer struct{ ... }

func NewUsageSyncer(executionStore ExecutionStore, usageStore UsageStore) *UsageSyncer
func (s *UsageSyncer) Run(ctx context.Context)  // blocks; cancel ctx to stop

// internal/infra/store/usage_store.go
func (s *UsageStore) Insert(record *model.UsageRecord) error
func (s *UsageStore) GetByExecutionID(executionID string) (*model.UsageRecord, error)
func (s *UsageStore) SumByWorker(workerID string, from, to time.Time) (*model.UsageSummary, error)
func (s *UsageStore) SumByDay(from, to time.Time) ([]model.DailyUsage, error)
func (s *UsageStore) SumTotal(from, to time.Time) (*model.UsageSummary, error)
```

---

## Files Changed

| File | Change |
|------|--------|
| `internal/infra/model/usage.go` | New |
| `internal/infra/store/usage_store.go` | New |
| `internal/infra/store/db.go` | +1 migration |
| `internal/ai/usage/parser.go` | New |
| `internal/ai/usage/claude_parser.go` | New |
| `internal/ai/usage/codex_parser.go` | New |
| `internal/ai/usage/pi_parser.go` | New |
| `internal/domain/usage/syncer.go` | New |
| `internal/app/app.go` | Register syncer as background goroutine |

**Not modified**: `bee_executions` table, `worker/manager.go`, any AI invoker.

---

## Estimation

~2–3 days. Majority of effort is the three engine parsers and testing against real log files.
