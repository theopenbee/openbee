# Empty Message Filter Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** When a user sends a message that becomes empty after the bot @mention is stripped (e.g. just `@貂蝉`), short-circuit the ingest pipeline and reply with a localized "your message looked empty" hint instead of writing to the DB, debouncing, or invoking the LLM.

**Architecture:** Add an `EmptyMessageHandler` interface and `DefaultEmptyMessageHandler` implementation in the `msgingest` package. Inject it into `Gateway` via a new `WithEmptyMessageHandler` option. In `Gateway.Dispatch`, insert an empty-content check after dedup and before the command/debounce branches so empty messages never reach storage.

**Tech Stack:** Go 1.x, `github.com/theopenbee/openbee/internal/domain/msgingest`, `github.com/theopenbee/openbee/internal/platform`, `gopkg.in/yaml.v3` for i18n locale files. Tests use the standard `testing` package.

**Reference spec:** `docs/superpowers/specs/2026-05-10-empty-message-filter-design.md`

---

## File Map

| File | Action | Responsibility |
|------|--------|----------------|
| `internal/infra/i18n/messages.go` | Modify | Add `EmptyMessage` field on `RuntimeMessages` and `EmptyMessageMessages` struct |
| `internal/infra/i18n/locales/zh.yaml` | Modify | Add `empty_message.hint` Chinese copy |
| `internal/infra/i18n/locales/en.yaml` | Modify | Add `empty_message.hint` English copy |
| `internal/domain/msgingest/empty_handler.go` | Create | `EmptyMessageHandler` interface + `DefaultEmptyMessageHandler` impl |
| `internal/domain/msgingest/empty_handler_test.go` | Create | Unit tests for the default handler |
| `internal/domain/msgingest/gateway.go` | Modify | Add `emptyHandler` field, `WithEmptyMessageHandler` option, empty-check branch in `Dispatch` |
| `internal/domain/msgingest/gateway_test.go` | Modify | Add 4 test cases for the empty-message branch |
| `internal/app/app.go` | Modify | Wire `DefaultEmptyMessageHandler` into the production `Gateway` |
| `CHANGELOG.md` | Modify | Add entry under the next unreleased version (English) |

---

## Task 1: Add i18n field and locale strings

**Files:**
- Modify: `internal/infra/i18n/messages.go:288-299` (the `RuntimeMessages` struct block)
- Modify: `internal/infra/i18n/locales/zh.yaml` (append under `runtime:`)
- Modify: `internal/infra/i18n/locales/en.yaml` (append under `runtime:`)

This task is data-only. The i18n package loads YAML into the struct at startup; if the struct field tag doesn't match the YAML key, the field will be the zero value (`""`). The verification step is the i18n_test which already parses both locales.

- [ ] **Step 1: Add new struct types in `messages.go`**

Edit `internal/infra/i18n/messages.go`. In the `RuntimeMessages` struct (currently spanning lines ~288-299), add the new field. After this change the block should look like:

```go
// RuntimeMessages holds server-runtime user-visible text (sent to IM users,
// platform placeholders, etc.) that must respond to the language setting.
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
	EmptyMessage    EmptyMessageMessages      `yaml:"empty_message"`
}
```

Then immediately after the `StatusCommandMessages` type definition (around line 348), add a new struct:

```go
// EmptyMessageMessages holds text sent to IM users when their message is
// empty after the bot @mention is stripped.
type EmptyMessageMessages struct {
	Hint string `yaml:"hint"`
}
```

- [ ] **Step 2: Add Chinese locale entry**

Edit `internal/infra/i18n/locales/zh.yaml`. After the last `status_command:` block (currently ending at line 276 with the `task_line:` entry), add at the same indentation (2 spaces, inside `runtime:`):

```yaml
  empty_message:
    hint: "📭 老板，您发的消息看起来是空的（可能只 @ 了我没带内容）。请补一下要我做什么？"
```

- [ ] **Step 3: Add English locale entry**

Edit `internal/infra/i18n/locales/en.yaml`. Find the matching `status_command:` block under `runtime:` and add the same block at the same indentation immediately after it:

