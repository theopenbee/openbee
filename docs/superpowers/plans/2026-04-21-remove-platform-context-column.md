# Remove `platform_context` Column Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove the redundant `platform_context` DB column and instead extract platform-native context from the existing `raw` field on demand at the feeder/dispatcher layer.

**Architecture:** Add an extractor registry to `internal/platform`, export `ExtractContext(raw string) string` from each platform handler, and call `platform.ExtractContext(platform, raw)` in feeder and dispatcher instead of reading from `ClaimedMessage.PlatformContext`. The `raw` field (already stored) replaces `platform_context` in `ClaimBatch`.

**Tech Stack:** Go, SQLite (modernc.org/sqlite), Feishu/DingTalk/WeCom SDK types.

---

## File Map

| File | Change |
|---|---|
| `internal/platform/context.go` | Add `RegisterExtractor` + `ExtractContext` |
| `internal/platform/context_test.go` | New: test registry |
| `internal/platform/feishu/handler.go` | `buildFeishuContext` → exported `ExtractContext(raw string)` |
| `internal/platform/dingtalk/handler.go` | `buildDingTalkContext` → exported `ExtractContext(raw string)` |
| `internal/platform/wecom/handler.go` | `buildWeComContext` → exported `ExtractContext(raw string)` |
| `internal/platform/interfaces.go` | Remove `PlatformContext` from `InboundMessage` |
| `internal/app/app.go` | Register extractors in `buildPlatforms` |
| `internal/infra/store/db.go` | Remove migration 41 entry |
| `internal/infra/store/message_store.go` | `BatchMsg` remove `PlatformContext`; `ClaimedMessage` remove `PlatformContext` add `Raw`; `ClaimBatch` SQL; `CreateBatch` args |
| `internal/infra/store/message_store_test.go` | Rename + rewrite `TestMessageStore_CreateBatch_PlatformContext` |
| `internal/domain/bee/feeder.go` | Use `platform.ExtractContext(m.Platform, m.Raw)` |
| `internal/domain/bee/feeder_internal_test.go` | Register test extractor, use `Raw` field |
| `internal/domain/task/dispatcher.go` | Use `platform.ExtractContext(t.ReplyTo.Platform, t.ReplyTo.Raw)` |
| `internal/domain/task/dispatcher_internal_test.go` | Register test extractor, use `Raw` field |

---

### Task 1: Add extractor registry to `internal/platform/context.go`

**Files:**
- Modify: `internal/platform/context.go`
- Create: `internal/platform/context_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/platform/context_test.go`:

```go
package platform_test

import (
	"testing"

	"github.com/theopenbee/openbee/internal/platform"
)

func TestExtractContext_Registered(t *testing.T) {
	platform.RegisterExtractor("testplatform", func(_ string) string {
		return `{"testplatform":{"key":"value"}}`
	})
	got := platform.ExtractContext("testplatform", "ignored-raw")
	if got != `{"testplatform":{"key":"value"}}` {
		t.Errorf("unexpected result: %q", got)
	}
}

func TestExtractContext_Unregistered(t *testing.T) {
	got := platform.ExtractContext("no-such-platform", "{}")
	if got != "" {
		t.Errorf("expected empty string for unregistered platform, got %q", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/platform/ -run TestExtractContext -v
```

Expected: FAIL — `platform.RegisterExtractor` and `platform.ExtractContext` undefined.

- [ ] **Step 3: Add registry to `internal/platform/context.go`**

Append to the file after the existing `BuildPlatformContext` function:

```go
var extractors = map[string]func(string) string{}

// RegisterExtractor registers a platform-specific context extractor.
// Called once at server startup per enabled platform.
func RegisterExtractor(name string, fn func(string) string) {
	extractors[name] = fn
}

// ExtractContext returns a platform_context JSON string for the given platform
// and raw event payload. Returns "" if no extractor is registered or raw is empty.
func ExtractContext(platformName, raw string) string {
	if raw == "" {
		return ""
	}
	fn, ok := extractors[platformName]
	if !ok {
		return ""
	}
	return fn(raw)
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./internal/platform/ -run TestExtractContext -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/platform/context.go internal/platform/context_test.go
git commit -m "feat: add ExtractContext registry to platform package"
```

---

### Task 2: Export `ExtractContext` from Feishu handler

