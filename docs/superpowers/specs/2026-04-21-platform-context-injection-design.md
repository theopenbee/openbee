# Platform Context Injection Design

**Date:** 2026-04-21
**Status:** Approved

## Problem

Third-party IM platform messages carry rich metadata (sender IDs, chat IDs, conversation types, display names, etc.) that bee and workers currently never see. Only a minimal `message_meta` / `task_meta` is injected at runtime:

```json
// Bee receives:
{"from": "feishu", "session_key": "feishu:oc_xxx:ou_xxx", "message_id": "internal-db-id"}

// Worker receives:
{"message_id": "internal-db-id", "task_id": "task-id"}
```

This prevents bee and workers from using third-party IM CLI tools (lark-cli, DingTalk CLI, WeCom CLI) flexibly — they lack the platform-native IDs needed to send messages, @mention users, reply to threads, etc.

## Goal

Inject platform-native context fields into both bee's `message_meta` and worker's `task_meta` so that any bee or worker can directly use those IDs with IM CLI tools — without parsing the raw event JSON themselves.

## Non-Goals

- Normalizing fields across platforms (each platform keeps its own field names)
- Exposing platform context via a new query CLI command
- Supporting platforms beyond DingTalk, Feishu, and WeCom

## Design

### Approach: Expand Meta Injection at Runtime

Add a `PlatformContext` string field (pre-serialized JSON) to `InboundMessage`. Each platform handler fills it in when constructing the message. The context flows through the pipeline into both bee's prompt and worker's task instruction.

### Data Flow

```
Platform handler
  → InboundMessage.PlatformContext (filled at source)
  → msgingest Gateway (passes through in BatchMsg)
  → bee_platform_messages DB (new platform_context TEXT column)
  → ClaimedMessage.PlatformContext (fetched back for bee)
  → buildPrompt() → messageMeta.PlatformContext → <message_meta>

Worker path (in-memory, no DB round-trip):
  → DispatchTask.ReplyTo.PlatformContext
  → buildInstruction() → taskMeta.PlatformContext → <task_meta>
```

### Platform Context Payloads

#### Feishu

Key: `"feishu"`

| Field | Source | Example |
|---|---|---|
| `open_id` | `sender.sender_id.open_id` | `ou_db3b61b4c59a5d4e3d533...` |
| `union_id` | `sender.sender_id.union_id` | `on_f3066ea510b83afa41e2e9...` |
| `chat_id` | `message.chat_id` | `oc_638e2544fcce65dc383cc2...` |
| `chat_type` | `message.chat_type` | `"group"` or `"p2p"` |
| `tenant_key` | `sender.tenant_key` | `1ad248ca308ddc85` |
| `message_id` | `message.message_id` | `om_x100b516a856334a8c4d4...` |

```json
{
  "feishu": {
    "open_id": "ou_db3b61b4c59a5d4e3d533...",
    "union_id": "on_f3066ea510b83afa41...",
    "chat_id": "oc_638e2544fcce65dc...",
    "chat_type": "group",
    "tenant_key": "1ad248ca308ddc85",
    "message_id": "om_x100b516a856334a..."
  }
}
```

#### DingTalk

Key: `"dingtalk"`

| Field | Source | Example |
|---|---|---|
| `sender_staff_id` | `senderStaffId` | `060139276627947909` |
| `sender_nick` | `senderNick` | `滕勇志` |
| `sender_corp_id` | `senderCorpId` | `dinga4232db39b38f741...` |
| `conversation_id` | `conversationId` | `cidvHh144KpyeCMc+ti0...` |
| `conversation_type` | `conversationType` | `"1"` (group) or `"2"` (private) |
| `conversation_title` | `conversationTitle` | group name or `""` |
| `is_admin` | `isAdmin` | `"true"` or `"false"` |
| `chatbot_corp_id` | `chatbotCorpId` | `dinga4232db39b38f741...` |

```json
{
  "dingtalk": {
    "sender_staff_id": "060139276627947909",
    "sender_nick": "滕勇志",
    "sender_corp_id": "dinga4232db39b38f741...",
    "conversation_id": "cidvHh144KpyeCMc+ti0...",
    "conversation_type": "1",
    "conversation_title": "",
    "is_admin": "true",
    "chatbot_corp_id": "dinga4232db39b38f741..."
  }
}
```

#### WeCom

Key: `"wecom"`

| Field | Source | Example |
|---|---|---|
| `userid` | `from.userid` | `TengYongZhi` |
| `chatid` | `chatid` | `wrk6UuEAAAO6bKxkLsG__FZx...` |
| `chattype` | `chattype` | `"group"` or `"single"` |
| `aibotid` | `aibotid` | `aibGVm7KiVeSbYuuX-kZ00...` |
| `msgid` | `msgid` | `4b4dff171a1892589c0df78...` |

```json
{
  "wecom": {
    "userid": "TengYongZhi",
    "chatid": "wrk6UuEAAAO6bKxkLsG__FZx...",
    "chattype": "group",
    "aibotid": "aibGVm7KiVeSbYuuX-kZ00...",
    "msgid": "4b4dff171a1892589c0df78..."
  }
}
```

