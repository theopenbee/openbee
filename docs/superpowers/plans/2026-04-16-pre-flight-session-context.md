# Pre-flight Session Context Write Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Write a `bee_session_contexts` record before bee/worker execution starts so sessions are immediately visible and recoverable on crash.

**Architecture:** Three files change. `manager.go` gains an explicit `resume bool` parameter and stops generating UUIDs. `dispatcher.go` pre-generates UUIDs and calls `UpsertSessionContext` before `ExecuteWorker`. `feeder.go` calls `UpsertSessionContext` before `runner.Run()`. All existing post-execution upserts are retained.

**Tech Stack:** Go, SQLite (`database/sql`), `github.com/google/uuid`

---

## File Map

| File | Change |
|------|--------|
| `internal/domain/worker/manager.go` | Add `resume bool` param to `ExecuteWorker`; remove `uuid.New()` and `resume := sessionID != ""` |
| `internal/domain/task/dispatcher.go` | Update `ExecutionManager` interface; add `uuid.New()` + pre-flight upsert in `executeWithHint`; add pre-flight upsert in resume path of `resolveExecution`; thread `resume` flag through both call sites |
| `internal/domain/task/dispatcher_test.go` | Update all mock `ExecuteWorker` signatures; update `fallbackExecManager` to use `resume bool`; add pre-flight upsert assertion tests |
| `internal/domain/bee/feeder.go` | Add pre-flight upsert before `runner.Run()` in `processBeeGroup` |
| `internal/domain/bee/feeder_test.go` | Add test asserting session context is written before `runner.Run()` |

---

### Task 1: Update `ExecuteWorker` in `manager.go` — add explicit `resume bool`

**Files:**
- Modify: `internal/domain/worker/manager.go:95-127`

**Background:** Currently `manager.go` derives `resume` from whether `sessionID` is empty, and generates a UUID when it's empty. After this change the caller owns UUID generation and explicitly passes `resume`.

- [ ] **Step 1: Write a failing test**

In `internal/domain/worker/manager_test.go`, find or create the test for `ExecuteWorker`. Add a test that calls `ExecuteWorker` with a non-empty `sessionID` and `resume=false` and verifies the worker process is started with `resume=false` (not as a resume).

First, check if a manager test file exists:
```bash
ls internal/domain/worker/
```

If `manager_test.go` exists, skip to Step 2 (the compile error from the signature change is your "failing test"). If it doesn't, the compile error from changing the interface will drive TDD here — proceed to Step 2.

- [ ] **Step 2: Change the `ExecuteWorker` signature in `manager.go`**

Open `internal/domain/worker/manager.go`. Replace lines 93–127:

```go
// ExecuteWorker runs a worker. When resume is true, the AI engine will attempt
// to resume the session identified by sessionID; otherwise it starts a fresh session.
// sessionID must always be non-empty; callers are responsible for generating it.
func (m *Manager) ExecuteWorker(ctx context.Context, workerID, triggerInput, sessionID string, resume bool) (model.WorkerExecution, error) {
	worker, err := m.workerStore.GetByID(workerID)
	if err != nil {
		return model.WorkerExecution{}, fmt.Errorf("get worker: %w", err)
	}

	exec, err := m.executionStore.Create(workerID, triggerInput, sessionID)
	if err != nil {
		return model.WorkerExecution{}, fmt.Errorf("create execution: %w", err)
	}

	if err := m.workerStore.UpdateStatus(worker.ID, model.WorkerStatusWorking); err != nil {
		log.Error("failed to update worker status", zap.Error(err))
	}

	if err := m.engine.Prepare(worker.WorkDir, ai.PrepareOptions{Role: ai.RoleWorker}); err != nil {
		log.Error("prepare worker workspace", zap.String("op", "execute"), zap.Error(err))
	}
	timeout := m.workerTimeout

	if err := m.launchRuntime(exec, worker, timeout, triggerInput, resume); err != nil {
		m.executionStore.UpdateResult(exec.ID, err.Error(), model.ExecStatusFailed)
		m.workerStore.UpdateStatus(worker.ID, model.WorkerStatusError)
		return exec, fmt.Errorf("start runtime: %w", err)
	}

	return exec, nil
}
```

Key changes:
- Added `resume bool` as the 5th parameter
- Removed `resume := sessionID != ""`
- Removed `if sessionID == "" { sessionID = uuid.New().String() }`
- Remove the `"github.com/google/uuid"` import from `manager.go` if it is now unused (check with `go build`)

- [ ] **Step 3: Verify compile errors appear in dependents**

```bash
go build ./...
```

