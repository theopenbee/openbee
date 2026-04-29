# /status Slash Command Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a chat-side `/status` slash command that, when sent to a bee, replies with the current session's active bees and running immediate tasks.

**Architecture:** New `StatusCommandHandler` in `internal/domain/command/`, mirroring the structure of the existing `EngineCommandHandler` / `ClearCommandHandler` / `StopCommandHandler`. Reuses existing store methods (`SessionStore.ListActiveSessionContexts`, `TaskStore.ListBySessionKey`, `WorkerStore.GetByID`) via three new dedicated interfaces. New i18n keys under `runtime.status_command`. Wired into the existing `msgingest.ChainHandlers` chain in `internal/app/app.go`.

**Tech Stack:** Go, SQLite (via existing stores), `internal/infra/i18n` (yaml-driven), `go.uber.org/zap`. Tests use the existing `command_test` package fakes pattern.

**Spec:** `docs/superpowers/specs/2026-04-29-status-command-design.md`

---

## File Structure

| File | Action | Responsibility |
|------|--------|----------------|
| `internal/domain/command/status.go` | Create | Handler, interfaces, and formatting logic for `/status`. |
| `internal/domain/command/status_test.go` | Create | Unit tests for handler and formatting. |
| `internal/domain/command/engine.go` | Modify | Add `CmdStatus = "/status"` constant. |
| `internal/infra/i18n/messages.go` | Modify | Add `StatusCommandMessages` struct + `StatusCommand` field on `RuntimeMessages`. |
| `internal/infra/i18n/locales/zh.yaml` | Modify | Add `runtime.status_command` block. |
| `internal/infra/i18n/locales/en.yaml` | Modify | Add `runtime.status_command` block. |
| `internal/app/app.go` | Modify | Construct `statusCmdHandler` and add to `cmdChain`. |

---

## Task 1: Add i18n keys

**Files:**
- Modify: `internal/infra/i18n/messages.go:280-290` (add field on `RuntimeMessages`)
- Modify: `internal/infra/i18n/messages.go:318-324` (add new struct after `StopCommandMessages`)
- Modify: `internal/infra/i18n/locales/zh.yaml:213-256`
- Modify: `internal/infra/i18n/locales/en.yaml:213-256`

- [ ] **Step 1: Add `StatusCommand` field to `RuntimeMessages`**

In `internal/infra/i18n/messages.go`, locate the `RuntimeMessages` struct (around line 281) and add the new field at the end:

```go
// platform placeholders, etc.) that must respond to the language setting.
type RuntimeMessages struct {
	FailureNotifier FailureNotifierMessages   `yaml:"failure_notifier"`
	Feishu          FeishuRuntimeMessages     `yaml:"feishu"`
	WeCom           WeComRuntimeMessages      `yaml:"wecom"`
	MCP             MCPRuntimeMessages        `yaml:"mcp"`
	Department      DepartmentRuntimeMessages `yaml:"department"`
	EngineCommand   EngineCommandMessages     `yaml:"engine_command"`
	ClearCommand    ClearCommandMessages      `yaml:"clear_command"`
	StopCommand     StopCommandMessages       `yaml:"stop_command"`
	StatusCommand   StatusCommandMessages     `yaml:"status_command"`
}
```

- [ ] **Step 2: Add `StatusCommandMessages` struct definition**

In the same file, immediately after the `StopCommandMessages` struct (after the closing brace around line 324), append:

```go
// StatusCommandMessages holds text sent to IM users by the /status command handler.
type StatusCommandMessages struct {
	Usage        string `yaml:"usage"`
	LookupFailed string `yaml:"lookup_failed"`
	Header       string `yaml:"header"`
	SectionBees  string `yaml:"section_bees"`  // contains %d
	SectionTasks string `yaml:"section_tasks"` // contains %d
	EmptyMarker  string `yaml:"empty_marker"`
	BeeLine      string `yaml:"bee_line"`  // contains %s, %s, %s (name, engine, last-active)
	TaskLine     string `yaml:"task_line"` // contains %s, %s, %s, %s (worker, content, runtime, exec-id)
}
```

- [ ] **Step 3: Add Chinese locale strings**

In `internal/infra/i18n/locales/zh.yaml`, append the `status_command` block after `stop_command` (after line 256):

