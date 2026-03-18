# Bee Brain Enhancement Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Enhance the bee with system status visibility (4 read-only tools), persistent memory (3 tools + 1 DB table), and self-reflection capabilities, enabling the bee to monitor workers, review its own history, and learn across sessions.

**Architecture:** Extend the existing MCP tool system with 7 new tools. Add a `bee_memories` table via migration. Add `ExecutionStore` and `MemoryStore` dependencies to `MCPServer`. Add a `liveLogs` buffer to `worker.Manager` for real-time log access. Update bee system rules with memory usage guidelines.

**Tech Stack:** Go, SQLite, existing MCP tool framework (`internal/mcp`), existing store patterns (`internal/store`).

---

## File Map

| File | Change |
|------|--------|
| `internal/toolnames/toolnames.go` | Add 7 new tool name constants |
| `internal/store/db.go` | Add migration v17: `bee_memories` table |
| `internal/store/memory_store.go` | New file: CRUD for `bee_memories` |
| `internal/store/memory_store_test.go` | New file: tests for MemoryStore |
| `internal/store/execution_store.go` | Add `ListBeeExecutions`, `ListRecent`, `GetLogsByID` methods |
| `internal/store/execution_store_test.go` | New/modify: tests for new methods |
| `internal/store/task_store.go` | Add `CountPendingByWorkerID`, `CountAllByStatus` methods |
| `internal/store/task_store_test.go` | New/modify: tests for count methods |
| `internal/store/worker_store.go` | Add `CountByStatus` method |
| `internal/worker/manager.go` | Add `liveLogs` field, `GetExecutionLogs` method |
| `internal/mcp/server.go` | Add `executionStore` and `memoryStore` fields to `MCPServer`; update `NewServer` signature |
| `internal/mcp/tools.go` | Add 7 tool schemas and handler cases in `callTool` |
| `internal/mcp/tools_test.go` | Add tests for all 7 new tools |
| `internal/app/app.go` | Update `NewServer` call site with new dependencies |
| `internal/claudemd/claudemd.go` | Add memory usage guidelines to `beeRules()` |

---

### Task 1: Add tool name constants

**Files:**
- Modify: `internal/toolnames/toolnames.go`

- [ ] **Step 1: Add 7 new constants**

```go
// After existing constants, add:
const (
	GetExecutionLogs  = "get_execution_logs"
	GetWorkerStatus   = "get_worker_status"
	GetSystemOverview = "get_system_overview"
	ListBeeExecutions = "list_bee_executions"
	SaveMemory        = "save_memory"
	GetMemory         = "get_memory"
	DeleteMemory      = "delete_memory"
)
```

- [ ] **Step 2: Verify it compiles**

```bash
cd /Users/tengyongzhi/work/theopenbee/openbee && go build ./internal/toolnames/...
```

Expected: success, no errors.

- [ ] **Step 3: Commit**

```bash
git add internal/toolnames/toolnames.go
git commit -m "feat: add tool name constants for bee brain enhancement"
```

---

### Task 2: Add `bee_memories` table migration and MemoryStore

**Files:**
- Modify: `internal/store/db.go`
- Create: `internal/store/memory_store.go`
- Create: `internal/store/memory_store_test.go`

- [ ] **Step 1: Write failing test for MemoryStore**

Create `internal/store/memory_store_test.go`:

```go
package store

import (
	"testing"
)

func TestMemoryStore_SaveAndGet(t *testing.T) {
	db, err := InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ms := NewMemoryStore(db)

	// Save a memory
	err = ms.Save("global", "test_key", "test_value")
	if err != nil {
		t.Fatal(err)
	}

	// Get single memory by key
	mem, err := ms.Get("global", "test_key")
	if err != nil {
		t.Fatal(err)
	}
	if mem == nil {
		t.Fatal("expected memory, got nil")
	}
	if mem.Value != "test_value" {
		t.Errorf("expected value 'test_value', got %q", mem.Value)
	}

	// Get non-existent key
	mem, err = ms.Get("global", "nonexistent")
	if err != nil {
		t.Fatal(err)
	}
	if mem != nil {
		t.Error("expected nil for non-existent key")
	}
}

func TestMemoryStore_Upsert(t *testing.T) {
	db, err := InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ms := NewMemoryStore(db)

	// Save initial
	if err := ms.Save("global", "key1", "value1"); err != nil {
		t.Fatal(err)
	}
	// Upsert same key
	if err := ms.Save("global", "key1", "value2"); err != nil {
		t.Fatal(err)
	}

	mem, err := ms.Get("global", "key1")
	if err != nil {
		t.Fatal(err)
	}
	if mem.Value != "value2" {
		t.Errorf("expected updated value 'value2', got %q", mem.Value)
	}
}

func TestMemoryStore_ListByScope(t *testing.T) {
	db, err := InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ms := NewMemoryStore(db)

	ms.Save("global", "key1", "val1")
	ms.Save("global", "key2", "val2")
	ms.Save("user123", "key3", "val3")

	memories, err := ms.ListByScope("global", 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(memories) != 2 {
		t.Errorf("expected 2 global memories, got %d", len(memories))
	}
}

func TestMemoryStore_Delete(t *testing.T) {
	db, err := InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ms := NewMemoryStore(db)
	ms.Save("global", "key1", "val1")

	if err := ms.Delete("global", "key1"); err != nil {
		t.Fatal(err)
	}
	mem, _ := ms.Get("global", "key1")
	if mem != nil {
		t.Error("expected nil after delete")
	}

	// Delete non-existent is no-op
	if err := ms.Delete("global", "nonexistent"); err != nil {
		t.Errorf("expected no error on delete of non-existent key, got %v", err)
	}
}
```