Expected: compile errors in `dispatcher.go` and any test files that call `ExecuteWorker`. This is expected — fix them in the next tasks.

---

### Task 2: Update `dispatcher.go` — interface, UUID generation, pre-flight upserts

**Files:**
- Modify: `internal/domain/task/dispatcher.go`

**Background:** `dispatcher.go` defines the `ExecutionManager` interface and has two call sites for `ExecuteWorker`: `executeWithHint` (fresh) and `resolveExecution` (resume). Both need pre-flight upserts and the updated call signature.

- [ ] **Step 1: Update the `ExecutionManager` interface**

In `internal/domain/task/dispatcher.go`, find the `ExecutionManager` interface (around line 29) and change the `ExecuteWorker` signature:

```go
// ExecutionManager manages worker executions.
type ExecutionManager interface {
	ExecuteWorker(ctx context.Context, workerID, input, sessionID string, resume bool) (model.WorkerExecution, error)
	CancelExecution(ctx context.Context, executionID string) error
}
```

- [ ] **Step 2: Add `uuid` import to `dispatcher.go`**

Add `"github.com/google/uuid"` to the import block in `internal/domain/task/dispatcher.go`.

- [ ] **Step 3: Update `executeWithHint` — pre-generate UUID and pre-flight upsert**

Replace the existing `executeWithHint` function (around line 320):

```go
// executeWithHint fetches the worker skill hint + persona and starts a fresh execution.
func (d *TaskDispatcher) executeWithHint(ctx context.Context, task DispatchTask, instruction string) (model.WorkerExecution, error) {
	hint, err := d.workerSkillHint(task.WorkerID)
	if err != nil {
		return model.WorkerExecution{}, err
	}
	sessionID := uuid.New().String()
	if task.SessionKey != "" && task.WorkerID != "" {
		if err := d.sessionStore.UpsertSessionContext(ctx, task.SessionKey, task.WorkerID, sessionID, d.engineName); err != nil {
			log.Error("pre-flight upsert session context", zap.Error(err))
		}
	}
	log.Info("executing worker", zap.String("workerID", task.WorkerID), zap.String("taskID", task.TaskID))
	return d.manager.ExecuteWorker(ctx, task.WorkerID, hint+"\n"+instruction, sessionID, false)
}
```

- [ ] **Step 4: Update `resolveExecution` — pre-flight upsert for resume path**

In `resolveExecution` (around line 329), replace the resume path:

```go
func (d *TaskDispatcher) resolveExecution(ctx context.Context, task DispatchTask, instruction string) (model.WorkerExecution, error) {
	if task.TaskType != model.TaskTypeImmediate {
		return d.executeWithHint(ctx, task, instruction)
	}
	sessionID, err := d.sessionStore.GetSessionContextForEngine(ctx, task.SessionKey, task.WorkerID, d.engineName)
	if err != nil {
		log.Error("get session context", zap.Error(err))
	}
	if sessionID == "" {
		return d.executeWithHint(ctx, task, instruction)
	}
	log.Info("resuming session", zap.String("sessionID", sessionID), zap.String("taskID", task.TaskID))
	if task.SessionKey != "" && task.WorkerID != "" {
		if err := d.sessionStore.UpsertSessionContext(ctx, task.SessionKey, task.WorkerID, sessionID, d.engineName); err != nil {
			log.Error("pre-flight upsert session context (resume)", zap.Error(err))
		}
	}
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
	return d.executeWithHint(ctx, task, instruction)
}
```

- [ ] **Step 5: Build to verify dispatcher compiles**

```bash
go build ./internal/domain/task/...
```

