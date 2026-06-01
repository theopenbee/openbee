# Simplify Round-2 Follow-up Fixes Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Address the 14 simplify-review findings on branch `feat/remove-execution-subcommand` — three real bugs in the new task reconciler (Windows liveness probe, O(N) DB roundtrips, missing ctx cancel check), one API consistency bug in `normalizeTaskExecutionLimit`, two store/dispatcher duplication pairs, a parameter-sprawl on `ExecutionStore.Create`, plus four small cleanups (alias, comments, dead code, op-name strings, tab alignment).

**Architecture:** All fixes are local refactors. Group A fixes correctness bugs in the reconciler. Group B fixes one RPC consistency bug. Group C consolidates duplicated helpers across dispatcher / task_store / clear / execution_store. Group D removes dead/noise (alias wrapper, narrative comments, dead test fakes, stringly-typed op names, tab alignment). No new abstractions, no behavior changes outside fewer DB calls and consistent error policy.

**Tech Stack:** Go 1.x, SQLite (`database/sql`), cobra CLI, `go.uber.org/zap`, existing test harness in `internal/...`.

---

## Execution Order

A → B → C → D. Each task is independent and ends with its own commit. Run `go build ./... && go test ./...` at the end of each group.

---

## Group A — Reconciler correctness (high priority)

### Task A1: Make `isAlive` cross-platform shared between daemon and reconciler (#1)

**Files:**
- Create: `internal/infra/utils/processalive_unix.go` (build tag `!windows`)
- Create: `internal/infra/utils/processalive_windows.go` (build tag `windows`)
- Modify: `cmd/openbee/daemon_unix.go` (delete `isAlive`, import from utils)
- Modify: `cmd/openbee/daemon_windows.go` (delete `isAlive`, import from utils)
- Modify: `internal/domain/task/reconciler.go` (delete `defaultProcessAlive` and `os`/`syscall` imports; use `utils.IsProcessAlive`)
- Test: `internal/infra/utils/processalive_test.go`

- [ ] **Step 1: Write a failing unit test for the new utility**

Create `internal/infra/utils/processalive_test.go`:

```go
package utils

import (
	"os"
	"testing"
)

func TestIsProcessAlive_Self(t *testing.T) {
	if !IsProcessAlive(os.Getpid()) {
		t.Fatal("current process must be alive")
	}
}

func TestIsProcessAlive_InvalidPID(t *testing.T) {
	if IsProcessAlive(0) {
		t.Fatal("pid 0 must not be reported alive")
	}
	if IsProcessAlive(-1) {
		t.Fatal("negative pid must not be reported alive")
	}
}

func TestIsProcessAlive_DeadPID(t *testing.T) {
	// PID 999999 is overwhelmingly unlikely to exist; if it does, skip.
	const pid = 999999
	if IsProcessAlive(pid) {
		t.Skip("pid 999999 happened to exist")
	}
}
```

Run: `go test ./internal/infra/utils/ -run TestIsProcessAlive -v`
Expected: FAIL — `IsProcessAlive` undefined.

- [ ] **Step 2: Implement POSIX variant**

Create `internal/infra/utils/processalive_unix.go`:

```go
//go:build !windows

package utils

import "syscall"

// IsProcessAlive reports whether a process with the given PID is currently running.
// Uses kill(pid, 0) — the zero-signal POSIX liveness probe.
// EPERM (process exists but owned by another user) is treated as "not alive" so
// we never signal a process we do not own.
func IsProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	return syscall.Kill(pid, 0) == nil
}
```

- [ ] **Step 3: Implement Windows variant**

Create `internal/infra/utils/processalive_windows.go`:

```go
//go:build windows

package utils

import "golang.org/x/sys/windows"

// IsProcessAlive reports whether a process with the given PID is currently running on Windows.
func IsProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(h)
	var code uint32
	if err := windows.GetExitCodeProcess(h, &code); err != nil {
		return false
	}
	const stillActive = 259
	return code == stillActive
}
```

- [ ] **Step 4: Run the unit test on host platform**

Run: `go test ./internal/infra/utils/ -run TestIsProcessAlive -v`
Expected: PASS.

- [ ] **Step 5: Replace daemon's `isAlive` with the utility**

In `cmd/openbee/daemon_unix.go`:
- Delete the local `isAlive` function (lines 44-52).
- Replace the body of `stopProcess` so the `!isAlive(pid)` call becomes `!utils.IsProcessAlive(pid)`.
- Add `"github.com/theopenbee/openbee/internal/infra/utils"` to imports.

In `cmd/openbee/daemon_windows.go`:
- Delete the local `isAlive` function (lines 48-61).
- Replace `!isAlive(pid)` in `stopProcess` with `!utils.IsProcessAlive(pid)`.
- Add the utils import.

If both files still reference `syscall`/`windows` for other reasons leave those imports; otherwise drop unused imports.

