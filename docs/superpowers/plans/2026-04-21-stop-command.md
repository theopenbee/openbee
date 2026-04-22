# `/stop` Command Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a `/stop` slash command that immediately terminates the running bee process for the current session and cancels all pending messages, while preserving session context.

**Architecture:** `FailReceived` runs first (while the bee is still `feeding`, preventing a `ClaimBatch` race), then `StopSession` cancels the running bee context. A new `StopCommandHandler` orchestrates both, sends per-message failure notifications via the existing notifier, then replies with a result summary. The Feeder gains an in-memory cancel registry (`map[string]context.CancelFunc`) guarded by a mutex.

**Tech Stack:** Go, SQLite, existing `command`/`store`/`bee` packages, i18n YAML.

---

## File Map

| Action | Path | Responsibility |
|--------|------|----------------|
| Modify | `internal/infra/i18n/messages.go` | Add `StopCommandMessages` struct + field |
| Modify | `internal/infra/i18n/locales/zh.yaml` | Chinese stop_command strings |
| Modify | `internal/infra/i18n/locales/en.yaml` | English stop_command strings |
| Modify | `internal/infra/store/message_store.go` | Add `FailReceived` method |
| Modify | `internal/infra/store/message_store_test.go` | Test `FailReceived` |
| Modify | `internal/domain/bee/feeder.go` | Add cancel registry + `StopSession` method |
| Create | `internal/domain/command/stop.go` | `StopCommandHandler` |
| Modify | `internal/domain/command/engine.go` | Add `CmdStop` constant |
| Create | `internal/domain/command/stop_test.go` | Tests for `StopCommandHandler` |
| Modify | `internal/app/app.go` | Wire `stopCmdHandler` into chain |

---

## Task 1: i18n — Add stop_command strings

**Files:**
- Modify: `internal/infra/i18n/messages.go`
- Modify: `internal/infra/i18n/locales/zh.yaml`
- Modify: `internal/infra/i18n/locales/en.yaml`

- [ ] **Step 1: Add `StopCommandMessages` struct and field to `messages.go`**

In `messages.go`, add the new struct after `ClearCommandMessages`:

```go
// StopCommandMessages holds text sent to IM users by the /stop command handler.
type StopCommandMessages struct {
	Stopped             string `yaml:"stopped"`              // bee was running, no pending messages
	StoppedWithMessages string `yaml:"stopped_with_messages"` // contains %d (cancelled count)
	CancelledMessages   string `yaml:"cancelled_messages"`   // bee not running; contains %d
	NothingToStop       string `yaml:"nothing_to_stop"`
}
```

In `RuntimeMessages`, add after `ClearCommand`:

```go
StopCommand StopCommandMessages `yaml:"stop_command"`
```

- [ ] **Step 2: Add Chinese strings to `zh.yaml`**

In `zh.yaml`, under `runtime:`, add after the `clear_command:` block:

```yaml
  stop_command:
    stopped: "✅ 已停止 bee 执行"
    stopped_with_messages: "✅ 已停止 bee 执行，取消了 %d 条待处理消息"
    cancelled_messages: "✅ 已取消 %d 条待处理消息"
    nothing_to_stop: "当前会话没有需要停止的内容"
```

- [ ] **Step 3: Add English strings to `en.yaml`**

In `en.yaml`, under `runtime:`, add after the `clear_command:` block:

```yaml
  stop_command:
    stopped: "✅ Stopped bee execution"
    stopped_with_messages: "✅ Stopped bee execution and cancelled %d pending message(s)"
    cancelled_messages: "✅ Cancelled %d pending message(s)"
    nothing_to_stop: "Nothing to stop in this session"
```

