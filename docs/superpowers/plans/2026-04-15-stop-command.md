# `/stop` Command Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a `/stop` slash command that intercepts messages before Bee processes them and terminates all running/pending Bee executions and Worker tasks for the session.

**Architecture:** A new `CommandInterceptor` struct in the `bee` package is injected into `Feeder`. At the start of `processBeeGroup`, before any Bee dispatch, the interceptor checks if the primary message is `/stop` and handles it—stopping the Bee execution, cancelling Worker tasks, clearing dispatcher queues, and sending a reply.

**Tech Stack:** Go, SQLite (via `database/sql`), existing store/model packages, `platform.PlatformSenderAdapter` for replies.

---

## File Map

| File | Action | Responsibility |
|------|--------|---------------|
| `internal/domain/bee/command_interceptor.go` | Create | CommandInterceptor struct, `/stop` logic |
| `internal/domain/bee/command_interceptor_test.go` | Create | Unit tests with in-memory SQLite + mock interfaces |
| `internal/domain/bee/feeder.go` | Modify | Add `commandInterceptor` field, `WithCommandInterceptor` option, `SetCommandInterceptor` method, call in `processBeeGroup` |
| `internal/app/app.go` | Modify | Construct `CommandInterceptor` after `feeder` and `disp` exist; inject via `SetCommandInterceptor` |

---

### Task 1: CommandInterceptor — core struct and `/stop` handler

**Files:**
- Create: `internal/domain/bee/command_interceptor.go`

- [ ] **Step 1: Create the file**

```go
package bee

import (
	"context"
	"strings"

	"go.uber.org/zap"

	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/infra/store"
	"github.com/theopenbee/openbee/internal/platform"
)

// executionStopper can kill a running process by execution ID.
type executionStopper interface {
	StopExecution(executionID string) error
}

// sessionClearer clears dispatcher queues and session contexts for a session.
type sessionClearer interface {
	ClearSession(sessionKey string)
}

// CommandInterceptor intercepts system slash commands before Feeder dispatches to Bee.
type CommandInterceptor struct {
	sessionStore *store.SessionStore
	execStore    *store.ExecutionStore
	taskStore    *store.TaskStore
	execStopper  executionStopper
	dispatcher   sessionClearer
	senders      map[string]platform.PlatformSenderAdapter
	engine       string
}

// NewCommandInterceptor creates a CommandInterceptor.
func NewCommandInterceptor(
	ss *store.SessionStore,
	es *store.ExecutionStore,
	ts *store.TaskStore,
	stopper executionStopper,
	clearer sessionClearer,
	senders map[string]platform.PlatformSenderAdapter,
	engine string,
) *CommandInterceptor {
	return &CommandInterceptor{
		sessionStore: ss,
		execStore:    es,
		taskStore:    ts,
		execStopper:  stopper,
		dispatcher:   clearer,
		senders:      senders,
		engine:       engine,
	}
}

// Intercept checks if the primary message is a slash command and handles it.
// Returns true if the message was handled and Feeder should skip normal dispatch.
func (c *CommandInterceptor) Intercept(ctx context.Context, sessionKey string, msgs []store.ClaimedMessage) (bool, error) {
	if len(msgs) == 0 {
		return false, nil
	}
	primary := msgs[len(msgs)-1]
	if !strings.EqualFold(strings.TrimSpace(primary.Content), "/stop") {
		return false, nil
	}
	return true, c.handleStop(ctx, sessionKey, primary)
}

func (c *CommandInterceptor) handleStop(ctx context.Context, sessionKey string, msg store.ClaimedMessage) error {
	stopped := false

	// 1. Find and stop Bee's running/pending execution for the current engine session.
	sessionID, err := c.sessionStore.GetSessionContextForEngine(ctx, sessionKey, store.BeeAgentID, c.engine)
	if err != nil {
		log.Warn("stop command: get session context", zap.String("sessionKey", sessionKey), zap.Error(err))
	} else if sessionID != "" {
		execs, listErr := c.execStore.ListBySessionID(sessionID)
		if listErr != nil {
			log.Warn("stop command: list executions", zap.String("sessionID", sessionID), zap.Error(listErr))
		}
		for _, e := range execs {
			if e.Status == model.ExecStatusRunning || e.Status == model.ExecStatusPending {
				if stopErr := c.execStopper.StopExecution(e.ID); stopErr != nil {
					log.Warn("stop command: stop execution", zap.String("execID", e.ID), zap.Error(stopErr))
				} else {
					stopped = true
				}
			}
		}
	}

	// 2. Cancel all pending/running Worker tasks for the session.
	n, err := c.taskStore.CancelBySessionKey(ctx, sessionKey)
	if err != nil {
		log.Warn("stop command: cancel tasks", zap.String("sessionKey", sessionKey), zap.Error(err))
	} else if n > 0 {
		stopped = true
	}

	// 3. Clear dispatcher in-memory queues.
	c.dispatcher.ClearSession(sessionKey)

	// 4. Reply to user.
	replyContent := "已停止当前会话的所有任务"
	if !stopped {
		replyContent = "当前会话没有正在运行的任务"
	}
	c.sendReply(ctx, msg, replyContent)
	return nil
}

func (c *CommandInterceptor) sendReply(ctx context.Context, msg store.ClaimedMessage, content string) {
	sender, ok := c.senders[msg.Platform]
	if !ok {
		log.Warn("stop command: no sender for platform", zap.String("platform", msg.Platform))
		return
	}
	outbound := platform.OutboundMessage{
		SessionKey: msg.SessionKey,
		Content:    content,
		ReplyTo: platform.InboundMessage{
			Platform:   msg.Platform,
			SessionKey: msg.SessionKey,
		},
		SourceType: "system",
	}
	if err := sender.Send(ctx, outbound); err != nil {
		log.Warn("stop command: send reply", zap.Error(err))
	}
}
```

