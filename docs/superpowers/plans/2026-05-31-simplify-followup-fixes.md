# Simplify Follow-up Fixes Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Address the 10 simplify-review findings on branch `feat/remove-execution-subcommand` — code reuse, quality, and efficiency cleanups identified after the task↔execution inversion landed.

**Architecture:** All fixes are local refactors. Group A is pure cleanup; Group B replaces stringly-typed branches with existing constants; Group C consolidates duplicated store/RPC helpers; Group D parallelizes one sequential pair of DB calls; Group E swaps a sentinel int for `cobra.Flag.Changed`. No new abstractions, no behavior changes outside of fewer DB round-trips for `toolListTasks`.

**Tech Stack:** Go 1.x, SQLite (`database/sql`), cobra CLI, `go.uber.org/zap`, existing test harness.

---

## Execution Order

A → B → C → D → E. Each task is independent and ends with its own commit. Run the full test suite at the end of each group.

---

## Group A — Pure cleanup (no behavior change)

### Task A1: Replace `splitTrimmed` with `utils.SplitAndTrim`

**Files:**
- Modify: `internal/infra/store/task_store.go:60` and `:68-82` (delete `splitTrimmed`)
- Modify: `internal/infra/store/task_store.go:1-10` (import block)

- [ ] **Step 1: Confirm there is no other caller of `splitTrimmed`**

Run: search the repo for the symbol.
Expected: only `appendCSVFilter` references it.

- [ ] **Step 2: Add the utils import and switch the call**

In `task_store.go` ensure the import block contains `"github.com/theopenbee/openbee/internal/infra/utils"` (it already imports `strings`; if `strings` is no longer used after removing `splitTrimmed`, drop it). Replace line 60:

```go
	values := utils.SplitAndTrim(value)
```

- [ ] **Step 3: Delete the `splitTrimmed` function**

Remove `task_store.go:68-82` (the doc comment and the function).

- [ ] **Step 4: Build + test**

```bash
go build ./...
go test ./internal/infra/store/...
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/infra/store/task_store.go
git commit -m "refactor: reuse utils.SplitAndTrim in appendCSVFilter"
```

---

### Task A2: Drop dead defensive pre-fill in `ListByTaskIDs`

**Files:**
- Modify: `internal/infra/store/execution_store.go:223-235`
- Verify: `internal/rpc/tools.go:466-472` (consumer reads `execsByTask[t.ID]`; nil-miss is fine because slice append-to-nil works and ranges over nil are no-ops)

- [ ] **Step 1: Write/confirm a test that documents the new contract**

In `internal/infra/store/execution_store_test.go`, find the existing `TestExecutionStore_ListByTaskIDs*` cases. Add or adjust one assertion confirming that a task id with no executions is **absent** from the returned map (or present with nil — pick the new contract; recommendation: absent).

```go
func TestExecutionStore_ListByTaskIDs_OmitsTasksWithoutExecutions(t *testing.T) {
	ctx := context.Background()
	s, _ := newTestExecutionStore(t)

	out, err := s.ListByTaskIDs(ctx, []string{"task-with-no-execs"}, 0)
	if err != nil {
		t.Fatalf("ListByTaskIDs: %v", err)
	}
	if _, ok := out["task-with-no-execs"]; ok {
		t.Fatalf("expected absent key for task with no executions, got %v", out["task-with-no-execs"])
	}
}
```

- [ ] **Step 2: Run the new test and watch it fail**

```bash
go test ./internal/infra/store/ -run TestExecutionStore_ListByTaskIDs_OmitsTasksWithoutExecutions -v
```

Expected: FAIL (current code pre-fills empty slices).

- [ ] **Step 3: Remove the pre-fill loop and rewrite the doc comment**

In `internal/infra/store/execution_store.go` replace lines 223-235 with:

```go
// ListByTaskIDs returns executions grouped by task_id, newest first within each
// task. Task ids with no executions are omitted from the result. When
// limitPerTask > 0, at most that many executions are returned per task;
// limitPerTask <= 0 returns all executions.
func (s *ExecutionStore) ListByTaskIDs(ctx context.Context, taskIDs []string, limitPerTask int) (map[string][]model.WorkerExecution, error) {
	if len(taskIDs) == 0 {
		return map[string][]model.WorkerExecution{}, nil
	}
	out := make(map[string][]model.WorkerExecution, len(taskIDs))
	args := stringsToArgs(taskIDs)
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/infra/store/... ./internal/rpc/...
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/infra/store/execution_store.go internal/infra/store/execution_store_test.go
git commit -m "refactor: drop empty-slice prefill in ListByTaskIDs"
```

