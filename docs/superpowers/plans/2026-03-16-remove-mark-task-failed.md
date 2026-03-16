# Remove mark_task_failed Tool — Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove the `mark_task_failed` MCP tool and have the dispatcher automatically mark tasks as failed when a worker process exits abnormally.

**Architecture:** Add `FailTask` to the store and dispatcher's `TaskStore` interface; call it in `waitForResult` when `ExecStatusFailed` is detected. Remove the tool registration, handler, and all LLM-facing instructions that reference it.

**Tech Stack:** Go, SQLite (`database/sql`), standard `testing` package

---

## Chunk 1: Add `FailTask` to store and dispatcher

### Task 1: Add `FailTask` to `store.TaskStore` (TDD)

**Files:**
- Modify: `internal/store/task_store_test.go`
- Modify: `internal/store/task_store.go`

- [ ] **Step 1: Write the failing tests**

Add to `internal/store/task_store_test.go` (in the `package store` test file, after existing tests):

```go
func TestTaskStore_FailTask_RegularTask_MarksAsFailed(t *testing.T) {
	ts, cleanup := newTaskStoreForTest(t)
	defer cleanup()

	ctx := context.Background()
	id, _ := ts.Create(ctx, model.Task{
		MessageID: "m1", WorkerID: "w1", Instruction: "x",
		Type: model.TaskTypeImmediate, Status: model.TaskStatusRunning,
		CreatedAt: 1, UpdatedAt: 1,
	})

	if err := ts.FailTask(ctx, id); err != nil {
		t.Fatalf("FailTask: %v", err)
	}

	got, _ := ts.GetByID(ctx, id)
	if got.Status != model.TaskStatusFailed {
		t.Errorf("expected status=failed, got %q", got.Status)
	}
}

func TestTaskStore_FailTask_ScheduledTask_WithCron_ResetsToPending(t *testing.T) {
	ts, cleanup := newTaskStoreForTest(t)
	defer cleanup()

	ctx := context.Background()
	id, _ := ts.Create(ctx, model.Task{
		MessageID: "m1", WorkerID: "w1", Instruction: "x",
		Type: model.TaskTypeScheduled, CronExpr: "* * * * *",
		Status: model.TaskStatusRunning,
		CreatedAt: 1, UpdatedAt: 1,
	})

	if err := ts.FailTask(ctx, id); err != nil {
		t.Fatalf("FailTask: %v", err)
	}

	got, _ := ts.GetByID(ctx, id)
	if got.Status != model.TaskStatusPending {
		t.Errorf("expected status=pending (reset for next run), got %q", got.Status)
	}
}

func TestTaskStore_FailTask_ScheduledTask_NoCron_MarksAsFailed(t *testing.T) {
	ts, cleanup := newTaskStoreForTest(t)
	defer cleanup()

	ctx := context.Background()
	id, _ := ts.Create(ctx, model.Task{
		MessageID: "m1", WorkerID: "w1", Instruction: "x",
		Type: model.TaskTypeScheduled, CronExpr: "",
		Status: model.TaskStatusRunning,
		CreatedAt: 1, UpdatedAt: 1,
	})

	if err := ts.FailTask(ctx, id); err != nil {
		t.Fatalf("FailTask: %v", err)
	}

	got, _ := ts.GetByID(ctx, id)
	if got.Status != model.TaskStatusFailed {
		t.Errorf("expected status=failed (no cron), got %q", got.Status)
	}
}

func TestTaskStore_FailTask_ScheduledTask_Cancelled_NoChange(t *testing.T) {
	ts, cleanup := newTaskStoreForTest(t)
	defer cleanup()

	ctx := context.Background()
	id, _ := ts.Create(ctx, model.Task{
		MessageID: "m1", WorkerID: "w1", Instruction: "x",
		Type: model.TaskTypeScheduled, CronExpr: "* * * * *",
		Status: model.TaskStatusCancelled,
		CreatedAt: 1, UpdatedAt: 1,
	})

	// FailTask on a cancelled scheduled task should not error
	if err := ts.FailTask(ctx, id); err != nil {
		t.Fatalf("FailTask on cancelled task: %v", err)
	}

	got, _ := ts.GetByID(ctx, id)
	if got.Status != model.TaskStatusCancelled {
		t.Errorf("expected status=cancelled (preserved), got %q", got.Status)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd /Users/tengteng/work/robobee/core && go test ./internal/store/... -run TestTaskStore_FailTask -v
```

