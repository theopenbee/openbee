# `/list` Slash Command Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a `/list [keyword]` slash command that prints the bee's worker directory, optionally filtered by case-insensitive substring match on `worker.description`.

**Architecture:** New `ListCommandHandler` under `internal/domain/command/`, parallel to the four existing handlers (`/engine`, `/clear`, `/stop`, `/status`). It reads from `*store.WorkerStore.List()`, filters and sorts in memory, formats via i18n templates, and is wired into the `msgingest.ChainHandlers` chain in `internal/app/app.go`. New i18n section `list_command` added to `RuntimeMessages`, zh.yaml, and en.yaml.

**Tech Stack:** Go 1.x, `database/sql` via `*store.WorkerStore`, `go.uber.org/zap`, `internal/infra/i18n` (struct-tagged YAML), `internal/platform` for IM reply.

**Spec reference:** `docs/superpowers/specs/2026-05-11-list-command-design.md`

---

## File Structure

- `internal/domain/command/list.go` (new) — `ListCommandHandler`, `WorkerLister` interface, `CmdList` constant, status-label helper, format helper.
- `internal/domain/command/list_test.go` (new) — black-box tests in `package command_test`, fake `WorkerLister`, reuses `fakeSender` / `makeReplyTo` from existing test files.
- `internal/infra/i18n/messages.go` (modify) — add `ListCommand ListCommandMessages` field to `RuntimeMessages`, define `ListCommandMessages` struct.
- `internal/infra/i18n/locales/zh.yaml` (modify) — append `list_command:` section under `runtime:`.
- `internal/infra/i18n/locales/en.yaml` (modify) — append `list_command:` section under `runtime:`.
- `internal/app/app.go` (modify) — construct `listCmdHandler` and append to `ChainHandlers`.
- `CHANGELOG.md` (modify) — add bullet under `[Unreleased] / Added`.

---

## Task 1: Add i18n schema and YAML entries

**Files:**
- Modify: `internal/infra/i18n/messages.go:295-348` (extend `RuntimeMessages`, add `ListCommandMessages` struct)
- Modify: `internal/infra/i18n/locales/zh.yaml:276` (append after `status_command:`)
- Modify: `internal/infra/i18n/locales/en.yaml:276` (append after `status_command:`)

- [ ] **Step 1: Add `ListCommand` field to `RuntimeMessages`**

In `internal/infra/i18n/messages.go`, find the `RuntimeMessages` struct (around line 289). Add a new field after `StatusCommand`:

```go
type RuntimeMessages struct {
    FailureNotifier FailureNotifierMessages   `yaml:"failure_notifier"`
    Feishu          FeishuRuntimeMessages     `yaml:"feishu"`
    WeCom           WeComRuntimeMessages      `yaml:"wecom"`
    RPC             RPCRuntimeMessages        `yaml:"rpc"`
    Department      DepartmentRuntimeMessages `yaml:"department"`
    EngineCommand   EngineCommandMessages     `yaml:"engine_command"`
    ClearCommand    ClearCommandMessages      `yaml:"clear_command"`
    StopCommand     StopCommandMessages       `yaml:"stop_command"`
    StatusCommand   StatusCommandMessages     `yaml:"status_command"`
    ListCommand     ListCommandMessages       `yaml:"list_command"`
}
```

- [ ] **Step 2: Define `ListCommandMessages` struct**

In the same file, after `StatusCommandMessages` (around line 348), add:

```go
// ListCommandMessages holds text sent to IM users by the /list command handler.
type ListCommandMessages struct {
    Usage         string `yaml:"usage"`
    LookupFailed  string `yaml:"lookup_failed"`
    HeaderAll     string `yaml:"header_all"`     // contains %d (count)
    HeaderSearch  string `yaml:"header_search"`  // contains %q, %d (keyword, count)
    EmptyAll      string `yaml:"empty_all"`
    EmptySearch   string `yaml:"empty_search"`
    Line          string `yaml:"line"`           // contains %s, %s, %s (name, status, description)
    StatusIdle    string `yaml:"status_idle"`
    StatusWorking string `yaml:"status_working"`
    StatusError   string `yaml:"status_error"`
}
```

- [ ] **Step 3: Add zh.yaml entries**

Append to `internal/infra/i18n/locales/zh.yaml` at end of file (after `status_command:` block, indented under `runtime:`):