- [ ] **Step 6: Replace reconciler's `defaultProcessAlive` with the utility**

In `internal/domain/task/reconciler.go`:
- Delete `defaultProcessAlive` (lines 154-168) and the `os` + `syscall` imports.
- Replace `processAlive: defaultProcessAlive,` (line 55) with `processAlive: utils.IsProcessAlive,`.
- Add `"github.com/theopenbee/openbee/internal/infra/utils"` to imports.

- [ ] **Step 7: Verify**

Run: `go build ./... && go test ./internal/domain/task/... ./internal/infra/utils/... ./cmd/openbee/...`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/infra/utils/processalive_unix.go internal/infra/utils/processalive_windows.go \
        internal/infra/utils/processalive_test.go cmd/openbee/daemon_unix.go \
        cmd/openbee/daemon_windows.go internal/domain/task/reconciler.go
git commit -m "refactor: share cross-platform IsProcessAlive between daemon and reconciler"
```

---

### Task A2: Batch reconciler DB lookups + add ctx cancel guard (#2, #3)

**Files:**
- Modify: `internal/domain/task/reconciler.go` (rewrite `reconcile`; drop `latestExecution`; require batch interface from execStore)
- Modify: `internal/domain/task/reconciler_test.go` (update test fakes to satisfy the batch-only interface; assert single round-trip)

The new exec-store contract used by the reconciler is two batch calls per tick instead of 2N. `GetRunningByTaskID` is no longer needed by the reconciler (callers in commands keep using it via the lookup interface).

- [ ] **Step 1: Write a failing test that asserts batch behavior**

In `internal/domain/task/reconciler_test.go`, add a test (or extend `TestReconciler_*`) that calls `reconcile` with 3 running tasks and asserts that:
- `ListByTaskIDsCalls == 1`
- `RunningExecIDsByTaskIDsCalls == 1`
- `GetRunningByTaskIDCalls == 0`

Extend the existing fake `reconcilerExecStore` so it counts calls per method. Example skeleton (paste-ready, adapt to existing fake conventions in the file):

```go
type recExecStoreFake struct {
	listByTaskIDsCalls   int
	runningIDsCalls      int
	getRunningCalls      int
	markAbandonedCalls   int
	listByTaskIDsResult  map[string][]model.WorkerExecution
	runningIDsResult     map[string]string
	markAbandonedResult  bool
}

func (f *recExecStoreFake) ListByTaskIDs(_ context.Context, _ []string, _ int) (map[string][]model.WorkerExecution, error) {
	f.listByTaskIDsCalls++
	return f.listByTaskIDsResult, nil
}
func (f *recExecStoreFake) RunningExecIDsByTaskIDs(_ context.Context, _ []string) (map[string]string, error) {
	f.runningIDsCalls++
	return f.runningIDsResult, nil
}
func (f *recExecStoreFake) MarkAbandoned(_ context.Context, _, _ string) (bool, error) {
	f.markAbandonedCalls++
	return f.markAbandonedResult, nil
}
```

Run: `go test ./internal/domain/task/ -run TestReconciler -v`
Expected: FAIL — fake doesn't satisfy new interface yet / counts wrong.

- [ ] **Step 2: Update the `reconcilerExecStore` interface**

In `internal/domain/task/reconciler.go`, replace the existing interface with:

```go
type reconcilerExecStore interface {
	ListByTaskIDs(ctx context.Context, taskIDs []string, limitPerTask int) (map[string][]model.WorkerExecution, error)
	RunningExecIDsByTaskIDs(ctx context.Context, taskIDs []string) (map[string]string, error)
	MarkAbandoned(ctx context.Context, id, result string) (bool, error)
}
```

Confirm `store.ExecutionStore` already implements `RunningExecIDsByTaskIDs` (added on this branch in `execution_store.go`).

- [ ] **Step 3: Rewrite `reconcile` to batch + check ctx**

Replace `reconcile`/`reconcileOne`/`latestExecution` (lines 73-152) with:

```go
func (r *Reconciler) reconcile(ctx context.Context) {
	tasks, err := r.taskStore.List(ctx, store.TaskFilter{Status: model.TaskStatusRunning})
	if err != nil {
		log.Error("reconciler: list running tasks", zap.Error(err))
		return
	}
	if len(tasks) == 0 {
		return
	}
	taskIDs := make([]string, len(tasks))
	for i, t := range tasks {
		taskIDs[i] = t.ID
	}
	runningIDs, err := r.execStore.RunningExecIDsByTaskIDs(ctx, taskIDs)
	if err != nil {
		log.Error("reconciler: running exec ids", zap.Error(err))
		return
	}
	latestByTask, err := r.execStore.ListByTaskIDs(ctx, taskIDs, 1)
	if err != nil {
		log.Error("reconciler: list executions", zap.Error(err))
		return
	}
	for _, t := range tasks {
		select {
		case <-ctx.Done():
			return
		default:
		}
		latest := pickLatest(latestByTask[t.ID], runningIDs[t.ID])
		if latest == nil {
			continue
		}
		r.applyDecision(ctx, t, *latest)
	}
}