Expected: compile error — `ts.FailTask undefined`

- [ ] **Step 3: Implement `FailTask` in `internal/store/task_store.go`**

Add after the `CompleteScheduledTask` function:

```go
// FailTask marks a task as failed. For scheduled tasks with a cron expression,
// it resets to pending instead so the task retries on the next scheduled run.
// Called by the dispatcher when a worker process exits abnormally.
func (s *TaskStore) FailTask(ctx context.Context, taskID string) error {
	task, err := s.GetByID(ctx, taskID)
	if err != nil {
		return fmt.Errorf("get task: %w", err)
	}
	if task.Type == model.TaskTypeScheduled && task.CronExpr != "" {
		_, err := s.CompleteScheduledTask(ctx, taskID)
		return err
	}
	return s.UpdateStatus(ctx, taskID, model.TaskStatusFailed)
}
```

Also update the stale `UpdateStatus` doc comment (line 204–205 of `task_store.go`) from:
```go
// UpdateStatus sets only the status of a task. Unlike SetExecution, it does
// not touch execution_id. Used by mark_task_success and mark_task_failed MCP tools.
```
to:
```go
// UpdateStatus sets only the status of a task. Unlike SetExecution, it does
// not touch execution_id. Used by mark_task_success MCP tool and FailTask.
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
cd /Users/tengteng/work/robobee/core && go test ./internal/store/... -run TestTaskStore_FailTask -v
```

Expected: all 4 tests PASS

- [ ] **Step 5: Run the full store test suite**

```bash
cd /Users/tengteng/work/robobee/core && go test ./internal/store/... -v
```

Expected: all tests PASS

- [ ] **Step 6: Commit**

```bash
cd /Users/tengteng/work/robobee/core && git add internal/store/task_store.go internal/store/task_store_test.go
git commit -m "feat(store): add FailTask method for dispatcher-driven failure handling"
```

---

### Task 2: Add `FailTask` to the dispatcher's `TaskStore` interface and wire it up

**Files:**
- Modify: `internal/task_dispatcher/dispatcher.go`
- Modify: `internal/task_dispatcher/dispatcher_test.go`

- [ ] **Step 1: Add `FailTask` stub to `mockTaskStore` in `dispatcher_test.go`**

In `internal/task_dispatcher/dispatcher_test.go`, update `mockTaskStore` (lines 44–46) from:

```go
type mockTaskStore struct{}

func (s *mockTaskStore) SetExecution(_ context.Context, _, _, _ string) error { return nil }
```

to:

```go
type mockTaskStore struct{}

func (s *mockTaskStore) SetExecution(_ context.Context, _, _, _ string) error { return nil }
func (s *mockTaskStore) FailTask(_ context.Context, _ string) error            { return nil }
```

- [ ] **Step 2: Run dispatcher tests to verify they still pass (pre-change baseline)**

```bash
cd /Users/tengteng/work/robobee/core && go test ./internal/task_dispatcher/... -v
```

Expected: PASS — the current interface has no `FailTask` requirement yet, so adding the stub is purely preparatory and doesn't break anything

- [ ] **Step 3: Add `FailTask` to the `TaskStore` interface and call it in `waitForResult`**

In `internal/task_dispatcher/dispatcher.go`:

**a)** Extend the `TaskStore` interface (after `SetExecution`):