```yaml
  status_command:
    usage: "用法：/status"
    lookup_failed: "⚠️ 查询会话状态失败，请稍后重试；若持续出现请检查服务端日志。"
    header: "当前会话状态："
    section_bees: "已激活 bee（%d）："
    section_tasks: "进行中任务（%d）："
    empty_marker: "  (无)"
    bee_line: "  - %s   引擎: %s   最近活跃: %s 前"
    task_line: "  - [%s] %s   已运行 %s   exec: %s"
```

- [ ] **Step 4: Add English locale strings**

In `internal/infra/i18n/locales/en.yaml`, append after `stop_command` (after line 256):

```yaml
  status_command:
    usage: "Usage: /status"
    lookup_failed: "⚠️ Failed to look up session status. Please retry; if this persists, check server logs."
    header: "Current session status:"
    section_bees: "Active bees (%d):"
    section_tasks: "Running tasks (%d):"
    empty_marker: "  (none)"
    bee_line: "  - %s   engine: %s   last active: %s ago"
    task_line: "  - [%s] %s   running for %s   exec: %s"
```

- [ ] **Step 5: Verify i18n loads cleanly**

Run: `go test ./internal/infra/i18n/...`
Expected: PASS (existing tests, plus the loader does not error on the new keys).

- [ ] **Step 6: Commit**

```bash
git add internal/infra/i18n/messages.go internal/infra/i18n/locales/zh.yaml internal/infra/i18n/locales/en.yaml
git commit -m "i18n: add status_command runtime message keys"
```

---

## Task 2: Add `CmdStatus` constant

**Files:**
- Modify: `internal/domain/command/engine.go:20-27`

- [ ] **Step 1: Add the constant**

In `internal/domain/command/engine.go`, extend the const block:

```go
const (
	// CmdEngine is the slash command that switches engines.
	CmdEngine = "/engine"
	// CmdClear is the slash command that clears session contexts.
	CmdClear = "/clear"
	// CmdStop is the slash command that stops the running bee and cancels pending messages.
	CmdStop = "/stop"
	// CmdStatus is the slash command that prints the current session status.
	CmdStatus = "/status"
)
```

- [ ] **Step 2: Verify build**

Run: `go build ./...`
Expected: PASS (no compile error; constant is unused but exported, so OK).

- [ ] **Step 3: Commit**

```bash
git add internal/domain/command/engine.go
git commit -m "command: add CmdStatus constant"
```

---

## Task 3: Test-drive `IsCommand` and Usage path

**Files:**
- Create: `internal/domain/command/status.go`
- Create: `internal/domain/command/status_test.go`

- [ ] **Step 1: Write the first failing test**

Create `internal/domain/command/status_test.go`:

```go
package command_test

import (
	"context"
	"testing"

	"github.com/theopenbee/openbee/internal/domain/command"
	"github.com/theopenbee/openbee/internal/domain/enginecfg"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/infra/store"
	"github.com/theopenbee/openbee/internal/platform"
)

// --- fakes for /status ---

type fakeStatusSessionLister struct {
	agents []store.SessionAgent
	err    error
}

func (f *fakeStatusSessionLister) ListActiveSessionContexts(_ context.Context, _, _ string) ([]store.SessionAgent, error) {
	return f.agents, f.err
}

type fakeStatusTaskLister struct {
	tasks []model.Task
	err   error
}

func (f *fakeStatusTaskLister) ListBySessionKey(_ context.Context, _, _, _ string) ([]model.Task, error) {
	return f.tasks, f.err
}

type fakeStatusWorkerLookup struct {
	byID map[string]model.Worker
	err  error
}

func (f *fakeStatusWorkerLookup) GetByID(id string) (model.Worker, error) {
	if f.err != nil {
		return model.Worker{}, f.err
	}
	w, ok := f.byID[id]
	if !ok {
		return model.Worker{Name: ""}, nil
	}
	return w, nil
}

func makeStatusHandler(
	agents []store.SessionAgent,
	tasks []model.Task,
	workers map[string]model.Worker,
) (*command.StatusCommandHandler, *fakeSender) {
	sender := &fakeSender{}
	senders := map[string]platform.PlatformSenderAdapter{"feishu": sender}
	sessions := &fakeStatusSessionLister{agents: agents}
	taskList := &fakeStatusTaskLister{tasks: tasks}
	wl := &fakeStatusWorkerLookup{byID: workers}
	engineCfg := enginecfg.NewStore("claude")
	h := command.NewStatusCommandHandler(sessions, taskList, wl, senders, engineCfg)
	return h, sender
}

func TestStatusCommand_IsCommand(t *testing.T) {
	h, _ := makeStatusHandler(nil, nil, nil)
	cases := map[string]bool{
		"/status":   true,
		"/status x": true,
		"/statuses": false,
		"hello":     false,
		"":          false,
	}
	for input, want := range cases {
		if got := h.IsCommand(input); got != want {
			t.Errorf("IsCommand(%q) = %v, want %v", input, got, want)
		}
	}
}

func TestStatusCommand_UsageOnExtraArgs(t *testing.T) {
	h, sender := makeStatusHandler(nil, nil, nil)
	handled := h.HandleCommand(context.Background(), "/status x", makeReplyTo())
	if !handled {
		t.Fatal("expected handled=true")
	}
	if len(sender.sent) != 1 || sender.sent[0] != "用法：/status" {
		t.Errorf("unexpected reply: %v", sender.sent)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/domain/command/ -run TestStatusCommand`
