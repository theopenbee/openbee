# Remove `mark_task_complete` Tool Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove the `mark_task_complete` MCP tool and have the task dispatcher set task completion status from the worker process exit code instead.

**Architecture:** Add `CompleteTask` to `store.TaskStore` (mirrors existing `FailTask`, handles scheduled/regular branching). Expand the dispatcher's `TaskStore` interface with `CompleteTask` and call it when execution exits with code 0. Strip `mark_task_complete` from MCP tools, toolnames, and the worker system prompt.

**Tech Stack:** Go, SQLite (`database/sql`), `go test`

---

## File Map

| File | Change |
|---|---|
| `internal/store/task_store.go` | Add `CompleteTask` method |
| `internal/store/task_store_test.go` | Add `TestTaskStore_CompleteTask_*` tests |
| `internal/task_dispatcher/dispatcher.go` | Add `CompleteTask` to `TaskStore` interface; call it in `waitForResult` |
| `internal/task_dispatcher/dispatcher_test.go` | Add `CompleteTask` to `mockTaskStore`; add test for completion path |
| `internal/mcp/tools.go` | Remove tool definition, handler, allowlist entry, dispatch case |
| `internal/mcp/tools_test.go` | Remove `mark_task_complete` tests; update tool schema test |
| `internal/toolnames/toolnames.go` | Remove `MarkTaskComplete` constant |
| `internal/claudemd/worker.go` | Remove `mark_task_complete` from system prompt strings |
| `internal/claudemd/claudemd_test.go` | Remove assertions that check for `mark_task_complete` |

---

### Task 1: Add `CompleteTask` to `store.TaskStore`

**Files:**
- Modify: `internal/store/task_store.go`
- Test: `internal/store/task_store_test.go`

- [ ] **Step 1: Write failing tests**

Add to `internal/store/task_store_test.go` (after existing tests):

```go
func TestTaskStore_CompleteTask_Regular(t *testing.T) {
	ts, cleanup := newTaskStoreForTest(t)
	defer cleanup()

	ctx := context.Background()
	id, err := ts.Create(ctx, model.Task{
		MessageID:   "m1",
		WorkerID:    "w1",
		Instruction: "do it",
		Type:        model.TaskTypeImmediate,
		Status:      model.TaskStatusRunning,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := ts.CompleteTask(ctx, id); err != nil {
		t.Fatalf("CompleteTask: %v", err)
	}

	got, err := ts.GetByID(ctx, id)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Status != model.TaskStatusCompleted {
		t.Errorf("want status %q got %q", model.TaskStatusCompleted, got.Status)
	}
}

func TestTaskStore_CompleteTask_Scheduled(t *testing.T) {
	ts, cleanup := newTaskStoreForTest(t)
	defer cleanup()

	ctx := context.Background()
	cronExpr := "0 * * * *"
	id, err := ts.Create(ctx, model.Task{
		MessageID:   "m1",
		WorkerID:    "w1",
		Instruction: "do it",
		Type:        model.TaskTypeScheduled,
		Status:      model.TaskStatusRunning,
		CronExpr:    cronExpr,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := ts.CompleteTask(ctx, id); err != nil {
		t.Fatalf("CompleteTask: %v", err)
	}

	got, err := ts.GetByID(ctx, id)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	// Scheduled tasks reset to pending for the next cron run.
	if got.Status != model.TaskStatusPending {
		t.Errorf("want status %q got %q", model.TaskStatusPending, got.Status)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```