**Files:**
- Modify: `internal/platform/feishu/handler.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/platform/feishu/handler_test.go`:

```go
func TestExtractContext_ValidFeishuRaw(t *testing.T) {
	// Minimal Feishu P2MessageReceiveV1 JSON with the fields we extract.
	raw := `{"schema":"2.0","header":{"event_id":"evt1","event_type":"im.message.receive_v1"},"event":{"sender":{"sender_id":{"open_id":"ou_abc","union_id":"on_abc"},"tenant_key":"tk1"},"message":{"message_id":"om_1","chat_id":"oc_xyz","chat_type":"group"}}}`
	got := ExtractContext(raw)
	if got == "" {
		t.Fatal("expected non-empty context")
	}
	if !strings.Contains(got, `"open_id"`) {
		t.Errorf("expected open_id in context, got: %q", got)
	}
	if !strings.Contains(got, "ou_abc") {
		t.Errorf("expected open_id value in context, got: %q", got)
	}
}

func TestExtractContext_InvalidRaw(t *testing.T) {
	got := ExtractContext("not-json")
	if got != "" {
		t.Errorf("expected empty string for invalid raw, got %q", got)
	}
}
```

Also add `"strings"` to the test file imports if not already present.

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/platform/feishu/ -run TestExtractContext -v
```

Expected: FAIL — `ExtractContext` undefined.

- [ ] **Step 3: Replace `buildFeishuContext` with exported `ExtractContext`**

In `internal/platform/feishu/handler.go`, replace the private function:

```go
// buildFeishuContext → remove this entire function
func buildFeishuContext(sender *larkim.EventSender, msg *larkim.EventMessage) string {
    return platform.BuildPlatformContext("feishu", map[string]string{ ... })
}
```

Add the exported version at the same location in the file:

```go
// ExtractContext extracts platform-native fields from a raw Feishu P2MessageReceiveV1 JSON payload.
// Returns "" if raw is not a valid event or required fields are absent.
func ExtractContext(raw string) string {
	var event larkim.P2MessageReceiveV1
	if err := json.Unmarshal([]byte(raw), &event); err != nil || event.Event == nil {
		return ""
	}
	sender := event.Event.Sender
	msg := event.Event.Message
	if sender == nil || msg == nil || sender.SenderId == nil {
		return ""
	}
	return platform.BuildPlatformContext("feishu", map[string]string{
		"open_id":    utils.DerefStrOrEmpty(sender.SenderId.OpenId),
		"union_id":   utils.DerefStrOrEmpty(sender.SenderId.UnionId),
		"chat_id":    utils.DerefStrOrEmpty(msg.ChatId),
		"chat_type":  utils.DerefStrOrEmpty(msg.ChatType),
		"tenant_key": utils.DerefStrOrEmpty(sender.TenantKey),
		"message_id": utils.DerefStrOrEmpty(msg.MessageId),
	})
}
```

- [ ] **Step 4: Update the call site in the handler**

In `internal/platform/feishu/handler.go`, find the existing call to `buildFeishuContext` (around line 173) and replace:

```go
// Before
platformCtxJSON := buildFeishuContext(sender, msg)
dispatch(platform.InboundMessage{
    ...
    PlatformContext:   platformCtxJSON,
})

// After — remove platformCtxJSON line and PlatformContext field entirely
dispatch(platform.InboundMessage{
    Platform:          "feishu",
    SenderID:          senderID,
    SessionKey:        "feishu:" + *msg.ChatId + ":" + senderID,
    Content:           textContent,
    Raw:               string(rawBytes),
    PlatformMessageID: utils.DerefStrOrEmpty(msg.MessageId),
    MessageTime:       utils.ParseMillis(msg.CreateTime),
})
```

Note: `PlatformContext` field still exists on `InboundMessage` at this point; it is removed in Task 5. Just stop assigning it here.

- [ ] **Step 5: Run tests**

```bash
go test ./internal/platform/feishu/ -run TestExtractContext -v
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/platform/feishu/handler.go internal/platform/feishu/handler_test.go
git commit -m "feat: export feishu.ExtractContext from raw, remove buildFeishuContext"
```

---

### Task 3: Export `ExtractContext` from DingTalk handler

**Files:**
- Modify: `internal/platform/dingtalk/handler.go`
- Modify: `internal/platform/dingtalk/handler_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/platform/dingtalk/handler_test.go`:

```go
func TestExtractContext_ValidDingTalkRaw(t *testing.T) {
	raw := `{"senderStaffId":"emp001","senderNick":"Alice","senderCorpId":"corp1","conversationId":"conv1","conversationType":"1","conversationTitle":"Test","isAdmin":false,"chatbotCorpId":"botcorp1","msgId":"msg1","createAt":1700000000000}`
	got := ExtractContext(raw)
	if got == "" {
		t.Fatal("expected non-empty context")
	}
	if !strings.Contains(got, "emp001") {
		t.Errorf("expected senderStaffId in context, got: %q", got)
	}
}