Expected: FAIL — `undefined: command.NewStatusCommandHandler` and friends.

- [ ] **Step 3: Create minimal `status.go` to make tests compile and pass usage cases**

Create `internal/domain/command/status.go`:

```go
package command

import (
	"context"
	"strings"

	"github.com/theopenbee/openbee/internal/domain/enginecfg"
	"github.com/theopenbee/openbee/internal/infra/i18n"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/infra/store"
	"github.com/theopenbee/openbee/internal/platform"
)

// StatusSessionLister is the subset of SessionStore needed by StatusCommandHandler.
type StatusSessionLister interface {
	ListActiveSessionContexts(ctx context.Context, sessionKey, beeEngine string) ([]store.SessionAgent, error)
}

// StatusTaskLister is the subset of TaskStore needed by StatusCommandHandler.
type StatusTaskLister interface {
	ListBySessionKey(ctx context.Context, sessionKey, status, taskType string) ([]model.Task, error)
}

// StatusWorkerLookup is the subset of WorkerStore needed by StatusCommandHandler
// to render task lines with the worker's display name.
type StatusWorkerLookup interface {
	GetByID(id string) (model.Worker, error)
}

// StatusCommandHandler handles the /status slash command.
type StatusCommandHandler struct {
	sessions  StatusSessionLister
	tasks     StatusTaskLister
	workers   StatusWorkerLookup
	senders   map[string]platform.PlatformSenderAdapter
	engineCfg *enginecfg.Store
}

func NewStatusCommandHandler(
	sessions StatusSessionLister,
	tasks StatusTaskLister,
	workers StatusWorkerLookup,
	senders map[string]platform.PlatformSenderAdapter,
	engineCfg *enginecfg.Store,
) *StatusCommandHandler {
	return &StatusCommandHandler{
		sessions:  sessions,
		tasks:     tasks,
		workers:   workers,
		senders:   senders,
		engineCfg: engineCfg,
	}
}

func (h *StatusCommandHandler) IsCommand(content string) bool {
	return isExactOrPrefixed(content, CmdStatus)
}

func (h *StatusCommandHandler) HandleCommand(ctx context.Context, content string, replyTo platform.InboundMessage) bool {
	fields := strings.Fields(content)
	if len(fields) == 0 || fields[0] != CmdStatus {
		return false
	}
	if len(fields) != 1 {
		h.reply(ctx, replyTo, i18n.M.Runtime.StatusCommand.Usage)
		return true
	}
	// Full implementation follows in later tasks.
	h.reply(ctx, replyTo, i18n.M.Runtime.StatusCommand.Usage)
	return true
}

func (h *StatusCommandHandler) reply(ctx context.Context, replyTo platform.InboundMessage, text string) {
	sendReply(ctx, h.senders, replyTo, text)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/domain/command/ -run TestStatusCommand`
Expected: PASS for `IsCommand` and `UsageOnExtraArgs`.

- [ ] **Step 5: Commit**

```bash
git add internal/domain/command/status.go internal/domain/command/status_test.go
git commit -m "command: scaffold /status handler with IsCommand and usage"
```

---

## Task 4: Test-drive format helpers (relative time + truncation)

**Files:**
- Modify: `internal/domain/command/status.go` (add helpers)
- Modify: `internal/domain/command/status_test.go` (add helper tests)

- [ ] **Step 1: Write failing tests for helpers**

Append to `internal/domain/command/status_test.go`:

