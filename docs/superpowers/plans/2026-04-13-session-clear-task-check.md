# Session Clear Running-Task Detection Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Move running-task detection from the Bee pre-query step into `clear_session`, so the backend returns `requires_confirmation` when active tasks exist, eliminating an unnecessary round-trip in the common case.

**Architecture:** Insert a task-detection check at the top of the `if !params.Force` block in `toolClearSession()`, before the existing multi-worker check. If `pending` or `running` tasks are found, return `requires_confirmation=true` with task summaries — same `force=true` re-call pattern as the existing worker confirmation. Update the Bee skill doc to remove the pre-query step.

**Tech Stack:** Go 1.21+, SQLite (via `store.TaskStore`), YAML i18n, Markdown skill docs

---

## File Map

| File | Action | Responsibility |
|------|--------|----------------|
| `internal/infra/i18n/messages.go` | Modify | Add `ClearSessionTasksConfirm` field to `MCPRuntimeMessages` |
| `internal/infra/i18n/locales/en.yaml` | Modify | Add English message text |
| `internal/infra/i18n/locales/zh.yaml` | Modify | Add Chinese message text |
| `internal/mcp/tools.go` | Modify | Insert task detection block in `toolClearSession()`; add `reason` field to worker confirmation |
| `internal/mcp/tools_test.go` | Modify | Add 3 new tests; fix 1 existing test broken by the behavior change |
| `internal/infra/skillinstall/skills/openbee-bee/references/session-management.md` | Modify | Remove pre-query step; update response handling instructions |

---

## Task 1: Add i18n message key for task confirmation

**Files:**
- Modify: `internal/infra/i18n/messages.go:300-302`
- Modify: `internal/infra/i18n/locales/en.yaml:217-218`
- Modify: `internal/infra/i18n/locales/zh.yaml:217-218`

- [ ] **Step 1: Add the struct field in messages.go**

In `internal/infra/i18n/messages.go`, change `MCPRuntimeMessages` from:

```go
type MCPRuntimeMessages struct {
	ClearSessionConfirm string `yaml:"clear_session_confirm"` // confirmation prompt; contains %d
}
```

to:

```go
type MCPRuntimeMessages struct {
	ClearSessionConfirm      string `yaml:"clear_session_confirm"`       // confirmation prompt; contains %d (worker count)
	ClearSessionTasksConfirm string `yaml:"clear_session_tasks_confirm"` // task confirmation prompt; contains %d (task count)
}
```

- [ ] **Step 2: Add English text in en.yaml**

In `internal/infra/i18n/locales/en.yaml`, change the `mcp:` block from:

```yaml
  mcp:
    clear_session_confirm: "This session has %d workers linked. Clearing will reset all worker and bee conversation contexts. Confirm by re-calling with force=true."
```

to:

```yaml
  mcp:
    clear_session_confirm: "This session has %d workers linked. Clearing will reset all worker and bee conversation contexts. Confirm by re-calling with force=true."
    clear_session_tasks_confirm: "There are %d active tasks in this session. Clearing will terminate them. Confirm by re-calling with force=true."
```

- [ ] **Step 3: Add Chinese text in zh.yaml**

In `internal/infra/i18n/locales/zh.yaml`, change the `mcp:` block from:

```yaml
  mcp:
    clear_session_confirm: "此会话链接了 %d 位员工，清空将重置所有员工和 bee 的对话上下文。请确认后以 force=true 重新调用。"
```

to:

```yaml
  mcp:
    clear_session_confirm: "此会话链接了 %d 位员工，清空将重置所有员工和 bee 的对话上下文。请确认后以 force=true 重新调用。"
    clear_session_tasks_confirm: "此会话有 %d 个任务正在运行，清空将终止这些任务。请确认后以 force=true 重新调用。"
```

- [ ] **Step 4: Verify compilation**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee && go build ./internal/infra/i18n/...
```

Expected: no output (clean build)

- [ ] **Step 5: Commit**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee
git add internal/infra/i18n/messages.go internal/infra/i18n/locales/en.yaml internal/infra/i18n/locales/zh.yaml
git commit -m "feat: add i18n key for clear_session task confirmation"
```

---

## Task 2: Write failing tests for task detection

**Files:**
- Modify: `internal/mcp/tools_test.go`

Before implementing the feature, write tests that describe the expected new behavior. These tests will fail until Task 3 is complete.

- [ ] **Step 1: Add the three new test functions**

Append the following to `internal/mcp/tools_test.go` (after the last `TestCallTool_ClearSession_*` function, around line 980):

