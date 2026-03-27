# Failure Notification Improvement Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the flat `[系统通知] 任务执行失败：<reason>` notification with a structured message that shows Worker name, retry count, and raw error so users can judge whether to retry.

**Architecture:** Add `model.FailureInfo` struct as the shared carrier. Update both `FailureNotifier` interfaces (in `task_dispatcher` and `bee` packages) to accept `FailureInfo` instead of a plain reason string. Update `PlatformFailureNotifier` to format the new message, then thread actual error strings through all call sites in `dispatcher.go` and `feeder.go`.

**Tech Stack:** Go, SQLite (via `store`), `go.uber.org/zap`

---

## File Map

| Action | File | What changes |
|--------|------|-------------|
| Modify | `internal/model/execution.go` | Add `FailureInfo` struct |
| Modify | `internal/task_dispatcher/failure_notifier.go` | New `NotifyTaskFailure` signature + format |
| Modify | `internal/task_dispatcher/dispatcher.go` | Update `FailureNotifier` interface, `notifyFailure` helper, 2 call sites |
| Modify | `internal/task_dispatcher/failure_notifier_test.go` | Update existing tests + add format coverage |
| Modify | `internal/task_dispatcher/dispatcher_test.go` | Update `mockFailureNotifier` |
| Modify | `internal/bee/feeder.go` | Update `FailureNotifier` interface, `rollback()`, all 5 call sites |
| Modify | `internal/bee/feeder_test.go` | Update `mockFailureNotifier` |

---

## Task 1: Add `model.FailureInfo`

**Files:**
- Modify: `internal/model/execution.go`

No test needed — this is a plain data struct with no logic.

- [ ] **Step 1: Add `FailureInfo` to `internal/model/execution.go`**

Append after the `WorkerExecution` struct (after line 24):

```go
// FailureInfo carries context for a task failure notification sent to the user.
type FailureInfo struct {
	Reason     string // raw error (exec.Result or err.Error())
	WorkerName string // worker or bee name for identification
	RetryCount int    // retries attempted; -1 means no retry mechanism (omit retry line)
	MaxRetries int    // max retry limit; ignored when RetryCount < 0
}
```

- [ ] **Step 2: Verify the package compiles**

```bash
go build ./internal/model/...
```

Expected: no output (success)

- [ ] **Step 3: Commit**

```bash
git add internal/model/execution.go
git commit -m "feat: add model.FailureInfo for structured failure notifications"
```

---

## Task 2: Update `task_dispatcher` package

Updates the `PlatformFailureNotifier` implementation, the `FailureNotifier` interface, the `notifyFailure` helper, and all call sites in `dispatcher.go`. All changes must land together for the package to compile.

**Files:**
- Modify: `internal/task_dispatcher/failure_notifier.go`
- Modify: `internal/task_dispatcher/dispatcher.go`
- Test: `internal/task_dispatcher/failure_notifier_test.go`
- Test: `internal/task_dispatcher/dispatcher_test.go`

- [ ] **Step 1: Update `failure_notifier_test.go` — existing tests use new signature**

Replace the four existing test calls to `NotifyTaskFailure(ctx, msgID, string)` with the new `FailureInfo` form. Replace the full file content:

```go
package task_dispatcher_test

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/theopenbee/openbee/internal/model"
	"github.com/theopenbee/openbee/internal/platform"
	"github.com/theopenbee/openbee/internal/store"
	"github.com/theopenbee/openbee/internal/task_dispatcher"
)

// --- helpers ---

type spySender struct {
	mu   sync.Mutex
	sent []platform.OutboundMessage
}

func (s *spySender) Send(_ context.Context, msg platform.OutboundMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sent = append(s.sent, msg)
	return nil
}

func setupNotifier(t *testing.T, platformID string) (*task_dispatcher.PlatformFailureNotifier, *store.MessageStore, *spySender) {
	t.Helper()
	db, err := store.InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	ms := store.NewMessageStore(db)
	sender := &spySender{}
	senders := map[string]platform.PlatformSenderAdapter{platformID: sender}
	notifier := task_dispatcher.NewPlatformFailureNotifier(ms, senders)
	return notifier, ms, sender
}

// --- tests ---

func TestPlatformFailureNotifier_Success(t *testing.T) {
	notifier, ms, sender := setupNotifier(t, "test")
	ctx := context.Background()

	_, err := ms.Create(ctx, "msg-1", "sess-1", "test", "hello", `{"raw":true}`, "", 0)
	if err != nil {
		t.Fatalf("create message: %v", err)
	}

	info := model.FailureInfo{
		Reason:     "API Error: content filtered",
		WorkerName: "my-worker",
		RetryCount: -1,
	}
	err = notifier.NotifyTaskFailure(ctx, "msg-1", info)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	sender.mu.Lock()
	defer sender.mu.Unlock()
	if len(sender.sent) != 1 {
		t.Fatalf("expected 1 message sent, got %d", len(sender.sent))
	}
	msg := sender.sent[0]
	if !strings.Contains(msg.Content, "任务执行失败") {
		t.Errorf("expected failure prefix, got: %s", msg.Content)
	}
	if !strings.Contains(msg.Content, "API Error: content filtered") {
		t.Errorf("expected reason in content, got: %s", msg.Content)
	}
	if msg.ReplyTo.Platform != "test" {
		t.Errorf("expected platform=test, got %s", msg.ReplyTo.Platform)
	}
}

func TestPlatformFailureNotifier_MessageNotFound(t *testing.T) {
	notifier, _, _ := setupNotifier(t, "test")
	ctx := context.Background()

	err := notifier.NotifyTaskFailure(ctx, "nonexistent-msg", model.FailureInfo{Reason: "some error", RetryCount: -1})
	if err == nil {
		t.Fatal("expected error for nonexistent message, got nil")
	}
	if !strings.Contains(err.Error(), "get message") {
		t.Errorf("expected 'get message' in error, got: %v", err)
	}
}

func TestPlatformFailureNotifier_UnknownPlatform(t *testing.T) {
	notifier, ms, _ := setupNotifier(t, "feishu")
	ctx := context.Background()

	_, err := ms.Create(ctx, "msg-2", "sess-2", "dingtalk", "hi", `{}`, "", 0)
	if err != nil {
		t.Fatalf("create message: %v", err)
	}

	err = notifier.NotifyTaskFailure(ctx, "msg-2", model.FailureInfo{Reason: "boom", RetryCount: -1})
	if err == nil {
		t.Fatal("expected error for unknown platform, got nil")
	}
	if !strings.Contains(err.Error(), "no sender for platform") {
		t.Errorf("expected 'no sender for platform' in error, got: %v", err)
	}
}

func TestPlatformFailureNotifier_TruncatesLongMessage(t *testing.T) {
	notifier, ms, sender := setupNotifier(t, "test")
	ctx := context.Background()

	_, err := ms.Create(ctx, "msg-3", "sess-3", "test", "hi", `{}`, "", 0)
	if err != nil {
		t.Fatalf("create message: %v", err)
	}

	longReason := strings.Repeat("错误", 300) // 600 Chinese chars
	info := model.FailureInfo{
		Reason:     longReason,
		WorkerName: "w",
		RetryCount: -1,
	}
	err = notifier.NotifyTaskFailure(ctx, "msg-3", info)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	sender.mu.Lock()
	defer sender.mu.Unlock()
	if len(sender.sent) != 1 {
		t.Fatalf("expected 1 message sent, got %d", len(sender.sent))
	}

	content := sender.sent[0].Content
	runes := []rune(content)
	if len(runes) > 500 {
		t.Errorf("expected content truncated to <= 500 runes, got %d runes", len(runes))
	}
	if !strings.HasSuffix(content, "…") {
		t.Errorf("expected truncated content to end with '…', got: %s", content[len(content)-10:])
	}
}

func TestPlatformFailureNotifier_StructuredFormat_WithRetry(t *testing.T) {
	notifier, ms, sender := setupNotifier(t, "test")
	ctx := context.Background()

	_, err := ms.Create(ctx, "msg-4", "sess-4", "test", "hi", `{}`, "", 0)
	if err != nil {
		t.Fatalf("create message: %v", err)
	}

	info := model.FailureInfo{
		Reason:     "exit status 1",
		WorkerName: "数据分析助手",
		RetryCount: 3,
		MaxRetries: 3,
	}
	if err := notifier.NotifyTaskFailure(ctx, "msg-4", info); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	sender.mu.Lock()
	defer sender.mu.Unlock()
	content := sender.sent[0].Content
	if !strings.Contains(content, "数据分析助手") {
		t.Errorf("expected WorkerName in content, got: %s", content)
	}
	if !strings.Contains(content, "已重试：3/3 次") {
		t.Errorf("expected retry line in content, got: %s", content)
	}
	if !strings.Contains(content, "exit status 1") {
		t.Errorf("expected Reason in content, got: %s", content)
	}
}

func TestPlatformFailureNotifier_StructuredFormat_NoRetry(t *testing.T) {
	notifier, ms, sender := setupNotifier(t, "test")
	ctx := context.Background()

	_, err := ms.Create(ctx, "msg-5", "sess-5", "test", "hi", `{}`, "", 0)
	if err != nil {
		t.Fatalf("create message: %v", err)
	}

	info := model.FailureInfo{
		Reason:     "launch failed",
		WorkerName: "worker-abc",
		RetryCount: -1,
	}
	if err := notifier.NotifyTaskFailure(ctx, "msg-5", info); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	sender.mu.Lock()
	defer sender.mu.Unlock()
	content := sender.sent[0].Content
	if strings.Contains(content, "已重试") {
		t.Errorf("expected no retry line when RetryCount=-1, got: %s", content)
	}
	if !strings.Contains(content, "worker-abc") {
		t.Errorf("expected WorkerName in content, got: %s", content)
	}
}
```