```go
import (
	// ... add this to the existing import block:
	"github.com/theopenbee/openbee/internal/domain/command/internal/statusfmt"
)
```

Wait — to keep helpers private and testable in the same `command` package, we'll use a `_internal_test.go` (white-box). Instead, expose helpers via an internal test file.

Replace the previous import note. Create `internal/domain/command/status_format_test.go`:

```go
package command

import "testing"

func TestFormatRelative(t *testing.T) {
	cases := []struct {
		seconds int64
		want    string
	}{
		{0, "0s"},
		{59, "59s"},
		{60, "1m"},
		{61, "1m"},
		{3599, "59m"},
		{3600, "1h"},
		{86399, "23h"},
		{86400, "1d"},
		{172800, "2d"},
	}
	for _, c := range cases {
		if got := formatRelative(c.seconds); got != c.want {
			t.Errorf("formatRelative(%d) = %q, want %q", c.seconds, got, c.want)
		}
	}
}

func TestFormatRelative_NegativeOrZero(t *testing.T) {
	// Clock skew or future timestamps must not panic; clamp to "0s".
	if got := formatRelative(-5); got != "0s" {
		t.Errorf("formatRelative(-5) = %q, want %q", got, "0s")
	}
}

func TestTruncateInstruction(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"hello", "hello"},
		{"line1\nline2", "line1 line2"},
		{"line1\r\nline2", "line1 line2"},
		{"a\tb", "a b"},
		// Exactly 40 runes — kept verbatim.
		{"0123456789012345678901234567890123456789", "0123456789012345678901234567890123456789"},
		// 41 runes — truncated to 40 + ellipsis.
		{"01234567890123456789012345678901234567890", "0123456789012345678901234567890123456789…"},
		// CJK runes counted by rune, not byte.
		{"中文中文中文中文中文中文中文中文中文中文中文中文中文中文中文中文中文中文中文中文中文",
			"中文中文中文中文中文中文中文中文中文中文中文中文中文中文中文中文中文中文中文中文…"},
	}
	for _, c := range cases {
		if got := truncateInstruction(c.in); got != c.want {
			t.Errorf("truncateInstruction(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestShortExecID(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"abc", "abc"},
		{"abcdef12", "abcdef12"},
		{"abcdef1234567890", "abcdef12"},
	}
	for _, c := range cases {
		if got := shortExecID(c.in); got != c.want {
			t.Errorf("shortExecID(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/domain/command/ -run "TestFormatRelative|TestTruncateInstruction|TestShortExecID"`
Expected: FAIL — `undefined: formatRelative`, `truncateInstruction`, `shortExecID`.

- [ ] **Step 3: Implement helpers in `status.go`**

Append to `internal/domain/command/status.go`:

```go
import (
	// add to existing import block:
	"fmt"
	"strings"
	"unicode/utf8"
)
```

(`strings` already imported in step Task 3; only add `fmt` and `unicode/utf8`. The final import block should contain: `context`, `fmt`, `strings`, `unicode/utf8`, plus the project paths.)

Append helper functions:

```go
const (
	maxInstructionRunes = 40
	shortExecIDLen      = 8
)

// formatRelative renders a duration in seconds as a coarse human string:
// "Ns" (<60s), "Nm" (<60m), "Nh" (<24h), or "Nd" (≥24h).
// Negative values are clamped to "0s" to handle clock skew.
func formatRelative(seconds int64) string {
	if seconds < 0 {
		seconds = 0
	}
	switch {
	case seconds < 60:
		return fmt.Sprintf("%ds", seconds)
	case seconds < 3600:
		return fmt.Sprintf("%dm", seconds/60)
	case seconds < 86400:
		return fmt.Sprintf("%dh", seconds/3600)
	default:
		return fmt.Sprintf("%dd", seconds/86400)
	}
}

// truncateInstruction collapses any whitespace runs to a single space and
// shortens the result to maxInstructionRunes runes, appending "…" if cut.
func truncateInstruction(s string) string {
	// Collapse all whitespace (incl. \n, \r, \t) into single spaces.
	s = strings.Join(strings.Fields(s), " ")
	if utf8.RuneCountInString(s) <= maxInstructionRunes {
		return s
	}
	runes := []rune(s)
	return string(runes[:maxInstructionRunes]) + "…"
}

// shortExecID returns the first shortExecIDLen characters of an execution id,
// or the whole string if shorter. Empty input returns empty.
func shortExecID(id string) string {
	if len(id) <= shortExecIDLen {
		return id
	}
	return id[:shortExecIDLen]
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/domain/command/ -run "TestFormatRelative|TestTruncateInstruction|TestShortExecID"`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/domain/command/status.go internal/domain/command/status_format_test.go
git commit -m "command: add status format helpers (relative time, truncation, short id)"
```

---

## Task 5: Test-drive happy path (bees + tasks)

**Files:**
- Modify: `internal/domain/command/status.go` (replace placeholder `HandleCommand` body)
- Modify: `internal/domain/command/status_test.go` (add happy-path test)

- [ ] **Step 1: Write failing happy-path test**

Append to `internal/domain/command/status_test.go`:

```go
import (
	// Ensure these are in the test import block:
	"strings"
	"time"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/infra/store"
)