```go
type TaskStore interface {
	SetExecution(ctx context.Context, taskID, executionID, status string) error
	FailTask(ctx context.Context, taskID string) error
}
```

**b)** In `waitForResult`, replace the `ExecStatusFailed` case:

```go
case model.ExecStatusFailed:
    // Terminal task status is set by the worker via mark_task_failed.
    return
```

with:

```go
case model.ExecStatusFailed:
    // Dispatcher sets terminal task status on abnormal worker exit.
    if taskID != "" {
        if err := d.taskStore.FailTask(ctx, taskID); err != nil {
            slog.Error("fail task", "component", "taskdispatcher", "taskID", taskID, "error", err)
        }
    }
    return
```

**c)** Update stale comments:

Line 145 — change:
```go
// can call mark_task_success/failed and send_message via MCP.
```
to:
```go
// can call mark_task_success and send_message via MCP.
```

Line 217 — change:
```go
// Terminal task status is set by the worker via mark_task_success.
```
(no change needed — this comment is already correct)

Line 225 — change:
```go
// Terminal task status is set by the worker via mark_task_failed.
```
to:
```go
// Dispatcher sets terminal task status on abnormal worker exit.
```

- [ ] **Step 3b: Write behavioral test — `FailTask` called on `ExecStatusFailed`**

In `internal/task_dispatcher/dispatcher_test.go`, replace the simple `mockTaskStore` with a recording version, and add a new test. First update `mockTaskStore` to capture `FailTask` calls:

```go
type mockTaskStore struct {
	mu          sync.Mutex
	failedTasks []string
}

func (s *mockTaskStore) SetExecution(_ context.Context, _, _, _ string) error { return nil }
func (s *mockTaskStore) FailTask(_ context.Context, taskID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failedTasks = append(s.failedTasks, taskID)
	return nil
}
```

Update `newTaskDispatcher` to accept and expose the mock store:

```go
func newTaskDispatcher(mgr task_dispatcher.ExecutionManager, eq task_dispatcher.ExecutionQuerier, ss task_dispatcher.SessionStore) (*task_dispatcher.TaskDispatcher, chan task_dispatcher.DispatchTask, *mockTaskStore) {
	in := make(chan task_dispatcher.DispatchTask, 4)
	ts := &mockTaskStore{}
	d := task_dispatcher.New(mgr, ts, ss, eq, in)
	return d, in, ts
}
```

Update all call sites of `newTaskDispatcher` — they currently use `d, in := newTaskDispatcher(...)`, change to `d, in, _ := newTaskDispatcher(...)`.

Add the behavioral test:

```go
func TestTaskDispatcher_ExecStatusFailed_CallsFailTask(t *testing.T) {
	mgr := &mockExecManager{
		execResult: model.WorkerExecution{ID: "exec-fail", SessionID: "sess-1"},
	}
	eq := &mockExecutionQuerier{
		result: model.WorkerExecution{ID: "exec-fail", Status: model.ExecStatusFailed},
	}
	d, in, ts := newTaskDispatcher(mgr, eq, newMockSessionStore())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go d.Run(ctx)

	task := task_dispatcher.DispatchTask{
		TaskID:      "task-fail-1",
		WorkerID:    "w1",
		SessionKey:  "s1",
		Instruction: "do something",
		ReplyTo:     platform.InboundMessage{Platform: "test", SessionKey: "s1"},
		TaskType:    "immediate",
		MessageID:   "msg-1",
	}
	in <- task

	if !waitForExecCount(mgr, 1, 2*time.Second) {
		t.Fatal("ExecuteWorker was not called within timeout")
	}
	// Wait for FailTask to be called
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		ts.mu.Lock()
		n := len(ts.failedTasks)
		ts.mu.Unlock()
		if n > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	ts.mu.Lock()
	defer ts.mu.Unlock()
	if len(ts.failedTasks) != 1 || ts.failedTasks[0] != "task-fail-1" {
		t.Errorf("expected FailTask called with task-fail-1, got %v", ts.failedTasks)
	}
}
```