// pickLatest returns the running execution (if any) reconstructed from runningID
// plus the latest-by-start-time row from listByTaskIDs. Running wins because the
// reconciler's job is to detect orphans for *currently claimed* executions.
func pickLatest(latest []model.WorkerExecution, runningID string) *model.WorkerExecution {
	if runningID != "" {
		for i := range latest {
			if latest[i].ID == runningID {
				return &latest[i]
			}
		}
		// Running row not present in the per-task latest set (rare race where the
		// running row started after the ListByTaskIDs snapshot). Skip this tick;
		// the next tick will catch it.
		return nil
	}
	if len(latest) == 0 {
		return nil
	}
	return &latest[0]
}

func (r *Reconciler) applyDecision(ctx context.Context, t model.Task, latest model.WorkerExecution) {
	switch latest.Status {
	case model.ExecStatusCompleted:
		if err := r.taskStore.CompleteTask(ctx, t.ID); err != nil {
			log.Error("reconciler: complete stale task", zap.String("taskID", t.ID), zap.String("execID", latest.ID), zap.Error(err))
			return
		}
		log.Warn("reconciler: marked stale running task completed",
			zap.String("taskID", t.ID), zap.String("execID", latest.ID))
	case model.ExecStatusFailed:
		if err := r.taskStore.FailTask(ctx, t.ID); err != nil {
			log.Error("reconciler: fail stale task", zap.String("taskID", t.ID), zap.String("execID", latest.ID), zap.Error(err))
			return
		}
		log.Warn("reconciler: marked stale running task failed",
			zap.String("taskID", t.ID), zap.String("execID", latest.ID))
	case model.ExecStatusRunning:
		if latest.AIProcessPID <= 0 || r.processAlive(latest.AIProcessPID) {
			return
		}
		if _, err := r.execStore.MarkAbandoned(ctx, latest.ID, "abandoned: process exited without completion signal"); err != nil {
			log.Error("reconciler: mark exec abandoned", zap.String("execID", latest.ID), zap.Error(err))
			return
		}
		if err := r.taskStore.FailTask(ctx, t.ID); err != nil {
			log.Error("reconciler: fail orphaned task", zap.String("taskID", t.ID), zap.String("execID", latest.ID), zap.Error(err))
			return
		}
		log.Warn("reconciler: swept orphaned running task",
			zap.String("taskID", t.ID), zap.String("execID", latest.ID), zap.Int("pid", latest.AIProcessPID))
	}
}
```

- [ ] **Step 4: Update `reconciler_export_test.go` if it exports `latestExecution`**

If the export file references the removed `latestExecution`, drop that export. Add an export for `pickLatest` if tests want to verify it directly.

- [ ] **Step 5: Run tests**

Run: `go test ./internal/domain/task/... -v`
Expected: PASS, including the new batch-call assertion.

- [ ] **Step 6: Commit**

```bash
git add internal/domain/task/reconciler.go internal/domain/task/reconciler_test.go internal/domain/task/reconciler_export_test.go
git commit -m "perf: batch reconciler DB calls and honour ctx cancellation per task"
```

---

### Task A3: Guard task status writes against no-op updates (#9)

**Files:**
- Modify: `internal/infra/store/task_store.go` (new `UpdateStatusIfRunning` method)
- Modify: `internal/domain/task/reconciler.go` (use guarded helper for the running→terminal transition; replace `CompleteTask`/`FailTask` calls in reconciler only)
- Test: `internal/infra/store/task_store_test.go` (new test)

**Scope note:** Only the reconciler gets the guarded write; dispatcher/handler call sites legitimately want unconditional updates and are out of scope.

- [ ] **Step 1: Write the failing store test**

Append to `task_store_test.go`:

```go
func TestTaskStore_UpdateStatusIfRunning_Skips_NonRunning(t *testing.T) {
	ctx := context.Background()
	store := newTestTaskStore(t)
	id := mustInsertTask(t, store, model.TaskStatusPending)

	changed, err := store.UpdateStatusIfRunning(ctx, id, model.TaskStatusCompleted)
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Fatal("expected no-op when current status != running")
	}
	got := mustGet(t, store, id)
	if got.Status != model.TaskStatusPending {
		t.Fatalf("status mutated: got %q", got.Status)
	}
}

