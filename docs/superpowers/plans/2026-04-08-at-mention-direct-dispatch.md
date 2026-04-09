# @姓名 Direct Dispatch Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** When a platform message starts with `@name `, route it directly to the named Worker without going through Bee scheduling.

**Architecture:** A helper function `parseDirectMention` detects the `@name ` prefix in the Feeder's `processBeeGroup`, looks up the Worker by name (case-insensitive), and dispatches a `DispatchTask` directly to the existing `TaskDispatcher` channel. If the Worker is not found, execution falls back to the normal Bee flow. Session continuity is maintained through the same `GetSessionContext`/`UpsertSessionContext` mechanism used by immediate tasks. The `buildInstruction` function in the dispatcher is updated to inject `message_id` even when there is no `task_id`, so directly-dispatched Workers can still call `send_message`.

**Tech Stack:** Go, SQLite (`database/sql`), `strings` stdlib (no new dependencies)

---

## File Map

| File | Change |
|---|---|
| `internal/infra/store/worker_store.go` | Add `GetByName(name string) (model.Worker, error)` |
| `internal/infra/store/worker_store_test.go` | Add tests for `GetByName` |
| `internal/domain/task/dispatcher.go` | Modify `buildInstruction` to emit `message_id` even without `task_id` |
| `internal/domain/task/dispatcher_test.go` | Add test verifying `message_id` header without `task_id` |
| `internal/domain/bee/feeder.go` | Add `WorkerNameLookup` interface, `WithDirectDispatch` option, `parseDirectMention` helper, `tryDirectDispatch` method; call from `processBeeGroup` |
| `internal/domain/bee/feeder_test.go` | Add three tests for direct dispatch behavior |
| `internal/app/app.go` | Pass `dispatchCh` and `workerStore` to Feeder via `WithDirectDispatch` |

---

## Task 1: Add `WorkerStore.GetByName`

**Files:**
- Modify: `internal/infra/store/worker_store.go`
- Modify: `internal/infra/store/worker_store_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `internal/infra/store/worker_store_test.go`:

```go
func TestWorkerStore_GetByName_ExactMatch(t *testing.T) {
	s := setupTestDB(t)
	s.Create(model.Worker{Name: "天天", WorkDir: "/tmp/tt"})

	got, err := s.GetByName("天天")
	if err != nil {
		t.Fatalf("GetByName: %v", err)
	}
	if got.Name != "天天" {
		t.Errorf("expected 天天, got %s", got.Name)
	}
}

func TestWorkerStore_GetByName_CaseInsensitive(t *testing.T) {
	s := setupTestDB(t)
	s.Create(model.Worker{Name: "Alice", WorkDir: "/tmp/alice"})

	got, err := s.GetByName("alice")
	if err != nil {
		t.Fatalf("GetByName lowercase: %v", err)
	}
	if got.Name != "Alice" {
		t.Errorf("expected Alice, got %s", got.Name)
	}

	got2, err := s.GetByName("ALICE")
	if err != nil {
		t.Fatalf("GetByName uppercase: %v", err)
	}
	if got2.Name != "Alice" {
		t.Errorf("expected Alice, got %s", got2.Name)
	}
}

func TestWorkerStore_GetByName_NotFound(t *testing.T) {
	s := setupTestDB(t)

	_, err := s.GetByName("nobody")
	if err == nil {
		t.Fatal("expected error for missing worker, got nil")
	}
}