```yaml
  empty_message:
    hint: "📭 Looks like your message is empty (perhaps just an @mention with no content). Could you tell me what you'd like me to do?"
```

- [ ] **Step 4: Run i18n test to verify both locales parse and resolve the new field**

Run: `cd /Users/tengyongzhi/work/bot-workspaces/openbee2 && go test ./internal/infra/i18n/...`
Expected: `PASS` for all existing tests. If the YAML is malformed or the struct tag is wrong, you'll see an unmarshal error.

- [ ] **Step 5: Commit**

```bash
git add internal/infra/i18n/messages.go internal/infra/i18n/locales/zh.yaml internal/infra/i18n/locales/en.yaml
git commit -m "i18n: add empty_message.hint copy in zh/en"
```

---

## Task 2: Create `EmptyMessageHandler` interface and default implementation (TDD)

**Files:**
- Create: `internal/domain/msgingest/empty_handler.go`
- Create: `internal/domain/msgingest/empty_handler_test.go`

The default handler depends on `map[string]platform.PlatformSenderAdapter` (existing project convention). It is called synchronously by `Gateway.Dispatch` after the gateway mutex is released, so the implementation must be safe to call without any external locking.

- [ ] **Step 1: Write the failing tests**

Create `internal/domain/msgingest/empty_handler_test.go`:

```go
package msgingest_test

import (
	"context"
	"errors"
	"testing"

	"github.com/theopenbee/openbee/internal/domain/msgingest"
	"github.com/theopenbee/openbee/internal/infra/i18n"
	"github.com/theopenbee/openbee/internal/platform"
)

type fakeSender struct {
	sent       []platform.OutboundMessage
	errReturn  error
}

func (f *fakeSender) Send(_ context.Context, msg platform.OutboundMessage) error {
	f.sent = append(f.sent, msg)
	return f.errReturn
}

func emptyInbound() platform.InboundMessage {
	return platform.InboundMessage{
		Platform:          "test",
		SessionKey:        "test:c1:u1",
		Content:           "",
		PlatformMessageID: "pm-1",
	}
}

func TestDefaultEmptyMessageHandler_SendsHint(t *testing.T) {
	// Load i18n so the hint copy is populated; Load("zh") is fine — it falls
	// back to embedded files; the test only asserts the value matches what
	// i18n.M resolves to (whatever language was loaded).
	if err := i18n.Load("zh"); err != nil {
		t.Fatalf("i18n.Load: %v", err)
	}
	sender := &fakeSender{}
	h := msgingest.NewDefaultEmptyMessageHandler(
		map[string]platform.PlatformSenderAdapter{"test": sender},
	)

	msg := emptyInbound()
	h.HandleEmpty(context.Background(), msg)

	if len(sender.sent) != 1 {
		t.Fatalf("expected 1 send, got %d", len(sender.sent))
	}
	got := sender.sent[0]
	if got.Content != i18n.M.Runtime.EmptyMessage.Hint {
		t.Errorf("Content = %q, want %q", got.Content, i18n.M.Runtime.EmptyMessage.Hint)
	}
	if got.SessionKey != msg.SessionKey {
		t.Errorf("SessionKey = %q, want %q", got.SessionKey, msg.SessionKey)
	}
	if got.ReplyTo.PlatformMessageID != msg.PlatformMessageID {
		t.Errorf("ReplyTo.PlatformMessageID = %q, want %q",
			got.ReplyTo.PlatformMessageID, msg.PlatformMessageID)
	}
	if got.SourceType != "system" {
		t.Errorf("SourceType = %q, want %q", got.SourceType, "system")
	}
}

func TestDefaultEmptyMessageHandler_NoSenderForPlatform(t *testing.T) {
	// Empty senders map → no sender for "test" platform.
	h := msgingest.NewDefaultEmptyMessageHandler(
		map[string]platform.PlatformSenderAdapter{},
	)
	// Must not panic, must not record any send (we have nothing to assert on
	// the call site; the test passes if the call returns without panicking).
	h.HandleEmpty(context.Background(), emptyInbound())
}

func TestDefaultEmptyMessageHandler_SenderError(t *testing.T) {
	sender := &fakeSender{errReturn: errors.New("send failed")}
	h := msgingest.NewDefaultEmptyMessageHandler(
		map[string]platform.PlatformSenderAdapter{"test": sender},
	)
	// Must not panic, must not retry. We only need 1 attempted send.
	h.HandleEmpty(context.Background(), emptyInbound())

	if len(sender.sent) != 1 {
		t.Errorf("expected exactly 1 send attempt, got %d", len(sender.sent))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/tengyongzhi/work/bot-workspaces/openbee2 && go test ./internal/domain/msgingest/ -run TestDefaultEmptyMessageHandler -v`
