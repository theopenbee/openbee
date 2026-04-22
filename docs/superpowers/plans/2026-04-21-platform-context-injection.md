# Platform Context Injection Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Inject platform-native context fields (sender IDs, chat IDs, chat type, display names, etc.) into both bee's `<message_meta>` and worker's `<task_meta>` so bee and workers can use third-party IM CLI tools without parsing raw event JSON themselves.

**Architecture:** Add a `PlatformContext string` (JSON) field to `InboundMessage`; each platform handler populates it; the context flows through the store (new `platform_context` DB column) for the bee path and through `DispatchTask.ReplyTo` for the worker path; both `messageMeta` and `taskMeta` are expanded to include it.

**Tech Stack:** Go, SQLite (modernc), encoding/json, DingTalk Stream SDK, Lark Go SDK, WeCom WebSocket

---

## File Map

| Action | File | Responsibility |
|---|---|---|
| Modify | `internal/platform/interfaces.go` | Add `PlatformContext string` to `InboundMessage` |
| Modify | `internal/infra/store/db.go` | Migration 41: add `platform_context` column |
| Modify | `internal/infra/store/message_store.go` | Add field to `BatchMsg`/`ClaimedMessage`, update SQL |
| Modify | `internal/platform/feishu/handler.go` | Populate `PlatformContext` for Feishu |
| Modify | `internal/platform/dingtalk/handler.go` | Populate `PlatformContext` for DingTalk |
| Modify | `internal/platform/wecom/handler.go` | Populate `PlatformContext` for WeCom |
| Modify | `internal/domain/bee/feeder.go` | Expand `messageMeta` with `PlatformContext` |
| Modify | `internal/domain/bee/feeder_internal_test.go` | Update + extend `buildPrompt` tests |
| Modify | `internal/domain/task/dispatcher.go` | Expand `taskMeta` with `PlatformContext` |
| Modify | `internal/infra/store/message_store_test.go` | Test `platform_context` round-trip |

---

## Task 1: Add `PlatformContext` to `InboundMessage`

**Files:**
- Modify: `internal/platform/interfaces.go`

- [ ] **Step 1: Add the field**

Open `internal/platform/interfaces.go`. Add `PlatformContext string` as the last field of `InboundMessage`:

```go
// InboundMessage carries a parsed message from any platform.
type InboundMessage struct {
	Platform          string
	SenderID          string
	SessionKey        string
	Content           string
	RawContent        string
	Raw               string
	PlatformMessageID string
	MessageTime       int64
	PlatformContext   string // JSON: platform-native fields keyed by platform name; empty for local/unknown
}
```

- [ ] **Step 2: Verify the project still compiles**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee && go build ./...
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add internal/platform/interfaces.go
git commit -m "feat: add PlatformContext field to InboundMessage"
```

---

## Task 2: DB Migration — Add `platform_context` Column

**Files:**
- Modify: `internal/infra/store/db.go`

- [ ] **Step 1: Add migration 41**

In `internal/infra/store/db.go`, append to the `migrations` slice after version 40:

```go
{
    version: 41,
    name:    "add_platform_context_to_platform_messages",
    sql:     `ALTER TABLE bee_platform_messages ADD COLUMN platform_context TEXT NOT NULL DEFAULT ''`,
},
```

- [ ] **Step 2: Verify migration runs cleanly**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee && go test ./internal/infra/store/... -run TestMessageStore -v
```

Expected: all existing store tests pass (migration applies automatically via `InitDB` in tests).

- [ ] **Step 3: Commit**

```bash
git add internal/infra/store/db.go
git commit -m "feat: migration 41 — add platform_context column to platform_messages"
```

---

## Task 3: Update `BatchMsg`, `ClaimedMessage`, and SQL

**Files:**
- Modify: `internal/infra/store/message_store.go`
- Modify: `internal/infra/store/message_store_test.go`

- [ ] **Step 1: Write a failing test**

