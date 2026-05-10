# /clear Confirmation Prompt — Task Details Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** When `/clear` (global form) needs confirmation due to running tasks, show each running task with its owning worker, instruction, runtime and exec ID — same format as `/status` — instead of just a count.

**Architecture:** Refactor the per-task formatting helpers from `status.go` into a shared `task_format.go` so both `/status` and `/clear` use one source of truth. Restructure the `clear_command.confirm_all_with_tasks` i18n template into composable parts (header / agent_line / tasks_header / footer) that the handler assembles programmatically. Add a `now` clock seam to `ClearCommandHandler` and extend its worker lookup interface with `GetByIDs` so it can resolve `task.WorkerID` to display names.

**Tech Stack:** Go, YAML i18n, table-driven tests under `internal/domain/command`.

**Spec:** `docs/superpowers/specs/2026-05-10-clear-confirm-task-details-design.md`

---

## File Map

- **Create:** `internal/domain/command/task_format.go` — shared `WorkerByIDsLookup` interface + `resolveWorkerNames` free function
- **Create:** `internal/domain/command/clear_test.go` — tests for `/clear` (none exist today)
- **Create:** `internal/domain/command/clear_export_test.go` — clock injection seam
- **Modify:** `internal/domain/command/status.go` — drop the duplicate `resolveWorkerNames` method, call shared free function
- **Modify:** `internal/domain/command/clear.go` — widen `WorkerNameLookup` to include `GetByIDs`, add `now` field, rewrite confirmation branch to multi-line
- **Modify:** `internal/infra/i18n/messages.go` — replace `ConfirmAllWithTasks` with 4 composable fields
- **Modify:** `internal/infra/i18n/locales/en.yaml` — add new fields, drop old field
- **Modify:** `internal/infra/i18n/locales/zh.yaml` — add new fields, drop old field

Each commit must leave the build green and all existing tests passing.

---

## Task 1: Extract `resolveWorkerNames` to a shared helper

**Files:**
- Create: `internal/domain/command/task_format.go`
- Modify: `internal/domain/command/status.go`
- Test: `internal/domain/command/status_test.go` (existing, must keep passing)

This is a pure refactor — same behaviour, no signature change visible to callers.

- [ ] **Step 1: Create `task_format.go` with the shared interface and helper**

Write file `internal/domain/command/task_format.go`:

```go
package command

import (
	"go.uber.org/zap"

	"github.com/theopenbee/openbee/internal/infra/model"
)

// WorkerByIDsLookup is the minimal worker-store surface needed to resolve
// task.WorkerID values to display names. Shared by /status and /clear.
type WorkerByIDsLookup interface {
	GetByIDs(ids []string) ([]model.Worker, error)
}

// resolveWorkerNames returns a {workerID -> name} map for the workers
// referenced by tasks. Returns nil on lookup error so callers fall back to
// raw IDs via workerNameOrFallback.
func resolveWorkerNames(workers WorkerByIDsLookup, tasks []model.Task) map[string]string {
	if len(tasks) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(tasks))
	ids := make([]string, 0, len(tasks))
	for _, t := range tasks {
		if t.WorkerID == "" {
			continue
		}
		if _, ok := seen[t.WorkerID]; ok {
			continue
		}
		seen[t.WorkerID] = struct{}{}
		ids = append(ids, t.WorkerID)
	}
	if len(ids) == 0 {
		return nil
	}
	ws, err := workers.GetByIDs(ids)
	if err != nil {
		log.Error("batch lookup workers", zap.Error(err))
		return nil
	}
	out := make(map[string]string, len(ws))
	for _, w := range ws {
		if w.Name != "" {
			out[w.ID] = w.Name
		}
	}
	return out
}
```

- [ ] **Step 2: Drop the duplicate method from `status.go`**

In `internal/domain/command/status.go`, delete the existing method (lines 142–174):

```go
// On error returns nil so the caller falls back to raw IDs.
func (h *StatusCommandHandler) resolveWorkerNames(tasks []model.Task) map[string]string {
	... (entire method)
}
```

In `formatStatus` (around line 105), change:

```go
workerNames := h.resolveWorkerNames(tasks)
```

to:

```go
workerNames := resolveWorkerNames(h.workers, tasks)
```

