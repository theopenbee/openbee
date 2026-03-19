# list_tasks Worker ID Filter Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `worker_id` as a new optional filter for `list_tasks`, enabling cross-session queries of all tasks belonging to a specific worker.

**Architecture:** Introduce a `TaskFilter` struct in the store layer and a unified `List` method that dynamically builds SQL based on any combination of `message_id`, `session_key`, and/or `worker_id`. Update `toolListTasks` in the MCP layer to accept `worker_id`, relax the existing mutual-exclusivity constraint to "at least one of the three fields required", and route all calls through `TaskFilter`/`List`.

**Tech Stack:** Go, SQLite (via `database/sql`), existing `appendCSVFilter` helper pattern

---

## File Map

| File | Change |
|------|--------|
| `internal/store/task_store.go` | Add `TaskFilter` struct + `List` method |
| `internal/store/task_store_test.go` | Add tests for `List` with worker_id scenarios |
| `internal/mcp/tools.go` | Update `toolListTasks`: params, validation, schema, call `List` |

---

### Task 1: Add `TaskFilter` + `List` to the store layer (TDD)

**Files:**
- Modify: `internal/store/task_store.go` — add `TaskFilter` struct and `List` method
- Modify: `internal/store/task_store_test.go` — add tests

---

- [ ] **Step 1.1: Add a test helper with two workers and two sessions**

Open `internal/store/task_store_test.go` and add this helper function after `newTaskStoreWithTwoSessions`:

```go
// newTaskStoreWithTwoWorkers sets up: w1 and w2 workers; m1 (session-A) and m2 (session-B) messages.
func newTaskStoreWithTwoWorkers(t *testing.T) (*TaskStore, func()) {
	t.Helper()
	db, err := InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	db.Exec(`INSERT INTO bee_workers (id,name,work_dir,status,created_at,updated_at) VALUES ('w1','W1','/','idle',1,1)`)
	db.Exec(`INSERT INTO bee_workers (id,name,work_dir,status,created_at,updated_at) VALUES ('w2','W2','/','idle',1,1)`)
	db.Exec(`INSERT INTO bee_platform_messages
		(id, session_key, platform, content, raw, platform_msg_id, received_at, created_at, updated_at)
		VALUES ('m1','session-A','feishu','hi','','',1,1,1)`)
	db.Exec(`INSERT INTO bee_platform_messages
		(id, session_key, platform, content, raw, platform_msg_id, received_at, created_at, updated_at)
		VALUES ('m2','session-B','feishu','bye','','',1,1,1)`)
	return NewTaskStore(db), func() { db.Close() }
}
```

---

- [ ] **Step 1.2: Write the failing tests for `List`**

Add the following three test functions to `internal/store/task_store_test.go`:

