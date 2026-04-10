# Worker Persona Injection on New Session Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** On new Worker sessions, inject the worker's name/description/memory alongside the skill hint prefix so the agent has full identity context from its first instruction.

**Architecture:** Add a `WorkerLookup` narrow interface to `TaskDispatcher` (injected via `WithWorkerLookup` option). On new/fresh sessions, `resolveExecution` calls a new `workerSkillHint` method that fetches worker metadata via the interface and builds a `<worker_persona>` block. Resume sessions are unchanged. Hard-fail if lookup returns an error.

**Tech Stack:** Go, SQLite (`store.WorkerStore` satisfies `WorkerLookup` without modification)

---

## File Structure

| File | Change |
|------|--------|
| `internal/domain/task/dispatcher.go` | Add `WorkerLookup` interface, `workerLookup` field, `WithWorkerLookup` option, `workerSkillHint` method; update `resolveExecution` |
| `internal/domain/task/dispatcher_test.go` | Add tests for persona injection with `WithWorkerLookup`, nil lookup fallback, and hard-fail on lookup error |
| `internal/app/app.go` | Pass `WithWorkerLookup(s.workerStore)` in `buildPipeline` |

`internal/ai/rules.go` — `WorkerPersona()` and `SkillHintPrefix()` are reused as-is, no changes needed.

---

### Task 1: Add WorkerLookup interface, field, and option to TaskDispatcher

**Files:**
- Modify: `internal/domain/task/dispatcher.go`

- [ ] **Step 1: Add the interface and field**

Open `internal/domain/task/dispatcher.go`. After the existing interface block (after `FailureNotifier`), add:

```go
// WorkerLookup fetches worker metadata for persona injection on new sessions.
type WorkerLookup interface {
	GetByID(id string) (model.Worker, error)
}
```

Add `workerLookup WorkerLookup` to the `TaskDispatcher` struct, after `engineName`:

```go
type TaskDispatcher struct {
	ctx              context.Context
	manager          ExecutionManager
	taskStore        TaskStore
	sessionStore     SessionStore
	execStore        ExecutionQuerier
	failureNotifier  FailureNotifier
	engineName       string
	workerLookup     WorkerLookup                     // optional; if nil, only skill hint is injected
	inCh             <-chan DispatchTask
	resultsCh        chan internalResult
	queues           map[string]*queueState
	clearCh          chan string
	cancelFuncs      map[string]context.CancelFunc
	cancelCh         chan string
}
```

- [ ] **Step 2: Add WithWorkerLookup option**

After the `WithEngine` option function, add:

```go
// WithWorkerLookup sets the lookup used to fetch worker metadata for persona injection.
func WithWorkerLookup(lookup WorkerLookup) Option {
	return func(d *TaskDispatcher) { d.workerLookup = lookup }
}
```

- [ ] **Step 3: Build and verify no compilation errors**

```bash
go build ./internal/domain/task/...
```

Expected: compiles with no errors.

- [ ] **Step 4: Commit**

```bash
git add internal/domain/task/dispatcher.go
git commit -m "feat(task): add WorkerLookup interface and WithWorkerLookup option"
```

---

### Task 2: Add workerSkillHint method and update resolveExecution

**Files:**
- Modify: `internal/domain/task/dispatcher.go`
- Modify: `internal/domain/task/dispatcher_test.go`

- [ ] **Step 1: Write the failing tests**

Add these three test functions to `internal/domain/task/dispatcher_test.go`.

First, add a `mockWorkerLookup` helper at the top of the mocks section (after `mockFailureNotifier`):

```go
type mockWorkerLookup struct {
	worker model.Worker
	err    error
}

func (m *mockWorkerLookup) GetByID(_ string) (model.Worker, error) {
	return m.worker, m.err
}
```

Then add the tests:

```go
func TestTaskDispatcher_NewSession_InjectsWorkerPersona(t *testing.T) {
	mgr := &mockExecManager{
		execResult: model.WorkerExecution{ID: "exec-1", SessionID: "sess-1", Status: model.ExecStatusCompleted},
	}
	eq := &mockExecutionQuerier{result: model.WorkerExecution{ID: "exec-1", Status: model.ExecStatusCompleted}}
	lookup := &mockWorkerLookup{
		worker: model.Worker{
			ID:          "w1",
			Name:        "毛毛",
			Description: "负责 openbee 开发",
			Memory:      "记住老板的偏好",
		},
	}
	d, in, _ := newTaskDispatcher(mgr, eq, newMockSessionStore(),
		task.WithWorkerLookup(lookup),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go d.Run(ctx)

	in <- immediateTask("s1", "w1", "do the thing")

	if !waitForExecCount(mgr, 1, 2*time.Second) {
		t.Fatal("ExecuteWorker was not called within timeout")
	}

	mgr.mu.Lock()
	instr := mgr.executedInstructions[0]
	mgr.mu.Unlock()

	if !strings.HasPrefix(instr, "use openbee-worker skill.") {
		t.Errorf("instruction missing skill hint prefix, got: %q", instr)
	}
	if !strings.Contains(instr, "<worker_persona>") {
		t.Errorf("instruction missing <worker_persona> tag, got: %q", instr)
	}
	if !strings.Contains(instr, "Name: 毛毛") {
		t.Errorf("instruction missing worker name, got: %q", instr)
	}
	if !strings.Contains(instr, "Description: 负责 openbee 开发") {
		t.Errorf("instruction missing worker description, got: %q", instr)
	}
	if !strings.Contains(instr, "记住老板的偏好") {
		t.Errorf("instruction missing worker memory, got: %q", instr)
	}
	if !strings.Contains(instr, "</worker_persona>") {
		t.Errorf("instruction missing </worker_persona> tag, got: %q", instr)
	}
}

func TestTaskDispatcher_NewSession_NilLookup_OnlySkillHint(t *testing.T) {
	mgr := &mockExecManager{
		execResult: model.WorkerExecution{ID: "exec-1", SessionID: "sess-1", Status: model.ExecStatusCompleted},
	}
	eq := &mockExecutionQuerier{result: model.WorkerExecution{ID: "exec-1", Status: model.ExecStatusCompleted}}
	// No WithWorkerLookup — workerLookup is nil
	d, in, _ := newTaskDispatcher(mgr, eq, newMockSessionStore())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go d.Run(ctx)

	in <- immediateTask("s1", "w1", "do the thing")

	if !waitForExecCount(mgr, 1, 2*time.Second) {
		t.Fatal("ExecuteWorker was not called within timeout")
	}

	mgr.mu.Lock()
	instr := mgr.executedInstructions[0]
	mgr.mu.Unlock()

	if !strings.HasPrefix(instr, "use openbee-worker skill.") {
		t.Errorf("instruction missing skill hint prefix, got: %q", instr)
	}
	if strings.Contains(instr, "<worker_persona>") {
		t.Errorf("instruction should not contain <worker_persona> when lookup is nil, got: %q", instr)
	}
}

func TestTaskDispatcher_NewSession_LookupError_FailsTask(t *testing.T) {
	mgr := &mockExecManager{
		execResult: model.WorkerExecution{ID: "exec-1"},
	}
	eq := &mockExecutionQuerier{result: model.WorkerExecution{ID: "exec-1", Status: model.ExecStatusCompleted}}
	lookup := &mockWorkerLookup{err: fmt.Errorf("worker not found")}
	notifier := &mockFailureNotifier{}
	d, in, ts := newTaskDispatcher(mgr, eq, newMockSessionStore(),
		task.WithWorkerLookup(lookup),
		task.WithFailureNotifier(notifier),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go d.Run(ctx)

	t1 := immediateTask("s1", "w1", "do the thing")
	t1.TaskID = "task-fail"
	t1.MessageID = "msg-fail"
	in <- t1

	if !notifier.waitForCall(2 * time.Second) {
		t.Fatal("failure notifier was not called within timeout")
	}

	ts.mu.Lock()
	failed := ts.failedTasks
	ts.mu.Unlock()
	if len(failed) == 0 || failed[0] != "task-fail" {
		t.Errorf("expected task-fail to be failed, got %v", failed)
	}
	// ExecuteWorker must NOT have been called
	mgr.mu.Lock()
	execCount := len(mgr.executedInstructions)
	mgr.mu.Unlock()
	if execCount != 0 {
		t.Errorf("ExecuteWorker should not be called on lookup error, got %d calls", execCount)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/domain/task/... -run "TestTaskDispatcher_NewSession" -v
```

Expected: FAIL — `workerSkillHint` method does not exist yet; tests for persona injection will fail because instruction won't contain `<worker_persona>`.

- [ ] **Step 3: Add workerSkillHint method**

In `internal/domain/task/dispatcher.go`, add this method before `resolveExecution`:

```go
// workerSkillHint returns the skill hint for a new worker session.
// If workerLookup is set, it fetches worker metadata and wraps it in a <worker_persona> block.
// Returns an error if the lookup fails.
func (d *TaskDispatcher) workerSkillHint(workerID string) (string, error) {
	hint := ai.SkillHintPrefix(ai.RoleWorker)
	if d.workerLookup == nil {
		return hint, nil
	}
	w, err := d.workerLookup.GetByID(workerID)
	if err != nil {
		return "", fmt.Errorf("lookup worker for persona hint: %w", err)
	}
	persona := ai.WorkerPersona(w.Name, w.Description, w.Memory)
	return hint + "\n<worker_persona>\n" + persona + "</worker_persona>", nil
}
```