```yaml
  list_command:
    usage: "用法：/list [关键词]"
    lookup_failed: "⚠️ 查询员工列表失败，请稍后重试；若持续出现请检查服务端日志。"
    header_all: "员工列表（共 %d 个）："
    header_search: "匹配 %q 的员工（共 %d 个）："
    empty_all: "  (暂无员工)"
    empty_search: "  (无匹配的员工)"
    line: "  - %s   状态: %s   %s"
    status_idle: "空闲"
    status_working: "工作中"
    status_error: "异常"
```

- [ ] **Step 4: Add en.yaml entries**

Append to `internal/infra/i18n/locales/en.yaml` at end of file (after `status_command:` block, indented under `runtime:`):

```yaml
  list_command:
    usage: "Usage: /list [keyword]"
    lookup_failed: "⚠️ Failed to list workers. Please retry; if this persists, check server logs."
    header_all: "Workers (%d):"
    header_search: "Workers matching %q (%d):"
    empty_all: "  (no workers)"
    empty_search: "  (no matches)"
    line: "  - %s   status: %s   %s"
    status_idle: "idle"
    status_working: "working"
    status_error: "error"
```

- [ ] **Step 5: Verify build still compiles**

Run: `go build ./...`
Expected: no output (success). Any compile error means the struct field name and YAML tag don't line up — re-check Step 1/2.

- [ ] **Step 6: Verify i18n loads**

Run: `go test ./internal/infra/i18n/...`
Expected: PASS (existing i18n tests verify YAML parses cleanly).

- [ ] **Step 7: Commit**

```bash
git add internal/infra/i18n/messages.go internal/infra/i18n/locales/zh.yaml internal/infra/i18n/locales/en.yaml
git commit -m "feat(i18n): add list_command messages"
```

---

## Task 2: Write failing tests for `ListCommandHandler`

**Files:**
- Create: `internal/domain/command/list_test.go`

We use TDD: write the full test file first against types that don't yet exist, confirm it fails to compile, then build the handler in Task 3 until tests pass.

- [ ] **Step 1: Create the test file**

Create `internal/domain/command/list_test.go`:

```go
package command_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/theopenbee/openbee/internal/domain/command"
	"github.com/theopenbee/openbee/internal/infra/i18n"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/platform"
)

type fakeWorkerLister struct {
	workers []model.Worker
	err     error
}

func (f *fakeWorkerLister) List() ([]model.Worker, error) {
	return f.workers, f.err
}

func makeListHandler(workers []model.Worker, err error) (*command.ListCommandHandler, *fakeSender) {
	sender := &fakeSender{}
	senders := map[string]platform.PlatformSenderAdapter{"feishu": sender}
	lister := &fakeWorkerLister{workers: workers, err: err}
	return command.NewListCommandHandler(lister, senders), sender
}

func TestListCommand_IsCommand(t *testing.T) {
	h, _ := makeListHandler(nil, nil)
	cases := map[string]bool{
		"/list":         true,
		"/list keyword": true,
		"/listfoo":      false,
		"hello":         false,
		"":              false,
	}
	for input, want := range cases {
		if got := h.IsCommand(input); got != want {
			t.Errorf("IsCommand(%q) = %v, want %v", input, got, want)
		}
	}
}

func TestListCommand_UsageOnExtraArgs(t *testing.T) {
	h, sender := makeListHandler(nil, nil)
	handled := h.HandleCommand(context.Background(), "/list a b", makeReplyTo())
	if !handled {
		t.Fatal("expected handled=true")
	}
	if len(sender.sent) != 1 || sender.sent[0] != i18n.M.Runtime.ListCommand.Usage {
		t.Errorf("expected usage reply, got %v", sender.sent)
	}
}

func TestListCommand_EmptyDirectory(t *testing.T) {
	h, sender := makeListHandler(nil, nil)
	handled := h.HandleCommand(context.Background(), "/list", makeReplyTo())
	if !handled {
		t.Fatal("expected handled=true")
	}
	if len(sender.sent) != 1 {
		t.Fatalf("expected 1 reply, got %d", len(sender.sent))
	}
	out := sender.sent[0]
	for _, want := range []string{"员工列表（共 0 个）：", "  (暂无员工)"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\n--- output ---\n%s", want, out)
		}
	}
}

func TestListCommand_AllWorkersSortedByName(t *testing.T) {
	workers := []model.Worker{
		{ID: "w1", Name: "张三", Description: "前端开发", Status: model.WorkerStatusWorking},
		{ID: "w2", Name: "李四", Description: "后端开发", Status: model.WorkerStatusError},
		{ID: "w3", Name: "小乔", Description: "负责 openbee 开发", Status: model.WorkerStatusIdle},
	}
	h, sender := makeListHandler(workers, nil)
	h.HandleCommand(context.Background(), "/list", makeReplyTo())
	out := sender.sent[0]

	if !strings.Contains(out, "员工列表（共 3 个）：") {
		t.Errorf("missing header\n%s", out)
	}
	// expected sort: 小乔 < 张三 < 李四 (by Go's default string < on UTF-8 bytes)
	idxXiao := strings.Index(out, "小乔")
	idxZhang := strings.Index(out, "张三")
	idxLi := strings.Index(out, "李四")
	if idxXiao < 0 || idxZhang < 0 || idxLi < 0 {
		t.Fatalf("missing one of the worker names:\n%s", out)
	}
	if !(idxXiao < idxZhang && idxZhang < idxLi) {
		t.Errorf("workers not in expected sort order; got positions xiao=%d zhang=%d li=%d\n%s",
			idxXiao, idxZhang, idxLi, out)
	}
	for _, want := range []string{
		"  - 小乔   状态: 空闲   负责 openbee 开发",
		"  - 张三   状态: 工作中   前端开发",
		"  - 李四   状态: 异常   后端开发",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing line %q\n%s", want, out)
		}
	}
}

func TestListCommand_KeywordSubstringMatch(t *testing.T) {
	workers := []model.Worker{
		{ID: "w1", Name: "alice", Description: "frontend dev", Status: model.WorkerStatusIdle},
		{ID: "w2", Name: "bob", Description: "openbee backend", Status: model.WorkerStatusIdle},
		{ID: "w3", Name: "carol", Description: "QA on openbee", Status: model.WorkerStatusIdle},
	}
	h, sender := makeListHandler(workers, nil)
	h.HandleCommand(context.Background(), "/list openbee", makeReplyTo())
	out := sender.sent[0]

	if !strings.Contains(out, `匹配 "openbee" 的员工（共 2 个）：`) {
		t.Errorf("missing search header\n%s", out)
	}
	if strings.Contains(out, "alice") {
		t.Errorf("alice should be filtered out\n%s", out)
	}
	if !strings.Contains(out, "bob") || !strings.Contains(out, "carol") {
		t.Errorf("expected bob and carol\n%s", out)
	}
}

func TestListCommand_KeywordCaseInsensitive(t *testing.T) {
	workers := []model.Worker{
		{ID: "w1", Name: "alice", Description: "openbee maintainer", Status: model.WorkerStatusIdle},
	}
	h, sender := makeListHandler(workers, nil)
	h.HandleCommand(context.Background(), "/list OPENBEE", makeReplyTo())
	out := sender.sent[0]
	if !strings.Contains(out, "alice") {
		t.Errorf("expected case-insensitive match for OPENBEE\n%s", out)
	}
}

func TestListCommand_KeywordNoMatch(t *testing.T) {
	workers := []model.Worker{
		{ID: "w1", Name: "alice", Description: "frontend", Status: model.WorkerStatusIdle},
	}
	h, sender := makeListHandler(workers, nil)
	h.HandleCommand(context.Background(), "/list zzznope", makeReplyTo())
	out := sender.sent[0]
	for _, want := range []string{`匹配 "zzznope" 的员工（共 0 个）：`, "  (无匹配的员工)"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q\n%s", want, out)
		}
	}
}

func TestListCommand_LookupError(t *testing.T) {
	h, sender := makeListHandler(nil, errors.New("boom"))
	handled := h.HandleCommand(context.Background(), "/list", makeReplyTo())
	if !handled {
		t.Fatal("expected handled=true")
	}
	if len(sender.sent) != 1 {
		t.Fatalf("expected 1 reply, got %d", len(sender.sent))
	}
	if sender.sent[0] != i18n.M.Runtime.ListCommand.LookupFailed {
		t.Errorf("expected lookup_failed reply, got %q", sender.sent[0])
	}
}

func TestListCommand_StatusLabels(t *testing.T) {
	workers := []model.Worker{
		{ID: "w1", Name: "a", Description: "x", Status: model.WorkerStatusIdle},
		{ID: "w2", Name: "b", Description: "x", Status: model.WorkerStatusWorking},
		{ID: "w3", Name: "c", Description: "x", Status: model.WorkerStatusError},
	}
	h, sender := makeListHandler(workers, nil)
	h.HandleCommand(context.Background(), "/list", makeReplyTo())
	out := sender.sent[0]
	for _, want := range []string{"空闲", "工作中", "异常"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing status label %q\n%s", want, out)
		}
	}
}

func TestListCommand_UnknownStatusFallsBack(t *testing.T) {
	workers := []model.Worker{
		{ID: "w1", Name: "a", Description: "x", Status: model.WorkerStatus("paused")},
	}
	h, sender := makeListHandler(workers, nil)
	h.HandleCommand(context.Background(), "/list", makeReplyTo())
	out := sender.sent[0]
	if !strings.Contains(out, "paused") {
		t.Errorf("expected unknown status to fall through verbatim\n%s", out)
	}
}
```

