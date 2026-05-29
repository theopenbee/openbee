# Remove `execution` Subcommand & Invert Task↔Execution — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove the `openbee ctl execution` subcommand and its `list_executions` RPC tool, invert the task↔execution relationship (executions carry `task_id`; tasks drop `execution_id`), and make `list_tasks` return each task's full execution history.

**Architecture:** Land the change as a non-breaking transition: first add `task_id` to executions and dual-write, migrate all readers off `task.ExecutionID`, then drop the old column/field, and finally delete the standalone execution query surface. Each task leaves the tree compiling and tests green.

**Tech Stack:** Go, SQLite (custom migration runner in `internal/infra/store/db.go`), cobra CLI, gin HTTP, React/TS web frontend.

Spec: `docs/remove-execution-subcommand-spec.md`.

---

## File Structure

- `internal/infra/store/db.go` — migrations 45 (add `task_id` + backfill) and 46 (drop `execution_id`).
- `internal/infra/model/execution.go` — add `WorkerExecution.TaskID`.
- `internal/infra/model/task.go` — remove `Task.ExecutionID`.
- `internal/infra/store/execution_store.go` — `Create` gains `taskID`; `task_id` in select/scan; new `GetRunningByTaskID`, `ListByTaskIDs`.
- `internal/infra/store/task_store.go` — drop `execution_id` from SQL/scan; remove `SetExecution`, `GetTaskByExecutionID`.
- `internal/domain/worker/execution.go` — `ExecuteWorker` gains `taskID`.
- `internal/domain/task/dispatcher.go` / `scheduler.go` — thread `taskID`; `SetExecution`→`UpdateStatus`; reverse lookup via `exec.TaskID`.
- `internal/rpc/tools.go` — `list_tasks` embeds executions; cancel/clear/worker-status use `task_id`; delete `toolListExecutions` + dispatch case.
- `internal/domain/command/{status.go,clear.go,task_format.go}` — resolve running exec id per task for display + clear-stop.
- `internal/api/task_handler.go` + `web/src/lib/types.ts` — drop `execution_id`.
- `internal/infra/auth/scopes.go` — remove `read:executions`.
- `internal/infra/utils/toolnames.go` — remove `ListExecutions`.
- `cmd/openbee/ctl_execution.go` — deleted.
- `internal/app/app.go` — wire running-exec lookup into command handlers.
- `CHANGELOG.md` — English entry.

---

## Task 1: Add `task_id` to executions (additive, dual-write)

**Files:**
- Modify: `internal/infra/store/db.go` (after migration version 44)
- Modify: `internal/infra/model/execution.go`
- Modify: `internal/infra/store/execution_store.go`
- Modify: `internal/domain/worker/execution.go`
- Modify: `internal/domain/task/dispatcher.go:34,279,349,365`
- Modify: `internal/domain/task/dispatcher_test.go` (ExecuteWorker mocks)
- Test: `internal/infra/store/execution_store_test.go`

- [ ] **Step 1: Add the migration (additive column + backfill)**

In `internal/infra/store/db.go`, append to the migrations slice after the `version: 44` entry:

```go
	{
		version: 45,
		name:    "add_task_id_to_executions_and_backfill",
		sql: `ALTER TABLE bee_executions ADD COLUMN task_id TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_executions_task_id ON bee_executions(task_id);
UPDATE bee_executions
   SET task_id = (SELECT t.id FROM bee_tasks t WHERE t.execution_id = bee_executions.id)
 WHERE EXISTS (SELECT 1 FROM bee_tasks t WHERE t.execution_id = bee_executions.id);`,
	},
```

- [ ] **Step 2: Add the model field**

In `internal/infra/model/execution.go`, add `TaskID` to `WorkerExecution` (right after `ID`):

```go
type WorkerExecution struct {
	ID           string          `json:"id" db:"id"`
	TaskID       string          `json:"task_id,omitempty" db:"task_id"`
	WorkerID     *string         `json:"worker_id,omitempty" db:"worker_id"`
```

- [ ] **Step 3: Write failing tests for the new store methods**

Add to `internal/infra/store/execution_store_test.go` (follow the existing test setup helpers in that file for creating a store/db):