- [ ] **Step 4: Update resolveExecution to use workerSkillHint**

Replace the entire `resolveExecution` method body with:

```go
func (d *TaskDispatcher) resolveExecution(ctx context.Context, task DispatchTask, instruction string) (model.WorkerExecution, error) {
	if task.TaskType != model.TaskTypeImmediate {
		hint, err := d.workerSkillHint(task.WorkerID)
		if err != nil {
			return model.WorkerExecution{}, err
		}
		log.Info("executing worker", zap.String("workerID", task.WorkerID), zap.String("taskID", task.TaskID))
		return d.manager.ExecuteWorker(ctx, task.WorkerID, hint+"\n"+instruction, "")
	}
	sessionID, err := d.sessionStore.GetSessionContextForEngine(ctx, task.SessionKey, task.WorkerID, d.engineName)
	if err != nil {
		log.Error("get session context", zap.Error(err))
	}
	if sessionID == "" {
		hint, err := d.workerSkillHint(task.WorkerID)
		if err != nil {
			return model.WorkerExecution{}, err
		}
		log.Info("executing worker", zap.String("workerID", task.WorkerID), zap.String("taskID", task.TaskID))
		return d.manager.ExecuteWorker(ctx, task.WorkerID, hint+"\n"+instruction, "")
	}
	log.Info("resuming session", zap.String("sessionID", sessionID), zap.String("taskID", task.TaskID))
	exec, err := d.manager.ExecuteWorker(ctx, task.WorkerID, instruction, sessionID)
	if err == nil {
		return exec, nil
	}
	log.Error("resume error, falling back to fresh", zap.Error(err))
	if clearErr := d.sessionStore.ClearSessionContexts(ctx, task.SessionKey); clearErr != nil {
		log.Error("clear stale session contexts", zap.String("sessionKey", task.SessionKey), zap.Error(clearErr))
	}
	hint, err := d.workerSkillHint(task.WorkerID)
	if err != nil {
		return model.WorkerExecution{}, err
	}
	return d.manager.ExecuteWorker(ctx, task.WorkerID, hint+"\n"+instruction, "")
}
```

- [ ] **Step 5: Run tests to verify they pass**

```bash
go test ./internal/domain/task/... -run "TestTaskDispatcher_NewSession" -v
```

Expected: all three `TestTaskDispatcher_NewSession_*` tests PASS.

- [ ] **Step 6: Run the full dispatcher test suite**

```bash
go test ./internal/domain/task/... -v
```

Expected: all tests PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/domain/task/dispatcher.go internal/domain/task/dispatcher_test.go
git commit -m "feat(task): inject worker persona on new session via WorkerLookup"
```

---

### Task 3: Wire WorkerLookup into buildPipeline

**Files:**
- Modify: `internal/app/app.go`

- [ ] **Step 1: Pass WithWorkerLookup in buildPipeline**

In `internal/app/app.go`, find the `buildPipeline` function signature:

```go
func buildPipeline(
	debounce time.Duration,
	engineName string,
	s appStores,
	mgr *worker.Manager,
	dispatchCh chan task.DispatchTask,
	failureNotifier task.FailureNotifier,
) (*msgingest.Gateway, *task.TaskDispatcher) {
	ingest := msgingest.New(s.msgStore, debounce)
	disp := task.New(mgr, s.taskStore, s.sessionStore, s.execStore, dispatchCh,
		task.WithFailureNotifier(failureNotifier),
		task.WithEngine(engineName),
	)
	return ingest, disp
}
```

Replace with:

```go
func buildPipeline(
	debounce time.Duration,
	engineName string,
	s appStores,
	mgr *worker.Manager,
	dispatchCh chan task.DispatchTask,
	failureNotifier task.FailureNotifier,
) (*msgingest.Gateway, *task.TaskDispatcher) {
	ingest := msgingest.New(s.msgStore, debounce)
	disp := task.New(mgr, s.taskStore, s.sessionStore, s.execStore, dispatchCh,
		task.WithFailureNotifier(failureNotifier),
		task.WithEngine(engineName),
		task.WithWorkerLookup(s.workerStore),
	)
	return ingest, disp
}
```

- [ ] **Step 2: Build the full binary**

```bash
go build ./...
```

Expected: compiles with no errors.

- [ ] **Step 3: Run all tests**

```bash
go test ./...
```

Expected: all tests PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/app/app.go
git commit -m "feat(app): wire WorkerLookup into task dispatcher"
```
