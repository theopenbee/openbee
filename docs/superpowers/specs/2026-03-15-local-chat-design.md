# Local Chat — Design Spec

**Date:** 2026-03-15
**Status:** Approved

## Overview

Add a local web-based chat interface to robobee/core so users can send messages (text, images, documents) directly from the web UI and have them processed by the same bee pipeline that handles Feishu and DingTalk messages. Replies from bee are pushed back to the web UI in real time via SSE.

---

## Requirements

- Multi-session: users manually create and name chat sessions (like ChatGPT)
- Messages go through the identical pipeline as Feishu/DingTalk: `msgingest.Gateway` → `bee.Feeder` → Claude bee → MCP tools → reply
- Files (images, documents) are uploaded to the robobee server's local storage; bee reads them by file path
- Replies are delivered to the frontend via SSE (Server-Sent Events); messages are stored in the DB as the source of truth

---

## Architecture

### Message Flow

```
Web UI (POST message)
  → LocalReceiver (in-process channel)
  → msgingest.Gateway (dedup + debounce)  [unchanged]
  → bee.Feeder (polls DB)                 [unchanged]
  → Claude bee process                    [unchanged]
  → MCP reply_to_message tool             [unchanged]
  → LocalSender.Send()
  → writes to local_replies table
  → SSE Hub broadcasts to connected frontend client
  → Web UI displays reply
```

### Session Key Format

`local:<sessionID>` — follows the same `<platform>:<id>` convention as `feishu:chatID:userID` and `dingtalk:chatID:userID`.

### Platform Integration

A new `local` platform is registered in `buildPlatforms()` in `cmd/server/app.go`. Its sender is added to `sendersByPlatform["local"]`. No other changes to the pipeline wiring are required.

---

## Backend

### New Package: `internal/platform/local`

**`LocalPlatform`** implements the `platform.Platform` interface:

- `ID() string` → `"local"`
- `Receiver() PlatformReceiverAdapter` → `*LocalReceiver`
- `Sender() PlatformSenderAdapter` → `*LocalSender`

**`LocalReceiver`** implements `platform.PlatformReceiverAdapter`:

- Holds an internal buffered channel of `platform.InboundMessage`
- `Start(ctx, dispatch)` — drains the channel and calls `dispatch(msg)` until ctx is cancelled
- `Enqueue(msg InboundMessage)` — called by the HTTP handler to inject a message into the channel

**`LocalSender`** implements `platform.PlatformSenderAdapter`:

- `Send(ctx, msg OutboundMessage)` — inserts a row into `local_replies`, then broadcasts to the SSE Hub for the session key

**`SSEHub`**:

- In-memory map of `sessionKey → []chan string`
- `Subscribe(sessionKey) (<-chan string, unsubscribe func())` — registers a new SSE client
- `Broadcast(sessionKey, data string)` — sends `data` to all subscribed clients for that session
- Protected by a mutex; unsubscribed channels are cleaned up on send failure or explicit unsubscribe

### New Store: `internal/store/local_session_store.go`

Manages the `local_sessions` table.

```go
type LocalSession struct {
    ID        string
    Name      string
    CreatedAt int64
    UpdatedAt int64
}

func (s *LocalSessionStore) Create(ctx, id, name string) error
func (s *LocalSessionStore) List(ctx) ([]LocalSession, error)
func (s *LocalSessionStore) Delete(ctx, id string) error
```

### New Store: `internal/store/local_reply_store.go`

Manages the `local_replies` table.

```go
type LocalReply struct {
    ID         string
    SessionKey string
    Content    string
    MediaPath  string
    CreatedAt  int64
}

func (s *LocalReplyStore) Create(ctx, id, sessionKey, content, mediaPath string) error
func (s *LocalReplyStore) ListBySession(ctx, sessionKey string) ([]LocalReply, error)
func (s *LocalReplyStore) DeleteBySession(ctx, sessionKey string) error
```

### New API Handler: `internal/api/local_chat_handler.go`

Registered under `/api/local/` in `router.go`. Requires access to:
- `LocalReceiver` (to enqueue messages)
- `LocalSessionStore`
- `LocalReplyStore`
- `MessageStore` (to query inbound messages for history)
- `SSEHub` (to stream replies)

