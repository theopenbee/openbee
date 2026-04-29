# `/engine` Command Scoped Busy-Check Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Split `/engine` command's global busy-check into bee-scoped and worker-scoped checks so that switching a specific worker's engine is only blocked by that worker's own activity.

**Architecture:** Add three scoped query methods to `ExecutionStore` and `TaskStore`, introduce `BeeBusyChecker` and `WorkerBusyChecker` interfaces in the command package, then refactor `EngineCommandHandler` to use them. Worker lookup moves before the busy check in the worker-level path so the worker ID is available for scoped queries.

**Tech Stack:** Go, SQLite (`database/sql`), standard library `testing`

---

## File Map

| File | Change |
|------|--------|
| `internal/infra/store/execution_store.go` | Add `HasActiveBeeExecutions`, `HasActiveExecutionsByWorkerID` |
| `internal/infra/store/execution_store_test.go` | Add tests for both new methods |
| `internal/infra/store/task_store.go` | Add `HasActiveImmediateTasksByWorkerID` |
| `internal/infra/store/task_store_test.go` | Add test for new method |
| `internal/domain/command/engine.go` | Replace `SystemBusyChecker` with `BeeBusyChecker`+`WorkerBusyChecker`; refactor handler |
| `internal/domain/command/engine_test.go` | Update fakes + add scoped busy-check tests |
| `internal/app/app.go` | Update wiring |

---

### Task 1: Add `HasActiveBeeExecutions` to ExecutionStore (TDD)

**Files:**
- Modify: `internal/infra/store/execution_store_test.go`
- Modify: `internal/infra/store/execution_store.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/infra/store/execution_store_test.go`:

```go
func TestExecutionStore_HasActiveBeeExecutions(t *testing.T) {
	db, err := InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	es := NewExecutionStore(db, t.TempDir())
	ctx := context.Background()

	// no executions → false
	active, err := es.HasActiveBeeExecutions(ctx)
	if err != nil {
		t.Fatalf("HasActiveBeeExecutions: %v", err)
	}
	if active {
		t.Error("expected false with no executions")
	}

	// create a bee execution (worker_id IS NULL), status pending
	bee, _ := es.CreateBeeExecution("s1", "prompt", "claude")
	active, err = es.HasActiveBeeExecutions(ctx)
	if err != nil {
		t.Fatalf("HasActiveBeeExecutions: %v", err)
	}
	if !active {
		t.Error("expected true with pending bee execution")
	}

	// complete the bee execution → false again
	_ = es.UpdateStatus(bee.ID, model.ExecStatusCompleted)
	active, err = es.HasActiveBeeExecutions(ctx)
	if err != nil {
		t.Fatalf("HasActiveBeeExecutions: %v", err)
	}
	if active {
		t.Error("expected false after completing bee execution")
	}

	// worker execution (worker_id NOT NULL) must not count
	db.Exec(`INSERT INTO bee_workers (id,name,work_dir,status,created_at,updated_at) VALUES ('w1','bot','/','idle',0,0)`)
	_, _ = es.Create("w1", "task", "s2", "claude")
	active, err = es.HasActiveBeeExecutions(ctx)
	if err != nil {
		t.Fatalf("HasActiveBeeExecutions: %v", err)
	}
	if active {
		t.Error("worker execution must not affect HasActiveBeeExecutions")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee2
go test ./internal/infra/store/... -run TestExecutionStore_HasActiveBeeExecutions -v
```

Expected: `FAIL` — `es.HasActiveBeeExecutions undefined`

- [ ] **Step 3: Implement the method**

Append to `internal/infra/store/execution_store.go` (after `HasActiveExecutions`):