---

### Task A3: Drop redundant `ORDER BY e.task_id ASC` from `ListByTaskIDs`

**Files:**
- Modify: `internal/infra/store/execution_store.go:240` and `:254`

- [ ] **Step 1: Change both ORDER BY clauses**

Replace `ORDER BY e.task_id ASC, e.started_at DESC, e.rowid DESC` with `ORDER BY e.started_at DESC, e.rowid DESC` in both query branches (the `limitPerTask <= 0` branch at line 240 and the windowed branch at line 254).

- [ ] **Step 2: Run tests**

```bash
go test ./internal/infra/store/...
```

Expected: PASS (existing tests group results into a map; intra-task ordering is what matters and is preserved).

- [ ] **Step 3: Commit**

```bash
git add internal/infra/store/execution_store.go
git commit -m "perf: drop redundant task_id sort in ListByTaskIDs"
```

---

### Task A4: Delete narrative / WHAT-restating comments

**Files:**
- Modify: `internal/rpc/tools.go:28-29` (workerNameCache doc — trim to a single line on the function)
- Modify: `internal/rpc/tools.go:670` (drop "Stop processes before cancelling …" if it merely narrates the order; if it carries the WHY about workers picking up new work, keep it — quoted verbatim below)
- Modify: `internal/domain/command/task_format.go:23-25` (drop the WHAT-restating doc on `runningExecIDsForTasks`; the name says it)
- Modify: `internal/infra/store/task_store.go:54-55` (drop the one-liner above `appendCSVFilter`)

Keep the comment at `tools.go:670` if it reads as `// Stop processes before cancelling DB records so workers don't pick up new work after cancellation.` — that's a non-obvious ordering invariant. Confirm by reading the line; only delete if it has degenerated to pure narration.

- [ ] **Step 1: Apply the deletions**

Open each file and remove only the doc lines flagged above. Do not touch the bodies.

- [ ] **Step 2: Build**

```bash
go build ./...
```

Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add internal/rpc/tools.go internal/domain/command/task_format.go internal/infra/store/task_store.go
git commit -m "docs: remove narrative comments restating function bodies"
```

---

### Task A5: Clean up scaffolding in `execution_store_test.go:117-119`

**Files:**
- Modify: `internal/infra/store/execution_store_test.go:117-119` (the `_, _, _ = e1, e2, e3` line plus the unused `e1`/`e2` returns above it)

- [ ] **Step 1: Replace ignored Create returns with blank identifiers**

For each `e1, _ := s.Create(...)` / `e2, _ := s.Create(...)` where the variable is never asserted, switch the LHS to `_, _ = s.Create(...)`. Delete the `_, _, _ = e1, e2, e3` placeholder. Keep `e3` if it is actually asserted.

- [ ] **Step 2: Run tests**

```bash
go test ./internal/infra/store/... -run TestExecutionStore -v
```

Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add internal/infra/store/execution_store_test.go
git commit -m "test: drop unused Create returns and placeholder discards"
```

---

## Group B — Stringly-typed code → constants

### Task B1: Use `model.TaskType*` constants in `toolCreateTask`

**Files:**
- Modify: `internal/rpc/tools.go:339-343` and `:347-359` and `:362`

- [ ] **Step 1: Replace the type-validation switch**

Replace lines 339-343:

```go
	switch params.Type {
	case model.TaskTypeImmediate, model.TaskTypeCountdown, model.TaskTypeScheduled:
	default:
		return nil, fmt.Errorf("type must be %s, %s, or %s",
			model.TaskTypeImmediate, model.TaskTypeCountdown, model.TaskTypeScheduled)
	}
```

- [ ] **Step 2: Replace the per-type branch switch**

Replace lines 347-359:

```go
	switch params.Type {
	case model.TaskTypeCountdown:
		if params.ScheduledAt == nil {
			return nil, fmt.Errorf("scheduled_at is required for countdown tasks")
		}
		if *params.ScheduledAt < nowMS+5000 {
			return nil, fmt.Errorf("scheduled_at must be at least 5 seconds in the future")
		}
	case model.TaskTypeScheduled:
		if params.CronExpr == "" {
			return nil, fmt.Errorf("cron_expr is required for scheduled tasks")
		}
	}
```

- [ ] **Step 3: Replace the `if params.Type == "scheduled"` check at line 362**

```go
	if params.Type == model.TaskTypeScheduled {
```