### API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/local/sessions` | Create a new session `{name: string}` |
| `GET` | `/api/local/sessions` | List all sessions |
| `DELETE` | `/api/local/sessions/:id` | Delete session and its messages/replies |
| `POST` | `/api/local/sessions/:id/messages` | Send a message (enqueues to LocalReceiver) |
| `GET` | `/api/local/sessions/:id/messages` | Fetch full conversation history (inbound + replies, ordered by time) |
| `POST` | `/api/local/sessions/:id/media` | Upload a file; returns `{path: string}` (server-local path) |
| `GET` | `/api/local/sessions/:id/stream` | SSE stream; pushes `data: <json>\n\n` for each new reply |

**Send message request body:**
```json
{
  "content": "请帮我检查这个文件",
  "media_path": "/Users/tengteng/.robobee/local-uploads/<sessionID>/deployment.yaml"
}
```
If `media_path` is present, the message content is prefixed with `[文件] <path>\n` before enqueueing so bee can locate and read the file.

**SSE event format:**
```
data: {"id":"<replyID>","content":"...","created_at":1234567890}

```

---

## Database Schema

### `local_sessions`

```sql
CREATE TABLE IF NOT EXISTS local_sessions (
    id         TEXT PRIMARY KEY,
    name       TEXT NOT NULL,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);
```

### `local_replies`

```sql
CREATE TABLE IF NOT EXISTS local_replies (
    id          TEXT PRIMARY KEY,
    session_key TEXT NOT NULL,
    content     TEXT NOT NULL,
    media_path  TEXT NOT NULL DEFAULT '',
    created_at  INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_local_replies_session_key ON local_replies(session_key);
```

Inbound messages from local sessions are stored in the existing `platform_messages` table with `platform = 'local'`. No schema changes to that table are needed.

---

## File Upload

Uploaded files are saved to `~/.robobee/local-uploads/<sessionID>/<originalFilename>`.

- The upload endpoint returns the absolute server path.
- The client includes this path in the subsequent send-message request.
- bee receives the path in the message content and can read the file directly (same machine).
- No file size limits are enforced at this stage; the OS filesystem is the constraint.

---

## Frontend

### New Pages

**`web/src/pages/local-chat.tsx`** — Session list
- Lists all sessions ordered by `updated_at` desc
- "New Chat" button opens a dialog to enter a session name
- Each item shows session name, last message preview, and timestamp
- Click navigates to `/local-chat/:id`

**`web/src/pages/local-chat-detail.tsx`** — Chat view
- Header: session name + back link
- Message area: scrollable, user messages right-aligned (blue bubble), bee replies left-aligned (grey bubble), `● ● ●` spinner while bee is processing
- Input area: textarea + paperclip button (file picker) + Send button
- File upload: on file selection, POST to media endpoint first, then include returned path in message send
- SSE: connects to `/api/local/sessions/:id/stream` on mount, disconnects on unmount; new replies append to message list and auto-scroll to bottom

### New Hook

**`web/src/hooks/use-local-chat.ts`**
- `useLocalSessions()` — React Query list query
- `useLocalMessages(sessionId)` — React Query list query for history
- `useLocalChatStream(sessionId, onReply)` — opens SSE connection via `EventSource`, calls `onReply(reply)` on each event, handles reconnect on error

### Modified Files

| File | Change |
|------|--------|
| `web/src/components/nav.tsx` | Add "本地对话 / Local Chat" nav item |
| `web/src/app.tsx` | Add routes `/local-chat` and `/local-chat/:id` |
| `web/src/lib/api.ts` | Add `api.localChat.*` methods |
| `web/src/locales/zh.json` | Add Chinese translation keys |
| `web/src/locales/en.json` | Add English translation keys |
| `cmd/server/app.go` | Register `local.NewPlatform()` in `buildPlatforms()` |
| `internal/api/router.go` | Register local chat handler routes |

---

## Error Handling

- **LocalReceiver channel full:** drop and log (same as `msgingest.Gateway.emit`)
- **SSE client disconnects:** unsubscribe from hub on write failure; no error propagated
- **File upload failure:** return HTTP 400/500; client shows error toast
- **Session not found:** return HTTP 404
- **bee processing failure:** message stays in `received` state and will be retried by Feeder on next tick (existing behaviour)

---

## Testing

- Unit test `LocalReceiver`: enqueue → Start drains and calls dispatch
- Unit test `LocalSender`: Send writes to DB and calls hub.Broadcast
- Unit test `SSEHub`: subscribe, broadcast, unsubscribe
- Unit test `LocalSessionStore` and `LocalReplyStore`: CRUD against in-memory SQLite
- Integration test: POST message → SSE stream receives reply (using test BeeRunner double)
- Frontend: manual smoke test of session create, message send, file upload, reply display