- [ ] **Step 3c: Run new behavioral test (should fail — FailTask not wired yet)**

```bash
cd /Users/tengteng/work/robobee/core && go test ./internal/task_dispatcher/... -run TestTaskDispatcher_ExecStatusFailed_CallsFailTask -v
```

Expected: FAIL — `FailTask` never called because `waitForResult` doesn't call it yet

- [ ] **Step 4: Run dispatcher tests**

```bash
cd /Users/tengteng/work/robobee/core && go test ./internal/task_dispatcher/... -v
```

Expected: all tests PASS (including new behavioral test, after Step 3 wired `FailTask` into `waitForResult`)

- [ ] **Step 5: Verify full build compiles**

```bash
cd /Users/tengteng/work/robobee/core && go build ./...
```

Expected: no errors

- [ ] **Step 6: Commit**

```bash
cd /Users/tengteng/work/robobee/core && git add internal/task_dispatcher/dispatcher.go internal/task_dispatcher/dispatcher_test.go
git commit -m "feat(dispatcher): auto-fail task on worker abnormal exit via FailTask"
```

---

## Chunk 2: Remove `mark_task_failed` tool

### Task 3: Remove tool from `toolnames`, `tools.go`, and its tests

**Files:**
- Modify: `internal/toolnames/toolnames.go`
- Modify: `internal/mcp/tools.go`
- Modify: `internal/mcp/tools_test.go`

- [ ] **Step 1: Remove `MarkTaskFailed` constant**

In `internal/toolnames/toolnames.go`, delete the line:

```go
MarkTaskFailed  = "mark_task_failed"
```

- [ ] **Step 2: Remove tool registration, handler, and dispatch case from `tools.go`**

In `internal/mcp/tools.go`:

**a)** Remove the `mark_task_failed` entry from the `Tools()` slice (lines 141–152):

```go
{
    Name:        toolnames.MarkTaskFailed,
    Description: "Mark a task as failed",
    InputSchema: map[string]any{
        "type":     "object",
        "required": []string{"task_id"},
        "properties": map[string]any{
            "task_id": map[string]string{"type": "string", "description": "Task ID to mark as failed"},
            "reason":  map[string]string{"type": "string", "description": "Optional failure reason"},
        },
    },
},
```

**b)** Remove the dispatch case from `callTool` (lines 206–207):

```go
case toolnames.MarkTaskFailed:
    return s.toolMarkTaskFailed(args)
```

**c)** Remove the `toolMarkTaskFailed` function entirely (lines 486–516).

- [ ] **Step 3: Update `tools_test.go`**

**a)** Remove the two `mark_task_failed` test functions (lines 265–332):
- `TestCallTool_MarkTaskFailed`
- `TestCallTool_MarkTaskFailed_NoReason`

**b)** In `TestToolSchemas_Count_AfterNewTools` (line 417), change `12` to `11` and update the comment:

```go
if len(schemas) != 11 {
    t.Errorf("expected 11 tool schemas, got %d", len(schemas))
}
```

**c)** In `TestToolSchemas_IncludesNewTools` (line 428), remove `"mark_task_failed"` from the slice:

```go
for _, want := range []string{"mark_task_success", "send_message"} {
```

- [ ] **Step 4: Run MCP tests**

```bash
cd /Users/tengteng/work/robobee/core && go test ./internal/mcp/... -v
```

Expected: all tests PASS; no reference to `mark_task_failed`

- [ ] **Step 5: Verify full build compiles**

```bash
cd /Users/tengteng/work/robobee/core && go build ./...
```

Expected: no errors

- [ ] **Step 6: Commit**

```bash
cd /Users/tengteng/work/robobee/core && git add internal/toolnames/toolnames.go internal/mcp/tools.go internal/mcp/tools_test.go
git commit -m "feat(mcp): remove mark_task_failed tool — failure handled by dispatcher"
```