- [ ] **Step 4: Sweep `tools.go` for any remaining literal `"pending"/"running"/"completed"/"failed"/"cancelled"` introduced in this branch (esp. around line 812-818) and switch to `model.TaskStatus*`. Do not touch unrelated pre-existing call sites unless they live in the diff for this branch.**

- [ ] **Step 5: Run tests**

```bash
go test ./internal/rpc/...
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/rpc/tools.go
git commit -m "refactor: use model.TaskType/TaskStatus constants in tools.go"
```

---

## Group C — Structural deduplication

### Task C1: Consolidate `ExecutionStore.Create` and `CreateBeeExecution`

**Files:**
- Modify: `internal/infra/store/execution_store.go:38-81`
- Audit & update callers: `internal/domain/worker/execution.go`, `internal/app/app.go`, `internal/domain/task/dispatcher.go`, any test fakes.

- [ ] **Step 1: Design the unified signature**

Introduce a single params struct and one internal insert:

```go
type ExecutionCreateParams struct {
	TaskID       string // empty for bee-owned execution
	WorkerID     string // empty for bee-owned execution
	SessionID    string
	TriggerInput string
	Engine       string
}

func (s *ExecutionStore) Create(params ExecutionCreateParams) (model.WorkerExecution, error) {
	millis := time.Now().UnixMilli()
	exec := model.WorkerExecution{
		ID:           uuid.New().String(),
		TaskID:       params.TaskID,
		SessionID:    params.SessionID,
		Engine:       params.Engine,
		TriggerInput: params.TriggerInput,
		Status:       model.ExecStatusPending,
		StartedAt:    &millis,
	}
	if params.WorkerID != "" {
		wid := params.WorkerID
		exec.WorkerID = &wid
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

Notes:
- The old `CreateBeeExecution` inserted `task_id = ''` and `worker_id IS NULL`. The unified version reproduces both: empty `TaskID` stays as empty string; empty `WorkerID` becomes a typed-nil `*string` so SQL gets `NULL`.
- Drop `CreateBeeExecution` entirely.

- [ ] **Step 2: Update every caller**

Find and update every call site:

```bash
go vet ./...
```

Expected after edits: each former `Create(workerID, taskID, triggerInput, sessionID, engine)` becomes `Create(ExecutionCreateParams{WorkerID: workerID, TaskID: taskID, ...})` and each former `CreateBeeExecution(sessionID, triggerInput, engine)` becomes `Create(ExecutionCreateParams{SessionID: sessionID, ...})`.

Likely callers (verify with grep before editing):
- `internal/domain/worker/execution.go`
- `internal/app/app.go`
- `internal/domain/task/dispatcher.go`
- tests under `internal/infra/store/`, `internal/domain/`, `internal/rpc/`

- [ ] **Step 3: Update test fakes**

If any fake/mock implements the old two-method interface, collapse it to the single method.

- [ ] **Step 4: Run the full suite**

```bash
go test ./...
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "refactor: unify ExecutionStore.Create and CreateBeeExecution"
```

---

### Task C2: Relocate `runningExecIDsForTasks` so `toolClearSession` can reuse it

**Files:**
- Create: `internal/infra/utils/execlookup.go` (or relocate inside an existing neutral package)
- Modify: `internal/domain/command/task_format.go:14-40` (remove the local helper + interface; re-export from utils, or import from the new location)
- Modify: `internal/rpc/tools.go:670-679` (use the shared helper)

**Decision point:** placing the helper in `internal/infra/utils` keeps the dependency direction clean (`domain/command` and `rpc` both depend on `infra/utils`). Keep the helper signature exactly the same.

- [ ] **Step 1: Create the new file with the helper and its interface**

```go
package utils

import (
	"context"

	"go.uber.org/zap"

	"github.com/theopenbee/openbee/internal/infra/model"
)

// RunningExecLookup resolves the running execution id for a set of tasks.
type RunningExecLookup interface {
	RunningExecIDsByTaskIDs(ctx context.Context, taskIDs []string) (map[string]string, error)
}