Expected: Compile error — `undefined: msgingest.NewDefaultEmptyMessageHandler`.

- [ ] **Step 3: Implement `empty_handler.go`**

Create `internal/domain/msgingest/empty_handler.go`:

```go
package msgingest

import (
	"context"

	"go.uber.org/zap"

	"github.com/theopenbee/openbee/internal/infra/i18n"
	"github.com/theopenbee/openbee/internal/platform"
)

// EmptyMessageHandler is invoked when an inbound message is empty after the
// bot @mention is stripped. Implementations should reply to the user (or
// no-op) without performing any DB writes or pipeline state mutations.
type EmptyMessageHandler interface {
	HandleEmpty(ctx context.Context, msg platform.InboundMessage)
}

// DefaultEmptyMessageHandler replies with a localized hint via the per-platform
// sender adapter.
type DefaultEmptyMessageHandler struct {
	senders map[string]platform.PlatformSenderAdapter
}

// NewDefaultEmptyMessageHandler constructs a handler bound to the given
// per-platform sender map.
func NewDefaultEmptyMessageHandler(senders map[string]platform.PlatformSenderAdapter) *DefaultEmptyMessageHandler {
	return &DefaultEmptyMessageHandler{senders: senders}
}

// HandleEmpty sends the empty-message hint to the user. If no sender is
// registered for msg.Platform, the call logs a warning and returns. Send
// errors are logged at warn level and not retried.
func (h *DefaultEmptyMessageHandler) HandleEmpty(ctx context.Context, msg platform.InboundMessage) {
	sender, ok := h.senders[msg.Platform]
	if !ok {
		log.Warn("no sender for empty-message reply",
			zap.String("platform", msg.Platform),
			zap.String("sessionKey", msg.SessionKey))
		return
	}
	out := platform.OutboundMessage{
		SessionKey: msg.SessionKey,
		Content:    i18n.M.Runtime.EmptyMessage.Hint,
		ReplyTo:    msg,
		SourceType: "system",
	}
	if err := sender.Send(ctx, out); err != nil {
		log.Warn("send empty-message reply failed",
			zap.String("platform", msg.Platform),
			zap.String("sessionKey", msg.SessionKey),
			zap.Error(err))
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/tengyongzhi/work/bot-workspaces/openbee2 && go test ./internal/domain/msgingest/ -run TestDefaultEmptyMessageHandler -v`
Expected: All three tests `PASS`.

- [ ] **Step 5: Commit**

```bash
git add internal/domain/msgingest/empty_handler.go internal/domain/msgingest/empty_handler_test.go
git commit -m "msgingest: add EmptyMessageHandler interface and default impl"
```

---

## Task 3: Add `WithEmptyMessageHandler` option and `emptyHandler` field on Gateway (TDD)

**Files:**
- Modify: `internal/domain/msgingest/gateway.go` (struct fields, Option func — no Dispatch changes yet)
- Modify: `internal/domain/msgingest/gateway_test.go` (one new test that exercises the option)

We split the wiring change from the Dispatch behavior so each TDD cycle is small.

- [ ] **Step 1: Write a failing test that injects the option**

Append to `internal/domain/msgingest/gateway_test.go`:

```go
// mockEmptyHandler records HandleEmpty invocations.
type mockEmptyHandler struct {
	mu    sync.Mutex
	calls []platform.InboundMessage
}

func (m *mockEmptyHandler) HandleEmpty(_ context.Context, msg platform.InboundMessage) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, msg)
}

func (m *mockEmptyHandler) callCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.calls)
}

// TestGateway_WithEmptyMessageHandler_OptionInstalls verifies that the option
// can be constructed without panic and that the option compiles into the
// Gateway. The behavior test (Dispatch triggers HandleEmpty) is in Task 4.
func TestGateway_WithEmptyMessageHandler_OptionInstalls(t *testing.T) {
	st := newMock()
	handler := &mockEmptyHandler{}
	g := msgingest.New(st, 100*time.Millisecond, noopHandler{},
		msgingest.WithEmptyMessageHandler(handler),
	)
	if g == nil {
		t.Fatal("New returned nil")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/tengyongzhi/work/bot-workspaces/openbee2 && go test ./internal/domain/msgingest/ -run TestGateway_WithEmptyMessageHandler_OptionInstalls -v`
Expected: Compile error — `undefined: msgingest.WithEmptyMessageHandler`.

- [ ] **Step 3: Add the field and option to `gateway.go`**

Edit `internal/domain/msgingest/gateway.go`. In the `Gateway` struct (currently at lines ~53-64), add `emptyHandler` as the last field:

```go
type Gateway struct {
	msgStore       MessageStore
	debounce       time.Duration
	sessions       map[string]*debounceState
	seen           map[string]struct{}
	seenPrev       map[string]struct{}
	mu             sync.Mutex
	out            chan IngestedMessage
	cmdCh          chan commandTask
	commandHandler CommandHandler
	botNameREs     map[string]*regexp.Regexp
	emptyHandler   EmptyMessageHandler
}
```

Below the existing `WithPlatformBotNames` function (around line 79) add a new option:

```go
// WithEmptyMessageHandler registers a handler invoked when a message is empty
// after the bot @mention is stripped. The empty-message short-circuit in
// Dispatch (no DB write, no debounce, no downstream emit) runs unconditionally
// once content is detected as empty; this option only controls whether a reply
// is sent. When no handler is registered, empty messages are silently dropped.
func WithEmptyMessageHandler(h EmptyMessageHandler) Option {
	return func(g *Gateway) { g.emptyHandler = h }
}
```

- [ ] **Step 4: Run all msgingest tests to verify nothing regressed**

Run: `cd /Users/tengyongzhi/work/bot-workspaces/openbee2 && go test ./internal/domain/msgingest/ -v`
Expected: All tests `PASS` including the new `TestGateway_WithEmptyMessageHandler_OptionInstalls`.

- [ ] **Step 5: Commit**

```bash
git add internal/domain/msgingest/gateway.go internal/domain/msgingest/gateway_test.go
git commit -m "msgingest: add WithEmptyMessageHandler option to Gateway"
```

---

## Task 4: Insert empty-message check in `Gateway.Dispatch` (TDD)

**Files:**
- Modify: `internal/domain/msgingest/gateway.go` (Dispatch body, around lines 137-167)
- Modify: `internal/domain/msgingest/gateway_test.go` (add four behavior tests)