- [ ] **Step 2: Run tests and confirm they fail to compile**

Run: `go test ./internal/domain/command/ -run TestListCommand -v`
Expected: build error mentioning `undefined: command.ListCommandHandler` and `undefined: command.NewListCommandHandler`. This is the failing-test baseline.

- [ ] **Step 3: Commit (test-only commit)**

```bash
git add internal/domain/command/list_test.go
git commit -m "test: failing tests for /list command"
```

---

## Task 3: Implement `ListCommandHandler`

**Files:**
- Create: `internal/domain/command/list.go`

- [ ] **Step 1: Write the handler**

Create `internal/domain/command/list.go`:

```go
package command

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"go.uber.org/zap"

	"github.com/theopenbee/openbee/internal/infra/i18n"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/platform"
)

// CmdList is the slash command that prints the worker directory.
const CmdList = "/list"

// WorkerLister is the subset of WorkerStore needed by ListCommandHandler.
type WorkerLister interface {
	List() ([]model.Worker, error)
}

// ListCommandHandler handles the /list slash command.
type ListCommandHandler struct {
	workers WorkerLister
	senders map[string]platform.PlatformSenderAdapter
}

func NewListCommandHandler(workers WorkerLister, senders map[string]platform.PlatformSenderAdapter) *ListCommandHandler {
	return &ListCommandHandler{workers: workers, senders: senders}
}

func (h *ListCommandHandler) IsCommand(content string) bool {
	return isExactOrPrefixed(content, CmdList)
}

func (h *ListCommandHandler) HandleCommand(ctx context.Context, content string, replyTo platform.InboundMessage) bool {
	fields := strings.Fields(content)
	if len(fields) == 0 || fields[0] != CmdList {
		return false
	}
	if len(fields) > 2 {
		h.reply(ctx, replyTo, i18n.M.Runtime.ListCommand.Usage)
		return true
	}

	keyword := ""
	if len(fields) == 2 {
		keyword = fields[1]
	}

	workers, err := h.workers.List()
	if err != nil {
		log.Error("list workers for /list", zap.Error(err))
		h.reply(ctx, replyTo, i18n.M.Runtime.ListCommand.LookupFailed)
		return true
	}

	if keyword != "" {
		kw := strings.ToLower(keyword)
		filtered := workers[:0:0]
		for _, w := range workers {
			if strings.Contains(strings.ToLower(w.Description), kw) {
				filtered = append(filtered, w)
			}
		}
		workers = filtered
	}

	sort.SliceStable(workers, func(i, j int) bool { return workers[i].Name < workers[j].Name })
	h.reply(ctx, replyTo, formatList(keyword, workers))
	return true
}

func formatList(keyword string, workers []model.Worker) string {
	m := i18n.M.Runtime.ListCommand
	lines := make([]string, 0, len(workers)+2)
	if keyword == "" {
		lines = append(lines, fmt.Sprintf(m.HeaderAll, len(workers)))
		if len(workers) == 0 {
			lines = append(lines, m.EmptyAll)
		}
	} else {
		lines = append(lines, fmt.Sprintf(m.HeaderSearch, keyword, len(workers)))
		if len(workers) == 0 {
			lines = append(lines, m.EmptySearch)
		}
	}
	for _, w := range workers {
		lines = append(lines, fmt.Sprintf(m.Line, w.Name, statusLabel(w.Status), w.Description))
	}
	return strings.Join(lines, "\n")
}

func statusLabel(s model.WorkerStatus) string {
	m := i18n.M.Runtime.ListCommand
	switch s {
	case model.WorkerStatusIdle:
		return m.StatusIdle
	case model.WorkerStatusWorking:
		return m.StatusWorking
	case model.WorkerStatusError:
		return m.StatusError
	default:
		return string(s)
	}
}

func (h *ListCommandHandler) reply(ctx context.Context, replyTo platform.InboundMessage, text string) {
	sendReply(ctx, h.senders, replyTo, text)
}
```