- [ ] **Step 2: Run test to confirm it fails**

```bash
cd /Users/tengyongzhi/work/theopenbee/openbee && go test ./internal/store/... -run TestMemoryStore -v
```

Expected: FAIL — `NewMemoryStore` undefined.

- [ ] **Step 3: Add migration v17 to `db.go`**

Add to the `migrations` slice in `internal/store/db.go`:

```go
{
	version: 17,
	name:    "20260318_create_bee_memories",
	sql: `CREATE TABLE IF NOT EXISTS bee_memories (
		id         TEXT PRIMARY KEY,
		scope      TEXT NOT NULL,
		key        TEXT NOT NULL,
		value      TEXT NOT NULL,
		created_at INTEGER NOT NULL,
		updated_at INTEGER NOT NULL,
		UNIQUE(scope, key)
	);`,
},
```

- [ ] **Step 4: Create `memory_store.go`**

Create `internal/store/memory_store.go`:

```go
package store

import (
	"database/sql"
	"time"

	"github.com/google/uuid"
)

type Memory struct {
	Key       string `json:"key"`
	Value     string `json:"value"`
	UpdatedAt int64  `json:"updated_at"`
}

type MemoryStore struct {
	db *sql.DB
}

func NewMemoryStore(db *sql.DB) *MemoryStore {
	return &MemoryStore{db: db}
}

func (s *MemoryStore) Save(scope, key, value string) error {
	now := time.Now().UnixMilli()
	_, err := s.db.Exec(
		`INSERT INTO bee_memories (id, scope, key, value, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(scope, key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
		uuid.New().String(), scope, key, value, now, now,
	)
	return err
}