```go
func TestExecutionStore_CreateWritesTaskID(t *testing.T) {
	s := newTestExecutionStore(t)
	exec, err := s.Create("w1", "task-1", "trigger", "sess-1", "claude")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, err := s.GetByID(exec.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.TaskID != "task-1" {
		t.Errorf("task_id: want task-1 got %q", got.TaskID)
	}
}

func TestExecutionStore_GetRunningByTaskID(t *testing.T) {
	s := newTestExecutionStore(t)
	running, _ := s.Create("w1", "task-1", "in", "sess-1", "claude")
	if err := s.UpdateStatus(running.ID, model.ExecStatusRunning); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}
	got, err := s.GetRunningByTaskID(context.Background(), "task-1")
	if err != nil {
		t.Fatalf("GetRunningByTaskID: %v", err)
	}
	if got == nil || got.ID != running.ID {
		t.Fatalf("want running exec %s, got %+v", running.ID, got)
	}
	none, err := s.GetRunningByTaskID(context.Background(), "task-x")
	if err != nil {
		t.Fatalf("GetRunningByTaskID(none): %v", err)
	}
	if none != nil {
		t.Errorf("want nil for unknown task, got %+v", none)
	}
}

func TestExecutionStore_ListByTaskIDs(t *testing.T) {
	s := newTestExecutionStore(t)
	e1, _ := s.Create("w1", "task-1", "first", "sess-1", "claude")
	e2, _ := s.Create("w1", "task-1", "second", "sess-2", "claude")
	_, _ = s.Create("w1", "task-2", "other", "sess-3", "claude")
	m, err := s.ListByTaskIDs(context.Background(), []string{"task-1", "task-2"})
	if err != nil {
		t.Fatalf("ListByTaskIDs: %v", err)
	}
	if len(m["task-1"]) != 2 {
		t.Fatalf("task-1 want 2 execs, got %d", len(m["task-1"]))
	}
	// newest first (e2 created after e1)
	if m["task-1"][0].ID != e2.ID || m["task-1"][1].ID != e1.ID {
		t.Errorf("expected newest-first ordering; got %s,%s", m["task-1"][0].ID, m["task-1"][1].ID)
	}
	if len(m["task-2"]) != 1 {
		t.Errorf("task-2 want 1 exec, got %d", len(m["task-2"]))
	}
}
```

If `newTestExecutionStore` does not already exist in the test file, reuse whatever store-construction helper the existing tests use (search the file for `NewExecutionStore(`); add a small helper if needed mirroring that setup.

- [ ] **Step 4: Run the new tests to verify they fail**

Run: `go test ./internal/infra/store/ -run 'TestExecutionStore_(CreateWritesTaskID|GetRunningByTaskID|ListByTaskIDs)' -v`
Expected: FAIL — `Create` signature mismatch / methods undefined.

- [ ] **Step 5: Update `Create`, `execSelect`, `scanExecution`, and add the new methods**

In `internal/infra/store/execution_store.go`:

Change `Create` (line ~38) to accept `taskID` and write it:

```go
func (s *ExecutionStore) Create(workerID, taskID, triggerInput, sessionID, engine string) (model.WorkerExecution, error) {
	millis := time.Now().UnixMilli()
	exec := model.WorkerExecution{
		ID:           uuid.New().String(),
		TaskID:       taskID,
		WorkerID:     &workerID,
		SessionID:    sessionID,
		Engine:       engine,
		TriggerInput: triggerInput,
		Status:       model.ExecStatusPending,
		StartedAt:    &millis,
	}
	_, err := s.db.Exec(
		`INSERT INTO bee_executions (id, task_id, worker_id, session_id, engine, trigger_input, status, result, ai_process_pid, started_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, '', 0, ?)`,
		exec.ID, exec.TaskID, exec.WorkerID, exec.SessionID, exec.Engine, exec.TriggerInput, exec.Status, millis,
	)
	if err != nil {
		return model.WorkerExecution{}, fmt.Errorf("insert execution: %w", err)
	}
	return exec, nil
}
```

Update `execSelect` (line ~82) to select `task_id`:

```go
const execSelect = `
SELECT e.id, e.task_id, e.worker_id, e.session_id, e.engine, e.trigger_input, e.status, e.result, e.log_path,
       e.ai_process_pid, e.started_at, e.completed_at, COALESCE(w.name, '')
FROM bee_executions e
LEFT JOIN bee_workers w ON w.id = e.worker_id`
```

Update `scanExecution` (line ~88) to scan `TaskID` in the new position:

```go
func scanExecution(scanner interface{ Scan(...any) error }) (model.WorkerExecution, error) {
	var e model.WorkerExecution
	err := scanner.Scan(&e.ID, &e.TaskID, &e.WorkerID, &e.SessionID, &e.Engine, &e.TriggerInput, &e.Status, &e.Result, &e.LogPath, &e.AIProcessPID, &e.StartedAt, &e.CompletedAt, &e.WorkerName)
	return e, err
}
```