### Runtime Injection

#### Bee (`<message_meta>`)

```json
{
  "from": "feishu",
  "session_key": "feishu:oc_638e2544...:ou_db3b61b4...",
  "message_id": "internal-db-id",
  "platform_context": {
    "feishu": {
      "open_id": "ou_db3b61b4...",
      "chat_id": "oc_638e2544...",
      "chat_type": "group",
      "tenant_key": "1ad248ca308ddc85",
      "message_id": "om_x100b516..."
    }
  }
}
```

#### Worker (`<task_meta>`)

```json
{
  "message_id": "internal-db-id",
  "task_id": "task-id",
  "platform_context": {
    "feishu": {
      "open_id": "ou_db3b61b4...",
      "chat_id": "oc_638e2544...",
      "chat_type": "group",
      "tenant_key": "1ad248ca308ddc85",
      "message_id": "om_x100b516..."
    }
  }
}
```

## File Changes

### 1. `internal/platform/interfaces.go`

Add `PlatformContext string` to `InboundMessage`:

```go
type InboundMessage struct {
    Platform          string
    SenderID          string
    SessionKey        string
    Content           string
    RawContent        string
    Raw               string
    PlatformMessageID string
    MessageTime       int64
    PlatformContext   string // JSON: platform-native fields, keyed by platform name
}
```

### 2. `internal/platform/feishu/handler.go`

Fill `PlatformContext` when dispatching:

```go
ctx := map[string]map[string]string{
    "feishu": {
        "open_id":    senderID,
        "union_id":   utils.DerefStr(sender.SenderId.UnionId),
        "chat_id":    utils.DerefStr(msg.ChatId),
        "chat_type":  utils.DerefStr(msg.ChatType),
        "tenant_key": utils.DerefStr(sender.TenantKey),
        "message_id": utils.DerefStr(msg.MessageId),
    },
}
ctxJSON, _ := json.Marshal(ctx)
dispatch(platform.InboundMessage{
    // ... existing fields ...
    PlatformContext: string(ctxJSON),
})
```

### 3. `internal/platform/dingtalk/handler.go`

Fill `PlatformContext` from `data *chatbot.BotCallbackDataModel`:

```go
ctx := map[string]map[string]string{
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
ctxJSON, _ := json.Marshal(ctx)
```

### 4. `internal/platform/wecom/handler.go`

Fill `PlatformContext` from `body *messageBody`:

```go
ctx := map[string]map[string]string{
    "wecom": {
        "userid":   body.From.UserID,
        "chatid":   chatID,
        "chattype": body.ChatType,
        "aibotid":  body.AiBotID,
        "msgid":    body.MsgID,
    },
}
ctxJSON, _ := json.Marshal(ctx)
```

### 5. `internal/infra/store/message_store.go`

- Add `PlatformContext string` to `BatchMsg`
- Add `PlatformContext string` to `ClaimedMessage`
- Update `CreateBatch` INSERT to include `platform_context`
- Update `ClaimBatch` SELECT to scan `platform_context`

### 6. DB Migration

Add migration version 41 to `internal/infra/store/db.go` (current latest is 40):

```go
{
    version: 41,
    name:    "add_platform_context_to_platform_messages",
    sql:     `ALTER TABLE bee_platform_messages ADD COLUMN platform_context TEXT NOT NULL DEFAULT ''`,
},
```

The column defaults to `''` (empty string) so existing rows are unaffected.

### 7. `internal/domain/bee/feeder.go`

Expand `messageMeta`:

```go
type messageMeta struct {
    From            string          `json:"from"`
    SessionKey      string          `json:"session_key"`
    MessageID       string          `json:"message_id"`
    PlatformContext json.RawMessage `json:"platform_context,omitempty"`
}
```

In `buildPrompt`, populate from `m.PlatformContext`:

```go
b, _ := json.Marshal(messageMeta{
    From:            m.Platform,
    SessionKey:      m.SessionKey,
    MessageID:       m.ID,
    PlatformContext: json.RawMessage(m.PlatformContext),
})
```

### 8. `internal/domain/task/dispatcher.go`

Expand `taskMeta`:

```go
type taskMeta struct {
    MessageID       string          `json:"message_id"`
    TaskID          string          `json:"task_id,omitempty"`
    PlatformContext json.RawMessage `json:"platform_context,omitempty"`
}
```

In `buildInstruction`, populate from `t.ReplyTo.PlatformContext`:

```go
b, _ := json.Marshal(taskMeta{
    MessageID:       t.MessageID,
    TaskID:          t.TaskID,
    PlatformContext: json.RawMessage(t.ReplyTo.PlatformContext),
})
```

## Out-of-Scope / Future Work

- Local platform: no platform context (not applicable)
- Telegram / Weixin: can be added in the same pattern when needed
- Normalizing sender name / chat type across platforms
