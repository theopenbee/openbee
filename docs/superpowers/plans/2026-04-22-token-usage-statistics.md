# Token Usage Statistics Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Parse token usage from execution log files and store it in a new `bee_usage_records` table, synced by a 60-second background job that handles both new and historical executions.

**Architecture:** A `UsageSyncer` background goroutine periodically queries for completed `bee_executions` that have no corresponding `bee_usage_records` row (LEFT JOIN IS NULL), reads each log file, extracts token/cost data, and inserts a record. Claude logs contain full token data in the `{"type":"result"}` event; Codex and Pi logs have no token data so their records are zero-value placeholders that prevent re-scanning.

**Tech Stack:** Go, SQLite (modernc.org/sqlite), `github.com/google/uuid`, `github.com/theopenbee/openbee/internal/ai` (ScanJSONLines helper), `github.com/stretchr/testify`

---

## File Map

| Action | Path | Responsibility |
|--------|------|----------------|
| Create | `internal/infra/model/usage.go` | `UsageRecord`, `UnsyncedExecution` structs |
| Create | `internal/infra/store/usage_store.go` | Insert, GetByExecutionID, ListUnsynced |
| Modify | `internal/infra/store/db.go` | Migration 41: CREATE TABLE bee_usage_records |
| Create | `internal/ai/usage/parser.go` | `UsageData` struct, `ParseUsageFromLog` dispatcher |
| Create | `internal/ai/usage/claude_parser.go` | Extract tokens from Claude stream-json result event |
| Create | `internal/ai/usage/codex_parser.go` | Zero-value (Codex logs have no token data) |
| Create | `internal/ai/usage/pi_parser.go` | Zero-value (Pi logs have no token data) |
| Create | `internal/domain/usage/syncer.go` | `UsageSyncer.Run(ctx)` background loop |
| Modify | `internal/app/app.go` | Add `usageStore` to `appStores`, register syncer in `runners` |
| Create | `internal/infra/store/usage_store_test.go` | Tests for Insert idempotency and ListUnsynced |
| Create | `internal/ai/usage/parser_test.go` | Tests for all three engine formats |
| Create | `internal/domain/usage/syncer_test.go` | Test the sync batch logic |

---

## Task 1: Database Migration + UsageRecord Model

**Files:**
- Modify: `internal/infra/store/db.go`
- Create: `internal/infra/model/usage.go`

- [ ] **Step 1.1: Write the failing table-exists test in `internal/infra/store/db_test.go`**

Add to the existing file (same package `store`):

```go
func TestMigration_UsageRecordsTable(t *testing.T) {
    db, err := InitDB(t.TempDir() + "/test.db")
    if err != nil {
        t.Fatalf("InitDB: %v", err)
    }
    defer db.Close()

    var name string
    if err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name='bee_usage_records'`).Scan(&name); err != nil {
        t.Fatalf("bee_usage_records table not found: %v", err)
    }

    // Verify execution_id unique constraint by inserting duplicate
    _, err = db.Exec(`INSERT INTO bee_usage_records (id, execution_id, model, input_tokens, output_tokens, cache_creation_tokens, cache_read_tokens, total_tokens, cost_usd, synced_at) VALUES ('r1','exec1','m',1,2,3,4,10,0.1,1)`)
    if err != nil {
        t.Fatalf("first insert failed: %v", err)
    }
    _, err = db.Exec(`INSERT OR IGNORE INTO bee_usage_records (id, execution_id, model, input_tokens, output_tokens, cache_creation_tokens, cache_read_tokens, total_tokens, cost_usd, synced_at) VALUES ('r2','exec1','m',1,2,3,4,10,0.1,1)`)
    if err != nil {
        t.Fatalf("INSERT OR IGNORE on duplicate should not error: %v", err)
    }
    var count int
    db.QueryRow(`SELECT COUNT(*) FROM bee_usage_records WHERE execution_id='exec1'`).Scan(&count)
    if count != 1 {
        t.Errorf("expected 1 row after duplicate INSERT OR IGNORE, got %d", count)
    }
}
```

- [ ] **Step 1.2: Run the test to verify it fails**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee
go test ./internal/infra/store/... -run TestMigration_UsageRecordsTable -v
```

