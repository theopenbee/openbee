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
  → MCP send_message tool                 [unchanged]
  → LocalSender.Send()
  → writes to local_replies table
  → SSE Hub broadcasts to connected frontend client
  → Web UI displays reply
```

### Session Key Format

`local:<sessionID>` — two segments, deliberately simpler than the three-segment keys used by external platforms (`feishu:chatID:userID`). The pipeline treats session keys as opaque strings so the different segment count has no functional impact.

### Platform Integration

The `local` platform is **always enabled** (not config-gated). It is constructed unconditionally in `buildApp()` and its sender is added to `sendersByPlatform["local"]`. Unlike Feishu/DingTalk, the `LocalReceiver` is also needed by the HTTP handler as a typed `*LocalReceiver`, so it is created at the `buildApp` level and threaded explicitly to both the runner list and the API server — not hidden inside `buildPlatforms()`.

### Dependency Wiring (`buildApp`)

`buildApp` is extended to:
1. Construct `*local.LocalReceiver`, `*local.LocalSender`, and `*local.SSEHub` directly.
2. Add the local platform to `sendersByPlatform["local"]`.
3. Add a runner for `localReceiver.Start(ctx, localIngest.Dispatch)`, following the same error-logging pattern as external platform runners:
   ```go
   runners = append(runners, func(ctx context.Context) {
       if err := localReceiver.Start(ctx, localIngest.Dispatch); err != nil {
           slog.Error("local receiver error", "error", err)
       }
   })
   ```
   Also add a runner for `localIngest.Run(ctx)`.
4. Pass `localReceiver`, `hub`, `localSessionStore`, and `localReplyStore` to a new `buildLocalChatHandler(...)` helper, which returns a `*api.LocalChatHandler`.
5. Pass `*api.LocalChatHandler` to `buildAPIServer` / `api.NewServer`.

`api.Server` gains one new field: `localChatHandler *LocalChatHandler`. Routes are registered by calling `s.localChatHandler.RegisterRoutes(api)` inside `setupRoutes()`.

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
- Channel full: drop and log (same pattern as `msgingest.Gateway.emit`)

**`LocalSender`** implements `platform.PlatformSenderAdapter`:

- `Send(ctx, msg OutboundMessage)` — reads the session key from `msg.ReplyTo.SessionKey` (not `msg.SessionKey`, which is empty), inserts a row into `local_replies`, then calls `hub.Broadcast(sessionKey, json)`.

**`SSEHub`**:

- In-memory map of `sessionKey → []chan string`
- `Subscribe(sessionKey) (<-chan string, unsubscribe func())` — registers a new SSE client
- `Broadcast(sessionKey, data string)` — sends `data` to all subscribed clients for that session; if a channel send would block, the subscriber is dropped and a warning is logged
- Protected by a mutex

### Debounce for Local Chat

The existing `msgingest.Gateway` debounce delay is a single global value. Setting it low (e.g. `200ms`) for local chat responsiveness would break Feishu/DingTalk, which need several seconds to batch multi-segment messages from the same sender.

**Solution:** construct a dedicated `localIngest` Gateway for the local platform with a fixed short debounce (100ms), separate from the global `ingest` Gateway:

```go
localIngest := msgingest.New(s.msgStore, 100*time.Millisecond)
```

The local receiver runner calls `localIngest.Dispatch` instead of `ingest.Dispatch`. A runner for `localIngest.Run(ctx)` is also added. The global `ingest` Gateway continues to serve Feishu/DingTalk unchanged. Both Gateways write to the same `platform_messages` table and are drained by the same `bee.Feeder`.

### New Store: `internal/store/local_session_store.go`

Manages the `local_sessions` table.

```go
type LocalSession struct {
    ID        string
    Name      string
    CreatedAt int64
    UpdatedAt int64
}