func TestStatusCommand_HappyPath(t *testing.T) {
	now := time.Now().Unix()
	agents := []store.SessionAgent{
		{AgentID: "w1", AgentType: "worker", Engine: "claude", Name: "貂蝉", UpdatedAt: now - 120}, // 2m
		{AgentID: "w2", AgentType: "worker", Engine: "codex", Name: "吕布", UpdatedAt: now - 18000}, // 5h
	}
	nowMs := time.Now().UnixMilli()
	tasks := []model.Task{
		{ID: "t1", WorkerID: "w1", Instruction: "新增 /status 指令的实现", ExecutionID: "a1b2c3d4e5f6", CreatedAt: nowMs - 83000, Status: model.TaskStatusRunning, Type: model.TaskTypeImmediate},
		{ID: "t2", WorkerID: "w2", Instruction: "修复登录 bug", ExecutionID: "e5f6a7b89999", CreatedAt: nowMs - 12000, Status: model.TaskStatusRunning, Type: model.TaskTypeImmediate},
	}
	workers := map[string]model.Worker{
		"w1": {ID: "w1", Name: "貂蝉"},
		"w2": {ID: "w2", Name: "吕布"},
	}
	h, sender := makeStatusHandler(agents, tasks, workers)
	handled := h.HandleCommand(context.Background(), "/status", makeReplyTo())
	if !handled {
		t.Fatal("expected handled=true")
	}
	if len(sender.sent) != 1 {
		t.Fatalf("expected 1 reply, got %d", len(sender.sent))
	}
	out := sender.sent[0]

	// Header + section headers
	for _, want := range []string{
		"当前会话状态：",
		"已激活 bee（2）：",
		"进行中任务（2）：",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\n--- output ---\n%s", want, out)
		}
	}
	// Bee lines
	for _, want := range []string{
		"- 貂蝉   引擎: claude   最近活跃: 2m 前",
		"- 吕布   引擎: codex   最近活跃: 5h 前",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\n--- output ---\n%s", want, out)
		}
	}
	// Task lines
	for _, want := range []string{
		"- [貂蝉] 新增 /status 指令的实现   已运行 1m   exec: a1b2c3d4",
		"- [吕布] 修复登录 bug   已运行 12s   exec: e5f6a7b8",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\n--- output ---\n%s", want, out)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/domain/command/ -run TestStatusCommand_HappyPath`
Expected: FAIL — current `HandleCommand` only returns Usage.

- [ ] **Step 3: Replace `HandleCommand` body with full implementation**

In `internal/domain/command/status.go`, replace the `HandleCommand` body and add a `formatStatus` helper:

```go
func (h *StatusCommandHandler) HandleCommand(ctx context.Context, content string, replyTo platform.InboundMessage) bool {
	fields := strings.Fields(content)
	if len(fields) == 0 || fields[0] != CmdStatus {
		return false
	}
	if len(fields) != 1 {
		h.reply(ctx, replyTo, i18n.M.Runtime.StatusCommand.Usage)
		return true
	}

	m := i18n.M.Runtime.StatusCommand
	sessionKey := replyTo.SessionKey

	agents, err := h.sessions.ListActiveSessionContexts(ctx, sessionKey, h.engineCfg.Get())
	if err != nil {
		log.Error("list session contexts for /status", zap.String("sessionKey", sessionKey), zap.Error(err))
		h.reply(ctx, replyTo, m.LookupFailed)
		return true
	}

	tasks, err := h.tasks.ListBySessionKey(ctx, sessionKey, model.TaskStatusRunning, model.TaskTypeImmediate)
	if err != nil {
		log.Error("list tasks for /status", zap.String("sessionKey", sessionKey), zap.Error(err))
		h.reply(ctx, replyTo, m.LookupFailed)
		return true
	}

	h.reply(ctx, replyTo, h.formatStatus(agents, tasks))
	return true
}

// formatStatus renders the full /status reply text. Both sections always appear,
// using i18n.empty_marker when the corresponding slice is empty.
func (h *StatusCommandHandler) formatStatus(agents []store.SessionAgent, tasks []model.Task) string {
	m := i18n.M.Runtime.StatusCommand
	nowSec := time.Now().Unix()
	nowMs := time.Now().UnixMilli()

	var b strings.Builder
	b.WriteString(m.Header)
	b.WriteByte('\n')

	// Bees section.
	fmt.Fprintf(&b, m.SectionBees, len(agents))
	b.WriteByte('\n')
	if len(agents) == 0 {
		b.WriteString(m.EmptyMarker)
		b.WriteByte('\n')
	} else {
		for _, a := range agents {
			fmt.Fprintf(&b, m.BeeLine, a.Name, a.Engine, formatRelative(nowSec-a.UpdatedAt))
			b.WriteByte('\n')
		}
	}

	// Tasks section.
	fmt.Fprintf(&b, m.SectionTasks, len(tasks))
	b.WriteByte('\n')
	if len(tasks) == 0 {
		b.WriteString(m.EmptyMarker)
	} else {
		for i, t := range tasks {
			workerName := h.lookupWorkerName(t.WorkerID)
			runtimeSec := (nowMs - t.CreatedAt) / 1000
			fmt.Fprintf(&b, m.TaskLine,
				workerName,
				truncateInstruction(t.Instruction),
				formatRelative(runtimeSec),
				shortExecID(t.ExecutionID),
			)
			if i < len(tasks)-1 {
				b.WriteByte('\n')
			}
		}
	}
	return b.String()
}

// lookupWorkerName resolves a worker id to its display name. On lookup failure
// or empty result it returns the id, so the user can still correlate the line.
func (h *StatusCommandHandler) lookupWorkerName(id string) string {
	if id == "" {
		return "?"
	}
	w, err := h.workers.GetByID(id)
	if err != nil || w.Name == "" {
		return id
	}
	return w.Name
}
```

Add `time` and `go.uber.org/zap` to the import block of `status.go`. The final import block should be:

```go
import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"go.uber.org/zap"

	"github.com/theopenbee/openbee/internal/domain/enginecfg"
	"github.com/theopenbee/openbee/internal/infra/i18n"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/infra/store"
	"github.com/theopenbee/openbee/internal/platform"
)
```

(`log` is the package-level zap logger declared in `engine.go`; reuse it without re-declaring.)

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/domain/command/ -run TestStatusCommand_HappyPath -v`
Expected: PASS, with output containing all the asserted substrings.

- [ ] **Step 5: Commit**

```bash
git add internal/domain/command/status.go internal/domain/command/status_test.go
git commit -m "command: implement /status happy path (bees + tasks)"
```

---

## Task 6: Test-drive empty states

**Files:**
- Modify: `internal/domain/command/status_test.go`

- [ ] **Step 1: Write failing tests for the three empty-state cases**

Append to `internal/domain/command/status_test.go`:

```go
func TestStatusCommand_EmptyBeesAndTasks(t *testing.T) {
	h, sender := makeStatusHandler(nil, nil, nil)
	h.HandleCommand(context.Background(), "/status", makeReplyTo())
	if len(sender.sent) != 1 {
		t.Fatalf("expected 1 reply, got %d", len(sender.sent))
	}
	out := sender.sent[0]
	for _, want := range []string{
		"已激活 bee（0）：",
		"进行中任务（0）：",
		"  (无)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\n--- output ---\n%s", want, out)
		}
	}
	// "(无)" must appear at least twice (one per empty section).
	if strings.Count(out, "  (无)") != 2 {
		t.Errorf("expected exactly two empty markers, got:\n%s", out)
	}
}