Add to `internal/infra/store/message_store_test.go`:

```go
func TestMessageStore_CreateBatch_PlatformContext(t *testing.T) {
	s := setupMessageStore(t)
	ctx := context.Background()

	msg := BatchMsg{
		ID: "ctx-1", SessionKey: "s1", Platform: "feishu",
		Content: "hello", Raw: "", PlatformMsgID: "pmsg-ctx-1",
		PlatformContext: `{"feishu":{"open_id":"ou_abc","chat_id":"oc_xyz","chat_type":"group"}}`,
		MessageTime: time.Now().UnixMilli(), Status: "received", MergedInto: "",
	}

	inserted, err := s.CreateBatch(ctx, []BatchMsg{msg})
	if err != nil {
		t.Fatalf("CreateBatch error: %v", err)
	}
	if inserted != 1 {
		t.Fatalf("expected 1 row, got %d", inserted)
	}

	// ClaimBatch should return it with platform_context intact.
	claimed, err := s.ClaimBatch(ctx, 10)
	if err != nil {
		t.Fatalf("ClaimBatch error: %v", err)
	}
	if len(claimed) != 1 {
		t.Fatalf("expected 1 claimed message, got %d", len(claimed))
	}
	if claimed[0].PlatformContext != msg.PlatformContext {
		t.Errorf("PlatformContext mismatch\nwant: %s\ngot:  %s", msg.PlatformContext, claimed[0].PlatformContext)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee && go test ./internal/infra/store/... -run TestMessageStore_CreateBatch_PlatformContext -v
```

Expected: compile error — `BatchMsg` has no field `PlatformContext`.

- [ ] **Step 3: Add `PlatformContext` to `BatchMsg`**

In `internal/infra/store/message_store.go`, update `BatchMsg`:

```go
type BatchMsg struct {
	ID              string
	SessionKey      string
	Platform        string
	Content         string
	Raw             string
	PlatformMsgID   string
	PlatformContext string
	MessageTime     int64
	Status          string
	MergedInto      string
}
```

- [ ] **Step 4: Add `PlatformContext` to `ClaimedMessage`**

```go
type ClaimedMessage struct {
	ID              string
	SessionKey      string
	Platform        string
	Content         string
	PlatformContext string
}
```

- [ ] **Step 5: Update `CreateBatch` INSERT to include the new column**

Find `CreateBatch` in `message_store.go`. The INSERT statement currently ends with `received_at, created_at, updated_at`. Update both the column list and the VALUES:

```go
func (s *MessageStore) CreateBatch(ctx context.Context, msgs []BatchMsg) (int64, error) {
	if len(msgs) == 0 {
		return 0, nil
	}
	now := time.Now().UnixMilli()
	placeholders := strings.Repeat("(?,?,?,?,?,?,?,?,?,?,?),", len(msgs))
	placeholders = placeholders[:len(placeholders)-1]

	args := make([]any, 0, len(msgs)*11)
	for _, m := range msgs {
		mt := m.MessageTime
		if mt == 0 {
			mt = now
		}
		args = append(args, m.ID, m.SessionKey, m.Platform, m.Content, m.Raw,
			m.PlatformMsgID, m.PlatformContext, mt, m.Status, m.MergedInto, now)
	}

	result, err := s.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO bee_platform_messages
		 (id, session_key, platform, content, raw, platform_msg_id, platform_context, received_at, status, merged_into, created_at)
		 VALUES `+placeholders,
		args...,
	)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}