- [ ] **Step 2: Run tests to confirm they fail**

```bash
go test ./internal/task_dispatcher/... -run TestPlatformFailureNotifier -v
```

Expected: compilation error — `NotifyTaskFailure` still takes `(ctx, string, string)`

- [ ] **Step 3: Update `failure_notifier.go` — new signature and format**

Replace the full file:

```go
package task_dispatcher

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/theopenbee/openbee/internal/model"
	"github.com/theopenbee/openbee/internal/platform"
	"github.com/theopenbee/openbee/internal/store"
)

// PlatformFailureNotifier implements FailureNotifier by looking up the originating
// message and sending a failure notification via the appropriate platform sender.
type PlatformFailureNotifier struct {
	msgStore *store.MessageStore
	senders  map[string]platform.PlatformSenderAdapter
}

// NewPlatformFailureNotifier creates a PlatformFailureNotifier.
func NewPlatformFailureNotifier(msgStore *store.MessageStore, senders map[string]platform.PlatformSenderAdapter) *PlatformFailureNotifier {
	return &PlatformFailureNotifier{msgStore: msgStore, senders: senders}
}

func (n *PlatformFailureNotifier) NotifyTaskFailure(ctx context.Context, messageID string, info model.FailureInfo) error {
	stored, err := n.msgStore.GetByID(ctx, messageID)
	if err != nil {
		return fmt.Errorf("get message for failure notification: %w", err)
	}

	sender, ok := n.senders[stored.Platform]
	if !ok {
		return fmt.Errorf("no sender for platform %q", stored.Platform)
	}

	var content string
	if info.RetryCount >= 0 {
		content = fmt.Sprintf("❌ 任务执行失败\nWorker：%s\n已重试：%d/%d 次\n错误：%s",
			info.WorkerName, info.RetryCount, info.MaxRetries, info.Reason)
	} else {
		content = fmt.Sprintf("❌ 任务执行失败\nWorker：%s\n错误：%s",
			info.WorkerName, info.Reason)
	}
	// Truncate very long error messages to avoid exceeding platform limits.
	// Use rune slice to avoid splitting multi-byte UTF-8 characters.
	const maxRunes = 500
	runes := []rune(content)
	if len(runes) > maxRunes {
		content = string(runes[:maxRunes-1]) + "…"
	}

	outbound := platform.OutboundMessage{
		Content: content,
		ReplyTo: platform.InboundMessage{
			Platform:   stored.Platform,
			SessionKey: stored.SessionKey,
			Raw:        stored.Raw,
		},
	}
	if err := sender.Send(ctx, outbound); err != nil {
		log.Error("send failure notification", zap.String("messageID", messageID), zap.Error(err))
		return fmt.Errorf("send failure notification: %w", err)
	}
	return nil
}
```

- [ ] **Step 4: Update `dispatcher.go` — interface, `notifyFailure` helper, and 2 call sites**

Four changes in `internal/task_dispatcher/dispatcher.go`:

**4a.** Update the `FailureNotifier` interface (lines 40-42). Replace:
```go
// FailureNotifier sends failure notifications to users when a worker execution
// fails at the system level (e.g. API error, content filtering) and the worker
// itself had no chance to call send_message.
type FailureNotifier interface {
	NotifyTaskFailure(ctx context.Context, messageID, reason string) error
}
```
With:
```go
// FailureNotifier sends failure notifications to users when a worker execution
// fails at the system level (e.g. API error, content filtering) and the worker
// itself had no chance to call send_message.
type FailureNotifier interface {
	NotifyTaskFailure(ctx context.Context, messageID string, info model.FailureInfo) error
}
```

**4b.** Update call site 1 (line 204 — launch failure). Replace:
```go
		d.notifyFailure(ctx, task.MessageID, err.Error())
```
With:
```go
		d.notifyFailure(ctx, task.MessageID, model.FailureInfo{
			Reason:     err.Error(),
			WorkerName: task.WorkerID,
			RetryCount: -1,
		})
```

**4c.** Update call site 2 (line 270 — abnormal exit). Replace:
```go
		d.notifyFailure(ctx, task.MessageID, exec.Result)
```
With:
```go
		d.notifyFailure(ctx, task.MessageID, model.FailureInfo{
			Reason:     exec.Result,
			WorkerName: exec.WorkerName,
			RetryCount: -1,
		})
```

**4d.** Update `notifyFailure` helper (lines 281-288). Replace:
```go
func (d *TaskDispatcher) notifyFailure(ctx context.Context, messageID, reason string) {
	if d.failureNotifier == nil || messageID == "" {
		return
	}
	if err := d.failureNotifier.NotifyTaskFailure(ctx, messageID, reason); err != nil {
		log.Error("notify task failure", zap.String("messageID", messageID), zap.Error(err))
	}
}
```
With:
```go
func (d *TaskDispatcher) notifyFailure(ctx context.Context, messageID string, info model.FailureInfo) {
	if d.failureNotifier == nil || messageID == "" {
		return
	}
	if err := d.failureNotifier.NotifyTaskFailure(ctx, messageID, info); err != nil {
		log.Error("notify task failure", zap.String("messageID", messageID), zap.Error(err))
	}
}
```

Add `"github.com/theopenbee/openbee/internal/model"` to the import block in `dispatcher.go` if not already present (it already is — line 12).

- [ ] **Step 5: Update `dispatcher_test.go` — update `mockFailureNotifier`**

In `internal/task_dispatcher/dispatcher_test.go`, replace the `failureCall` struct and `mockFailureNotifier.NotifyTaskFailure` method (lines 85-100):

```go
type mockFailureNotifier struct {
	mu    sync.Mutex
	calls []failureCall
}

type failureCall struct {
	messageID string
	info      model.FailureInfo
}

func (n *mockFailureNotifier) NotifyTaskFailure(_ context.Context, messageID string, info model.FailureInfo) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.calls = append(n.calls, failureCall{messageID: messageID, info: info})
	return nil
}
```

Also check the two test assertions that access `fn.calls[0].reason` (lines ~577 and ~693) and update them to `fn.calls[0].info.Reason` if present. Search the file:

```bash
grep -n "\.reason" internal/task_dispatcher/dispatcher_test.go
```

If any hits, change `.reason` → `.info.Reason`.

- [ ] **Step 6: Run all `task_dispatcher` tests**

```bash
go test ./internal/task_dispatcher/... -v
```

Expected: all tests PASS

- [ ] **Step 7: Commit**

```bash
git add internal/task_dispatcher/failure_notifier.go \
        internal/task_dispatcher/dispatcher.go \
        internal/task_dispatcher/failure_notifier_test.go \
        internal/task_dispatcher/dispatcher_test.go
git commit -m "feat: update task_dispatcher FailureNotifier to use model.FailureInfo"
```

---

## Task 3: Update `bee` package

Updates `bee.FailureNotifier` interface, `rollback()` internals (thread real errors through instead of hardcoded strings), and the `feeder_test.go` mock.

**Files:**
- Modify: `internal/bee/feeder.go`
- Test: `internal/bee/feeder_test.go`

- [ ] **Step 1: Update `feeder_test.go` — update `mockFailureNotifier`**

In `internal/bee/feeder_test.go`, replace the `mockFailureNotifier` struct and its `NotifyTaskFailure` method (lines 328-338):

```go
type mockFailureNotifier struct {
	mu   sync.Mutex
	msgs []string
}

func (m *mockFailureNotifier) NotifyTaskFailure(_ context.Context, messageID string, _ model.FailureInfo) error {
	m.mu.Lock()
	m.msgs = append(m.msgs, messageID)
	m.mu.Unlock()
	return nil
}
```