---

## Chunk 3: Update worker system rules and test

### Task 4: Remove `mark_task_failed` from worker rules in `claudemd.go`

**Files:**
- Modify: `internal/claudemd/claudemd.go`
- Modify: `internal/claudemd/claudemd_test.go`

- [ ] **Step 1: Update `workerRules()` in `claudemd.go`**

The `workerRules()` function (lines 128–160) references `toolnames.MarkTaskFailed` in two places. Replace the entire `workerRules()` function body — the metadata section and task status section — removing all mentions of `mark_task_failed`:

Change the system metadata description (line 144) from:
```go
- **task_id** — 当前任务的唯一标识，用于调用 `+"`%s`"+` 或 `+"`%s`"+` 标记任务状态
```
(which references both `MarkTaskSuccess` and `MarkTaskFailed`)

to (referencing only `MarkTaskSuccess`):
```go
- **task_id** — 当前任务的唯一标识，用于调用 `+"`%s`"+` 标记任务成功
```

Change the task status section (lines 149–156) from:
```go
## 任务状态标记

任务执行结束后，你必须根据执行结果标记任务状态：

- **成功** — 调用 `+"`%s`"+` 并附上结果摘要
- **失败** — 调用 `+"`%s`"+` 并附上失败原因

这是每个任务的最后一步，不可遗漏。先调用 `+"`%s`"+` 通知结果，再标记状态。
```

to:
```go
## 任务状态标记

任务执行完成后，必须调用 `+"`%s`"+` 标记任务成功，并附上结果摘要。

这是每个任务的最后一步，不可遗漏。先调用 `+"`%s`"+` 通知结果，再调用 `+"`%s`"+` 标记完成。
```

Update the format args at the bottom of `workerRules()` (line 158) accordingly — remove `toolnames.MarkTaskFailed` from all format arg lists.

The full updated format args for the `return` sprintf call should use only `toolnames.MarkTaskSuccess` and `toolnames.SendMessage`:

```go
toolnames.MarkTaskSuccess, toolnames.SendMessage,
toolnames.MarkTaskSuccess,
toolnames.SendMessage, toolnames.MarkTaskSuccess)
```

- [ ] **Step 2: Update `claudemd_test.go`**

In `TestEnsureSystemRules_WritesWorkerRulesWithName` (lines 49–87):

Remove the positive assertion for `mark_task_failed` (lines 69–71):
```go
if !strings.Contains(content, "mark_task_failed") {
    t.Error("missing worker-specific rules (mark_task_failed)")
}
```

Add a negative assertion that `mark_task_failed` is NOT present:
```go
if strings.Contains(content, "mark_task_failed") {
    t.Error("worker rules must not contain mark_task_failed (failure is now system-handled)")
}
```

- [ ] **Step 3: Run claudemd tests**

```bash
cd /Users/tengteng/work/robobee/core && go test ./internal/claudemd/... -v
```

Expected: all tests PASS

- [ ] **Step 4: Run full test suite**

```bash
cd /Users/tengteng/work/robobee/core && go test ./...
```

Expected: all tests PASS

- [ ] **Step 5: Commit**

```bash
cd /Users/tengteng/work/robobee/core && git add internal/claudemd/claudemd.go internal/claudemd/claudemd_test.go
git commit -m "feat(claudemd): remove mark_task_failed from worker system rules"
```

---

## Final verification

- [ ] **Verify no remaining references to `mark_task_failed`**

```bash
cd /Users/tengteng/work/robobee/core && grep -r "mark_task_failed\|MarkTaskFailed\|toolMarkTaskFailed" --include="*.go" .
```

Expected: no output

- [ ] **Full build and test**

```bash
cd /Users/tengteng/work/robobee/core && go build ./... && go test ./...
```

Expected: build succeeds, all tests PASS