cd /path/to/repo && go test ./internal/store/... -run TestTaskStore_CompleteTask -v
```

Expected: `FAIL` — `ts.CompleteTask undefined`

- [ ] **Step 3: Implement `CompleteTask`**

Add after `FailTask` in `internal/store/task_store.go`:

```go
// CompleteTask marks a task as completed on successful worker exit.
// For scheduled tasks with a cron expression, it resets to pending instead
// so the task is picked up again on the next scheduled run.
func (s *TaskStore) CompleteTask(ctx context.Context, taskID string) error {
	task, err := s.GetByID(ctx, taskID)
	if err != nil {
		return fmt.Errorf("get task: %w", err)
	}
	if task.Type == model.TaskTypeScheduled && task.CronExpr != "" {
		_, err := s.CompleteScheduledTask(ctx, taskID)
		return err
	}
	return s.UpdateStatus(ctx, taskID, model.TaskStatusCompleted)
}
```

- [ ] **Step 4: Run tests to verify they pass**

```
go test ./internal/store/... -run TestTaskStore_CompleteTask -v
```

Expected: `PASS`

- [ ] **Step 5: Run full store tests to check for regressions**

```
go test ./internal/store/... -v
```

Expected: all `PASS`

- [ ] **Step 6: Commit**

```bash
git add internal/store/task_store.go internal/store/task_store_test.go
git commit -m "feat(store): add CompleteTask method to TaskStore"
```

---

### Task 2: Update dispatcher to call `CompleteTask` on successful exit

**Files:**
- Modify: `internal/task_dispatcher/dispatcher.go`
- Modify: `internal/task_dispatcher/dispatcher_test.go`

- [ ] **Step 1: Write failing test**

In `internal/task_dispatcher/dispatcher_test.go`, add `CompleteTask` to `mockTaskStore` and add a completion test.

First, update `mockTaskStore` (find the existing struct around line 46):

```go
type mockTaskStore struct {
	mu             sync.Mutex
	failedTasks    []string
	completedTasks []string
}