func TestStatusCommand_BeesOnly_NoTasks(t *testing.T) {
	now := time.Now().Unix()
	agents := []store.SessionAgent{
		{AgentID: "w1", AgentType: "worker", Engine: "claude", Name: "貂蝉", UpdatedAt: now - 30},
	}
	h, sender := makeStatusHandler(agents, nil, map[string]model.Worker{"w1": {ID: "w1", Name: "貂蝉"}})
	h.HandleCommand(context.Background(), "/status", makeReplyTo())
	out := sender.sent[0]
	if !strings.Contains(out, "已激活 bee（1）：") {
		t.Errorf("missing bees header\n%s", out)
	}
	if !strings.Contains(out, "进行中任务（0）：") {
		t.Errorf("missing tasks header\n%s", out)
	}
	if strings.Count(out, "  (无)") != 1 {
		t.Errorf("expected exactly one empty marker, got:\n%s", out)
	}
}

func TestStatusCommand_TasksOnly_NoBees(t *testing.T) {
	nowMs := time.Now().UnixMilli()
	tasks := []model.Task{
		{ID: "t1", WorkerID: "w1", Instruction: "do thing", ExecutionID: "deadbeef0000", CreatedAt: nowMs - 5000, Status: model.TaskStatusRunning, Type: model.TaskTypeImmediate},
	}
	workers := map[string]model.Worker{"w1": {ID: "w1", Name: "貂蝉"}}
	h, sender := makeStatusHandler(nil, tasks, workers)
	h.HandleCommand(context.Background(), "/status", makeReplyTo())
	out := sender.sent[0]
	if !strings.Contains(out, "已激活 bee（0）：") {
		t.Errorf("missing bees header\n%s", out)
	}
	if !strings.Contains(out, "进行中任务（1）：") {
		t.Errorf("missing tasks header\n%s", out)
	}
	if !strings.Contains(out, "[貂蝉] do thing") {
		t.Errorf("missing task line\n%s", out)
	}
	if strings.Count(out, "  (无)") != 1 {
		t.Errorf("expected exactly one empty marker, got:\n%s", out)
	}
}
```

- [ ] **Step 2: Run tests**

Run: `go test ./internal/domain/command/ -run "TestStatusCommand_Empty|TestStatusCommand_BeesOnly|TestStatusCommand_TasksOnly" -v`
Expected: PASS — the implementation from Task 5 already handles these cases.

- [ ] **Step 3: Commit**

```bash
git add internal/domain/command/status_test.go
git commit -m "command: cover /status empty-state branches with tests"
```

---

## Task 7: Test-drive store-failure error handling

**Files:**
- Modify: `internal/domain/command/status_test.go`

- [ ] **Step 1: Write failing tests for the two store-error paths**

Append to `internal/domain/command/status_test.go`:

```go
import "errors"

