# Usage Sync Logging & engine Field Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add debug-level logging to the usage sync process and persist the AI engine name in `bee_usage_records`.

**Architecture:** Expose the engine detected in `ParseUsage` via `UsageData.Engine`, thread it through the model → store → syncer pipeline, update the DB schema in the CREATE TABLE statement (not via ALTER), and add structured debug logs at three points in `syncBatch`.

**Tech Stack:** Go, SQLite (modernc), go.uber.org/zap

**Worktree:** `/Users/tengyongzhi/work/bot-workspaces/openbee/.worktrees/feat-token-usage`

---

## File Map

| Action | File | Change |
|--------|------|--------|
| Modify | `internal/ai/usage/parser.go` | Add `Engine string` to `UsageData`; set it in `ParseUsage` |
| Modify | `internal/ai/usage/parser_test.go` | Assert `data.Engine` in existing tests |
| Modify | `internal/infra/model/usage.go` | Add `Engine string` field to `UsageRecord` |
| Modify | `internal/infra/store/db.go` | Add `engine` column to `CREATE TABLE bee_usage_records` |
| Modify | `internal/infra/store/usage_store.go` | Add `engine` to all SQL (select, scan, insert) |
| Modify | `internal/domain/usage/syncer.go` | Set `Engine: data.Engine`; add debug logs |

---

## Task 1: Expose Engine in UsageData

**Files:**
- Modify: `internal/ai/usage/parser.go`
- Modify: `internal/ai/usage/parser_test.go`

- [ ] **Step 1: Add failing assertions to existing parser tests**

In `internal/ai/usage/parser_test.go`, add `assert.Equal(t, "claude", data.Engine)` to `TestParseUsage_Claude_Success`, `assert.Equal(t, "codex", data.Engine)` to `TestParseUsage_Codex_WithSessionFile`, and `assert.Equal(t, "pi", data.Engine)` to `TestParseUsage_Pi_ReadsSessionFile`.

Updated `TestParseUsage_Claude_Success` (add one line after the existing assertions):
```go
assert.Equal(t, "claude", data.Engine)
```

Updated `TestParseUsage_Codex_WithSessionFile` (add one line after the existing assertions):
```go
assert.Equal(t, "codex", data.Engine)
```

Updated `TestParseUsage_Pi_ReadsSessionFile` (add one line after the existing assertions):
```go
assert.Equal(t, "pi", data.Engine)
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee/.worktrees/feat-token-usage
go test ./internal/ai/usage/... -v -run "TestParseUsage_Claude_Success|TestParseUsage_Codex_WithSessionFile|TestParseUsage_Pi_ReadsSessionFile"
```

Expected: FAIL — `data.Engine` field does not exist yet.

- [ ] **Step 3: Add Engine field to UsageData and set it in ParseUsage**

In `internal/ai/usage/parser.go`, update `UsageData` struct:
```go
type UsageData struct {
	Engine              string
	Model               string
	InputTokens         int64
	OutputTokens        int64
	CacheCreationTokens int64
	CacheReadTokens     int64
	TotalTokens         int64
	CostUSD             float64
}
```

Update `ParseUsage` to set `Engine` on the returned data. Replace the `switch eng` block:
```go
switch eng {
case engineClaude:
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return &UsageData{}, nil
	}
	data, err := parseClaudeUsageFromReader(f)
	if err != nil {
		return data, err
	}
	data.Engine = "claude"
	return data, nil
case enginePi:
	sessionFile := filepath.Join(ctx.PiSessionsDir, ctx.SessionID+".jsonl")
	data, err := parsePiUsage(sessionFile, ctx.StartedAt, ctx.CompletedAt)
	if err != nil {
		return data, err
	}
	data.Engine = "pi"
	return data, nil
case engineCodex:
	data, err := parseCodexUsage(ctx.CodexStoreDir, ctx.CodexSessionsDir, ctx.SessionID, ctx.StartedAt, ctx.CompletedAt)
	if err != nil {
		return data, err
	}
	data.Engine = "codex"
	return data, nil
default:
	return &UsageData{}, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee/.worktrees/feat-token-usage
go test ./internal/ai/usage/... -v
```

Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee/.worktrees/feat-token-usage
git add internal/ai/usage/parser.go internal/ai/usage/parser_test.go
git commit -m "feat(usage): expose Engine field in UsageData"
```

---

## Task 2: Add engine to UsageRecord model

**Files:**
- Modify: `internal/infra/model/usage.go`

- [ ] **Step 1: Add Engine field**

In `internal/infra/model/usage.go`, update `UsageRecord`:
```go
type UsageRecord struct {
	ID                  string  `json:"id" db:"id"`
	ExecutionID         string  `json:"execution_id" db:"execution_id"`
	Engine              string  `json:"engine" db:"engine"`
	Model               string  `json:"model" db:"model"`
	InputTokens         int64   `json:"input_tokens" db:"input_tokens"`
	OutputTokens        int64   `json:"output_tokens" db:"output_tokens"`
	CacheCreationTokens int64   `json:"cache_creation_tokens" db:"cache_creation_tokens"`
	CacheReadTokens     int64   `json:"cache_read_tokens" db:"cache_read_tokens"`
	TotalTokens         int64   `json:"total_tokens" db:"total_tokens"`
	CostUSD             float64 `json:"cost_usd" db:"cost_usd"`
	SyncedAt            int64   `json:"synced_at" db:"synced_at"`
}
```

- [ ] **Step 2: Verify compile**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee/.worktrees/feat-token-usage
go build ./internal/infra/model/...
```

