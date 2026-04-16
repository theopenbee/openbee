# /stop Command Ingest-Level Interception Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `/stop` execute immediately upon arrival at the ingest layer, bypassing the FIFO message queue and the `feeding`-status block that prevents it from running while Bee is active.

**Architecture:** `msgingest.Gateway.Dispatch()` checks an `InboundInterceptor` before entering the debounce/queue path. `bee.CommandInterceptor` implements this interface: on `/stop` it fires `handleStop` in a goroutine (non-blocking) and returns true. The Feeder-level `CommandInterceptor` wiring is removed since it is no longer needed.

**Tech Stack:** Go, SQLite (`*store.MessageStore`), existing `bee.CommandInterceptor` and `msgingest.Gateway`

---

### Task 1: Add `InboundInterceptor` support to `msgingest.Gateway`

**Files:**
- Modify: `internal/domain/msgingest/gateway.go`
- Modify: `internal/domain/msgingest/gateway_test.go`

- [ ] **Step 1: Write two failing tests**

Add to `internal/domain/msgingest/gateway_test.go` (after the existing tests):

```go
// testInterceptor is a test-only implementation of msgingest.InboundInterceptor.
type testInterceptor struct {
	fn func(platform.InboundMessage) bool
}

func (ti *testInterceptor) InterceptInbound(_ context.Context, msg platform.InboundMessage) bool {
	return ti.fn(msg)
}

// TestGateway_Interceptor_Stop_NotQueued verifies that when the interceptor returns
// true the message bypasses debounce entirely — no CreateBatch call, no emit.
func TestGateway_Interceptor_Stop_NotQueued(t *testing.T) {
	st := newMock()
	g := msgingest.New(st, 100*time.Millisecond)

	called := false
	g.SetInboundInterceptor(&testInterceptor{fn: func(msg platform.InboundMessage) bool {
		if strings.TrimSpace(msg.Content) == "/stop" {
			called = true
			return true
		}
		return false
	}})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go g.Run(ctx)

	g.Dispatch(inbound("s1", "/stop", "cmd-1"))

	select {
	case msg := <-g.Out():
		t.Fatalf("expected no emit for intercepted command, got: %+v", msg)
	case <-time.After(300 * time.Millisecond):
	}

	if !called {
		t.Error("expected interceptor to be called")
	}
	if len(st.batches) != 0 {
		t.Errorf("expected no CreateBatch calls, got %d", len(st.batches))
	}
}

// TestGateway_Interceptor_NormalMessage_PassesThrough verifies that when the
// interceptor returns false the message goes through normal debounce.
func TestGateway_Interceptor_NormalMessage_PassesThrough(t *testing.T) {
	st := newMock()
	g := msgingest.New(st, 100*time.Millisecond)
	g.SetInboundInterceptor(&testInterceptor{fn: func(_ platform.InboundMessage) bool {
		return false
	}})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go g.Run(ctx)

	g.Dispatch(inbound("s1", "hello", "m1"))

	select {
	case <-g.Out():
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout: expected debounced message to be emitted")
	}

	if len(st.batches) != 1 {
		t.Errorf("expected 1 CreateBatch call, got %d", len(st.batches))
	}
}
```