```go
// HasActiveBeeExecutions reports whether bee-owned executions (worker_id IS NULL)
// with status pending or running exist.
func (s *ExecutionStore) HasActiveBeeExecutions(ctx context.Context) (bool, error) {
	var exists int
	err := s.db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM bee_executions WHERE worker_id IS NULL AND status IN (?, ?))`,
		model.ExecStatusPending, model.ExecStatusRunning,
	).Scan(&exists)
	return exists == 1, err
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./internal/infra/store/... -run TestExecutionStore_HasActiveBeeExecutions -v
```

Expected: `PASS`

- [ ] **Step 5: Commit**

```bash
git add internal/infra/store/execution_store.go internal/infra/store/execution_store_test.go
git commit -m "feat(store): add HasActiveBeeExecutions to ExecutionStore"
```

---

### Task 2: Add `HasActiveExecutionsByWorkerID` to ExecutionStore (TDD)

**Files:**
- Modify: `internal/infra/store/execution_store_test.go`
- Modify: `internal/infra/store/execution_store.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/infra/store/execution_store_test.go`:

```go
func TestExecutionStore_HasActiveExecutionsByWorkerID(t *testing.T) {
	db, err := InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	es := NewExecutionStore(db, t.TempDir())
	ctx := context.Background()

	db.Exec(`INSERT INTO bee_workers (id,name,work_dir,status,created_at,updated_at) VALUES ('w1','alice','/','idle',0,0)`)
	db.Exec(`INSERT INTO bee_workers (id,name,work_dir,status,created_at,updated_at) VALUES ('w2','bob','/','idle',0,0)`)

	// no executions → false for both workers
	active, err := es.HasActiveExecutionsByWorkerID(ctx, "w1")
	if err != nil {
		t.Fatalf("HasActiveExecutionsByWorkerID: %v", err)
	}
	if active {
		t.Error("expected false with no executions")
	}

	// create pending execution for w1
	exec1, _ := es.Create("w1", "task", "s1", "claude")
	active, err = es.HasActiveExecutionsByWorkerID(ctx, "w1")
	if err != nil {
		t.Fatalf("HasActiveExecutionsByWorkerID: %v", err)
	}
	if !active {
		t.Error("expected true for w1 with pending execution")
	}

	// w2 must not be affected by w1's execution
	active, err = es.HasActiveExecutionsByWorkerID(ctx, "w2")
	if err != nil {
		t.Fatalf("HasActiveExecutionsByWorkerID w2: %v", err)
	}
	if active {
		t.Error("w2 should not be affected by w1's execution")
	}

	// complete w1's execution → false
	_ = es.UpdateStatus(exec1.ID, model.ExecStatusCompleted)
	active, err = es.HasActiveExecutionsByWorkerID(ctx, "w1")
	if err != nil {
		t.Fatalf("HasActiveExecutionsByWorkerID: %v", err)
	}
	if active {
		t.Error("expected false after completing w1 execution")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/infra/store/... -run TestExecutionStore_HasActiveExecutionsByWorkerID -v
```

Expected: `FAIL` — `es.HasActiveExecutionsByWorkerID undefined`

- [ ] **Step 3: Implement the method**

Append to `internal/infra/store/execution_store.go` (after `HasActiveBeeExecutions`):

```go
// HasActiveExecutionsByWorkerID reports whether the given worker has any
// pending or running executions.
func (s *ExecutionStore) HasActiveExecutionsByWorkerID(ctx context.Context, workerID string) (bool, error) {
	var exists int
	err := s.db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM bee_executions WHERE worker_id = ? AND status IN (?, ?))`,
		workerID, model.ExecStatusPending, model.ExecStatusRunning,
	).Scan(&exists)
	return exists == 1, err
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./internal/infra/store/... -run TestExecutionStore_HasActiveExecutionsByWorkerID -v
```

Expected: `PASS`

- [ ] **Step 5: Commit**

```bash
git add internal/infra/store/execution_store.go internal/infra/store/execution_store_test.go
git commit -m "feat(store): add HasActiveExecutionsByWorkerID to ExecutionStore"
```

---

### Task 3: Add `HasActiveImmediateTasksByWorkerID` to TaskStore (TDD)

**Files:**
- Modify: `internal/infra/store/task_store_test.go`
- Modify: `internal/infra/store/task_store.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/infra/store/task_store_test.go`:

```go
func TestTaskStore_HasActiveImmediateTasksByWorkerID(t *testing.T) {
	db, err := InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	defer db.Close()

	// Two workers, two messages
	db.Exec(`INSERT INTO bee_workers (id,name,work_dir,status,created_at,updated_at) VALUES ('w1','alice','/','idle',1,1)`)
	db.Exec(`INSERT INTO bee_workers (id,name,work_dir,status,created_at,updated_at) VALUES ('w2','bob','/','idle',1,1)`)
	db.Exec(`INSERT INTO bee_platform_messages (id,session_key,platform,content,raw,platform_msg_id,received_at,created_at,updated_at) VALUES ('m1','s','feishu','hi','','',1,1,1)`)
	db.Exec(`INSERT INTO bee_platform_messages (id,session_key,platform,content,raw,platform_msg_id,received_at,created_at,updated_at) VALUES ('m2','s2','feishu','hi','','',1,1,1)`)

	ts := NewTaskStore(db)
	ctx := context.Background()

	// no tasks → false
	active, err := ts.HasActiveImmediateTasksByWorkerID(ctx, "w1")
	if err != nil {
		t.Fatalf("HasActiveImmediateTasksByWorkerID: %v", err)
	}
	if active {
		t.Error("expected false with no tasks")
	}

	// create pending immediate task for w1
	now := time.Now().UnixMilli()
	id1, _ := ts.Create(ctx, model.Task{
		MessageID: "m1", WorkerID: "w1", Instruction: "go",
		Type: model.TaskTypeImmediate, Status: model.TaskStatusPending,
		CreatedAt: now, UpdatedAt: now,
	})

	active, err = ts.HasActiveImmediateTasksByWorkerID(ctx, "w1")
	if err != nil {
		t.Fatalf("HasActiveImmediateTasksByWorkerID: %v", err)
	}
	if !active {
		t.Error("expected true for w1 with pending immediate task")
	}

	// w2 must not be affected by w1's task
	active, err = ts.HasActiveImmediateTasksByWorkerID(ctx, "w2")
	if err != nil {
		t.Fatalf("HasActiveImmediateTasksByWorkerID w2: %v", err)
	}
	if active {
		t.Error("w2 should not be affected by w1's task")
	}

	// complete w1's task → false
	_ = ts.UpdateStatus(ctx, id1, model.TaskStatusCompleted)
	active, err = ts.HasActiveImmediateTasksByWorkerID(ctx, "w1")
	if err != nil {
		t.Fatalf("HasActiveImmediateTasksByWorkerID: %v", err)
	}
	if active {
		t.Error("expected false after completing w1 task")
	}

	// scheduled task for w1 must NOT count
	_, _ = ts.Create(ctx, model.Task{
		MessageID: "m2", WorkerID: "w1", Instruction: "cron",
		Type: model.TaskTypeScheduled, Status: model.TaskStatusPending,
		CreatedAt: now, UpdatedAt: now,
	})
	active, err = ts.HasActiveImmediateTasksByWorkerID(ctx, "w1")
	if err != nil {
		t.Fatalf("HasActiveImmediateTasksByWorkerID: %v", err)
	}
	if active {
		t.Error("scheduled task must not affect HasActiveImmediateTasksByWorkerID")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/infra/store/... -run TestTaskStore_HasActiveImmediateTasksByWorkerID -v
```

Expected: `FAIL` — `ts.HasActiveImmediateTasksByWorkerID undefined`

- [ ] **Step 3: Implement the method**

Append to `internal/infra/store/task_store.go` (after `HasActiveImmediateTasks`):

```go
// HasActiveImmediateTasksByWorkerID reports whether the given worker has any
// pending or running immediate tasks.
func (s *TaskStore) HasActiveImmediateTasksByWorkerID(ctx context.Context, workerID string) (bool, error) {
	var exists int
	err := s.db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM bee_tasks WHERE worker_id = ? AND type = ? AND status IN (?, ?))`,
		workerID, model.TaskTypeImmediate, model.TaskStatusPending, model.TaskStatusRunning,
	).Scan(&exists)
	return exists == 1, err
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./internal/infra/store/... -run TestTaskStore_HasActiveImmediateTasksByWorkerID -v
```

Expected: `PASS`

- [ ] **Step 5: Run all store tests to ensure no regressions**

```bash
go test ./internal/infra/store/... -v 2>&1 | tail -20
```

Expected: all `PASS`

- [ ] **Step 6: Commit**

```bash
git add internal/infra/store/task_store.go internal/infra/store/task_store_test.go
git commit -m "feat(store): add HasActiveImmediateTasksByWorkerID to TaskStore"
```

---

### Task 4: Refactor `engine.go` — new interfaces and handler logic

**Files:**
- Modify: `internal/domain/command/engine.go`

- [ ] **Step 1: Replace interfaces and composite types**

In `internal/domain/command/engine.go`, replace the block from `// MessageActivityChecker` through the end of `type compositeBusyChecker struct` (lines 48–83) with:

```go
// MessageActivityChecker reports whether active platform messages exist.
type MessageActivityChecker interface {
	HasActiveMessages(ctx context.Context) (bool, error)
}

// BeeExecutionActivityChecker reports whether active bee-owned executions exist.
type BeeExecutionActivityChecker interface {
	HasActiveBeeExecutions(ctx context.Context) (bool, error)
}

// WorkerExecutionActivityChecker reports whether active executions exist for a specific worker.
type WorkerExecutionActivityChecker interface {
	HasActiveExecutionsByWorkerID(ctx context.Context, workerID string) (bool, error)
}

// WorkerTaskActivityChecker reports whether active immediate tasks exist for a specific worker.
type WorkerTaskActivityChecker interface {
	HasActiveImmediateTasksByWorkerID(ctx context.Context, workerID string) (bool, error)
}

// BeeBusyChecker gates bee-level engine switches.
type BeeBusyChecker interface {
	HasActiveMessages(ctx context.Context) (bool, error)
	HasActiveBeeExecutions(ctx context.Context) (bool, error)
}

// WorkerBusyChecker gates worker-level engine switches.
// All checks are scoped to a single worker by ID.
type WorkerBusyChecker interface {
	HasActiveExecutionsByWorkerID(ctx context.Context, workerID string) (bool, error)
	HasActiveImmediateTasksByWorkerID(ctx context.Context, workerID string) (bool, error)
}

type compositeBeeBusyChecker struct {
	MessageActivityChecker
	BeeExecutionActivityChecker
}

type compositeWorkerBusyChecker struct {
	WorkerExecutionActivityChecker
	WorkerTaskActivityChecker
}

// NewBeeBusyChecker composes a BeeBusyChecker from its two activity checkers.
func NewBeeBusyChecker(msg MessageActivityChecker, exec BeeExecutionActivityChecker) BeeBusyChecker {
	return compositeBeeBusyChecker{msg, exec}
}

// NewWorkerBusyChecker composes a WorkerBusyChecker from its two activity checkers.
func NewWorkerBusyChecker(exec WorkerExecutionActivityChecker, task WorkerTaskActivityChecker) WorkerBusyChecker {
	return compositeWorkerBusyChecker{exec, task}
}
```

- [ ] **Step 2: Update `EngineCommandHandler` struct and constructor**

Replace the `EngineCommandHandler` struct and `NewEngineCommandHandler` function:

```go
// EngineCommandHandler handles the /engine slash command.
type EngineCommandHandler struct {
	workers    WorkerRepository
	sysCfg     SystemConfigWriter
	validator  EngineValidator
	senders    map[string]platform.PlatformSenderAdapter
	beeBusy    BeeBusyChecker
	workerBusy WorkerBusyChecker
	engineCfg  *enginecfg.Store
}

func NewEngineCommandHandler(
	workers WorkerRepository,
	sysCfg SystemConfigWriter,
	senders map[string]platform.PlatformSenderAdapter,
	validator EngineValidator,
	beeBusy BeeBusyChecker,
	workerBusy WorkerBusyChecker,
	engineCfg *enginecfg.Store,
) *EngineCommandHandler {
	return &EngineCommandHandler{
		workers:    workers,
		sysCfg:     sysCfg,
		validator:  validator,
		senders:    senders,
		beeBusy:    beeBusy,
		workerBusy: workerBusy,
		engineCfg:  engineCfg,
	}
}
```

- [ ] **Step 3: Replace `HandleCommand`**

Replace the `HandleCommand` method:

```go
func (h *EngineCommandHandler) HandleCommand(ctx context.Context, content string, replyTo platform.InboundMessage) bool {
	fields := strings.Fields(content)
	if len(fields) == 0 || fields[0] != CmdEngine {
		return false
	}

	if len(fields) == 1 || len(fields) > 3 {
		h.reply(ctx, replyTo, i18n.M.Runtime.EngineCommand.Usage)
		return true
	}

	engineName := fields[1]
	if !h.isValidEngine(ctx, replyTo, engineName) {
		return true
	}

	if len(fields) == 2 {
		if busyMsg, busy := h.checkBeeBusy(ctx); busy {
			h.reply(ctx, replyTo, busyMsg)
			return true
		}
		h.handleBeeEngine(ctx, replyTo, engineName)
	} else {
		workerName := fields[2]
		m := i18n.M.Runtime.EngineCommand
		w, err := h.workers.GetByName(workerName)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				h.reply(ctx, replyTo, fmt.Sprintf(m.WorkerNotFound, workerName))
			} else {
				log.Error("get worker by name for /engine", zap.String("name", workerName), zap.Error(err))
				h.reply(ctx, replyTo, m.SwitchFailed)
			}
			return true
		}
		if busyMsg, busy := h.checkWorkerBusy(ctx, w.ID); busy {
			h.reply(ctx, replyTo, busyMsg)
			return true
		}
		h.handleWorkerEngine(ctx, replyTo, engineName, w)
	}
	return true
}
```

- [ ] **Step 4: Replace `checkBusy` with two scoped methods**

Remove the existing `checkBusy` method and add:

```go
func (h *EngineCommandHandler) checkBeeBusy(ctx context.Context) (string, bool) {
	m := i18n.M.Runtime.EngineCommand
	checks := []struct {
		fn   func(context.Context) (bool, error)
		busy string
		warn string
	}{
		{h.beeBusy.HasActiveMessages, m.BusyMessages, "engine command: failed to check active messages"},
		{h.beeBusy.HasActiveBeeExecutions, m.BusyExecutions, "engine command: failed to check active bee executions"},
	}
	for _, c := range checks {
		active, err := c.fn(ctx)
		if err != nil {
			log.Warn(c.warn, zap.Error(err))
			continue
		}
		if active {
			return c.busy, true
		}
	}
	return "", false
}

func (h *EngineCommandHandler) checkWorkerBusy(ctx context.Context, workerID string) (string, bool) {
	m := i18n.M.Runtime.EngineCommand
	execActive, err := h.workerBusy.HasActiveExecutionsByWorkerID(ctx, workerID)
	if err != nil {
		log.Warn("engine command: failed to check active worker executions", zap.Error(err))
	} else if execActive {
		return m.BusyExecutions, true
	}
	taskActive, err := h.workerBusy.HasActiveImmediateTasksByWorkerID(ctx, workerID)
	if err != nil {
		log.Warn("engine command: failed to check active worker tasks", zap.Error(err))
	} else if taskActive {
		return m.BusyTasks, true
	}
	return "", false
}
```

- [ ] **Step 5: Update `handleWorkerEngine` signature**

Replace `handleWorkerEngine` — it no longer needs to look up the worker since `HandleCommand` does it now:

```go
func (h *EngineCommandHandler) handleWorkerEngine(ctx context.Context, replyTo platform.InboundMessage, engineName string, w model.Worker) {
	m := i18n.M.Runtime.EngineCommand
	if err := h.workers.UpdateEngine(w.ID, engineName); err != nil {
		h.reply(ctx, replyTo, m.SwitchFailed)
		return
	}
	h.reply(ctx, replyTo, fmt.Sprintf(m.WorkerSwitched, w.Name, engineName))
}
```

- [ ] **Step 6: Verify the file compiles (tests will fail — that's expected)**

```bash
go build ./internal/domain/command/...
```

Expected: build errors from `engine_test.go` referencing old types — that's fine for now.

---

### Task 5: Update `engine_test.go`

**Files:**
- Modify: `internal/domain/command/engine_test.go`

- [ ] **Step 1: Replace `fakeActivityChecker` with two scoped fakes**

Replace the existing `fakeActivityChecker` struct and `notBusy` var (lines 81–97) with:

```go
type fakeBeeBusyChecker struct {
	activeMessages   bool
	activeBeeExecs   bool
	err              error
}

func (f *fakeBeeBusyChecker) HasActiveMessages(_ context.Context) (bool, error) {
	return f.activeMessages, f.err
}
func (f *fakeBeeBusyChecker) HasActiveBeeExecutions(_ context.Context) (bool, error) {
	return f.activeBeeExecs, f.err
}

type fakeWorkerBusyChecker struct {
	activeExecs bool
	activeTasks bool
	err         error
}

func (f *fakeWorkerBusyChecker) HasActiveExecutionsByWorkerID(_ context.Context, _ string) (bool, error) {
	return f.activeExecs, f.err
}
func (f *fakeWorkerBusyChecker) HasActiveImmediateTasksByWorkerID(_ context.Context, _ string) (bool, error) {
	return f.activeTasks, f.err
}

var notBeeBusy    = &fakeBeeBusyChecker{}
var notWorkerBusy = &fakeWorkerBusyChecker{}
```

- [ ] **Step 2: Update `makeHandler`**

Replace the `makeHandler` function:

```go
func makeHandler(workers map[string]model.Worker) (*command.EngineCommandHandler, *fakeSender, *fakeSysConfig, *enginecfg.Store) {
	sender := &fakeSender{}
	cfg := &fakeSysConfig{vals: make(map[string]string)}
	repo := &fakeWorkerRepo{workers: workers}
	senders := map[string]platform.PlatformSenderAdapter{"feishu": sender}
	beeBusy := command.NewBeeBusyChecker(notBeeBusy, notBeeBusy)
	workerBusy := command.NewWorkerBusyChecker(notWorkerBusy, notWorkerBusy)
	engineCfg := enginecfg.NewStore("")
	h := command.NewEngineCommandHandler(repo, cfg, senders, defaultValidator, beeBusy, workerBusy, engineCfg)
	return h, sender, cfg, engineCfg
}
```

- [ ] **Step 3: Replace old busy tests with scoped tests**

Remove `TestEngineCommand_BusyMessages`, `TestEngineCommand_BusyExecutions`, `TestEngineCommand_BusyTasks`, `TestEngineCommand_BusyBlocksWorkerSwitch`, `TestEngineCommand_BusyDoesNotBlockUsage`, and `TestEngineCommand_SwitchBeeEngine_DBError`, `TestEngineCommand_SwitchWorkerEngine_UpdateError` (keep the last two if they don't reference old types — check first).

The tests that reference `command.NewSystemBusyChecker` must be updated. Replace all of them with:

```go
func TestEngineCommand_BeeBusy_ActiveMessages(t *testing.T) {
	sender := &fakeSender{}
	cfg := &fakeSysConfig{vals: make(map[string]string)}
	repo := &fakeWorkerRepo{workers: map[string]model.Worker{}}
	senders := map[string]platform.PlatformSenderAdapter{"feishu": sender}
	beeBusy := command.NewBeeBusyChecker(&fakeBeeBusyChecker{activeMessages: true}, notBeeBusy)
	workerBusy := command.NewWorkerBusyChecker(notWorkerBusy, notWorkerBusy)
	h := command.NewEngineCommandHandler(repo, cfg, senders, defaultValidator, beeBusy, workerBusy, enginecfg.NewStore(""))

	handled := h.HandleCommand(context.Background(), "/engine codex", makeReplyTo())
	if !handled {
		t.Fatal("expected handled=true")
	}
	want := "当前有消息正在接收或处理中，无法切换引擎，请等待完成后再试。"
	if len(sender.sent) != 1 || sender.sent[0] != want {
		t.Errorf("unexpected reply: %v", sender.sent)
	}
}

func TestEngineCommand_BeeBusy_ActiveBeeExecutions(t *testing.T) {
	sender := &fakeSender{}
	cfg := &fakeSysConfig{vals: make(map[string]string)}
	repo := &fakeWorkerRepo{workers: map[string]model.Worker{}}
	senders := map[string]platform.PlatformSenderAdapter{"feishu": sender}
	beeBusy := command.NewBeeBusyChecker(notBeeBusy, &fakeBeeBusyChecker{activeBeeExecs: true})
	workerBusy := command.NewWorkerBusyChecker(notWorkerBusy, notWorkerBusy)
	h := command.NewEngineCommandHandler(repo, cfg, senders, defaultValidator, beeBusy, workerBusy, enginecfg.NewStore(""))

	handled := h.HandleCommand(context.Background(), "/engine codex", makeReplyTo())
	if !handled {
		t.Fatal("expected handled=true")
	}
	want := "当前有执行中的 execution，无法切换引擎，请等待完成后再试。"
	if len(sender.sent) != 1 || sender.sent[0] != want {
		t.Errorf("unexpected reply: %v", sender.sent)
	}
}

func TestEngineCommand_WorkerBusy_ActiveExecutions(t *testing.T) {
	workers := map[string]model.Worker{"alice": {ID: "w1", Name: "alice", Engine: "claude"}}
	sender := &fakeSender{}
	cfg := &fakeSysConfig{vals: make(map[string]string)}
	repo := &fakeWorkerRepo{workers: workers}
	senders := map[string]platform.PlatformSenderAdapter{"feishu": sender}
	beeBusy := command.NewBeeBusyChecker(notBeeBusy, notBeeBusy)
	workerBusy := command.NewWorkerBusyChecker(&fakeWorkerBusyChecker{activeExecs: true}, notWorkerBusy)
	h := command.NewEngineCommandHandler(repo, cfg, senders, defaultValidator, beeBusy, workerBusy, enginecfg.NewStore(""))

	handled := h.HandleCommand(context.Background(), "/engine codex alice", makeReplyTo())
	if !handled {
		t.Fatal("expected handled=true")
	}
	want := "当前有执行中的 execution，无法切换引擎，请等待完成后再试。"
	if len(sender.sent) != 1 || sender.sent[0] != want {
		t.Errorf("unexpected reply: %v", sender.sent)
	}
}

func TestEngineCommand_WorkerBusy_ActiveTasks(t *testing.T) {
	workers := map[string]model.Worker{"alice": {ID: "w1", Name: "alice", Engine: "claude"}}
	sender := &fakeSender{}
	cfg := &fakeSysConfig{vals: make(map[string]string)}
	repo := &fakeWorkerRepo{workers: workers}
	senders := map[string]platform.PlatformSenderAdapter{"feishu": sender}
	beeBusy := command.NewBeeBusyChecker(notBeeBusy, notBeeBusy)
	workerBusy := command.NewWorkerBusyChecker(notWorkerBusy, &fakeWorkerBusyChecker{activeTasks: true})
	h := command.NewEngineCommandHandler(repo, cfg, senders, defaultValidator, beeBusy, workerBusy, enginecfg.NewStore(""))

	handled := h.HandleCommand(context.Background(), "/engine codex alice", makeReplyTo())
	if !handled {
		t.Fatal("expected handled=true")
	}
	want := "当前有即时任务正在等待或执行中，无法切换引擎，请等待完成后再试。"
	if len(sender.sent) != 1 || sender.sent[0] != want {
		t.Errorf("unexpected reply: %v", sender.sent)
	}
}

func TestEngineCommand_WorkerSwitch_NotBlockedByOtherWorker(t *testing.T) {
	// KEY scenario: alice is free, but bob is busy — alice's switch must succeed
	workers := map[string]model.Worker{"alice": {ID: "w1", Name: "alice", Engine: "claude"}}
	sender := &fakeSender{}
	cfg := &fakeSysConfig{vals: make(map[string]string)}
	repo := &fakeWorkerRepo{workers: workers}
	senders := map[string]platform.PlatformSenderAdapter{"feishu": sender}
	// beeBusy has active messages (bee is busy) — but worker switch should not care
	beeBusy := command.NewBeeBusyChecker(&fakeBeeBusyChecker{activeMessages: true}, notBeeBusy)
	workerBusy := command.NewWorkerBusyChecker(notWorkerBusy, notWorkerBusy)
	h := command.NewEngineCommandHandler(repo, cfg, senders, defaultValidator, beeBusy, workerBusy, enginecfg.NewStore(""))

	handled := h.HandleCommand(context.Background(), "/engine codex alice", makeReplyTo())
	if !handled {
		t.Fatal("expected handled=true")
	}
	// Should succeed, not be blocked
	want := `已将 Worker "alice" 的 engine 切换为 codex`
	if len(sender.sent) != 1 || sender.sent[0] != want {
		t.Errorf("unexpected reply: %v", sender.sent)
	}
}

func TestEngineCommand_BusyDoesNotBlockUsage(t *testing.T) {
	sender := &fakeSender{}
	cfg := &fakeSysConfig{vals: make(map[string]string)}
	repo := &fakeWorkerRepo{}
	senders := map[string]platform.PlatformSenderAdapter{"feishu": sender}
	beeBusy := command.NewBeeBusyChecker(&fakeBeeBusyChecker{activeMessages: true, activeBeeExecs: true}, &fakeBeeBusyChecker{activeBeeExecs: true})
	workerBusy := command.NewWorkerBusyChecker(&fakeWorkerBusyChecker{activeExecs: true}, &fakeWorkerBusyChecker{activeTasks: true})
	h := command.NewEngineCommandHandler(repo, cfg, senders, defaultValidator, beeBusy, workerBusy, enginecfg.NewStore(""))

	handled := h.HandleCommand(context.Background(), "/engine", makeReplyTo())
	if !handled {
		t.Fatal("expected handled=true")
	}
	want := "用法：\n/engine {engine} — 切换默认 engine\n/engine {engine} {workerName} — 切换指定 worker 的 engine"
	if len(sender.sent) != 1 || sender.sent[0] != want {
		t.Errorf("unexpected reply: %v", sender.sent)
	}
}

func TestEngineCommand_SwitchBeeEngine_DBError(t *testing.T) {
	engineCfg := enginecfg.NewStore("claude")
	sender := &fakeSender{}
	cfg := &fakeSysConfig{vals: make(map[string]string), setErr: errors.New("db error")}
	repo := &fakeWorkerRepo{workers: map[string]model.Worker{}}
	senders := map[string]platform.PlatformSenderAdapter{"feishu": sender}
	beeBusy := command.NewBeeBusyChecker(notBeeBusy, notBeeBusy)
	workerBusy := command.NewWorkerBusyChecker(notWorkerBusy, notWorkerBusy)
	h := command.NewEngineCommandHandler(repo, cfg, senders, defaultValidator, beeBusy, workerBusy, engineCfg)

	handled := h.HandleCommand(context.Background(), "/engine codex", makeReplyTo())
	if !handled {
		t.Fatal("expected handled=true")
	}
	if engineCfg.Get() != "claude" {
		t.Errorf("expected engineCfg to remain claude, got %s", engineCfg.Get())
	}
	want := "切换失败，请稍后重试"
	if len(sender.sent) != 1 || sender.sent[0] != want {
		t.Errorf("unexpected reply: %v", sender.sent)
	}
}

func TestEngineCommand_SwitchWorkerEngine_UpdateError(t *testing.T) {
	workers := map[string]model.Worker{"alice": {ID: "w1", Name: "alice", Engine: "claude"}}
	sender := &fakeSender{}
	cfg := &fakeSysConfig{vals: make(map[string]string)}
	repo := &fakeWorkerRepo{workers: workers, updateErr: errors.New("update error")}
	senders := map[string]platform.PlatformSenderAdapter{"feishu": sender}
	beeBusy := command.NewBeeBusyChecker(notBeeBusy, notBeeBusy)
	workerBusy := command.NewWorkerBusyChecker(notWorkerBusy, notWorkerBusy)
	h := command.NewEngineCommandHandler(repo, cfg, senders, defaultValidator, beeBusy, workerBusy, enginecfg.NewStore(""))

	handled := h.HandleCommand(context.Background(), "/engine codex alice", makeReplyTo())
	if !handled {
		t.Fatal("expected handled=true")
	}
	want := "切换失败，请稍后重试"
	if len(sender.sent) != 1 || sender.sent[0] != want {
		t.Errorf("unexpected reply: %v", sender.sent)
	}
}
```

- [ ] **Step 4: Run command tests**

```bash
go test ./internal/domain/command/... -v
```

Expected: all `PASS`

- [ ] **Step 5: Commit**

```bash
git add internal/domain/command/engine.go internal/domain/command/engine_test.go
git commit -m "feat(command): split /engine busy-check into bee-scoped and worker-scoped checkers"
```

---

### Task 6: Update `app.go` wiring

**Files:**
- Modify: `internal/app/app.go`

- [ ] **Step 1: Replace the wiring lines**

In `internal/app/app.go` around line 157, replace:

```go
busyChecker := command.NewSystemBusyChecker(s.msgStore, s.execStore, s.taskStore)
engineCmdHandler := command.NewEngineCommandHandler(s.workerStore, s.systemConfigStore, sendersByPlatform, mgr, busyChecker, engineCfg)
```

with:

```go
beeBusy := command.NewBeeBusyChecker(s.msgStore, s.execStore)
workerBusy := command.NewWorkerBusyChecker(s.execStore, s.taskStore)
engineCmdHandler := command.NewEngineCommandHandler(s.workerStore, s.systemConfigStore, sendersByPlatform, mgr, beeBusy, workerBusy, engineCfg)
```

- [ ] **Step 2: Build to verify no compilation errors**

```bash
go build ./...
```

Expected: clean build, no errors

- [ ] **Step 3: Run all tests**

```bash
go test ./... 2>&1 | tail -30
```

Expected: all packages `ok`, no `FAIL`

- [ ] **Step 4: Commit**

```bash
git add internal/app/app.go
git commit -m "feat(app): wire BeeBusyChecker and WorkerBusyChecker for /engine command"
```

---

## Self-Review Checklist

- [x] **Spec coverage:** All five spec sections covered — store methods (Tasks 1–3), interfaces (Task 4), handler logic (Task 4), wiring (Task 6), tests (Task 5)
- [x] **Placeholder scan:** No TBDs, all steps have actual code
- [x] **Type consistency:** `NewBeeBusyChecker`, `NewWorkerBusyChecker`, `fakeBeeBusyChecker`, `fakeWorkerBusyChecker` names are consistent across Tasks 4, 5, 6
- [x] **`handleWorkerEngine` signature:** Takes `model.Worker` in Task 4 Step 5 — matches call site in Task 4 Step 3
- [x] **`notBeeBusy` / `notWorkerBusy`:** Both implement their respective interfaces; `NewBeeBusyChecker(notBeeBusy, notBeeBusy)` works because `fakeBeeBusyChecker` satisfies both `MessageActivityChecker` and `BeeExecutionActivityChecker`