// RunningExecIDsForTasks resolves running exec ids for the given tasks. On
// lookup error it logs with the supplied op name and returns an empty map so
// callers can keep going without an exec id column.
func RunningExecIDsForTasks(ctx context.Context, logger *zap.Logger, lookup RunningExecLookup, tasks []model.Task, op string) map[string]string {
	if len(tasks) == 0 {
		return map[string]string{}
	}
	taskIDs := make([]string, 0, len(tasks))
	for _, t := range tasks {
		taskIDs = append(taskIDs, t.ID)
	}
	execIDs, err := lookup.RunningExecIDsByTaskIDs(ctx, taskIDs)
	if err != nil {
		logger.Error("resolve running exec ids", zap.String("op", op), zap.Error(err))
		return map[string]string{}
	}
	return execIDs
}
```

Logger is injected because `utils` should not depend on the package-level `log` defined in `domain/command` and `rpc`.

- [ ] **Step 2: Delete the local helper from `task_format.go` and update its callers**

Remove lines 14-40 of `internal/domain/command/task_format.go`. Where it was called (search the file), replace with `utils.RunningExecIDsForTasks(ctx, log, lookup, tasks, op)`.

- [ ] **Step 3: Replace the inline lookup in `toolClearSession`**

In `internal/rpc/tools.go:670-679` replace the explicit taskIDs / RunningExecIDsByTaskIDs / log block with:

```go
	execIDs := utils.RunningExecIDsForTasks(ctx, log, s.executionStore, tasksToStop, "clear_session")
```

Drop the now-unused local `taskIDs` slice.

- [ ] **Step 4: Run tests**

```bash
go test ./internal/domain/command/... ./internal/rpc/...
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "refactor: share RunningExecIDsForTasks helper across domain and rpc"
```

---

## Group D — Concurrency

### Task D1: Parallelize `CountTasks` + `List` in `toolListTasks`

**Files:**
- Modify: `internal/rpc/tools.go:447-454`

- [ ] **Step 1: Write a regression test that asserts both return paths still behave correctly**

This is a concurrency rewrite; correctness is preserved by `errgroup`-style fan-in. The existing `toolListTasks` tests cover the happy path and the error paths (count error vs list error). Confirm they still pass after the rewrite — add coverage only if a path is missing.

- [ ] **Step 2: Replace the sequential block with a parallel fan-in**

```go
	var (
		total int
		tasks []model.Task
	)
	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		t, err := s.taskStore.CountTasks(gctx, filter)
		if err != nil {
			return fmt.Errorf("count tasks: %w", err)
		}
		total = t
		return nil
	})
	g.Go(func() error {
		ts, err := s.taskStore.List(gctx, filter)
		if err != nil {
			return fmt.Errorf("list tasks: %w", err)
		}
		tasks = ts
		return nil
	})
	if err := g.Wait(); err != nil {
		return nil, err
	}
```

Add `"golang.org/x/sync/errgroup"` to the imports.

- [ ] **Step 3: Run tests**

```bash
go test ./internal/rpc/...
```

Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/rpc/tools.go
git commit -m "perf: parallelize CountTasks and List in toolListTasks"
```

---

## Group E — Flag UX

### Task E1: Use `cobra.Flag.Changed` for `--execution-limit`

**Files:**
- Modify: `cmd/openbee/ctl_task.go:19, 51-53, 105`

- [ ] **Step 1: Change the default to 0 and gate forwarding on `Flag().Changed`**

In the var block:

```go
	taskListExecutionLimit int
```

(Keep zero default.)

In the flag registration (`init`), change line 105:

```go
	ctlTaskListCmd.Flags().IntVar(&taskListExecutionLimit, "execution-limit", 0, "Executions per task (default: 10, max: 100; 0 = all for one matching task)")
```

In `RunE`, replace lines 51-53:

```go
		if cmd.Flags().Changed("execution-limit") {
			a["execution_limit"] = taskListExecutionLimit
		}
```

- [ ] **Step 2: Run the CLI manually to spot-check**

```bash
go build -o /tmp/openbee ./cmd/openbee
/tmp/openbee ctl task list --worker-id some-id --help
```

Expected: help renders cleanly; running without `--execution-limit` does not forward the flag (verify with a printout / debugger / test fake if convenient).

- [ ] **Step 3: Run tests**

```bash
go test ./cmd/...
```

Expected: PASS (or no tests; the change is small and CLI plumbing).

- [ ] **Step 4: Commit**

```bash
git add cmd/openbee/ctl_task.go
git commit -m "refactor: gate ctl task list --execution-limit on Flag.Changed"
```

---

## Final verification

- [ ] **Step 1: Full suite**

```bash
go build ./...
go test ./...
```

Expected: all PASS.

- [ ] **Step 2: Quick visual diff vs main**

```bash
git diff main..HEAD --stat
```

Confirm only the targeted files changed and no stray edits leaked in.

- [ ] **Step 3: Update CHANGELOG.md if a Group is user-visible**

Only Task D1 (perf) and Task E1 (flag UX) are arguably user-visible. Add a single line under the existing branch entry if appropriate. Per memory rule, changelog content must be in English.