func TestStatusCommand_SessionListErr(t *testing.T) {
	sender := &fakeSender{}
	senders := map[string]platform.PlatformSenderAdapter{"feishu": sender}
	sessions := &fakeStatusSessionLister{err: errors.New("boom")}
	taskList := &fakeStatusTaskLister{}
	wl := &fakeStatusWorkerLookup{}
	engineCfg := enginecfg.NewStore("claude")
	h := command.NewStatusCommandHandler(sessions, taskList, wl, senders, engineCfg)

	h.HandleCommand(context.Background(), "/status", makeReplyTo())
	if len(sender.sent) != 1 {
		t.Fatalf("expected 1 reply, got %d", len(sender.sent))
	}
	if !strings.Contains(sender.sent[0], "查询会话状态失败") {
		t.Errorf("expected lookup_failed reply, got %q", sender.sent[0])
	}
}

func TestStatusCommand_TaskListErr(t *testing.T) {
	sender := &fakeSender{}
	senders := map[string]platform.PlatformSenderAdapter{"feishu": sender}
	sessions := &fakeStatusSessionLister{agents: nil}
	taskList := &fakeStatusTaskLister{err: errors.New("boom")}
	wl := &fakeStatusWorkerLookup{}
	engineCfg := enginecfg.NewStore("claude")
	h := command.NewStatusCommandHandler(sessions, taskList, wl, senders, engineCfg)

	h.HandleCommand(context.Background(), "/status", makeReplyTo())
	if len(sender.sent) != 1 {
		t.Fatalf("expected 1 reply, got %d", len(sender.sent))
	}
	if !strings.Contains(sender.sent[0], "查询会话状态失败") {
		t.Errorf("expected lookup_failed reply, got %q", sender.sent[0])
	}
}
```

- [ ] **Step 2: Run tests**

Run: `go test ./internal/domain/command/ -run "TestStatusCommand_SessionListErr|TestStatusCommand_TaskListErr" -v`
Expected: PASS — the implementation in Task 5 already returns `LookupFailed` on these errors.

- [ ] **Step 3: Commit**

```bash
git add internal/domain/command/status_test.go
git commit -m "command: cover /status store-failure paths with tests"
```

---

## Task 8: Wire `StatusCommandHandler` into the dispatch chain

**Files:**
- Modify: `internal/app/app.go:158-161`

- [ ] **Step 1: Construct and wire the handler**

In `internal/app/app.go`, locate the lines (around 158–161):

```go
engineCmdHandler := command.NewEngineCommandHandler(s.workerStore, s.systemConfigStore, sendersByPlatform, mgr, busyChecker, engineCfg)
clearCmdHandler := command.NewClearCommandHandler(s.workerStore, s.sessionStore, s.taskStore, mgr, disp, sendersByPlatform, engineCfg)
stopCmdHandler := command.NewStopCommandHandler(feeder, s.msgStore, sendersByPlatform)
cmdChain := msgingest.ChainHandlers(engineCmdHandler, clearCmdHandler, stopCmdHandler)
```

Modify to add `statusCmdHandler` and append it to the chain:

```go
engineCmdHandler := command.NewEngineCommandHandler(s.workerStore, s.systemConfigStore, sendersByPlatform, mgr, busyChecker, engineCfg)
clearCmdHandler := command.NewClearCommandHandler(s.workerStore, s.sessionStore, s.taskStore, mgr, disp, sendersByPlatform, engineCfg)
stopCmdHandler := command.NewStopCommandHandler(feeder, s.msgStore, sendersByPlatform)
statusCmdHandler := command.NewStatusCommandHandler(s.sessionStore, s.taskStore, s.workerStore, sendersByPlatform, engineCfg)
cmdChain := msgingest.ChainHandlers(engineCmdHandler, clearCmdHandler, stopCmdHandler, statusCmdHandler)
```

- [ ] **Step 2: Verify the build**

Run: `go build ./...`
Expected: PASS. If `s.sessionStore` / `s.taskStore` / `s.workerStore` types do not satisfy the new interfaces, the compiler will report it; in that case verify the method signatures match exactly:
- `SessionStore.ListActiveSessionContexts(ctx, sessionKey, beeEngine string) ([]store.SessionAgent, error)`
- `TaskStore.ListBySessionKey(ctx, sessionKey, status, taskType string) ([]model.Task, error)`
- `WorkerStore.GetByID(id string) (model.Worker, error)`

(All three already exist in the codebase; no store changes are required.)

- [ ] **Step 3: Commit**

```bash
git add internal/app/app.go
git commit -m "app: register /status command handler in dispatch chain"
```

---

## Task 9: Final verification

- [ ] **Step 1: Run the full test suite**

Run: `go test ./...`
Expected: PASS for all packages.

- [ ] **Step 2: Run go vet**

Run: `go vet ./...`
Expected: no warnings.

- [ ] **Step 3: Build all binaries**

Run: `go build ./...`
Expected: PASS.

- [ ] **Step 4: (Optional, manual) Smoke-test in dev**

If a local dev setup is available, start the daemon, send `/status` from a connected platform, and verify a reply appears with the expected layout. Then send `/status garbage` and confirm the usage line.

- [ ] **Step 5: No additional commit needed unless smoke-test reveals issues.**

---

## Self-Review Notes

- **Spec coverage:** §3.1–3.5 → Task 1 (i18n keys) + Task 5 (`formatStatus`) + Task 4 (helpers) + Task 3 (Usage). §4.1–4.6 → Tasks 2, 3, 5, 8. §5 → Task 7. §6 → Tasks 3, 5, 6, 7. §8 risk acknowledged: relative-time helper duplicated in domain layer instead of imported from `cmd/openbee`.
- **Worker-name lookup:** Plan picks the `WorkerStore.GetByID` path (design §4.3); the alternative (in-memory map from `SessionAgent`) is not needed because `GetByID` already exists.
- **i18n format strings:** `bee_line` carries the trailing "前" / " ago" suffix so the helper need only emit the bare duration. `task_line` has no trailing suffix on the runtime field.
- **Empty-state count assertion:** `strings.Count(out, "  (无)") == 2` is correct because both empty sections emit the indented marker; if either section produces a real line, that section emits no marker.