```go
// --- clear_session task detection ---

func TestCallTool_ClearSession_RunningTaskRequiresConfirmation(t *testing.T) {
	s, db, _, clearer := setupMCPServerWithClear(t)
	ctx := context.Background()

	ms := store.NewMessageStore(db)
	ms.Create(ctx, "msg-rt1", "session-RT", "feishu", "hi", `{}`, "", 0) //nolint

	workerResult, _ := s.CallTool(ctx, "create_worker", mustMarshal(t, map[string]any{"name": "W"}))
	w := workerResult.(model.Worker)

	ts := store.NewTaskStore(db)
	ts.Create(ctx, model.Task{ //nolint
		MessageID: "msg-rt1", WorkerID: w.ID, Instruction: "long running task",
		Type: model.TaskTypeImmediate, Status: model.TaskStatusRunning,
		CreatedAt: 1, UpdatedAt: 1,
	})

	result, err := s.CallTool(ctx, "clear_session", mustMarshal(t, map[string]any{
		"session_key": "session-RT",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := result.(map[string]any)
	if m["requires_confirmation"] != true {
		t.Errorf("expected requires_confirmation=true, got %v", m)
	}
	if m["reason"] != "running_tasks" {
		t.Errorf("expected reason=running_tasks, got %v", m["reason"])
	}
	tasks, ok := m["running_tasks"].([]map[string]string)
	if !ok || len(tasks) != 1 {
		t.Errorf("expected running_tasks with 1 entry, got %v", m["running_tasks"])
	} else {
		if tasks[0]["instruction"] != "long running task" {
			t.Errorf("expected instruction='long running task', got %v", tasks[0]["instruction"])
		}
		if tasks[0]["status"] != "running" {
			t.Errorf("expected status=running, got %v", tasks[0]["status"])
		}
	}

	// ClearSession must NOT have been called.
	clearer.mu.Lock()
	defer clearer.mu.Unlock()
	if len(clearer.cleared) != 0 {
		t.Errorf("ClearSession must not be called on confirmation prompt, got %v", clearer.cleared)
	}
}

func TestCallTool_ClearSession_PendingTaskRequiresConfirmation(t *testing.T) {
	s, db, _, clearer := setupMCPServerWithClear(t)
	ctx := context.Background()

	ms := store.NewMessageStore(db)
	ms.Create(ctx, "msg-pt1", "session-PT", "feishu", "hi", `{}`, "", 0) //nolint

	workerResult, _ := s.CallTool(ctx, "create_worker", mustMarshal(t, map[string]any{"name": "W"}))
	w := workerResult.(model.Worker)

	ts := store.NewTaskStore(db)
	ts.Create(ctx, model.Task{ //nolint
		MessageID: "msg-pt1", WorkerID: w.ID, Instruction: "queued task",
		Type: model.TaskTypeImmediate, Status: model.TaskStatusPending,
		CreatedAt: 1, UpdatedAt: 1,
	})

	result, err := s.CallTool(ctx, "clear_session", mustMarshal(t, map[string]any{
		"session_key": "session-PT",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := result.(map[string]any)
	if m["requires_confirmation"] != true {
		t.Errorf("expected requires_confirmation=true for pending task, got %v", m)
	}
	if m["reason"] != "running_tasks" {
		t.Errorf("expected reason=running_tasks, got %v", m["reason"])
	}

	clearer.mu.Lock()
	defer clearer.mu.Unlock()
	if len(clearer.cleared) != 0 {
		t.Errorf("ClearSession must not be called on confirmation prompt")
	}
}

func TestCallTool_ClearSession_ForceSkipsTaskDetection(t *testing.T) {
	s, db, stopper, clearer := setupMCPServerWithClear(t)
	ctx := context.Background()

	ms := store.NewMessageStore(db)
	ms.Create(ctx, "msg-fsd1", "session-FSD", "feishu", "hi", `{}`, "", 0) //nolint

	workerResult, _ := s.CallTool(ctx, "create_worker", mustMarshal(t, map[string]any{"name": "W"}))
	w := workerResult.(model.Worker)

	ts := store.NewTaskStore(db)
	taskID, _ := ts.Create(ctx, model.Task{
		MessageID: "msg-fsd1", WorkerID: w.ID, Instruction: "long task",
		Type: model.TaskTypeImmediate, Status: model.TaskStatusRunning,
		CreatedAt: 1, UpdatedAt: 1,
	})
	ts.SetExecution(ctx, taskID, "exec-fsd-1", model.TaskStatusRunning) //nolint

	result, err := s.CallTool(ctx, "clear_session", mustMarshal(t, map[string]any{
		"session_key": "session-FSD",
		"force":       true,
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := result.(map[string]any)
	if m["cleared"] != true {
		t.Errorf("expected cleared=true with force=true, got %v", m)
	}

	stopper.mu.Lock()
	defer stopper.mu.Unlock()
	if len(stopper.stopped) != 1 || stopper.stopped[0] != "exec-fsd-1" {
		t.Errorf("expected StopExecution(exec-fsd-1), got %v", stopper.stopped)
	}

	clearer.mu.Lock()
	defer clearer.mu.Unlock()
	if len(clearer.cleared) != 1 || clearer.cleared[0] != "session-FSD" {
		t.Errorf("expected ClearSession(session-FSD), got %v", clearer.cleared)
	}
}
```

