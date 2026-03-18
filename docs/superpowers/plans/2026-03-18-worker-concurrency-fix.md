# Worker Concurrency Fix Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Prevent the same worker from running multiple Claude Code processes concurrently by serializing all tasks for a given worker in a single queue, regardless of session origin.

**Architecture:** Change `queueKey` in `TaskDispatcher` from `sessionKey|workerID` to `workerID` only, and update `clearQueues` to filter pending tasks by session rather than deleting by key prefix. All other components are unchanged.

**Tech Stack:** Go, `internal/task_dispatcher` package, `sync/atomic` for test concurrency control.

---

## File Map

| File | Change |
|------|--------|
| `internal/task_dispatcher/dispatcher.go` | Modify `queueKey()` and `clearQueues()` |
| `internal/task_dispatcher/dispatcher_test.go` | Add 3 new tests; verify existing tests still pass |

---

### Task 1: Change `queueKey` to worker-only scope

**Files:**
- Modify: `internal/task_dispatcher/dispatcher.go:118-120`

- [ ] **Step 1: Write the failing test for `queueKey` isolation**

Add to `internal/task_dispatcher/dispatcher_test.go` (inside the `task_dispatcher_test` package, after the existing `--- Tests ---` section):

```go
func TestQueueKey_IgnoresSessionKey(t *testing.T) {
	// Same workerID, different sessionKeys must produce the same key.
	// This is the contract that prevents cross-session concurrent execution.
	k1 := task_dispatcher.ExportedQueueKey("session-a", "worker-1")
	k2 := task_dispatcher.ExportedQueueKey("session-b", "worker-1")
	if k1 != k2 {
		t.Errorf("expected same key for different sessions, got %q and %q", k1, k2)
	}
	if k1 != "worker-1" {
		t.Errorf("expected key to equal workerID, got %q", k1)
	}
}
```

> Note: `queueKey` is unexported. Export it temporarily for testing by adding to `dispatcher.go`:
> ```go
> // ExportedQueueKey is exported for testing only.
> var ExportedQueueKey = queueKey
> ```

- [ ] **Step 2: Run the test to confirm it fails**

```bash
cd /Users/tengyongzhi/work/theopenbee/openbee
go test ./internal/task_dispatcher/... -run TestQueueKey_IgnoresSessionKey -v
```

Expected: FAIL — the current `queueKey` returns `sessionKey + "|" + workerID`, so `k1 != k2`.

- [ ] **Step 3: Change `queueKey` in `dispatcher.go`**

In `internal/task_dispatcher/dispatcher.go`, replace:

```go
func queueKey(sessionKey, workerID string) string {
	return sessionKey + "|" + workerID
}
```

With:

```go
func queueKey(_, workerID string) string {
	return workerID
}
```

Also add the test export immediately after:

```go
// ExportedQueueKey is exported for testing only.
var ExportedQueueKey = queueKey
```

- [ ] **Step 4: Run the test to confirm it passes**

```bash
go test ./internal/task_dispatcher/... -run TestQueueKey_IgnoresSessionKey -v
```

Expected: PASS.

- [ ] **Step 5: Run the full dispatcher test suite**

```bash
go test ./internal/task_dispatcher/... -v
```

Expected: all existing tests pass. If `TestTaskDispatcher_ClearSession_ClearsQueueAndSessionContexts` fails, that is expected — it will be fixed in Task 2.

- [ ] **Step 6: Commit**

```bash
git add internal/task_dispatcher/dispatcher.go internal/task_dispatcher/dispatcher_test.go
git commit -m "fix: serialize task dispatch per workerID to prevent concurrent execution"
```

---

### Task 2: Update `clearQueues` to filter by session

**Files:**
- Modify: `internal/task_dispatcher/dispatcher.go:153-163`

The old `clearQueues` deleted map entries by `sessionKey|` prefix. With workerID-only keys, it must instead iterate all queues and remove pending tasks that belong to the given session.