func (s *LocalSessionStore) Create(ctx context.Context, id, name string) error
func (s *LocalSessionStore) List(ctx context.Context) ([]LocalSession, error)
func (s *LocalSessionStore) Delete(ctx context.Context, id string) error
```

### New Store: `internal/store/local_reply_store.go`

Manages the `local_replies` table.

```go
type LocalReply struct {
    ID         string
    SessionKey string
    Content    string
    CreatedAt  int64
}

func (s *LocalReplyStore) Create(ctx context.Context, id, sessionKey, content string) error
func (s *LocalReplyStore) ListBySession(ctx context.Context, sessionKey string) ([]LocalReply, error)
func (s *LocalReplyStore) DeleteBySession(ctx context.Context, sessionKey string) error
```

Note: `LocalReply` has no `MediaPath` field. Bee replies via the MCP `send_message` tool and may include file content inline; bee does not reply with a server file path.

### New API Handler: `internal/api/local_chat_handler.go`

```go
type LocalChatHandler struct {
    receiver     *local.LocalReceiver
    hub          *local.SSEHub
    sessionStore *store.LocalSessionStore
    replyStore   *store.LocalReplyStore
    msgStore     *store.MessageStore
}

func (h *LocalChatHandler) RegisterRoutes(rg *gin.RouterGroup)
```

### API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/local/sessions` | Create a new session `{name: string}` |
| `GET` | `/api/local/sessions` | List all sessions |
| `DELETE` | `/api/local/sessions/:id` | Delete session and all related data (see cascade below) |
| `POST` | `/api/local/sessions/:id/messages` | Send a message (enqueues to LocalReceiver) |
| `GET` | `/api/local/sessions/:id/messages` | Fetch full conversation history (inbound + replies, ordered by time) |
| `POST` | `/api/local/sessions/:id/media` | Upload a file; returns `{path: string}` (server-local absolute path) |
| `GET` | `/api/local/sessions/:id/stream` | SSE stream; pushes `data: <json>\n\n` for each new reply |

**Send message request body:**
```json
{
  "content": "请帮我检查这个文件",
  "media_path": "/Users/tengteng/.robobee/local-uploads/<sessionID>/deployment.yaml"
}
```
If `media_path` is present, the server validates that it is within `~/.robobee/local-uploads/<sessionID>/` before constructing the message (path traversal prevention). The message content is then prefixed with `[文件] <path>\n`.

**SSE event format:**
```
data: {"id":"<replyID>","content":"...","created_at":1234567890}

```

**SSE and gzip:** The gzip middleware is applied to `s.router` globally via `router.Use(gzip.Gzip(...))`, so registering the SSE route on a different group does not bypass it. The correct fix is to register gzip with an exclusion for the SSE path pattern:

```go
router.Use(gzip.Gzip(gzip.DefaultCompression,
    gzip.WithExcludedPathsRegexs([]string{`/api/local/sessions/.+/stream`}),
))
```

This ensures all other endpoints retain gzip compression while the SSE stream is served uncompressed and unflushed.

---

## Database Schema

Migrations are added to `internal/store/db.go` as the next sequential versions (currently the highest is migration 12 — these become **13** and **14**).

### Migration 13: `local_sessions`

```sql
CREATE TABLE IF NOT EXISTS local_sessions (
    id         TEXT PRIMARY KEY,
    name       TEXT NOT NULL,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);
```

### Migration 14: `local_replies`

```sql
CREATE TABLE IF NOT EXISTS local_replies (
    id          TEXT PRIMARY KEY,
    session_key TEXT NOT NULL,
    content     TEXT NOT NULL,
    created_at  INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_local_replies_session_key ON local_replies(session_key);
```

Inbound messages from local sessions are stored in the existing `platform_messages` table with `platform = 'local'`. No schema changes to that table are needed.

### Conversation History Query

`GET /api/local/sessions/:id/messages` merges inbound messages and replies ordered by time. Inbound messages must be filtered to exclude `status = 'merged'` rows (debounce-merged duplicates):