The `StatusWorkerLookup` interface in `status.go` (line 34) already declares `GetByIDs`, so it satisfies `WorkerByIDsLookup`. No interface change needed.

If the `zap` import becomes unused after the deletion, remove it from `status.go` imports.

- [ ] **Step 3: Verify status tests still pass**

Run: `go test ./internal/domain/command/... -run TestStatusCommand -v`

Expected: all `TestStatusCommand_*` tests PASS.

- [ ] **Step 4: Verify the whole package builds**

Run: `go build ./...`

Expected: no compile errors.

- [ ] **Step 5: Commit**

```bash
git add internal/domain/command/task_format.go internal/domain/command/status.go
git commit -m "refactor(command): extract resolveWorkerNames to shared helper"
```

---

## Task 2: Add new i18n fields (keep old field for now)

**Files:**
- Modify: `internal/infra/i18n/messages.go`
- Modify: `internal/infra/i18n/locales/en.yaml`
- Modify: `internal/infra/i18n/locales/zh.yaml`

We add new fields alongside `ConfirmAllWithTasks` so the build stays green. Task 6 deletes the old field after the new code is wired up and tested.

- [ ] **Step 1: Add fields to the Go struct**

In `internal/infra/i18n/messages.go`, replace the `ClearCommandMessages` struct (lines 314–325):

```go
// ClearCommandMessages holds text sent to IM users by the /clear command handler.
type ClearCommandMessages struct {
	Usage               string `yaml:"usage"`
	WorkerNotFound      string `yaml:"worker_not_found"`        // contains %s (worker name)
	WorkerDuplicate     string `yaml:"worker_duplicate"`        // contains %s, %s (name, id list)
	NoContext           string `yaml:"no_context"`
	LookupFailed        string `yaml:"lookup_failed"`
	ConfirmAllWithTasks string `yaml:"confirm_all_with_tasks"`  // DEPRECATED: removed in follow-up commit
	ConfirmHeader       string `yaml:"confirm_header"`
	ConfirmAgentLine    string `yaml:"confirm_agent_line"`      // contains %s, %s (name, engine)
	ConfirmTasksHeader  string `yaml:"confirm_tasks_header"`    // contains %d (task count)
	ConfirmFooter       string `yaml:"confirm_footer"`
	Cleared             string `yaml:"cleared"`                 // contains %s (agent/engine list)
	ClearedWithTasks    string `yaml:"cleared_with_tasks"`      // contains %s, %d (list, cancelled count)
	WorkerCleared       string `yaml:"worker_cleared"`          // contains %s, %s (worker name, engine)
}
```

- [ ] **Step 2: Add fields to en.yaml**

In `internal/infra/i18n/locales/en.yaml`, locate the `clear_command:` block (line 250). Replace the whole block with:

```yaml
  clear_command:
    usage: "Usage:\n/clear — clear all session contexts for this session\n/clear {workerName} — clear session context for a specific worker"
    worker_not_found: "Worker %q not found"
    worker_duplicate: "Multiple workers named %q found:\n%s"
    no_context: "No session contexts to clear."
    lookup_failed: "⚠️ Failed to look up session contexts. Please retry; if this persists, check server logs."
    confirm_all_with_tasks: "⚠️ Will clear session contexts: %s\nThis will also stop %d running task(s).\nSend /clear again within 30s to confirm."
    confirm_header: "⚠️ Will clear session contexts:"
    confirm_agent_line: "  - %s (%s)"
    confirm_tasks_header: "This will also stop %d running task(s):"
    confirm_footer: "Send /clear again within 30s to confirm."
    cleared: "✅ Cleared: %s"
    cleared_with_tasks: "✅ Cleared: %s. Cancelled %d task(s)."
    worker_cleared: "✅ Cleared %s (engine: %s)."
```

- [ ] **Step 3: Add fields to zh.yaml**

In `internal/infra/i18n/locales/zh.yaml`, replace the `clear_command:` block (line 250) with:

```yaml
  clear_command:
    usage: "用法：\n/clear — 清除当前会话的全部上下文\n/clear {workerName} — 清除指定 worker 的上下文"
    worker_not_found: "Worker %q 不存在"
    worker_duplicate: "存在多个同名 Worker %q：\n%s"
    no_context: "没有可清除的会话"
    lookup_failed: "⚠️ 查询会话上下文失败，请稍后重试；若持续出现请检查服务端日志。"
    confirm_all_with_tasks: "⚠️ 将清除以下会话上下文：%s\n同时将终止 %d 个运行中任务。\n30s 内再发一次 /clear 确认。"
    confirm_header: "⚠️ 将清除以下会话上下文："
    confirm_agent_line: "  - %s（%s）"
    confirm_tasks_header: "同时将终止 %d 个运行中任务："
    confirm_footer: "30s 内再发一次 /clear 确认。"
    cleared: "✅ 已清除：%s"
    cleared_with_tasks: "✅ 已清除：%s，取消了 %d 个任务。"
    worker_cleared: "✅ 已清除 %s（engine：%s）的会话上下文。"
```

- [ ] **Step 4: Verify i18n loading still works**

Run: `go test ./internal/infra/i18n/... -v`

Expected: all i18n tests PASS.

- [ ] **Step 5: Verify the whole project builds**

Run: `go build ./...`

Expected: no compile errors.

- [ ] **Step 6: Commit**

```bash
git add internal/infra/i18n/messages.go internal/infra/i18n/locales/en.yaml internal/infra/i18n/locales/zh.yaml
git commit -m "feat(i18n): add composable clear_command confirm fields"
```

---

## Task 3: Add clock seam to `ClearCommandHandler` (test export)

**Files:**
- Modify: `internal/domain/command/clear.go`
- Create: `internal/domain/command/clear_export_test.go`

This isolates the test seam in its own commit so the next task is purely about behavior.

- [ ] **Step 1: Add `now` field to handler**

In `internal/domain/command/clear.go`, modify the struct (lines 43–54):

```go
type ClearCommandHandler struct {
	workers      WorkerNameLookup
	sessions     ClearSessionStore
	tasks        ClearTaskStore
	execStopper  ClearExecStopper
	sessionClear ClearSessionDispatcher
	senders      map[string]platform.PlatformSenderAdapter
	engineCfg    *enginecfg.Store

	now func() time.Time

	mu      sync.Mutex
	pending map[string]time.Time // key: sessionKey + "::" + normalized command → expiry
}
```

In `NewClearCommandHandler` (lines 56–75), initialize `now: time.Now`:

```go
return &ClearCommandHandler{
	workers:      workers,
	sessions:     sessions,
	tasks:        tasks,
	execStopper:  execStopper,
	sessionClear: sessionClear,
	senders:      senders,
	engineCfg:    engineCfg,
	now:          time.Now,
	pending:      make(map[string]time.Time),
}
```

- [ ] **Step 2: Create the export test seam**

Write file `internal/domain/command/clear_export_test.go`:

```go
package command

import "time"

// SetClearClockForTest overrides the time source used by /clear. Available
// to external _test packages so production callers cannot mutate the clock.
func SetClearClockForTest(h *ClearCommandHandler, now func() time.Time) {
	h.now = now
}
```

- [ ] **Step 3: Verify build**

Run: `go build ./...`

Expected: no compile errors.

- [ ] **Step 4: Verify existing tests still pass**

Run: `go test ./internal/domain/command/...`

Expected: all PASS (no behavior change).

- [ ] **Step 5: Commit**

```bash
git add internal/domain/command/clear.go internal/domain/command/clear_export_test.go
git commit -m "refactor(clear): inject clock for testability"
```

---

## Task 4: Write failing test for the new multi-line confirmation prompt

**Files:**
- Create: `internal/domain/command/clear_test.go`

Establish the test setup first so following tasks have a place to add cases. The headline test covers the new multi-line format.