func (s *MemoryStore) Get(scope, key string) (*Memory, error) {
	row := s.db.QueryRow(
		`SELECT key, value, updated_at FROM bee_memories WHERE scope = ? AND key = ?`,
		scope, key,
	)
	var m Memory
	err := row.Scan(&m.Key, &m.Value, &m.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func (s *MemoryStore) ListByScope(scope string, limit int) ([]Memory, error) {
	rows, err := s.db.Query(
		`SELECT key, value, updated_at FROM bee_memories WHERE scope = ? ORDER BY updated_at DESC LIMIT ?`,
		scope, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var memories []Memory
	for rows.Next() {
		var m Memory
		if err := rows.Scan(&m.Key, &m.Value, &m.UpdatedAt); err != nil {
			return nil, err
		}
		memories = append(memories, m)
	}
	return memories, rows.Err()
}

func (s *MemoryStore) Delete(scope, key string) error {
	_, err := s.db.Exec(
		`DELETE FROM bee_memories WHERE scope = ? AND key = ?`,
		scope, key,
	)
	return err
}
```

- [ ] **Step 5: Run tests to verify they pass**

```bash
cd /Users/tengyongzhi/work/theopenbee/openbee && go test ./internal/store/... -run TestMemoryStore -v
```

Expected: all 4 tests PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/store/db.go internal/store/memory_store.go internal/store/memory_store_test.go
git commit -m "feat: add bee_memories table and MemoryStore"
```

---

### Task 3: Add ExecutionStore query methods

**Files:**
- Modify: `internal/store/execution_store.go`
- Modify or create: `internal/store/execution_store_test.go`

- [ ] **Step 1: Write failing tests**

Add to `internal/store/execution_store_test.go` (create if needed):

```go
package store

import (
	"testing"
)

func TestExecutionStore_ListBeeExecutions(t *testing.T) {
	db, err := InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	es := NewExecutionStore(db)

	// Create a bee execution (worker_id = NULL)
	bee1, err := es.CreateBeeExecution("session1", "user said hello")
	if err != nil {
		t.Fatal(err)
	}
	_ = bee1

	// Create a worker execution (should not appear)
	db.Exec(`INSERT INTO workers (id, name, work_dir, status, created_at, updated_at) VALUES ('w1','test','/tmp','idle',0,0)`)
	_, err = es.Create("w1", "worker task", "session2")
	if err != nil {
		t.Fatal(err)
	}

	results, err := es.ListBeeExecutions(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 bee execution, got %d", len(results))
	}
}

func TestExecutionStore_GetLogsByID(t *testing.T) {
	db, err := InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	es := NewExecutionStore(db)
	exec, _ := es.CreateBeeExecution("session1", "test")

	// Update logs with multiline content
	logs := "line1\nline2\nline3\nline4\nline5"
	es.UpdateLogs(exec.ID, logs)

	// Get last 3 lines
	result, err := es.GetLogsByID(exec.ID)
	if err != nil {
		t.Fatal(err)
	}
	if result.Logs != logs {
		t.Errorf("expected full logs, got %q", result.Logs)
	}
}
```

- [ ] **Step 2: Run tests to confirm they fail**

```bash
cd /Users/tengyongzhi/work/theopenbee/openbee && go test ./internal/store/... -run "TestExecutionStore_ListBee|TestExecutionStore_GetLogs" -v
```

Expected: FAIL — methods not defined.

- [ ] **Step 3: Implement `ListBeeExecutions`, `ListRecent`, and `GetLogsByID`**

Add to `internal/store/execution_store.go`:

```go
// ListBeeExecutions returns the bee's own execution history (worker_id IS NULL).
func (s *ExecutionStore) ListBeeExecutions(limit int) ([]model.WorkerExecution, error) {
	rows, err := s.db.Query(
		execSelect+` WHERE e.worker_id IS NULL ORDER BY e.started_at DESC LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanExecutions(rows)
}

// ListRecent returns the most recent executions (all types).
func (s *ExecutionStore) ListRecent(limit int) ([]model.WorkerExecution, error) {
	rows, err := s.db.Query(
		execSelect+` ORDER BY e.started_at DESC LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanExecutions(rows)
}

// GetLogsByID returns an execution's metadata and full logs.
// Returns (nil, nil) if execution not found — this deliberately differs from
// GetByID which returns an error, because the caller (toolGetExecutionLogs)
// needs to distinguish "not found" from "DB error".
func (s *ExecutionStore) GetLogsByID(id string) (*model.WorkerExecution, error) {
	row := s.db.QueryRow(execSelect+` WHERE e.id = ?`, id)
	exec, err := scanExecution(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &exec, nil
}
```

> Note: Check if `scanExecutions` (plural) helper exists. If not, add it to iterate rows and call `scanExecution` per row. Also check if `scanExecution` accepts a `*sql.Row` — the existing pattern likely uses a scanner interface. Adapt to match the existing scan pattern in the file.

- [ ] **Step 4: Run tests to verify they pass**

```bash
cd /Users/tengyongzhi/work/theopenbee/openbee && go test ./internal/store/... -run "TestExecutionStore_ListBee|TestExecutionStore_GetLogs" -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/store/execution_store.go internal/store/execution_store_test.go
git commit -m "feat: add ListBeeExecutions and GetLogsByID to ExecutionStore"
```

---

### Task 4: Add TaskStore and WorkerStore query methods

**Files:**
- Modify: `internal/store/task_store.go`
- Modify: `internal/store/worker_store.go`
- Modify or create: test files

- [ ] **Step 1: Write failing test for `CountPendingByWorkerID`**

Add to `internal/store/task_store_test.go`:

```go
func TestTaskStore_CountPendingByWorkerID(t *testing.T) {
	ts, cleanup := newTaskStoreForTest(t)
	defer cleanup()

	ctx := context.Background()

	// Create a pending task (uses existing test worker and message fixtures)
	ts.Create(ctx, model.Task{
		MessageID:   testMessageID,  // use the constant from existing test fixtures
		WorkerID:    testWorkerID,   // use the constant from existing test fixtures
		Instruction: "do something",
		Type:        "immediate",
		Status:      "pending",
	})

	count, err := ts.CountPendingByWorkerID(ctx, testWorkerID)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("expected 1 pending task, got %d", count)
	}
}
```

> Note: Adapt test fixture IDs to match whatever `newTaskStoreForTest` uses.

- [ ] **Step 2: Write failing test for `CountByStatus` on WorkerStore**

Add to `internal/store/worker_store_test.go` (create if needed, use `package store`):

```go
func TestWorkerStore_CountByStatus(t *testing.T) {
	db, err := InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ws := NewWorkerStore(db)

	// Create workers with different statuses via direct insert
	db.Exec(`INSERT INTO workers (id,name,work_dir,status,created_at,updated_at) VALUES ('w1','a','/tmp','idle',0,0)`)
	db.Exec(`INSERT INTO workers (id,name,work_dir,status,created_at,updated_at) VALUES ('w2','b','/tmp','idle',0,0)`)
	db.Exec(`INSERT INTO workers (id,name,work_dir,status,created_at,updated_at) VALUES ('w3','c','/tmp','working',0,0)`)

	counts, err := ws.CountByStatus()
	if err != nil {
		t.Fatal(err)
	}
	if counts["idle"] != 2 || counts["working"] != 1 {
		t.Errorf("unexpected counts: %v", counts)
	}
}
```

- [ ] **Step 2b: Write failing test for `CountAllByStatus` on TaskStore**

Add to `internal/store/task_store_test.go`:

```go
func TestTaskStore_CountAllByStatus(t *testing.T) {
	ts, cleanup := newTaskStoreForTest(t)
	defer cleanup()

	ctx := context.Background()

	ts.Create(ctx, model.Task{
		MessageID: testMessageID, WorkerID: testWorkerID,
		Instruction: "task1", Type: "immediate", Status: "pending",
	})
	ts.Create(ctx, model.Task{
		MessageID: testMessageID, WorkerID: testWorkerID,
		Instruction: "task2", Type: "immediate", Status: "pending",
	})

	counts, err := ts.CountAllByStatus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if counts["pending"] != 2 {
		t.Errorf("expected 2 pending, got %d", counts["pending"])
	}
}
```

- [ ] **Step 3: Run tests to confirm they fail**

```bash
cd /Users/tengyongzhi/work/theopenbee/openbee && go test ./internal/store/... -run "TestTaskStore_CountPending|TestWorkerStore_CountByStatus" -v
```

Expected: FAIL.

- [ ] **Step 4: Implement `CountPendingByWorkerID`**

Add to `internal/store/task_store.go`:

```go
func (s *TaskStore) CountPendingByWorkerID(ctx context.Context, workerID string) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM tasks WHERE worker_id = ? AND status = 'pending'`,
		workerID,
	).Scan(&count)
	return count, err
}
```

- [ ] **Step 5: Implement `CountAllByStatus` on TaskStore**

Add to `internal/store/task_store.go`:

```go
func (s *TaskStore) CountAllByStatus(ctx context.Context) (map[string]int, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT status, COUNT(*) FROM tasks GROUP BY status`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	counts := make(map[string]int)
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return nil, err
		}
		counts[status] = count
	}
	return counts, rows.Err()
}
```

- [ ] **Step 6: Implement `CountByStatus` on WorkerStore**

Add to `internal/store/worker_store.go`:

```go
func (s *WorkerStore) CountByStatus() (map[string]int, error) {
	rows, err := s.db.Query(`SELECT status, COUNT(*) FROM workers GROUP BY status`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	counts := make(map[string]int)
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return nil, err
		}
		counts[status] = count
	}
	return counts, rows.Err()
}
```

- [ ] **Step 7: Run tests to verify they pass**

```bash
cd /Users/tengyongzhi/work/theopenbee/openbee && go test ./internal/store/... -run "TestTaskStore_CountPending|TestTaskStore_CountAll|TestWorkerStore_CountByStatus" -v
```

Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/store/task_store.go internal/store/task_store_test.go internal/store/worker_store.go internal/store/worker_store_test.go
git commit -m "feat: add CountPendingByWorkerID, CountAllByStatus, and CountByStatus store methods"
```

---

### Task 5: Add `liveLogs` buffer and `GetExecutionLogs` to worker Manager

**Files:**
- Modify: `internal/worker/manager.go`

- [ ] **Step 1: Add `liveLogSnapshots` field to Manager struct**

We use a snapshot approach to avoid data races: `monitorExecution` periodically writes a snapshot of the current logs under the write lock, and `GetExecutionLogs` reads the snapshot under a read lock. This avoids concurrent access to `strings.Builder`.

In the `Manager` struct definition, add:

```go
liveLogSnapshots map[string]string // execution_id -> latest log snapshot
```

Initialize it in `NewManager`:

```go
liveLogSnapshots: make(map[string]string),
```

- [ ] **Step 2: Populate `liveLogSnapshots` in `monitorExecution`**

In the `monitorExecution` method, after each write to `rawLogsBuilder` (inside the output loop where `rawLogsBuilder.WriteString(...)` is called), add a snapshot update:

```go
m.mu.Lock()
m.liveLogSnapshots[exec.ID] = rawLogsBuilder.String()
m.mu.Unlock()
```

At the end of `monitorExecution` (in the cleanup/defer section), clean up:

```go
m.mu.Lock()
delete(m.liveLogSnapshots, exec.ID)
m.mu.Unlock()
```

> Note: The snapshot is updated on every log write, which is acceptable since log writes are infrequent (one per Claude output event). The write lock is held only briefly for the map assignment.

- [ ] **Step 3: Add `GetExecutionLogs` method**

```go
// GetExecutionLogs returns the current logs for a running execution.
// Returns empty string if execution not found in memory.
func (m *Manager) GetExecutionLogs(executionID string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.liveLogSnapshots[executionID]
}
```

- [ ] **Step 4: Verify it compiles**

```bash
cd /Users/tengyongzhi/work/theopenbee/openbee && go build ./internal/worker/...
```

Expected: success.

- [ ] **Step 5: Commit**

```bash
git add internal/worker/manager.go
git commit -m "feat: add liveLogs buffer and GetExecutionLogs to worker Manager"
```

---

### Task 6: Update MCPServer dependencies

**Files:**
- Modify: `internal/mcp/server.go`
- Modify: `internal/app/app.go`

- [ ] **Step 1: Add `executionStore` and `memoryStore` to MCPServer struct**

In `internal/mcp/server.go`, add to the `MCPServer` struct:

```go
executionStore *store.ExecutionStore
memoryStore    *store.MemoryStore
```

- [ ] **Step 2: Update `NewServer` signature**

Add `es *store.ExecutionStore, ms *store.MemoryStore` parameters and assign them in the constructor.

- [ ] **Step 3: Update call site in `app.go`**

In `internal/app/app.go`, update the `mcp.NewServer(...)` call to pass `s.executionStore` (or however it's referenced) and a new `store.NewMemoryStore(db)`.

> Note: Check how `executionStore` is created in `app.go`. It may already exist on the stores struct. If `MemoryStore` needs to be created, add `memoryStore := store.NewMemoryStore(db)` before the `NewServer` call.

- [ ] **Step 4: Update test setup in `tools_test.go`**

Update `setupMCPServerWithMessaging` and any other `NewServer` call sites in tests to include the new parameters. Pass `store.NewExecutionStore(db)` and `store.NewMemoryStore(db)`.

- [ ] **Step 5: Verify everything compiles and existing tests pass**

```bash
cd /Users/tengyongzhi/work/theopenbee/openbee && go build ./... && go test ./internal/mcp/... -v
```

Expected: all existing tests PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/mcp/server.go internal/app/app.go internal/mcp/tools_test.go
git commit -m "feat: add ExecutionStore and MemoryStore dependencies to MCPServer"
```

---

### Task 7: Implement system status MCP tools

**Files:**
- Modify: `internal/mcp/tools.go`
- Modify: `internal/mcp/tools_test.go`

- [ ] **Step 1: Write failing tests for `get_execution_logs`**

Add to `internal/mcp/tools_test.go`:

```go
func TestCallTool_GetExecutionLogs(t *testing.T) {
	s := setupMCPServerWithMessaging(t)

	// Call with non-existent execution
	result, err := s.CallTool(toolnames.GetExecutionLogs, mustMarshal(t, map[string]any{
		"execution_id": "nonexistent",
	}))
	if err == nil {
		t.Fatal("expected error for non-existent execution")
	}
	_ = result
}
```

- [ ] **Step 2: Write failing tests for `get_worker_status`**

```go
func TestCallTool_GetWorkerStatus(t *testing.T) {
	s := setupMCPServerWithMessaging(t)

	// Create a worker first
	s.CallTool(toolnames.CreateWorker, mustMarshal(t, map[string]any{
		"name":        "status-test",
		"description": "test",
		"memory":      "test",
	}))

	// List to get the ID
	listResult, _ := s.CallTool(toolnames.ListWorkers, nil)
	// Parse worker ID from result, then call get_worker_status
	result, err := s.CallTool(toolnames.GetWorkerStatus, mustMarshal(t, map[string]any{
		"worker_id": extractWorkerID(t, listResult),
	}))
	if err != nil {
		t.Fatal(err)
	}
	_ = result
}
```

> Note: Adapt test helpers to match existing patterns. The key is to verify the tool returns valid JSON with expected fields.

- [ ] **Step 3: Write failing tests for `get_system_overview`**

```go
func TestCallTool_GetSystemOverview(t *testing.T) {
	s := setupMCPServerWithMessaging(t)

	result, err := s.CallTool(toolnames.GetSystemOverview, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Verify result contains workers and tasks sections
	_ = result
}
```

- [ ] **Step 4: Write failing tests for `list_bee_executions`**

```go
func TestCallTool_ListBeeExecutions(t *testing.T) {
	s := setupMCPServerWithMessaging(t)

	result, err := s.CallTool(toolnames.ListBeeExecutions, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Should return empty array initially
	_ = result
}
```

- [ ] **Step 5: Run tests to confirm they fail**

```bash
cd /Users/tengyongzhi/work/theopenbee/openbee && go test ./internal/mcp/... -run "TestCallTool_GetExecutionLogs|TestCallTool_GetWorkerStatus|TestCallTool_GetSystemOverview|TestCallTool_ListBeeExecutions" -v
```

Expected: FAIL.

- [ ] **Step 6: Add tool schemas for 4 status tools**

Add to `toolSchemas()` in `internal/mcp/tools.go`:

```go
{
	Name:        toolnames.GetExecutionLogs,
	Description: "查看某个执行记录的最新日志。返回执行的最后N行日志。",
	InputSchema: map[string]any{
		"type":     "object",
		"required": []string{"execution_id"},
		"properties": map[string]any{
			"execution_id": map[string]string{"type": "string", "description": "执行记录ID"},
			"tail":         map[string]string{"type": "integer", "description": "返回最后N行日志，默认50"},
		},
	},
},
{
	Name:        toolnames.GetWorkerStatus,
	Description: "查看员工的当前状态，包括是否在工作、正在执行什么任务、待处理任务数量。",
	InputSchema: map[string]any{
		"type":     "object",
		"required": []string{"worker_id"},
		"properties": map[string]any{
			"worker_id": map[string]string{"type": "string", "description": "员工ID"},
		},
	},
},
{
	Name:        toolnames.GetSystemOverview,
	Description: "查看系统整体概况：员工状态分布、任务状态统计、最近5条执行记录。",
	InputSchema: map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	},
},
{
	Name:        toolnames.ListBeeExecutions,
	Description: "查看 bee 自己的执行历史记录，用于自我反思和改进。",
	InputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"limit": map[string]string{"type": "integer", "description": "返回记录数量，默认10"},
		},
	},
},
```

- [ ] **Step 7: Add handler cases for 4 status tools in `callTool`**

Add to the `callTool` switch in `internal/mcp/tools.go`:

```go
case toolnames.GetExecutionLogs:
	return s.toolGetExecutionLogs(args)
case toolnames.GetWorkerStatus:
	return s.toolGetWorkerStatus(args)
case toolnames.GetSystemOverview:
	return s.toolGetSystemOverview(args)
case toolnames.ListBeeExecutions:
	return s.toolListBeeExecutions(args)
```

- [ ] **Step 8: Implement `toolGetExecutionLogs`**

```go
func (s *MCPServer) toolGetExecutionLogs(args json.RawMessage) (any, error) {
	var p struct {
		ExecutionID string `json:"execution_id"`
		Tail        int    `json:"tail"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}
	if p.ExecutionID == "" {
		return nil, fmt.Errorf("execution_id is required")
	}
	if p.Tail <= 0 {
		p.Tail = 50
	}

	// Try live logs first
	liveLogs := s.manager.GetExecutionLogs(p.ExecutionID)

	// Fall back to DB
	exec, err := s.executionStore.GetLogsByID(p.ExecutionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get execution: %w", err)
	}
	if exec == nil {
		return nil, fmt.Errorf("execution %s not found", p.ExecutionID)
	}

	logs := exec.Logs
	if liveLogs != "" {
		logs = liveLogs
	}

	// Tail N lines
	lines := strings.Split(logs, "\n")
	if len(lines) > p.Tail {
		lines = lines[len(lines)-p.Tail:]
	}

	return map[string]any{
		"execution_id": exec.ID,
		"worker_id":    exec.WorkerID,
		"status":       string(exec.Status),
		"logs":         strings.Join(lines, "\n"),
	}, nil
}
```

- [ ] **Step 9: Implement `toolGetWorkerStatus`**

```go
func (s *MCPServer) toolGetWorkerStatus(args json.RawMessage) (any, error) {
	var p struct {
		WorkerID string `json:"worker_id"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}
	if p.WorkerID == "" {
		return nil, fmt.Errorf("worker_id is required")
	}

	worker, err := s.workerStore.GetByID(p.WorkerID)
	if err != nil {
		return nil, fmt.Errorf("worker not found: %w", err)
	}

	result := map[string]any{
		"worker_id":         worker.ID,
		"name":              worker.Name,
		"status":            string(worker.Status),
		"current_execution": nil,
	}

	// Get running execution for this worker and find associated task_id
	execs, err := s.executionStore.ListByWorkerID(worker.ID)
	if err == nil {
		for _, e := range execs {
			if e.Status == "running" {
				execInfo := map[string]any{
					"id":          e.ID,
					"task_id":     nil,
					"instruction": e.TriggerInput,
					"started_at":  e.StartedAt,
				}
				// Find the task associated with this execution
				ctx := context.Background()
				tasks, terr := s.taskStore.ListByMessageID(ctx, "", "running", "")
				if terr == nil {
					for _, task := range tasks {
						if task.ExecutionID == e.ID {
							execInfo["task_id"] = task.ID
							break
						}
					}
				}
				result["current_execution"] = execInfo
				break
			}
		}
	}

	// Count pending tasks
	ctx := context.Background()
	pendingCount, err := s.taskStore.CountPendingByWorkerID(ctx, p.WorkerID)
	if err != nil {
		pendingCount = 0
	}
	result["pending_tasks_count"] = pendingCount

	return result, nil
}
```

- [ ] **Step 10: Implement `toolGetSystemOverview`**

```go
func (s *MCPServer) toolGetSystemOverview(args json.RawMessage) (any, error) {
	// Worker counts
	workerCounts, err := s.workerStore.CountByStatus()
	if err != nil {
		return nil, fmt.Errorf("failed to get worker counts: %w", err)
	}
	total := 0
	for _, c := range workerCounts {
		total += c
	}

	// Task counts
	ctx := context.Background()
	taskCounts, err := s.taskStore.CountAllByStatus(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get task counts: %w", err)
	}

	// Scheduled active count: count tasks with type=scheduled and status=pending
	// This requires a dedicated query since CountAllByStatus only groups by status
	scheduledActive := 0
	// TODO: If needed, add CountScheduledActive method. For now, this is approximated
	// by noting that scheduled tasks cycle through pending status.

	// Recent executions (all types, not just bee)
	recentExecs, _ := s.executionStore.ListRecent(5)

	recentList := make([]map[string]any, 0, len(recentExecs))
	for _, e := range recentExecs {
		recentList = append(recentList, map[string]any{
			"id":           e.ID,
			"worker_name":  e.WorkerName,
			"status":       string(e.Status),
			"started_at":   e.StartedAt,
			"completed_at": e.CompletedAt,
		})
	}

	return map[string]any{
		"workers": map[string]any{
			"total":   total,
			"idle":    workerCounts["idle"],
			"working": workerCounts["working"],
			"error":   workerCounts["error"],
		},
		"tasks": map[string]any{
			"pending":          taskCounts["pending"],
			"running":          taskCounts["running"],
			"completed":        taskCounts["completed"],
			"failed":           taskCounts["failed"],
			"cancelled":        taskCounts["cancelled"],
			"scheduled_active": scheduledActive,
		},
		"recent_executions": recentList,
	}, nil
}
```

> Note: The implementation uses `CountAllByStatus()` and `ListRecent()` methods added in earlier tasks. The `scheduled_active` count may need a dedicated `CountScheduledActive` method on TaskStore if precise tracking of scheduled-type pending tasks is needed — add it during implementation if the TODO above needs resolution.

- [ ] **Step 11: Implement `toolListBeeExecutions`**

```go
func (s *MCPServer) toolListBeeExecutions(args json.RawMessage) (any, error) {
	var p struct {
		Limit int `json:"limit"`
	}
	if args != nil {
		json.Unmarshal(args, &p)
	}
	if p.Limit <= 0 {
		p.Limit = 10
	}

	execs, err := s.executionStore.ListBeeExecutions(p.Limit)
	if err != nil {
		return nil, fmt.Errorf("failed to list bee executions: %w", err)
	}

	results := make([]map[string]any, 0, len(execs))
	for _, e := range execs {
		triggerInput := e.TriggerInput
		if len(triggerInput) > 200 {
			triggerInput = triggerInput[:200]
		}
		result := e.Result
		if len(result) > 200 {
			result = result[:200]
		}
		results = append(results, map[string]any{
			"id":            e.ID,
			"trigger_input": triggerInput,
			"status":        string(e.Status),
			"started_at":    e.StartedAt,
			"completed_at":  e.CompletedAt,
			"result":        result,
		})
	}

	return results, nil
}
```

- [ ] **Step 12: Run tests to verify they pass**

```bash
cd /Users/tengyongzhi/work/theopenbee/openbee && go test ./internal/mcp/... -run "TestCallTool_GetExecutionLogs|TestCallTool_GetWorkerStatus|TestCallTool_GetSystemOverview|TestCallTool_ListBeeExecutions" -v
```

Expected: PASS.

- [ ] **Step 13: Commit**

```bash
git add internal/mcp/tools.go internal/mcp/tools_test.go
git commit -m "feat: implement system status MCP tools (get_execution_logs, get_worker_status, get_system_overview, list_bee_executions)"
```

---

### Task 8: Implement memory MCP tools

**Files:**
- Modify: `internal/mcp/tools.go`
- Modify: `internal/mcp/tools_test.go`

- [ ] **Step 1: Write failing tests for memory tools**

Add to `internal/mcp/tools_test.go`:

```go
func TestCallTool_SaveMemory(t *testing.T) {
	s := setupMCPServerWithMessaging(t)

	result, err := s.CallTool(toolnames.SaveMemory, mustMarshal(t, map[string]any{
		"scope": "global",
		"key":   "test_pref",
		"value": "user likes concise replies",
	}))
	if err != nil {
		t.Fatal(err)
	}
	_ = result
}

func TestCallTool_GetMemory(t *testing.T) {
	s := setupMCPServerWithMessaging(t)

	// Save first
	s.CallTool(toolnames.SaveMemory, mustMarshal(t, map[string]any{
		"scope": "global",
		"key":   "pref1",
		"value": "value1",
	}))

	// Get by key
	result, err := s.CallTool(toolnames.GetMemory, mustMarshal(t, map[string]any{
		"scope": "global",
		"key":   "pref1",
	}))
	if err != nil {
		t.Fatal(err)
	}
	_ = result

	// List by scope (no key)
	result2, err := s.CallTool(toolnames.GetMemory, mustMarshal(t, map[string]any{
		"scope": "global",
	}))
	if err != nil {
		t.Fatal(err)
	}
	_ = result2
}

func TestCallTool_DeleteMemory(t *testing.T) {
	s := setupMCPServerWithMessaging(t)

	s.CallTool(toolnames.SaveMemory, mustMarshal(t, map[string]any{
		"scope": "global",
		"key":   "to_delete",
		"value": "temp",
	}))

	result, err := s.CallTool(toolnames.DeleteMemory, mustMarshal(t, map[string]any{
		"scope": "global",
		"key":   "to_delete",
	}))
	if err != nil {
		t.Fatal(err)
	}
	_ = result

	// Verify deleted
	getResult, _ := s.CallTool(toolnames.GetMemory, mustMarshal(t, map[string]any{
		"scope": "global",
		"key":   "to_delete",
	}))
	_ = getResult
}
```

- [ ] **Step 2: Run tests to confirm they fail**

```bash
cd /Users/tengyongzhi/work/theopenbee/openbee && go test ./internal/mcp/... -run "TestCallTool_SaveMemory|TestCallTool_GetMemory|TestCallTool_DeleteMemory" -v
```

Expected: FAIL.

- [ ] **Step 3: Add tool schemas for 3 memory tools**

Add to `toolSchemas()`:

```go
{
	Name:        toolnames.SaveMemory,
	Description: "保存或更新一条记忆。scope 为 'global' 表示全局经验，或传入 session_key 表示特定用户的偏好。",
	InputSchema: map[string]any{
		"type":     "object",
		"required": []string{"scope", "key", "value"},
		"properties": map[string]any{
			"scope": map[string]string{"type": "string", "description": "记忆范围：'global' 或 session_key"},
			"key":   map[string]string{"type": "string", "description": "记忆标识符，如 'user_language_preference'"},
			"value": map[string]string{"type": "string", "description": "记忆内容"},
		},
	},
},
{
	Name:        toolnames.GetMemory,
	Description: "读取记忆。传入 key 返回单条记忆，不传 key 返回该 scope 下所有记忆（最多50条）。",
	InputSchema: map[string]any{
		"type":     "object",
		"required": []string{"scope"},
		"properties": map[string]any{
			"scope": map[string]string{"type": "string", "description": "记忆范围：'global' 或 session_key"},
			"key":   map[string]string{"type": "string", "description": "记忆标识符（可选，不传则返回该范围下所有记忆）"},
		},
	},
},
{
	Name:        toolnames.DeleteMemory,
	Description: "删除一条记忆。删除不存在的记忆不会报错。",
	InputSchema: map[string]any{
		"type":     "object",
		"required": []string{"scope", "key"},
		"properties": map[string]any{
			"scope": map[string]string{"type": "string", "description": "记忆范围"},
			"key":   map[string]string{"type": "string", "description": "记忆标识符"},
		},
	},
},
```

- [ ] **Step 4: Add handler cases in `callTool`**

```go
case toolnames.SaveMemory:
	return s.toolSaveMemory(args)
case toolnames.GetMemory:
	return s.toolGetMemory(args)
case toolnames.DeleteMemory:
	return s.toolDeleteMemory(args)
```

- [ ] **Step 5: Implement memory tool handlers**

```go
func (s *MCPServer) toolSaveMemory(args json.RawMessage) (any, error) {
	var p struct {
		Scope string `json:"scope"`
		Key   string `json:"key"`
		Value string `json:"value"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}
	if p.Scope == "" || p.Key == "" || p.Value == "" {
		return nil, fmt.Errorf("scope, key, and value are required")
	}
	if err := s.memoryStore.Save(p.Scope, p.Key, p.Value); err != nil {
		return nil, fmt.Errorf("failed to save memory: %w", err)
	}
	return map[string]string{"status": "saved"}, nil
}

func (s *MCPServer) toolGetMemory(args json.RawMessage) (any, error) {
	var p struct {
		Scope string `json:"scope"`
		Key   string `json:"key"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}
	if p.Scope == "" {
		return nil, fmt.Errorf("scope is required")
	}
	if p.Key != "" {
		mem, err := s.memoryStore.Get(p.Scope, p.Key)
		if err != nil {
			return nil, fmt.Errorf("failed to get memory: %w", err)
		}
		if mem == nil {
			return nil, nil
		}
		return mem, nil
	}
	memories, err := s.memoryStore.ListByScope(p.Scope, 50)
	if err != nil {
		return nil, fmt.Errorf("failed to list memories: %w", err)
	}
	return memories, nil
}

func (s *MCPServer) toolDeleteMemory(args json.RawMessage) (any, error) {
	var p struct {
		Scope string `json:"scope"`
		Key   string `json:"key"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}
	if p.Scope == "" || p.Key == "" {
		return nil, fmt.Errorf("scope and key are required")
	}
	if err := s.memoryStore.Delete(p.Scope, p.Key); err != nil {
		return nil, fmt.Errorf("failed to delete memory: %w", err)
	}
	return map[string]string{"status": "deleted"}, nil
}
```

- [ ] **Step 6: Run tests to verify they pass**

```bash
cd /Users/tengyongzhi/work/theopenbee/openbee && go test ./internal/mcp/... -run "TestCallTool_SaveMemory|TestCallTool_GetMemory|TestCallTool_DeleteMemory" -v
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/mcp/tools.go internal/mcp/tools_test.go
git commit -m "feat: implement memory MCP tools (save_memory, get_memory, delete_memory)"
```

---

### Task 9: Update bee system rules with memory guidelines

**Files:**
- Modify: `internal/claudemd/claudemd.go`

- [ ] **Step 1: Add memory usage section to `beeRules()`**

In `internal/claudemd/claudemd.go`, at the end of the `beeRules()` function return string, append:

```go
## 记忆管理

你拥有持久化记忆系统，可以跨会话积累经验和记住用户偏好。

### 记忆工具
- ` + toolnames.SaveMemory + ` - 保存或更新记忆
- ` + toolnames.GetMemory + ` - 读取记忆
- ` + toolnames.DeleteMemory + ` - 删除记忆

### 使用规则
- 处理消息前，先加载相关记忆：
  - get_memory(scope=当前session_key) 获取该用户的偏好
  - get_memory(scope="global") 获取全局经验
- 发现用户偏好时，主动用 save_memory 保存
- 反思时将结论存为 global 记忆
- 使用描述性的 key，如 "user_language_preference"、"task_assignment_insight"

## 系统状态查看

你可以查看系统运行状态，以便更好地做出决策。

### 状态工具
- ` + toolnames.GetExecutionLogs + ` - 查看执行日志
- ` + toolnames.GetWorkerStatus + ` - 查看员工状态
- ` + toolnames.GetSystemOverview + ` - 系统整体概况
- ` + toolnames.ListBeeExecutions + ` - 查看自己的执行历史

### 使用场景
- 用户询问任务状态时，用 get_worker_status 或 get_system_overview 查看
- 需要自我反思时，用 list_bee_executions 回顾历史，用 get_execution_logs 查看详情
- 分配任务前，可先查看 get_system_overview 了解各员工负载
```

- [ ] **Step 2: Verify it compiles**

```bash
cd /Users/tengyongzhi/work/theopenbee/openbee && go build ./internal/claudemd/...
```

Expected: success.

- [ ] **Step 3: Commit**

```bash
git add internal/claudemd/claudemd.go
git commit -m "feat: add memory and system status guidelines to bee system rules"
```

---

### Task 10: Full integration test and verification

**Files:**
- No new files — verification only

- [ ] **Step 1: Run all tests**

```bash
cd /Users/tengyongzhi/work/theopenbee/openbee && go test ./... -v
```

Expected: all tests PASS.

- [ ] **Step 2: Verify tool count**

The `TestToolSchemas` test (if it checks tool count) will need updating. The total should now be 18 tools (11 existing + 7 new).

```bash
cd /Users/tengyongzhi/work/theopenbee/openbee && go test ./internal/mcp/... -run TestToolSchemas -v
```

If it fails due to count mismatch, update the expected count in the test.

- [ ] **Step 3: Build the full binary**

```bash
cd /Users/tengyongzhi/work/theopenbee/openbee && go build ./cmd/...
```

Expected: success.

- [ ] **Step 4: Commit any remaining fixes**

```bash
git add internal/mcp/tools_test.go && git commit -m "fix: update test expectations for new tool count"
```

Only if step 2 required changes.