Expected: exits 0.

- [ ] **Step 3: Commit**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee/.worktrees/feat-token-usage
git add internal/infra/model/usage.go
git commit -m "feat(usage): add Engine field to UsageRecord model"
```

---

## Task 3: Add engine column to DB schema

**Files:**
- Modify: `internal/infra/store/db.go:376-387`

- [ ] **Step 1: Update CREATE TABLE statement**

In `internal/infra/store/db.go`, find the `create_bee_usage_records` migration (version 41) and add `engine TEXT NOT NULL DEFAULT ''` after `model`:
```sql
CREATE TABLE IF NOT EXISTS bee_usage_records (
    id                    TEXT PRIMARY KEY,
    execution_id          TEXT NOT NULL UNIQUE,
    model                 TEXT NOT NULL DEFAULT '',
    engine                TEXT NOT NULL DEFAULT '',
    input_tokens          INTEGER NOT NULL DEFAULT 0,
    output_tokens         INTEGER NOT NULL DEFAULT 0,
    cache_creation_tokens INTEGER NOT NULL DEFAULT 0,
    cache_read_tokens     INTEGER NOT NULL DEFAULT 0,
    total_tokens          INTEGER NOT NULL DEFAULT 0,
    cost_usd              REAL NOT NULL DEFAULT 0,
    synced_at             INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_usage_execution_id ON bee_usage_records(execution_id);
CREATE INDEX IF NOT EXISTS idx_usage_synced_at ON bee_usage_records(synced_at);
CREATE INDEX IF NOT EXISTS idx_executions_status ON bee_executions(status);
```

- [ ] **Step 2: Verify DB tests pass**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee/.worktrees/feat-token-usage
go test ./internal/infra/store/... -v -run "TestInitDB|TestMigrations"
```

Expected: all PASS (fresh DB includes the new column).

- [ ] **Step 3: Commit**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee/.worktrees/feat-token-usage
git add internal/infra/store/db.go
git commit -m "feat(usage): add engine column to bee_usage_records schema"
```

---

## Task 4: Update UsageStore SQL

**Files:**
- Modify: `internal/infra/store/usage_store.go`

- [ ] **Step 1: Write a failing store test**

Add a new test at the bottom of `internal/infra/store/db_test.go`:
```go
func TestUsageStore_InsertBatch_Engine(t *testing.T) {
	db, err := InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	defer db.Close()

	// Insert a worker and execution so foreign-key-style logic works.
	_, err = db.Exec(`INSERT INTO bee_executions (id, session_id, status, log_path, started_at, completed_at) VALUES ('exec-1','sess-1','completed','/tmp/x.log',1,2)`)
	if err != nil {
		t.Fatalf("insert execution: %v", err)
	}

	store := NewUsageStore(db)
	record := &model.UsageRecord{
		ID:          "rec-1",
		ExecutionID: "exec-1",
		Engine:      "claude",
		Model:       "claude-sonnet-4-6",
		TotalTokens: 100,
		SyncedAt:    1000,
	}
	if err := store.InsertBatch([]*model.UsageRecord{record}); err != nil {
		t.Fatalf("InsertBatch: %v", err)
	}

	got, err := store.GetByExecutionID("exec-1")
	if err != nil {
		t.Fatalf("GetByExecutionID: %v", err)
	}
	if got == nil {
		t.Fatal("expected record, got nil")
	}
	if got.Engine != "claude" {
		t.Errorf("Engine: want %q, got %q", "claude", got.Engine)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee/.worktrees/feat-token-usage
go test ./internal/infra/store/... -v -run TestUsageStore_InsertBatch_Engine
```

Expected: FAIL — column or scan mismatch.

- [ ] **Step 3: Update usageSelect, scanUsageRecord, Insert, InsertBatch**

Replace the entire content of `internal/infra/store/usage_store.go` with:
```go
package store

import (
	"database/sql"
	"fmt"

	"github.com/theopenbee/openbee/internal/infra/model"
)

type UsageStore struct {
	db *sql.DB
}

func NewUsageStore(db *sql.DB) *UsageStore {
	return &UsageStore{db: db}
}

const usageSelect = `SELECT id, execution_id, engine, model, input_tokens, output_tokens, cache_creation_tokens, cache_read_tokens, total_tokens, cost_usd, synced_at FROM bee_usage_records`

func scanUsageRecord(scanner interface{ Scan(...any) error }) (model.UsageRecord, error) {
	var r model.UsageRecord
	err := scanner.Scan(&r.ID, &r.ExecutionID, &r.Engine, &r.Model, &r.InputTokens, &r.OutputTokens,
		&r.CacheCreationTokens, &r.CacheReadTokens, &r.TotalTokens, &r.CostUSD, &r.SyncedAt)
	return r, err
}

func (s *UsageStore) Insert(record *model.UsageRecord) error {
	_, err := s.db.Exec(
		`INSERT OR IGNORE INTO bee_usage_records
         (id, execution_id, engine, model, input_tokens, output_tokens, cache_creation_tokens, cache_read_tokens, total_tokens, cost_usd, synced_at)
         VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		record.ID, record.ExecutionID, record.Engine, record.Model,
		record.InputTokens, record.OutputTokens,
		record.CacheCreationTokens, record.CacheReadTokens,
		record.TotalTokens, record.CostUSD, record.SyncedAt,
	)
	if err != nil {
		return fmt.Errorf("insert usage record: %w", err)
	}
	return nil
}

func (s *UsageStore) InsertBatch(records []*model.UsageRecord) error {
	if len(records) == 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	stmt, err := tx.Prepare(`INSERT OR IGNORE INTO bee_usage_records
         (id, execution_id, engine, model, input_tokens, output_tokens, cache_creation_tokens, cache_read_tokens, total_tokens, cost_usd, synced_at)
         VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("prepare insert: %w", err)
	}
	defer stmt.Close()
	for _, r := range records {
		if _, err := stmt.Exec(r.ID, r.ExecutionID, r.Engine, r.Model,
			r.InputTokens, r.OutputTokens,
			r.CacheCreationTokens, r.CacheReadTokens,
			r.TotalTokens, r.CostUSD, r.SyncedAt); err != nil {
			tx.Rollback()
			return fmt.Errorf("insert usage record %s: %w", r.ExecutionID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

func (s *UsageStore) GetByExecutionID(executionID string) (*model.UsageRecord, error) {
	row := s.db.QueryRow(usageSelect+` WHERE execution_id = ?`, executionID)
	r, err := scanUsageRecord(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get usage record: %w", err)
	}
	return &r, nil
}

func (s *UsageStore) ListUnsynced(limit int) ([]model.UnsyncedExecution, error) {
	rows, err := s.db.Query(
		`SELECT e.id, e.log_path, e.session_id,
		        COALESCE(e.started_at, 0), COALESCE(e.completed_at, 0)
         FROM bee_executions e
         LEFT JOIN bee_usage_records u ON e.id = u.execution_id
         WHERE e.status IN (?, ?)
           AND e.log_path != ''
           AND u.id IS NULL
         LIMIT ?`, model.ExecStatusCompleted, model.ExecStatusFailed, limit)
	if err != nil {
		return nil, fmt.Errorf("list unsynced executions: %w", err)
	}
	defer rows.Close()

	result := make([]model.UnsyncedExecution, 0, limit)
	for rows.Next() {
		var e model.UnsyncedExecution
		if err := rows.Scan(&e.ID, &e.LogPath, &e.SessionID, &e.StartedAt, &e.CompletedAt); err != nil {
			return nil, fmt.Errorf("scan unsynced execution: %w", err)
		}
		result = append(result, e)
	}
	return result, rows.Err()
}
```

- [ ] **Step 4: Run all store tests**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee/.worktrees/feat-token-usage
go test ./internal/infra/store/... -v
```

Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee/.worktrees/feat-token-usage
git add internal/infra/store/usage_store.go internal/infra/store/db_test.go
git commit -m "feat(usage): add engine to UsageStore SQL and add store test"
```

---

## Task 5: Update Syncer — set Engine and add debug logs

**Files:**
- Modify: `internal/domain/usage/syncer.go`

- [ ] **Step 1: Update syncer**

Replace the full content of `internal/domain/usage/syncer.go` with:
```go
package usage

import (
	"context"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	usageparser "github.com/theopenbee/openbee/internal/ai/usage"
	"github.com/theopenbee/openbee/internal/infra/logger"
	"github.com/theopenbee/openbee/internal/infra/model"
)

var log = logger.With(zap.String("component", "usagesyncer"))

type usageSyncStore interface {
	ListUnsynced(limit int) ([]model.UnsyncedExecution, error)
	InsertBatch(records []*model.UsageRecord) error
}

type SyncerConfig struct {
	PiSessionsDir    string // e.g. ~/.openbee/.pi/sessions
	CodexStoreDir    string // e.g. ~/.openbee/.codex/sessions (uuid→thread_id mapping files)
	CodexSessionsDir string // codex native sessions dir, e.g. ~/.codex/sessions
}

type UsageSyncer struct {
	store     usageSyncStore
	cfg       SyncerConfig
	interval  time.Duration
	batchSize int
}

func NewUsageSyncer(store usageSyncStore, interval time.Duration, batchSize int, cfg SyncerConfig) *UsageSyncer {
	return &UsageSyncer{store: store, cfg: cfg, interval: interval, batchSize: batchSize}
}

func (s *UsageSyncer) Run(ctx context.Context) {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			for s.syncBatch(ctx) {
				if ctx.Err() != nil {
					return
				}
			}
		case <-ctx.Done():
			return
		}
	}
}

func (s *UsageSyncer) syncBatch(ctx context.Context) bool {
	execs, err := s.store.ListUnsynced(s.batchSize)
	if err != nil {
		log.Error("list unsynced executions", zap.Error(err))
		return false
	}

	log.Debug("sync batch start", zap.Int("unsynced_count", len(execs)))

	start := time.Now()
	now := start.UnixMilli()
	records := make([]*model.UsageRecord, 0, len(execs))
	for _, exec := range execs {
		if ctx.Err() != nil {
			return false
		}
		parseCtx := usageparser.ParseContext{
			LogPath:          exec.LogPath,
			SessionID:        exec.SessionID,
			PiSessionsDir:    s.cfg.PiSessionsDir,
			CodexStoreDir:    s.cfg.CodexStoreDir,
			CodexSessionsDir: s.cfg.CodexSessionsDir,
			StartedAt:        exec.StartedAt,
			CompletedAt:      exec.CompletedAt,
		}
		data, err := usageparser.ParseUsage(parseCtx)
		if err != nil {
			log.Error("parse usage", zap.String("executionID", exec.ID), zap.Error(err))
		}
		if data == nil {
			data = &usageparser.UsageData{}
		}
		log.Debug("parsed usage",
			zap.String("executionID", exec.ID),
			zap.String("engine", data.Engine),
			zap.Int64("total_tokens", data.TotalTokens),
			zap.Float64("cost_usd", data.CostUSD),
		)
		records = append(records, &model.UsageRecord{
			ID:                  uuid.New().String(),
			ExecutionID:         exec.ID,
			Engine:              data.Engine,
			Model:               data.Model,
			InputTokens:         data.InputTokens,
			OutputTokens:        data.OutputTokens,
			CacheCreationTokens: data.CacheCreationTokens,
			CacheReadTokens:     data.CacheReadTokens,
			TotalTokens:         data.TotalTokens,
			CostUSD:             data.CostUSD,
			SyncedAt:            now,
		})
	}
	if err := s.store.InsertBatch(records); err != nil {
		log.Error("insert usage batch", zap.Error(err))
	}

	log.Debug("sync batch done",
		zap.Int("count", len(records)),
		zap.Int64("duration_ms", time.Since(start).Milliseconds()),
	)

	return len(execs) == s.batchSize
}
```

- [ ] **Step 2: Build entire project**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee/.worktrees/feat-token-usage
go build ./...
```

Expected: exits 0, no errors.

- [ ] **Step 3: Run all tests**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee/.worktrees/feat-token-usage
go test ./...
```

Expected: all PASS.

- [ ] **Step 4: Commit**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee/.worktrees/feat-token-usage
git add internal/domain/usage/syncer.go
git commit -m "feat(usage): set Engine in UsageRecord and add debug logs to syncBatch"
```