- [ ] **Step 4: Verify i18n loads without error**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee2
go build ./internal/infra/i18n/...
```

Expected: no errors.

- [ ] **Step 5: Commit**

```bash
git add internal/infra/i18n/messages.go internal/infra/i18n/locales/zh.yaml internal/infra/i18n/locales/en.yaml
git commit -m "feat(i18n): add stop_command strings"
```

---

## Task 2: MessageStore — Add `FailReceived`

**Files:**
- Modify: `internal/infra/store/message_store.go`
- Modify: `internal/infra/store/message_store_test.go`

- [ ] **Step 1: Write the failing test**

In `message_store_test.go`, add this test:

```go
func TestMessageStore_FailReceived(t *testing.T) {
	s := setupMessageStore(t)
	ctx := context.Background()

	// Insert: two received in session A, one received in session B, one feeding in session A
	insert := func(id, sessionKey, status string) {
		_, err := s.db.ExecContext(ctx,
			`INSERT INTO bee_platform_messages (id, session_key, platform, content, raw, received_at, status, created_at, updated_at)
             VALUES (?, ?, 'test', 'x', '', 0, ?, 0, 0)`,
			id, sessionKey, status)
		if err != nil {
			t.Fatalf("insert %s: %v", id, err)
		}
	}
	insert("msg-a1", "sessionA", MsgStatusReceived)
	insert("msg-a2", "sessionA", MsgStatusReceived)
	insert("msg-b1", "sessionB", MsgStatusReceived)
	insert("msg-a3", "sessionA", MsgStatusFeeding)

	ids, err := s.FailReceived(ctx, "sessionA")
	if err != nil {
		t.Fatalf("FailReceived: %v", err)
	}

	// Only received messages for sessionA are returned
	if len(ids) != 2 {
		t.Fatalf("expected 2 IDs, got %d: %v", len(ids), ids)
	}
	got := map[string]bool{ids[0]: true, ids[1]: true}
	if !got["msg-a1"] || !got["msg-a2"] {
		t.Errorf("expected msg-a1 and msg-a2, got %v", ids)
	}

	// Verify DB: msg-a1, msg-a2 are failed
	for _, id := range []string{"msg-a1", "msg-a2"} {
		var status string
		s.db.QueryRowContext(ctx, `SELECT status FROM bee_platform_messages WHERE id = ?`, id).Scan(&status)
		if status != MsgStatusFailed {
			t.Errorf("%s: want status=failed, got %q", id, status)
		}
	}

	// msg-b1 and msg-a3 are untouched
	for id, want := range map[string]string{"msg-b1": MsgStatusReceived, "msg-a3": MsgStatusFeeding} {
		var status string
		s.db.QueryRowContext(ctx, `SELECT status FROM bee_platform_messages WHERE id = ?`, id).Scan(&status)
		if status != want {
			t.Errorf("%s: want status=%s, got %q", id, want, status)
		}
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee2
go test ./internal/infra/store/ -run TestMessageStore_FailReceived -v
```

Expected: FAIL — `s.FailReceived undefined`.

- [ ] **Step 3: Implement `FailReceived` in `message_store.go`**

Add after `MarkFailed`:

```go
// FailReceived marks all 'received' messages for sessionKey as 'failed'.
// Returns the IDs of the affected messages.
func (s *MessageStore) FailReceived(ctx context.Context, sessionKey string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id FROM bee_platform_messages WHERE session_key = ? AND status = ?`,
		sessionKey, MsgStatusReceived)
	if err != nil {
		return nil, fmt.Errorf("select received: %w", err)
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, nil
	}
	if err := s.UpdateStatusBatch(ctx, ids, MsgStatusFailed); err != nil {
		return nil, fmt.Errorf("mark failed: %w", err)
	}
	return ids, nil
}
```

- [ ] **Step 4: Run the test to verify it passes**

```bash
go test ./internal/infra/store/ -run TestMessageStore_FailReceived -v
```

Expected: PASS.

- [ ] **Step 5: Run the full store test suite**

```bash
go test ./internal/infra/store/... -v 2>&1 | tail -20
```

Expected: all tests PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/infra/store/message_store.go internal/infra/store/message_store_test.go
git commit -m "feat(store): add MessageStore.FailReceived"
```

---

## Task 3: Feeder — Add cancel registry and `StopSession`

**Files:**
- Modify: `internal/domain/bee/feeder.go`

- [ ] **Step 1: Add `runningMu` and `running` fields to `Feeder`**

In `feeder.go`, update the `Feeder` struct. Add after `workerLookup`:

```go
runningMu sync.Mutex
running   map[string]context.CancelFunc
```

Add `"sync"` to the import block if not already present.

- [ ] **Step 2: Initialize `running` in `NewFeeder`**

In `NewFeeder`, update the struct literal to include:

```go
running: make(map[string]context.CancelFunc),
```

- [ ] **Step 3: Register and deregister cancel func in `processBeeGroup`**

In `processBeeGroup`, replace the existing:

```go
beeCtx, cancel := context.WithTimeout(ctx, f.cfg.Engine.Timeout.Bee)
defer cancel()
```

With:

```go
beeCtx, cancel := context.WithTimeout(ctx, f.cfg.Engine.Timeout.Bee)
f.runningMu.Lock()
f.running[sessionKey] = cancel
f.runningMu.Unlock()
defer func() {
	cancel()
	f.runningMu.Lock()
	delete(f.running, sessionKey)
	f.runningMu.Unlock()
}()
```

- [ ] **Step 4: Add `StopSession` method**

Add after `NewFeeder`:

```go
// StopSession cancels the running bee for sessionKey.
// Returns true if a bee was running and its context was cancelled.
func (f *Feeder) StopSession(sessionKey string) bool {
	f.runningMu.Lock()
	cancel, ok := f.running[sessionKey]
	f.runningMu.Unlock()
	if ok {
		cancel()
	}
	return ok
}
```

- [ ] **Step 5: Build to verify no compilation errors**

```bash
go build ./internal/domain/bee/...
```

Expected: no errors.

- [ ] **Step 6: Commit**

```bash
git add internal/domain/bee/feeder.go
git commit -m "feat(bee): add cancel registry and StopSession to Feeder"
```

---

## Task 4: Add `CmdStop` constant

**Files:**
- Modify: `internal/domain/command/engine.go`

- [ ] **Step 1: Add the constant**

In `engine.go`, add after `CmdClear`:

```go
// CmdStop is the slash command that stops the running bee and cancels pending messages.
CmdStop = "/stop"
```

- [ ] **Step 2: Build**

```bash
go build ./internal/domain/command/...
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add internal/domain/command/engine.go
git commit -m "feat(command): add CmdStop constant"
```

---

## Task 5: Implement `StopCommandHandler`

**Files:**
- Create: `internal/domain/command/stop.go`
- Create: `internal/domain/command/stop_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/domain/command/stop_test.go`:

```go
package command_test

import (
	"context"
	"testing"

	"github.com/theopenbee/openbee/internal/domain/command"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/platform"
)

// --- fakes for stop ---

type fakeBeeStopper struct {
	stopped    bool // whether StopSession was called
	wasRunning bool // what to return
}

func (f *fakeBeeStopper) StopSession(_ string) bool {
	f.stopped = true
	return f.wasRunning
}

type fakeStopMsgStore struct {
	ids []string
	err error
}

func (f *fakeStopMsgStore) FailReceived(_ context.Context, _ string) ([]string, error) {
	return f.ids, f.err
}

type fakeStopNotifier struct {
	notified []string
}

func (f *fakeStopNotifier) NotifyTaskFailure(_ context.Context, messageID string, _ model.FailureInfo) error {
	f.notified = append(f.notified, messageID)
	return nil
}

type fakeStopSender struct {
	sent []string
}

func (f *fakeStopSender) Send(_ context.Context, _ platform.OutboundMessage) error {
	f.sent = append(f.sent, "sent")
	return nil
}
func (f *fakeStopSender) SendReply(_ context.Context, _ platform.InboundMessage, msg platform.OutboundMessage) error {
	f.sent = append(f.sent, msg.Text)
	return nil
}

func makeStopReplyTo() platform.InboundMessage {
	return platform.InboundMessage{
		Platform:   "feishu",
		SessionKey: "feishu:chat1:userA",
		Content:    "/stop",
	}
}

func TestStop_IsCommand(t *testing.T) {
	h := command.NewStopCommandHandler(
		&fakeBeeStopper{},
		&fakeStopMsgStore{},
		&fakeStopNotifier{},
		nil,
	)
	if !h.IsCommand("/stop") {
		t.Error("expected IsCommand('/stop') = true")
	}
	if h.IsCommand("/stopp") {
		t.Error("expected IsCommand('/stopp') = false")
	}
	if h.IsCommand("/clear") {
		t.Error("expected IsCommand('/clear') = false")
	}
}

func TestStop_BeeRunning_PendingMessages(t *testing.T) {
	stopper := &fakeBeeStopper{wasRunning: true}
	msgStore := &fakeStopMsgStore{ids: []string{"msg-1", "msg-2"}}
	notifier := &fakeStopNotifier{}
	sender := &fakeStopSender{}

	h := command.NewStopCommandHandler(stopper, msgStore, notifier,
		map[string]platform.PlatformSenderAdapter{"feishu": sender})
	h.HandleCommand(context.Background(), "/stop", makeStopReplyTo())

	if !stopper.stopped {
		t.Error("expected StopSession to be called")
	}
	if len(notifier.notified) != 2 {
		t.Errorf("expected 2 notifications, got %d", len(notifier.notified))
	}
	if len(sender.sent) != 1 {
		t.Fatalf("expected 1 reply, got %d", len(sender.sent))
	}
	// Should mention both stopping bee and cancelling 2 messages
	reply := sender.sent[0]
	if reply == "" {
		t.Error("expected non-empty reply")
	}
}

func TestStop_BeeRunning_NoMessages(t *testing.T) {
	stopper := &fakeBeeStopper{wasRunning: true}
	msgStore := &fakeStopMsgStore{ids: nil}
	sender := &fakeStopSender{}

	h := command.NewStopCommandHandler(stopper, msgStore, &fakeStopNotifier{},
		map[string]platform.PlatformSenderAdapter{"feishu": sender})
	h.HandleCommand(context.Background(), "/stop", makeStopReplyTo())

	if len(sender.sent) != 1 {
		t.Fatalf("expected 1 reply, got %d", len(sender.sent))
	}
}

func TestStop_NothingToStop(t *testing.T) {
	stopper := &fakeBeeStopper{wasRunning: false}
	msgStore := &fakeStopMsgStore{ids: nil}
	sender := &fakeStopSender{}

	h := command.NewStopCommandHandler(stopper, msgStore, &fakeStopNotifier{},
		map[string]platform.PlatformSenderAdapter{"feishu": sender})
	h.HandleCommand(context.Background(), "/stop", makeStopReplyTo())

	if len(sender.sent) != 1 {
		t.Fatalf("expected 1 reply, got %d", len(sender.sent))
	}
}

func TestStop_OnlyMessages(t *testing.T) {
	stopper := &fakeBeeStopper{wasRunning: false}
	msgStore := &fakeStopMsgStore{ids: []string{"msg-3"}}
	notifier := &fakeStopNotifier{}
	sender := &fakeStopSender{}

	h := command.NewStopCommandHandler(stopper, msgStore, notifier,
		map[string]platform.PlatformSenderAdapter{"feishu": sender})
	h.HandleCommand(context.Background(), "/stop", makeStopReplyTo())

	if len(notifier.notified) != 1 {
		t.Errorf("expected 1 notification, got %d", len(notifier.notified))
	}
	if len(sender.sent) != 1 {
		t.Fatalf("expected 1 reply, got %d", len(sender.sent))
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/domain/command/ -run TestStop -v
```

Expected: FAIL — `command.NewStopCommandHandler undefined`.

- [ ] **Step 3: Implement `stop.go`**

Create `internal/domain/command/stop.go`:

```go
package command

import (
	"context"
	"fmt"

	"github.com/theopenbee/openbee/internal/infra/i18n"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/platform"
)

// BeeStopper cancels the running bee for a session.
type BeeStopper interface {
	StopSession(sessionKey string) bool
}

// StopMessageStore cancels pending messages for a session.
type StopMessageStore interface {
	FailReceived(ctx context.Context, sessionKey string) ([]string, error)
}

// StopFailureNotifier sends failure notifications for individual messages.
type StopFailureNotifier interface {
	NotifyTaskFailure(ctx context.Context, messageID string, info model.FailureInfo) error
}

// StopCommandHandler handles the /stop command.
type StopCommandHandler struct {
	feeder   BeeStopper
	msgs     StopMessageStore
	notifier StopFailureNotifier
	senders  map[string]platform.PlatformSenderAdapter
}

// NewStopCommandHandler creates a StopCommandHandler.
func NewStopCommandHandler(
	feeder BeeStopper,
	msgs StopMessageStore,
	notifier StopFailureNotifier,
	senders map[string]platform.PlatformSenderAdapter,
) *StopCommandHandler {
	return &StopCommandHandler{feeder: feeder, msgs: msgs, notifier: notifier, senders: senders}
}

func (h *StopCommandHandler) IsCommand(content string) bool {
	return content == CmdStop
}

func (h *StopCommandHandler) HandleCommand(ctx context.Context, content string, replyTo platform.InboundMessage) bool {
	if content != CmdStop {
		return false
	}
	sessionKey := replyTo.SessionKey
	m := i18n.M.Runtime.StopCommand

	// Step 1: fail pending received messages (while bee is still feeding — avoids ClaimBatch race)
	ids, err := h.msgs.FailReceived(ctx, sessionKey)
	if err != nil {
		log.Error("stop: fail received messages", zap_field("sessionKey", sessionKey), zap_field("err", err))
	}

	// Step 2: notify each cancelled message (preserves existing failure notification behavior)
	for _, id := range ids {
		if notifyErr := h.notifier.NotifyTaskFailure(ctx, id, model.FailureInfo{
			Reason:     "stopped by /stop",
			WorkerName: "bee",
		}); notifyErr != nil {
			log.Error("stop: notify failure", zap_field("messageID", id), zap_field("err", notifyErr))
		}
	}

	// Step 3: cancel the running bee
	beeWasStopped := h.feeder.StopSession(sessionKey)

	// Step 4: reply with result
	var reply string
	switch {
	case beeWasStopped && len(ids) > 0:
		reply = fmt.Sprintf(m.StoppedWithMessages, len(ids))
	case beeWasStopped:
		reply = m.Stopped
	case len(ids) > 0:
		reply = fmt.Sprintf(m.CancelledMessages, len(ids))
	default:
		reply = m.NothingToStop
	}
	sendReply(ctx, h.senders, replyTo, reply)
	return true
}
```

Note: `zap_field` is a placeholder — use `zap.String` and `zap.Error` directly:

Replace `zap_field("sessionKey", sessionKey)` with `zap.String("sessionKey", sessionKey)` and `zap_field("err", err)` with `zap.Error(err)`. Also add `"go.uber.org/zap"` to imports.

The corrected `stop.go`:

```go
package command

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/theopenbee/openbee/internal/infra/i18n"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/platform"
)

// BeeStopper cancels the running bee for a session.
type BeeStopper interface {
	StopSession(sessionKey string) bool
}

// StopMessageStore cancels pending messages for a session.
type StopMessageStore interface {
	FailReceived(ctx context.Context, sessionKey string) ([]string, error)
}

// StopFailureNotifier sends failure notifications for individual messages.
type StopFailureNotifier interface {
	NotifyTaskFailure(ctx context.Context, messageID string, info model.FailureInfo) error
}

// StopCommandHandler handles the /stop command.
type StopCommandHandler struct {
	feeder   BeeStopper
	msgs     StopMessageStore
	notifier StopFailureNotifier
	senders  map[string]platform.PlatformSenderAdapter
}

// NewStopCommandHandler creates a StopCommandHandler.
func NewStopCommandHandler(
	feeder BeeStopper,
	msgs StopMessageStore,
	notifier StopFailureNotifier,
	senders map[string]platform.PlatformSenderAdapter,
) *StopCommandHandler {
	return &StopCommandHandler{feeder: feeder, msgs: msgs, notifier: notifier, senders: senders}
}

func (h *StopCommandHandler) IsCommand(content string) bool {
	return content == CmdStop
}

func (h *StopCommandHandler) HandleCommand(ctx context.Context, content string, replyTo platform.InboundMessage) bool {
	if content != CmdStop {
		return false
	}
	sessionKey := replyTo.SessionKey
	m := i18n.M.Runtime.StopCommand

	ids, err := h.msgs.FailReceived(ctx, sessionKey)
	if err != nil {
		log.Error("stop: fail received messages", zap.String("sessionKey", sessionKey), zap.Error(err))
	}

	for _, id := range ids {
		if notifyErr := h.notifier.NotifyTaskFailure(ctx, id, model.FailureInfo{
			Reason:     "stopped by /stop",
			WorkerName: "bee",
		}); notifyErr != nil {
			log.Error("stop: notify failure", zap.String("messageID", id), zap.Error(notifyErr))
		}
	}

	beeWasStopped := h.feeder.StopSession(sessionKey)

	var reply string
	switch {
	case beeWasStopped && len(ids) > 0:
		reply = fmt.Sprintf(m.StoppedWithMessages, len(ids))
	case beeWasStopped:
		reply = m.Stopped
	case len(ids) > 0:
		reply = fmt.Sprintf(m.CancelledMessages, len(ids))
	default:
		reply = m.NothingToStop
	}
	sendReply(ctx, h.senders, replyTo, reply)
	return true
}
```

- [ ] **Step 4: Run the tests**

```bash
go test ./internal/domain/command/ -run TestStop -v
```

Expected: all 5 TestStop_* tests PASS.

- [ ] **Step 5: Run the full command test suite**

```bash
go test ./internal/domain/command/... -v 2>&1 | tail -20
```

Expected: all tests PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/domain/command/stop.go internal/domain/command/stop_test.go
git commit -m "feat(command): add StopCommandHandler for /stop"
```

---

## Task 6: Wire `StopCommandHandler` in app.go

**Files:**
- Modify: `internal/app/app.go`

- [ ] **Step 1: Create and register `stopCmdHandler`**

In `app.go`, find the line:

```go
cmdChain := msgingest.ChainHandlers(engineCmdHandler, clearCmdHandler)
```

Replace it with:

```go
stopCmdHandler := command.NewStopCommandHandler(feeder, s.msgStore, failureNotifier, sendersByPlatform)
cmdChain := msgingest.ChainHandlers(engineCmdHandler, clearCmdHandler, stopCmdHandler)
```

(`feeder`, `failureNotifier`, and `sendersByPlatform` are already defined earlier in the same function.)

- [ ] **Step 2: Build the full binary**

```bash
go build ./...
```

Expected: no errors.

- [ ] **Step 3: Run all tests**

```bash
go test ./... 2>&1 | grep -E "FAIL|ok" | head -30
```

Expected: no FAIL lines.

- [ ] **Step 4: Commit**

```bash
git add internal/app/app.go
git commit -m "feat(app): wire /stop command handler"
```

---

## Self-Review

**Spec coverage:**
- ✅ Terminate running bee → Task 3 (`StopSession`) + Task 5 (handler calls it)
- ✅ Cancel pending messages → Task 2 (`FailReceived`) + Task 5 (handler calls it)
- ✅ Preserve session context → nothing deletes session records; confirmed by non-goals
- ✅ Send failure notifications per message → Task 5, step 3
- ✅ Result summary message with 4 cases → Task 5, `switch` block + Task 1 (strings)
- ✅ Execution order (FailReceived before StopSession) → Task 5 handles this correctly
- ✅ App wiring → Task 6

**Placeholder scan:** None found.

**Type consistency:**
- `BeeStopper.StopSession(sessionKey string) bool` — defined Task 5, satisfied by `*bee.Feeder.StopSession` from Task 3 ✅
- `StopMessageStore.FailReceived(ctx, sessionKey) ([]string, error)` — defined Task 5, implemented in Task 2 ✅
- `StopFailureNotifier.NotifyTaskFailure` — matches `bee.FailureNotifier` used in `failureNotifier` from app.go ✅
- `CmdStop` constant — defined Task 4, used Task 5 ✅
- `i18n.M.Runtime.StopCommand.{Stopped,StoppedWithMessages,CancelledMessages,NothingToStop}` — defined Task 1, used Task 5 ✅
