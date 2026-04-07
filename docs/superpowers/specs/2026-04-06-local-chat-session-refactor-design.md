# Local Chat Session Refactor Design

**Date:** 2026-04-06
**Status:** Approved

## Overview

Refactor local chat to remove the explicit session creation step. Currently users must create a named session before chatting; after this refactor, opening `/local-chat` immediately shows a ready chat interface with no setup required.

## Goals

- Eliminate session creation friction
- Default single conversation with no session management UI
- Support conversation reset via natural language commands
- Preserve chat history for user reference

## Confirmed Design Decisions

| Decision | Choice |
|---|---|
| Session model | Single default session, no creation needed |
| Reset trigger | Text commands: 'clear' / '重置对话' etc. |
| After reset | Messages preserved, AI memory (session context) cleared |
| Navigation | `/local-chat` directly shows chat UI |
| Technical approach | Remove sessions layer, fixed `session_key = "local:default"` |
| Reset mechanism | AI detects intent → calls existing CLI tool to clear session context |
| bee_local_sessions table | Mark deprecated (keep data, do not DROP) |

---

## Section 1: Data Layer

### Fixed Session Key

All local chat messages use a single hardcoded session key:

```
local:default
```

### Table Changes

**bee_local_sessions** — marked deprecated. No DROP. Existing rows remain in DB but are no longer read or written by the application. A comment in the migration documents the deprecation.

**Unchanged tables:**
- `bee_platform_messages` — stores user messages (session_key = "local:default")
- `bee_outbound_messages` — stores AI replies (session_key = "local:default")
- `bee_session_contexts` — stores AI memory per session key; cleared on reset

### Database Migration

New migration version:
```sql
-- Deprecate bee_local_sessions table (no longer used by application)
-- Data preserved for reference. Do not query this table.
-- All local chat now uses session_key = 'local:default'
```

### Legacy Data

Existing messages with `session_key = "local:{old_uuid}"` remain in DB untouched. They will not appear in the UI (frontend only queries `local:default`). Safe isolation with no data loss.

---

## Section 2: API Layer

### Removed Endpoints

```
POST   /api/local/sessions                    (removed)
GET    /api/local/sessions                    (removed)
DELETE /api/local/sessions/:id               (removed)
POST   /api/local/sessions/:id/messages      (removed)
GET    /api/local/sessions/:id/messages      (removed)
POST   /api/local/sessions/:id/media         (removed)
GET    /api/local/sessions/:id/media/:filename (removed)
GET    /api/local/sessions/:id/stream        (removed)
```

### New Endpoints

```
POST /api/local/messages          # Send user message
GET  /api/local/messages          # Fetch all messages
POST /api/local/media             # Upload file attachment
GET  /api/local/media/:filename   # Serve uploaded file
GET  /api/local/stream            # SSE real-time stream
```

All handlers hardcode `session_key = "local:default"`. No session ID parameter anywhere.

**Media file storage:** Currently files are stored under `~/.openbee/local-uploads/{sessionId}/`. After refactor, use the fixed path `~/.openbee/local-uploads/default/` for all uploads.

### Backend Changes

`internal/api/local_chat_handler.go` — simplified significantly:
- Remove all session CRUD handlers
- Remove session ID extraction from path params
- All operations reference `"local:default"` directly
- `internal/infra/store/local_session_store.go` — deleted

---

## Section 3: AI Reset Mechanism

When the user sends a reset intent (e.g. 'clear', '重置对话', 'start over'), Bee handles it naturally without any special frontend interception.

**Flow:**
```
User: "clear"
  → Message sent to Bee normally via POST /api/local/messages
  → Bee recognizes reset intent
  → Bee calls existing CLI tool to clear session context
  → bee_session_contexts row for session_key = "local:default" is deleted
  → Bee replies with confirmation (e.g. "对话记忆已清空，我们重新开始吧！")
  → Frontend chat history remains visible (scroll to see past messages)
  → AI starts fresh with no memory of prior conversation
```

**No new backend endpoint needed.** The existing CLI tooling handles context clearing.

---

## Section 4: Frontend Changes

### Route Changes

```
BEFORE:
  /local-chat        → Session list page (local-chat.tsx)
  /local-chat/:id    → Chat detail page (local-chat-detail.tsx)

AFTER:
  /local-chat        → Chat page (local-chat.tsx, rewritten)
```

### File Changes

| File | Action |
|---|---|
| `web/src/pages/local-chat.tsx` | Rewrite as single chat page (was: session list) |
| `web/src/pages/local-chat-detail.tsx` | Delete |
| `web/src/hooks/use-local-chat.ts` | Remove session hooks, simplify remaining |

### Hook Changes

**Removed hooks:**
- `useLocalSessions()`
- `useCreateSession()`
- `useDeleteSession()`

**Simplified hooks (remove `sessionId` parameter):**
- `useLocalMessages()` — queries `/api/local/messages`
- `useSendMessage()` — posts to `/api/local/messages`
- `useLocalChatStream(onReply)` — connects to `/api/local/stream`

React Query keys simplified:
```
["local-messages"]   (was: ["local-messages", sessionId])
["local-stream"]     (was: per-session)
```

---

## Section 5: Migration Strategy

### Execution Order

1. Add DB migration (mark bee_local_sessions deprecated)
2. Simplify backend: remove session store, rewrite handler
3. Update frontend: new route, rewrite local-chat.tsx, delete detail page, simplify hooks
4. Remove `internal/infra/store/local_session_store.go`
5. Verify SSE streaming works end-to-end with new paths

### Rollback Safety

- `bee_local_sessions` data preserved (not dropped)
- Legacy `local:{uuid}` messages in DB untouched
- No destructive DB operations

---

## Files to Change

**Backend:**
- `internal/api/local_chat_handler.go` — rewrite
- `internal/infra/store/local_session_store.go` — delete
- `internal/infra/migrations/` — add new migration version
- Route registration — remove session routes, add new flat routes

**Frontend:**
- `web/src/pages/local-chat.tsx` — rewrite
- `web/src/pages/local-chat-detail.tsx` — delete
- `web/src/hooks/use-local-chat.ts` — simplify
- Router config — update route definition

**Platform layer:**
- Verify `internal/platform/local/` (receiver.go, sender.go, hub.go) work unchanged with `"local:default"` as session key