Add `"github.com/theopenbee/openbee/internal/model"` to the import block in `feeder_test.go`.

- [ ] **Step 2: Run tests to confirm they fail**

```bash
go test ./internal/bee/... -run TestFeeder_ExhaustsRetries -v
```

Expected: compilation error — `bee.FailureNotifier` interface still has old signature

- [ ] **Step 3: Update `feeder.go` — interface, `rollback()`, and all call sites**

**3a.** Update `bee.FailureNotifier` interface (lines 27-29). Replace:
```go
// FailureNotifier sends a notification to the user when a message is permanently failed.
type FailureNotifier interface {
	NotifyTaskFailure(ctx context.Context, messageID, reason string) error
}
```
With:
```go
// FailureNotifier sends a notification to the user when a message is permanently failed.
type FailureNotifier interface {
	NotifyTaskFailure(ctx context.Context, messageID string, info model.FailureInfo) error
}
```

**3b.** Restructure `rollback()` to track full messages (not just IDs) for failed entries, and build `model.FailureInfo` per message. Replace the entire `rollback` function (lines 243-267):

```go
func (f *Feeder) rollback(ctx context.Context, msgs []store.ClaimedMessage, reason string) {
	ids := make([]string, len(msgs))
	var failedMsgs []store.ClaimedMessage
	for i, m := range msgs {
		ids[i] = m.ID
		if m.RetryCount+1 >= MaxRetries {
			failedMsgs = append(failedMsgs, m)
		}
	}
	if err := f.taskStore.DeletePendingByMessageIDs(ctx, ids); err != nil {
		log.Error("rollback delete tasks", zap.Error(err))
	}
	if err := f.msgStore.RollbackWithRetry(ctx, ids, MaxRetries); err != nil {
		log.Error("rollback with retry", zap.Error(err))
		return
	}
	for _, m := range failedMsgs {
		log.Warn("message exhausted retries", zap.String("messageID", m.ID))
		if f.failureNotifier != nil {
			info := model.FailureInfo{
				Reason:     reason,
				WorkerName: m.SessionKey,
				RetryCount: m.RetryCount + 1,
				MaxRetries: MaxRetries,
			}
			if notifyErr := f.failureNotifier.NotifyTaskFailure(ctx, m.ID, info); notifyErr != nil {
				log.Error("notify bee failure", zap.String("messageID", m.ID), zap.Error(notifyErr))
			}
		}
	}
}
```

**3c.** Update the 6 `rollback` call sites to pass real error strings instead of hardcoded Chinese messages.

Replace line 125:
```go
		f.rollback(ctx, msgs, "内部错误：无法写入配置文件")
```
With:
```go
		f.rollback(ctx, msgs, err.Error())
```

Replace line 146:
```go
		f.rollback(ctx, msgs, "内部错误：无法读取会话上下文")
```
With:
```go
		f.rollback(ctx, msgs, err.Error())
```

Replace line 172:
```go
		f.rollback(ctx, msgs, "内部错误：无法创建执行记录")
```
With:
```go
		f.rollback(ctx, msgs, err.Error())
```

Replace line 180 (note: `err` here is from `PrepareLogPath`):
```go
		f.rollback(ctx, msgs, "内部错误：无法创建日志文件")
```
With:
```go
		f.rollback(ctx, msgs, err.Error())
```

Replace line 191 (runner.Run error — `err` is in scope from line 187):
```go
		f.rollback(ctx, msgs, "AI 处理失败，请稍后重试")
```
With:
```go
		f.rollback(ctx, msgs, err.Error())
```

Replace line 213 (drainErr path):
```go
		f.rollback(ctx, msgs, "AI 处理失败，请稍后重试")
```
With:
```go
		f.rollback(ctx, msgs, drainErr.Error())
```

- [ ] **Step 4: Run all `bee` tests**

```bash
go test ./internal/bee/... -v
```

Expected: all tests PASS

- [ ] **Step 5: Run full test suite to confirm no regressions**

```bash
go test ./...
```

Expected: all tests PASS

- [ ] **Step 6: Commit**

```bash
git add internal/bee/feeder.go internal/bee/feeder_test.go
git commit -m "feat: update bee FailureNotifier to use model.FailureInfo with real errors"
```