func TestWorkerStore_GetByName_DuplicateName_ReturnsEarliest(t *testing.T) {
	s := setupTestDB(t)
	first, _ := s.Create(model.Worker{Name: "Bot", WorkDir: "/tmp/bot1"})
	s.Create(model.Worker{Name: "Bot", WorkDir: "/tmp/bot2"})

	got, err := s.GetByName("bot")
	if err != nil {
		t.Fatalf("GetByName: %v", err)
	}
	if got.ID != first.ID {
		t.Errorf("expected earliest worker %s, got %s", first.ID, got.ID)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee2
go test ./internal/infra/store/ -run TestWorkerStore_GetByName -v
```

Expected: FAIL — `s.GetByName undefined`

- [ ] **Step 3: Implement `GetByName`**

Add to `internal/infra/store/worker_store.go` after `GetByID`:

```go
// GetByName returns the worker with the given name (case-insensitive).
// If multiple workers share the same name, the one created earliest is returned.
// Returns an error (sql.ErrNoRows) if no worker is found.
func (s *WorkerStore) GetByName(name string) (model.Worker, error) {
	row := s.db.QueryRow(
		`SELECT `+workerColumns+` FROM bee_workers
		 WHERE LOWER(name) = LOWER(?)
		 ORDER BY created_at ASC
		 LIMIT 1`,
		name,
	)
	return scanWorker(row)
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/infra/store/ -run TestWorkerStore_GetByName -v
```

Expected: all four tests PASS

- [ ] **Step 5: Commit**

```bash
git add internal/infra/store/worker_store.go internal/infra/store/worker_store_test.go
git commit -m "feat: add WorkerStore.GetByName for case-insensitive worker lookup"
```

---

## Task 2: Fix `buildInstruction` to always emit `message_id`

**Files:**
- Modify: `internal/domain/task/dispatcher.go`
- Modify: `internal/domain/task/dispatcher_test.go`

- [ ] **Step 1: Write the failing test**

In `internal/domain/task/dispatcher_test.go`, add this test after the existing tests. The test dispatches a task with `MessageID` but no `TaskID` and verifies the instruction received by the worker contains a `message_id:` header but no `task_id:` header.

First, locate the existing `mockExecManager` struct in that file — it already has `executedInstructions []string` and captures what is passed to `ExecuteWorker`. Add this test:

```go
func TestDispatcher_BuildInstruction_MessageIDWithoutTaskID(t *testing.T) {
	dispatchCh := make(chan DispatchTask, 8)
	mgr := &mockExecManager{
		execResult: model.WorkerExecution{
			ID:        "exec-1",
			SessionID: "sess-1",
			Status:    model.ExecStatusCompleted,
		},
	}
	querier := &mockExecutionQuerier{result: model.WorkerExecution{
		ID:     "exec-1",
		Status: model.ExecStatusCompleted,
	}}
	taskStore := &mockTaskStore{}
	sessionStore := &mockSessionStore{}

	d := New(mgr, taskStore, sessionStore, querier, dispatchCh)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go d.Run(ctx)

	dispatchCh <- DispatchTask{
		TaskID:      "",
		MessageID:   "msg-abc",
		WorkerID:    "w1",
		SessionKey:  "sk1",
		Instruction: "do something",
		TaskType:    model.TaskTypeImmediate,
	}

	time.Sleep(300 * time.Millisecond)

	mgr.mu.Lock()
	instructions := mgr.executedInstructions
	mgr.mu.Unlock()

	if len(instructions) == 0 {
		t.Fatal("expected worker to be called")
	}
	instr := instructions[0]
	if !strings.Contains(instr, "message_id: msg-abc") {
		t.Errorf("expected message_id header in instruction, got:\n%s", instr)
	}
	if strings.Contains(instr, "task_id:") {
		t.Errorf("expected no task_id header when TaskID is empty, got:\n%s", instr)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

```bash
go test ./internal/domain/task/ -run TestDispatcher_BuildInstruction_MessageIDWithoutTaskID -v
```

Expected: FAIL — instruction does not contain `message_id:` header (current code skips header when TaskID is empty)

- [ ] **Step 3: Modify `buildInstruction` in `internal/domain/task/dispatcher.go`**

Replace the existing `buildInstruction` function:

```go
// buildInstruction prepends task metadata to the instruction so workers
// can call mark_task_success and send_message via MCP.
func buildInstruction(t DispatchTask) string {
	if t.TaskID == "" && t.MessageID == "" {
		return t.Instruction
	}
	header := fmt.Sprintf("---\nmessage_id: %s\n", t.MessageID)
	if t.TaskID != "" {
		header += fmt.Sprintf("task_id: %s\n", t.TaskID)
	}
	return header + "---\n\n" + t.Instruction
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/domain/task/ -v
```

Expected: all tests PASS including the new one

- [ ] **Step 5: Commit**

```bash
git add internal/domain/task/dispatcher.go internal/domain/task/dispatcher_test.go
git commit -m "feat: inject message_id header in worker instruction even without task_id"
```

---

## Task 3: Add direct dispatch to Feeder

**Files:**
- Modify: `internal/domain/bee/feeder.go`
- Modify: `internal/domain/bee/feeder_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `internal/domain/bee/feeder_test.go`:

```go
// mockWorkerLookup implements bee.WorkerNameLookup for tests.
type mockWorkerLookup struct {
	worker model.Worker
	err    error
}

func (m *mockWorkerLookup) GetByName(_ string) (model.Worker, error) {
	return m.worker, m.err
}

func TestFeeder_DirectDispatch_NoPrefix_FallsBackToBee(t *testing.T) {
	db, ms, ts, ss, es := setupFeederDB(t)
	insertMessage(t, db, "m1", "sk1", "hello world")

	runner := &mockBeeRunner{}
	dispatchCh := make(chan task.DispatchTask, 8)
	lookup := &mockWorkerLookup{err: fmt.Errorf("not found")}

	f := bee.NewFeeder(ms, ts, ss, es, runner, "/tmp", config.BeeConfig{
		Feeder: config.FeederConfig{Timeout: 5 * time.Second, MaxConcurrentBee: 5},
	}, bee.WithDirectDispatch(dispatchCh, lookup))

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	go f.Run(ctx)
	time.Sleep(700 * time.Millisecond)

	// Bee must have been called (normal flow)
	if len(runner.getCalls()) == 0 {
		t.Error("expected bee runner to be called for non-@mention message")
	}
	// Nothing in dispatch channel
	if len(dispatchCh) != 0 {
		t.Error("expected no task in dispatchCh for non-@mention message")
	}
}

func TestFeeder_DirectDispatch_WorkerNotFound_FallsBackToBee(t *testing.T) {
	db, ms, ts, ss, es := setupFeederDB(t)
	insertMessage(t, db, "m1", "sk1", "@unknown do something")

	runner := &mockBeeRunner{}
	dispatchCh := make(chan task.DispatchTask, 8)
	lookup := &mockWorkerLookup{err: fmt.Errorf("sql: no rows")}

	cfg := config.BeeConfig{}
	cfg.Feeder.Timeout = 5 * time.Second
	cfg.Feeder.MaxConcurrentBee = 5
	f := bee.NewFeeder(ms, ts, ss, es, runner, "/tmp", cfg,
		bee.WithDirectDispatch(dispatchCh, lookup))

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	go f.Run(ctx)
	time.Sleep(700 * time.Millisecond)

	if len(runner.getCalls()) == 0 {
		t.Error("expected bee runner to be called when worker not found")
	}
	if len(dispatchCh) != 0 {
		t.Error("expected no task in dispatchCh when worker not found")
	}
}

func TestFeeder_DirectDispatch_Success_SkipsBee(t *testing.T) {
	db, ms, ts, ss, es := setupFeederDB(t)
	insertMessage(t, db, "m1", "sk1", "@天天 write a report")

	runner := &mockBeeRunner{}
	dispatchCh := make(chan task.DispatchTask, 8)
	lookup := &mockWorkerLookup{worker: model.Worker{ID: "worker-tt", Name: "天天"}}

	cfg := config.BeeConfig{}
	cfg.Feeder.Timeout = 5 * time.Second
	cfg.Feeder.MaxConcurrentBee = 5
	f := bee.NewFeeder(ms, ts, ss, es, runner, "/tmp", cfg,
		bee.WithDirectDispatch(dispatchCh, lookup))

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	go f.Run(ctx)
	time.Sleep(700 * time.Millisecond)

	// Bee must NOT have been called
	if len(runner.getCalls()) != 0 {
		t.Error("expected bee runner NOT to be called for direct dispatch")
	}

	// Task must be in dispatchCh
	if len(dispatchCh) == 0 {
		t.Fatal("expected a DispatchTask in dispatchCh")
	}
	dt := <-dispatchCh
	if dt.WorkerID != "worker-tt" {
		t.Errorf("expected WorkerID worker-tt, got %s", dt.WorkerID)
	}
	if dt.Instruction != "write a report" {
		t.Errorf("expected instruction 'write a report', got %q", dt.Instruction)
	}
	if dt.SessionKey != "sk1" {
		t.Errorf("expected SessionKey sk1, got %s", dt.SessionKey)
	}
	if dt.MessageID != "m1" {
		t.Errorf("expected MessageID m1, got %s", dt.MessageID)
	}

	// Message must be marked bee_processed
	var status string
	db.QueryRow(`SELECT status FROM bee_platform_messages WHERE id='m1'`).Scan(&status)
	if status != "bee_processed" {
		t.Errorf("expected bee_processed, got %q", status)
	}
}
```

The test file imports `task` and `config` packages. Add them to the import block at the top of `feeder_test.go` if not already present:

```go
import (
	// existing imports ...
	"github.com/theopenbee/openbee/internal/domain/task"
	"github.com/theopenbee/openbee/internal/infra/config"
)
```

Note: `config.FeederConfig` — check the actual field name. In `config.BeeConfig`, the feeder settings are at `cfg.Feeder.Timeout` and `cfg.Feeder.MaxConcurrentBee`.

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/domain/bee/ -run TestFeeder_DirectDispatch -v
```

Expected: FAIL — `bee.WithDirectDispatch undefined`, `bee.WorkerNameLookup undefined`

- [ ] **Step 3: Add `WorkerNameLookup`, `WithDirectDispatch`, `parseDirectMention`, and `tryDirectDispatch` to `internal/domain/bee/feeder.go`**

**3a. Add imports** — ensure `strings` is imported (it likely already is).

**3b. Add interface and option** — add after the existing `FailureNotifier` interface:

```go
// WorkerNameLookup resolves a worker by display name.
type WorkerNameLookup interface {
	GetByName(name string) (model.Worker, error)
}
```

Add two new fields to the `Feeder` struct (after the existing `sem` field):

```go
workerLookup     WorkerNameLookup
directDispatchCh chan<- task.DispatchTask
```

Add the option constructor (after `WithFailureNotifier`):

```go
// WithDirectDispatch enables @mention direct routing. When a message starts
// with "@name ", the Feeder looks up the worker by name and dispatches
// directly to ch, bypassing Bee. Falls back to Bee if the worker is not found.
func WithDirectDispatch(ch chan<- task.DispatchTask, lookup WorkerNameLookup) Option {
	return func(f *Feeder) {
		f.directDispatchCh = ch
		f.workerLookup = lookup
	}
}
```

**3c. Add `parseDirectMention` helper** — add as a package-level function at the bottom of `feeder.go`:

```go
// parseDirectMention checks whether content starts with "@name " and returns
// the worker name and the remaining instruction (leading whitespace trimmed).
// Returns ok=false if content does not match the pattern.
func parseDirectMention(content string) (workerName, instruction string, ok bool) {
	if len(content) < 2 || content[0] != '@' {
		return "", "", false
	}
	rest := content[1:]
	spaceIdx := strings.IndexAny(rest, " \t\n\r")
	if spaceIdx < 0 {
		return "", "", false
	}
	workerName = rest[:spaceIdx]
	if workerName == "" {
		return "", "", false
	}
	instruction = strings.TrimSpace(rest[spaceIdx:])
	return workerName, instruction, true
}
```

**3d. Add `tryDirectDispatch` method** — add after `parseDirectMention`:

```go
// tryDirectDispatch checks if the primary message starts with "@name " and,
// if the named worker exists, dispatches to directDispatchCh instead of Bee.
// Returns true if the message was handled; false means fall back to Bee.
func (f *Feeder) tryDirectDispatch(ctx context.Context, sessionKey string, msgs []store.ClaimedMessage) bool {
	if f.directDispatchCh == nil || f.workerLookup == nil {
		return false
	}

	primary := msgs[len(msgs)-1]
	workerName, instruction, ok := parseDirectMention(primary.Content)
	if !ok {
		return false
	}

	worker, err := f.workerLookup.GetByName(workerName)
	if err != nil {
		log.Info("@mention: worker not found, falling back to bee",
			zap.String("name", workerName))
		return false
	}

	dt := task.DispatchTask{
		WorkerID:    worker.ID,
		SessionKey:  sessionKey,
		Instruction: instruction,
		MessageID:   primary.ID,
		TaskType:    model.TaskTypeImmediate,
	}

	select {
	case f.directDispatchCh <- dt:
	default:
		log.Warn("@mention: dispatch channel full, falling back to bee",
			zap.String("workerID", worker.ID))
		return false
	}

	msgIDs := make([]string, len(msgs))
	for i, m := range msgs {
		msgIDs[i] = m.ID
	}
	if err := f.msgStore.MarkBeeProcessed(ctx, msgIDs); err != nil {
		log.Error("@mention: mark bee_processed", zap.Error(err))
	}

	log.Info("@mention: direct dispatch to worker",
		zap.String("name", workerName), zap.String("workerID", worker.ID))
	return true
}
```

**3e. Add imports to `feeder.go`** — ensure the following are imported:

```go
import (
	// existing imports ...
	"github.com/theopenbee/openbee/internal/domain/task"
	"github.com/theopenbee/openbee/internal/infra/model"
)
```

**3f. Call `tryDirectDispatch` in `processBeeGroup`** — insert the call after the content-merge loop and before `buildPrompt`:

```go
	// (existing merge loop ends here)

	// @mention check: route directly to the named worker, skipping Bee.
	if f.tryDirectDispatch(ctx, sessionKey, msgs) {
		return
	}

	prompt := buildPrompt(msgs)
	// (rest of existing code unchanged)
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/domain/bee/ -run TestFeeder_DirectDispatch -v
```

Expected: all three tests PASS

- [ ] **Step 5: Run the full test suite**

```bash
go test ./...
```

Expected: all packages PASS

- [ ] **Step 6: Commit**

```bash
git add internal/domain/bee/feeder.go internal/domain/bee/feeder_test.go
git commit -m "feat: add @mention direct dispatch to Feeder"
```

---

## Task 4: Wire dependencies in `app.go`

**Files:**
- Modify: `internal/app/app.go`

- [ ] **Step 1: Locate `buildBee` in `internal/app/app.go`**

Find the function:

```go
func buildBee(cfg config.BeeConfig, s appStores, dispatchCh chan task.DispatchTask, failureNotifier bee.FailureNotifier) (*bee.Feeder, *task.Scheduler) {
```

- [ ] **Step 2: Pass `WithDirectDispatch` to the Feeder**

Inside `buildBee`, find where `bee.NewFeeder` is called and add `bee.WithDirectDispatch`:

```go
feeder := bee.NewFeeder(
	s.msgStore, s.taskStore, s.sessionStore, s.execStore,
	bee.NewBeeProcess(cfg),
	config.DefaultBeeWorkDir(),
	cfg,
	bee.WithFailureNotifier(failureNotifier),
	bee.WithDirectDispatch(dispatchCh, s.workerStore), // new
)
```

The `dispatchCh` parameter is already passed into `buildBee`. The `s.workerStore` is already available in `appStores`. No new variables needed.

- [ ] **Step 3: Build to verify compilation**

```bash
go build ./...
```

Expected: builds without errors

- [ ] **Step 4: Run the full test suite**

```bash
go test ./...
```

Expected: all packages PASS

- [ ] **Step 5: Commit**

```bash
git add internal/app/app.go
git commit -m "feat: wire @mention direct dispatch into app"
```

---

## Self-Review

**Spec coverage:**
- Worker not found → Bee fallback: covered in `tryDirectDispatch` (err branch returns false)
- Strip @name prefix: covered in `parseDirectMention` returning `instruction = TrimSpace(rest[spaceIdx:])`
- Case-insensitive matching: covered in `GetByName` with `LOWER(name)=LOWER(?)`
- Same name → earliest: covered in `GetByName` with `ORDER BY created_at ASC LIMIT 1`
- Session continuity: covered — `DispatchTask.SessionKey` + `TaskType=immediate` means `TaskDispatcher.resolveExecution` uses `GetSessionContext` as normal
- `message_id` in instruction without `task_id`: covered in Task 2
- No DB schema change: confirmed — no migration files needed
- 4 files changed: confirmed

**Placeholder scan:** No TBD/TODO/incomplete steps.

**Type consistency:**
- `WorkerNameLookup.GetByName` matches `WorkerStore.GetByName` signature
- `bee.WithDirectDispatch(ch chan<- task.DispatchTask, lookup WorkerNameLookup)` — `chan<- task.DispatchTask` matches `dispatchCh` type in `app.go`
- `task.DispatchTask` fields used: `WorkerID`, `SessionKey`, `Instruction`, `MessageID`, `TaskType` — all defined in `internal/domain/task/task.go`
- `model.TaskTypeImmediate` — defined in `internal/infra/model/task.go`