```go
func TestTaskStore_List_ByWorkerID(t *testing.T) {
	ts, cleanup := newTaskStoreWithTwoWorkers(t)
	defer cleanup()
	ctx := context.Background()
	now := time.Now().UnixMilli()

	// w1 has tasks in session-A and session-B; w2 has a task in session-A
	ts.Create(ctx, model.Task{MessageID: "m1", WorkerID: "w1", Instruction: "w1-sessA", Type: model.TaskTypeImmediate, Status: model.TaskStatusPending, CreatedAt: now, UpdatedAt: now})
	ts.Create(ctx, model.Task{MessageID: "m2", WorkerID: "w1", Instruction: "w1-sessB", Type: model.TaskTypeImmediate, Status: model.TaskStatusCompleted, CreatedAt: now, UpdatedAt: now})
	ts.Create(ctx, model.Task{MessageID: "m1", WorkerID: "w2", Instruction: "w2-sessA", Type: model.TaskTypeImmediate, Status: model.TaskStatusPending, CreatedAt: now, UpdatedAt: now})

	tasks, err := ts.List(ctx, TaskFilter{WorkerID: "w1"})
	if err != nil {
		t.Fatalf("List by worker_id: %v", err)
	}
	if len(tasks) != 2 {
		t.Errorf("expected 2 tasks for w1 across sessions, got %d", len(tasks))
	}
	for _, task := range tasks {
		if task.WorkerID != "w1" {
			t.Errorf("expected all tasks to belong to w1, got worker_id=%q", task.WorkerID)
		}
	}
}

func TestTaskStore_List_ByWorkerIDAndSessionKey(t *testing.T) {
	ts, cleanup := newTaskStoreWithTwoWorkers(t)
	defer cleanup()
	ctx := context.Background()
	now := time.Now().UnixMilli()

	ts.Create(ctx, model.Task{MessageID: "m1", WorkerID: "w1", Instruction: "w1-sessA", Type: model.TaskTypeImmediate, Status: model.TaskStatusPending, CreatedAt: now, UpdatedAt: now})
	ts.Create(ctx, model.Task{MessageID: "m2", WorkerID: "w1", Instruction: "w1-sessB", Type: model.TaskTypeImmediate, Status: model.TaskStatusPending, CreatedAt: now, UpdatedAt: now})
	ts.Create(ctx, model.Task{MessageID: "m1", WorkerID: "w2", Instruction: "w2-sessA", Type: model.TaskTypeImmediate, Status: model.TaskStatusPending, CreatedAt: now, UpdatedAt: now})

	// w1 + session-A: should return only the 1 task that is both w1 AND in session-A
	tasks, err := ts.List(ctx, TaskFilter{WorkerID: "w1", SessionKey: "session-A"})
	if err != nil {
		t.Fatalf("List by worker_id+session_key: %v", err)
	}
	if len(tasks) != 1 {
		t.Errorf("expected 1 task for w1 in session-A, got %d", len(tasks))
	}
	if len(tasks) == 1 && tasks[0].Instruction != "w1-sessA" {
		t.Errorf("expected instruction 'w1-sessA', got %q", tasks[0].Instruction)
	}
}

func TestTaskStore_List_ByWorkerIDAndStatus(t *testing.T) {
	ts, cleanup := newTaskStoreWithTwoWorkers(t)
	defer cleanup()
	ctx := context.Background()
	now := time.Now().UnixMilli()

	ts.Create(ctx, model.Task{MessageID: "m1", WorkerID: "w1", Instruction: "pending", Type: model.TaskTypeImmediate, Status: model.TaskStatusPending, CreatedAt: now, UpdatedAt: now})
	ts.Create(ctx, model.Task{MessageID: "m2", WorkerID: "w1", Instruction: "completed", Type: model.TaskTypeImmediate, Status: model.TaskStatusCompleted, CreatedAt: now, UpdatedAt: now})
	ts.Create(ctx, model.Task{MessageID: "m1", WorkerID: "w2", Instruction: "w2-pending", Type: model.TaskTypeImmediate, Status: model.TaskStatusPending, CreatedAt: now, UpdatedAt: now})

	tasks, err := ts.List(ctx, TaskFilter{WorkerID: "w1", Status: "pending"})
	if err != nil {
		t.Fatalf("List by worker_id+status: %v", err)
	}
	if len(tasks) != 1 {
		t.Errorf("expected 1 pending task for w1, got %d", len(tasks))
	}
}
```

---

- [ ] **Step 1.3: Run tests to confirm they fail**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee
go test ./internal/store/... -run "TestTaskStore_List_" -v
```

Expected: compile error `ts.List undefined` or similar — confirms tests are genuinely new.

---

- [ ] **Step 1.4: Add `TaskFilter` struct and `List` method to `task_store.go`**

In `internal/store/task_store.go`, insert the following immediately after the closing brace of `appendCSVFilter` (after line 68), before the `ListByMessageID` function:

```go
// TaskFilter specifies filtering criteria for List.
// message_id and session_key are mutually exclusive.
// At least one of message_id, session_key, or worker_id must be non-empty.
type TaskFilter struct {
	MessageID  string
	SessionKey string
	WorkerID   string
	Status     string // comma-separated, e.g. "pending,running"
	Type       string // comma-separated, e.g. "immediate,countdown"
}