func TestExtractContext_InvalidDingTalkRaw(t *testing.T) {
	got := ExtractContext("not-json")
	if got != "" {
		t.Errorf("expected empty string for invalid raw, got %q", got)
	}
}
```

Also ensure `"strings"` is in the test file imports.

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/platform/dingtalk/ -run TestExtractContext -v
```

Expected: FAIL — `ExtractContext` undefined.

- [ ] **Step 3: Replace `buildDingTalkContext` with exported `ExtractContext`**

In `internal/platform/dingtalk/handler.go`, replace:

```go
// Remove this:
func buildDingTalkContext(data *chatbot.BotCallbackDataModel) string {
    return platform.BuildPlatformContext("dingtalk", map[string]string{ ... })
}
```

Add:

```go
// ExtractContext extracts platform-native fields from a raw DingTalk BotCallbackDataModel JSON payload.
func ExtractContext(raw string) string {
	var data chatbot.BotCallbackDataModel
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		return ""
	}
	return platform.BuildPlatformContext("dingtalk", map[string]string{
		"sender_staff_id":    data.SenderStaffId,
		"sender_nick":        data.SenderNick,
		"sender_corp_id":     data.SenderCorpId,
		"conversation_id":    data.ConversationId,
		"conversation_type":  data.ConversationType,
		"conversation_title": data.ConversationTitle,
		"is_admin":           strconv.FormatBool(data.IsAdmin),
		"chatbot_corp_id":    data.ChatbotCorpId,
	})
}
```

Ensure `"encoding/json"` is in the imports (it should already be there).

- [ ] **Step 4: Update the call site in the handler**

Find the existing call to `buildDingTalkContext` (around line 132) and replace:

```go
// Before
platformCtxJSON := buildDingTalkContext(data)
msg := platform.InboundMessage{
    ...
    PlatformContext:  platformCtxJSON,
}

// After — remove platformCtxJSON line, remove PlatformContext field
msg := platform.InboundMessage{
    Platform:         "dingtalk",
    SenderID:         data.SenderStaffId,
    SessionKey:       "dingtalk:" + data.ConversationId + ":" + data.SenderStaffId,
    Content:          textContent,
    Raw:              string(rawBytes),
    PlatformMessageID: data.MsgId,
    MessageTime:      data.CreateAt,
}
```

- [ ] **Step 5: Run tests**

```bash
go test ./internal/platform/dingtalk/ -run TestExtractContext -v
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/platform/dingtalk/handler.go internal/platform/dingtalk/handler_test.go
git commit -m "feat: export dingtalk.ExtractContext from raw, remove buildDingTalkContext"
```

---

### Task 4: Export `ExtractContext` from WeCom handler

**Files:**
- Modify: `internal/platform/wecom/handler.go`
- Modify: `internal/platform/wecom/handler_test.go`

Background: WeCom `raw` stores a serialized `WsFrame{Headers, Body}` where `Body` is a `messageBody` JSON. The extractor must unmarshal the frame, then unmarshal the body, then replicate the `chatID` derivation from `processMessage`.

- [ ] **Step 1: Write the failing test**

Add to `internal/platform/wecom/handler_test.go`:

```go
func TestExtractContext_ValidWeComRaw(t *testing.T) {
	// WsFrame with a messageBody in Body
	body := `{"msgid":"msg1","aibotid":"bot1","chatid":"","chattype":"single","from":{"userid":"user1"},"create_time":1700000000}`
	frame := `{"cmd":"aibot_callback","headers":{"req_id":"req1"},"body":` + body + `}`
	got := ExtractContext(frame)
	if got == "" {
		t.Fatal("expected non-empty context")
	}
	if !strings.Contains(got, "user1") {
		t.Errorf("expected userid in context, got: %q", got)
	}
}

func TestExtractContext_InvalidWeComRaw(t *testing.T) {
	got := ExtractContext("not-json")
	if got != "" {
		t.Errorf("expected empty string for invalid raw, got %q", got)
	}
}
```

Also ensure `"strings"` is in the test file imports.

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/platform/wecom/ -run TestExtractContext -v
```

Expected: FAIL — `ExtractContext` undefined.

- [ ] **Step 3: Replace `buildWeComContext` with exported `ExtractContext`**

In `internal/platform/wecom/handler.go`, replace:

```go
// Remove this:
func buildWeComContext(body messageBody, chatID, senderID string) string {
    return platform.BuildPlatformContext("wecom", map[string]string{ ... })
}
```

Add:

```go
// ExtractContext extracts platform-native fields from a raw WeCom WsFrame JSON payload.
func ExtractContext(raw string) string {
	var frame WsFrame
	if err := json.Unmarshal([]byte(raw), &frame); err != nil {
		return ""
	}
	var body messageBody
	if err := json.Unmarshal(frame.Body, &body); err != nil {
		return ""
	}
	chatID := body.From.UserID
	if body.ChatType == chatTypeGroup {
		chatID = body.ChatID
	}
	senderID := body.From.UserID
	return platform.BuildPlatformContext("wecom", map[string]string{
		"userid":   senderID,
		"chatid":   chatID,
		"chattype": body.ChatType,
		"aibotid":  body.AiBotID,
		"msgid":    body.MsgID,
	})
}
```

- [ ] **Step 4: Update the call site in the handler**

Find the existing call to `buildWeComContext` (around line 275) and replace:

```go
// Before
rawBytes, _ := json.Marshal(frame)
platformCtxJSON := buildWeComContext(body, chatID, senderID)
dispatch(platform.InboundMessage{
    ...
    PlatformContext:   platformCtxJSON,
})

// After — remove platformCtxJSON line, remove PlatformContext field
rawBytes, _ := json.Marshal(frame)
dispatch(platform.InboundMessage{
    Platform:          "wecom",
    SenderID:          senderID,
    SessionKey:        "wecom:" + chatID + ":" + senderID,
    Content:           content,
    RawContent:        rawText,
    Raw:               string(rawBytes),
    PlatformMessageID: body.MsgID,
    MessageTime:       body.CreateTime * 1000,
})
```

- [ ] **Step 5: Run tests**

```bash
go test ./internal/platform/wecom/ -run TestExtractContext -v
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/platform/wecom/handler.go internal/platform/wecom/handler_test.go
git commit -m "feat: export wecom.ExtractContext from raw, remove buildWeComContext"
```

---

### Task 5: Remove `PlatformContext` from `InboundMessage`; register extractors in `app.go`

**Files:**
- Modify: `internal/platform/interfaces.go`
- Modify: `internal/app/app.go`

- [ ] **Step 1: Remove `PlatformContext` from `InboundMessage`**

In `internal/platform/interfaces.go`, remove the field:

```go
// Before
type InboundMessage struct {
    Platform          string
    SenderID          string
    SessionKey        string
    Content           string
    RawContent        string
    Raw               string
    PlatformMessageID string
    MessageTime       int64
    PlatformContext   string  // ← remove this line
}

// After
type InboundMessage struct {
    Platform          string
    SenderID          string
    SessionKey        string
    Content           string
    RawContent        string
    Raw               string
    PlatformMessageID string
    MessageTime       int64
}
```

- [ ] **Step 2: Verify the build**

```bash
go build ./...
```

Expected: any remaining references to `InboundMessage.PlatformContext` will error here. The only references should be in `dispatcher.go` and `dispatcher_internal_test.go` which are handled in Task 8 — check that these are the only errors. If other files fail, fix them now.

- [ ] **Step 3: Register extractors in `internal/app/app.go`**

In `internal/app/app.go`, find the `buildPlatforms` function and add registrations at the top of the function body. Also add the required imports:

```go
import (
    // existing imports ...
    "github.com/theopenbee/openbee/internal/platform"
    "github.com/theopenbee/openbee/internal/platform/dingtalk"
    "github.com/theopenbee/openbee/internal/platform/feishu"
    "github.com/theopenbee/openbee/internal/platform/wecom"
)