Expected: FAIL — `bee_usage_records table not found`

- [ ] **Step 1.3: Add migration 41 to `internal/infra/store/db.go`**

Find the closing `}` of the `migrations` slice (after version 40 entry, around line 371) and insert before it:

```go
    {
        version: 41,
        name:    "create_bee_usage_records",
        sql: `
        CREATE TABLE IF NOT EXISTS bee_usage_records (
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
        CREATE INDEX IF NOT EXISTS idx_usage_execution_id ON bee_usage_records(execution_id);
        CREATE INDEX IF NOT EXISTS idx_usage_synced_at ON bee_usage_records(synced_at);
    `,
    },
```

- [ ] **Step 1.4: Create `internal/infra/model/usage.go`**

```go
package model

// UsageRecord holds token consumption and cost for a single execution.
type UsageRecord struct {
    ID                   string  `json:"id" db:"id"`
    ExecutionID          string  `json:"execution_id" db:"execution_id"`
    Model                string  `json:"model" db:"model"`
    InputTokens          int64   `json:"input_tokens" db:"input_tokens"`
    OutputTokens         int64   `json:"output_tokens" db:"output_tokens"`
    CacheCreationTokens  int64   `json:"cache_creation_tokens" db:"cache_creation_tokens"`
    CacheReadTokens      int64   `json:"cache_read_tokens" db:"cache_read_tokens"`
    TotalTokens          int64   `json:"total_tokens" db:"total_tokens"`
    CostUSD              float64 `json:"cost_usd" db:"cost_usd"`
    SyncedAt             int64   `json:"synced_at" db:"synced_at"`
}

// UnsyncedExecution is a lightweight query result used by UsageSyncer.
type UnsyncedExecution struct {
    ID      string
    LogPath string
}
```

- [ ] **Step 1.5: Run the test to verify it passes**

```bash
go test ./internal/infra/store/... -run TestMigration_UsageRecordsTable -v
```

Expected: PASS

- [ ] **Step 1.6: Verify all existing migration tests still pass**

```bash
go test ./internal/infra/store/... -run TestMigrations -v
go test ./internal/infra/store/... -run TestInitDB -v
```

Expected: all PASS

- [ ] **Step 1.7: Commit**

```bash
git add internal/infra/store/db.go internal/infra/store/db_test.go internal/infra/model/usage.go
git commit -m "feat: add bee_usage_records migration and UsageRecord model"
```

---

## Task 2: UsageStore

**Files:**
- Create: `internal/infra/store/usage_store.go`
- Create: `internal/infra/store/usage_store_test.go`

- [ ] **Step 2.1: Write `internal/infra/store/usage_store_test.go`**

```go
package store

import (
    "testing"
    "time"

    "github.com/google/uuid"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
    "github.com/theopenbee/openbee/internal/infra/model"
)

func TestUsageStore_Insert_And_Get(t *testing.T) {
    db, err := InitDB(t.TempDir() + "/test.db")
    require.NoError(t, err)
    defer db.Close()

    us := NewUsageStore(db)
    execID := uuid.New().String()
    record := &model.UsageRecord{
        ID:                  uuid.New().String(),
        ExecutionID:         execID,
        Model:               "claude-sonnet-4-6",
        InputTokens:         100,
        OutputTokens:        200,
        CacheCreationTokens: 50,
        CacheReadTokens:     300,
        TotalTokens:         650,
        CostUSD:             0.42,
        SyncedAt:            time.Now().UnixMilli(),
    }

    require.NoError(t, us.Insert(record))

    got, err := us.GetByExecutionID(execID)
    require.NoError(t, err)
    require.NotNil(t, got)
    assert.Equal(t, execID, got.ExecutionID)
    assert.Equal(t, "claude-sonnet-4-6", got.Model)
    assert.Equal(t, int64(100), got.InputTokens)
    assert.Equal(t, int64(650), got.TotalTokens)
    assert.InDelta(t, 0.42, got.CostUSD, 0.001)
}

func TestUsageStore_Insert_Idempotent(t *testing.T) {
    db, err := InitDB(t.TempDir() + "/test.db")
    require.NoError(t, err)
    defer db.Close()

    us := NewUsageStore(db)
    execID := uuid.New().String()
    record := &model.UsageRecord{
        ID:          uuid.New().String(),
        ExecutionID: execID,
        SyncedAt:    time.Now().UnixMilli(),
    }

    require.NoError(t, us.Insert(record))
    require.NoError(t, us.Insert(record)) // second call must not error
}

func TestUsageStore_GetByExecutionID_NotFound(t *testing.T) {
    db, err := InitDB(t.TempDir() + "/test.db")
    require.NoError(t, err)
    defer db.Close()

    us := NewUsageStore(db)
    got, err := us.GetByExecutionID("no-such-id")
    require.NoError(t, err)
    assert.Nil(t, got)
}

func TestUsageStore_ListUnsynced(t *testing.T) {
    db, err := InitDB(t.TempDir() + "/test.db")
    require.NoError(t, err)
    defer db.Close()

    ws := NewWorkerStore(db)
    es := NewExecutionStore(db, t.TempDir())
    us := NewUsageStore(db)

    w, _ := ws.Create(model.Worker{Name: "bot", WorkDir: "/tmp"})

    // Create 3 completed executions with a log_path
    for i := range 3 {
        exec, _ := es.Create(w.ID, "task", uuid.New().String())
        db.Exec(`UPDATE bee_executions SET status='completed', log_path='/tmp/fake.log' WHERE id=?`, exec.ID)
        _ = i
    }

    // Create 1 failed execution with log_path
    exec4, _ := es.Create(w.ID, "task", uuid.New().String())
    db.Exec(`UPDATE bee_executions SET status='failed', log_path='/tmp/fake.log' WHERE id=?`, exec4.ID)

    // Create 1 pending execution (should NOT appear)
    es.Create(w.ID, "task", uuid.New().String())

    unsynced, err := us.ListUnsynced(10)
    require.NoError(t, err)
    assert.Len(t, unsynced, 4)

    // Sync one of them, it should disappear from the list
    require.NoError(t, us.Insert(&model.UsageRecord{
        ID:          uuid.New().String(),
        ExecutionID: unsynced[0].ID,
        SyncedAt:    time.Now().UnixMilli(),
    }))

    unsynced2, err := us.ListUnsynced(10)
    require.NoError(t, err)
    assert.Len(t, unsynced2, 3)
}
```

- [ ] **Step 2.2: Run the test to verify it fails**

```bash
go test ./internal/infra/store/... -run TestUsageStore -v
```

Expected: FAIL — `NewUsageStore undefined`

- [ ] **Step 2.3: Create `internal/infra/store/usage_store.go`**

```go
package store

import (
    "database/sql"
    "fmt"
    "time"

    "github.com/theopenbee/openbee/internal/infra/model"
)

type UsageStore struct {
    db *sql.DB
}

func NewUsageStore(db *sql.DB) *UsageStore {
    return &UsageStore{db: db}
}

// Insert writes a usage record. Uses INSERT OR IGNORE so duplicate execution_id calls are safe.
func (s *UsageStore) Insert(record *model.UsageRecord) error {
    _, err := s.db.Exec(
        `INSERT OR IGNORE INTO bee_usage_records
         (id, execution_id, model, input_tokens, output_tokens, cache_creation_tokens, cache_read_tokens, total_tokens, cost_usd, synced_at)
         VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
        record.ID, record.ExecutionID, record.Model,
        record.InputTokens, record.OutputTokens,
        record.CacheCreationTokens, record.CacheReadTokens,
        record.TotalTokens, record.CostUSD, record.SyncedAt,
    )
    if err != nil {
        return fmt.Errorf("insert usage record: %w", err)
    }
    return nil
}

