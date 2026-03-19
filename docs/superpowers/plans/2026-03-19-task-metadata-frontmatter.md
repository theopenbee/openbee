# Task Metadata Frontmatter Format Change Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the `[系统元数据] task_id=xxx message_id=xxx` metadata prefix format with YAML frontmatter (`---\ntask_id: xxx\n---`) in the task dispatcher, and update worker documentation and tests accordingly.

**Architecture:** The change touches two production files and two test files. `dispatcher.go` generates the format; `claudemd.go` documents it for LLM workers. Both tests verify their respective outputs. No new abstractions or files are needed.

**Tech Stack:** Go, standard library only (`fmt`, `strings`)

---

### Task 1: Update `buildInstruction` format in `dispatcher.go`

**Files:**
- Modify: `internal/task_dispatcher/dispatcher.go:179-185`
- Test: `internal/task_dispatcher/dispatcher_test.go` (existing test, updated in Task 2)

- [ ] **Step 1: Open the file and locate `buildInstruction`**

  File: `internal/task_dispatcher/dispatcher.go`, lines 177-185.
  The function currently reads:
  ```go
  func buildInstruction(task DispatchTask) string {
      if task.TaskID == "" {
          return task.Instruction
      }
      return fmt.Sprintf("[系统元数据] task_id=%s message_id=%s\n\n%s",
          task.TaskID, task.MessageID, task.Instruction)
  }
  ```

- [ ] **Step 2: Replace the format string**

  Change the `fmt.Sprintf` line to:
  ```go
  return fmt.Sprintf("---\ntask_id: %s\nmessage_id: %s\n---\n\n%s",
      task.TaskID, task.MessageID, task.Instruction)
  ```
  The empty-`task_id` guard (`if task.TaskID == "" { return task.Instruction }`) must be preserved unchanged.

- [ ] **Step 3: Run existing tests to verify the current state**

  ```bash
  go test ./internal/task_dispatcher/... -v -run TestBuildInstruction
  ```
  Expected: FAIL — the test still checks for `"task_id=task-abc"` (old format).

---

### Task 2: Update `dispatcher_test.go` assertions

**Files:**
- Modify: `internal/task_dispatcher/dispatcher_test.go:200-204`

- [ ] **Step 1: Locate the assertions**

  In the test that calls `buildInstruction` (around line 196-208), find:
  ```go
  if !strings.Contains(instr, "task_id=task-abc") {
      t.Errorf("instruction missing task_id injection, got: %q", instr)
  }
  if !strings.Contains(instr, "message_id=msg-xyz") {
      t.Errorf("instruction missing message_id injection, got: %q", instr)
  }
  ```

- [ ] **Step 2: Replace the two assertions with three**

  ```go
  if !strings.HasPrefix(instr, "---\n") {
      t.Errorf("instruction missing frontmatter prefix, got: %q", instr)
  }
  if !strings.Contains(instr, "task_id: task-abc") {
      t.Errorf("instruction missing task_id injection, got: %q", instr)
  }
  if !strings.Contains(instr, "message_id: msg-xyz") {
      t.Errorf("instruction missing message_id injection, got: %q", instr)
  }
  ```

- [ ] **Step 3: Run the test and verify it passes**

  ```bash
  go test ./internal/task_dispatcher/... -v -run TestBuildInstruction
  ```
  Expected: PASS

- [ ] **Step 4: Run the full dispatcher test suite**

  ```bash
  go test ./internal/task_dispatcher/... -v
  ```
  Expected: all PASS

- [ ] **Step 5: Commit**

  ```bash
  git add internal/task_dispatcher/dispatcher.go internal/task_dispatcher/dispatcher_test.go
  git commit -m "feat: change task metadata format to YAML frontmatter"
  ```

---

### Task 3: Update `workerRules` in `claudemd.go`

**Files:**
- Modify: `internal/claudemd/claudemd.go:232-254`

- [ ] **Step 1: Locate the `workerRules` function**

  File: `internal/claudemd/claudemd.go`, lines 225-255.
  The function builds a string that is prepended to worker system prompts.
  The section to change is the `fmt.Sprintf` block starting at line 232.

- [ ] **Step 2: Replace the entire `fmt.Sprintf` block (lines 232-254)**

  Replace the whole block — from `return namePrefix + "\n" + fmt.Sprintf(`` to the closing `)`  — with the following. Do NOT try to incrementally remove/add lines; replace the whole block at once to avoid argument-count mismatches:

  ```go
  return namePrefix + "\n" + fmt.Sprintf(`
  ## 任务状态标记（强制 — 不可省略）

  每个任务的指令以 YAML frontmatter 开头，其中包含 task_id 和 message_id：

  - **task_id** — 当前任务的唯一标识，用于调用 `+"`%s`"+` 标记任务成功
  - **message_id** — 原始用户消息的标识，用于调用 `+"`%s`"+` 回复用户（可能为空）

  无论任务执行成功还是失败，无论过程中发生了什么，你都必须调用 `+"`%s`"+` 标记任务完成。

  这是每个任务的最后一步，绝对不可遗漏。先调用 `+"`%s`"+` 通知结果，再调用 `+"`%s`"+` 标记完成。如果你没有调用 `+"`%s`"+`，任务将永远处于运行状态，这是严重错误。
  `,
      toolnames.MarkTaskComplete, toolnames.SendMessage,
      toolnames.MarkTaskComplete,
      toolnames.SendMessage, toolnames.MarkTaskComplete, toolnames.MarkTaskComplete)
  }
  ```

  Note the backtick-escaping pattern (`` `+"`%s`"+` ``) is already used throughout the file for inline code in template strings — follow the same pattern.

- [ ] **Step 4: Build to catch any argument-count mismatches**

  ```bash
  go build ./internal/claudemd/...
  ```
  Expected: no errors. If there is a `%!(EXTRA` or `%!` error, the number of format verbs and arguments is mismatched — recount the `%s` occurrences and the argument list.

---

### Task 4: Update `claudemd_test.go` assertions

**Files:**
- Modify: `internal/claudemd/claudemd_test.go:72-74`

- [ ] **Step 1: Locate the assertion**

  Find (around line 72):
  ```go
  if !strings.Contains(content, "系统元数据") {
      t.Error("missing worker-specific 系统元数据 section")
  }
  ```

- [ ] **Step 2: Replace with two assertions**

  ```go
  if !strings.Contains(content, "task_id") {
      t.Error("missing task_id field explanation in worker rules")
  }
  if strings.Contains(content, "系统元数据") {
      t.Error("old 系统元数据 section should have been removed")
  }
  ```

- [ ] **Step 3: Run the claudemd tests**

  ```bash
  go test ./internal/claudemd/... -v
  ```
  Expected: all PASS

- [ ] **Step 4: Run the full test suite**

  ```bash
  go test ./...
  ```
  Expected: all PASS

- [ ] **Step 5: Commit**

  ```bash
  git add internal/claudemd/claudemd.go internal/claudemd/claudemd_test.go
  git commit -m "refactor: merge worker metadata docs into 任务状态标记 section"
  ```