Update `CreateBeeExecution` (line ~60) INSERT to include the new column (bee executions have no task):

```go
	_, err := s.db.Exec(
		`INSERT INTO bee_executions (id, task_id, worker_id, session_id, engine, trigger_input, status, result, ai_process_pid, started_at)
		 VALUES (?, '', NULL, ?, ?, ?, ?, '', 0, ?)`,
		exec.ID, exec.SessionID, exec.Engine, exec.TriggerInput, exec.Status, millis,
	)
```

Add the two new methods (place near `GetRunningByWorkerID`):

```go
// GetRunningByTaskID returns the currently running execution for a task, or nil if none.
func (s *ExecutionStore) GetRunningByTaskID(ctx context.Context, taskID string) (*model.WorkerExecution, error) {
	row := s.db.QueryRowContext(ctx, execSelect+` WHERE e.task_id = ? AND e.status = ? LIMIT 1`, taskID, model.ExecStatusRunning)
	e, err := scanExecution(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get running execution by task: %w", err)
	}
	return &e, nil
}

// ListByTaskIDs returns executions grouped by task_id, newest first within each task.
func (s *ExecutionStore) ListByTaskIDs(ctx context.Context, taskIDs []string) (map[string][]model.WorkerExecution, error) {
	out := make(map[string][]model.WorkerExecution, len(taskIDs))
	if len(taskIDs) == 0 {
		return out, nil
	}
	args := make([]any, 0, len(taskIDs))
	for _, id := range taskIDs {
		args = append(args, id)
	}
	q := execSelect + ` WHERE e.task_id IN (` + inPlaceholders(len(taskIDs)) + `) ORDER BY e.started_at DESC`
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list executions by task ids: %w", err)
	}
	defer rows.Close()
	execs, err := scanExecutions(rows)
	if err != nil {
		return nil, err
	}
	for _, e := range execs {
		out[e.TaskID] = append(out[e.TaskID], e)
	}
	return out, nil
}
```

Note: `inPlaceholders` already exists in the `store` package (used by `task_store.go`). Confirm with `grep -n "func inPlaceholders" internal/infra/store/*.go`; reuse it.

- [ ] **Step 6: Thread `taskID` through `ExecuteWorker` and the dispatcher**

In `internal/domain/worker/execution.go`, change `ExecuteWorker` (line ~18) signature and the `Create` call (line ~26):

```go
func (m *Manager) ExecuteWorker(ctx context.Context, workerID, taskID, triggerInput, sessionID string, resume bool) (model.WorkerExecution, error) {
```
```go
	exec, err := m.executionStore.Create(workerID, taskID, triggerInput, sessionID, engineName)
```

In `internal/domain/task/dispatcher.go`, update the `ExecutionManager` interface (line ~34):

```go
	ExecuteWorker(ctx context.Context, workerID, taskID, input, sessionID string, resume bool) (model.WorkerExecution, error)
```

Update both call sites to pass `task.TaskID`:
- `executeFresh` (line ~349): `return d.manager.ExecuteWorker(ctx, task.WorkerID, task.TaskID, prefix+instruction, sessionID, false)`
- `resolveExecution` (line ~365): `exec, err := d.manager.ExecuteWorker(ctx, task.WorkerID, task.TaskID, instruction, sessionID, true)`

- [ ] **Step 7: Update ExecuteWorker mocks in dispatcher_test.go**

Every mock implementing `ExecuteWorker` must add the `taskID` param. The mocks (search `func .*ExecuteWorker`): `mockExecManager` (line ~29), `quickCancelExecManager` (~66), `orderedMockManager` (~261), `blockingExecManager` (~733), `alwaysFailExecManager` (~746), `fallbackExecManager` (~758), `cancelTrackingExecManager` (~887). Insert one extra `_` (or named) string param after `workerID`. Example for `mockExecManager`:

```go
func (m *mockExecManager) ExecuteWorker(_ context.Context, _, _, instruction, sessionID string, resume bool) (model.WorkerExecution, error) {
```

Apply the analogous one-param insertion to every other mock.

- [ ] **Step 8: Run build + tests**

Run: `go build ./... && go test ./internal/infra/store/ ./internal/domain/task/ ./internal/domain/worker/ -run 'Execution|Dispatcher|ExecuteWorker' -v`
Expected: PASS (new store tests green; dispatcher tests still pass).

- [ ] **Step 9: Commit**