- [ ] **Step 2: Verify it compiles**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee && go build ./internal/domain/bee/...
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add internal/domain/bee/command_interceptor.go
git commit -m "feat(bee): add CommandInterceptor for slash command handling"
```

---

### Task 2: Tests for CommandInterceptor

**Files:**
- Create: `internal/domain/bee/command_interceptor_test.go`

- [ ] **Step 1: Write the test file**

```go
package bee_test

import (
	"context"
	"sync"
	"testing"

	"github.com/theopenbee/openbee/internal/domain/bee"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/infra/store"
	"github.com/theopenbee/openbee/internal/platform"
)

// --- mock types ---

type mockExecStopper struct {
	mu      sync.Mutex
	stopped []string
	err     error
}

func (m *mockExecStopper) StopExecution(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopped = append(m.stopped, id)
	return m.err
}

type mockSessionClearer struct {
	mu      sync.Mutex
	cleared []string
}

func (m *mockSessionClearer) ClearSession(key string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cleared = append(m.cleared, key)
}

type mockSender struct {
	mu   sync.Mutex
	sent []platform.OutboundMessage
}

func (m *mockSender) Send(_ context.Context, msg platform.OutboundMessage) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sent = append(m.sent, msg)
	return nil
}

// --- setup helper ---

func setupCommandInterceptorTest(t *testing.T) (
	*store.SessionStore,
	*store.ExecutionStore,
	*store.TaskStore,
	*mockExecStopper,
	*mockSessionClearer,
	*mockSender,
	*bee.CommandInterceptor,
) {
	t.Helper()
	db, err := store.InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	ss := store.NewSessionStore(db)
	es := store.NewExecutionStore(db, t.TempDir())
	ts := store.NewTaskStore(db)

	stopper := &mockExecStopper{}
	clearer := &mockSessionClearer{}
	sender := &mockSender{}
	senders := map[string]platform.PlatformSenderAdapter{"local": sender}

	ci := bee.NewCommandInterceptor(ss, es, ts, stopper, clearer, senders, "claude-code")
	return ss, es, ts, stopper, clearer, sender, ci
}

// --- tests ---