// GetByExecutionID returns the usage record for the given execution, or nil if not found.
func (s *UsageStore) GetByExecutionID(executionID string) (*model.UsageRecord, error) {
    row := s.db.QueryRow(
        `SELECT id, execution_id, model, input_tokens, output_tokens, cache_creation_tokens, cache_read_tokens, total_tokens, cost_usd, synced_at
         FROM bee_usage_records WHERE execution_id = ?`, executionID)
    var r model.UsageRecord
    err := row.Scan(&r.ID, &r.ExecutionID, &r.Model, &r.InputTokens, &r.OutputTokens,
        &r.CacheCreationTokens, &r.CacheReadTokens, &r.TotalTokens, &r.CostUSD, &r.SyncedAt)
    if err == sql.ErrNoRows {
        return nil, nil
    }
    if err != nil {
        return nil, fmt.Errorf("get usage record: %w", err)
    }
    return &r, nil
}

// ListUnsynced returns up to limit completed/failed executions that have no usage record.
func (s *UsageStore) ListUnsynced(limit int) ([]model.UnsyncedExecution, error) {
    rows, err := s.db.Query(
        `SELECT e.id, e.log_path
         FROM bee_executions e
         LEFT JOIN bee_usage_records u ON e.id = u.execution_id
         WHERE e.status IN ('completed', 'failed')
           AND e.log_path != ''
           AND u.id IS NULL
         LIMIT ?`, limit)
    if err != nil {
        return nil, fmt.Errorf("list unsynced executions: %w", err)
    }
    defer rows.Close()

    var result []model.UnsyncedExecution
    for rows.Next() {
        var e model.UnsyncedExecution
        if err := rows.Scan(&e.ID, &e.LogPath); err != nil {
            return nil, fmt.Errorf("scan unsynced execution: %w", err)
        }
        result = append(result, e)
    }
    return result, rows.Err()
}