```bash
git add internal/infra/store/db.go internal/infra/model/execution.go internal/infra/store/execution_store.go internal/infra/store/execution_store_test.go internal/domain/worker/execution.go internal/domain/task/dispatcher.go internal/domain/task/dispatcher_test.go
git commit -m "feat: add task_id to executions and thread it through dispatch"
```

---

## Task 2: `list_tasks` returns associated executions

**Files:**
- Modify: `internal/rpc/tools.go:407-438` (`toolListTasks`)
- Test: `internal/rpc/tools_test.go`

- [ ] **Step 1: Write a failing test**

Add to `internal/rpc/tools_test.go` (follow the existing test harness used by other `toolListTasks`/`CallTool` tests in that file — reuse its server/store setup helpers):

```go
func TestToolListTasks_IncludesExecutions(t *testing.T) {
	srv, deps := newTestServer(t) // use the file's existing helper
	ctx := context.Background()

	taskID, _ := deps.taskStore.Create(ctx, model.Task{
		MessageID: deps.messageID, WorkerID: "w1", Instruction: "do x",
		Type: model.TaskTypeImmediate, Status: model.TaskStatusRunning,
	})
	exec, _ := deps.executionStore.Create("w1", taskID, "do x", "sess-1", "claude")

	raw, err := srv.CallTool(ctx, utils.ListTasks, json.RawMessage(`{"worker_id":"w1"}`))
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	b, _ := json.Marshal(raw)
	if !strings.Contains(string(b), exec.ID) {
		t.Fatalf("expected executions to include %s, got %s", exec.ID, string(b))
	}
}
```

Adapt `newTestServer`/`deps` names to the actual helpers in `tools_test.go`.

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/rpc/ -run TestToolListTasks_IncludesExecutions -v`
Expected: FAIL — response has no execution data.

- [ ] **Step 3: Embed executions in the response**

In `internal/rpc/tools.go`, add a wrapper type (near `toolListTasks`) and attach executions. `model.Task` has no JSON tags, so the embedded (anonymous) field keeps its existing PascalCase keys; add `Executions` alongside:

```go
type taskWithExecutions struct {
	model.Task
	Executions []model.WorkerExecution `json:"executions"`
}
```

Replace the tail of `toolListTasks` (the `if tasks == nil ... return tasks, nil` block) with:

```go
	if tasks == nil {
		tasks = []model.Task{}
	}
	taskIDs := make([]string, 0, len(tasks))
	for _, t := range tasks {
		taskIDs = append(taskIDs, t.ID)
	}
	execsByTask, err := s.executionStore.ListByTaskIDs(ctx, taskIDs)
	if err != nil {
		return nil, fmt.Errorf("list executions for tasks: %w", err)
	}
	out := make([]taskWithExecutions, 0, len(tasks))
	for _, t := range tasks {
		execs := execsByTask[t.ID]
		if execs == nil {
			execs = []model.WorkerExecution{}
		}
		out = append(out, taskWithExecutions{Task: t, Executions: execs})
	}
	return out, nil
```

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./internal/rpc/ -run TestToolListTasks_IncludesExecutions -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/rpc/tools.go internal/rpc/tools_test.go
git commit -m "feat: embed associated executions in list_tasks response"
```

---

## Task 3: Migrate all `task.ExecutionID` readers to `task_id` lookups

**Files:**
- Modify: `internal/rpc/tools.go` (`toolGetWorkerStatus:714-726`, `toolCancelTask:462-479`, `toolClearSession:634-645`)
- Modify: `internal/domain/task/dispatcher.go:309-313,398-415` and `waitForResult`
- Modify: `internal/domain/task/scheduler.go:100`
- Modify: `internal/domain/command/task_format.go`, `status.go`, `clear.go`
- Modify: `internal/app/app.go:168,170`
- Modify tests: `internal/rpc/tools_test.go`, `internal/domain/command/{status_test.go,clear_test.go}`

- [ ] **Step 1: `toolGetWorkerStatus` — use `exec.TaskID` directly**

In `internal/rpc/tools.go`, replace lines ~714-726 with:

```go
	if er.err == nil && er.exec != nil {
		result["current_execution"] = map[string]any{
			"id":          er.exec.ID,
			"task_id":     er.exec.TaskID,
			"instruction": er.exec.TriggerInput,
			"started_at":  er.exec.StartedAt,
		}
	}
```

- [ ] **Step 2: `toolCancelTask` — stop via running execution lookup**