- [ ] **Step 1: Write the failing test for cross-session queue filtering**

Add to `internal/task_dispatcher/dispatcher_test.go`:

```go
func TestTaskDispatcher_ClearSession_OnlyRemovesMatchingSession(t *testing.T) {
	ss := newMockSessionStore()
	blocker := make(chan struct{})
	mgr := &blockingExecManager{blocker: blocker}
	eq := &mockExecutionQuerier{result: model.WorkerExecution{ID: "exec-x", Status: model.ExecStatusCompleted}}

	in := make(chan task_dispatcher.DispatchTask, 8)
	d := task_dispatcher.New(mgr, &mockTaskStore{}, ss, eq, in)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go d.Run(ctx)

	// t1 from s1 starts executing (blocks)
	t1 := immediateTask("s1", "w1", "s1-first")
	t1.TaskID = "t1"
	in <- t1
	time.Sleep(50 * time.Millisecond) // wait for t1 to start blocking

	// t2 from s1 queued as pending
	t2 := immediateTask("s1", "w1", "s1-second")
	t2.TaskID = "t2"
	in <- t2

	// t3 from s2 (different session, same worker) queued as pending
	t3 := immediateTask("s2", "w1", "s2-task")
	t3.TaskID = "t3"
	in <- t3

	time.Sleep(30 * time.Millisecond) // let pending tasks register

	// Clear session s1 — should remove t2 but NOT t3
	d.ClearSession("s1")
	time.Sleep(50 * time.Millisecond)

	// Unblock t1
	close(blocker)

	// Wait for t3 to execute (s2's task should still run)
	deadline2 := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline2) {
		if atomic.LoadInt64(&mgr.completed) >= 2 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if atomic.LoadInt64(&mgr.completed) < 2 {
		t.Fatalf("expected 2 executions (t1 + t3), got %d", atomic.LoadInt64(&mgr.completed))
	}

	// t2 from s1 must NOT have executed
	if atomic.LoadInt64(&mgr.started) > 2 {
		t.Errorf("expected at most 2 executions started (t2 should be cleared), got %d", atomic.LoadInt64(&mgr.started))
	}

	// Session contexts for s1 must have been cleared
	ss.mu.Lock()
	defer ss.mu.Unlock()
	if len(ss.cleared) == 0 || ss.cleared[0] != "s1" {
		t.Errorf("expected ClearSessionContexts called with s1, got %v", ss.cleared)
	}
}
```

- [ ] **Step 2: Run the test to confirm it fails**

```bash
go test ./internal/task_dispatcher/... -run TestTaskDispatcher_ClearSession_OnlyRemovesMatchingSession -v
```

Expected: FAIL — the current `clearQueues` deletes by prefix, which can't match workerID-only keys properly.

- [ ] **Step 3: Replace `clearQueues` in `dispatcher.go`**

Replace the existing `clearQueues` method body:

```go
func (d *TaskDispatcher) clearQueues(sessionKey string) {
	prefix := sessionKey + "|"
	for key := range d.queues {
		if strings.HasPrefix(key, prefix) {
			delete(d.queues, key)
		}
	}
	if err := d.sessionStore.ClearSessionContexts(d.ctx, sessionKey); err != nil {
		slog.Error("clear session contexts", "component", "taskdispatcher", "sessionKey", sessionKey, "error", err)
	}
}
```

With:

```go
func (d *TaskDispatcher) clearQueues(sessionKey string) {
	for key, state := range d.queues {
		var remaining []DispatchTask
		for _, t := range state.pendingTasks {
			if t.SessionKey != sessionKey {
				remaining = append(remaining, t)
			}
		}
		state.pendingTasks = remaining
		if !state.executing && len(state.pendingTasks) == 0 {
			delete(d.queues, key)
		}
	}
	if err := d.sessionStore.ClearSessionContexts(d.ctx, sessionKey); err != nil {
		slog.Error("clear session contexts", "component", "taskdispatcher", "sessionKey", sessionKey, "error", err)
	}
}
```