func TestCommandInterceptor_NonCommand_NotHandled(t *testing.T) {
	_, _, _, _, _, _, ci := setupCommandInterceptorTest(t)
	ctx := context.Background()

	msgs := []store.ClaimedMessage{{ID: "m1", SessionKey: "local:1", Platform: "local", Content: "hello world"}}
	handled, err := ci.Intercept(ctx, "local:1", msgs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if handled {
		t.Error("expected handled=false for non-command message")
	}
}

func TestCommandInterceptor_EmptyContent_NotHandled(t *testing.T) {
	_, _, _, _, _, _, ci := setupCommandInterceptorTest(t)
	ctx := context.Background()

	msgs := []store.ClaimedMessage{{ID: "m1", SessionKey: "local:1", Platform: "local", Content: "   "}}
	handled, _ := ci.Intercept(ctx, "local:1", msgs)
	if handled {
		t.Error("expected handled=false for whitespace-only message")
	}
}

func TestCommandInterceptor_Stop_NoActiveTasks_SendsNoTasksReply(t *testing.T) {
	_, _, _, _, clearer, sender, ci := setupCommandInterceptorTest(t)
	ctx := context.Background()

	msgs := []store.ClaimedMessage{{ID: "m1", SessionKey: "local:1", Platform: "local", Content: "/stop"}}
	handled, err := ci.Intercept(ctx, "local:1", msgs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !handled {
		t.Error("expected handled=true for /stop")
	}
	if len(clearer.cleared) == 0 || clearer.cleared[0] != "local:1" {
		t.Errorf("expected ClearSession called with 'local:1', got %v", clearer.cleared)
	}
	if len(sender.sent) == 0 {
		t.Fatal("expected reply sent")
	}
	if sender.sent[0].Content != "当前会话没有正在运行的任务" {
		t.Errorf("unexpected reply: %q", sender.sent[0].Content)
	}
}

func TestCommandInterceptor_Stop_WithRunningExecution_StopsAndReplies(t *testing.T) {
	ss, es, _, stopper, clearer, sender, ci := setupCommandInterceptorTest(t)
	ctx := context.Background()

	// Seed session context and a running execution
	sessionID := "sess-abc"
	if err := ss.UpsertSessionContext(ctx, "local:1", store.BeeAgentID, sessionID, "claude-code"); err != nil {
		t.Fatal(err)
	}
	exec, err := es.CreateBeeExecution(sessionID, "do something")
	if err != nil {
		t.Fatal(err)
	}
	// Mark it running
	if err := es.UpdateStatus(exec.ID, model.ExecStatusRunning); err != nil {
		t.Fatal(err)
	}

	msgs := []store.ClaimedMessage{{ID: "m1", SessionKey: "local:1", Platform: "local", Content: "/stop"}}
	handled, err := ci.Intercept(ctx, "local:1", msgs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !handled {
		t.Error("expected handled=true for /stop")
	}
	stopper.mu.Lock()
	defer stopper.mu.Unlock()
	if len(stopper.stopped) == 0 || stopper.stopped[0] != exec.ID {
		t.Errorf("expected StopExecution called with %q, got %v", exec.ID, stopper.stopped)
	}
	if len(clearer.cleared) == 0 {
		t.Error("expected ClearSession called")
	}
	if len(sender.sent) == 0 {
		t.Fatal("expected reply sent")
	}
	if sender.sent[0].Content != "已停止当前会话的所有任务" {
		t.Errorf("unexpected reply: %q", sender.sent[0].Content)
	}
}

func TestCommandInterceptor_Stop_StopExecutionError_StillSendsReply(t *testing.T) {
	ss, es, _, stopper, _, sender, ci := setupCommandInterceptorTest(t)
	ctx := context.Background()

	sessionID := "sess-xyz"
	if err := ss.UpsertSessionContext(ctx, "local:1", store.BeeAgentID, sessionID, "claude-code"); err != nil {
		t.Fatal(err)
	}
	exec, err := es.CreateBeeExecution(sessionID, "some task")
	if err != nil {
		t.Fatal(err)
	}
	if err := es.UpdateStatus(exec.ID, model.ExecStatusRunning); err != nil {
		t.Fatal(err)
	}
	// Simulate stop failure (process already exited)
	stopper.err = context.DeadlineExceeded

	msgs := []store.ClaimedMessage{{ID: "m1", SessionKey: "local:1", Platform: "local", Content: "/stop"}}
	handled, err := ci.Intercept(ctx, "local:1", msgs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !handled {
		t.Error("expected handled=true")
	}
	// Reply is still sent even when StopExecution fails
	if len(sender.sent) == 0 {
		t.Fatal("expected reply sent despite stop error")
	}
}

func TestCommandInterceptor_Stop_CaseInsensitive(t *testing.T) {
	_, _, _, _, _, sender, ci := setupCommandInterceptorTest(t)
	ctx := context.Background()

	for _, content := range []string{"/STOP", "/Stop", "  /stop  "} {
		msgs := []store.ClaimedMessage{{ID: "m1", SessionKey: "local:1", Platform: "local", Content: content}}
		handled, _ := ci.Intercept(ctx, "local:1", msgs)
		if !handled {
			t.Errorf("expected /stop to be handled regardless of case/whitespace, got false for %q", content)
		}
		_ = sender
	}
}
```

- [ ] **Step 2: Run tests**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee && go test ./internal/domain/bee/... -run TestCommandInterceptor -v
```

Expected output: all 5 tests PASS. If any fail, check that `es.UpdateStatus` exists in `ExecutionStore`.

> **Note:** If `UpdateStatus` doesn't exist, look for the equivalent method to change execution status in `store/execution_store.go`. Common alternatives: `UpdateResult` with the relevant status arg.

- [ ] **Step 3: Commit**

```bash
git add internal/domain/bee/command_interceptor_test.go
git commit -m "test(bee): add CommandInterceptor unit tests"
```

---

### Task 3: Feeder integration

**Files:**
- Modify: `internal/domain/bee/feeder.go`

- [ ] **Step 1: Add `commandInterceptor` field to `Feeder` struct** (after existing fields, around line 59)

```go
// In Feeder struct, add after workerLookup:
commandInterceptor *CommandInterceptor
```

- [ ] **Step 2: Add `WithCommandInterceptor` option and `SetCommandInterceptor` method** (add after `WithWorkerDispatch`, around line 46)

```go
// WithCommandInterceptor enables slash-command interception before Bee dispatch.
func WithCommandInterceptor(ci *CommandInterceptor) Option {
	return func(f *Feeder) { f.commandInterceptor = ci }
}

// SetCommandInterceptor sets the command interceptor. Useful when the interceptor
// depends on components created after the Feeder (e.g. TaskDispatcher).
func (f *Feeder) SetCommandInterceptor(ci *CommandInterceptor) {
	f.commandInterceptor = ci
}
```

- [ ] **Step 3: Call interceptor in `processBeeGroup`** (at the very top of the function, before `tryDirectDispatch`, around line 147)

```go
func (f *Feeder) processBeeGroup(ctx context.Context, sessionKey string, msgs []store.ClaimedMessage) {
	// Intercept system commands before Bee dispatch.
	if f.commandInterceptor != nil {
		if handled, err := f.commandInterceptor.Intercept(ctx, sessionKey, msgs); err != nil {
			log.Warn("command interceptor error", zap.String("sessionKey", sessionKey), zap.Error(err))
		} else if handled {
			if err := f.msgStore.MarkBeeProcessed(ctx, messageIDs(msgs)); err != nil {
				log.Error("command: mark bee_processed", zap.String("sessionKey", sessionKey), zap.Error(err))
			}
			return
		}
	}

	if f.tryDirectDispatch(ctx, msgs) {
	// ... rest of existing function unchanged
```

- [ ] **Step 4: Verify it compiles**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee && go build ./internal/domain/bee/...
```

Expected: no errors.

- [ ] **Step 5: Run existing feeder tests to check for regressions**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee && go test ./internal/domain/bee/... -v -timeout 30s
```

Expected: all existing tests PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/domain/bee/feeder.go
git commit -m "feat(bee): integrate CommandInterceptor into Feeder"
```

---

### Task 4: Wire CommandInterceptor in app.go

**Files:**
- Modify: `internal/app/app.go`

- [ ] **Step 1: Add `bee` import alias if not already present**

The `bee` package is already imported as `"github.com/theopenbee/openbee/internal/domain/bee"` in `app.go`. No change needed.

- [ ] **Step 2: Construct and inject CommandInterceptor after both `feeder` and `disp` exist** (after line 119, i.e., after `buildPipeline` call)

Find this block in `BuildApp`:

```go
feeder, sched := buildBee(cfg.Bee, s, dispatchCh, failureNotifier, engine, envSvc)
ingest, disp := buildPipeline(cfg.Bee.MessageDebounce, cfg.Bee.EffectiveEngine(), s, mgr, dispatchCh, failureNotifier)
```

Add these two lines immediately after:

```go
ci := bee.NewCommandInterceptor(s.sessionStore, s.execStore, s.taskStore, mgr, disp, sendersByPlatform, cfg.Bee.EffectiveEngine())
feeder.SetCommandInterceptor(ci)
```

- [ ] **Step 3: Verify the whole project compiles**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee && go build ./...
```

Expected: no errors.

- [ ] **Step 4: Run all tests**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee && go test ./... -timeout 60s
```

Expected: all tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/app/app.go
git commit -m "feat(app): wire CommandInterceptor into Feeder"
```

---

## Self-Review Checklist

- [x] **Spec coverage:** All requirements covered — intercept before Bee, stop Bee execution, cancel Worker tasks, clear dispatcher, reply to user, edge cases handled.
- [x] **No placeholders:** All steps have actual code.
- [x] **Type consistency:** `executionStopper`/`sessionClearer` interfaces defined in Task 1 are used consistently. `*store.SessionStore`, `*store.ExecutionStore`, `*store.TaskStore` concrete types used throughout.
- [x] **`UpdateStatus` note:** Added a note in Task 2 to check if this method exists; if not, the test should use the available alternative.