Expected: compiles cleanly (test files still fail — that's fine for now).

---

### Task 3: Update all test mocks and existing tests in `dispatcher_test.go`

**Files:**
- Modify: `internal/domain/task/dispatcher_test.go`

**Background:** Four mock types implement `ExecuteWorker` with the old signature. All need updating. `fallbackExecManager` currently uses `sessionID != ""` to detect resume — it must switch to the `resume bool` parameter.

- [ ] **Step 1: Update `mockExecManager.ExecuteWorker`**

Find (around line 27):
```go
func (m *mockExecManager) ExecuteWorker(_ context.Context, _, instruction, sessionID string) (model.WorkerExecution, error) {
	m.mu.Lock()
	if sessionID != "" {
		m.resumedWithSessionID = sessionID
	}
	m.executedInstructions = append(m.executedInstructions, instruction)
	m.mu.Unlock()
	return m.execResult, nil
}
```

Replace with:
```go
func (m *mockExecManager) ExecuteWorker(_ context.Context, _, instruction, sessionID string, resume bool) (model.WorkerExecution, error) {
	m.mu.Lock()
	if resume {
		m.resumedWithSessionID = sessionID
	}
	m.executedInstructions = append(m.executedInstructions, instruction)
	m.mu.Unlock()
	return m.execResult, nil
}
```

- [ ] **Step 2: Update `blockingExecManager.ExecuteWorker`**

Find (around line 628):
```go
func (m *blockingExecManager) ExecuteWorker(_ context.Context, _, _, _ string) (model.WorkerExecution, error) {
```

Replace with:
```go
func (m *blockingExecManager) ExecuteWorker(_ context.Context, _, _, _ string, _ bool) (model.WorkerExecution, error) {
```

- [ ] **Step 3: Update `alwaysFailExecManager.ExecuteWorker`**

Find (around line 641):
```go
func (m *alwaysFailExecManager) ExecuteWorker(_ context.Context, _, _, _ string) (model.WorkerExecution, error) {
```

Replace with:
```go
func (m *alwaysFailExecManager) ExecuteWorker(_ context.Context, _, _, _ string, _ bool) (model.WorkerExecution, error) {
```

- [ ] **Step 4: Update `fallbackExecManager.ExecuteWorker`**

Find (around line 653):
```go
func (m *fallbackExecManager) ExecuteWorker(_ context.Context, _, _, sessionID string) (model.WorkerExecution, error) {
	if sessionID != "" {
		return model.WorkerExecution{}, fmt.Errorf("session broken")
	}
	atomic.AddInt64(&m.freshCount, 1)
	return m.freshResult, nil
}
```

Replace with:
```go
func (m *fallbackExecManager) ExecuteWorker(_ context.Context, _, _, _ string, resume bool) (model.WorkerExecution, error) {
	if resume {
		return model.WorkerExecution{}, fmt.Errorf("session broken")
	}
	atomic.AddInt64(&m.freshCount, 1)
	return m.freshResult, nil
}
```

- [ ] **Step 5: Update `cancelTrackingExecManager.ExecuteWorker`**

Search for any remaining mock that still has the old 4-arg signature:
```bash
grep -n "ExecuteWorker" internal/domain/task/dispatcher_test.go
```

For each one found with the old signature, add `, _ bool` as the last parameter.

- [ ] **Step 6: Run existing dispatcher tests**

```bash
go test ./internal/domain/task/... -v -count=1
```

Expected: all existing tests pass. No new tests yet.

- [ ] **Step 7: Commit**

```bash
git add internal/domain/worker/manager.go internal/domain/task/dispatcher.go internal/domain/task/dispatcher_test.go
git commit -m "feat: add explicit resume flag to ExecuteWorker and pre-flight session upsert for workers"
```

---

### Task 4: Add pre-flight upsert tests for dispatcher

**Files:**
- Modify: `internal/domain/task/dispatcher_test.go`

**Background:** Verify that `UpsertSessionContext` is called **before** `ExecuteWorker` in both the fresh and resume paths.

- [ ] **Step 1: Add a mock that tracks call order**

Add this type near the other mocks in `dispatcher_test.go` (after `mockExecManager`):

```go
// orderedMockManager records whether UpsertSessionContext was called before ExecuteWorker.
type orderedMockManager struct {
	mu            sync.Mutex
	callOrder     []string // "upsert" or "execute"
	execResult    model.WorkerExecution
	receivedResume bool
	receivedSessID string
}

func (m *orderedMockManager) ExecuteWorker(_ context.Context, _, _, sessionID string, resume bool) (model.WorkerExecution, error) {
	m.mu.Lock()
	m.callOrder = append(m.callOrder, "execute")
	m.receivedResume = resume
	m.receivedSessID = sessionID
	m.mu.Unlock()
	return m.execResult, nil
}

func (m *orderedMockManager) CancelExecution(_ context.Context, _ string) error { return nil }

// orderedMockSessionStore wraps mockSessionStore and records upsert calls.
type orderedMockSessionStore struct {
	*mockSessionStore
	outer *orderedMockManager
}

func (s *orderedMockSessionStore) UpsertSessionContext(ctx context.Context, sessionKey, agentID, sessionID, engine string) error {
	s.outer.mu.Lock()
	s.outer.callOrder = append(s.outer.callOrder, "upsert")
	s.outer.mu.Unlock()
	return s.mockSessionStore.UpsertSessionContext(ctx, sessionKey, agentID, sessionID, engine)
}
```

- [ ] **Step 2: Write test — pre-flight upsert called before ExecuteWorker on fresh session**

Add this test:

```go
func TestTaskDispatcher_FreshSession_PreflightUpsertBeforeExecute(t *testing.T) {
	mgr := &orderedMockManager{
		execResult: model.WorkerExecution{ID: "exec-1", SessionID: "new-session"},
	}
	baseSS := newMockSessionStore()
	ss := &orderedMockSessionStore{mockSessionStore: baseSS, outer: mgr}
	eq := &mockExecutionQuerier{result: model.WorkerExecution{ID: "exec-1", Status: model.ExecStatusCompleted}}

	d, in, _ := newTaskDispatcher(mgr, eq, ss)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go d.Run(ctx)

	in <- immediateTask("s1", "w1", "first message")

	if !waitForExecCount2(mgr, 1, 2*time.Second) {
		t.Fatal("ExecuteWorker was not called within timeout")
	}

	mgr.mu.Lock()
	order := append([]string{}, mgr.callOrder...)
	resume := mgr.receivedResume
	sessID := mgr.receivedSessID
	mgr.mu.Unlock()

	if len(order) < 2 {
		t.Fatalf("expected at least 2 calls (upsert + execute), got %v", order)
	}
	if order[0] != "upsert" || order[1] != "execute" {
		t.Errorf("expected upsert before execute, got order %v", order)
	}
	if resume {
		t.Error("expected resume=false for fresh session")
	}
	if sessID == "" {
		t.Error("expected non-empty sessionID passed to ExecuteWorker")
	}
	// Verify pre-flight upsert stored the same sessionID that was passed to ExecuteWorker
	stored := baseSS.sessionID("s1", "w1", "")
	if stored != sessID {
		t.Errorf("pre-flight stored sessionID %q differs from ExecuteWorker sessionID %q", stored, sessID)
	}
}
```

Add this helper (needed by the test above):

```go
func waitForExecCount2(mgr *orderedMockManager, n int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		mgr.mu.Lock()
		var count int
		for _, c := range mgr.callOrder {
			if c == "execute" {
				count++
			}
		}
		mgr.mu.Unlock()
		if count >= n {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}
```

- [ ] **Step 3: Write test — pre-flight upsert called before ExecuteWorker on resume**

Add this test:

```go
func TestTaskDispatcher_ResumeSession_PreflightUpsertBeforeExecute(t *testing.T) {
	mgr := &orderedMockManager{
		execResult: model.WorkerExecution{ID: "exec-1", SessionID: "prior-session-id"},
	}
	baseSS := newMockSessionStore()
	_ = baseSS.UpsertSessionContext(context.Background(), "s1", "w1", "prior-session-id", "claude")
	ss := &orderedMockSessionStore{mockSessionStore: baseSS, outer: mgr}
	eq := &mockExecutionQuerier{result: model.WorkerExecution{ID: "exec-1", Status: model.ExecStatusCompleted}}

	d, in, _ := newTaskDispatcher(mgr, eq, ss, task.WithEngine("claude"))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go d.Run(ctx)

	in <- immediateTask("s1", "w1", "follow-up")

	if !waitForExecCount2(mgr, 1, 2*time.Second) {
		t.Fatal("ExecuteWorker was not called within timeout")
	}

	mgr.mu.Lock()
	order := append([]string{}, mgr.callOrder...)
	resume := mgr.receivedResume
	sessID := mgr.receivedSessID
	mgr.mu.Unlock()

	if len(order) < 2 {
		t.Fatalf("expected at least 2 calls (upsert + execute), got %v", order)
	}
	if order[0] != "upsert" || order[1] != "execute" {
		t.Errorf("expected upsert before execute, got order %v", order)
	}
	if !resume {
		t.Error("expected resume=true for existing session")
	}
	if sessID != "prior-session-id" {
		t.Errorf("expected prior-session-id, got %q", sessID)
	}
}
```

- [ ] **Step 4: Run the new tests to verify they fail first**

```bash
go test ./internal/domain/task/... -run "TestTaskDispatcher_FreshSession_PreflightUpsertBeforeExecute|TestTaskDispatcher_ResumeSession_PreflightUpsertBeforeExecute" -v
```

Expected: PASS (the pre-flight logic was already added in Task 2 — if they pass, the implementation is verified).

If they FAIL, something in Task 2 was missed — re-check `executeWithHint` and `resolveExecution`.

- [ ] **Step 5: Run all dispatcher tests**

```bash
go test ./internal/domain/task/... -v -count=1
```

Expected: all pass.

- [ ] **Step 6: Commit**

```bash
git add internal/domain/task/dispatcher_test.go
git commit -m "test: add pre-flight upsert ordering tests for dispatcher"
```

---

### Task 5: Add pre-flight upsert to `feeder.go`

**Files:**
- Modify: `internal/domain/bee/feeder.go:145-200`

**Background:** `processBeeGroup` already resolves `sessionID` before calling `runner.Run()`. We add one `UpsertSessionContext` call between the two.

- [ ] **Step 1: Write the failing test first**

Add this test to `internal/domain/bee/feeder_test.go`:

```go
func TestFeeder_PreflightSessionContextWrittenBeforeRun(t *testing.T) {
	db, ms, ts, ss, es := setupFeederDB(t)
	insertMessage(t, db, "m1", "feishu:c:u", "hello")

	var upsertCalledBeforeRun atomic.Bool

	// We can't intercept store calls directly, so we verify by checking that
	// the session context row exists immediately after Run() is called.
	// Use a runner that checks the DB during its Run() call.
	checkRunner := &checkingBeeRunner{
		onRun: func() {
			ctx := context.Background()
			sid, _, err := ss.GetSessionContext(ctx, "feishu:c:u", store.BeeAgentID)
			if err == nil && sid != "" {
				upsertCalledBeforeRun.Store(true)
			}
		},
	}

	f := newFeeder(ms, ts, ss, es, checkRunner)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	go f.Run(ctx)
	time.Sleep(700 * time.Millisecond)

	if !upsertCalledBeforeRun.Load() {
		t.Error("expected session context to be written before runner.Run() is called")
	}
}
```

Add the helper runner type (add near `mockBeeRunner` in `feeder_test.go`):

```go
type checkingBeeRunner struct {
	onRun func()
}

func (r *checkingBeeRunner) Prepare(_ string, _ ai.PrepareOptions) error { return nil }

func (r *checkingBeeRunner) Run(_ context.Context, _, prompt string, opts ai.RunOptions, logPath string) (ai.Process, <-chan ai.Output, error) {
	if r.onRun != nil {
		r.onRun()
	}
	ch := make(chan ai.Output, 1)
	ch <- ai.Output{Type: ai.OutputDone}
	close(ch)
	return &mockProcess{}, ch, nil
}

func (r *checkingBeeRunner) ExtractResult(_ string) string { return "" }
```

- [ ] **Step 2: Run the test to verify it fails**

```bash
go test ./internal/domain/bee/... -run TestFeeder_PreflightSessionContextWrittenBeforeRun -v
```

Expected: FAIL — `upsertCalledBeforeRun` is false because the upsert hasn't been added yet.

- [ ] **Step 3: Add the pre-flight upsert to `processBeeGroup` in `feeder.go`**

In `internal/domain/bee/feeder.go`, find `processBeeGroup`. After the `sessionID` resolution block (around line 156-160) and before `buildPrompt` / `CreateBeeExecution`, insert:

```go
// Pre-flight: write session context before execution so the session is
// immediately visible and recoverable on crash.
if err := f.sessionStore.UpsertSessionContext(ctx, sessionKey, store.BeeAgentID, sessionID, f.cfg.EffectiveEngine()); err != nil {
    log.Error("pre-flight upsert bee session context", zap.String("sessionKey", sessionKey), zap.Error(err))
    // non-fatal: execution continues
}
```

The exact insertion point is after this existing block:
```go
resume := sessionID != ""
if sessionID == "" {
    sessionID = uuid.New().String()
}
```

And before:
```go
for i, m := range msgs {
```

- [ ] **Step 4: Run the test to verify it passes**

```bash
go test ./internal/domain/bee/... -run TestFeeder_PreflightSessionContextWrittenBeforeRun -v
```

Expected: PASS.

- [ ] **Step 5: Run all feeder tests**

```bash
go test ./internal/domain/bee/... -v -count=1
```

Expected: all pass. In particular `TestFeeder_FirstTick_UsesNewSessionID` should still pass (it checks post-execution persistence, which is unchanged).

- [ ] **Step 6: Commit**

```bash
git add internal/domain/bee/feeder.go internal/domain/bee/feeder_test.go
git commit -m "feat: write bee session context before runner.Run() (pre-flight upsert)"
```

---

### Task 6: Final verification

- [ ] **Step 1: Run the full test suite**

```bash
go test ./... -count=1
```

Expected: all tests pass, no compilation errors.

- [ ] **Step 2: Verify the pre-flight write appears for both Bee and Worker in logs**

If you have a local instance, trigger a first-time bee message and a first-time worker task. In the logs you should see:

For bee (no error line = success):
- No `"pre-flight upsert bee session context"` error log

For worker (no error line = success):
- No `"pre-flight upsert session context"` error log

- [ ] **Step 3: Final commit if any loose ends**

```bash
git add -p  # review any unstaged changes
git commit -m "chore: pre-flight session context write cleanup"
```
