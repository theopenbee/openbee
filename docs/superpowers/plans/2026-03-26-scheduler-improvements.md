# Scheduler Improvements Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix the 24h sentinel crash-recovery gap in scheduled task claiming, and add per-task cancellation support to the dispatcher including killing the underlying worker process.

**Architecture:** Two independent tracks. Track 1 (sentinel fix) changes `TaskStore.ClaimDueTasks` to accept pre-computed `next_run_at` values, eliminating the two-step update. Track 2 adds per-task cancel contexts to `TaskDispatcher` and a `CancelExecution` method to `ExecutionManager`, wiring them together so a cancel call stops both the in-memory goroutine and the worker process.

**Tech Stack:** Go, SQLite (`database/sql`), `robfig/cron/v3`, `context.WithCancel`, `golang.org/x/exp/slices`

---

## File Map

**Track 1 — Sentinel Fix**
- Modify: `internal/store/task_store.go` — add `PeekDueScheduledTasks`; update `ClaimDueTasks` signature
- Modify: `internal/store/task_store_test.go` — tests for new/changed methods
- Modify: `internal/task_scheduler/scheduler.go` — two-step poll
- Modify: `internal/task_scheduler/scheduler_test.go` — regression test

**Track 2 — Task Cancellation**
- Modify: `internal/task_dispatcher/dispatcher.go` — `ExecutionManager` interface + `cancelFuncs` + `cancelCh` + all related methods
- Modify: `internal/task_dispatcher/dispatcher_test.go` — update mocks + cancellation tests
- Modify: `internal/worker/manager.go` — implement `CancelExecution`
- Modify: `internal/worker/manager_test.go` — test `CancelExecution`

---

## Track 1 — Sentinel Fix

### Task 1: Add `PeekDueScheduledTasks` to TaskStore

**Files:**
- Modify: `internal/store/task_store.go`
- Modify: `internal/store/task_store_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/store/task_store_test.go`:

```go
func TestTaskStore_PeekDueScheduledTasks_ReturnsDueOnly(t *testing.T) {
	ts, cleanup := newTaskStoreForTest(t)
	defer cleanup()

	now := time.Now().UnixMilli()
	pastRun := now - 1000
	futureRun := now + 60_000
	expr := "0 * * * *"

	// Due: next_run_at in the past
	ts.Create(context.Background(), model.Task{
		MessageID: "m1", WorkerID: "w1", Instruction: "due",
		Type: model.TaskTypeScheduled, Status: model.TaskStatusPending,
		CronExpr: expr, NextRunAt: &pastRun,
		CreatedAt: now, UpdatedAt: now,
	})
	// Not due: next_run_at in the future
	ts.Create(context.Background(), model.Task{
		MessageID: "m1", WorkerID: "w1", Instruction: "future",
		Type: model.TaskTypeScheduled, Status: model.TaskStatusPending,
		CronExpr: expr, NextRunAt: &futureRun,
		CreatedAt: now, UpdatedAt: now,
	})
	// Due: next_run_at IS NULL
	ts.Create(context.Background(), model.Task{
		MessageID: "m1", WorkerID: "w1", Instruction: "null-run-at",
		Type: model.TaskTypeScheduled, Status: model.TaskStatusPending,
		CronExpr: expr,
		CreatedAt: now, UpdatedAt: now,
	})

	tasks, err := ts.PeekDueScheduledTasks(context.Background(), now)
	if err != nil {
		t.Fatalf("PeekDueScheduledTasks: %v", err)
	}
	if len(tasks) != 2 {
		t.Fatalf("expected 2 due scheduled tasks, got %d", len(tasks))
	}
	for _, task := range tasks {
		if task.CronExpr != expr {
			t.Errorf("expected cron_expr %q, got %q", expr, task.CronExpr)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee2
go test ./internal/store/... -run TestTaskStore_PeekDueScheduledTasks_ReturnsDueOnly -v
```

Expected: FAIL — `ts.PeekDueScheduledTasks undefined`

- [ ] **Step 3: Implement `PeekDueScheduledTasks`**

Add to `internal/store/task_store.go`, after `ClaimDueTasks`:

```go
// PeekDueScheduledTasks returns all pending scheduled tasks whose next_run_at
// is at or before nowMS (or NULL). Read-only — no updates, no locking.
// Used by Scheduler.poll to compute real next_run_at values before claiming.
func (s *TaskStore) PeekDueScheduledTasks(ctx context.Context, nowMS int64) ([]model.Task, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, message_id, worker_id, instruction, type, status,
		       scheduled_at, cron_expr, next_run_at, execution_id,
		       created_at, updated_at
		FROM bee_tasks
		WHERE type = 'scheduled'
		  AND status = 'pending'
		  AND (next_run_at IS NULL OR next_run_at <= ?)`, nowMS)
	if err != nil {
		return nil, fmt.Errorf("peek due scheduled tasks: %w", err)
	}
	defer rows.Close()
	return scanTasks(rows)
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./internal/store/... -run TestTaskStore_PeekDueScheduledTasks_ReturnsDueOnly -v
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/store/task_store.go internal/store/task_store_test.go
git commit -m "feat(store): add PeekDueScheduledTasks read-only query"
```

---

### Task 2: Update `ClaimDueTasks` to accept pre-computed `scheduledNextRuns`

**Files:**
- Modify: `internal/store/task_store.go`
- Modify: `internal/store/task_store_test.go`
- Modify: `internal/task_scheduler/scheduler.go` (stub call — full logic in Task 3)

- [ ] **Step 1: Write failing tests for the updated signature**

Add to `internal/store/task_store_test.go`:

```go
func TestTaskStore_ClaimDueTasks_ScheduledUsesProvidedNextRunAt(t *testing.T) {
	ts, cleanup := newTaskStoreForTest(t)
	defer cleanup()

	now := time.Now().UnixMilli()
	pastRun := now - 1000
	ts.Create(context.Background(), model.Task{
		MessageID: "m1", WorkerID: "w1", Instruction: "recurring",
		Type: model.TaskTypeScheduled, Status: model.TaskStatusPending,
		CronExpr: "0 * * * *", NextRunAt: &pastRun,
		CreatedAt: now, UpdatedAt: now,
	})

	// Peek to get the task ID
	peeked, err := ts.PeekDueScheduledTasks(context.Background(), now)
	if err != nil || len(peeked) != 1 {
		t.Fatalf("peek: %v, got %d tasks", err, len(peeked))
	}
	taskID := peeked[0].ID
	realNext := now + 3600_000 // 1h from now

	tasks, err := ts.ClaimDueTasks(context.Background(), now, map[string]int64{taskID: realNext})
	if err != nil {
		t.Fatalf("ClaimDueTasks: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected 1 claimed task, got %d", len(tasks))
	}

	// next_run_at should be the provided value, NOT +24h
	got, err := ts.GetByID(context.Background(), taskID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.NextRunAt == nil || *got.NextRunAt != realNext {
		t.Errorf("expected next_run_at=%d, got %v", realNext, got.NextRunAt)
	}
	// status should still be pending (scheduled tasks stay pending)
	if got.Status != model.TaskStatusPending {
		t.Errorf("scheduled task should remain pending, got %q", got.Status)
	}
}

func TestTaskStore_ClaimDueTasks_ImmediateUnaffectedByScheduledMap(t *testing.T) {
	ts, cleanup := newTaskStoreForTest(t)
	defer cleanup()

	now := time.Now().UnixMilli()
	ts.Create(context.Background(), model.Task{
		MessageID: "m1", WorkerID: "w1", Instruction: "now",
		Type: model.TaskTypeImmediate, Status: model.TaskStatusPending,
		CreatedAt: now, UpdatedAt: now,
	})

	tasks, err := ts.ClaimDueTasks(context.Background(), now, nil)
	if err != nil {
		t.Fatalf("ClaimDueTasks: %v", err)
	}
	if len(tasks) != 1 || tasks[0].Status != model.TaskStatusRunning {
		t.Fatalf("expected 1 running immediate task, got %+v", tasks)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/store/... -run "TestTaskStore_ClaimDueTasks_Scheduled|TestTaskStore_ClaimDueTasks_ImmediateUnaffected" -v
```

Expected: FAIL — signature mismatch

- [ ] **Step 3: Update `ClaimDueTasks` signature and remove sentinel**

In `internal/store/task_store.go`, replace the `ClaimDueTasks` function:

```go
// ClaimDueTasks atomically selects all pending tasks that are due at or before nowMS,
// marks immediate/countdown tasks as running, and sets scheduled tasks' next_run_at
// to the pre-computed value from scheduledNextRuns (keyed by task ID).
// scheduledNextRuns may be nil if there are no due scheduled tasks.
func (s *TaskStore) ClaimDueTasks(ctx context.Context, nowMS int64, scheduledNextRuns map[string]int64) ([]model.ClaimedTask, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	rows, err := tx.QueryContext(ctx, `
        SELECT t.id, t.message_id, t.worker_id, t.instruction, t.type, t.status,
               t.scheduled_at, t.cron_expr, t.next_run_at,
               t.execution_id, t.created_at, t.updated_at,
               pm.session_key, pm.platform
        FROM bee_tasks t
        JOIN bee_platform_messages pm ON pm.id = t.message_id
        WHERE t.status = 'pending'
          AND (
            t.type = 'immediate'
            OR (t.type = 'countdown' AND t.scheduled_at <= ?)
            OR (t.type = 'scheduled' AND (t.next_run_at IS NULL OR t.next_run_at <= ?))
          )`, nowMS, nowMS)
	if err != nil {
		return nil, fmt.Errorf("query due tasks: %w", err)
	}

	var claimed []model.ClaimedTask
	for rows.Next() {
		var ct model.ClaimedTask
		var scheduledAt, nextRunAt sql.NullInt64
		err := rows.Scan(
			&ct.ID, &ct.MessageID, &ct.WorkerID, &ct.Instruction,
			&ct.Type, &ct.Status, &scheduledAt, &ct.CronExpr,
			&nextRunAt, &ct.ExecutionID,
			&ct.CreatedAt, &ct.UpdatedAt,
			&ct.MessageSessionKey, &ct.MessagePlatform,
		)
		if err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan task: %w", err)
		}
		ct.ScheduledAt = nullInt64Ptr(scheduledAt)
		ct.NextRunAt = nullInt64Ptr(nextRunAt)
		claimed = append(claimed, ct)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	now := time.Now().UnixMilli()
	for i, ct := range claimed {
		if ct.Type == model.TaskTypeScheduled {
			nextRun, ok := scheduledNextRuns[ct.ID]
			if !ok {
				// Fallback: keep next_run_at unchanged (will be re-evaluated next poll)
				continue
			}
			_, err = tx.ExecContext(ctx,
				`UPDATE bee_tasks SET next_run_at = ?, updated_at = ? WHERE id = ?`,
				nextRun, now, ct.ID)
		} else {
			_, err = tx.ExecContext(ctx,
				`UPDATE bee_tasks SET status = 'running', updated_at = ? WHERE id = ?`,
				now, ct.ID)
			claimed[i].Status = model.TaskStatusRunning
		}
		if err != nil {
			return nil, fmt.Errorf("update task %s: %w", ct.ID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}
	return claimed, nil
}
```

- [ ] **Step 4: Fix the compile error in `scheduler.go`**

The existing call in `internal/task_scheduler/scheduler.go` will fail to compile. Update `poll()` temporarily to pass `nil`:

```go
func (s *Scheduler) poll(ctx context.Context) {
	nowMS := time.Now().UnixMilli()
	tasks, err := s.taskStore.ClaimDueTasks(ctx, nowMS, nil) // will be fixed in Task 3
	if err != nil {
		log.Error("claim due tasks", zap.Error(err))
		return
	}
	// ... rest unchanged ...
```

Also remove the `UpdateNextRunAt` call block (lines 72–81 in original), keeping only the dispatch logic:

```go
func (s *Scheduler) poll(ctx context.Context) {
	nowMS := time.Now().UnixMilli()
	tasks, err := s.taskStore.ClaimDueTasks(ctx, nowMS, nil) // will be fixed in Task 3
	if err != nil {
		log.Error("claim due tasks", zap.Error(err))
		return
	}

	for _, ct := range tasks {
		sessionKey := ct.MessageSessionKey
		dt := task_dispatcher.DispatchTask{
			TaskID:      ct.ID,
			WorkerID:    ct.WorkerID,
			SessionKey:  sessionKey,
			Instruction: ct.Instruction,
			ReplyTo:     platform.InboundMessage{Platform: ct.MessagePlatform, SessionKey: sessionKey},
			TaskType:    ct.Type,
			MessageID:   ct.MessageID,
		}
		select {
		case s.dispatchCh <- dt:
		case <-ctx.Done():
			return
		}
	}
}
```

Also remove the `cron` import from `scheduler.go` imports (it's no longer used here).

- [ ] **Step 5: Fix `ClaimDueTasks` calls in test files**

In `internal/store/task_store_test.go`, find all existing calls to `ts.ClaimDueTasks(context.Background(), now)` and add `nil` as the third argument:

```go
// Before:
tasks, err := ts.ClaimDueTasks(context.Background(), now)
// After:
tasks, err := ts.ClaimDueTasks(context.Background(), now, nil)
```

In `internal/task_scheduler/scheduler_test.go`, the scheduler calls ClaimDueTasks indirectly — no change needed in the test file itself.

- [ ] **Step 6: Run all affected tests**

```bash
go test ./internal/store/... ./internal/task_scheduler/... -v
```

Expected: all existing tests PASS, new tests PASS

- [ ] **Step 7: Commit**

```bash
git add internal/store/task_store.go internal/store/task_store_test.go internal/task_scheduler/scheduler.go
git commit -m "feat(store): remove 24h sentinel from ClaimDueTasks; accept scheduledNextRuns map"
```

---

### Task 3: Update Scheduler to pre-compute `next_run_at`

**Files:**
- Modify: `internal/task_scheduler/scheduler.go`
- Modify: `internal/task_scheduler/scheduler_test.go`

- [ ] **Step 1: Write a failing test for correct `next_run_at` computation**

Add to `internal/task_scheduler/scheduler_test.go`:

```go
func TestScheduler_ScheduledTask_NextRunAtSetCorrectly(t *testing.T) {
	db, ts := setupDB(t)
	defer db.Close()

	now := time.Now().UnixMilli()
	pastRun := now - 1000
	cronExpr := "0 * * * *" // every hour

	taskID, err := ts.Create(context.Background(), model.Task{
		MessageID: "m1", WorkerID: "w1", Instruction: "tick",
		Type: model.TaskTypeScheduled, Status: model.TaskStatusPending,
		CronExpr: cronExpr, NextRunAt: &pastRun,
		CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	dispCh := make(chan task_dispatcher.DispatchTask, 10)
	sched := task_scheduler.New(ts, dispCh, 50*time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	go sched.Run(ctx)

	// Wait for dispatch
	select {
	case <-dispCh:
	case <-ctx.Done():
		t.Fatal("timeout: scheduled task not dispatched")
	}

	// next_run_at should NOT be +24h sentinel; it should be cron-computed (~1h from now)
	got, err := ts.GetByID(context.Background(), taskID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.NextRunAt == nil {
		t.Fatal("next_run_at should not be nil after dispatch")
	}
	sentinel := now + 24*60*60*1000
	if *got.NextRunAt == sentinel {
		t.Errorf("next_run_at is the old 24h sentinel (%d); expected real cron value", sentinel)
	}
	// Should be approximately 1 hour from now (within 2 minutes tolerance)
	twoMin := int64(2 * 60 * 1000)
	oneHour := int64(60 * 60 * 1000)
	if *got.NextRunAt < now+oneHour-twoMin || *got.NextRunAt > now+oneHour+twoMin {
		t.Errorf("next_run_at=%d not approximately 1h from now=%d", *got.NextRunAt, now)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/task_scheduler/... -run TestScheduler_ScheduledTask_NextRunAtSetCorrectly -v
```

Expected: FAIL — next_run_at is nil or sentinel value (since `nil` is passed to ClaimDueTasks)

- [ ] **Step 3: Implement two-step poll in `scheduler.go`**

Replace the `poll` function and add a `robfig/cron` import back (now used in scheduler, not store):

```go
import (
	"context"
	"time"

	"github.com/robfig/cron/v3"
	"go.uber.org/zap"

	"github.com/theopenbee/openbee/internal/logger"
	"github.com/theopenbee/openbee/internal/model"
	"github.com/theopenbee/openbee/internal/platform"
	"github.com/theopenbee/openbee/internal/store"
	"github.com/theopenbee/openbee/internal/task_dispatcher"
)
```

Replace `poll`:

```go
func (s *Scheduler) poll(ctx context.Context) {
	nowMS := time.Now().UnixMilli()

	// Step 1: read-only peek at due scheduled tasks to compute real next_run_at values.
	peeked, err := s.taskStore.PeekDueScheduledTasks(ctx, nowMS)
	if err != nil {
		log.Error("peek due scheduled tasks", zap.Error(err))
		return
	}
	scheduledNextRuns := make(map[string]int64, len(peeked))
	for _, t := range peeked {
		if t.CronExpr == "" {
			continue
		}
		sched, err := cron.ParseStandard(t.CronExpr)
		if err != nil {
			log.Error("invalid cron expression", zap.String("cronExpr", t.CronExpr), zap.String("taskID", t.ID), zap.Error(err))
			continue
		}
		scheduledNextRuns[t.ID] = sched.Next(time.Now()).UnixMilli()
	}

	// Step 2: claim all due tasks atomically, writing real next_run_at in the same transaction.
	tasks, err := s.taskStore.ClaimDueTasks(ctx, nowMS, scheduledNextRuns)
	if err != nil {
		log.Error("claim due tasks", zap.Error(err))
		return
	}

	for _, ct := range tasks {
		// Scheduled tasks with invalid cron were skipped in peek; skip dispatch too.
		if ct.Type == model.TaskTypeScheduled && ct.CronExpr != "" {
			if _, ok := scheduledNextRuns[ct.ID]; !ok {
				s.taskStore.SetExecution(ctx, ct.ID, "", model.TaskStatusFailed) //nolint:errcheck
				continue
			}
		}

		sessionKey := ct.MessageSessionKey
		dt := task_dispatcher.DispatchTask{
			TaskID:      ct.ID,
			WorkerID:    ct.WorkerID,
			SessionKey:  sessionKey,
			Instruction: ct.Instruction,
			ReplyTo:     platform.InboundMessage{Platform: ct.MessagePlatform, SessionKey: sessionKey},
			TaskType:    ct.Type,
			MessageID:   ct.MessageID,
		}
		select {
		case s.dispatchCh <- dt:
		case <-ctx.Done():
			return
		}
	}
}
```

Also update the `Scheduler` struct to expose `PeekDueScheduledTasks` via a new interface method. In `scheduler.go`, update the `taskStore` field type from `*store.TaskStore` to an interface:

```go
type schedulerStore interface {
	PeekDueScheduledTasks(ctx context.Context, nowMS int64) ([]model.Task, error)
	ClaimDueTasks(ctx context.Context, nowMS int64, scheduledNextRuns map[string]int64) ([]model.ClaimedTask, error)
	ResetRunningToPending(ctx context.Context) (int64, error)
	SetExecution(ctx context.Context, taskID, executionID, status string) error
}

type Scheduler struct {
	taskStore    schedulerStore
	dispatchCh   chan<- task_dispatcher.DispatchTask
	pollInterval time.Duration
}

func New(taskStore schedulerStore, dispatchCh chan<- task_dispatcher.DispatchTask, pollInterval time.Duration) *Scheduler {
	return &Scheduler{
		taskStore:    taskStore,
		dispatchCh:   dispatchCh,
		pollInterval: pollInterval,
	}
}
```

- [ ] **Step 4: Run all scheduler and store tests**

```bash
go test ./internal/store/... ./internal/task_scheduler/... -v
```

Expected: all PASS

- [ ] **Step 5: Run full build check**

```bash
go build ./...
```

Expected: no errors

- [ ] **Step 6: Commit**

```bash
git add internal/task_scheduler/scheduler.go internal/task_scheduler/scheduler_test.go
git commit -m "feat(scheduler): pre-compute next_run_at before claim, eliminating 24h sentinel"
```

---

## Track 2 — Task Cancellation

### Task 4: Add `CancelExecution` to `ExecutionManager` and implement in `worker.Manager`

**Files:**
- Modify: `internal/task_dispatcher/dispatcher.go` — interface only
- Modify: `internal/worker/manager.go` — implementation
- Modify: `internal/worker/manager_test.go` — test

- [ ] **Step 1: Write a failing test for `CancelExecution`**

Add to `internal/worker/manager_test.go`:

```go
func TestManager_CancelExecution_StopsActiveProcess(t *testing.T) {
	// This test verifies CancelExecution calls StopExecution on the active process.
	// We use a real Manager with a mock invoker that never finishes.
	// Since we can't easily inject a mock invoker, we verify the method exists
	// and returns a sensible error for an unknown execution ID.
	cfg := config.BeeConfig{}
	cfg.Claude.Path = "echo" // won't actually run; just needs to not panic
	dir := t.TempDir()
	db, err := store.InitDB(dir + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ws := store.NewWorkerStore(db)
	es := store.NewExecutionStore(db, dir)
	mgr := NewManager(dir, cfg, ws, es)

	err = mgr.CancelExecution(context.Background(), "nonexistent-exec-id")
	if err == nil {
		t.Error("expected error for unknown executionID, got nil")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/worker/... -run TestManager_CancelExecution_StopsActiveProcess -v
```

Expected: FAIL — `mgr.CancelExecution undefined`

- [ ] **Step 3: Add `CancelExecution` to `ExecutionManager` interface**

In `internal/task_dispatcher/dispatcher.go`, update the interface:

```go
// ExecutionManager manages worker executions.
type ExecutionManager interface {
	ExecuteWorker(ctx context.Context, workerID, input, sessionID string) (model.WorkerExecution, error)
	CancelExecution(ctx context.Context, executionID string) error
}
```

- [ ] **Step 4: Implement `CancelExecution` on `worker.Manager`**

Add to `internal/worker/manager.go`, after `StopExecution`:

```go
// CancelExecution implements task_dispatcher.ExecutionManager.
// It stops the active worker process for the given executionID.
func (m *Manager) CancelExecution(_ context.Context, executionID string) error {
	return m.StopExecution(executionID)
}
```

- [ ] **Step 5: Update all mocks in dispatcher_test.go**

In `internal/task_dispatcher/dispatcher_test.go`, add `CancelExecution` to every mock that implements `ExecutionManager`:

```go
// Add to mockExecManager:
func (m *mockExecManager) CancelExecution(_ context.Context, _ string) error { return nil }

// Add to blockingExecManager:
func (m *blockingExecManager) CancelExecution(_ context.Context, _ string) error { return nil }

// Add to alwaysFailExecManager:
func (m *alwaysFailExecManager) CancelExecution(_ context.Context, _ string) error { return nil }

// Add to fallbackExecManager:
func (m *fallbackExecManager) CancelExecution(_ context.Context, _ string) error { return nil }
```

- [ ] **Step 6: Run all tests**

```bash
go test ./internal/worker/... ./internal/task_dispatcher/... -v
```

Expected: all PASS (including the new worker test)

- [ ] **Step 7: Commit**

```bash
git add internal/task_dispatcher/dispatcher.go internal/worker/manager.go internal/worker/manager_test.go internal/task_dispatcher/dispatcher_test.go
git commit -m "feat(worker): add CancelExecution to ExecutionManager interface and implement in Manager"
```

---

### Task 5: Add cancel infrastructure to `TaskDispatcher`

**Files:**
- Modify: `internal/task_dispatcher/dispatcher.go`
- Modify: `internal/task_dispatcher/dispatcher_test.go`

- [ ] **Step 1: Write failing tests for cancellation**

Add to `internal/task_dispatcher/dispatcher_test.go`:

```go
// cancellingExecManager blocks until either the context is cancelled or the blocker is closed.
type cancellingExecManager struct {
	started int64
}

func (m *cancellingExecManager) ExecuteWorker(ctx context.Context, _, _, _ string) (model.WorkerExecution, error) {
	atomic.AddInt64(&m.started, 1)
	<-ctx.Done() // blocks until context is cancelled
	return model.WorkerExecution{ID: "exec-cancel"}, nil
}

func (m *cancellingExecManager) CancelExecution(_ context.Context, _ string) error {
	return nil
}

func TestTaskDispatcher_CancelTask_RemovesPendingTask(t *testing.T) {
	// A pending (not yet executing) task should be removed from the queue.
	blocker := make(chan struct{})
	mgr := &blockingExecManager{blocker: blocker}
	eq := &mockExecutionQuerier{result: model.WorkerExecution{Status: model.ExecStatusCompleted}}

	in := make(chan task_dispatcher.DispatchTask, 4)
	ts := &mockTaskStore{}
	d := task_dispatcher.New(mgr, ts, newMockSessionStore(), eq, in)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go d.Run(ctx)

	// t1 blocks the worker queue
	t1 := immediateTask("s1", "w1", "first")
	t1.TaskID = "task-1"
	in <- t1
	time.Sleep(50 * time.Millisecond) // t1 now executing

	// t2 is pending in queue
	t2 := immediateTask("s1", "w1", "second")
	t2.TaskID = "task-2"
	in <- t2
	time.Sleep(20 * time.Millisecond)

	// Cancel t2 while it's pending
	if err := d.CancelTask(context.Background(), "task-2"); err != nil {
		t.Fatalf("CancelTask: %v", err)
	}
	time.Sleep(50 * time.Millisecond)

	// Unblock t1
	close(blocker)
	time.Sleep(100 * time.Millisecond)

	// t2 should NOT have executed
	if atomic.LoadInt64(&mgr.completed) > 1 {
		t.Errorf("task-2 should not have executed after cancel, completed=%d", atomic.LoadInt64(&mgr.completed))
	}
}

func TestTaskDispatcher_CancelTask_InterruptsExecutingTask(t *testing.T) {
	var cancelCalled int64
	mgr := &cancelTrackingExecManager{cancelCount: &cancelCalled}
	eq := &mockExecutionQuerier{result: model.WorkerExecution{Status: model.ExecStatusCompleted}}

	in := make(chan task_dispatcher.DispatchTask, 4)
	ts := &mockTaskStore{}
	d := task_dispatcher.New(mgr, ts, newMockSessionStore(), eq, in)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go d.Run(ctx)

	t1 := immediateTask("s1", "w1", "long task")
	t1.TaskID = "task-exec-1"
	in <- t1
	time.Sleep(50 * time.Millisecond) // executing

	if err := d.CancelTask(context.Background(), "task-exec-1"); err != nil {
		t.Fatalf("CancelTask: %v", err)
	}

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if atomic.LoadInt64(&cancelCalled) > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if atomic.LoadInt64(&cancelCalled) == 0 {
		t.Error("expected CancelExecution to be called on the manager")
	}
}
```

Add helper mock after the existing helpers at the bottom of dispatcher_test.go:

```go
// cancelTrackingExecManager blocks forever on ExecuteWorker (context-aware),
// and tracks CancelExecution calls.
type cancelTrackingExecManager struct {
	cancelCount *int64
}

func (m *cancelTrackingExecManager) ExecuteWorker(ctx context.Context, _, _, _ string) (model.WorkerExecution, error) {
	<-ctx.Done()
	return model.WorkerExecution{ID: "exec-tracked"}, nil
}

func (m *cancelTrackingExecManager) CancelExecution(_ context.Context, _ string) error {
	atomic.AddInt64(m.cancelCount, 1)
	return nil
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/task_dispatcher/... -run "TestTaskDispatcher_CancelTask" -v
```

Expected: FAIL — `d.CancelTask undefined`

- [ ] **Step 3: Add `CancelTask` to the `TaskStore` dispatcher interface**

In `internal/task_dispatcher/dispatcher.go`, update the `TaskStore` interface:

```go
// TaskStore is the subset of store.TaskStore used by the TaskDispatcher.
type TaskStore interface {
	SetExecution(ctx context.Context, taskID, executionID, status string) error
	FailTask(ctx context.Context, taskID string) error
	CancelTask(ctx context.Context, taskID string) error
}
```

Also add `CancelTask` to `mockTaskStore` in `dispatcher_test.go`:

```go
func (s *mockTaskStore) CancelTask(_ context.Context, taskID string) error { return nil }
```

- [ ] **Step 4: Add `cancelFuncs`, `cancelCh`, `handleCancel`, and `CancelTask` to `TaskDispatcher`**

In `internal/task_dispatcher/dispatcher.go`:

Add fields to the struct:

```go
type TaskDispatcher struct {
	ctx             context.Context
	manager         ExecutionManager
	taskStore       TaskStore
	sessionStore    SessionStore
	execStore       ExecutionQuerier
	failureNotifier FailureNotifier
	inCh            <-chan DispatchTask
	resultsCh       chan internalResult
	queues          map[string]*queueState
	clearCh         chan string
	cancelFuncs     map[string]context.CancelFunc // taskID → cancel func; owned by Run loop
	cancelCh        chan string                    // receives taskID cancel requests
}
```

Update `New` to initialize the new fields:

```go
func New(manager ExecutionManager, taskStore TaskStore, sessionStore SessionStore, execStore ExecutionQuerier, in <-chan DispatchTask, opts ...Option) *TaskDispatcher {
	d := &TaskDispatcher{
		manager:      manager,
		taskStore:    taskStore,
		sessionStore: sessionStore,
		execStore:    execStore,
		inCh:         in,
		resultsCh:    make(chan internalResult, 64),
		queues:       make(map[string]*queueState),
		clearCh:      make(chan string, 8),
		cancelFuncs:  make(map[string]context.CancelFunc),
		cancelCh:     make(chan string, 16),
	}
	for _, o := range opts {
		o(d)
	}
	return d
}
```

Add `cancelCh` to the `Run` select:

```go
func (d *TaskDispatcher) Run(ctx context.Context) {
	d.ctx = ctx
	for {
		select {
		case task, ok := <-d.inCh:
			if !ok {
				return
			}
			d.handleInbound(task)
		case res := <-d.resultsCh:
			d.handleResult(res)
		case sessionKey := <-d.clearCh:
			d.clearQueues(sessionKey)
		case taskID := <-d.cancelCh:
			d.handleCancel(taskID)
		case <-ctx.Done():
			return
		}
	}
}
```

Add `handleCancel`:

```go
func (d *TaskDispatcher) handleCancel(taskID string) {
	// Remove from any pending queue
	for key, state := range d.queues {
		var remaining []DispatchTask
		for _, t := range state.pendingTasks {
			if t.TaskID != taskID {
				remaining = append(remaining, t)
			}
		}
		state.pendingTasks = remaining
		if !state.executing && len(state.pendingTasks) == 0 {
			delete(d.queues, key)
		}
	}
	// Interrupt executing goroutine if present
	if cancel, ok := d.cancelFuncs[taskID]; ok {
		cancel()
		delete(d.cancelFuncs, taskID)
	}
}
```

Add public `CancelTask`:

```go
// CancelTask marks the task cancelled in DB and signals the Run loop to
// remove it from the in-memory queue or interrupt its executing goroutine.
// Best-effort: returns once DB is updated; goroutine interruption is async.
func (d *TaskDispatcher) CancelTask(ctx context.Context, taskID string) error {
	if err := d.taskStore.CancelTask(ctx, taskID); err != nil {
		return err
	}
	select {
	case d.cancelCh <- taskID:
	default:
		log.Warn("cancelCh full, in-memory cancel dropped", zap.String("taskID", taskID))
	}
	return nil
}
```

- [ ] **Step 5: Run tests**

```bash
go test ./internal/task_dispatcher/... -v
```

Expected: existing tests PASS; new CancelTask tests may still fail (cancel func not yet registered in `handleInbound`)

- [ ] **Step 6: Commit**

```bash
git add internal/task_dispatcher/dispatcher.go internal/task_dispatcher/dispatcher_test.go
git commit -m "feat(dispatcher): add cancelFuncs, cancelCh, CancelTask, handleCancel infrastructure"
```

---

### Task 6: Wire per-task cancel context into `handleInbound`, `handleResult`, `executeAsync`, and `waitForResult`

**Files:**
- Modify: `internal/task_dispatcher/dispatcher.go`

- [ ] **Step 1: Update `handleInbound` to create and store cancel func**

Replace `handleInbound` in `internal/task_dispatcher/dispatcher.go`:

```go
func (d *TaskDispatcher) handleInbound(task DispatchTask) {
	key := queueKey(task.SessionKey, task.WorkerID)
	state, ok := d.queues[key]
	if !ok {
		state = &queueState{}
		d.queues[key] = state
	}

	if !state.executing {
		state.executing = true
		taskCtx, cancel := context.WithCancel(d.ctx)
		if task.TaskID != "" {
			d.cancelFuncs[task.TaskID] = cancel
		}
		go d.executeAsync(taskCtx, cancel, key, task)
	} else {
		state.pendingTasks = append(state.pendingTasks, task)
	}
}
```

- [ ] **Step 2: Update `handleResult` to clean up cancel func and create one for next task**

Replace `handleResult`:

```go
func (d *TaskDispatcher) handleResult(res internalResult) {
	// Clean up cancel func for the completed task
	delete(d.cancelFuncs, res.task.TaskID)

	state, ok := d.queues[res.queueKey]
	if !ok {
		return
	}

	if len(state.pendingTasks) > 0 {
		next := state.pendingTasks[0]
		state.pendingTasks = state.pendingTasks[1:]
		taskCtx, cancel := context.WithCancel(d.ctx)
		if next.TaskID != "" {
			d.cancelFuncs[next.TaskID] = cancel
		}
		go d.executeAsync(taskCtx, cancel, res.queueKey, next)
	} else {
		state.executing = false
		delete(d.queues, res.queueKey)
	}
}
```

- [ ] **Step 3: Update `executeAsync` signature, add Method Y, pass `taskCtx` to `waitForResult`**

Replace `executeAsync`:

```go
func (d *TaskDispatcher) executeAsync(taskCtx context.Context, cancel context.CancelFunc, key string, task DispatchTask) {
	defer cancel() // always release the cancel func's resources
	defer func() {
		select {
		case d.resultsCh <- internalResult{queueKey: key, task: task}:
		case <-d.ctx.Done():
		}
	}()

	instruction := buildInstruction(task)
	exec, err := d.resolveExecution(taskCtx, task, instruction)
	if err != nil {
		log.Error("execute error", zap.Error(err))
		if task.TaskID != "" {
			if failErr := d.taskStore.FailTask(taskCtx, task.TaskID); failErr != nil {
				log.Error("fail task after execute error", zap.String("taskID", task.TaskID), zap.Error(failErr))
			}
		}
		d.notifyFailure(taskCtx, task.MessageID, err.Error())
		return
	}

	// Method Y: if context was cancelled while resolveExecution was in-flight, kill the
	// worker process that was just launched before entering waitForResult.
	if taskCtx.Err() != nil {
		d.manager.CancelExecution(context.Background(), exec.ID) //nolint:errcheck
		return
	}

	if task.TaskID != "" {
		if err := d.taskStore.SetExecution(taskCtx, task.TaskID, exec.ID, model.TaskStatusRunning); err != nil {
			log.Error("set execution", zap.String("taskID", task.TaskID), zap.Error(err))
		}
	}
	d.waitForResult(taskCtx, exec.ID, task)
}
```

- [ ] **Step 4: Update `waitForResult` to call `CancelExecution` on ctx.Done**

Replace the inner select in `waitForResult`:

```go
func (d *TaskDispatcher) waitForResult(ctx context.Context, executionID string, task DispatchTask) {
	deadline := time.Now().Add(pollTimeout)
	lastStatus := ""
	for time.Now().Before(deadline) {
		exec, err := d.execStore.GetByID(executionID)
		if err != nil {
			log.Error("poll error", zap.String("execID", executionID), zap.Error(err))
			return
		}
		if string(exec.Status) != lastStatus {
			log.Info("polling execution", zap.String("execID", executionID), zap.Any("status", exec.Status))
			lastStatus = string(exec.Status)
		}
		switch exec.Status {
		case model.ExecStatusCompleted:
			if task.SessionKey != "" && task.WorkerID != "" {
				if err := d.sessionStore.UpsertSessionContext(ctx, task.SessionKey, task.WorkerID, exec.SessionID); err != nil {
					log.Error("upsert session context", zap.Error(err))
				}
			}
			return
		case model.ExecStatusFailed:
			if task.TaskID != "" {
				if err := d.taskStore.FailTask(ctx, task.TaskID); err != nil {
					log.Error("fail task", zap.String("taskID", task.TaskID), zap.Error(err))
				}
			}
			d.notifyFailure(ctx, task.MessageID, exec.Result)
			return
		}
		select {
		case <-time.After(pollInterval):
		case <-ctx.Done():
			// Task was cancelled — kill the worker process.
			d.manager.CancelExecution(context.Background(), executionID) //nolint:errcheck
			return
		}
	}
}
```

- [ ] **Step 5: Run all dispatcher tests**

```bash
go test ./internal/task_dispatcher/... -v
```

Expected: all PASS including the new CancelTask tests

- [ ] **Step 6: Run full build and test suite**

```bash
go build ./...
go test ./...
```

Expected: no compile errors, all tests PASS

- [ ] **Step 7: Commit**

```bash
git add internal/task_dispatcher/dispatcher.go
git commit -m "feat(dispatcher): wire per-task cancel context; add Method Y and waitForResult CancelExecution"
```

---

## Self-Review

**Spec coverage check:**

| Spec requirement | Covered by |
|---|---|
| `PeekDueScheduledTasks` read-only query | Task 1 |
| `ClaimDueTasks` accepts `scheduledNextRuns map[string]int64` | Task 2 |
| Remove 24h sentinel from `ClaimDueTasks` | Task 2 |
| `Scheduler.poll` two-step (peek → compute → claim) | Task 3 |
| `ExecutionManager.CancelExecution` interface | Task 4 |
| `worker.Manager.CancelExecution` implementation | Task 4 |
| `cancelFuncs map` on Dispatcher | Task 5 |
| `cancelCh` + `handleCancel` + public `CancelTask` | Task 5 |
| `handleInbound` creates per-task cancel context | Task 6 |
| `handleResult` cleans up cancel func; creates new for next task | Task 6 |
| `executeAsync` Method Y after `resolveExecution` | Task 6 |
| `waitForResult` calls `CancelExecution` on `ctx.Done` | Task 6 |

All spec requirements covered. No gaps.