```sql
SELECT 'user' AS role, content, received_at AS ts
FROM platform_messages
WHERE session_key = ? AND status != 'merged'
UNION ALL
SELECT 'bee' AS role, content, created_at AS ts
FROM local_replies
WHERE session_key = ?
ORDER BY ts ASC
```

---

## File Upload

Uploaded files are saved to `~/.robobee/local-uploads/<sessionID>/<originalFilename>`.

- The upload endpoint returns the absolute server path.
- The client includes this path in the subsequent send-message request.
- The server validates that the supplied `media_path` is within `~/.robobee/local-uploads/<sessionID>/` using `filepath.Rel` — paths outside this directory are rejected with HTTP 400.
- bee receives the path in the message content and can read the file directly (same machine).
- No file size limits are enforced at this stage.

---

## Delete Session Cascade

`DELETE /api/local/sessions/:id` must clean up all four locations in a single transaction (or best-effort sequential deletes with logging on partial failure):

1. `DELETE FROM platform_messages WHERE session_key = 'local:<id>'`
2. `DELETE FROM local_replies WHERE session_key = 'local:<id>'`
3. `DELETE FROM session_contexts WHERE session_key = 'local:<id>'` (prevents stale bee session on re-creation)
4. `DELETE FROM local_sessions WHERE id = '<id>'`

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
- **SSE reconnect:** On `EventSource` error, close and reopen after 2s backoff. On reconnect, re-fetch full history from `GET /api/local/sessions/:id/messages` to fill any gap — do **not** rely on SSE `Last-Event-ID` replay (the server does not implement it).

### New Hook

**`web/src/hooks/use-local-chat.ts`**
- `useLocalSessions()` — React Query list query
- `useLocalMessages(sessionId)` — React Query list query for history
- `useLocalChatStream(sessionId, onReply)` — opens SSE connection via `EventSource`, calls `onReply(reply)` on each event, handles reconnect with history re-fetch on error

### Modified Files

| File | Change |
|------|--------|
| `web/src/components/nav.tsx` | Add "本地对话 / Local Chat" nav item; use prefix match (`pathname.startsWith(link.href)`) for active state so `/local-chat/:id` also highlights the nav item |
| `web/src/app.tsx` | Add routes `/local-chat` and `/local-chat/:id` |
| `web/src/lib/api.ts` | Add `api.localChat.*` methods |
| `web/src/locales/zh.json` | Add Chinese translation keys |
| `web/src/locales/en.json` | Add English translation keys |
| `cmd/server/app.go` | Construct local platform and wire dependencies (see Dependency Wiring section) |
| `internal/api/router.go` | Accept and register `*LocalChatHandler`; register SSE route outside gzip middleware group |

---

## Error Handling

- **LocalReceiver channel full:** drop and log
- **SSE client disconnects:** unsubscribe from hub on write failure; no error propagated
- **File upload failure:** return HTTP 400/500; client shows error toast
- **`media_path` outside upload dir:** return HTTP 400
- **Session not found:** return HTTP 404
- **bee processing failure:** message stays in `received` state and will be retried by Feeder on next tick (existing behaviour)

---

## Testing

- Unit test `LocalReceiver`: enqueue → Start drains and calls dispatch
- Unit test `LocalSender`: Send reads `ReplyTo.SessionKey`, writes to DB, calls hub.Broadcast
- Unit test `SSEHub`: subscribe, broadcast, unsubscribe, blocked-client drop
- Unit test `LocalSessionStore` and `LocalReplyStore`: CRUD against in-memory SQLite
- Unit test delete cascade: all four tables cleaned up
- Unit test path validation: reject `media_path` outside upload dir
- Integration test: POST message → SSE stream receives reply (using test BeeRunner double)
- Frontend: manual smoke test of session create, message send, file upload, reply display, SSE reconnect