- [ ] **Step 2: Run the new tests to confirm they fail**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee && go test ./internal/mcp/... -run "TestCallTool_ClearSession_RunningTaskRequiresConfirmation|TestCallTool_ClearSession_PendingTaskRequiresConfirmation|TestCallTool_ClearSession_ForceSkipsTaskDetection" -v 2>&1 | tail -20
```

Expected: all three tests FAIL (either `requires_confirmation` is missing or `cleared=true` is returned when it shouldn't be)

- [ ] **Step 3: Commit the failing tests**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee
git add internal/mcp/tools_test.go
git commit -m "test: add failing tests for clear_session task detection"
```

---

## Task 3: Implement task detection in toolClearSession

**Files:**
- Modify: `internal/mcp/tools.go:534-561`

- [ ] **Step 1: Insert task detection block and add `reason` to worker response**

In `internal/mcp/tools.go`, replace the existing `if !params.Force {` block (lines 534–561):

```go
	// Two-step confirmation: if more than one worker has a session context and
	// force is not set, return a confirmation prompt without clearing anything.
	if !params.Force {
		agents, err := s.sessionStore.ListSessionContexts(ctx, params.SessionKey)
		if err != nil {
			return nil, fmt.Errorf("list session contexts: %w", err)
		}
		var workers []map[string]string
		seenWorkers := make(map[string]struct{})
		for _, a := range agents {
			if a.AgentType == "worker" {
				if _, exists := seenWorkers[a.AgentID]; exists {
					continue
				}
				seenWorkers[a.AgentID] = struct{}{}
				workers = append(workers, map[string]string{
					"worker_id": a.AgentID,
					"name":      a.Name,
				})
			}
		}
		if len(workers) > 1 {
			return map[string]any{
				"requires_confirmation": true,
				"worker_count":          len(workers),
				"linked_workers":        workers,
				"message":               fmt.Sprintf(i18n.M.Runtime.MCP.ClearSessionConfirm, len(workers)),
			}, nil
		}
	}
```

with:

```go
	if !params.Force {
		// Detect active tasks first; cancelling them is destructive so require
		// explicit confirmation before proceeding.
		activeTasks, err := s.taskStore.ListBySessionKey(ctx, params.SessionKey, "pending,running", "")
		if err != nil {
			return nil, fmt.Errorf("list active tasks: %w", err)
		}
		if len(activeTasks) > 0 {
			taskSummaries := make([]map[string]string, 0, len(activeTasks))
			for _, t := range activeTasks {
				taskSummaries = append(taskSummaries, map[string]string{
					"task_id":     t.ID,
					"instruction": t.Instruction,
					"status":      t.Status,
				})
			}
			return map[string]any{
				"requires_confirmation": true,
				"reason":               "running_tasks",
				"running_tasks":        taskSummaries,
				"message":              fmt.Sprintf(i18n.M.Runtime.MCP.ClearSessionTasksConfirm, len(activeTasks)),
			}, nil
		}

		// If more than one worker has a session context, require confirmation
		// before resetting all their conversation histories.
		agents, err := s.sessionStore.ListSessionContexts(ctx, params.SessionKey)
		if err != nil {
			return nil, fmt.Errorf("list session contexts: %w", err)
		}
		var workers []map[string]string
		seenWorkers := make(map[string]struct{})
		for _, a := range agents {
			if a.AgentType == "worker" {
				if _, exists := seenWorkers[a.AgentID]; exists {
					continue
				}
				seenWorkers[a.AgentID] = struct{}{}
				workers = append(workers, map[string]string{
					"worker_id": a.AgentID,
					"name":      a.Name,
				})
			}
		}
		if len(workers) > 1 {
			return map[string]any{
				"requires_confirmation": true,
				"reason":               "multiple_workers",
				"worker_count":          len(workers),
				"linked_workers":        workers,
				"message":               fmt.Sprintf(i18n.M.Runtime.MCP.ClearSessionConfirm, len(workers)),
			}, nil
		}
	}
```

