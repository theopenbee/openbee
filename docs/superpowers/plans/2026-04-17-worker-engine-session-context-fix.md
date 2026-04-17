# Worker Engine Session Context Fix Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix `TaskDispatcher` so that session context operations use each worker's configured engine rather than the system-default engine, ensuring `bee_session_contexts` records are stored and retrieved under the correct engine key.

**Architecture:** Add a `resolveWorkerEngine(workerID string) string` helper to `TaskDispatcher` that looks up the worker's configured engine via the existing `WorkerLookup` interface and falls back to `d.engineName` when the lookup is unavailable or the worker has no engine set. Replace all three uses of `d.engineName` in session context operations (`resolveExecution` and `upsertSessionContext`) with calls to this helper.

**Tech Stack:** Go, `internal/domain/task` package, existing `WorkerLookup` interface (`GetByID`), `mockWorkerLookup` and `mockSessionStore` test helpers.

---

## File Map

| Action | File | Purpose |
|--------|------|---------|
| Modify | `internal/domain/task/dispatcher.go` | Add `resolveWorkerEngine`; replace `d.engineName` in session context ops |
| Modify | `internal/domain/task/dispatcher_test.go` | Add test verifying worker engine is used in session context |

---

### Task 1: Add `resolveWorkerEngine` helper and fix session context operations (TDD)

**Files:**
- Modify: `internal/domain/task/dispatcher_test.go` (append test at end of file)
- Modify: `internal/domain/task/dispatcher.go:326-326` (add method after `workerSkillHint`)
- Modify: `internal/domain/task/dispatcher.go:344` (fix `resolveExecution`)
- Modify: `internal/domain/task/dispatcher.go:359-360` (fix `resolveExecution` error path)
- Modify: `internal/domain/task/dispatcher.go:421` (fix `upsertSessionContext`)

- [ ] **Step 1: Write the failing test**

Append to `internal/domain/task/dispatcher_test.go` (after the last `}` in the file):

```go
func TestTaskDispatcher_WorkerEngine_UsedInSessionContext(t *testing.T) {
	mgr := &mockExecManager{
		execResult: model.WorkerExecution{ID: "exec-1", SessionID: "sess-pi-1", Status: model.ExecStatusCompleted},
	}
	eq := &mockExecutionQuerier{result: model.WorkerExecution{ID: "exec-1", Status: model.ExecStatusCompleted}}
	ss := newMockSessionStore()
	lookup := &mockWorkerLookup{
		worker: model.Worker{ID: "w1", Engine: "pi"},
	}
	// System default is "kimi", but the worker is configured with "pi".
	d, in, _ := newTaskDispatcher(mgr, eq, ss,
		task.WithEngine("kimi"),
		task.WithWorkerLookup(lookup),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go d.Run(ctx)

	in <- immediateTask("sk-1", "w1", "do the thing")

	if !waitForExecCount(mgr, 1, 2*time.Second) {
		t.Fatal("timeout waiting for execution")
	}

	// Session context must be stored under the worker's engine ("pi"), not the system default ("kimi").
	if got := ss.sessionID("sk-1", "w1", "pi"); got == "" {
		t.Error("expected session context stored under engine 'pi', got nothing")
	}
	if got := ss.sessionID("sk-1", "w1", "kimi"); got != "" {
		t.Errorf("session context must not be stored under system-default engine 'kimi', got %q", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee2
go test ./internal/domain/task/... -run TestTaskDispatcher_WorkerEngine_UsedInSessionContext -v
```

Expected: FAIL — session context is stored under `"kimi"` (the current bug), not `"pi"`.

- [ ] **Step 3: Add `resolveWorkerEngine` helper to `dispatcher.go`**

In `internal/domain/task/dispatcher.go`, insert the following method immediately after the closing `}` of `workerSkillHint` (after line 326):

```go
// resolveWorkerEngine returns the engine name to use for session context operations.
// If workerLookup is set and the worker has a configured engine, that name is returned.
// Otherwise falls back to d.engineName (the system-default engine).
func (d *TaskDispatcher) resolveWorkerEngine(workerID string) string {
	if d.workerLookup != nil {
		if w, err := d.workerLookup.GetByID(workerID); err == nil && w.Engine != "" {
			return w.Engine
		}
	}
	return d.engineName
}
```

- [ ] **Step 4: Fix `resolveExecution` — replace `d.engineName` with `resolveWorkerEngine`**

In `internal/domain/task/dispatcher.go`, replace the body of `resolveExecution` starting at the session context lookup. Find:

```go
	sessionID, err := d.sessionStore.GetSessionContextForEngine(ctx, task.SessionKey, task.WorkerID, d.engineName)
	if err != nil {
		log.Error("get session context", zap.Error(err))
	}
	if sessionID == "" {
		return d.executeWithHint(ctx, task, instruction)
	}
	log.Info("resuming session", zap.String("sessionID", sessionID), zap.String("taskID", task.TaskID))
	d.upsertSessionContext(ctx, task, sessionID)
	exec, err := d.manager.ExecuteWorker(ctx, task.WorkerID, instruction, sessionID, true)
	if err == nil {
		return exec, nil
	}
	log.Error("resume error, falling back to fresh", zap.Error(err))
	if task.SessionKey != "" && task.WorkerID != "" {
		if clearErr := d.sessionStore.DeleteSessionContextForEngine(ctx, task.SessionKey, task.WorkerID, d.engineName); clearErr != nil {
			log.Error("clear stale session context", zap.String("sessionKey", task.SessionKey), zap.String("workerID", task.WorkerID), zap.String("engine", d.engineName), zap.Error(clearErr))
		}
	}
```

Replace with:

```go
	engineName := d.resolveWorkerEngine(task.WorkerID)
	sessionID, err := d.sessionStore.GetSessionContextForEngine(ctx, task.SessionKey, task.WorkerID, engineName)
	if err != nil {
		log.Error("get session context", zap.Error(err))
	}
	if sessionID == "" {
		return d.executeWithHint(ctx, task, instruction)
	}
	log.Info("resuming session", zap.String("sessionID", sessionID), zap.String("taskID", task.TaskID))
	d.upsertSessionContext(ctx, task, sessionID)
	exec, err := d.manager.ExecuteWorker(ctx, task.WorkerID, instruction, sessionID, true)
	if err == nil {
		return exec, nil
	}
	log.Error("resume error, falling back to fresh", zap.Error(err))
	if task.SessionKey != "" && task.WorkerID != "" {
		if clearErr := d.sessionStore.DeleteSessionContextForEngine(ctx, task.SessionKey, task.WorkerID, engineName); clearErr != nil {
			log.Error("clear stale session context", zap.String("sessionKey", task.SessionKey), zap.String("workerID", task.WorkerID), zap.String("engine", engineName), zap.Error(clearErr))
		}
	}
```

- [ ] **Step 5: Fix `upsertSessionContext` — replace `d.engineName` with `resolveWorkerEngine`**

In `internal/domain/task/dispatcher.go`, find `upsertSessionContext`:

```go
func (d *TaskDispatcher) upsertSessionContext(ctx context.Context, task DispatchTask, sessionID string) {
	if task.SessionKey == "" || task.WorkerID == "" || sessionID == "" {
		return
	}
	if err := d.sessionStore.UpsertSessionContext(ctx, task.SessionKey, task.WorkerID, sessionID, d.engineName); err != nil {
		log.Error("upsert session context", zap.String("sessionKey", task.SessionKey), zap.Error(err))
	}
}
```

Replace with:

```go
func (d *TaskDispatcher) upsertSessionContext(ctx context.Context, task DispatchTask, sessionID string) {
	if task.SessionKey == "" || task.WorkerID == "" || sessionID == "" {
		return
	}
	if err := d.sessionStore.UpsertSessionContext(ctx, task.SessionKey, task.WorkerID, sessionID, d.resolveWorkerEngine(task.WorkerID)); err != nil {
		log.Error("upsert session context", zap.String("sessionKey", task.SessionKey), zap.Error(err))
	}
}
```

- [ ] **Step 6: Run the new test to verify it passes**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee2
go test ./internal/domain/task/... -run TestTaskDispatcher_WorkerEngine_UsedInSessionContext -v
```

Expected: PASS.

- [ ] **Step 7: Run the full task package test suite**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee2
go test ./internal/domain/task/... -v
```

Expected: all tests PASS. The new method does not affect existing tests — when `workerLookup` is nil or `worker.Engine` is empty, `resolveWorkerEngine` falls back to `d.engineName`, which is identical to previous behavior.

- [ ] **Step 8: Run the full test suite**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee2
go test ./...
```

Expected: all tests PASS.

- [ ] **Step 9: Commit**

```bash
git add internal/domain/task/dispatcher.go internal/domain/task/dispatcher_test.go
git commit -m "fix(task): use worker engine for session context operations"
```

---

## Self-Review

### Spec Coverage

| Requirement | Covered by |
|-------------|-----------|
| Session contexts stored under worker's configured engine, not system default | Step 3–5 (implementation), Step 1 (test) |
| Fallback to system default when worker has no engine set | `resolveWorkerEngine` fallback to `d.engineName`; covered by all existing tests |
| No cleanup of old session contexts on engine change | No cleanup code added — stale records are silently abandoned |
| Resume failure after engine switch falls back to fresh session | Existing `resolveExecution` fallback logic is unchanged; still handles this |

### Placeholder Scan

No TBD, TODO, or incomplete steps. All code blocks are complete.

### Type Consistency

- `resolveWorkerEngine(workerID string) string` — used as `d.resolveWorkerEngine(task.WorkerID)` in Steps 4 and 5. `task.WorkerID` is `string`. Return type `string` matches the parameter type of `GetSessionContextForEngine`, `UpsertSessionContext`, and `DeleteSessionContextForEngine`.
- `engineName` local variable in Step 4 is type `string` — consistent with all usages in the same block.
- `model.Worker.Engine` is `string` — matches return type of `resolveWorkerEngine`.