In `internal/rpc/tools.go` `toolCancelTask`, replace the `if task.ExecutionID != "" { ... }` block (lines ~462-479) with:

```go
	running, err := s.executionStore.GetRunningByTaskID(ctx, params.TaskID)
	if err != nil {
		return nil, fmt.Errorf("get running execution: %w", err)
	}
	if running != nil {
		var stopErr error
		if s.execStopper != nil {
			stopErr = s.execStopper.StopExecution(running.ID)
		}
		if stopErr != nil {
			log.Debug("stop execution: process not active",
				zap.String("op", "cancel_task"),
				zap.String("executionID", running.ID),
				zap.Error(stopErr))
			s.finalizeCancelledExecution(ctx, running.ID)
		}
	}
```

The earlier `task, err := s.taskStore.GetByID(ctx, params.TaskID)` fetch (line ~458) is now unused for execution; remove that fetch and its error handling if `task` is otherwise unused (verify: it is only used for `task.ExecutionID`). Remove the now-dead lines.

- [ ] **Step 3: `toolClearSession` — stop via running execution lookup**

In `internal/rpc/tools.go`, replace the stop loop (lines ~634-645) with:

```go
	for _, t := range tasksToStop {
		running, err := s.executionStore.GetRunningByTaskID(ctx, t.ID)
		if err != nil || running == nil {
			continue
		}
		if err := s.execStopper.StopExecution(running.ID); err != nil {
			log.Debug("stop execution: process not active",
				zap.String("op", "clear_session"),
				zap.String("executionID", running.ID),
				zap.Error(err))
			s.finalizeCancelledExecution(ctx, running.ID)
		}
	}
```

- [ ] **Step 4: Dispatcher — `SetExecution`→`UpdateStatus`, reverse lookup via `exec.TaskID`**

In `internal/domain/task/dispatcher.go`:

Replace the `SetExecution` call (line ~310):

```go
	if task.TaskID != "" {
		if err := d.taskStore.UpdateStatus(taskCtx, task.TaskID, model.TaskStatusRunning); err != nil {
			log.Error("update task status", zap.String("taskID", task.TaskID), zap.Error(err))
		}
	}
```

Update the `TaskStore` interface (line ~45) — replace `SetExecution(...)` with:

```go
	UpdateStatus(ctx context.Context, taskID, status string) error
```

(Keep `CompleteTask`, `FailTask`, `CancelTask`.)

If `waitForResult` or `getWorkerStatus`-style code in the dispatcher uses `GetTaskByExecutionID`, replace with `exec.TaskID`. (The only `GetTaskByExecutionID` caller is `toolGetWorkerStatus`, already handled in Step 1; confirm with grep.)

- [ ] **Step 5: Scheduler — `SetExecution`→`UpdateStatus`**

In `internal/domain/task/scheduler.go`, replace the interface method (line ~21) and the call (line ~100):

Interface:
```go
	UpdateStatus(ctx context.Context, taskID, status string) error
```
Call:
```go
				s.taskStore.UpdateStatus(ctx, ct.ID, model.TaskStatusFailed) //nolint:errcheck
```

- [ ] **Step 6: Update dispatcher/scheduler mocks**

In `internal/domain/task/dispatcher_test.go`, replace `mockTaskStore.SetExecution` (line ~85) with:

```go
func (s *mockTaskStore) UpdateStatus(_ context.Context, _, _ string) error { return nil }
```

In the scheduler test file (search `SetExecution` under `internal/domain/task/`), apply the same rename to any scheduler-store mock.

- [ ] **Step 7: Command display — resolve running exec id per task**

In `internal/domain/command/task_format.go`, add a lookup interface and change `formatTaskLine` to take a `taskID→execID` map:

```go
// RunningExecLookup resolves the running execution id for a set of tasks.
type RunningExecLookup interface {
	RunningExecIDsByTaskIDs(ctx context.Context, taskIDs []string) (map[string]string, error)
}

func formatTaskLine(format string, t model.Task, workerNames, execIDs map[string]string, nowMs int64) string {
	runtimeSec := (nowMs - t.CreatedAt) / 1000
	return fmt.Sprintf(format,
		workerNameOrFallback(workerNames, t.WorkerID),
		utils.TruncateRunes(strings.Join(strings.Fields(t.Instruction), " "), maxInstructionRunes),
		formatRelative(runtimeSec),
		shortExecID(execIDs[t.ID]),
	)
}
```

Add the implementing method to `ExecutionStore` in `internal/infra/store/execution_store.go`:

```go
// RunningExecIDsByTaskIDs returns a map of task_id -> running execution id.
func (s *ExecutionStore) RunningExecIDsByTaskIDs(ctx context.Context, taskIDs []string) (map[string]string, error) {
	out := make(map[string]string, len(taskIDs))
	if len(taskIDs) == 0 {
		return out, nil
	}
	args := []any{model.ExecStatusRunning}
	for _, id := range taskIDs {
		args = append(args, id)
	}
	q := `SELECT task_id, id FROM bee_executions WHERE status = ? AND task_id IN (` + inPlaceholders(len(taskIDs)) + `)`
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("running exec ids by task ids: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var taskID, execID string
		if err := rows.Scan(&taskID, &execID); err != nil {
			return nil, err
		}
		out[taskID] = execID
	}
	return out, rows.Err()
}
```

- [ ] **Step 8: Wire the lookup into `/status` and `/clear`**

In `internal/domain/command/status.go`:
- Add field `runningExecs RunningExecLookup` to `StatusCommandHandler` and a constructor param (append it).
- In `formatStatus`, before the task loop, build the map:

```go
	taskIDs := make([]string, 0, len(tasks))
	for _, t := range tasks {
		taskIDs = append(taskIDs, t.ID)
	}
	execIDs, err := h.runningExecs.RunningExecIDsByTaskIDs(context.Background(), taskIDs)
	if err != nil {
		log.Error("resolve running exec ids for /status", zap.Error(err))
		execIDs = map[string]string{}
	}
```
- Change the `formatTaskLine` call (line ~125) to pass `execIDs`:
```go
			lines = append(lines, formatTaskLine(m.TaskLine, t, workerNames, execIDs, nowMs))
```
  (Note: `formatStatus` currently has no `ctx`; either thread `ctx` from `formatStatus`'s caller or use `context.Background()` as shown. Threading `ctx` is cleaner — `HandleCommand` already has `ctx`; pass it into `formatStatus`.)

In `internal/domain/command/clear.go`:
- Add field `runningExecs RunningExecLookup` to `ClearCommandHandler` + constructor param.
- Replace the stop loop in `handleClearAll` (lines ~132-139):

```go
	taskIDs := make([]string, 0, len(runningTasks))
	for _, t := range runningTasks {
		taskIDs = append(taskIDs, t.ID)
	}
	execIDs, err := h.runningExecs.RunningExecIDsByTaskIDs(ctx, taskIDs)
	if err != nil {
		log.Error("resolve running exec ids for /clear", zap.Error(err))
		execIDs = map[string]string{}
	}
	for _, t := range runningTasks {
		execID := execIDs[t.ID]
		if execID == "" {
			continue
		}
		if err := h.execStopper.StopExecution(execID); err != nil {
			log.Error("stop execution for /clear", zap.String("executionID", execID), zap.Error(err))
		}
	}
```
- In `formatConfirmPrompt`, build `execIDs` the same way (it has `h.now()` but no ctx; thread `ctx` from `handleClearAll`/`handleClearWorker` into `formatConfirmPrompt`, or use `context.Background()`), and pass to `formatTaskLine` (line ~168).

- [ ] **Step 9: Wire constructors in app.go**

In `internal/app/app.go`, pass `s.executionStore` as the new `runningExecs` arg:

```go
	clearCmdHandler := command.NewClearCommandHandler(s.workerStore, s.sessionStore, s.taskStore, mgr, disp, s.executionStore, sendersByPlatform, engineCfg)
	statusCmdHandler := command.NewStatusCommandHandler(s.sessionStore, s.taskStore, s.workerStore, s.executionStore, sendersByPlatform, engineCfg)
```

(Match the parameter position to where you added the field in each constructor. Confirm `s.executionStore` is the field name in `app.go` with `grep -n "executionStore" internal/app/app.go`.)

- [ ] **Step 10: Update command tests**

In `internal/domain/command/{status_test.go,clear_test.go}`:
- The test tasks set `ExecutionID: "..."`. Since display now comes from the lookup, remove `ExecutionID` from the `model.Task` literals (it is removed from the model in Task 4 anyway) and instead provide a fake `RunningExecLookup` returning the expected `task.ID -> execID` map.
- Add a minimal fake:
```go
type fakeRunningExecs map[string]string
func (f fakeRunningExecs) RunningExecIDsByTaskIDs(_ context.Context, ids []string) (map[string]string, error) {
	return map[string]string(f), nil
}
```
- Pass it into the handler constructors in those tests, keyed by the task IDs used, so the asserted short-exec-id output matches.

- [ ] **Step 11: Build + test**

Run: `go build ./... && go test ./internal/rpc/ ./internal/domain/task/ ./internal/domain/command/ -v`
Expected: PASS.

- [ ] **Step 12: Commit**

```bash
git add internal/rpc/tools.go internal/domain/task/dispatcher.go internal/domain/task/scheduler.go internal/domain/task/dispatcher_test.go internal/domain/command/ internal/infra/store/execution_store.go internal/app/app.go
git commit -m "refactor: resolve task executions via task_id instead of task.execution_id"
```

---

## Task 4: Drop `execution_id` column, field, and dead store methods

**Files:**
- Modify: `internal/infra/store/db.go` (migration 46)
- Modify: `internal/infra/store/task_store.go` (SQL + scan; remove `SetExecution`, `GetTaskByExecutionID`)
- Modify: `internal/infra/model/task.go`
- Modify: `internal/api/task_handler.go:25,112`
- Modify: `web/src/lib/types.ts:103`
- Modify tests: `internal/infra/store/task_store_test.go`

- [ ] **Step 1: Add the drop-column migration**

In `internal/infra/store/db.go`, append after migration 45:

```go
	{
		version: 46,
		name:    "drop_execution_id_from_tasks",
		sql:     `ALTER TABLE bee_tasks DROP COLUMN execution_id`,
	},
```

- [ ] **Step 2: Remove `execution_id` from all task SQL and scans**

In `internal/infra/store/task_store.go`, remove `execution_id` / `t.execution_id` from each column list and the corresponding scan target. Specifically:

- `Create` INSERT (lines ~30-37): drop `execution_id` from the column list, drop one `?`, and remove the `""` arg.

```go
	_, err := s.db.ExecContext(ctx, `
        INSERT INTO bee_tasks
            (id, message_id, worker_id, instruction, type, status,
             scheduled_at, cron_expr, next_run_at,
             created_at, updated_at)
        VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		id, t.MessageID, t.WorkerID, t.Instruction, t.Type, t.Status,
		t.ScheduledAt, t.CronExpr, t.NextRunAt,
		now, now,
	)