func TestTaskStore_UpdateStatusIfRunning_Transitions(t *testing.T) {
	ctx := context.Background()
	store := newTestTaskStore(t)
	id := mustInsertTask(t, store, model.TaskStatusRunning)

	changed, err := store.UpdateStatusIfRunning(ctx, id, model.TaskStatusFailed)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("expected transition from running")
	}
	got := mustGet(t, store, id)
	if got.Status != model.TaskStatusFailed {
		t.Fatalf("status not updated: %q", got.Status)
	}
}
```

(Use existing test helpers in the file; if `newTestTaskStore`/`mustInsertTask`/`mustGet` don't exist verbatim, follow the local convention.)

Run: `go test ./internal/infra/store/ -run TestTaskStore_UpdateStatusIfRunning -v`
Expected: FAIL — method undefined.

- [ ] **Step 2: Implement `UpdateStatusIfRunning`**

In `task_store.go`, add next to `UpdateStatus`:

```go
// UpdateStatusIfRunning transitions a task to `next` only if its current status
// is `running`. Returns true when the row changed. Used by the reconciler to
// avoid bumping updated_at on tasks already moved to a terminal state by the
// dispatcher in the time between List and the write.
func (s *TaskStore) UpdateStatusIfRunning(ctx context.Context, taskID string, next string) (bool, error) {
	res, err := s.db.ExecContext(ctx,
		`UPDATE bee_tasks SET status = ?, updated_at = ? WHERE id = ? AND status = ?`,
		next, time.Now().UnixMilli(), taskID, model.TaskStatusRunning)
	if err != nil {
		return false, fmt.Errorf("update task status if running: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("rows affected: %w", err)
	}
	return n > 0, nil
}
```

- [ ] **Step 3: Run store tests**

Run: `go test ./internal/infra/store/ -run TestTaskStore_UpdateStatusIfRunning -v`
Expected: PASS.

- [ ] **Step 4: Switch reconciler to use the guarded helper**

In `internal/domain/task/reconciler.go`:
- Extend `reconcilerTaskStore` interface: drop `CompleteTask`/`FailTask`; add `UpdateStatusIfRunning(ctx, id, next string) (bool, error)`.
- In `applyDecision`, replace each `r.taskStore.CompleteTask(...)` with `r.taskStore.UpdateStatusIfRunning(ctx, t.ID, model.TaskStatusCompleted)` and each `FailTask` with `... model.TaskStatusFailed`. Log only when `changed == true` (skip the warn log otherwise; the dispatcher already handled it).

Adjust the fake in `reconciler_test.go` accordingly (drop `CompleteTask`/`FailTask` counters, add `UpdateStatusIfRunningCalls`).

- [ ] **Step 5: Verify everything**

Run: `go test ./internal/domain/task/... ./internal/infra/store/...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/infra/store/task_store.go internal/infra/store/task_store_test.go \
        internal/domain/task/reconciler.go internal/domain/task/reconciler_test.go
git commit -m "perf: guard reconciler task status writes against no-op updates"
```

---

## Group B — RPC consistency

### Task B1: Make `normalizeTaskExecutionLimit` consistent (#4)

**Files:**
- Modify: `internal/rpc/tools.go:1213-1230` (`normalizeTaskExecutionLimit`)
- Modify: `internal/rpc/tools_test.go` (extend test table for the chosen policy)

**Decision required (default to "clamp both" — symmetric, friendlier API):** treat `*raw < 0` the same as `*raw > maxTaskExecutionLimit`: silently clamp to bounds (0 and `maxTaskExecutionLimit`).

- [ ] **Step 1: Write the failing test**

Append to `tools_test.go`:

```go
func TestNormalizeTaskExecutionLimit_NegativeIsClampedToZero(t *testing.T) {
	raw := -5
	got, err := normalizeTaskExecutionLimit(&raw, 1)
	if err != nil {
		t.Fatal(err)
	}
	if got != 0 {
		t.Fatalf("expected clamp to 0, got %d", got)
	}
}
```

Also keep the existing zero/normal/over-max cases.

Run: `go test ./internal/rpc/ -run TestNormalizeTaskExecutionLimit -v`
Expected: FAIL — current code returns an error.

- [ ] **Step 2: Update the implementation**

In `tools.go:1213-1230`, replace the function with:

```go
func normalizeTaskExecutionLimit(raw *int, matchedTasks int) (int, error) {
	if raw == nil {
		return defaultTaskExecutionLimit, nil
	}
	v := *raw
	if v == 0 {
		if matchedTasks != 1 {
			return 0, fmt.Errorf("execution_limit=0 requires exactly one matching task; use task_id to select one task")
		}
		return 0, nil
	}
	if v < 0 {
		return 0, nil
	}
	if v > maxTaskExecutionLimit {
		return maxTaskExecutionLimit, nil
	}
	return v, nil
}
```

The `matchedTasks` gate stays — it is the only path that genuinely needs an error because a 0 limit on N tasks has no defined behavior.

- [ ] **Step 3: Run the test**

Run: `go test ./internal/rpc/ -run TestNormalizeTaskExecutionLimit -v`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/rpc/tools.go internal/rpc/tools_test.go
git commit -m "fix: clamp negative execution_limit instead of erroring"
```

---

## Group C — Dedupe and parameter sprawl

### Task C1: Collapse `clearWorkerQueue` + `clearQueues` (#5)

**Files:**
- Modify: `internal/domain/task/dispatcher.go:260-294`
- Test: `internal/domain/task/dispatcher_test.go` (existing tests should still pass; add one if no test currently exercises the worker-scoped path)

- [ ] **Step 1: Run existing tests for context**

Run: `go test ./internal/domain/task/ -run TestTaskDispatcher -v`
Note which tests exist that hit `clearWorkerQueue` / `clearQueues`. If neither path is covered, add a quick test using the existing fake setup before refactoring.

- [ ] **Step 2: Introduce `dropQueued`**

Replace lines 260-294 with:

```go
// dropQueued removes pending tasks from every queue for which keep(task) is false.
// Empty, idle queues are deleted from d.queues.
func (d *TaskDispatcher) dropQueued(keep func(DispatchTask) bool) {
	for key, state := range d.queues {
		var remaining []DispatchTask
		for _, t := range state.pendingTasks {
			if keep(t) {
				remaining = append(remaining, t)
			}
		}
		state.pendingTasks = remaining
		if !state.executing && len(state.pendingTasks) == 0 {
			delete(d.queues, key)
		}
	}
}

// clearWorkerQueue drops queued (not-yet-executing) tasks for the (sessionKey, workerID) pair.
// Tasks already running keep going — the command layer stops their executions separately.
func (d *TaskDispatcher) clearWorkerQueue(sessionKey, workerID string) {
	d.dropQueued(func(t DispatchTask) bool {
		return t.SessionKey != sessionKey || t.WorkerID != workerID
	})
}

func (d *TaskDispatcher) clearQueues(sessionKey string) {
	d.dropQueued(func(t DispatchTask) bool { return t.SessionKey != sessionKey })
}
```

Note: `clearWorkerQueue` previously short-circuited when the (sessionKey, workerID) queue key wasn't present. The new form iterates every queue but applies the same predicate; cost difference is negligible (a few map iterations) and the behavior is identical because the predicate keeps tasks that don't match.

- [ ] **Step 3: Run tests**

Run: `go test ./internal/domain/task/ -v`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/domain/task/dispatcher.go
git commit -m "refactor: unify clear-queue methods on dispatcher via dropQueued"
```

---

### Task C2: Route `ListBySessionKey`/`ListBySessionAndWorker` + cancel pair through filter structs (#6)

**Files:**
- Modify: `internal/infra/store/task_store.go` (delete the four narrow methods; add a `CancelFilter` for cancel, route list through existing `TaskFilter` via `List`)
- Modify: callers in `internal/domain/command/clear.go` (use `List(ctx, store.TaskFilter{...})` and `Cancel(ctx, store.CancelFilter{...})`)
- Modify: `internal/infra/store/task_store_test.go`, `internal/domain/command/clear_test.go`, `fakes_test.go`

**Approach:** `TaskFilter` already has `SessionKey`, `WorkerID`, `Status`, `Type` — list path just needs callers to use `s.List(ctx, TaskFilter{...})`. Cancel side gets a new `CancelFilter` struct (no equivalent today) and a single `Cancel(ctx, CancelFilter)` method.

- [ ] **Step 1: Confirm `TaskFilter` shape**

In `task_store.go` (around line 68), verify `TaskFilter` contains `SessionKey`, `WorkerID`, `Status`, `Type`. Spot-check: `git grep "type TaskFilter struct" internal/infra/store/`.

If those fields exist (they do per this branch), no new struct needed for the list path.

- [ ] **Step 2: Delete `ListBySessionKey` and `ListBySessionAndWorker`**

In `task_store.go`, remove both methods (lines ~141-181). Callers will use `List(ctx, TaskFilter{...})` directly.

- [ ] **Step 3: Add `CancelFilter` and `Cancel`**

In `task_store.go`, add near `TaskFilter`:

```go
// CancelFilter selects which pending/running tasks Cancel will move to cancelled.
// Either SessionKey, WorkerID, or both may be set. Empty Type matches all types.
type CancelFilter struct {
	SessionKey string
	WorkerID   string
	Type       string
}
```

Add the unified `Cancel`:

```go
// Cancel marks tasks selected by f as cancelled. Returns the number of rows updated.
// At least one of f.SessionKey or f.WorkerID must be set.
func (s *TaskStore) Cancel(ctx context.Context, f CancelFilter) (int64, error) {
	if f.SessionKey == "" && f.WorkerID == "" {
		return 0, fmt.Errorf("cancel: SessionKey or WorkerID required")
	}
	q := `UPDATE bee_tasks SET status = 'cancelled', updated_at = ?
	      WHERE status IN ('pending', 'running')`
	args := []any{time.Now().UnixMilli()}
	if f.SessionKey != "" {
		q += " AND message_id IN (SELECT id FROM bee_platform_messages WHERE session_key = ?)"
		args = append(args, f.SessionKey)
	}
	if f.WorkerID != "" {
		q += " AND worker_id = ?"
		args = append(args, f.WorkerID)
	}
	if f.Type != "" {
		q += " AND type = ?"
		args = append(args, f.Type)
	}
	res, err := s.db.ExecContext(ctx, q, args...)
	if err != nil {
		return 0, fmt.Errorf("cancel tasks: %w", err)
	}
	return res.RowsAffected()
}
```

- [ ] **Step 4: Delete `CancelBySessionKey` and `CancelBySessionAndWorker`**

Remove both methods (lines ~307-341). `CancelByWorkerID` stays as-is for now (single-field; its caller in worker manager is fine).

- [ ] **Step 5: Update callers in `clear.go`**

Update the `clearTaskStore` interface to:

```go
type clearTaskStore interface {
	List(ctx context.Context, f store.TaskFilter) ([]model.Task, error)
	Cancel(ctx context.Context, f store.CancelFilter) (int64, error)
}
```

Then rewrite the four call sites:
- `handleClearAll` line 125: `h.tasks.ListBySessionKey(ctx, sessionKey, model.TaskStatusRunning, model.TaskTypeImmediate)` → `h.tasks.List(ctx, store.TaskFilter{SessionKey: sessionKey, Status: model.TaskStatusRunning, Type: model.TaskTypeImmediate})`.
- `handleClearAll` line 149: `h.tasks.CancelBySessionKey(ctx, sessionKey, model.TaskTypeImmediate)` → `h.tasks.Cancel(ctx, store.CancelFilter{SessionKey: sessionKey, Type: model.TaskTypeImmediate})`.
- `handleClearWorker` line 210: `ListBySessionAndWorker(ctx, sessionKey, w.ID, ...)` → `List(ctx, store.TaskFilter{SessionKey: sessionKey, WorkerID: w.ID, Status: model.TaskStatusRunning, Type: model.TaskTypeImmediate})`.
- `handleClearWorker` line 236: `CancelBySessionAndWorker(ctx, sessionKey, w.ID, model.TaskTypeImmediate)` → `Cancel(ctx, store.CancelFilter{SessionKey: sessionKey, WorkerID: w.ID, Type: model.TaskTypeImmediate})`.

- [ ] **Step 6: Update fake in `fakes_test.go`**

The existing fake's `ListBySessionKey` / `ListBySessionAndWorker` / `CancelBySessionKey` / `CancelBySessionAndWorker` methods get replaced by `List(ctx, f TaskFilter)` and `Cancel(ctx, f CancelFilter)`. Branch on `f.WorkerID == ""` (or some other distinguishing field) to preserve the per-path error-injection behavior the tests rely on.

- [ ] **Step 7: Update store tests**

In `task_store_test.go`, replace tests for the deleted methods with tests on `List`/`Cancel` exercising the same scenarios:
- `TaskFilter{SessionKey: ..., Status: "running", Type: "immediate"}` returns all session tasks.
- `TaskFilter{SessionKey: ..., WorkerID: ..., Status: "running", Type: "immediate"}` returns only that worker's tasks.
- `CancelFilter{SessionKey: ...}` cancels all in session.
- `CancelFilter{SessionKey: ..., WorkerID: ...}` only cancels that worker's.
- `CancelFilter{}` returns the required-field error.

Run: `go build ./... && go test ./internal/infra/store/... ./internal/domain/command/...`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/infra/store/task_store.go internal/infra/store/task_store_test.go \
        internal/domain/command/clear.go internal/domain/command/fakes_test.go \
        internal/domain/command/clear_test.go
git commit -m "refactor: route session+worker task queries through TaskFilter and CancelFilter"
```

---

### Task C3: Extract `stopRunningExecutions` and a single confirm-prompt builder in `clear.go` (#7)

**Files:**
- Modify: `internal/domain/command/clear.go`

- [ ] **Step 1: Introduce `stopRunningExecutions`**

After `formatConfirmPrompt`, add:

```go
// stopRunningExecutions resolves the running exec IDs for tasks and stops each one.
// op is a short tag used only in log messages.
func (h *ClearCommandHandler) stopRunningExecutions(ctx context.Context, tasks []model.Task, op string) {
	execIDs := runningExecIDsForTasks(ctx, h.runningExecs, tasks, op)
	for _, t := range tasks {
		execID := execIDs[t.ID]
		if execID == "" {
			continue
		}
		if err := h.execStopper.StopExecution(execID); err != nil {
			log.Error("stop execution for "+op, zap.String("executionID", execID), zap.Error(err))
		}
	}
}
```

Replace the two inline blocks at lines 138-147 and 225-234 with `h.stopRunningExecutions(ctx, runningTasks, "clear")` and `... "clear_worker"`.

- [ ] **Step 2: Unify the two confirm-prompt builders**

Define a single helper:

```go
type confirmPromptArgs struct {
	headerLines []string
	tasks       []model.Task
	workerNames map[string]string
	footer      string
	op          string
}

func (h *ClearCommandHandler) renderConfirmPrompt(ctx context.Context, a confirmPromptArgs) string {
	nowMs := h.now().UnixMilli()
	execIDs := runningExecIDsForTasks(ctx, h.runningExecs, a.tasks, a.op)
	lines := make([]string, 0, 5+len(a.headerLines)+len(a.tasks))
	lines = append(lines, a.headerLines...)
	lines = append(lines, "")
	lines = append(lines, fmt.Sprintf(i18n.M.Runtime.ClearCommand.ConfirmTasksHeader, len(a.tasks)))
	for _, t := range a.tasks {
		lines = append(lines, formatTaskLine(i18n.M.Runtime.StatusCommand.TaskLine, t, a.workerNames, execIDs, nowMs))
	}
	lines = append(lines, "")
	lines = append(lines, a.footer)
	return strings.Join(lines, "\n")
}
```

Reduce `formatConfirmPrompt` to build the header/footer from `m.ConfirmHeader`, `m.ConfirmAgentLine`, `m.ConfirmFooter`, call `renderConfirmPrompt`, and return. Same for `formatWorkerConfirmPrompt` with `m.WorkerConfirmHeader`/`m.WorkerConfirmFooter`.

- [ ] **Step 3: Run tests**

Run: `go test ./internal/domain/command/... -v`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/domain/command/clear.go
git commit -m "refactor: extract stopRunningExecutions and renderConfirmPrompt in clear handler"
```

---

### Task C4: Replace `ExecutionStore.Create` positional sprawl with a struct (#8)

**Files:**
- Modify: `internal/infra/store/execution_store.go` (new method `Create(ctx, ExecutionCreate)`; keep old signature as a thin shim if many callers, else replace and migrate callers)
- Modify: every caller of `Create`/`CreateBeeExecution` — see Step 1 list

- [ ] **Step 1: Inventory callers**

Run: search for `.Create(` in `execution_store.go` callers.
Expected: a small list — `internal/domain/task/dispatcher.go`, `internal/domain/worker/execution.go`, and several test files. Capture the full list before editing.

- [ ] **Step 2: Define the parameter struct + new method**

In `execution_store.go`:

```go
// ExecutionCreate carries the inputs for ExecutionStore.Create.
// WorkerID is empty for bee-side executions; TaskID is empty for raw exec rows
// not driven by a task.
type ExecutionCreate struct {
	WorkerID     string
	TaskID       string
	TriggerInput string
	SessionID    string
	Engine       string
}

func (s *ExecutionStore) Create(ctx context.Context, c ExecutionCreate) (string, error) {
	// existing body, reading from c.* instead of named positional params
}
```

Drop the old positional `Create(workerID, taskID, triggerInput, sessionID, engine string)` overload.

- [ ] **Step 3: Migrate callers**

For each caller, rewrite as:

```go
execID, err := store.Create(ctx, store.ExecutionCreate{
    WorkerID:     workerID,
    TaskID:       taskID,
    TriggerInput: triggerInput,
    SessionID:    sessionID,
    Engine:       engineName,
})
```

For test sites that previously passed `""` for `workerID`/`taskID`, simply omit the field.

- [ ] **Step 4: Build and test**

Run: `go build ./... && go test ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/infra/store/execution_store.go internal/infra/store/execution_store_test.go \
        internal/domain/task/dispatcher.go internal/domain/worker/execution.go \
        internal/domain/worker/manager_test.go
git commit -m "refactor: replace ExecutionStore.Create positional args with ExecutionCreate struct"
```

---

### Task C5: Delete `RunningExecLookup` alias + `runningExecIDsForTasks` wrapper (#10)

**Files:**
- Modify: `internal/domain/command/task_format.go` (delete alias + wrapper)
- Modify: callers in `internal/domain/command/clear.go`, `status.go` (use `utils.RunningExecIDsForTasks` directly)

- [ ] **Step 1: Delete the alias and wrapper**

In `task_format.go`, delete lines 18-24:

```go
// RunningExecLookup is re-exported from utils so callers in this package can
// satisfy the helper without importing utils directly.
type RunningExecLookup = utils.RunningExecLookup

func runningExecIDsForTasks(...) {...}
```

- [ ] **Step 2: Update callers**

Replace every call site `runningExecIDsForTasks(ctx, h.runningExecs, tasks, op)` with `utils.RunningExecIDsForTasks(ctx, log, h.runningExecs, tasks, op)`.

Update field types: the struct fields previously typed `RunningExecLookup` change to `utils.RunningExecLookup`.

- [ ] **Step 3: Build + test**

Run: `go build ./... && go test ./internal/domain/command/...`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/domain/command/task_format.go internal/domain/command/clear.go \
        internal/domain/command/status.go internal/domain/command/clear_test.go \
        internal/domain/command/status_test.go internal/domain/command/fakes_test.go
git commit -m "refactor: drop RunningExecLookup alias and wrapper in command package"
```

---

## Group D — Cleanup (no behavior change)

### Task D1: Trim narrative comments (#11)

**Files:**
- Modify: `internal/domain/task/dispatcher.go:340-343` — keep one-line WHY ("Deadline is enforced by launchRuntime; do not duplicate.").
- Modify: `internal/domain/worker/execution.go:60-63` — keep one-line WHY ("Cancelling the dispatcher must cancel the worker process."). Drop the change-log sentence.
- Modify: `internal/domain/command/clear.go:250-251` — delete the multi-line comment; the code is self-explanatory.
- Modify: `internal/domain/task/reconciler.go:62-69` (now mostly removed by A2; finish trimming if any narrative remains).

For each file:
- [ ] Inspect the comment, keep only non-obvious WHY, delete narrative referring to past bugs.
- [ ] Build: `go build ./...`
- [ ] Commit:

```bash
git add internal/domain/task/dispatcher.go internal/domain/worker/execution.go internal/domain/command/clear.go internal/domain/task/reconciler.go
git commit -m "docs: trim narrative comments to non-obvious WHY"
```

---

### Task D2: Remove dead test fakes and rename stale test (#12)

**Files:**
- Modify: `internal/domain/command/clear_test.go:41-43` — delete unused `workerListErr` / `workerCancelErr` fields. If a test should exist for those error paths, add one instead; otherwise remove the fields.
- Modify: `internal/infra/store/execution_store_test.go` — rename `TestExecutionStore_CreateBeeExecution` to `TestExecutionStore_Create_EmptyWorkerID` (or similar accurate name).

- [ ] **Step 1**: Grep `workerListErr` / `workerCancelErr` across the repo. Run: confirm no production code references either.
- [ ] **Step 2**: Delete the unused fields.
- [ ] **Step 3**: Rename the test.
- [ ] **Step 4**: Build + test:

```bash
go test ./internal/domain/command/... ./internal/infra/store/...
```

- [ ] **Step 5**: Commit:

```bash
git add internal/domain/command/clear_test.go internal/domain/command/fakes_test.go internal/infra/store/execution_store_test.go
git commit -m "test: drop dead fake fields and rename CreateBeeExecution test"
```

---

### Task D3: Move op-name strings into constants (#13)

**Files:**
- Modify: `internal/domain/command/clear.go` — declare:

```go
const (
	clearOpAll                = "clear"
	clearOpAllConfirm         = "clear_confirm"
	clearOpWorker             = "clear_worker"
	clearOpWorkerConfirm      = "clear_worker_confirm"
)
```

Replace the four literals at lines 138, 167, 225, 268 with the constants.

- Modify: `internal/rpc/tools.go` — declare `const clearSessionOp = "clear_session"` near the existing tool-name constants and replace line 686.

- [ ] **Step 1**: Add constants.
- [ ] **Step 2**: Replace literals.
- [ ] **Step 3**: Build:

```bash
go build ./... && go test ./internal/domain/command/... ./internal/rpc/...
```

- [ ] **Step 4**: Commit:

```bash
git add internal/domain/command/clear.go internal/rpc/tools.go
git commit -m "refactor: name clear op-strings via constants"
```

---

### Task D4: Restore tab alignment in `messages.go` (#14)

**Files:**
- Modify: `internal/infra/i18n/messages.go:287-290` — re-align with tabs to match surrounding struct fields.

- [ ] **Step 1**: Open `internal/infra/i18n/messages.go`, lines 287-290; change any space-indented alignment back to a single tab between field name and type, matching adjacent lines.
- [ ] **Step 2**: Run `gofmt -l internal/infra/i18n/messages.go` — should output nothing.
- [ ] **Step 3**: Build:

```bash
go build ./...
```

- [ ] **Step 4**: Commit:

```bash
git add internal/infra/i18n/messages.go
git commit -m "style: restore tab alignment in i18n messages"
```

---

## Final verification

After every commit in Groups A–D lands:

- [ ] Run full test suite:

```bash
go build ./...
go test ./...
```

Expected: PASS.

- [ ] Diff summary check:

```bash
git log --oneline main..HEAD
```

Confirm 13–14 new commits, one per task.

---

## Self-Review Notes

Coverage:
- #1 → A1
- #2 → A2
- #3 → A2 (cancel guard)
- #4 → B1
- #5 → C1
- #6 → C2
- #7 → C3
- #8 → C4
- #9 → A3
- #10 → C5
- #11 → D1
- #12 → D2
- #13 → D3
- #14 → D4

Not in scope (deliberately):
- Original review item #15 (`waitForResult` 2s polling) — pre-dates this branch and would change the dispatcher's event model; out of scope.
- `toolGetSystemOverview` parallelization (mentioned in efficiency review) — not on the original 14-item list.