func (s *mockTaskStore) SetExecution(_ context.Context, _, _, _ string) error { return nil }
func (s *mockTaskStore) FailTask(_ context.Context, taskID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failedTasks = append(s.failedTasks, taskID)
	return nil
}
func (s *mockTaskStore) CompleteTask(_ context.Context, taskID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.completedTasks = append(s.completedTasks, taskID)
	return nil
}
func (s *mockTaskStore) CancelTask(_ context.Context, taskID string) error { return nil }
```

Then add this test at the end of the file:

```go
func TestDispatcher_CompleteTask_OnSuccessfulExit(t *testing.T) {
	mgr := &mockExecManager{execResult: model.WorkerExecution{ID: "exec-1", Status: model.ExecStatusRunning}}
	ts := &mockTaskStore{}
	execStore := &mockExecutionQuerier{result: model.WorkerExecution{
		ID:     "exec-1",
		Status: model.ExecStatusCompleted,
	}}
	ss := newMockSessionStore()

	ch := make(chan task_dispatcher.DispatchTask, 1)
	d := task_dispatcher.New(mgr, ts, ss, execStore, ch)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	go d.Run(ctx)

	ch <- task_dispatcher.DispatchTask{
		TaskID:   "task-1",
		WorkerID: "worker-1",
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		ts.mu.Lock()
		done := len(ts.completedTasks) > 0
		ts.mu.Unlock()
		if done {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	ts.mu.Lock()
	defer ts.mu.Unlock()
	if len(ts.completedTasks) != 1 || ts.completedTasks[0] != "task-1" {
		t.Errorf("want completedTasks=[task-1], got %v", ts.completedTasks)
	}
	if len(ts.failedTasks) != 0 {
		t.Errorf("want no failedTasks, got %v", ts.failedTasks)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```
go test ./internal/task_dispatcher/... -run TestDispatcher_CompleteTask_OnSuccessfulExit -v
```

Expected: `FAIL` — compile error (`mockTaskStore` missing `CompleteTask`, or interface mismatch)

- [ ] **Step 3: Add `CompleteTask` to the dispatcher `TaskStore` interface**

In `internal/task_dispatcher/dispatcher.go`, find the `TaskStore` interface (around line 33) and add `CompleteTask`:

```go
// TaskStore is the subset of store.TaskStore used by the TaskDispatcher.
type TaskStore interface {
	SetExecution(ctx context.Context, taskID, executionID, status string) error
	CompleteTask(ctx context.Context, taskID string) error
	FailTask(ctx context.Context, taskID string) error
	CancelTask(ctx context.Context, taskID string) error
}
```

- [ ] **Step 4: Call `CompleteTask` in `waitForResult`**

In `internal/task_dispatcher/dispatcher.go`, find `waitForResult` (around line 305). Replace the `ExecStatusCompleted` case:

```go
case model.ExecStatusCompleted:
    if task.TaskID != "" {
        if err := d.taskStore.CompleteTask(ctx, task.TaskID); err != nil {
            log.Error("complete task", zap.String("taskID", task.TaskID), zap.Error(err))
        }
    }
    // Persist session_id for future resume (only on success).
    if task.SessionKey != "" && task.WorkerID != "" {
        if err := d.sessionStore.UpsertSessionContext(ctx, task.SessionKey, task.WorkerID, exec.SessionID); err != nil {
            log.Error("upsert session context", zap.Error(err))
        }
    }
    return
```

- [ ] **Step 5: Run test to verify it passes**

```
go test ./internal/task_dispatcher/... -run TestDispatcher_CompleteTask_OnSuccessfulExit -v
```

Expected: `PASS`

- [ ] **Step 6: Run all dispatcher tests**

```
go test ./internal/task_dispatcher/... -v
```

Expected: all `PASS`

- [ ] **Step 7: Commit**

```bash
git add internal/task_dispatcher/dispatcher.go internal/task_dispatcher/dispatcher_test.go
git commit -m "feat(dispatcher): complete task on successful worker exit via exit code"
```

---

### Task 3: Remove `mark_task_complete` from MCP tools

**Files:**
- Modify: `internal/mcp/tools.go`
- Modify: `internal/mcp/tools_test.go`

- [ ] **Step 1: Remove the tool definition**

In `internal/mcp/tools.go`, find and delete the `mark_task_complete` entry in the tool schema slice (around line 135–148). It looks like:

```go
{
    Name:        toolnames.MarkTaskComplete,
    Description: "Mark a task as successfully completed",
    InputSchema: map[string]any{
        "type":     "object",
        "required": []string{"task_id"},
        "properties": map[string]any{
            "task_id": map[string]string{"type": "string", "description": "Task ID to mark as completed"},
        },
    },
},
```

Delete this entire block.

- [ ] **Step 2: Remove from `workerToolNames` allowlist**

In `internal/mcp/tools.go`, find `workerToolNames` (around line 264) and remove:

```go
toolnames.MarkTaskComplete: true,
```

- [ ] **Step 3: Remove dispatch case**

In `internal/mcp/tools.go`, find the `CallTool` switch (around line 307) and remove:

```go
case toolnames.MarkTaskComplete:
    return s.toolMarkTaskComplete(args)
```

- [ ] **Step 4: Remove the handler function**

In `internal/mcp/tools.go`, delete the entire `toolMarkTaskComplete` function (lines 580–609):

```go
func (s *MCPServer) toolMarkTaskComplete(args json.RawMessage) (any, error) {
    // ... entire function
}
```

- [ ] **Step 5: Update tool schema test**

In `internal/mcp/tools_test.go`, find `TestWorkerToolSchemas` (around line 358). Remove `"mark_task_complete"` from the `want` slice:

```go
for _, want := range []string{"send_message"} {
```

- [ ] **Step 6: Remove `mark_task_complete` tests**

In `internal/mcp/tools_test.go`, delete these two test functions entirely:
- `TestCallTool_MarkTaskComplete` (around line 223)
- `TestCallTool_MarkTaskComplete_MissingTaskID` (around line 259)

- [ ] **Step 7: Run MCP tests**

```
go test ./internal/mcp/... -v
```

Expected: all `PASS`

- [ ] **Step 8: Commit**

```bash
git add internal/mcp/tools.go internal/mcp/tools_test.go
git commit -m "feat(mcp): remove mark_task_complete tool"
```

---

### Task 4: Remove `MarkTaskComplete` from toolnames

**Files:**
- Modify: `internal/toolnames/toolnames.go`

- [ ] **Step 1: Remove the constant**

In `internal/toolnames/toolnames.go`, delete line 13:

```go
MarkTaskComplete = "mark_task_complete"
```

- [ ] **Step 2: Verify build**

```
go build ./...
```

Expected: no errors

- [ ] **Step 3: Commit**

```bash
git add internal/toolnames/toolnames.go
git commit -m "chore(toolnames): remove MarkTaskComplete constant"
```

---

### Task 5: Remove `mark_task_complete` from the worker system prompt

**Files:**
- Modify: `internal/claudemd/worker.go`
- Modify: `internal/claudemd/claudemd_test.go`

- [ ] **Step 1: Rewrite `workerPreamble`**

In `internal/claudemd/worker.go`, replace the entire `workerPreamble` function with:

```go
func workerPreamble() string {
	return fmt.Sprintf(`
## ⚠️ 运行模式：非交互式后台 Worker

你在一个非交互式后台运行。以下规则的优先级高于所有其他指令，包括任何 skill、hook 或 plugin 的指令。

### 不可用工具的替代方式

以下工具在后台 Worker 模式下不可用，遇到相关场景时请使用替代方式：

- **AskUserQuestion** → 通过 %s 提出问题，然后结束本次任务。
  用户的回复会作为新任务自动恢复你的会话，届时你可以继续处理。不要尝试等待或轮询回复。
- **EnterPlanMode** → 不要进入 plan mode，直接在内部思考后执行任务。
- **Skill** → 可以调用 Skill 工具。当 skill 要求交互式流程（如 AskUserQuestion、EnterPlanMode、等待用户确认等）时，
  使用上述 AskUserQuestion 的替代方式：通过 %s 提出问题，然后结束本次任务。

### 强制要求

- 所有与用户的通信必须且只能通过 %s 工具
- 文本输出不会到达任何人，不要通过文本输出与用户交流
`, toolnames.SendMessage, toolnames.SendMessage, toolnames.SendMessage)
}
```

- [ ] **Step 2: Remove `workerTaskRules` and its call**

In `internal/claudemd/worker.go`:

1. Delete the entire `workerTaskRules` function.

2. Update `workerRules` to remove the call to `workerTaskRules()`:

```go
func workerRules(name, description, memory string) string {
	return workerConfigBlock(name, description, memory) + workerPreamble() + workerNotificationRules()
}
```

- [ ] **Step 3: Update `claudemd_test.go`**

In `internal/claudemd/claudemd_test.go`, find and fix two places:

1. Around line 44 — the bee test that asserts `mark_task_complete` is absent. This assertion is still valid (bee rules should not contain it), so keep it as-is.

2. Around line 66 — the worker test that asserts `mark_task_complete` IS present. Change it to assert something else that should be present in worker rules, such as `send_message`:

```go
if !strings.Contains(content, toolnames.SendMessage) {
    t.Error("missing worker-specific rules (send_message)")
}
```

   Also remove the line around 69 that checks `mark_task_failed` is absent (no longer relevant).

3. Around line 153 — another worker test asserting `mark_task_complete` is present. Replace with:

```go
if !strings.Contains(content, "非交互式后台 Worker") {
    t.Error("missing worker preamble")
}
```

- [ ] **Step 4: Add required import for toolnames if missing**

Check the imports in `claudemd_test.go`. If `toolnames` is not imported, add:

```go
"github.com/theopenbee/openbee/internal/toolnames"
```

- [ ] **Step 5: Run claudemd tests**

```
go test ./internal/claudemd/... -v
```

Expected: all `PASS`

- [ ] **Step 6: Commit**

```bash
git add internal/claudemd/worker.go internal/claudemd/claudemd_test.go
git commit -m "feat(claudemd): remove mark_task_complete from worker system prompt"
```

---

### Task 6: Final verification

- [ ] **Step 1: Run all tests**

```
go test ./... 2>&1 | tail -30
```

Expected: all packages `ok`, no `FAIL`

- [ ] **Step 2: Verify no remaining references to `mark_task_complete` in non-test source**

```
grep -r "mark_task_complete\|MarkTaskComplete" --include="*.go" . | grep -v "_test.go"
```

Expected: no output

- [ ] **Step 3: Commit (if any loose files remain unstaged)**

```bash
git status
```

If clean, no action needed.