```

> **Note:** Check the existing `CreateBatch` implementation for the exact column order before applying. The key change is adding `platform_context` after `platform_msg_id` in both the column list and VALUES. Also add `updated_at` if the existing implementation includes it — match the existing column list exactly, adding `platform_context` in the right position.

- [ ] **Step 6: Update `ClaimBatch` SELECT to scan `platform_context`**

In `ClaimBatch`, the SELECT currently reads `id, session_key, platform, content`. Add `platform_context`:

```go
rows, err := tx.QueryContext(ctx,
    `SELECT id, session_key, platform, content, platform_context
     FROM bee_platform_messages m
     WHERE status = ?
       AND session_key NOT IN (
           SELECT session_key FROM bee_platform_messages WHERE status = ?
       )
       AND received_at = (
           SELECT MIN(received_at)
           FROM bee_platform_messages m2
           WHERE m2.session_key = m.session_key
             AND m2.status = ?
       )
     ORDER BY received_at ASC
     LIMIT ?`, MsgStatusReceived, MsgStatusFeeding, MsgStatusReceived, batchSize)
```

And update the Scan call:

```go
var m ClaimedMessage
if err := rows.Scan(&m.ID, &m.SessionKey, &m.Platform, &m.Content, &m.PlatformContext); err != nil {
```

- [ ] **Step 7: Run the new test to verify it passes**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee && go test ./internal/infra/store/... -run TestMessageStore -v
```

Expected: all pass, including `TestMessageStore_CreateBatch_PlatformContext`.

- [ ] **Step 8: Commit**

```bash
git add internal/infra/store/message_store.go internal/infra/store/message_store_test.go
git commit -m "feat: propagate platform_context through BatchMsg/ClaimedMessage and SQL"
```

---

## Task 4: Populate `PlatformContext` in Feishu Handler

**Files:**
- Modify: `internal/platform/feishu/handler.go`

- [ ] **Step 1: Build and populate `PlatformContext` in the dispatch call**

In `feishu/handler.go`, find the `dispatch(platform.InboundMessage{...})` call inside `OnP2MessageReceiveV1`. Before the dispatch call, add:

```go
platformCtxJSON := buildFeishuContext(sender, msg)
```

Add this helper anywhere in the file (not inside a method):

```go
// buildFeishuContext serializes Feishu-specific message fields for injection into
// message_meta / task_meta so bee and workers can use lark-cli without parsing Raw.
func buildFeishuContext(sender *larkim.EventSender, msg *larkim.EventMessage) string {
	fields := map[string]map[string]string{
		"feishu": {
			"open_id":    utils.DerefStr(sender.SenderId.OpenId),
			"union_id":   utils.DerefStr(sender.SenderId.UnionId),
			"chat_id":    utils.DerefStr(msg.ChatId),
			"chat_type":  utils.DerefStr(msg.ChatType),
			"tenant_key": utils.DerefStr(sender.TenantKey),
			"message_id": utils.DerefStr(msg.MessageId),
		},
	}
	b, _ := json.Marshal(fields)
	return string(b)
}
```

Then add `PlatformContext: platformCtxJSON` to the `dispatch` call:

```go
dispatch(platform.InboundMessage{
    Platform:          "feishu",
    SenderID:          senderID,
    SessionKey:        "feishu:" + *msg.ChatId + ":" + senderID,
    Content:           textContent,
    Raw:               string(rawBytes),
    PlatformMessageID: utils.DerefStrOrEmpty(msg.MessageId),
    MessageTime:       utils.ParseMillis(msg.CreateTime),
    PlatformContext:   platformCtxJSON,
})
```

- [ ] **Step 2: Verify the build**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee && go build ./internal/platform/feishu/...
```

Expected: no errors. (The handler test uses the live SDK and is harder to unit test for context; the round-trip will be verified by the feeder tests.)

- [ ] **Step 3: Commit**

```bash
git add internal/platform/feishu/handler.go
git commit -m "feat: populate PlatformContext in Feishu handler"
```

---

## Task 5: Populate `PlatformContext` in DingTalk Handler

**Files:**
- Modify: `internal/platform/dingtalk/handler.go`

- [ ] **Step 1: Add `buildDingTalkContext` helper and populate in dispatch**

In `dingtalk/handler.go`, find the `dispatch(platform.InboundMessage{...})` call inside the `RegisterChatBotCallbackRouter` callback. Add before the dispatch:

```go
platformCtxJSON := buildDingTalkContext(data)
```

Add the helper (import `"strconv"` is already present in the file):

```go
// buildDingTalkContext serializes DingTalk-specific fields for injection into
// message_meta / task_meta so bee and workers can use DingTalk CLI tools.
func buildDingTalkContext(data *chatbot.BotCallbackDataModel) string {
	fields := map[string]map[string]string{
		"dingtalk": {
			"sender_staff_id":    data.SenderStaffId,
			"sender_nick":        data.SenderNick,
			"sender_corp_id":     data.SenderCorpId,
			"conversation_id":    data.ConversationId,
			"conversation_type":  data.ConversationType,
			"conversation_title": data.ConversationTitle,
			"is_admin":           strconv.FormatBool(data.IsAdmin),
			"chatbot_corp_id":    data.ChatbotCorpId,
		},
	}
	b, _ := json.Marshal(fields)
	return string(b)
}
```

Then add `PlatformContext: platformCtxJSON` to the `dispatch` call:

```go
msg := platform.InboundMessage{
    Platform:          "dingtalk",
    SenderID:          data.SenderStaffId,
    SessionKey:        "dingtalk:" + data.ConversationId + ":" + data.SenderStaffId,
    Content:           textContent,
    Raw:               string(rawBytes),
    PlatformMessageID: data.MsgId,
    MessageTime:       data.CreateAt,
    PlatformContext:   platformCtxJSON,
}
```

- [ ] **Step 2: Verify the build**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee && go build ./internal/platform/dingtalk/...
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add internal/platform/dingtalk/handler.go
git commit -m "feat: populate PlatformContext in DingTalk handler"
```

---

## Task 6: Populate `PlatformContext` in WeCom Handler

**Files:**
- Modify: `internal/platform/wecom/handler.go`

- [ ] **Step 1: Add `buildWeComContext` helper and populate in dispatch**

In `wecom/handler.go`, find the `dispatch(platform.InboundMessage{...})` call. Add before it:

```go
platformCtxJSON := buildWeComContext(body, chatID, senderID)
```

Add the helper:

```go
// buildWeComContext serializes WeCom-specific fields for injection into
// message_meta / task_meta so bee and workers can use WeCom CLI tools.
func buildWeComContext(body *messageBody, chatID, senderID string) string {
	fields := map[string]map[string]string{
		"wecom": {
			"userid":  senderID,
			"chatid":  chatID,
			"chattype": body.ChatType,
			"aibotid": body.AiBotID,
			"msgid":   body.MsgID,
		},
	}
	b, _ := json.Marshal(fields)
	return string(b)
}
```

> **Note:** Check the exact local variable names in `wecom/handler.go` for `chatID` and `senderID` — they may differ slightly. `body.AiBotID` maps to JSON field `"aibotid"` (the struct field name is `AiBotID` from the `messageBody` struct at the top of the file).

Then add `PlatformContext: platformCtxJSON` to the `dispatch` call:

```go
dispatch(platform.InboundMessage{
    Platform:          "wecom",
    SenderID:          senderID,
    SessionKey:        "wecom:" + chatID + ":" + senderID,
    Content:           content,
    RawContent:        rawText,
    Raw:               string(rawBytes),
    PlatformMessageID: body.MsgID,
    MessageTime:       body.CreateTime * 1000,
    PlatformContext:   platformCtxJSON,
})
```

- [ ] **Step 2: Verify the build**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee && go build ./internal/platform/wecom/...
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add internal/platform/wecom/handler.go
git commit -m "feat: populate PlatformContext in WeCom handler"
```

---

## Task 7: Expand `messageMeta` in Bee Feeder

**Files:**
- Modify: `internal/domain/bee/feeder.go`
- Modify: `internal/domain/bee/feeder_internal_test.go`

- [ ] **Step 1: Write failing tests for `buildPrompt` with platform context**

Add to `internal/domain/bee/feeder_internal_test.go`:

```go
func TestBuildPrompt_WithPlatformContext(t *testing.T) {
	ctx := `{"feishu":{"open_id":"ou_abc","chat_id":"oc_xyz","chat_type":"group","tenant_key":"t1","message_id":"om_1","union_id":"on_1"}}`
	msgs := []store.ClaimedMessage{
		{ID: "msg-1", Platform: "feishu", SessionKey: "feishu:oc_xyz:ou_abc", Content: "hello", PlatformContext: ctx},
	}
	got := buildPrompt(msgs, "")

	if !strings.Contains(got, `"platform_context"`) {
		t.Errorf("expected platform_context in message_meta, got: %q", got)
	}
	if !strings.Contains(got, `"ou_abc"`) {
		t.Errorf("expected open_id value in message_meta, got: %q", got)
	}
}

func TestBuildPrompt_NoPlatformContext(t *testing.T) {
	msgs := []store.ClaimedMessage{
		{ID: "msg-1", Platform: "local", SessionKey: "local:default:local", Content: "hello", PlatformContext: ""},
	}
	got := buildPrompt(msgs, "")

	if strings.Contains(got, `"platform_context"`) {
		t.Errorf("platform_context should be omitted when empty, got: %q", got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee && go test ./internal/domain/bee/ -run "TestBuildPrompt_WithPlatformContext|TestBuildPrompt_NoPlatformContext" -v
```

Expected: FAIL — `platform_context` not present in output.

- [ ] **Step 3: Expand `messageMeta` and update `buildPrompt`**

In `internal/domain/bee/feeder.go`, update `messageMeta`:

```go
type messageMeta struct {
	From            string          `json:"from"`
	SessionKey      string          `json:"session_key"`
	MessageID       string          `json:"message_id"`
	PlatformContext json.RawMessage `json:"platform_context,omitempty"`
}
```

Add `"encoding/json"` to imports if not present (it is already used via `json.Marshal`).

Update `buildPrompt` to populate the new field:

```go
func buildPrompt(msgs []store.ClaimedMessage, skillHint string) string {
	var sb strings.Builder
	sb.Grow(len(msgs) * 128)
	if skillHint != "" {
		sb.WriteString(skillHint)
		sb.WriteByte('\n')
	}
	for i, m := range msgs {
		if i > 0 {
			sb.WriteByte('\n')
		}
		meta := messageMeta{
			From:       m.Platform,
			SessionKey: m.SessionKey,
			MessageID:  m.ID,
		}
		if m.PlatformContext != "" {
			meta.PlatformContext = json.RawMessage(m.PlatformContext)
		}
		b, _ := json.Marshal(meta)
		fmt.Fprintf(&sb, "<message_meta>%s</message_meta>\n<message_content>\n%s\n</message_content>\n", b, m.Content)
	}
	return sb.String()
}
```

- [ ] **Step 4: Run all feeder tests**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee && go test ./internal/domain/bee/ -v
```

Expected: all pass, including both new tests.

- [ ] **Step 5: Commit**

```bash
git add internal/domain/bee/feeder.go internal/domain/bee/feeder_internal_test.go
git commit -m "feat: include platform_context in bee message_meta"
```

---

## Task 8: Expand `taskMeta` in Task Dispatcher

**Files:**
- Modify: `internal/domain/task/dispatcher.go`
- Modify: `internal/domain/task/dispatcher_test.go`

- [ ] **Step 1: Write a failing test for `buildInstruction` with platform context**

`buildInstruction` is a package-private function. Add a new file `internal/domain/task/dispatcher_internal_test.go` with package `task` (not `task_test`):

```go
package task

import (
	"strings"
	"testing"

	"github.com/theopenbee/openbee/internal/platform"
)

func TestBuildInstruction_WithPlatformContext(t *testing.T) {
	ctx := `{"feishu":{"open_id":"ou_abc","chat_id":"oc_xyz"}}`
	task := DispatchTask{
		TaskID:    "task-1",
		MessageID: "msg-1",
		ReplyTo: platform.InboundMessage{
			PlatformContext: ctx,
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

func TestBuildInstruction_NoPlatformContext(t *testing.T) {
	task := DispatchTask{
		TaskID:      "task-1",
		MessageID:   "msg-1",
		Instruction: "do something",
	}
	got := buildInstruction(task)

	if strings.Contains(got, `"platform_context"`) {
		t.Errorf("platform_context should be omitted when empty, got: %q", got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee && go test ./internal/domain/task/ -run "TestBuildInstruction" -v
```

Expected: FAIL — `platform_context` not present in output.

- [ ] **Step 3: Expand `taskMeta` and update `buildInstruction`**

In `internal/domain/task/dispatcher.go`, update `taskMeta`:

```go
type taskMeta struct {
	MessageID       string          `json:"message_id"`
	TaskID          string          `json:"task_id,omitempty"`
	PlatformContext json.RawMessage `json:"platform_context,omitempty"`
}
```

Update `buildInstruction`:

```go
func buildInstruction(t DispatchTask) string {
	if t.TaskID != "" || t.MessageID != "" {
		meta := taskMeta{
			MessageID: t.MessageID,
			TaskID:    t.TaskID,
		}
		if t.ReplyTo.PlatformContext != "" {
			meta.PlatformContext = json.RawMessage(t.ReplyTo.PlatformContext)
		}
		b, _ := json.Marshal(meta)
		return fmt.Sprintf("<task_meta>%s</task_meta>\n<task_content>\n%s\n</task_content>", b, t.Instruction)
	}
	return t.Instruction
}
```

- [ ] **Step 4: Run all task dispatcher tests**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee && go test ./internal/domain/task/... -v
```

Expected: all pass, including both new tests.

- [ ] **Step 5: Full build check**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee && go build ./...
```

Expected: no errors.

- [ ] **Step 6: Run all tests**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee && go test ./...
```

Expected: all tests pass.

- [ ] **Step 7: Commit**

```bash
git add internal/domain/task/dispatcher.go internal/domain/task/dispatcher_internal_test.go
git commit -m "feat: include platform_context in worker task_meta"
```

---

## Self-Review

**Spec coverage check:**

| Spec requirement | Covered by |
|---|---|
| `PlatformContext string` added to `InboundMessage` | Task 1 |
| DB migration 41 adds `platform_context` column | Task 2 |
| `BatchMsg` / `ClaimedMessage` carry `PlatformContext` | Task 3 |
| Feishu handler fills `open_id`, `union_id`, `chat_id`, `chat_type`, `tenant_key`, `message_id` | Task 4 |
| DingTalk handler fills `sender_staff_id`, `sender_nick`, `sender_corp_id`, `conversation_id`, `conversation_type`, `conversation_title`, `is_admin`, `chatbot_corp_id` | Task 5 |
| WeCom handler fills `userid`, `chatid`, `chattype`, `aibotid`, `msgid` | Task 6 |
| Bee `messageMeta` expanded with `platform_context`, omitted when empty | Task 7 |
| Worker `taskMeta` expanded with `platform_context`, omitted when empty | Task 8 |

**Placeholder scan:** No TBDs, TODOs, or vague steps — each step has exact code.

**Type consistency:**
- `PlatformContext string` used consistently in `InboundMessage`, `BatchMsg`, `ClaimedMessage`
- `json.RawMessage` used in `messageMeta.PlatformContext` and `taskMeta.PlatformContext` — both populated with `json.RawMessage(someString)` guarded by `!= ""`
- `buildFeishuContext` / `buildDingTalkContext` / `buildWeComContext` return `string`; all callers assign to `platformCtxJSON` before dispatch