- [ ] **Step 2: Run the three new tests to confirm they pass**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee && go test ./internal/mcp/... -run "TestCallTool_ClearSession_RunningTaskRequiresConfirmation|TestCallTool_ClearSession_PendingTaskRequiresConfirmation|TestCallTool_ClearSession_ForceSkipsTaskDetection" -v 2>&1 | tail -20
```

Expected: all three PASS

- [ ] **Step 3: Fix the existing regression — `TestCallTool_ClearSession_CancelsAndStopsTasks`**

This test creates active tasks then calls `clear_session` without `force`. After our change it will now get `requires_confirmation` instead of clearing. Update it to pass `force: true`:

In `internal/mcp/tools_test.go`, find `TestCallTool_ClearSession_CancelsAndStopsTasks` (~line 524) and change the `CallTool` invocation from:

```go
	result, err := s.CallTool(context.Background(), "clear_session", mustMarshal(t, map[string]any{
		"session_key": "session-Y",
	}))
```

to:

```go
	result, err := s.CallTool(context.Background(), "clear_session", mustMarshal(t, map[string]any{
		"session_key": "session-Y",
		"force":       true,
	}))
```

- [ ] **Step 4: Run all clear_session tests**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee && go test ./internal/mcp/... -run "TestCallTool_ClearSession" -v 2>&1 | tail -30
```

Expected: all tests PASS (no failures)

- [ ] **Step 5: Run the full test suite**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee && go test ./... 2>&1 | tail -20
```

Expected: all packages PASS

- [ ] **Step 6: Commit**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee
git add internal/mcp/tools.go internal/mcp/tools_test.go
git commit -m "feat: detect running tasks in clear_session and require confirmation"
```

---

## Task 4: Update Bee skill documentation

**Files:**
- Modify: `internal/infra/skillinstall/skills/openbee-bee/references/session-management.md`

- [ ] **Step 1: Replace the Clear Entire Session section**

In `session-management.md`, replace the entire `## Clear Entire Session` section:

```markdown
## Clear Entire Session

When the user sends a message indicating they want to clear/reset the entire conversation (e.g., "clear", "reset context", etc.):

1. Run `openbee ctl task list --session-key <key> --status pending,running` to check for active tasks. If any exist, per notification spec (item 5 — active tasks found before clearing), notify the user via `openbee ctl message send` before proceeding: "There are N tasks currently being processed (Task IDs: ...). Clearing the context will terminate these tasks. Do you confirm continuing?" Then wait for user confirmation before proceeding.

2. Run `openbee ctl session clear --session-key <key>` (without `--force` by default):
   - If it returns `requires_confirmation=true`: per notification spec (item 5 — clearing requires second confirmation), via `openbee ctl message send`, show the user the list of affected workers and inform them "This operation will reset the conversation context of the following workers: [list]. Please confirm whether to continue. After confirmation, the history of all the above workers will be cleared." After user confirms, re-run with `--force`.
   - If it returns `cleared=true`: per notification spec (item 5 — clearing succeeds), inform the user: "Session cleared. All workers' conversation contexts have been reset; you can start a new conversation."
```

with:

```markdown
## Clear Entire Session

When the user sends a message indicating they want to clear/reset the entire conversation (e.g., "clear", "reset context", etc.):

1. Run `openbee ctl session clear --session-key <key>` (without `--force` by default):
   - If it returns `requires_confirmation=true` with `reason=running_tasks`: per notification spec (item 5 — active tasks found before clearing), via `openbee ctl message send`, show the user the running task list and ask: "There are N tasks currently being processed (Tasks: [list of instructions]). Clearing the context will terminate these tasks. Do you confirm continuing?" After user confirms, re-run with `--force`.
   - If it returns `requires_confirmation=true` with `reason=multiple_workers` (or no `reason` field): per notification spec (item 5 — clearing requires second confirmation), via `openbee ctl message send`, show the user the list of affected workers and inform them "This operation will reset the conversation context of the following workers: [list]. Please confirm whether to continue. After confirmation, the history of all the above workers will be cleared." After user confirms, re-run with `--force`.
   - If it returns `cleared=true`: per notification spec (item 5 — clearing succeeds), inform the user: "Session cleared. All workers' conversation contexts have been reset; you can start a new conversation."
```

- [ ] **Step 2: Commit**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee
git add internal/infra/skillinstall/skills/openbee-bee/references/session-management.md
git commit -m "docs: update bee skill to remove task pre-query step from session clear"
```