func buildPlatforms(...) []platform.Platform {
    // Add at the very top:
    platform.RegisterExtractor("feishu", feishu.ExtractContext)
    platform.RegisterExtractor("dingtalk", dingtalk.ExtractContext)
    platform.RegisterExtractor("wecom", wecom.ExtractContext)

    mediaSvc := media.NewService()
    // ... rest of existing function unchanged
}
```

- [ ] **Step 4: Verify the build**

```bash
go build ./...
```

Expected: builds cleanly (except `dispatcher.go` and `dispatcher_internal_test.go` which still reference `PlatformContext` — those are fixed in Task 8).

- [ ] **Step 5: Commit**

```bash
git add internal/platform/interfaces.go internal/app/app.go
git commit -m "refactor: remove PlatformContext from InboundMessage, register extractors in app"
```

---

### Task 6: Update store layer — remove migration 41, update `BatchMsg`/`ClaimBatch`/`CreateBatch`

**Files:**
- Modify: `internal/infra/store/db.go`
- Modify: `internal/infra/store/message_store.go`
- Modify: `internal/infra/store/message_store_test.go`

- [ ] **Step 1: Write the failing test**

In `internal/infra/store/message_store_test.go`, replace `TestMessageStore_CreateBatch_PlatformContext` entirely with:

```go
func TestMessageStore_CreateBatch_RawRoundtrip(t *testing.T) {
	s := setupMessageStore(t)
	ctx := context.Background()

	msg := BatchMsg{
		ID: "raw-1", SessionKey: "s1", Platform: "feishu",
		Content: "hello", Raw: `{"event":{"sender":{"sender_id":{"open_id":"ou_abc"}}}}`,
		PlatformMsgID: "pmsg-raw-1",
		MessageTime: time.Now().UnixMilli(), Status: "received", MergedInto: "",
	}

	inserted, err := s.CreateBatch(ctx, []BatchMsg{msg})
	if err != nil {
		t.Fatalf("CreateBatch error: %v", err)
	}
	if inserted != 1 {
		t.Fatalf("expected 1 row, got %d", inserted)
	}

	claimed, err := s.ClaimBatch(ctx, 10)
	if err != nil {
		t.Fatalf("ClaimBatch error: %v", err)
	}
	if len(claimed) != 1 {
		t.Fatalf("expected 1 claimed message, got %d", len(claimed))
	}
	if claimed[0].Raw != msg.Raw {
		t.Errorf("Raw mismatch\nwant: %s\ngot:  %s", msg.Raw, claimed[0].Raw)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/infra/store/ -run TestMessageStore_CreateBatch_RawRoundtrip -v
```

Expected: FAIL — `BatchMsg` has no field `PlatformContext` (actually it does still exist, so the test should fail because `ClaimedMessage` still has `PlatformContext` not `Raw`). The test will compile but the assertion `claimed[0].Raw` won't exist yet.

- [ ] **Step 3: Remove migration 41 from `internal/infra/store/db.go`**

Find and delete the entire migration 41 entry:

```go
// Remove this block:
{
    version: 41,
    name:    "add_platform_context_to_platform_messages",
    sql:     `ALTER TABLE bee_platform_messages ADD COLUMN platform_context TEXT NOT NULL DEFAULT ''`,
},
```

- [ ] **Step 4: Update `BatchMsg` — remove `PlatformContext` field**

In `internal/infra/store/message_store.go`, update the struct:

```go
// Before
type BatchMsg struct {
    ID              string
    SessionKey      string
    Platform        string
    Content         string
    Raw             string
    PlatformMsgID   string
    PlatformContext string  // ← remove
    MessageTime     int64
    Status          string
    MergedInto      string
}

// After
type BatchMsg struct {
    ID            string
    SessionKey    string
    Platform      string
    Content       string
    Raw           string
    PlatformMsgID string
    MessageTime   int64
    Status        string
    MergedInto    string
}
```

- [ ] **Step 5: Update `ClaimedMessage` — remove `PlatformContext`, add `Raw`**

```go
// Before
type ClaimedMessage struct {
    ...
    PlatformContext string
}

// After
type ClaimedMessage struct {
    ID         string
    SessionKey string
    Platform   string
    Content    string
    Raw        string
}
```

- [ ] **Step 6: Update `ClaimBatch` SQL and scan**

Find the SELECT in `ClaimBatch` (around line 144) and update:

```go
// Before
rows, err := tx.QueryContext(ctx,
    `SELECT id, session_key, platform, content, platform_context
     FROM bee_platform_messages m
     WHERE status = ?
       AND session_key NOT IN ( ... )
     ...`)
// scan: rows.Scan(&m.ID, &m.SessionKey, &m.Platform, &m.Content, &m.PlatformContext)

// After
rows, err := tx.QueryContext(ctx,
    `SELECT id, session_key, platform, content, raw
     FROM bee_platform_messages m
     WHERE status = ?
       AND session_key NOT IN ( ... )
     ...`)
// scan: rows.Scan(&m.ID, &m.SessionKey, &m.Platform, &m.Content, &m.Raw)
```

- [ ] **Step 7: Update `CreateBatch` INSERT — remove `platform_context`**

In `internal/infra/store/message_store.go`, update the `CreateBatch` function (around line 274):

```go
// Before (12 columns, 12 placeholders per row)
placeholders := strings.Repeat("(?,?,?,?,?,?,?,?,?,?,?,?),", len(msgs))
args := make([]any, 0, len(msgs)*12)
for _, m := range msgs {
    args = append(args, m.ID, m.SessionKey, m.Platform, m.Content, m.Raw,
        m.PlatformMsgID, m.PlatformContext, mt, m.Status, m.MergedInto, now, now)
}
// INSERT columns: id, session_key, platform, content, raw, platform_msg_id, platform_context, received_at, status, merged_into, created_at, updated_at

// After (11 columns, 11 placeholders per row)
placeholders := strings.Repeat("(?,?,?,?,?,?,?,?,?,?,?),", len(msgs))
args := make([]any, 0, len(msgs)*11)
for _, m := range msgs {
    args = append(args, m.ID, m.SessionKey, m.Platform, m.Content, m.Raw,
        m.PlatformMsgID, mt, m.Status, m.MergedInto, now, now)
}
// INSERT columns: id, session_key, platform, content, raw, platform_msg_id, received_at, status, merged_into, created_at, updated_at
```

- [ ] **Step 8: Run tests**

```bash
go test ./internal/infra/store/ -v
```

Expected: PASS including `TestMessageStore_CreateBatch_RawRoundtrip`.

- [ ] **Step 9: Commit**

```bash
git add internal/infra/store/db.go internal/infra/store/message_store.go internal/infra/store/message_store_test.go
git commit -m "refactor: remove platform_context from store layer, ClaimBatch returns raw"
```

---

### Task 7: Update feeder to use `platform.ExtractContext`

**Files:**
- Modify: `internal/domain/bee/feeder.go`
- Modify: `internal/domain/bee/feeder_internal_test.go`

- [ ] **Step 1: Update the failing test**

In `internal/domain/bee/feeder_internal_test.go`, update `TestBuildPrompt_WithPlatformContext`:

```go
func TestBuildPrompt_WithPlatformContext(t *testing.T) {
	platform.RegisterExtractor("testplatform", func(_ string) string {
		return `{"feishu":{"open_id":"ou_abc","chat_id":"oc_xyz","chat_type":"group","tenant_key":"t1","message_id":"om_1","union_id":"on_1"}}`
	})
	msgs := []store.ClaimedMessage{
		{ID: "msg-1", Platform: "testplatform", SessionKey: "feishu:oc_xyz:ou_abc", Content: "hello", Raw: "any-raw"},
	}
	got := buildPrompt(msgs, "")

	if !strings.Contains(got, `"platform_context"`) {
		t.Errorf("expected platform_context in message_meta, got: %q", got)
	}
	if !strings.Contains(got, `"ou_abc"`) {
		t.Errorf("expected open_id value in message_meta, got: %q", got)
	}
}
```

Update `TestBuildPrompt_NoPlatformContext`:

```go
func TestBuildPrompt_NoPlatformContext(t *testing.T) {
	msgs := []store.ClaimedMessage{
		{ID: "msg-1", Platform: "local", SessionKey: "local:default:local", Content: "hello", Raw: ""},
	}
	got := buildPrompt(msgs, "")

	if strings.Contains(got, `"platform_context"`) {
		t.Errorf("platform_context should be omitted when empty, got: %q", got)
	}
}
```

Add `"github.com/theopenbee/openbee/internal/platform"` to the imports in `feeder_internal_test.go`.

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/domain/bee/ -run TestBuildPrompt -v
```

Expected: FAIL — `store.ClaimedMessage` no longer has `PlatformContext` field (compilation error), or runtime failure.

- [ ] **Step 3: Update `feeder.go` to call `platform.ExtractContext`**

Add `"github.com/theopenbee/openbee/internal/platform"` to imports in `internal/domain/bee/feeder.go`.

Find the section in `feeder.go` that builds `message_meta` (around line 348-354):

```go
// Before
if m.PlatformContext != "" {
    meta.PlatformContext = json.RawMessage(m.PlatformContext)
}

// After
if ctx := platform.ExtractContext(m.Platform, m.Raw); ctx != "" {
    meta.PlatformContext = json.RawMessage(ctx)
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/domain/bee/ -run TestBuildPrompt -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/domain/bee/feeder.go internal/domain/bee/feeder_internal_test.go
git commit -m "refactor: feeder derives platform_context from raw via ExtractContext"
```

---

### Task 8: Update dispatcher to use `platform.ExtractContext`

**Files:**
- Modify: `internal/domain/task/dispatcher.go`
- Modify: `internal/domain/task/dispatcher_internal_test.go`

- [ ] **Step 1: Update the failing tests**

In `internal/domain/task/dispatcher_internal_test.go`, update `TestBuildInstruction_WithPlatformContext`:

```go
func TestBuildInstruction_WithPlatformContext(t *testing.T) {
	platform.RegisterExtractor("testplatform", func(_ string) string {
		return `{"feishu":{"open_id":"ou_abc","chat_id":"oc_xyz"}}`
	})
	task := DispatchTask{
		TaskID:    "task-1",
		MessageID: "msg-1",
		ReplyTo: platform.InboundMessage{
			Platform: "testplatform",
			Raw:      "any-raw",
		},
		Instruction: "do something",
	}
	got := buildInstruction(task)

	if !strings.Contains(got, `"platform_context"`) {
		t.Errorf("expected platform_context in task_meta, got: %q", got)
	}
	if !strings.Contains(got, `"ou_abc"`) {
		t.Errorf("expected open_id value in task_meta, got: %q", got)
	}
	if !strings.Contains(got, "do something") {
		t.Errorf("expected instruction in output, got: %q", got)
	}
}
```

`TestBuildInstruction_NoPlatformContext` does not reference `PlatformContext` and should still pass as-is — verify it compiles cleanly.

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/domain/task/ -run TestBuildInstruction -v
```

Expected: FAIL — `platform.InboundMessage` no longer has `PlatformContext` field (compilation error from the old test code).

- [ ] **Step 3: Update `dispatcher.go`**

Add `"github.com/theopenbee/openbee/internal/platform"` to imports in `internal/domain/task/dispatcher.go`.

Find the section that builds `task_meta` (around line 256-261):

```go
// Before
if t.ReplyTo.PlatformContext != "" {
    meta.PlatformContext = json.RawMessage(t.ReplyTo.PlatformContext)
}

// After
if ctx := platform.ExtractContext(t.ReplyTo.Platform, t.ReplyTo.Raw); ctx != "" {
    meta.PlatformContext = json.RawMessage(ctx)
}
```

- [ ] **Step 4: Run all tests**

```bash
go test ./...
```

Expected: all tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/domain/task/dispatcher.go internal/domain/task/dispatcher_internal_test.go
git commit -m "refactor: dispatcher derives platform_context from raw via ExtractContext"
```

---

### Task 9: Final verification

- [ ] **Step 1: Run the full test suite**

```bash
go test ./... -count=1
```

Expected: all PASS, zero failures.

- [ ] **Step 2: Build the binary**

```bash
go build ./...
```

Expected: clean build, zero errors.

- [ ] **Step 3: Final commit if any cleanup needed**

If any minor fixes were needed during the above steps that were not committed individually, commit them now. Otherwise skip.