```

- `GetByID` SELECT (lines ~47-51): remove `execution_id`.
- `List` SELECT (lines ~126-129): remove `t.execution_id`.
- `ListBySessionKey` SELECT (lines ~158-160): remove `t.execution_id`.
- `ClaimDueTasks` SELECT (lines ~188-191) and its `rows.Scan` (remove `&ct.ExecutionID` at line ~211).
- `PeekDueScheduledTasks` SELECT (lines ~261-263): remove `execution_id`.
- `scanTask` (line ~489): remove `&t.ExecutionID`.
- `scanTasks` (line ~508): remove `&t.ExecutionID`.

Delete `SetExecution` (lines ~275-281) and `GetTaskByExecutionID` (lines ~455-471) entirely.

- [ ] **Step 3: Remove `ExecutionID` from the model**

In `internal/infra/model/task.go`, delete the `ExecutionID string` line from `Task`.

- [ ] **Step 4: Remove `execution_id` from the HTTP task response**

In `internal/api/task_handler.go`: delete `ExecutionID string `json:"execution_id"`` (line ~25) from `taskResponse`, and delete the `ExecutionID: t.ExecutionID,` assignment (line ~112).

In `web/src/lib/types.ts`: delete `execution_id: string` (line ~103) from `Task`.

- [ ] **Step 5: Update task_store tests**

In `internal/infra/store/task_store_test.go`:
- Delete `TestTaskStore_SetExecution` (line ~104) and `TestTaskStore_GetTaskByExecutionID` (line ~871).
- In other tests, remove assertions/usages of `ExecutionID` and replace any `ts.SetExecution(ctx, id, "exec-1", status)` calls with `ts.UpdateStatus(ctx, id, status)` (drop the exec-id arg). The test at line ~202-204 asserting `execution_id` untouched should be removed.

Also fix `internal/rpc/tools_test.go` lines ~571-578 and ~1125: replace `ts.SetExecution(ctx, id, "exec-...", running)` with creating an execution carrying the task id (`deps.executionStore.Create("w", id, ..., running-status)`) plus `ts.UpdateStatus(ctx, id, model.TaskStatusRunning)`, so cancel/clear paths find a running execution.

- [ ] **Step 6: Build + full test**

Run: `go build ./... && go test ./...`
Expected: PASS. (Run `cd web && npm run build` or the repo's TS typecheck to confirm the web type change compiles.)

- [ ] **Step 7: Commit**

```bash
git add internal/infra/store/db.go internal/infra/store/task_store.go internal/infra/store/task_store_test.go internal/infra/model/task.go internal/api/task_handler.go web/src/lib/types.ts internal/rpc/tools_test.go
git commit -m "refactor: drop execution_id from tasks now that executions carry task_id"
```

---

## Task 5: Remove the `execution` CLI subcommand, RPC tool, and scope

**Files:**
- Delete: `cmd/openbee/ctl_execution.go`
- Modify: `internal/infra/utils/toolnames.go`
- Modify: `internal/rpc/tools.go` (dispatch case + `toolListExecutions`)
- Modify: `internal/infra/auth/scopes.go`
- Modify tests: any referencing `ListExecutions` / `ScopeReadExecutions` / `toolListExecutions`

- [ ] **Step 1: Delete the CLI file**

```bash
git rm cmd/openbee/ctl_execution.go
```

- [ ] **Step 2: Remove the tool name constant**

In `internal/infra/utils/toolnames.go`, delete the `ListExecutions = "list_executions"` line (~30).

- [ ] **Step 3: Remove the dispatch case and handler**

In `internal/rpc/tools.go`: delete the `case utils.ListExecutions: return s.toolListExecutions(ctx, args)` block (lines ~117-118) and the entire `toolListExecutions` function (lines ~1280-1310).

Do NOT remove `ExecutionFilter`, `ListFiltered`, `executionFilterWhere`, `pagedResult`, or `normalizePage` — they are still used by `internal/api/session_handler.go` and other tools.

- [ ] **Step 4: Remove the scope**

In `internal/infra/auth/scopes.go`: delete `ScopeReadExecutions` const (~15), its entry in `AllScopes` (~23), and the `utils.ListExecutions: ScopeReadExecutions` line in `toolScopeMap` (~66).

- [ ] **Step 5: Fix references in tests**

Run `grep -rn "ListExecutions\|ScopeReadExecutions\|toolListExecutions\|list_executions\|read:executions" --include=*.go internal cmd` and update/remove any remaining references (e.g., scope-validation tests listing all scopes, RPC dispatch tests).

- [ ] **Step 6: Build + full test**

Run: `go build ./... && go test ./...`
Expected: PASS.

- [ ] **Step 7: Verify the CLI no longer exposes the subcommand**

Run: `go run ./cmd/openbee ctl execution list 2>&1 | head -5`
Expected: an "unknown command" error from cobra (subcommand gone).

- [ ] **Step 8: Commit**

```bash
git add -A
git commit -m "feat: remove execution subcommand, list_executions tool, and read:executions scope"
```

---

## Task 6: CHANGELOG entry

**Files:**
- Modify: `CHANGELOG.md`

- [ ] **Step 1: Add an English entry**

Per project convention, changelog content must be in English. Add a bullet under the current/unreleased section noting:
- Removed the `openbee ctl execution` subcommand and the `list_executions` tool / `read:executions` scope.
- `list_tasks` now returns each task's associated execution records.
- Internal: executions now reference their task via `task_id` (replacing `bee_tasks.execution_id`).

Match the existing CHANGELOG formatting/section style already present in the file.

- [ ] **Step 2: Commit**

```bash
git add CHANGELOG.md
git commit -m "docs: update changelog for execution subcommand removal"
```

---

## Self-Review Notes

- **Spec coverage:** migrations (T1,T4), model changes (T1,T4), store methods (T1,T3), dispatcher/scheduler (T1,T3), cancel/clear (T3), `/status` `/clear` display via latest/running exec id (T3), `list_tasks` executions (T2), HTTP + web field removal (T4), tool/CLI/scope removal (T5), CHANGELOG (T6). All four confirmed decisions covered.
- **Transition safety:** `task_id` is added and dual-written (T1) before any reader moves off `execution_id` (T3) and before the old column is dropped (T4) — the tree compiles and tests pass at every commit.
- **Type consistency:** new methods used consistently — `Create(workerID, taskID, triggerInput, sessionID, engine)`, `GetRunningByTaskID(ctx, taskID)`, `ListByTaskIDs(ctx, taskIDs)`, `RunningExecIDsByTaskIDs(ctx, taskIDs)`, `ExecuteWorker(ctx, workerID, taskID, input, sessionID, resume)`, `taskWithExecutions{Task, Executions}`.
- **Shared code preserved:** `ExecutionFilter`/`ListFiltered` kept for the web session API; only the RPC `list_executions` surface is removed.
