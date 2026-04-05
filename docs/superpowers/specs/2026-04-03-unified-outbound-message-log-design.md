# Unified Outbound Message Log Design

**Date:** 2026-04-03
**Status:** Approved — implementing

## Background

Currently inbound messages (user → Bee) are stored in `bee_platform_messages` across all platforms. Outbound messages (Bee → user) are fire-and-forget except for Local Chat, where they are written to `bee_local_replies` for history display.

The goal is to persist every outbound message — from any platform — with full audit metadata, enabling unified history viewing, cross-platform analytics, and operational traceability.

## Goals

- Persist all outbound messages (all platforms: local, feishu, weixin, telegram, dingtalk, wecom)
- Full audit record: content, send status, failure reason, source (bee/worker/system), triggering inbound message ID
- Support three query dimensions: by session, by platform, by source (Worker/Bee)
- Migrate Local Chat history: deprecate `bee_local_replies` writes; existing rows migrated to new table
- Retain SSE real-time broadcast for Local Chat

## Non-Goals

- Delivery acknowledgements from external platforms (read receipts)
- Modifying the inbound message pipeline

## Architecture

### New Table: `bee_outbound_messages`

```sql
CREATE TABLE bee_outbound_messages (
    id              TEXT PRIMARY KEY,
    session_key     TEXT NOT NULL,
    platform        TEXT NOT NULL,
    content         TEXT NOT NULL DEFAULT '',
    media_path      TEXT NOT NULL DEFAULT '',
    status          TEXT NOT NULL DEFAULT 'sent' CHECK(status IN ('sent','failed')),
    platform_msg_id TEXT NOT NULL DEFAULT '',
    source_type     TEXT NOT NULL DEFAULT '',   -- 'bee' | 'worker' | 'system' | ''
    source_id       TEXT NOT NULL DEFAULT '',   -- worker_id or ''
    inbound_msg_id  TEXT NOT NULL DEFAULT '',   -- bee_platform_messages.id that triggered this
    error           TEXT NOT NULL DEFAULT '',
    retry_count     INTEGER NOT NULL DEFAULT 0,
    sent_at         INTEGER NOT NULL,
    created_at      INTEGER NOT NULL
)
```

**Indexes (Option 3 — multi-dimensional with cursor-friendly descending order):**
- `(session_key, sent_at DESC)` — conversation history queries
- `(platform, sent_at DESC)` — per-platform queries
- `(source_id, sent_at DESC) WHERE source_id != ''` — per-Worker/Bee queries

**Data migration:** Existing `bee_local_replies` rows are copied into `bee_outbound_messages` in the same migration sequence (platform='local', status='sent').

### OutboundMessage Struct Change

Add three audit fields to `platform.OutboundMessage`:

```go
type OutboundMessage struct {
    SessionKey    string
    Content       string
    ReplyTo       InboundMessage
    MediaPath     string
    SourceType    string // "bee" | "worker" | "system"
    SourceID      string // worker_id when SourceType is "worker"
    InboundMsgID  string // ID of the triggering bee_platform_messages row
}
```

### Logging Decorator

`store.LoggingPlatformSenderAdapter` wraps any `platform.PlatformSenderAdapter`. It:
1. Calls the inner `Send()`
2. Records the result (success or failure + error) to `OutboundMessageStore`

This keeps individual platform senders unchanged.

### OutboundMessageStore

New store in `internal/store/outbound_message_store.go` with:
- `Create()` — insert a sent/failed record
- `ListBySessionKey()` — for chat history (replaces `LocalReplyStore.ListBySession`)
- `DeleteBySessionKey()` — cascade delete on session removal

### LocalSender Changes

Remove the `LocalReplyStore` dependency. `LocalSender.Send()` only broadcasts via SSEHub. Persistence is handled by the logging decorator wrapping it in `app.go`.

### MCP toolSendMessage Changes

Populate `SourceType` and `SourceID` from the request context (worker ID key), and `InboundMsgID` from `params.MessageID`.

### Failure Notifier Changes

Set `SourceType = "system"` on outbound messages sent by the failure notifier.

### LocalChatHandler Changes

- `getMessages`: read replies from `OutboundMessageStore.ListBySessionKey` instead of `LocalReplyStore`
- `deleteSession`: call `OutboundMessageStore.DeleteBySessionKey` instead of `LocalReplyStore.DeleteBySession`

### app.go Changes

- Add `outboundMsgStore` to `appStores`
- Wrap all senders with `LoggingPlatformSenderAdapter` before storing in `sendersByPlatform`
- Remove `localReplyStore` from `appStores` and `LocalChatHandler` construction
- Pass `outboundMsgStore` to `LocalChatHandler`

## Migration Sequence

| Version | Description |
|---------|-------------|
| 17 | Create `bee_outbound_messages` table |
| 18 | Create indexes on `bee_outbound_messages` |
| 19 | Migrate existing `bee_local_replies` into `bee_outbound_messages` |

## Files Changed

| File | Change |
|------|--------|
| `internal/store/db.go` | Add migrations 17–19 |
| `internal/store/outbound_message_store.go` | New store |
| `internal/platform/interfaces.go` | Add 3 fields to OutboundMessage |
| `internal/platform/local/sender.go` | Remove LocalReplyStore; SSE only |
| `internal/store/logging_sender_adapter.go` | New decorator |
| `internal/mcp/tools.go` | Populate source fields on OutboundMessage |
| `internal/task_dispatcher/failure_notifier.go` | Set SourceType="system" |
| `internal/api/local_chat_handler.go` | Use OutboundMessageStore |
| `internal/app/app.go` | Wire new store and decorator |