Also add missing imports to the test file (add `"context"` and `"strings"` if not already present).

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee/.worktrees/feature/stop-command
go test ./internal/domain/msgingest/... -run "TestGateway_Interceptor" -v
```

Expected: FAIL — `g.SetInboundInterceptor undefined`

- [ ] **Step 3: Implement `InboundInterceptor` interface, field, setter, and Dispatch check**

In `internal/domain/msgingest/gateway.go`, add the interface definition after the imports:

```go
// InboundInterceptor intercepts an inbound message before it enters the debounce queue.
// Returns true if the message was handled and should not be queued.
type InboundInterceptor interface {
	InterceptInbound(ctx context.Context, msg platform.InboundMessage) bool
}
```

Add `interceptor InboundInterceptor` field to the `Gateway` struct (after `out chan IngestedMessage`):

```go
type Gateway struct {
	msgStore    MessageStore
	debounce    time.Duration
	sessions    map[string]*debounceState
	seen        map[string]struct{}
	mu          sync.Mutex
	out         chan IngestedMessage
	interceptor InboundInterceptor
}
```

Add setter after `func (g *Gateway) Out()`:

```go
// SetInboundInterceptor registers an interceptor called at the start of every Dispatch.
// Must be called before goroutines start (not concurrency-safe).
func (g *Gateway) SetInboundInterceptor(h InboundInterceptor) {
	g.interceptor = h
}
```

Add intercept check at the very top of `Dispatch()`, before the mutex lock:

```go
func (g *Gateway) Dispatch(msg platform.InboundMessage) {
	if g.interceptor != nil && g.interceptor.InterceptInbound(context.Background(), msg) {
		return
	}
	g.mu.Lock()
	// ... rest of existing Dispatch logic unchanged
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/domain/msgingest/... -v
```

Expected: all tests PASS including the two new ones.

- [ ] **Step 5: Commit**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee/.worktrees/feature/stop-command
git add internal/domain/msgingest/gateway.go internal/domain/msgingest/gateway_test.go
git commit -m "feat(msgingest): add InboundInterceptor support to Gateway"
```

---

### Task 2: Add `InterceptInbound()` to `bee.CommandInterceptor`

**Files:**
- Modify: `internal/domain/bee/command_interceptor.go`
- Modify: `internal/domain/bee/command_interceptor_test.go`

- [ ] **Step 1: Write three failing tests**

Add to `internal/domain/bee/command_interceptor_test.go` (after the existing tests):

```go
func TestCommandInterceptor_InterceptInbound_Stop_ReturnsTrue(t *testing.T) {
	_, _, _, _, clearer, _, sender, ci := setupCommandInterceptorTest(t)

	msg := platform.InboundMessage{
		Platform:   "local",
		SessionKey: "local:1",
		Content:    "/stop",
	}
	handled := ci.InterceptInbound(context.Background(), msg)
	if !handled {
		t.Error("expected InterceptInbound to return true for /stop")
	}

	// Give the goroutine time to run handleStop.
	time.Sleep(100 * time.Millisecond)

	clearer.mu.Lock()
	defer clearer.mu.Unlock()
	if len(clearer.cleared) == 0 || clearer.cleared[0] != "local:1" {
		t.Errorf("expected ClearSession called with 'local:1', got %v", clearer.cleared)
	}
	if len(sender.sent) == 0 {
		t.Fatal("expected reply sent")
	}
}

func TestCommandInterceptor_InterceptInbound_NonStop_ReturnsFalse(t *testing.T) {
	_, _, _, _, _, _, _, ci := setupCommandInterceptorTest(t)

	msg := platform.InboundMessage{
		Platform:   "local",
		SessionKey: "local:1",
		Content:    "hello world",
	}
	if ci.InterceptInbound(context.Background(), msg) {
		t.Error("expected InterceptInbound to return false for non-stop message")
	}
}

func TestCommandInterceptor_InterceptInbound_CaseInsensitive(t *testing.T) {
	_, _, _, _, _, _, _, ci := setupCommandInterceptorTest(t)

	for _, content := range []string{"/STOP", "/Stop", "  /stop  "} {
		msg := platform.InboundMessage{
			Platform:   "local",
			SessionKey: "local:1",
			Content:    content,
		}
		if !ci.InterceptInbound(context.Background(), msg) {
			t.Errorf("expected handled=true for %q", content)
		}
		time.Sleep(50 * time.Millisecond) // drain goroutine between iterations
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/domain/bee/... -run "TestCommandInterceptor_InterceptInbound" -v
```

Expected: FAIL — `ci.InterceptInbound undefined`

- [ ] **Step 3: Implement `InterceptInbound()`**

Add to `internal/domain/bee/command_interceptor.go` (before `handleStop`):

```go
// InterceptInbound implements msgingest.InboundInterceptor.
// Returns true and fires handleStop asynchronously when msg is a /stop command.
func (c *CommandInterceptor) InterceptInbound(ctx context.Context, msg platform.InboundMessage) bool {
	if !strings.EqualFold(strings.TrimSpace(msg.Content), cmdStop) {
		return false
	}
	go c.handleStop(context.Background(), msg.SessionKey, store.ClaimedMessage{
		SessionKey: msg.SessionKey,
		Platform:   msg.Platform,
		Content:    msg.Content,
	})
	return true
}
```

All required imports (`context`, `strings`, `store`, `platform`) are already present in the file.

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/domain/bee/... -v
```

Expected: all tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/domain/bee/command_interceptor.go internal/domain/bee/command_interceptor_test.go
git commit -m "feat(bee): add InterceptInbound to CommandInterceptor for ingest-level /stop"
```

---

### Task 3: Remove Feeder-level `CommandInterceptor`

**Files:**
- Modify: `internal/domain/bee/feeder.go`

- [ ] **Step 1: Delete `commandInterceptor` field and `SetCommandInterceptor` from `feeder.go`**

Remove field from the `Feeder` struct (around line 64):
```go
// DELETE this line:
commandInterceptor *CommandInterceptor
```

Remove `SetCommandInterceptor` method entirely (lines 48-50):
```go
// DELETE this entire method:
func (f *Feeder) SetCommandInterceptor(ci *CommandInterceptor) {
	f.commandInterceptor = ci
}
```

Remove the intercept block from `processBeeGroup` (lines 152-161):
```go
// DELETE this entire block:
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
```

- [ ] **Step 2: Run tests and build to verify no regressions**

```bash
go test ./internal/domain/bee/... -v
go build ./...
```

Expected: all tests PASS, build succeeds.

- [ ] **Step 3: Commit**

```bash
git add internal/domain/bee/feeder.go
git commit -m "refactor(bee): remove Feeder-level CommandInterceptor — /stop now handled at ingest layer"
```

---

### Task 4: Update wiring in `app.go`

**Files:**
- Modify: `internal/app/app.go`

- [ ] **Step 1: Replace `feeder.SetCommandInterceptor` with gateway setters**

In `internal/app/app.go`, find the block (around line 121-122):

```go
ci := bee.NewCommandInterceptor(s.sessionStore, s.execStore, s.taskStore, mgr, disp, s.msgStore, sendersByPlatform, cfg.Bee.EffectiveEngine())
feeder.SetCommandInterceptor(ci)
```

Replace with:

```go
ci := bee.NewCommandInterceptor(s.sessionStore, s.execStore, s.taskStore, mgr, disp, s.msgStore, sendersByPlatform, cfg.Bee.EffectiveEngine())
ingest.SetInboundInterceptor(ci)
localIngest.SetInboundInterceptor(ci)
```

- [ ] **Step 2: Verify the build**

```bash
go build ./...
```

Expected: build succeeds with no errors.

- [ ] **Step 3: Run full test suite**

```bash
go test ./...
```

Expected: all tests PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/app/app.go
git commit -m "feat(app): wire CommandInterceptor into ingest gateways for immediate /stop handling"
```