// List returns tasks matching the given filter. If session_key is set, tasks are
// joined with bee_platform_messages to resolve the session. Results are ordered
// by created_at DESC.
func (s *TaskStore) List(ctx context.Context, f TaskFilter) ([]model.Task, error) {
	q := `SELECT t.id, t.message_id, t.worker_id, t.instruction, t.type, t.status,
	             t.scheduled_at, t.cron_expr, t.next_run_at, t.execution_id,
	             t.created_at, t.updated_at
	      FROM bee_tasks t`
	if f.SessionKey != "" {
		q += ` JOIN bee_platform_messages pm ON t.message_id = pm.id`
	}
	q += ` WHERE 1=1`
	var args []any
	if f.MessageID != "" {
		q += ` AND t.message_id = ?`
		args = append(args, f.MessageID)
	}
	if f.SessionKey != "" {
		q += ` AND pm.session_key = ?`
		args = append(args, f.SessionKey)
	}
	if f.WorkerID != "" {
		q += ` AND t.worker_id = ?`
		args = append(args, f.WorkerID)
	}
	q, args = appendCSVFilter(q, args, "status", f.Status)
	q, args = appendCSVFilter(q, args, "type", f.Type)
	q += ` ORDER BY t.created_at DESC`
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}
	defer rows.Close()
	return scanTasks(rows)
}
```

---

- [ ] **Step 1.5: Run tests to confirm they pass**

```bash
go test ./internal/store/... -run "TestTaskStore_List_" -v
```

Expected: all three `TestTaskStore_List_*` tests PASS.

---

- [ ] **Step 1.6: Run the full store test suite**

```bash
go test ./internal/store/... -v
```

Expected: all tests pass, no regressions.

---

- [ ] **Step 1.7: Commit**

```bash
git add internal/store/task_store.go internal/store/task_store_test.go
git commit -m "feat(store): add TaskFilter struct and List method for flexible task querying"
```

---

### Task 2: Update `toolListTasks` in the MCP layer

**Files:**
- Modify: `internal/mcp/tools.go` — update params struct, validation, schema, and call site

---

- [ ] **Step 2.1: Add `store` import to `tools.go`**

In `internal/mcp/tools.go`, add `"github.com/theopenbee/openbee/internal/store"` to the import block. The existing imports in `tools.go` are:

```go
import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
	"go.uber.org/zap"
	"github.com/theopenbee/openbee/internal/model"
	"github.com/theopenbee/openbee/internal/platform"
	"github.com/theopenbee/openbee/internal/toolnames"
)
```

Add the new import line:

```go
import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
	"go.uber.org/zap"
	"github.com/theopenbee/openbee/internal/model"
	"github.com/theopenbee/openbee/internal/platform"
	"github.com/theopenbee/openbee/internal/store"
	"github.com/theopenbee/openbee/internal/toolnames"
)
```

---

- [ ] **Step 2.2: Update the `list_tasks` tool schema definition**

In `internal/mcp/tools.go`, find the `list_tasks` tool schema block (around line 107–119). Replace it:

**Old:**
```go
{
    Name:        toolnames.ListTasks,
    Description: "List tasks filtered by message_id or session_key (mutually exclusive), optionally filtered by status and/or type",
    InputSchema: map[string]any{
        "type": "object",
        "properties": map[string]any{
            "message_id":  map[string]string{"type": "string", "description": "Filter by message ID"},
            "session_key": map[string]string{"type": "string", "description": "Filter by session key (mutually exclusive with message_id)"},
            "status":      map[string]string{"type": "string", "description": "Optional status filter, supports comma-separated values e.g. 'pending,running'"},
            "type":        map[string]string{"type": "string", "description": "Optional type filter, supports comma-separated values e.g. 'scheduled' or 'immediate,countdown'"},
        },
    },
},
```

**New:**
```go
{
    Name:        toolnames.ListTasks,
    Description: "List tasks filtered by message_id, session_key, and/or worker_id. message_id and session_key are mutually exclusive; at least one of message_id, session_key, or worker_id is required.",
    InputSchema: map[string]any{
        "type": "object",
        "properties": map[string]any{
            "message_id":  map[string]string{"type": "string", "description": "Filter by message ID (mutually exclusive with session_key)"},
            "session_key": map[string]string{"type": "string", "description": "Filter by session key (mutually exclusive with message_id)"},
            "worker_id":   map[string]string{"type": "string", "description": "Filter by worker ID across all sessions; can be combined with session_key"},
            "status":      map[string]string{"type": "string", "description": "Optional status filter, supports comma-separated values e.g. 'pending,running'"},
            "type":        map[string]string{"type": "string", "description": "Optional type filter, supports comma-separated values e.g. 'scheduled' or 'immediate,countdown'"},
        },
    },
},
```

---

- [ ] **Step 2.3: Update `toolListTasks` function body**

In `internal/mcp/tools.go`, replace the entire `toolListTasks` function (lines 472–503):

**Old:**
```go
func (s *MCPServer) toolListTasks(args json.RawMessage) (any, error) {
	var params struct {
		MessageID  string `json:"message_id"`
		SessionKey string `json:"session_key"`
		Status     string `json:"status"`
		Type       string `json:"type"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, fmt.Errorf("invalid args: %w", err)
	}
	if params.MessageID != "" && params.SessionKey != "" {
		return nil, fmt.Errorf("message_id and session_key are mutually exclusive")
	}
	if params.MessageID == "" && params.SessionKey == "" {
		return nil, fmt.Errorf("either message_id or session_key is required")
	}

	var tasks []model.Task
	var err error
	if params.SessionKey != "" {
		tasks, err = s.taskStore.ListBySessionKey(context.Background(), params.SessionKey, params.Status, params.Type)
	} else {
		tasks, err = s.taskStore.ListByMessageID(context.Background(), params.MessageID, params.Status, params.Type)
	}
	if err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}
	if tasks == nil {
		tasks = []model.Task{}
	}
	return tasks, nil
}
```

**New:**
```go
func (s *MCPServer) toolListTasks(args json.RawMessage) (any, error) {
	var params struct {
		MessageID  string `json:"message_id"`
		SessionKey string `json:"session_key"`
		WorkerID   string `json:"worker_id"`
		Status     string `json:"status"`
		Type       string `json:"type"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, fmt.Errorf("invalid args: %w", err)
	}
	if params.MessageID != "" && params.SessionKey != "" {
		return nil, fmt.Errorf("message_id and session_key are mutually exclusive")
	}
	if params.MessageID == "" && params.SessionKey == "" && params.WorkerID == "" {
		return nil, fmt.Errorf("at least one of message_id, session_key, or worker_id is required")
	}
	tasks, err := s.taskStore.List(context.Background(), store.TaskFilter{
		MessageID:  params.MessageID,
		SessionKey: params.SessionKey,
		WorkerID:   params.WorkerID,
		Status:     params.Status,
		Type:       params.Type,
	})
	if err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}
	if tasks == nil {
		tasks = []model.Task{}
	}
	return tasks, nil
}
```

---

- [ ] **Step 2.4: Build to verify compilation**

```bash
go build ./...
```

Expected: no errors.

---

- [ ] **Step 2.5: Run full test suite**

```bash
go test ./...
```

Expected: all tests pass.

---

- [ ] **Step 2.6: Commit**

```bash
git add internal/mcp/tools.go
git commit -m "feat(mcp): add worker_id filter to list_tasks, support cross-session queries"
```

---

## Done

After both tasks complete, `list_tasks` will support:

| Parameters | Behaviour |
|-----------|-----------|
| `worker_id=X` | All tasks for worker X across all sessions |
| `worker_id=X, session_key=Y` | Tasks for worker X within session Y only |
| `worker_id=X, status=pending` | Pending tasks for worker X across all sessions |
| `session_key=Y` | All tasks in session Y (existing behaviour, unchanged) |
| `message_id=Z` | All tasks for message Z (existing behaviour, unchanged) |