This is the core behavior change. The empty check runs **after** dedup (so platform retries don't double-reply) and **before** the command check (so empty content never enters DB or debounce).

- [ ] **Step 1: Write the failing behavior tests**

Append to `internal/domain/msgingest/gateway_test.go`:

```go
// TestGateway_EmptyAfterStrip_InvokesEmptyHandler verifies that a message
// becoming empty after stripBotMention is routed to EmptyMessageHandler,
// never written to DB, and never emitted.
func TestGateway_EmptyAfterStrip_InvokesEmptyHandler(t *testing.T) {
	st := newMock()
	handler := &mockEmptyHandler{}
	g := msgingest.New(st, 100*time.Millisecond, noopHandler{},
		msgingest.WithPlatformBotNames(map[string]string{"test": "Bot"}),
		msgingest.WithEmptyMessageHandler(handler),
	)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go g.Run(ctx)

	g.Dispatch(inbound("s1", "@Bot", "m1"))

	// Wait briefly for the synchronous HandleEmpty call.
	deadline := time.After(300 * time.Millisecond)
	for handler.callCount() == 0 {
		select {
		case <-deadline:
			t.Fatal("HandleEmpty not invoked within 300ms")
		case <-time.After(10 * time.Millisecond):
		}
	}

	if got := handler.callCount(); got != 1 {
		t.Errorf("HandleEmpty calls = %d, want 1", got)
	}
	if len(st.batches) != 0 {
		t.Errorf("CreateBatch should not have been called, got %d batches", len(st.batches))
	}
	select {
	case msg := <-g.Out():
		t.Errorf("expected no emit, got %+v", msg)
	case <-time.After(200 * time.Millisecond):
		// expected
	}
}

// TestGateway_EmptyAfterStrip_DedupStillApplies verifies that two empty
// messages sharing one PlatformMessageID produce exactly one HandleEmpty call.
func TestGateway_EmptyAfterStrip_DedupStillApplies(t *testing.T) {
	st := newMock()
	handler := &mockEmptyHandler{}
	g := msgingest.New(st, 100*time.Millisecond, noopHandler{},
		msgingest.WithPlatformBotNames(map[string]string{"test": "Bot"}),
		msgingest.WithEmptyMessageHandler(handler),
	)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go g.Run(ctx)

	g.Dispatch(inbound("s1", "@Bot", "pm-dup"))
	g.Dispatch(inbound("s1", "@Bot", "pm-dup")) // same PlatformMessageID → dropped

	time.Sleep(150 * time.Millisecond)

	if got := handler.callCount(); got != 1 {
		t.Errorf("HandleEmpty calls = %d, want 1 (duplicate should be dedup'd)", got)
	}
}

// TestGateway_EmptyAfterStrip_NoHandler_NoOp verifies that when no handler is
// installed, empty messages still short-circuit (no DB write, no emit) — they
// are silently dropped. This preserves backward compatibility for tests that
// don't install the option.
func TestGateway_EmptyAfterStrip_NoHandler_NoOp(t *testing.T) {
	st := newMock()
	g := msgingest.New(st, 100*time.Millisecond, noopHandler{},
		msgingest.WithPlatformBotNames(map[string]string{"test": "Bot"}),
	)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go g.Run(ctx)

	g.Dispatch(inbound("s1", "@Bot", "m1"))

	// Wait past the debounce window to make sure nothing surfaces.
	time.Sleep(250 * time.Millisecond)

	if len(st.batches) != 0 {
		t.Errorf("CreateBatch should not have been called for empty msg, got %d batches", len(st.batches))
	}
	select {
	case msg := <-g.Out():
		t.Errorf("expected no emit, got %+v", msg)
	default:
		// expected
	}
}

// TestGateway_NonEmpty_DoesNotInvokeEmptyHandler verifies that normal (non-empty)
// messages bypass the empty branch and reach the debounce path as before.
func TestGateway_NonEmpty_DoesNotInvokeEmptyHandler(t *testing.T) {
	st := newMock()
	handler := &mockEmptyHandler{}
	g := msgingest.New(st, 100*time.Millisecond, noopHandler{},
		msgingest.WithPlatformBotNames(map[string]string{"test": "Bot"}),
		msgingest.WithEmptyMessageHandler(handler),
	)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go g.Run(ctx)

	g.Dispatch(inbound("s1", "@Bot hello", "m1"))

	select {
	case msg := <-g.Out():
		if msg.Content != "hello" {
			t.Errorf("emitted content = %q, want %q", msg.Content, "hello")
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout waiting for non-empty message to be emitted")
	}

	if got := handler.callCount(); got != 0 {
		t.Errorf("HandleEmpty calls = %d, want 0 for non-empty msg", got)
	}
}
```

- [ ] **Step 2: Run the new tests to verify they fail**

Run: `cd /Users/tengyongzhi/work/bot-workspaces/openbee2 && go test ./internal/domain/msgingest/ -run TestGateway_EmptyAfterStrip -v && go test ./internal/domain/msgingest/ -run TestGateway_NonEmpty_DoesNotInvokeEmptyHandler -v`
Expected:
- `TestGateway_EmptyAfterStrip_InvokesEmptyHandler` FAIL (HandleEmpty not called; the empty msg goes into debounce and CreateBatch gets called)
- `TestGateway_EmptyAfterStrip_DedupStillApplies` FAIL (same reason)
- `TestGateway_EmptyAfterStrip_NoHandler_NoOp` FAIL (CreateBatch gets called with empty content)
- `TestGateway_NonEmpty_DoesNotInvokeEmptyHandler` PASS (already correct, since HandleEmpty is never called yet)

- [ ] **Step 3: Insert the empty-check branch in `Dispatch`**

Edit `internal/domain/msgingest/gateway.go`. The current `Dispatch` body has dedup followed by the command check at line ~158. Insert the empty branch between them. The block from line 137 to about line 167 should become:

```go
// Dispatch is called by a platform receiver for each inbound message.
// All seen-map and debounce-state mutations are protected by g.mu.
func (g *Gateway) Dispatch(msg platform.InboundMessage) {
	stripped := g.stripBotMention(msg.Content, msg.Platform)
	g.mu.Lock()

	if msg.PlatformMessageID != "" {
		_, dup := g.seen[msg.PlatformMessageID]
		if !dup {
			_, dup = g.seenPrev[msg.PlatformMessageID]
		}
		if dup {
			g.mu.Unlock()
			log.Info("duplicate dropped", zap.String("platformMsgID", msg.PlatformMessageID))
			return
		}
		if len(g.seen) >= seenMaxSize {
			g.seenPrev = g.seen
			g.seen = make(map[string]struct{})
		}
		g.seen[msg.PlatformMessageID] = struct{}{}
	}

	// Empty-message short-circuit: must come after dedup (so platform retries
	// don't cause double replies) and before the command/debounce branches (so
	// empty content never enters DB or accumulation state).
	if strings.TrimSpace(stripped) == "" {
		g.mu.Unlock()
		log.Info("empty message after strip",
			zap.String("sessionKey", msg.SessionKey),
			zap.String("platform", msg.Platform),
			zap.String("platformMsgID", msg.PlatformMessageID))
		if g.emptyHandler != nil {
			g.emptyHandler.HandleEmpty(context.Background(), msg)
		}
		return
	}

	if g.commandHandler.IsCommand(stripped) {
		g.mu.Unlock()
		select {
		case g.cmdCh <- commandTask{stripped, msg}:
		default:
			log.Warn("command channel full, dropping command", zap.String("sessionKey", msg.SessionKey))
		}
		return
	}

	// ...existing debounce accumulation (unchanged below)...
```

(Leave the rest of the function — from "Accumulate into debounce state." through the final `g.mu.Unlock()` — unchanged.)

- [ ] **Step 4: Run the full msgingest test suite to verify all tests pass**

Run: `cd /Users/tengyongzhi/work/bot-workspaces/openbee2 && go test ./internal/domain/msgingest/ -v`
Expected: All tests `PASS`, including:
- Pre-existing tests (`TestGateway_Dedup_InMemory`, `TestGateway_Debounce_*`, `TestGateway_BotMention_*`, `TestStripBotMention`, etc.)
- All four new empty-branch tests
- The previously-installed `TestGateway_WithEmptyMessageHandler_OptionInstalls`

- [ ] **Step 5: Run the whole module's tests to catch broader regressions**

Run: `cd /Users/tengyongzhi/work/bot-workspaces/openbee2 && go build ./... && go test ./...`
Expected: Build succeeds, all tests pass. Long-running platform tests may be slow but should not fail.

- [ ] **Step 6: Commit**

```bash
git add internal/domain/msgingest/gateway.go internal/domain/msgingest/gateway_test.go
git commit -m "msgingest: filter empty messages after bot-mention strip"
```

---

## Task 5: Wire `DefaultEmptyMessageHandler` into production Gateway

**Files:**
- Modify: `internal/app/app.go:172-179` (the production `msgingest.New` call)

The production Gateway is constructed in `internal/app/app.go`. It already receives `sendersByPlatform` (line 161). We need to append the new option.

- [ ] **Step 1: Edit the `msgingest.New` call**

Edit `internal/app/app.go`. The current call (lines 172-179):

```go
ingest := msgingest.New(s.msgStore, cfg.Bee.MessageDebounce, cmdChain,
    msgingest.WithPlatformBotNames(map[string]string{
        feishu.PlatformID:   cfg.Bee.Platforms.Feishu.BotName,
        dingtalk.PlatformID: cfg.Bee.Platforms.DingTalk.BotName,
        wecom.PlatformID:    cfg.Bee.Platforms.WeCom.BotName,
        telegram.PlatformID: cfg.Bee.Platforms.Telegram.BotName,
        weixin.PlatformID:   cfg.Bee.Platforms.Weixin.BotName,
    }))
```

becomes:

```go
ingest := msgingest.New(s.msgStore, cfg.Bee.MessageDebounce, cmdChain,
    msgingest.WithPlatformBotNames(map[string]string{
        feishu.PlatformID:   cfg.Bee.Platforms.Feishu.BotName,
        dingtalk.PlatformID: cfg.Bee.Platforms.DingTalk.BotName,
        wecom.PlatformID:    cfg.Bee.Platforms.WeCom.BotName,
        telegram.PlatformID: cfg.Bee.Platforms.Telegram.BotName,
        weixin.PlatformID:   cfg.Bee.Platforms.Weixin.BotName,
    }),
    msgingest.WithEmptyMessageHandler(msgingest.NewDefaultEmptyMessageHandler(sendersByPlatform)),
)
```

Leave the `localIngest` construction (line 180) **untouched** — the local-chat ingest path doesn't have IM users to reply to, so it correctly remains opt-out.

- [ ] **Step 2: Build the binary to verify no compile errors**

Run: `cd /Users/tengyongzhi/work/bot-workspaces/openbee2 && go build ./...`
Expected: Build succeeds (no output, exit 0).

- [ ] **Step 3: Run app-package tests if any exist**

Run: `cd /Users/tengyongzhi/work/bot-workspaces/openbee2 && go test ./internal/app/...`
Expected: PASS (or "no test files" — both are acceptable).

- [ ] **Step 4: Commit**

```bash
git add internal/app/app.go
git commit -m "app: wire DefaultEmptyMessageHandler into bee ingest gateway"
```

---

## Task 6: Update CHANGELOG (English)

**Files:**
- Modify: `CHANGELOG.md`

Per project conventions, all CHANGELOG entries are written in English.

- [ ] **Step 1: Find or create the next unreleased section**

Open `CHANGELOG.md` and locate the topmost unreleased section (or the section for the next version about to ship). If a new section is needed, follow the existing date/version formatting style already used in the file.

- [ ] **Step 2: Append the new entry**

Under the relevant section, add a line in the existing list style. Example wording:

```markdown
- Filter empty messages after bot @mention strip: reply with a localized hint and skip DB write, debounce, and LLM invocation.
```

- [ ] **Step 3: Commit**

```bash
git add CHANGELOG.md
git commit -m "docs: changelog entry for empty message filter"
```

---

## Task 7: Final end-to-end verification

- [ ] **Step 1: Full build**

Run: `cd /Users/tengyongzhi/work/bot-workspaces/openbee2 && go build ./...`
Expected: exit 0, no output.

- [ ] **Step 2: Full test suite**

Run: `cd /Users/tengyongzhi/work/bot-workspaces/openbee2 && go test ./...`
Expected: All packages `ok` or `[no test files]`. No `FAIL`.

- [ ] **Step 3: `go vet`**

Run: `cd /Users/tengyongzhi/work/bot-workspaces/openbee2 && go vet ./...`
Expected: exit 0, no warnings.

- [ ] **Step 4: Confirm log line is observable**

Read back `internal/domain/msgingest/gateway.go` and confirm the `log.Info("empty message after strip", ...)` is present in the new branch (so operators can grep for it in production).

- [ ] **Step 5: Final git status check**

Run: `git status` and `git log --oneline -10`
Expected: Working tree clean; the last several commits are the ones added by this plan (i18n → handler → option → dispatch → wiring → changelog).

---

## Out of Scope (do not implement)

- Per-session rate limiting for empty replies.
- Persisting empty messages with a `dropped` status row.
- Metrics counters or dashboards.
- Changes to any `internal/platform/*` receiver. The single chokepoint in `Gateway.Dispatch` covers all platforms uniformly.