Tests run with the `zh` locale (loaded by `engine_test.go`'s `TestMain`), so expected strings are Chinese.

- [ ] **Step 1: Write `clear_test.go` with the headline failing test**

Write file `internal/domain/command/clear_test.go`:

```go
package command_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/theopenbee/openbee/internal/domain/command"
	"github.com/theopenbee/openbee/internal/domain/enginecfg"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/infra/store"
	"github.com/theopenbee/openbee/internal/platform"
)

// --- fakes for /clear ---

type fakeClearSessionStore struct {
	agents          []store.SessionAgent
	listErr         error
	deleted         bool
	deleteErr       error
	deletedSessions []string // captured calls for assertion
}

func (f *fakeClearSessionStore) ListActiveSessionContexts(_ context.Context, _, _ string) ([]store.SessionAgent, error) {
	return f.agents, f.listErr
}

func (f *fakeClearSessionStore) DeleteSessionContextForEngine(_ context.Context, sessionKey, _, _ string) (bool, error) {
	f.deletedSessions = append(f.deletedSessions, sessionKey)
	return f.deleted, f.deleteErr
}

type fakeClearTaskStore struct {
	tasks         []model.Task
	listErr       error
	cancelled     int64
	cancelErr     error
}

func (f *fakeClearTaskStore) ListBySessionKey(_ context.Context, _, _, _ string) ([]model.Task, error) {
	return f.tasks, f.listErr
}

func (f *fakeClearTaskStore) CancelBySessionKey(_ context.Context, _, _ string) (int64, error) {
	return f.cancelled, f.cancelErr
}

type fakeClearWorkerLookup struct {
	byName map[string][]model.Worker
	byID   map[string]model.Worker
	err    error
}

func (f *fakeClearWorkerLookup) ListByName(name string) ([]model.Worker, error) {
	return f.byName[name], f.err
}

func (f *fakeClearWorkerLookup) GetByIDs(ids []string) ([]model.Worker, error) {
	if f.err != nil {
		return nil, f.err
	}
	out := make([]model.Worker, 0, len(ids))
	for _, id := range ids {
		if w, ok := f.byID[id]; ok {
			out = append(out, w)
		}
	}
	return out, nil
}

type fakeClearExecStopper struct {
	stopped []string
}

func (f *fakeClearExecStopper) StopExecution(executionID string) error {
	f.stopped = append(f.stopped, executionID)
	return nil
}

type fakeClearSessionDispatcher struct {
	cleared []string
}

func (f *fakeClearSessionDispatcher) ClearSession(sessionKey string) {
	f.cleared = append(f.cleared, sessionKey)
}

type clearFixture struct {
	handler  *command.ClearCommandHandler
	sender   *fakeSender
	sessions *fakeClearSessionStore
	tasks    *fakeClearTaskStore
	stopper  *fakeClearExecStopper
	disp     *fakeClearSessionDispatcher
}

func makeClearFixture(
	agents []store.SessionAgent,
	tasks []model.Task,
	workersByID map[string]model.Worker,
	clock time.Time,
) *clearFixture {
	sender := &fakeSender{}
	senders := map[string]platform.PlatformSenderAdapter{"feishu": sender}
	sessions := &fakeClearSessionStore{agents: agents, deleted: true}
	taskStore := &fakeClearTaskStore{tasks: tasks}
	workers := &fakeClearWorkerLookup{byID: workersByID}
	stopper := &fakeClearExecStopper{}
	disp := &fakeClearSessionDispatcher{}
	engineCfg := enginecfg.NewStore("claude")
	h := command.NewClearCommandHandler(workers, sessions, taskStore, stopper, disp, senders, engineCfg)
	command.SetClearClockForTest(h, func() time.Time { return clock })
	return &clearFixture{
		handler:  h,
		sender:   sender,
		sessions: sessions,
		tasks:    taskStore,
		stopper:  stopper,
		disp:     disp,
	}
}

func TestClearCommand_ConfirmPromptListsTasksAndAgents(t *testing.T) {
	clock := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	nowMs := clock.UnixMilli()
	agents := []store.SessionAgent{
		{AgentID: "w1", AgentType: "worker", Engine: "claude", Name: "关羽", UpdatedAt: nowMs - 30*1000},
		{AgentID: "w2", AgentType: "worker", Engine: "claude", Name: "马超", UpdatedAt: nowMs - 30*1000},
		{AgentID: "bee", AgentType: "bee", Engine: "claude", Name: "bee", UpdatedAt: nowMs - 30*1000},
	}
	tasks := []model.Task{
		{ID: "t1", WorkerID: "w1", Instruction: "帮我写一个排序算法", ExecutionID: "a1b2c3d4e5f6", CreatedAt: nowMs - 180*1000, Status: model.TaskStatusRunning, Type: model.TaskTypeImmediate},
		{ID: "t2", WorkerID: "bee", Instruction: "总结今天的会议纪要", ExecutionID: "e5f6a7b89999", CreatedAt: nowMs - 12*1000, Status: model.TaskStatusRunning, Type: model.TaskTypeImmediate},
	}
	workers := map[string]model.Worker{
		"w1":  {ID: "w1", Name: "关羽"},
		"bee": {ID: "bee", Name: "bee"},
	}
	fx := makeClearFixture(agents, tasks, workers, clock)

	handled := fx.handler.HandleCommand(context.Background(), "/clear", makeReplyTo())
	if !handled {
		t.Fatal("expected handled=true")
	}
	if len(fx.sender.sent) != 1 {
		t.Fatalf("expected 1 reply, got %d", len(fx.sender.sent))
	}
	out := fx.sender.sent[0]

	for _, want := range []string{
		"⚠️ 将清除以下会话上下文：",
		"  - 关羽（claude）",
		"  - 马超（claude）",
		"  - bee（claude）",
		"同时将终止 2 个运行中任务：",
		"- [关羽] 帮我写一个排序算法   已运行 3m   exec: a1b2c3d4",
		"- [bee] 总结今天的会议纪要   已运行 12s   exec: e5f6a7b8",
		"30s 内再发一次 /clear 确认。",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\n--- output ---\n%s", want, out)
		}
	}

	// Pending must be set; tasks must NOT have been stopped or cancelled yet.
	if len(fx.stopper.stopped) != 0 {
		t.Errorf("expected no executions stopped on first /clear, got %v", fx.stopper.stopped)
	}
	if len(fx.disp.cleared) != 0 {
		t.Errorf("expected no session cleared on first /clear, got %v", fx.disp.cleared)
	}
}
```

- [ ] **Step 2: Run the test and verify it fails**

Run: `go test ./internal/domain/command/... -run TestClearCommand_ConfirmPromptListsTasksAndAgents -v`

Expected: FAIL because current `clear.go` produces the old single-line `confirm_all_with_tasks` template, not the new multi-line layout.

The failure message will look like missing strings such as `"  - 关羽（claude）"` and `"- [关羽] 帮我写一个排序算法   已运行 3m   exec: a1b2c3d4"`.

- [ ] **Step 3: Commit (failing test on its own commit so the change driving it is visible)**

```bash
git add internal/domain/command/clear_test.go
git commit -m "test(clear): add failing test for multi-line confirm prompt"
```

---

## Task 5: Make the failing test pass — multi-line confirmation in `clear.go`

**Files:**
- Modify: `internal/domain/command/clear.go`

- [ ] **Step 1: Widen the worker-lookup interface**

In `internal/domain/command/clear.go`, replace the `WorkerNameLookup` interface (lines 21–23):

```go
type WorkerNameLookup interface {
	ListByName(name string) ([]model.Worker, error)
	GetByIDs(ids []string) ([]model.Worker, error)
}
```

The production implementation `*store.WorkerStore` already has both methods (verified in `internal/infra/store/worker_store.go:61` and `:121`), and the constructor wiring in `internal/app/app.go:168` passes `s.workerStore` — no app-level change needed.

- [ ] **Step 2: Import `utils` and `model` (already imported), confirm imports**

The file already imports `model` and `i18n`. Add these imports if missing — open the import block and ensure these are present:

```go
import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/theopenbee/openbee/internal/domain/enginecfg"
	"github.com/theopenbee/openbee/internal/infra/i18n"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/infra/store"
	"github.com/theopenbee/openbee/internal/infra/utils"
	"github.com/theopenbee/openbee/internal/platform"
)
```

- [ ] **Step 3: Replace the confirmation branch in `handleClearAll`**

In `internal/domain/command/clear.go`, replace lines 122–127:

```go
	if len(runningTasks) > 0 && !confirmed {
		list := formatAgentList(agents)
		h.storePending(pendingKey)
		h.reply(ctx, replyTo, fmt.Sprintf(m.ConfirmAllWithTasks, list, len(runningTasks)))
		return
	}
```

with:

```go
	if len(runningTasks) > 0 && !confirmed {
		h.storePending(pendingKey)
		h.reply(ctx, replyTo, h.formatConfirmPrompt(agents, runningTasks))
		return
	}
```

- [ ] **Step 4: Add `formatConfirmPrompt` method**

Add this method to `clear.go` (place it just below `handleClearAll`, before `handleClearWorker`):

```go
func (h *ClearCommandHandler) formatConfirmPrompt(agents []store.SessionAgent, tasks []model.Task) string {
	m := i18n.M.Runtime.ClearCommand
	statusM := i18n.M.Runtime.StatusCommand
	nowMs := h.now().UnixMilli()
	workerNames := resolveWorkerNames(h.workers, tasks)

	lines := make([]string, 0, 1+len(agents)+1+1+len(tasks)+1+1)
	lines = append(lines, m.ConfirmHeader)
	for _, a := range agents {
		lines = append(lines, fmt.Sprintf(m.ConfirmAgentLine, a.Name, a.Engine))
	}
	lines = append(lines, "")
	lines = append(lines, fmt.Sprintf(m.ConfirmTasksHeader, len(tasks)))
	for _, t := range tasks {
		runtimeSec := (nowMs - t.CreatedAt) / 1000
		lines = append(lines, fmt.Sprintf(statusM.TaskLine,
			workerNameOrFallback(workerNames, t.WorkerID),
			utils.TruncateRunes(strings.Join(strings.Fields(t.Instruction), " "), maxInstructionRunes),
			formatRelative(runtimeSec),
			shortExecID(t.ExecutionID),
		))
	}
	lines = append(lines, "")
	lines = append(lines, m.ConfirmFooter)
	return strings.Join(lines, "\n")
}
```

`maxInstructionRunes`, `formatRelative`, `shortExecID`, and `workerNameOrFallback` are already defined in the same `command` package (`status.go`), so no new declarations are needed.

`resolveWorkerNames` is the free function you created in Task 1 (`task_format.go`). It takes a `WorkerByIDsLookup`, and `WorkerNameLookup` now embeds that contract via its own `GetByIDs` method — so passing `h.workers` works (Go's structural typing).

- [ ] **Step 5: Run the new test — it must pass now**

Run: `go test ./internal/domain/command/... -run TestClearCommand_ConfirmPromptListsTasksAndAgents -v`

Expected: PASS.

- [ ] **Step 6: Run all tests in the command package**

Run: `go test ./internal/domain/command/...`

Expected: all PASS (no regressions in `/status`, `/engine`, `/stop`).

- [ ] **Step 7: Run the full test suite**

Run: `go test ./...`

Expected: all PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/domain/command/clear.go
git commit -m "feat(clear): list running tasks and workers in confirm prompt"
```

---

## Task 6: Cover regressions and the worker-fallback path

**Files:**
- Modify: `internal/domain/command/clear_test.go`

Add two more cases to lock in the existing confirmation flow and the raw-ID fallback.

- [ ] **Step 1: Add the "confirm within 30s clears tasks" test**

Append to `internal/domain/command/clear_test.go`:

```go
func TestClearCommand_ConfirmedWithin30sStopsAndClears(t *testing.T) {
	clock := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	nowMs := clock.UnixMilli()
	agents := []store.SessionAgent{
		{AgentID: "w1", AgentType: "worker", Engine: "claude", Name: "关羽", UpdatedAt: nowMs - 1000},
	}
	tasks := []model.Task{
		{ID: "t1", WorkerID: "w1", Instruction: "do work", ExecutionID: "exec-1234abcd", CreatedAt: nowMs - 5000, Status: model.TaskStatusRunning, Type: model.TaskTypeImmediate},
	}
	workers := map[string]model.Worker{"w1": {ID: "w1", Name: "关羽"}}
	fx := makeClearFixture(agents, tasks, workers, clock)
	fx.tasks.cancelled = 1

	// First /clear -> confirmation prompt.
	fx.handler.HandleCommand(context.Background(), "/clear", makeReplyTo())
	if len(fx.sender.sent) != 1 {
		t.Fatalf("expected 1 reply after first /clear, got %d", len(fx.sender.sent))
	}

	// Second /clear within 30s -> actually stop & clear.
	fx.handler.HandleCommand(context.Background(), "/clear", makeReplyTo())
	if len(fx.sender.sent) != 2 {
		t.Fatalf("expected 2 replies total, got %d", len(fx.sender.sent))
	}
	if got, want := fx.stopper.stopped, []string{"exec-1234abcd"}; len(got) != 1 || got[0] != want[0] {
		t.Errorf("expected stopped=%v, got %v", want, got)
	}
	if len(fx.disp.cleared) != 1 {
		t.Errorf("expected ClearSession called once, got %d", len(fx.disp.cleared))
	}
	if !strings.Contains(fx.sender.sent[1], "✅ 已清除：") {
		t.Errorf("expected cleared message, got: %s", fx.sender.sent[1])
	}
}
```

- [ ] **Step 2: Add the worker-name-fallback test**

Append:

```go
func TestClearCommand_ConfirmPromptFallsBackToWorkerID(t *testing.T) {
	clock := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	nowMs := clock.UnixMilli()
	agents := []store.SessionAgent{
		{AgentID: "ghost", AgentType: "worker", Engine: "claude", Name: "ghost", UpdatedAt: nowMs - 1000},
	}
	tasks := []model.Task{
		{ID: "t1", WorkerID: "ghost", Instruction: "do something", ExecutionID: "deadbeef0000", CreatedAt: nowMs - 5000, Status: model.TaskStatusRunning, Type: model.TaskTypeImmediate},
	}
	// workers map deliberately empty -> GetByIDs returns no entries.
	fx := makeClearFixture(agents, tasks, nil, clock)

	fx.handler.HandleCommand(context.Background(), "/clear", makeReplyTo())
	if len(fx.sender.sent) != 1 {
		t.Fatalf("expected 1 reply, got %d", len(fx.sender.sent))
	}
	out := fx.sender.sent[0]
	if !strings.Contains(out, "[ghost] do something") {
		t.Errorf("expected raw worker id fallback in task line, got:\n%s", out)
	}
}
```

- [ ] **Step 3: Run the new tests**

Run: `go test ./internal/domain/command/... -run TestClearCommand -v`

Expected: all three `TestClearCommand_*` tests PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/domain/command/clear_test.go
git commit -m "test(clear): cover confirmed flow and worker-id fallback"
```

---

## Task 7: Remove the deprecated `ConfirmAllWithTasks` field

**Files:**
- Modify: `internal/infra/i18n/messages.go`
- Modify: `internal/infra/i18n/locales/en.yaml`
- Modify: `internal/infra/i18n/locales/zh.yaml`

Now that nothing references the old field, drop it.

- [ ] **Step 1: Verify there are no remaining references**

Run: `git grep -n "ConfirmAllWithTasks\|confirm_all_with_tasks"`

Expected: hits only inside the three i18n files (struct field + two yaml lines). If anything else shows up, stop and investigate before deleting.

- [ ] **Step 2: Delete the Go field**

In `internal/infra/i18n/messages.go`, remove this line from `ClearCommandMessages`:

```go
	ConfirmAllWithTasks string `yaml:"confirm_all_with_tasks"`  // DEPRECATED: removed in follow-up commit
```

- [ ] **Step 3: Delete the en.yaml line**

In `internal/infra/i18n/locales/en.yaml`, delete this line under `clear_command:`:

```yaml
    confirm_all_with_tasks: "⚠️ Will clear session contexts: %s\nThis will also stop %d running task(s).\nSend /clear again within 30s to confirm."
```

- [ ] **Step 4: Delete the zh.yaml line**

In `internal/infra/i18n/locales/zh.yaml`, delete this line under `clear_command:`:

```yaml
    confirm_all_with_tasks: "⚠️ 将清除以下会话上下文：%s\n同时将终止 %d 个运行中任务。\n30s 内再发一次 /clear 确认。"
```

- [ ] **Step 5: Verify build and tests still pass**

Run: `go build ./... && go test ./...`

Expected: all PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/infra/i18n/messages.go internal/infra/i18n/locales/en.yaml internal/infra/i18n/locales/zh.yaml
git commit -m "chore(i18n): drop deprecated clear_command.confirm_all_with_tasks"
```

---

## Done — Final Verification

- [ ] **Step 1: Run the full suite once more**

Run: `go build ./... && go test ./...`

Expected: PASS.

- [ ] **Step 2: Eyeball a manual run (optional, but nice)**

If the user wants visual confirmation, point them at the new test:

```bash
go test ./internal/domain/command/... -run TestClearCommand_ConfirmPromptListsTasksAndAgents -v
```

The test output prints the full sender reply on failure — running with `-v` and forcing a failure (e.g. break the format briefly) is a quick way to see the rendered prompt.