Notes:
- `log` and `isExactOrPrefixed` are package-level in `engine.go`, already in scope.
- `sendReply` is also in `engine.go` and shared by all handlers.

- [ ] **Step 2: Run the tests**

Run: `go test ./internal/domain/command/ -run TestListCommand -v`
Expected: all 9 `TestListCommand_*` cases PASS.

- [ ] **Step 3: Run the full command package tests to check for regressions**

Run: `go test ./internal/domain/command/...`
Expected: all PASS.

- [ ] **Step 4: Run go vet**

Run: `go vet ./internal/domain/command/...`
Expected: no output.

- [ ] **Step 5: Commit**

```bash
git add internal/domain/command/list.go
git commit -m "feat: add ListCommandHandler for /list command"
```

---

## Task 4: Wire the handler into the chain

**Files:**
- Modify: `internal/app/app.go:170-171`

- [ ] **Step 1: Add the handler construction and chain entry**

In `internal/app/app.go`, replace the block at lines 170-171 with:

```go
		statusCmdHandler := command.NewStatusCommandHandler(s.sessionStore, s.taskStore, s.workerStore, sendersByPlatform, engineCfg)
		listCmdHandler := command.NewListCommandHandler(s.workerStore, sendersByPlatform)
		cmdChain := msgingest.ChainHandlers(engineCmdHandler, clearCmdHandler, stopCmdHandler, statusCmdHandler, listCmdHandler)
```

- [ ] **Step 2: Verify the build still compiles**

Run: `go build ./...`
Expected: no output.

- [ ] **Step 3: Run the full test suite to confirm no regressions**

Run: `go test ./...`
Expected: all PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/app/app.go
git commit -m "feat: wire /list command into msgingest chain"
```

---

## Task 5: Update CHANGELOG

**Files:**
- Modify: `CHANGELOG.md:3-6`

- [ ] **Step 1: Add an `Added` section under `[Unreleased]` for the new command**

In `CHANGELOG.md`, replace lines 3-6 with:

```markdown
## [Unreleased]

### Added
- Add `/list [keyword]` command to print all workers with status and description; keyword performs case-insensitive substring search on `worker.description`.

### Changed
- Enhance `/clear` command output with more detailed information
```

(English content per project rule: changelog must be written in English.)

- [ ] **Step 2: Commit**

```bash
git add CHANGELOG.md
git commit -m "docs: changelog entry for /list command"
```

---

## Task 6: Manual end-to-end smoke (optional but recommended)

This task is informational — it is not automated. Use it to confirm the command behaves correctly in a running bee against a real IM platform.

**Files:** none (manual verification)

- [ ] **Step 1: Start the bee locally**

Run: `make run` (or the project's usual local-run command — check `Makefile`).
Expected: server boots, IM platform connected.

- [ ] **Step 2: Send `/list` from your IM client**

Expected reply (assuming you have at least one worker named "小乔"):
```
员工列表（共 N 个）：
  - 小乔   状态: 空闲   负责 openbee 开发
  ...
```

- [ ] **Step 3: Send `/list openbee`**

Expected reply contains only workers whose description includes "openbee" (case-insensitive) with the search-form header.

- [ ] **Step 4: Send `/list nonexistent-keyword`**

Expected reply:
```
匹配 "nonexistent-keyword" 的员工（共 0 个）：
  (无匹配的员工)
```

- [ ] **Step 5: Send `/list a b c`**

Expected reply: `用法：/list [关键词]`

---

## Verification Checklist

Before declaring done:

- [ ] `go build ./...` succeeds
- [ ] `go test ./...` all pass
- [ ] `go vet ./...` clean
- [ ] All 9 `TestListCommand_*` cases pass
- [ ] Spec file at `docs/superpowers/specs/2026-05-11-list-command-design.md` exists and matches implementation
- [ ] Changelog updated under `[Unreleased] / Added`
- [ ] Commits are small and focused (one per task)