Also update the stale comment on the `queues` field in the `TaskDispatcher` struct (currently says `// 按 sessionKey|workerID 分组的串行队列`) to `// 按 workerID 分组的串行队列`.

Then remove the now-unused `"strings"` import if `strings.HasPrefix` was the only usage. Check whether `strings` is used elsewhere in the file first:

```bash
grep -n '"strings"' internal/task_dispatcher/dispatcher.go
grep -n 'strings\.' internal/task_dispatcher/dispatcher.go
```

If `strings` is only used in `clearQueues`, remove it from the import block.

- [ ] **Step 4: Run all dispatcher tests**

```bash
go test ./internal/task_dispatcher/... -v
```

Expected: all tests pass, including the new one and the previously-failing `TestTaskDispatcher_ClearSession_ClearsQueueAndSessionContexts` (that test uses same-session tasks for the same worker, which still works with the new logic).

- [ ] **Step 5: Commit**

```bash
git add internal/task_dispatcher/dispatcher.go internal/task_dispatcher/dispatcher_test.go
git commit -m "fix: update clearQueues to filter pending tasks by session instead of key prefix"
```

---

### Task 3: Add regression test for cross-session serial execution

This is the core regression test that proves the bug is fixed: two different sessions dispatching to the same worker must not run concurrently.

**Files:**
- Modify: `internal/task_dispatcher/dispatcher_test.go`

- [ ] **Step 1: Add the regression test**

Add to `internal/task_dispatcher/dispatcher_test.go`:

```go
func TestTaskDispatcher_CrossSession_SameWorker_Serialized(t *testing.T) {
	// Two different sessions both dispatch to the same worker.
	// They must execute sequentially — never concurrently.
	blocker := make(chan struct{})
	mgr := &blockingExecManager{blocker: blocker}
	eq := &mockExecutionQuerier{result: model.WorkerExecution{ID: "exec-x", Status: model.ExecStatusCompleted}}
	d, in, _ := newTaskDispatcher(mgr, eq, newMockSessionStore())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go d.Run(ctx)

	// Session s1 dispatches to worker w1
	t1 := immediateTask("s1", "w1", "from-s1")
	t1.TaskID = "task-s1"
	// Session s2 also dispatches to worker w1
	t2 := immediateTask("s2", "w1", "from-s2")
	t2.TaskID = "task-s2"

	in <- t1
	in <- t2

	// Wait for first task to start
	time.Sleep(50 * time.Millisecond)

	// Only one execution should have started — the second must be queued
	if atomic.LoadInt64(&mgr.started) != 1 {
		t.Fatalf("expected exactly 1 execution started (second should be queued), got %d", atomic.LoadInt64(&mgr.started))
	}

	// Unblock the first execution
	close(blocker)

	// Both should eventually complete
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadInt64(&mgr.completed) >= 2 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if atomic.LoadInt64(&mgr.completed) < 2 {
		t.Errorf("expected both tasks to complete, only %d completed", atomic.LoadInt64(&mgr.completed))
	}
}
```

- [ ] **Step 2: Run the regression test**

```bash
go test ./internal/task_dispatcher/... -run TestTaskDispatcher_CrossSession_SameWorker_Serialized -v
```

Expected: PASS (the fix from Tasks 1 and 2 makes this work).

- [ ] **Step 3: Run the full test suite**

```bash
go test ./internal/task_dispatcher/... -v -race
```

The `-race` flag is important — it catches data races that timing-based tests can miss. Expected: all tests pass, no race conditions reported.

- [ ] **Step 4: Run the broader test suite**

```bash
go test ./... -count=1
```

Expected: all packages pass.

- [ ] **Step 5: Commit**

```bash
git add internal/task_dispatcher/dispatcher_test.go
git commit -m "test: add regression test for cross-session worker serialization"
```