// nowMS returns the current time in milliseconds.
func nowMS() int64 {
    return time.Now().UnixMilli()
}
```

- [ ] **Step 2.4: Run the tests to verify they pass**

```bash
go test ./internal/infra/store/... -run TestUsageStore -v
```

Expected: all PASS

- [ ] **Step 2.5: Commit**

```bash
git add internal/infra/store/usage_store.go internal/infra/store/usage_store_test.go
git commit -m "feat: add UsageStore with Insert, GetByExecutionID, ListUnsynced"
```

---

## Task 3: Log Parsers

**Files:**
- Create: `internal/ai/usage/parser.go`
- Create: `internal/ai/usage/claude_parser.go`
- Create: `internal/ai/usage/codex_parser.go`
- Create: `internal/ai/usage/pi_parser.go`
- Create: `internal/ai/usage/parser_test.go`

- [ ] **Step 3.1: Write `internal/ai/usage/parser_test.go`**

```go
package usage

import (
    "os"
    "path/filepath"
    "testing"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func writeTempLog(t *testing.T, content string) string {
    t.Helper()
    path := filepath.Join(t.TempDir(), "test.log")
    require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
    return path
}

func TestParseUsageFromLog_Claude_Success(t *testing.T) {
    log := `{"type":"system","subtype":"init","cwd":"/tmp"}
{"type":"assistant","message":{"model":"claude-sonnet-4-6","id":"msg_01","type":"message","role":"assistant","content":[]}}
{"type":"result","subtype":"success","is_error":false,"total_cost_usd":0.35289275,"usage":{"input_tokens":8,"cache_creation_input_tokens":14984,"cache_read_input_tokens":103792,"output_tokens":2849}}`

    data, err := ParseUsageFromLog(writeTempLog(t, log))
    require.NoError(t, err)
    assert.Equal(t, "claude-sonnet-4-6", data.Model)
    assert.Equal(t, int64(8), data.InputTokens)
    assert.Equal(t, int64(2849), data.OutputTokens)
    assert.Equal(t, int64(14984), data.CacheCreationTokens)
    assert.Equal(t, int64(103792), data.CacheReadTokens)
    assert.Equal(t, int64(8+2849+14984+103792), data.TotalTokens)
    assert.InDelta(t, 0.35289275, data.CostUSD, 0.000001)
}

func TestParseUsageFromLog_Claude_NoResultEvent(t *testing.T) {
    // Incomplete log (process killed mid-run) — should return zero-value, no error
    log := `{"type":"system","subtype":"init","cwd":"/tmp"}
{"type":"assistant","message":{"model":"claude-sonnet-4-6","id":"msg_01","type":"message","role":"assistant","content":[]}}`

    data, err := ParseUsageFromLog(writeTempLog(t, log))
    require.NoError(t, err)
    assert.Equal(t, int64(0), data.TotalTokens)
    assert.Equal(t, float64(0), data.CostUSD)
}

func TestParseUsageFromLog_Codex(t *testing.T) {
    log := `{"type":"thread.started","thread_id":"thread_abc123"}
{"type":"item.completed","item":{"type":"agent_message","text":"hello"}}`

    data, err := ParseUsageFromLog(writeTempLog(t, log))
    require.NoError(t, err)
    assert.Equal(t, int64(0), data.TotalTokens)
    assert.Equal(t, "", data.Model)
}

func TestParseUsageFromLog_Pi(t *testing.T) {
    log := `{"type":"agent_end","messages":[{"role":"assistant","content":[{"type":"text","text":"done"}]}]}`

    data, err := ParseUsageFromLog(writeTempLog(t, log))
    require.NoError(t, err)
    assert.Equal(t, int64(0), data.TotalTokens)
}

func TestParseUsageFromLog_EmptyFile(t *testing.T) {
    data, err := ParseUsageFromLog(writeTempLog(t, ""))
    require.NoError(t, err)
    assert.Equal(t, int64(0), data.TotalTokens)
}

func TestParseUsageFromLog_FileNotFound(t *testing.T) {
    data, err := ParseUsageFromLog("/no/such/file.log")
    require.NoError(t, err) // missing file is not an error — returns zero-value
    assert.Equal(t, int64(0), data.TotalTokens)
}
```

- [ ] **Step 3.2: Run the test to verify it fails**

```bash
go test ./internal/ai/usage/... -v
```

Expected: FAIL — package not found or `ParseUsageFromLog` undefined

- [ ] **Step 3.3: Create `internal/ai/usage/parser.go`**

```go
package usage

import (
    "encoding/json"
    "os"

    ai "github.com/theopenbee/openbee/internal/ai"
)

// UsageData holds the parsed token counts and cost from an execution log.
type UsageData struct {
    Model               string
    InputTokens         int64
    OutputTokens        int64
    CacheCreationTokens int64
    CacheReadTokens     int64
    TotalTokens         int64
    CostUSD             float64
}

// ParseUsageFromLog reads the log file at logPath, auto-detects the engine
// format, and returns token usage data. Returns a zero-value UsageData (not
// an error) when the file is missing, empty, or contains no token data.
func ParseUsageFromLog(logPath string) (*UsageData, error) {
    f, err := os.Open(logPath)
    if err != nil {
        return &UsageData{}, nil
    }
    defer f.Close()

    var data UsageData
    var model string

    ai.ScanJSONLines(f, func(line string) bool {
        var peek struct {
            Type string `json:"type"`
        }
        if json.Unmarshal([]byte(line), &peek) != nil {
            return true
        }
        switch peek.Type {
        case "assistant":
            if m := extractClaudeModel(line); m != "" {
                model = m
            }
        case "result":
            // Claude stream-json terminal event — extract full usage.
            extractClaudeResult(line, &data)
            data.Model = model
            return false
        case "thread.started", "agent_end":
            // Codex / Pi — logs contain no token data; stop scanning.
            return false
        }
        return true
    })

    return &data, nil
}
```

- [ ] **Step 3.4: Create `internal/ai/usage/claude_parser.go`**

```go
package usage

import "encoding/json"

type claudeResultEvent struct {
    Type        string            `json:"type"`
    TotalCostUSD float64          `json:"total_cost_usd"`
    Usage       claudeUsageFields `json:"usage"`
}

type claudeUsageFields struct {
    InputTokens              int64 `json:"input_tokens"`
    OutputTokens             int64 `json:"output_tokens"`
    CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
    CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
}

type claudeAssistantEvent struct {
    Message struct {
        Model string `json:"model"`
    } `json:"message"`
}

// extractClaudeModel returns the model name from a Claude assistant event line,
// or "" if the line does not contain a model field.
func extractClaudeModel(line string) string {
    var event claudeAssistantEvent
    if json.Unmarshal([]byte(line), &event) != nil {
        return ""
    }
    return event.Message.Model
}

// extractClaudeResult parses a Claude result event line and populates data.
func extractClaudeResult(line string, data *UsageData) {
    var event claudeResultEvent
    if json.Unmarshal([]byte(line), &event) != nil {
        return
    }
    data.InputTokens = event.Usage.InputTokens
    data.OutputTokens = event.Usage.OutputTokens
    data.CacheCreationTokens = event.Usage.CacheCreationInputTokens
    data.CacheReadTokens = event.Usage.CacheReadInputTokens
    data.TotalTokens = data.InputTokens + data.OutputTokens + data.CacheCreationTokens + data.CacheReadTokens
    data.CostUSD = event.TotalCostUSD
}
```

- [ ] **Step 3.5: Create `internal/ai/usage/codex_parser.go`**

```go
package usage

// Codex execution logs (--json format) contain message content but no token
// usage fields. This file is a placeholder for future Codex token support.
// ParseUsageFromLog returns zero-value UsageData for Codex logs.
```

- [ ] **Step 3.6: Create `internal/ai/usage/pi_parser.go`**

```go
package usage

// Pi agent execution logs (--mode json format) contain message content but no
// token usage fields. This file is a placeholder for future Pi token support.
// ParseUsageFromLog returns zero-value UsageData for Pi logs.
```

- [ ] **Step 3.7: Run the tests to verify they pass**

```bash
go test ./internal/ai/usage/... -v
```

Expected: all PASS

- [ ] **Step 3.8: Commit**

```bash
git add internal/ai/usage/
git commit -m "feat: add token usage log parsers for Claude, Codex, Pi"
```

---

## Task 4: Usage Syncer

**Files:**
- Create: `internal/domain/usage/syncer.go`
- Create: `internal/domain/usage/syncer_test.go`

- [ ] **Step 4.1: Write `internal/domain/usage/syncer_test.go`**

```go
package usage

import (
    "testing"
    "time"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
    "github.com/theopenbee/openbee/internal/infra/model"
)

// stubStore implements usageSyncStore for testing.
type stubStore struct {
    unsynced []model.UnsyncedExecution
    inserted []*model.UsageRecord
}

func (s *stubStore) ListUnsynced(limit int) ([]model.UnsyncedExecution, error) {
    if len(s.unsynced) > limit {
        return s.unsynced[:limit], nil
    }
    return s.unsynced, nil
}

func (s *stubStore) Insert(record *model.UsageRecord) error {
    s.inserted = append(s.inserted, record)
    return nil
}

func TestUsageSyncer_SyncBatch_InsertsRecord(t *testing.T) {
    // Write a minimal Claude log with a result event to a temp file
    dir := t.TempDir()
    logPath := dir + "/exec.log"
    logContent := `{"type":"assistant","message":{"model":"claude-sonnet-4-6","content":[]}}
{"type":"result","subtype":"success","is_error":false,"total_cost_usd":0.10,"usage":{"input_tokens":5,"output_tokens":10,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}`
    require.NoError(t, writeFile(logPath, logContent))

    stub := &stubStore{
        unsynced: []model.UnsyncedExecution{
            {ID: "exec-1", LogPath: logPath},
        },
    }

    syncer := NewUsageSyncer(stub, 60*time.Second, 50)
    more := syncer.syncBatch()

    assert.False(t, more, "batch < limit so more should be false")
    require.Len(t, stub.inserted, 1)
    assert.Equal(t, "exec-1", stub.inserted[0].ExecutionID)
    assert.Equal(t, "claude-sonnet-4-6", stub.inserted[0].Model)
    assert.Equal(t, int64(15), stub.inserted[0].TotalTokens)
    assert.InDelta(t, 0.10, stub.inserted[0].CostUSD, 0.001)
}

func TestUsageSyncer_SyncBatch_MissingLog(t *testing.T) {
    stub := &stubStore{
        unsynced: []model.UnsyncedExecution{
            {ID: "exec-2", LogPath: "/no/such/file.log"},
        },
    }
    syncer := NewUsageSyncer(stub, 60*time.Second, 50)
    _ = syncer.syncBatch()

    // Missing log → zero-value record inserted to prevent retry
    require.Len(t, stub.inserted, 1)
    assert.Equal(t, "exec-2", stub.inserted[0].ExecutionID)
    assert.Equal(t, int64(0), stub.inserted[0].TotalTokens)
}

func TestUsageSyncer_SyncBatch_MoreWhenFull(t *testing.T) {
    stub := &stubStore{
        unsynced: []model.UnsyncedExecution{
            {ID: "e1", LogPath: "/no/such/1.log"},
            {ID: "e2", LogPath: "/no/such/2.log"},
        },
    }
    syncer := NewUsageSyncer(stub, 60*time.Second, 2) // batchSize == len(unsynced)
    more := syncer.syncBatch()
    assert.True(t, more, "full batch should signal more")
}

// writeFile is a test helper.
func writeFile(path, content string) error {
    return os.WriteFile(path, []byte(content), 0o644)
}
```

Add `"os"` to the import block.

- [ ] **Step 4.2: Run to verify failure**

```bash
go test ./internal/domain/usage/... -v
```

Expected: FAIL — package not found

- [ ] **Step 4.3: Create `internal/domain/usage/syncer.go`**

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
    Insert(record *model.UsageRecord) error
}

// UsageSyncer periodically syncs token usage from execution logs into bee_usage_records.
type UsageSyncer struct {
    store     usageSyncStore
    interval  time.Duration
    batchSize int
}

// NewUsageSyncer creates a UsageSyncer with the given poll interval and batch size.
func NewUsageSyncer(store usageSyncStore, interval time.Duration, batchSize int) *UsageSyncer {
    return &UsageSyncer{store: store, interval: interval, batchSize: batchSize}
}

// Run polls on interval until ctx is cancelled. If a batch is full it re-runs
// immediately to drain any backlog before waiting for the next tick.
func (s *UsageSyncer) Run(ctx context.Context) {
    ticker := time.NewTicker(s.interval)
    defer ticker.Stop()
    for {
        select {
        case <-ticker.C:
            for s.syncBatch() {
            }
        case <-ctx.Done():
            return
        }
    }
}

// syncBatch processes one batch of unsynced executions. Returns true if the
// batch was full (caller should re-run to drain further backlog).
func (s *UsageSyncer) syncBatch() bool {
    execs, err := s.store.ListUnsynced(s.batchSize)
    if err != nil {
        log.Error("list unsynced executions", zap.Error(err))
        return false
    }

    for _, exec := range execs {
        data, _ := usageparser.ParseUsageFromLog(exec.LogPath)
        record := &model.UsageRecord{
            ID:                  uuid.New().String(),
            ExecutionID:         exec.ID,
            Model:               data.Model,
            InputTokens:         data.InputTokens,
            OutputTokens:        data.OutputTokens,
            CacheCreationTokens: data.CacheCreationTokens,
            CacheReadTokens:     data.CacheReadTokens,
            TotalTokens:         data.TotalTokens,
            CostUSD:             data.CostUSD,
            SyncedAt:            time.Now().UnixMilli(),
        }
        if err := s.store.Insert(record); err != nil {
            log.Error("insert usage record", zap.String("executionID", exec.ID), zap.Error(err))
        }
    }

    return len(execs) == s.batchSize
}
```

- [ ] **Step 4.4: Run the tests to verify they pass**

```bash
go test ./internal/domain/usage/... -v
```

Expected: all PASS

- [ ] **Step 4.5: Commit**

```bash
git add internal/domain/usage/
git commit -m "feat: add UsageSyncer background job"
```

---

## Task 5: Wire Up in app.go

**Files:**
- Modify: `internal/app/app.go`

- [ ] **Step 5.1: Add `usageStore` to the `appStores` struct**

In `internal/app/app.go`, find the `appStores` struct (around line 215) and add:

```go
type appStores struct {
    workerStore       *store.WorkerStore
    envConfigStore    *store.EnvConfigStore
    systemConfigStore *store.SystemConfigStore
    execStore         *store.ExecutionStore
    msgStore          *store.MessageStore
    taskStore         *store.TaskStore
    sessionStore      *store.SessionStore
    outboundMsgStore  *store.OutboundMessageStore
    constraintStore   *store.ConstraintStore
    departmentStore   *store.DepartmentStore
    statsStore        *store.StatsStore
    usageStore        *store.UsageStore           // ADD THIS LINE
}
```

- [ ] **Step 5.2: Instantiate `UsageStore` in `buildStores`**

In the `buildStores` function (around line 234), add to the return struct:

```go
    return db, appStores{
        workerStore:       store.NewWorkerStore(db),
        envConfigStore:    store.NewEnvConfigStore(db),
        systemConfigStore: store.NewSystemConfigStore(db),
        execStore:         store.NewExecutionStore(db, config.DefaultLogsDir()),
        msgStore:          store.NewMessageStore(db),
        taskStore:         store.NewTaskStore(db),
        sessionStore:      store.NewSessionStore(db),
        outboundMsgStore:  store.NewOutboundMessageStore(db),
        constraintStore:   store.NewConstraintStore(db),
        departmentStore:   store.NewDepartmentStore(db),
        statsStore:        store.NewStatsStore(db),
        usageStore:        store.NewUsageStore(db),   // ADD THIS LINE
    }, nil
```

- [ ] **Step 5.3: Create the syncer and register it in `runners`**

In the `newApp` function, after the existing `runners` slice is built (after line 188), add:

```go
    usageSyncer := usagesyncer.NewUsageSyncer(s.usageStore, 60*time.Second, 50)
    runners = append(runners, func(ctx context.Context) { usageSyncer.Run(ctx) })
```

Add the import at the top of the file:

```go
    usagesyncer "github.com/theopenbee/openbee/internal/domain/usage"
```

- [ ] **Step 5.4: Build to verify no compile errors**

```bash
go build ./...
```

Expected: success with no errors

- [ ] **Step 5.5: Run the full test suite**

```bash
go test ./...
```

Expected: all PASS (or pre-existing failures only — no new failures)

- [ ] **Step 5.6: Commit**

```bash
git add internal/app/app.go
git commit -m "feat: wire UsageSyncer into app startup"
```

---

## Self-Review

**Spec coverage check:**
- ✅ `bee_usage_records` table with 7 required fields (model, input, output, cache_creation, cache_read, total, cost) — Task 1
- ✅ Scheduled sync job (60s interval) — Tasks 4 + 5
- ✅ New table with execution_id FK, no-usage-record = unsynced — Tasks 1 + 2
- ✅ Claude parser (full data) — Task 3
- ✅ Codex parser (zero-value, logs have no token data) — Task 3
- ✅ Pi parser (zero-value, logs have no token data) — Task 3
- ✅ Error handling: missing log → zero-value record (prevents retry) — Task 4
- ✅ Batch of 50, re-runs when full — Task 4
- ✅ Zero UI changes — confirmed

**Type consistency check:**
- `UsageData` defined in `parser.go`, used in `syncer.go` via import ✅
- `model.UsageRecord` defined in `model/usage.go`, used in `store` and `syncer` ✅
- `model.UnsyncedExecution` defined in `model/usage.go`, returned by `ListUnsynced`, used in `syncer` ✅
- `usageSyncStore` interface in `syncer.go` matches `UsageStore` method signatures ✅
- `syncBatch()` in tests matches `syncBatch()` in implementation ✅
