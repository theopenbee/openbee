# Local Chat Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a local web-based chat interface so users can send messages (text + uploaded files) via the web UI and have them processed by the same bee pipeline that handles Feishu/DingTalk, with replies pushed back via SSE.

**Architecture:** A new `internal/platform/local` package implements the existing `Platform` interface with `LocalReceiver` (HTTP-triggered, in-process channel) and `LocalSender` (writes to `local_replies` table + broadcasts via `SSEHub`). A dedicated `localIngest` Gateway with 100ms debounce serves local messages without disturbing Feishu/DingTalk's debounce. The frontend gains a `/local-chat` section with session list and chat detail pages.

**Tech Stack:** Go 1.22+, Gin, SQLite (`github.com/mattn/go-sqlite3`), `github.com/google/uuid`, React 18, TypeScript, React Query (`@tanstack/react-query`), `EventSource` API, shadcn/ui components

---

## File Map

### New files
| Path | Responsibility |
|------|---------------|
| `internal/platform/local/hub.go` | SSEHub: in-memory pub/sub for SSE clients |
| `internal/platform/local/receiver.go` | LocalReceiver: PlatformReceiverAdapter backed by channel |
| `internal/platform/local/sender.go` | LocalSender: PlatformSenderAdapter that writes replies + broadcasts |
| `internal/platform/local/platform.go` | LocalPlatform: bundles receiver + sender, implements Platform |
| `internal/platform/local/hub_test.go` | Tests for SSEHub |
| `internal/platform/local/receiver_test.go` | Tests for LocalReceiver |
| `internal/platform/local/sender_test.go` | Tests for LocalSender |
| `internal/store/local_session_store.go` | LocalSessionStore: CRUD for local_sessions table + `TouchUpdatedAt` (bumps updated_at on message send) |
| `internal/store/local_reply_store.go` | LocalReplyStore: insert/list/delete for local_replies table |
| `internal/store/local_session_store_test.go` | Tests for LocalSessionStore |
| `internal/store/local_reply_store_test.go` | Tests for LocalReplyStore |
| `internal/api/local_chat_handler.go` | REST + SSE endpoints for local chat |
| `web/src/pages/local-chat.tsx` | Session list page |
| `web/src/pages/local-chat-detail.tsx` | Chat detail page (messages + input + file upload) |
| `web/src/hooks/use-local-chat.ts` | React Query hooks + SSE hook |

### Modified files
| Path | Change |
|------|--------|
| `internal/store/db.go` | Add migrations 13 (local_sessions) and 14 (local_replies) |
| `internal/store/message_store.go` | Add `ListBySessionKey`, `DeleteBySessionKey` |
| `internal/api/router.go` | Accept `*LocalChatHandler`; exclude SSE path from gzip; register routes |
| `cmd/server/app.go` | Construct local platform, `localIngest` gateway, wire to app |
| `web/src/lib/api.ts` | Add `api.localChat.*` methods |
| `web/src/components/nav.tsx` | Add "本地对话 / Local Chat" nav item |
| `web/src/app.tsx` | Add `/local-chat` and `/local-chat/:id` routes |
| `web/src/locales/zh.json` | Chinese translation keys |
| `web/src/locales/en.json` | English translation keys |

---

## Chunk 1: Database Migrations + Store Layer

### Task 1: Add DB migrations 13 and 14

**Files:**
- Modify: `internal/store/db.go`

- [ ] **Step 1: Add migration 13 (local_sessions) and 14 (local_replies) to the migrations slice**

Append to the `migrations` slice in `internal/store/db.go` after the existing version 12 entry:

```go
{
    version: 13,
    name:    "20260315_create_table_local_sessions",
    sql: `CREATE TABLE IF NOT EXISTS local_sessions (
    id         TEXT PRIMARY KEY,
    name       TEXT NOT NULL,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
)`,
},
{
    version: 14,
    name:    "20260315_create_table_local_replies",
    sql: `CREATE TABLE IF NOT EXISTS local_replies (
    id          TEXT PRIMARY KEY,
    session_key TEXT NOT NULL,
    content     TEXT NOT NULL,
    created_at  INTEGER NOT NULL
)`,
},
{
    version: 15,
    name:    "20260315_create_index_local_replies_session_key",
    sql:     `CREATE INDEX IF NOT EXISTS idx_local_replies_session_key ON local_replies(session_key)`,
},
```

- [ ] **Step 2: Verify the migration runs clean**

```bash
cd /Users/tengteng/work/robobee/core
go test ./internal/store/... -run TestDB -v
```

Expected: existing DB tests pass. If no `TestDB` exists, run:
```bash
go build ./...
```
Expected: compiles with no errors.

- [ ] **Step 3: Commit**

```bash
git add internal/store/db.go
git commit -m "feat(store): add migrations 13-15 for local_sessions and local_replies"
```

---

### Task 2: LocalSessionStore

**Files:**
- Create: `internal/store/local_session_store.go`
- Create: `internal/store/local_session_store_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/store/local_session_store_test.go`:

```go
package store_test

import (
    "context"
    "testing"

    "github.com/robobee/core/internal/store"
)

func setupLocalSessionDB(t *testing.T) *store.LocalSessionStore {
    t.Helper()
    db, err := store.InitDB(t.TempDir() + "/test.db")
    if err != nil {
        t.Fatalf("InitDB: %v", err)
    }
    t.Cleanup(func() { db.Close() })
    return store.NewLocalSessionStore(db)
}

func TestLocalSessionStore_CreateAndList(t *testing.T) {
    s := setupLocalSessionDB(t)
    ctx := context.Background()

    if err := s.Create(ctx, "id-1", "My Session"); err != nil {
        t.Fatalf("Create: %v", err)
    }

    sessions, err := s.List(ctx)
    if err != nil {
        t.Fatalf("List: %v", err)
    }
    if len(sessions) != 1 {
        t.Fatalf("expected 1 session, got %d", len(sessions))
    }
    if sessions[0].ID != "id-1" || sessions[0].Name != "My Session" {
        t.Errorf("unexpected session: %+v", sessions[0])
    }
}

func TestLocalSessionStore_List_OrdersByUpdatedAtDesc(t *testing.T) {
    s := setupLocalSessionDB(t)
    ctx := context.Background()

    s.Create(ctx, "id-1", "First")  //nolint:errcheck
    s.Create(ctx, "id-2", "Second") //nolint:errcheck
    s.TouchUpdatedAt(ctx, "id-1")   //nolint:errcheck

    sessions, _ := s.List(ctx)
    if sessions[0].ID != "id-1" {
        t.Errorf("expected id-1 first (most recently updated), got %s", sessions[0].ID)
    }
}

func TestLocalSessionStore_Delete(t *testing.T) {
    s := setupLocalSessionDB(t)
    ctx := context.Background()

    s.Create(ctx, "id-1", "To Delete") //nolint:errcheck
    if err := s.Delete(ctx, "id-1"); err != nil {
        t.Fatalf("Delete: %v", err)
    }

    sessions, _ := s.List(ctx)
    if len(sessions) != 0 {
        t.Errorf("expected 0 sessions after delete, got %d", len(sessions))
    }
}

func TestLocalSessionStore_List_EmptyReturnsSlice(t *testing.T) {
    s := setupLocalSessionDB(t)
    sessions, err := s.List(context.Background())
    if err != nil {
        t.Fatalf("List: %v", err)
    }
    if sessions == nil {
        t.Error("expected non-nil slice, got nil")
    }
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd /Users/tengteng/work/robobee/core
go test ./internal/store/... -run TestLocalSessionStore -v
```

Expected: compile error — `store.LocalSessionStore` undefined.

- [ ] **Step 3: Implement LocalSessionStore**

Create `internal/store/local_session_store.go`:

```go
package store

import (
    "context"
    "database/sql"
    "time"
)

// LocalSession is a row from the local_sessions table.
type LocalSession struct {
    ID        string `json:"id"`
    Name      string `json:"name"`
    CreatedAt int64  `json:"created_at"`
    UpdatedAt int64  `json:"updated_at"`
}

// LocalSessionStore manages the local_sessions table.
type LocalSessionStore struct {
    db *sql.DB
}

// NewLocalSessionStore constructs a LocalSessionStore.
func NewLocalSessionStore(db *sql.DB) *LocalSessionStore {
    return &LocalSessionStore{db: db}
}

// Create inserts a new local session.
func (s *LocalSessionStore) Create(ctx context.Context, id, name string) error {
    now := time.Now().UnixMilli()
    _, err := s.db.ExecContext(ctx,
        `INSERT INTO local_sessions (id, name, created_at, updated_at) VALUES (?, ?, ?, ?)`,
        id, name, now, now,
    )
    return err
}

// List returns all sessions ordered by updated_at descending.
func (s *LocalSessionStore) List(ctx context.Context) ([]LocalSession, error) {
    rows, err := s.db.QueryContext(ctx,
        `SELECT id, name, created_at, updated_at FROM local_sessions ORDER BY updated_at DESC`,
    )
    if err != nil {
        return nil, err
    }
    defer rows.Close()
    sessions := []LocalSession{}
    for rows.Next() {
        var sess LocalSession
        if err := rows.Scan(&sess.ID, &sess.Name, &sess.CreatedAt, &sess.UpdatedAt); err != nil {
            return nil, err
        }
        sessions = append(sessions, sess)
    }
    return sessions, rows.Err()
}

// Delete removes a session by ID.
func (s *LocalSessionStore) Delete(ctx context.Context, id string) error {
    _, err := s.db.ExecContext(ctx, `DELETE FROM local_sessions WHERE id = ?`, id)
    return err
}

// TouchUpdatedAt bumps updated_at to now for the given session.
// Called by the HTTP handler on every message send so the session list stays sorted by activity.
func (s *LocalSessionStore) TouchUpdatedAt(ctx context.Context, id string) error {
    _, err := s.db.ExecContext(ctx,
        `UPDATE local_sessions SET updated_at = ? WHERE id = ?`,
        time.Now().UnixMilli(), id,
    )
    return err
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
cd /Users/tengteng/work/robobee/core
go test ./internal/store/... -run TestLocalSessionStore -v
```

Expected: all 4 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/store/local_session_store.go internal/store/local_session_store_test.go
git commit -m "feat(store): add LocalSessionStore with CRUD"
```

---

### Task 3: LocalReplyStore

**Files:**
- Create: `internal/store/local_reply_store.go`
- Create: `internal/store/local_reply_store_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/store/local_reply_store_test.go`:

```go
package store_test

import (
    "context"
    "testing"

    "github.com/robobee/core/internal/store"
)

func setupLocalReplyDB(t *testing.T) *store.LocalReplyStore {
    t.Helper()
    db, err := store.InitDB(t.TempDir() + "/test.db")
    if err != nil {
        t.Fatalf("InitDB: %v", err)
    }
    t.Cleanup(func() { db.Close() })
    return store.NewLocalReplyStore(db)
}

func TestLocalReplyStore_CreateAndList(t *testing.T) {
    s := setupLocalReplyDB(t)
    ctx := context.Background()

    if err := s.Create(ctx, "r-1", "local:sess-1", "Hello from bee"); err != nil {
        t.Fatalf("Create: %v", err)
    }

    replies, err := s.ListBySession(ctx, "local:sess-1")
    if err != nil {
        t.Fatalf("ListBySession: %v", err)
    }
    if len(replies) != 1 {
        t.Fatalf("expected 1 reply, got %d", len(replies))
    }
    if replies[0].Content != "Hello from bee" {
        t.Errorf("unexpected content: %q", replies[0].Content)
    }
}

func TestLocalReplyStore_ListBySession_IsolatesSessions(t *testing.T) {
    s := setupLocalReplyDB(t)
    ctx := context.Background()

    s.Create(ctx, "r-1", "local:sess-A", "For A") //nolint:errcheck
    s.Create(ctx, "r-2", "local:sess-B", "For B") //nolint:errcheck

    repliesA, _ := s.ListBySession(ctx, "local:sess-A")
    if len(repliesA) != 1 || repliesA[0].Content != "For A" {
        t.Errorf("session A isolation failed: %+v", repliesA)
    }
}

func TestLocalReplyStore_DeleteBySession(t *testing.T) {
    s := setupLocalReplyDB(t)
    ctx := context.Background()

    s.Create(ctx, "r-1", "local:sess-1", "Reply 1") //nolint:errcheck
    s.Create(ctx, "r-2", "local:sess-1", "Reply 2") //nolint:errcheck

    if err := s.DeleteBySession(ctx, "local:sess-1"); err != nil {
        t.Fatalf("DeleteBySession: %v", err)
    }

    replies, _ := s.ListBySession(ctx, "local:sess-1")
    if len(replies) != 0 {
        t.Errorf("expected 0 replies after delete, got %d", len(replies))
    }
}

func TestLocalReplyStore_ListBySession_EmptyReturnsSlice(t *testing.T) {
    s := setupLocalReplyDB(t)
    replies, err := s.ListBySession(context.Background(), "local:nobody")
    if err != nil {
        t.Fatalf("ListBySession: %v", err)
    }
    if replies == nil {
        t.Error("expected non-nil slice, got nil")
    }
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/store/... -run TestLocalReplyStore -v
```

Expected: compile error — `store.LocalReplyStore` undefined.

- [ ] **Step 3: Implement LocalReplyStore**

Create `internal/store/local_reply_store.go`:

```go
package store

import (
    "context"
    "database/sql"
    "time"
)

// LocalReply is a row from the local_replies table.
type LocalReply struct {
    ID         string `json:"id"`
    SessionKey string `json:"session_key"`
    Content    string `json:"content"`
    CreatedAt  int64  `json:"created_at"`
}

// LocalReplyStore manages the local_replies table.
type LocalReplyStore struct {
    db *sql.DB
}

// NewLocalReplyStore constructs a LocalReplyStore.
func NewLocalReplyStore(db *sql.DB) *LocalReplyStore {
    return &LocalReplyStore{db: db}
}

// Create inserts a new reply row.
func (s *LocalReplyStore) Create(ctx context.Context, id, sessionKey, content string) error {
    _, err := s.db.ExecContext(ctx,
        `INSERT INTO local_replies (id, session_key, content, created_at) VALUES (?, ?, ?, ?)`,
        id, sessionKey, content, time.Now().UnixMilli(),
    )
    return err
}

// ListBySession returns all replies for a session key ordered by created_at.
func (s *LocalReplyStore) ListBySession(ctx context.Context, sessionKey string) ([]LocalReply, error) {
    rows, err := s.db.QueryContext(ctx,
        `SELECT id, session_key, content, created_at FROM local_replies
         WHERE session_key = ? ORDER BY created_at ASC`,
        sessionKey,
    )
    if err != nil {
        return nil, err
    }
    defer rows.Close()
    replies := []LocalReply{}
    for rows.Next() {
        var r LocalReply
        if err := rows.Scan(&r.ID, &r.SessionKey, &r.Content, &r.CreatedAt); err != nil {
            return nil, err
        }
        replies = append(replies, r)
    }
    return replies, rows.Err()
}

// DeleteBySession removes all replies for the given session key.
func (s *LocalReplyStore) DeleteBySession(ctx context.Context, sessionKey string) error {
    _, err := s.db.ExecContext(ctx,
        `DELETE FROM local_replies WHERE session_key = ?`, sessionKey,
    )
    return err
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/store/... -run TestLocalReplyStore -v
```

Expected: all 4 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/store/local_reply_store.go internal/store/local_reply_store_test.go
git commit -m "feat(store): add LocalReplyStore with create/list/delete"
```

---

### Task 4: MessageStore additions

**Files:**
- Modify: `internal/store/message_store.go`
- Create: `internal/store/message_store_local_test.go`

The conversation history endpoint needs to fetch non-merged inbound messages for a session, and the delete cascade needs to remove all messages for a session.

Note: new tests go in a **new file** `message_store_local_test.go` (not appended to `message_store_test.go`) to avoid conflicting with that file's `package store` declaration.

- [ ] **Step 1: Write the failing tests**

Create `internal/store/message_store_local_test.go`:

```go
package store_test

import (
    "context"
    "testing"

    "github.com/robobee/core/internal/store"
)

func TestMessageStore_ListBySessionKey_ExcludesMerged(t *testing.T) {
    db, err := store.InitDB(t.TempDir() + "/test.db")
    if err != nil {
        t.Fatalf("InitDB: %v", err)
    }
    defer db.Close()
    s := store.NewMessageStore(db)
    ctx := context.Background()

    // Insert one received and one merged message for the same session
    s.CreateBatch(ctx, []store.BatchMsg{ //nolint:errcheck
        {ID: "m1", SessionKey: "local:s1", Platform: "local", Content: "hello",
         Status: "received", MessageTime: 1000},
        {ID: "m2", SessionKey: "local:s1", Platform: "local", Content: "world",
         Status: "merged", MergedInto: "m1", MessageTime: 900},
    })

    msgs, err := s.ListBySessionKey(ctx, "local:s1")
    if err != nil {
        t.Fatalf("ListBySessionKey: %v", err)
    }
    if len(msgs) != 1 {
        t.Fatalf("expected 1 non-merged message, got %d", len(msgs))
    }
    if msgs[0].ID != "m1" {
        t.Errorf("expected message m1, got %s", msgs[0].ID)
    }
}

func TestMessageStore_DeleteBySessionKey(t *testing.T) {
    db, err := store.InitDB(t.TempDir() + "/test.db")
    if err != nil {
        t.Fatalf("InitDB: %v", err)
    }
    defer db.Close()
    s := store.NewMessageStore(db)
    ctx := context.Background()

    s.CreateBatch(ctx, []store.BatchMsg{ //nolint:errcheck
        {ID: "m1", SessionKey: "local:s1", Platform: "local", Content: "a",
         Status: "received", MessageTime: 1000},
        {ID: "m2", SessionKey: "local:s2", Platform: "local", Content: "b",
         Status: "received", MessageTime: 1000},
    })

    if err := s.DeleteBySessionKey(ctx, "local:s1"); err != nil {
        t.Fatalf("DeleteBySessionKey: %v", err)
    }

    msgs, _ := s.ListBySessionKey(ctx, "local:s1")
    if len(msgs) != 0 {
        t.Errorf("expected 0 messages for s1, got %d", len(msgs))
    }
    msgs2, _ := s.ListBySessionKey(ctx, "local:s2")
    if len(msgs2) != 1 {
        t.Errorf("s2 should be unaffected, got %d", len(msgs2))
    }
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/store/... -run "TestMessageStore_ListBySessionKey|TestMessageStore_DeleteBySessionKey" -v
```

Expected: compile error — `s.ListBySessionKey` and `s.DeleteBySessionKey` undefined.

- [ ] **Step 3: Add ListBySessionKey and DeleteBySessionKey to message_store.go**

Add this struct and these methods to `internal/store/message_store.go`:

```go
// InboundMessage is a non-merged platform_messages row for display in chat history.
type InboundMessage struct {
    ID         string
    Content    string
    ReceivedAt int64
}

// ListBySessionKey returns all non-merged messages for a session, ordered by received_at ASC.
func (s *MessageStore) ListBySessionKey(ctx context.Context, sessionKey string) ([]InboundMessage, error) {
    rows, err := s.db.QueryContext(ctx,
        `SELECT id, content, received_at FROM platform_messages
         WHERE session_key = ? AND status != 'merged'
         ORDER BY received_at ASC`,
        sessionKey,
    )
    if err != nil {
        return nil, err
    }
    defer rows.Close()
    msgs := []InboundMessage{}
    for rows.Next() {
        var m InboundMessage
        if err := rows.Scan(&m.ID, &m.Content, &m.ReceivedAt); err != nil {
            return nil, err
        }
        msgs = append(msgs, m)
    }
    return msgs, rows.Err()
}

// DeleteBySessionKey removes all platform_messages for the given session key.
func (s *MessageStore) DeleteBySessionKey(ctx context.Context, sessionKey string) error {
    _, err := s.db.ExecContext(ctx,
        `DELETE FROM platform_messages WHERE session_key = ?`, sessionKey,
    )
    return err
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/store/... -run "TestMessageStore_ListBySessionKey|TestMessageStore_DeleteBySessionKey" -v
```

Expected: both tests PASS.

- [ ] **Step 5: Run all store tests to check nothing is broken**

```bash
go test ./internal/store/... -v
```

Expected: all tests PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/store/message_store.go internal/store/message_store_local_test.go
git commit -m "feat(store): add ListBySessionKey and DeleteBySessionKey to MessageStore"
```

---

## Chunk 2: Local Platform Package

### Task 5: SSEHub

**Files:**
- Create: `internal/platform/local/hub.go`
- Create: `internal/platform/local/hub_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/platform/local/hub_test.go`:

```go
package local_test

import (
    "testing"
    "time"

    "github.com/robobee/core/internal/platform/local"
)

func TestSSEHub_SubscribeAndBroadcast(t *testing.T) {
    h := local.NewSSEHub()
    ch, unsub := h.Subscribe("local:s1")
    defer unsub()

    h.Broadcast("local:s1", `{"id":"r1"}`)

    select {
    case got := <-ch:
        if got != `{"id":"r1"}` {
            t.Errorf("expected JSON, got %q", got)
        }
    case <-time.After(100 * time.Millisecond):
        t.Fatal("timeout waiting for broadcast")
    }
}

func TestSSEHub_Broadcast_IsolatesSessions(t *testing.T) {
    h := local.NewSSEHub()
    chA, unsubA := h.Subscribe("local:s1")
    defer unsubA()
    chB, unsubB := h.Subscribe("local:s2")
    defer unsubB()

    h.Broadcast("local:s1", "for-A")

    select {
    case <-chA:
    case <-time.After(100 * time.Millisecond):
        t.Fatal("A should have received broadcast")
    }
    select {
    case msg := <-chB:
        t.Fatalf("B should not have received broadcast, got %q", msg)
    case <-time.After(50 * time.Millisecond):
        // expected
    }
}

func TestSSEHub_Unsubscribe_StopsReceiving(t *testing.T) {
    h := local.NewSSEHub()
    _, unsub := h.Subscribe("local:s1")
    unsub()

    // Should not panic on broadcast to empty subscriber list
    h.Broadcast("local:s1", "orphan")
}

func TestSSEHub_MultipleSubscribers(t *testing.T) {
    h := local.NewSSEHub()
    ch1, unsub1 := h.Subscribe("local:s1")
    ch2, unsub2 := h.Subscribe("local:s1")
    defer unsub1()
    defer unsub2()

    h.Broadcast("local:s1", "hello")

    for _, ch := range []<-chan string{ch1, ch2} {
        select {
        case got := <-ch:
            if got != "hello" {
                t.Errorf("expected hello, got %q", got)
            }
        case <-time.After(100 * time.Millisecond):
            t.Fatal("timeout: both subscribers should receive")
        }
    }
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/platform/local/... -run TestSSEHub -v
```

Expected: compile error — package doesn't exist yet.

- [ ] **Step 3: Implement SSEHub**

Create `internal/platform/local/hub.go`:

```go
package local

import (
    "log/slog"
    "sync"
)

// SSEHub manages Server-Sent Events subscriptions keyed by session key.
type SSEHub struct {
    mu          sync.Mutex
    subscribers map[string][]chan string
}

// NewSSEHub constructs an SSEHub.
func NewSSEHub() *SSEHub {
    return &SSEHub{subscribers: make(map[string][]chan string)}
}

// Subscribe registers a new SSE client for the given session key.
// Returns the receive channel and an unsubscribe function the caller must invoke on disconnect.
func (h *SSEHub) Subscribe(sessionKey string) (<-chan string, func()) {
    ch := make(chan string, 8)
    h.mu.Lock()
    h.subscribers[sessionKey] = append(h.subscribers[sessionKey], ch)
    h.mu.Unlock()

    return ch, func() {
        h.mu.Lock()
        defer h.mu.Unlock()
        subs := h.subscribers[sessionKey]
        for i, s := range subs {
            if s == ch {
                h.subscribers[sessionKey] = append(subs[:i], subs[i+1:]...)
                break
            }
        }
        close(ch)
    }
}

// Broadcast sends data to all subscribers of the given session key.
// If a subscriber's channel is full, it is dropped with a warning.
func (h *SSEHub) Broadcast(sessionKey, data string) {
    h.mu.Lock()
    subs := make([]chan string, len(h.subscribers[sessionKey]))
    copy(subs, h.subscribers[sessionKey])
    h.mu.Unlock()

    for _, ch := range subs {
        select {
        case ch <- data:
        default:
            slog.Warn("sse hub: subscriber channel full, dropping", "sessionKey", sessionKey)
        }
    }
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/platform/local/... -run TestSSEHub -v
```

Expected: all 4 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/platform/local/hub.go internal/platform/local/hub_test.go
git commit -m "feat(local): add SSEHub for server-sent events broadcast"
```

---

### Task 6: LocalReceiver

**Files:**
- Create: `internal/platform/local/receiver.go`
- Create: `internal/platform/local/receiver_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/platform/local/receiver_test.go`:

```go
package local_test

import (
    "context"
    "testing"
    "time"

    "github.com/robobee/core/internal/platform"
    "github.com/robobee/core/internal/platform/local"
)

func TestLocalReceiver_EnqueueAndDispatch(t *testing.T) {
    r := local.NewLocalReceiver(8)
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    var dispatched []platform.InboundMessage
    done := make(chan struct{})

    go func() {
        r.Start(ctx, func(msg platform.InboundMessage) { //nolint:errcheck
            dispatched = append(dispatched, msg)
            if len(dispatched) == 1 {
                close(done)
            }
        })
    }()

    r.Enqueue(platform.InboundMessage{
        Platform:   "local",
        SessionKey: "local:s1",
        Content:    "hello",
    })

    select {
    case <-done:
    case <-time.After(200 * time.Millisecond):
        t.Fatal("timeout: dispatch not called")
    }

    if dispatched[0].Content != "hello" {
        t.Errorf("expected hello, got %q", dispatched[0].Content)
    }
}

func TestLocalReceiver_Start_ReturnsNilOnCancel(t *testing.T) {
    r := local.NewLocalReceiver(8)
    ctx, cancel := context.WithCancel(context.Background())

    errCh := make(chan error, 1)
    go func() {
        errCh <- r.Start(ctx, func(platform.InboundMessage) {})
    }()

    cancel()

    select {
    case err := <-errCh:
        if err != nil {
            t.Errorf("expected nil error on cancel, got %v", err)
        }
    case <-time.After(200 * time.Millisecond):
        t.Fatal("Start did not return after context cancel")
    }
}

func TestLocalReceiver_Enqueue_DropWhenFull(t *testing.T) {
    // Channel size 1, never drained — second enqueue must not block
    r := local.NewLocalReceiver(1)

    r.Enqueue(platform.InboundMessage{Content: "first"})
    // This must return immediately (not block) even though channel is full
    done := make(chan struct{})
    go func() {
        r.Enqueue(platform.InboundMessage{Content: "second"})
        close(done)
    }()

    select {
    case <-done:
    case <-time.After(100 * time.Millisecond):
        t.Fatal("Enqueue blocked on full channel")
    }
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/platform/local/... -run TestLocalReceiver -v
```

Expected: compile error.

- [ ] **Step 3: Implement LocalReceiver**

Create `internal/platform/local/receiver.go`:

```go
package local

import (
    "context"
    "log/slog"

    "github.com/robobee/core/internal/platform"
)

// LocalReceiver implements platform.PlatformReceiverAdapter.
// Messages are injected via Enqueue and dispatched to the registered handler in Start.
type LocalReceiver struct {
    ch chan platform.InboundMessage
}

// NewLocalReceiver constructs a LocalReceiver with the given channel buffer size.
func NewLocalReceiver(bufSize int) *LocalReceiver {
    return &LocalReceiver{ch: make(chan platform.InboundMessage, bufSize)}
}

// Start blocks, dispatching each enqueued message, until ctx is cancelled.
// Implements platform.PlatformReceiverAdapter.
func (r *LocalReceiver) Start(ctx context.Context, dispatch func(platform.InboundMessage)) error {
    for {
        select {
        case msg := <-r.ch:
            dispatch(msg)
        case <-ctx.Done():
            return nil
        }
    }
}

// Enqueue adds a message to the dispatch queue.
// If the channel is full the message is dropped and a warning is logged.
func (r *LocalReceiver) Enqueue(msg platform.InboundMessage) {
    select {
    case r.ch <- msg:
    default:
        slog.Warn("local receiver: channel full, dropping message",
            "component", "local", "sessionKey", msg.SessionKey)
    }
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/platform/local/... -run TestLocalReceiver -v
```

Expected: all 3 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/platform/local/receiver.go internal/platform/local/receiver_test.go
git commit -m "feat(local): add LocalReceiver (PlatformReceiverAdapter)"
```

---

### Task 7: LocalSender + LocalPlatform

**Files:**
- Create: `internal/platform/local/sender.go`
- Create: `internal/platform/local/sender_test.go`
- Create: `internal/platform/local/platform.go`

- [ ] **Step 1: Write the failing tests for LocalSender**

Create `internal/platform/local/sender_test.go`:

```go
package local_test

import (
    "context"
    "database/sql"
    "encoding/json"
    "testing"

    "github.com/robobee/core/internal/platform"
    "github.com/robobee/core/internal/platform/local"
    "github.com/robobee/core/internal/store"
)

func setupSenderDB(t *testing.T) (*store.LocalReplyStore, *sql.DB) {
    t.Helper()
    db, err := store.InitDB(t.TempDir() + "/test.db")
    if err != nil {
        t.Fatalf("InitDB: %v", err)
    }
    t.Cleanup(func() { db.Close() })
    return store.NewLocalReplyStore(db), db
}

func TestLocalSender_Send_WritesReplyAndBroadcasts(t *testing.T) {
    replyStore, _ := setupSenderDB(t)
    hub := local.NewSSEHub()
    sender := local.NewLocalSender(replyStore, hub)

    ch, unsub := hub.Subscribe("local:sess-1")
    defer unsub()

    msg := platform.OutboundMessage{
        ReplyTo: platform.InboundMessage{SessionKey: "local:sess-1"},
        Content: "Reply content",
    }
    if err := sender.Send(context.Background(), msg); err != nil {
        t.Fatalf("Send: %v", err)
    }

    // Verify DB write
    replies, err := replyStore.ListBySession(context.Background(), "local:sess-1")
    if err != nil {
        t.Fatalf("ListBySession: %v", err)
    }
    if len(replies) != 1 || replies[0].Content != "Reply content" {
        t.Errorf("unexpected replies: %+v", replies)
    }

    // Verify SSE broadcast
    select {
    case data := <-ch:
        var payload map[string]any
        if err := json.Unmarshal([]byte(data), &payload); err != nil {
            t.Fatalf("broadcast data is not valid JSON: %v", err)
        }
        if payload["content"] != "Reply content" {
            t.Errorf("broadcast content mismatch: %v", payload)
        }
    default:
        t.Fatal("expected SSE broadcast but channel was empty")
    }
}

func TestLocalSender_Send_UsesReplyToSessionKey(t *testing.T) {
    replyStore, _ := setupSenderDB(t)
    hub := local.NewSSEHub()
    sender := local.NewLocalSender(replyStore, hub)

    // OutboundMessage.SessionKey is empty — only ReplyTo.SessionKey should be used
    msg := platform.OutboundMessage{
        SessionKey: "",
        ReplyTo:    platform.InboundMessage{SessionKey: "local:correct-session"},
        Content:    "test",
    }
    if err := sender.Send(context.Background(), msg); err != nil {
        t.Fatalf("Send: %v", err)
    }

    replies, _ := replyStore.ListBySession(context.Background(), "local:correct-session")
    if len(replies) != 1 {
        t.Errorf("expected reply in correct session, got %d replies", len(replies))
    }
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/platform/local/... -run TestLocalSender -v
```

Expected: compile error.

- [ ] **Step 3: Implement LocalSender**

Create `internal/platform/local/sender.go`:

```go
package local

import (
    "context"
    "encoding/json"
    "time"

    "github.com/google/uuid"
    "github.com/robobee/core/internal/platform"
    "github.com/robobee/core/internal/store"
)

// LocalSender implements platform.PlatformSenderAdapter.
// It writes bee replies to the local_replies table and broadcasts via SSEHub.
type LocalSender struct {
    replyStore *store.LocalReplyStore
    hub        *SSEHub
}

// NewLocalSender constructs a LocalSender.
func NewLocalSender(replyStore *store.LocalReplyStore, hub *SSEHub) *LocalSender {
    return &LocalSender{replyStore: replyStore, hub: hub}
}

// Send stores the reply and broadcasts it to any connected SSE clients.
// The session key is read from msg.ReplyTo.SessionKey (msg.SessionKey is always empty).
func (s *LocalSender) Send(ctx context.Context, msg platform.OutboundMessage) error {
    sessionKey := msg.ReplyTo.SessionKey
    id := uuid.New().String()

    if err := s.replyStore.Create(ctx, id, sessionKey, msg.Content); err != nil {
        return err
    }

    data, _ := json.Marshal(map[string]any{
        "id":         id,
        "content":    msg.Content,
        "created_at": time.Now().UnixMilli(),
    })
    s.hub.Broadcast(sessionKey, string(data))
    return nil
}
```

- [ ] **Step 4: Implement LocalPlatform**

Create `internal/platform/local/platform.go`:

```go
package local

import "github.com/robobee/core/internal/platform"

// LocalPlatform bundles LocalReceiver and LocalSender and implements platform.Platform.
type LocalPlatform struct {
    receiver *LocalReceiver
    sender   *LocalSender
}

// NewPlatform constructs a LocalPlatform from pre-built receiver and sender.
func NewPlatform(receiver *LocalReceiver, sender *LocalSender) *LocalPlatform {
    return &LocalPlatform{receiver: receiver, sender: sender}
}

func (p *LocalPlatform) ID() string                              { return "local" }
func (p *LocalPlatform) Receiver() platform.PlatformReceiverAdapter { return p.receiver }
func (p *LocalPlatform) Sender() platform.PlatformSenderAdapter  { return p.sender }
```

- [ ] **Step 5: Run tests**

```bash
go test ./internal/platform/local/... -v
```

Expected: all tests PASS.

- [ ] **Step 6: Verify entire codebase compiles**

```bash
go build ./...
```

Expected: no errors.

- [ ] **Step 7: Commit**

```bash
git add internal/platform/local/
git commit -m "feat(local): add LocalSender and LocalPlatform"
```

---

## Chunk 3: API Handler + App Wiring

### Task 8: LocalChatHandler

**Files:**
- Create: `internal/api/local_chat_handler.go`
- Modify: `internal/api/router.go`

- [ ] **Step 1: Implement LocalChatHandler**

Create `internal/api/local_chat_handler.go`:

```go
package api

import (
    "fmt"
    "io"
    "log/slog"
    "net/http"
    "os"
    "path/filepath"
    "sort"
    "strings"
    "time"

    "github.com/gin-gonic/gin"
    "github.com/google/uuid"
    "github.com/robobee/core/internal/platform"
    "github.com/robobee/core/internal/platform/local"
    "github.com/robobee/core/internal/store"
)

// LocalChatHandler handles all /api/local/* endpoints.
type LocalChatHandler struct {
    receiver     *local.LocalReceiver
    hub          *local.SSEHub
    sessionStore *store.LocalSessionStore
    replyStore   *store.LocalReplyStore
    msgStore     *store.MessageStore
    sessionCtx   *store.SessionStore
}

// NewLocalChatHandler constructs a LocalChatHandler.
func NewLocalChatHandler(
    receiver *local.LocalReceiver,
    hub *local.SSEHub,
    sessionStore *store.LocalSessionStore,
    replyStore *store.LocalReplyStore,
    msgStore *store.MessageStore,
    sessionCtx *store.SessionStore,
) *LocalChatHandler {
    return &LocalChatHandler{
        receiver:     receiver,
        hub:          hub,
        sessionStore: sessionStore,
        replyStore:   replyStore,
        msgStore:     msgStore,
        sessionCtx:   sessionCtx,
    }
}

// RegisterRoutes registers all non-SSE routes on the given router group.
func (h *LocalChatHandler) RegisterRoutes(rg *gin.RouterGroup) {
    rg.POST("/local/sessions", h.createSession)
    rg.GET("/local/sessions", h.listSessions)
    rg.DELETE("/local/sessions/:id", h.deleteSession)
    rg.POST("/local/sessions/:id/messages", h.sendMessage)
    rg.GET("/local/sessions/:id/messages", h.getMessages)
    rg.POST("/local/sessions/:id/media", h.uploadMedia)
}

// StreamReplies is registered separately on the raw router to bypass gzip middleware.
func (h *LocalChatHandler) StreamReplies(c *gin.Context) {
    sessionID := c.Param("id")
    sessionKey := "local:" + sessionID

    c.Header("Content-Type", "text/event-stream")
    c.Header("Cache-Control", "no-cache")
    c.Header("Connection", "keep-alive")

    ch, unsub := h.hub.Subscribe(sessionKey)
    defer unsub()

    ctx := c.Request.Context()
    for {
        select {
        case data, ok := <-ch:
            if !ok {
                return
            }
            fmt.Fprintf(c.Writer, "data: %s\n\n", data)
            c.Writer.Flush()
        case <-ctx.Done():
            return
        }
    }
}

func (h *LocalChatHandler) createSession(c *gin.Context) {
    var body struct {
        Name string `json:"name" binding:"required"`
    }
    if err := c.ShouldBindJSON(&body); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }
    id := uuid.New().String()
    if err := h.sessionStore.Create(c.Request.Context(), id, body.Name); err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    c.JSON(http.StatusCreated, store.LocalSession{
        ID:        id,
        Name:      body.Name,
        CreatedAt: time.Now().UnixMilli(),
        UpdatedAt: time.Now().UnixMilli(),
    })
}

func (h *LocalChatHandler) listSessions(c *gin.Context) {
    sessions, err := h.sessionStore.List(c.Request.Context())
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    c.JSON(http.StatusOK, sessions)
}

func (h *LocalChatHandler) deleteSession(c *gin.Context) {
    id := c.Param("id")
    sessionKey := "local:" + id
    ctx := c.Request.Context()

    // Best-effort cascade: log failures but continue so the session row is always removed.
    if err := h.msgStore.DeleteBySessionKey(ctx, sessionKey); err != nil {
        slog.Error("deleteSession: delete messages", "sessionKey", sessionKey, "error", err)
    }
    if err := h.replyStore.DeleteBySession(ctx, sessionKey); err != nil {
        slog.Error("deleteSession: delete replies", "sessionKey", sessionKey, "error", err)
    }
    if err := h.sessionCtx.ClearSessionContexts(ctx, sessionKey); err != nil {
        slog.Error("deleteSession: clear session contexts", "sessionKey", sessionKey, "error", err)
    }
    if err := h.sessionStore.Delete(ctx, id); err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}

func (h *LocalChatHandler) sendMessage(c *gin.Context) {
    id := c.Param("id")
    sessionKey := "local:" + id

    var body struct {
        Content   string `json:"content" binding:"required"`
        MediaPath string `json:"media_path"`
    }
    if err := c.ShouldBindJSON(&body); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    content := body.Content
    if body.MediaPath != "" {
        uploadDir, err := localUploadDir(id)
        if err != nil {
            c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
            return
        }
        rel, err := filepath.Rel(uploadDir, body.MediaPath)
        if err != nil || strings.HasPrefix(rel, "..") {
            c.JSON(http.StatusBadRequest, gin.H{"error": "invalid media_path: must be within upload directory"})
            return
        }
        content = "[文件] " + body.MediaPath + "\n" + content
    }

    h.receiver.Enqueue(platform.InboundMessage{
        Platform:    "local",
        SenderID:    "web",
        SessionKey:  sessionKey,
        Content:     content,
        RawContent:  content,
        MessageTime: time.Now().UnixMilli(),
    })

    h.sessionStore.TouchUpdatedAt(c.Request.Context(), id) //nolint:errcheck

    c.JSON(http.StatusAccepted, gin.H{"status": "queued"})
}

type chatMessage struct {
    Role      string `json:"role"`
    Content   string `json:"content"`
    Timestamp int64  `json:"ts"`
}

func (h *LocalChatHandler) getMessages(c *gin.Context) {
    id := c.Param("id")
    sessionKey := "local:" + id
    ctx := c.Request.Context()

    inbound, err := h.msgStore.ListBySessionKey(ctx, sessionKey)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    replies, err := h.replyStore.ListBySession(ctx, sessionKey)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    combined := make([]chatMessage, 0, len(inbound)+len(replies))
    for _, m := range inbound {
        combined = append(combined, chatMessage{Role: "user", Content: m.Content, Timestamp: m.ReceivedAt})
    }
    for _, r := range replies {
        combined = append(combined, chatMessage{Role: "bee", Content: r.Content, Timestamp: r.CreatedAt})
    }
    sort.Slice(combined, func(i, j int) bool { return combined[i].Timestamp < combined[j].Timestamp })

    c.JSON(http.StatusOK, combined)
}

func (h *LocalChatHandler) uploadMedia(c *gin.Context) {
    id := c.Param("id")

    file, header, err := c.Request.FormFile("file")
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "missing 'file' field"})
        return
    }
    defer file.Close()

    uploadDir, err := localUploadDir(id)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    if err := os.MkdirAll(uploadDir, 0o755); err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    destPath := filepath.Join(uploadDir, header.Filename)
    dest, err := os.Create(destPath)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    defer dest.Close()

    if _, err := io.Copy(dest, file); err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    c.JSON(http.StatusOK, gin.H{"path": destPath})
}

func localUploadDir(sessionID string) (string, error) {
    home, err := os.UserHomeDir()
    if err != nil {
        return "", fmt.Errorf("get home dir: %w", err)
    }
    return filepath.Join(home, ".robobee", "local-uploads", sessionID), nil
}
```

- [ ] **Step 2: Modify router.go to wire LocalChatHandler and exclude SSE path from gzip**

In `internal/api/router.go`:

1. Add `localChatHandler *LocalChatHandler` field to `Server` struct.
2. Update `NewServer` to accept and store `*LocalChatHandler`.
3. Change gzip registration to exclude the SSE path.
4. Call `s.localChatHandler.RegisterRoutes(api)` in `setupRoutes`.
5. Register the SSE route directly on `s.router` (outside the api group but after gzip is already configured with the exclusion).

Full updated `internal/api/router.go`:

```go
package api

import (
    "io/fs"
    "net/http"
    "strings"

    "github.com/gin-contrib/cors"
    "github.com/gin-contrib/gzip"
    "github.com/gin-gonic/gin"
    "github.com/robobee/core/internal/mcp"
    "github.com/robobee/core/internal/store"
    "github.com/robobee/core/internal/worker"
)

type Server struct {
    router           *gin.Engine
    workerStore      *store.WorkerStore
    executionStore   *store.ExecutionStore
    manager          *worker.Manager
    mcpServer        *mcp.MCPServer
    mcpAPIKey        string
    staticFS         fs.FS
    localChatHandler *LocalChatHandler
}

func NewServer(
    ws *store.WorkerStore,
    es *store.ExecutionStore,
    mgr *worker.Manager,
    mcpSrv *mcp.MCPServer,
    mcpAPIKey string,
    staticFS fs.FS,
    localChat *LocalChatHandler,
) *Server {
    router := gin.Default()
    router.Use(gzip.Gzip(gzip.DefaultCompression,
        gzip.WithExcludedPathsRegexs([]string{`/api/local/sessions/.+/stream`}),
    ))
    router.Use(cors.New(cors.Config{
        AllowOrigins:     []string{"*"},
        AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
        AllowHeaders:     []string{"Origin", "Content-Type", "Authorization", "Accept-Language", "X-API-Key"},
        ExposeHeaders:    []string{"Content-Length"},
        AllowCredentials: false,
    }))
    s := &Server{
        router:           router,
        workerStore:      ws,
        executionStore:   es,
        manager:          mgr,
        mcpServer:        mcpSrv,
        mcpAPIKey:        mcpAPIKey,
        staticFS:         staticFS,
        localChatHandler: localChat,
    }
    s.setupRoutes()
    return s
}

func (s *Server) setupRoutes() {
    api := s.router.Group("/api")
    {
        // Workers
        api.POST("/workers", s.createWorker)
        api.GET("/workers", s.listWorkers)
        api.GET("/workers/:id", s.getWorker)
        api.PUT("/workers/:id", s.updateWorker)
        api.DELETE("/workers/:id", s.deleteWorker)

        // Worker executions
        api.GET("/workers/:id/executions", s.listWorkerExecutions)

        // Sessions
        api.GET("/sessions/:sessionId/executions", s.listSessionExecutions)

        // Executions
        api.GET("/executions", s.listExecutions)
        api.GET("/executions/:id", s.getExecution)
        // WebSocket logs
        api.GET("/executions/:id/logs", s.streamLogs)

        // Local chat (all except SSE stream)
        if s.localChatHandler != nil {
            s.localChatHandler.RegisterRoutes(api)
        }
    }

    // SSE stream — registered directly to bypass gzip buffering
    if s.localChatHandler != nil {
        s.router.GET("/api/local/sessions/:id/stream", s.localChatHandler.StreamReplies)
    }

    // MCP — only registered when an API key is configured
    if s.mcpServer != nil {
        mcpGroup := s.router.Group("/mcp")
        s.mcpServer.RegisterRoutes(mcpGroup, s.mcpAPIKey)
    }

    if s.staticFS != nil {
        sub, _ := fs.Sub(s.staticFS, "dist")
        httpFS := http.FS(sub)

        indexHTML, _ := fs.ReadFile(sub, "index.html")

        s.router.NoRoute(func(c *gin.Context) {
            path := strings.TrimPrefix(c.Request.URL.Path, "/")
            if path != "" {
                f, err := sub.Open(path)
                if err == nil {
                    f.Close()
                    c.FileFromFS(path, httpFS)
                    return
                }
            }
            c.Data(http.StatusOK, "text/html; charset=utf-8", indexHTML)
        })
    }
}

func (s *Server) Run(addr string) error {
    return s.router.Run(addr)
}
```

- [ ] **Step 3: Commit — note: does NOT compile yet**

`router.go` now requires a `*LocalChatHandler` argument that `app.go` doesn't pass yet. Do NOT run `go build ./...` until Task 9 is done.

```bash
git add internal/api/local_chat_handler.go internal/api/router.go
git commit -m "feat(api): add LocalChatHandler and wire SSE-safe gzip exclusion"
```

---

### Task 9: App wiring

**Files:**
- Modify: `cmd/server/app.go`

- [ ] **Step 1: Update app.go to wire local platform, localIngest, and LocalChatHandler**

Replace `cmd/server/app.go` with the following (preserve all existing logic, add local sections):

```go
package main

import (
    "context"
    "database/sql"
    "fmt"
    "log/slog"
    "os"
    "os/signal"
    "syscall"
    "time"

    "github.com/gin-gonic/gin"
    "github.com/robobee/core/internal/api"
    "github.com/robobee/core/internal/bee"
    "github.com/robobee/core/internal/config"
    "github.com/robobee/core/internal/mcp"
    "github.com/robobee/core/internal/media"
    "github.com/robobee/core/internal/msgingest"
    "github.com/robobee/core/internal/platform"
    "github.com/robobee/core/internal/platform/dingtalk"
    "github.com/robobee/core/internal/platform/feishu"
    "github.com/robobee/core/internal/platform/local"
    "github.com/robobee/core/internal/store"
    "github.com/robobee/core/internal/task_dispatcher"
    "github.com/robobee/core/internal/task_scheduler"
    "github.com/robobee/core/internal/worker"
    webui "github.com/robobee/core/web"
)

// App holds all wired-up components and runs the server.
type App struct {
    db      *sql.DB
    server  *api.Server
    runners []func(ctx context.Context)
    addr    string
}

// Run starts all goroutines, waits for a signal, then shuts down.
func (a *App) Run() {
    ctx, cancel := context.WithCancel(context.Background())

    for _, r := range a.runners {
        r := r
        go r(ctx)
    }

    quit := make(chan os.Signal, 1)
    signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
    go func() {
        <-quit
        slog.Info("Shutting down...")
        cancel()
        a.db.Close()
        os.Exit(0)
    }()

    slog.Info("RoboBee Core starting", "addr", a.addr)
    if err := a.server.Run(a.addr); err != nil {
        slog.Error("server error", "error", err)
        os.Exit(1)
    }
}

// buildApp wires all components together. Returns a ready-to-run App.
func buildApp(cfg config.Config) (*App, error) {
    if !cfg.Server.Debug {
        gin.SetMode(gin.ReleaseMode)
    }

    if cfg.Bee.MCP.APIKey == "" {
        return nil, fmt.Errorf("bee.mcp.api_key must be set — bee requires MCP to create tasks")
    }

    db, s, err := buildStores(cfg.Database)
    if err != nil {
        return nil, err
    }

    mgr := buildWorkerManager(cfg.Bee, s)

    dispatchCh := make(chan task_dispatcher.DispatchTask, 128)

    sendersByPlatform := make(map[string]platform.PlatformSenderAdapter)

    feeder, sched := buildBee(cfg.Bee, s, dispatchCh)
    ingest, disp := buildPipeline(cfg.Bee.MessageDebounce, s, mgr, dispatchCh)

    // Local platform — always enabled, separate gateway with short debounce
    localHub := local.NewSSEHub()
    localReceiver := local.NewLocalReceiver(64)
    localSender := local.NewLocalSender(s.localReplyStore, localHub)
    localIngest := msgingest.New(s.msgStore, 100*time.Millisecond)
    sendersByPlatform["local"] = localSender

    mcpSrv := mcp.NewServer(s.workerStore, mgr, s.taskStore, s.msgStore, sendersByPlatform, mgr, disp)
    platforms := buildPlatforms(cfg.Bee.Platforms.Feishu, cfg.Bee.Platforms.DingTalk)

    // Populate sender map for external platforms
    for _, p := range platforms {
        sendersByPlatform[p.ID()] = p.Sender()
    }

    // Synchronous startup recovery — must run before goroutines start
    feeder.RecoverFeeding(context.Background())
    sched.RecoverRunning(context.Background())

    runners := []func(ctx context.Context){
        func(ctx context.Context) { ingest.Run(ctx) },
        func(ctx context.Context) { localIngest.Run(ctx) },
        func(ctx context.Context) { feeder.Run(ctx) },
        func(ctx context.Context) { sched.Run(ctx) },
        func(ctx context.Context) { disp.Run(ctx) },
        func(ctx context.Context) {
            if err := localReceiver.Start(ctx, localIngest.Dispatch); err != nil {
                slog.Error("local receiver error", "error", err)
            }
        },
    }
    for _, p := range platforms {
        recv := p.Receiver()
        runners = append(runners, func(ctx context.Context) {
            if err := recv.Start(ctx, ingest.Dispatch); err != nil {
                slog.Error("platform receiver error", "error", err)
            }
        })
    }

    localChatHandler := api.NewLocalChatHandler(
        localReceiver, localHub,
        s.localSessionStore, s.localReplyStore,
        s.msgStore, s.sessionStore,
    )

    srv := buildAPIServer(cfg.Bee.MCP, s, mgr, mcpSrv, localChatHandler)
    addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)

    return &App{db: db, server: srv, runners: runners, addr: addr}, nil
}

// appStores groups all store instances for passing to sub-builders.
type appStores struct {
    workerStore      *store.WorkerStore
    execStore        *store.ExecutionStore
    msgStore         *store.MessageStore
    taskStore        *store.TaskStore
    sessionStore     *store.SessionStore
    localSessionStore *store.LocalSessionStore
    localReplyStore  *store.LocalReplyStore
}

func buildStores(cfg config.DatabaseConfig) (*sql.DB, appStores, error) {
    db, err := store.InitDB(cfg.Path)
    if err != nil {
        return nil, appStores{}, fmt.Errorf("init database: %w", err)
    }
    return db, appStores{
        workerStore:      store.NewWorkerStore(db),
        execStore:        store.NewExecutionStore(db),
        msgStore:         store.NewMessageStore(db),
        taskStore:        store.NewTaskStore(db),
        sessionStore:     store.NewSessionStore(db),
        localSessionStore: store.NewLocalSessionStore(db),
        localReplyStore:  store.NewLocalReplyStore(db),
    }, nil
}

func buildWorkerManager(bc config.BeeConfig, s appStores) *worker.Manager {
    return worker.NewManager(config.DefaultWorkerBaseDir(), bc, s.workerStore, s.execStore)
}

func buildBee(cfg config.BeeConfig, s appStores, dispatchCh chan task_dispatcher.DispatchTask) (*bee.Feeder, *task_scheduler.Scheduler) {
    beeProcess := bee.NewBeeProcess(cfg)
    feeder := bee.NewFeeder(s.msgStore, s.taskStore, s.sessionStore, beeProcess, config.DefaultBeeWorkDir(), cfg)
    sched := task_scheduler.New(s.taskStore, dispatchCh, cfg.Feeder.Interval)
    return feeder, sched
}

func buildPipeline(
    debounce time.Duration,
    s appStores,
    mgr *worker.Manager,
    dispatchCh chan task_dispatcher.DispatchTask,
) (*msgingest.Gateway, *task_dispatcher.TaskDispatcher) {
    ingest := msgingest.New(s.msgStore, debounce)
    disp := task_dispatcher.New(mgr, s.taskStore, s.sessionStore, s.execStore, dispatchCh)
    return ingest, disp
}

func buildPlatforms(fc config.FeishuConfig, dc config.DingTalkConfig) []platform.Platform {
    mediaSvc := media.NewService()
    var result []platform.Platform
    if fc.Enabled {
        result = append(result, feishu.NewPlatform(fc, mediaSvc))
    }
    if dc.Enabled {
        result = append(result, dingtalk.NewPlatform(dc, mediaSvc))
    }
    return result
}

func buildAPIServer(cfg config.MCPConfig, s appStores, mgr *worker.Manager, mcpSrv *mcp.MCPServer, localChat *api.LocalChatHandler) *api.Server {
    return api.NewServer(s.workerStore, s.execStore, mgr, mcpSrv, cfg.APIKey, webui.DistFS, localChat)
}
```

- [ ] **Step 2: Compile and verify**

```bash
go build ./...
```

Expected: compiles with no errors.

- [ ] **Step 3: Run all Go tests**

```bash
go test ./...
```

Expected: all tests PASS.

- [ ] **Step 4: Commit**

```bash
git add cmd/server/app.go
git commit -m "feat(app): wire local platform, localIngest gateway, and LocalChatHandler"
```

---

## Chunk 4: Frontend

### Task 10: API types and client

**Files:**
- Modify: `web/src/lib/api.ts`
- Modify: `web/src/locales/zh.json`
- Modify: `web/src/locales/en.json`

- [ ] **Step 1: Add localChat API methods to api.ts**

Add to `web/src/lib/api.ts` after the `sessions` block:

```typescript
localChat: {
  listSessions: async () => {
    const sessions = await fetchAPI<LocalChatSession[] | null>("/local/sessions")
    return Array.isArray(sessions) ? sessions : []
  },
  createSession: (name: string) =>
    fetchAPI<LocalChatSession>("/local/sessions", {
      method: "POST",
      body: JSON.stringify({ name }),
    }),
  deleteSession: (id: string) =>
    fetchAPI(`/local/sessions/${id}`, { method: "DELETE" }),
  sendMessage: (sessionId: string, content: string, mediaPath?: string) =>
    fetchAPI(`/local/sessions/${sessionId}/messages`, {
      method: "POST",
      body: JSON.stringify({ content, media_path: mediaPath }),
    }),
  getMessages: async (sessionId: string) => {
    const msgs = await fetchAPI<ChatMessage[] | null>(`/local/sessions/${sessionId}/messages`)
    return Array.isArray(msgs) ? msgs : []
  },
  uploadMedia: async (sessionId: string, file: File) => {
    const form = new FormData()
    form.append("file", file)
    const res = await fetch(`${API_BASE}/local/sessions/${sessionId}/media`, {
      method: "POST",
      body: form,
    })
    if (!res.ok) throw new Error(await res.text())
    return res.json() as Promise<{ path: string }>
  },
},
```

Also add these type definitions to `web/src/lib/types.ts`:

```typescript
export interface LocalChatSession {
  id: string
  name: string
  created_at: number
  updated_at: number
}

export interface ChatMessage {
  role: "user" | "bee"
  content: string
  ts: number
}
```

Update the import at the top of `web/src/lib/api.ts` to:

```typescript
import type { Worker, WorkerExecution, LocalChatSession, ChatMessage } from "./types"
```

- [ ] **Step 2: Add translation keys**

Add to `web/src/locales/zh.json`:
```json
"localChat": {
  "title": "本地对话",
  "newChat": "新建对话",
  "sessionNamePlaceholder": "对话名称",
  "inputPlaceholder": "输入消息...",
  "send": "发送",
  "uploadFile": "上传文件",
  "processing": "处理中...",
  "deleteSession": "删除对话",
  "emptyState": "暂无对话，点击"新建对话"开始"
}
```

Add to `web/src/locales/en.json`:
```json
"localChat": {
  "title": "Local Chat",
  "newChat": "New Chat",
  "sessionNamePlaceholder": "Session name",
  "inputPlaceholder": "Type a message...",
  "send": "Send",
  "uploadFile": "Upload file",
  "processing": "Processing...",
  "deleteSession": "Delete session",
  "emptyState": "No sessions yet. Click \"New Chat\" to start."
}
```

- [ ] **Step 3: Verify frontend compiles**

```bash
cd /Users/tengteng/work/robobee/core/web
pnpm build
```

Expected: builds successfully (may warn about unused imports — fix those).

- [ ] **Step 4: Commit**

```bash
cd /Users/tengteng/work/robobee/core
git add web/src/lib/api.ts web/src/lib/types.ts web/src/locales/zh.json web/src/locales/en.json
git commit -m "feat(web): add localChat API client, types, and i18n keys"
```

---

### Task 11: use-local-chat hook

**Files:**
- Create: `web/src/hooks/use-local-chat.ts`

- [ ] **Step 1: Create the hook**

Create `web/src/hooks/use-local-chat.ts`:

```typescript
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query"
import { useEffect, useRef } from "react"
import { api } from "@/lib/api"
import { config } from "@/lib/config"
import type { ChatMessage } from "@/lib/types"

export function useLocalSessions() {
  return useQuery({
    queryKey: ["local-sessions"],
    queryFn: () => api.localChat.listSessions(),
  })
}

export function useLocalMessages(sessionId: string) {
  return useQuery({
    queryKey: ["local-messages", sessionId],
    queryFn: () => api.localChat.getMessages(sessionId),
    enabled: !!sessionId,
  })
}

export function useCreateSession() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (name: string) => api.localChat.createSession(name),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["local-sessions"] }),
  })
}

export function useDeleteSession() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => api.localChat.deleteSession(id),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["local-sessions"] }),
  })
}

export function useSendMessage(sessionId: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ content, mediaPath }: { content: string; mediaPath?: string }) =>
      api.localChat.sendMessage(sessionId, content, mediaPath),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["local-sessions"] })
    },
  })
}

// useLocalChatStream subscribes to SSE for a session.
// Calls onReply on each new reply event and re-fetches history on reconnect.
export function useLocalChatStream(
  sessionId: string,
  onReply: (msg: ChatMessage) => void
) {
  const queryClient = useQueryClient()
  const onReplyRef = useRef(onReply)
  onReplyRef.current = onReply

  useEffect(() => {
    if (!sessionId) return
    let es: EventSource
    let reconnectTimer: ReturnType<typeof setTimeout>

    const connect = () => {
      es = new EventSource(`${config.apiUrl}/local/sessions/${sessionId}/stream`)

      es.onmessage = (event) => {
        try {
          const data = JSON.parse(event.data)
          onReplyRef.current({
            role: "bee",
            content: data.content,
            ts: data.created_at,
          })
        } catch {
          // ignore malformed events
        }
      }

      es.onerror = () => {
        es.close()
        // Re-fetch full history to fill any gap, then reconnect
        queryClient.invalidateQueries({ queryKey: ["local-messages", sessionId] })
        reconnectTimer = setTimeout(connect, 2000)
      }
    }

    connect()

    return () => {
      clearTimeout(reconnectTimer)
      es?.close()
    }
  }, [sessionId, queryClient])
}
```

- [ ] **Step 2: Verify frontend compiles**

```bash
cd /Users/tengteng/work/robobee/core/web && pnpm build
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
cd /Users/tengteng/work/robobee/core
git add web/src/hooks/use-local-chat.ts
git commit -m "feat(web): add use-local-chat hooks with SSE streaming"
```

---

### Task 12: Session list page

**Files:**
- Create: `web/src/pages/local-chat.tsx`

- [ ] **Step 1: Create the session list page**

Create `web/src/pages/local-chat.tsx`:

```tsx
import { useState } from "react"
import { useNavigate } from "react-router-dom"
import { useTranslation } from "react-i18next"
import { useLocalSessions, useCreateSession, useDeleteSession } from "@/hooks/use-local-chat"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Dialog, DialogContent, DialogHeader } from "@/components/ui/dialog"
import { Card, CardContent } from "@/components/ui/card"

export function LocalChat() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { data: sessions = [], isLoading } = useLocalSessions()
  const createSession = useCreateSession()
  const deleteSession = useDeleteSession()
  const [dialogOpen, setDialogOpen] = useState(false)
  const [newName, setNewName] = useState("")

  const handleCreate = async () => {
    if (!newName.trim()) return
    const session = await createSession.mutateAsync(newName.trim())
    setNewName("")
    setDialogOpen(false)
    navigate(`/local-chat/${session.id}`)
  }

  if (isLoading) return <p>Loading...</p>

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-2xl font-bold">{t("localChat.title")}</h1>
        <Button onClick={() => setDialogOpen(true)}>{t("localChat.newChat")}</Button>
      </div>

      {sessions.length === 0 ? (
        <p className="text-muted-foreground">{t("localChat.emptyState")}</p>
      ) : (
        <div className="space-y-2">
          {sessions.map((sess) => (
            <Card
              key={sess.id}
              className="cursor-pointer hover:bg-accent transition-colors"
              onClick={() => navigate(`/local-chat/${sess.id}`)}
            >
              <CardContent className="py-3 px-4 flex items-center justify-between">
                <div>
                  <p className="font-medium">{sess.name}</p>
                  <p className="text-xs text-muted-foreground">
                    {new Date(sess.updated_at).toLocaleString()}
                  </p>
                </div>
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={(e) => {
                    e.stopPropagation()
                    deleteSession.mutate(sess.id)
                  }}
                >
                  ✕
                </Button>
              </CardContent>
            </Card>
          ))}
        </div>
      )}

      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent>
          <DialogHeader>{t("localChat.newChat")}</DialogHeader>
          <Input
            placeholder={t("localChat.sessionNamePlaceholder")}
            value={newName}
            onChange={(e) => setNewName(e.target.value)}
            onKeyDown={(e) => e.key === "Enter" && handleCreate()}
            autoFocus
          />
          <Button onClick={handleCreate} disabled={!newName.trim() || createSession.isPending}>
            {t("localChat.newChat")}
          </Button>
        </DialogContent>
      </Dialog>
    </div>
  )
}
```

- [ ] **Step 2: Verify frontend compiles**

```bash
cd /Users/tengteng/work/robobee/core/web && pnpm build
```

- [ ] **Step 3: Commit**

```bash
cd /Users/tengteng/work/robobee/core
git add web/src/pages/local-chat.tsx
git commit -m "feat(web): add local chat session list page"
```

---

### Task 13: Chat detail page + nav + routes

**Files:**
- Create: `web/src/pages/local-chat-detail.tsx`
- Modify: `web/src/app.tsx`
- Modify: `web/src/components/nav.tsx`

- [ ] **Step 1: Create the chat detail page**

Create `web/src/pages/local-chat-detail.tsx`:

```tsx
import { useState, useRef, useEffect, useCallback } from "react"
import { useParams, Link } from "react-router-dom"
import { useTranslation } from "react-i18next"
import {
  useLocalMessages,
  useSendMessage,
  useLocalChatStream,
} from "@/hooks/use-local-chat"
import { api } from "@/lib/api"
import { Button } from "@/components/ui/button"
import { Textarea } from "@/components/ui/textarea"
import type { ChatMessage } from "@/lib/types"

export function LocalChatDetail() {
  const { t } = useTranslation()
  const { id } = useParams<{ id: string }>()
  const sessionId = id!

  const { data: history = [] } = useLocalMessages(sessionId)
  const sendMessage = useSendMessage(sessionId)

  const [localMessages, setLocalMessages] = useState<ChatMessage[]>([])
  const [input, setInput] = useState("")
  const [isProcessing, setIsProcessing] = useState(false)
  const [pendingMediaPath, setPendingMediaPath] = useState<string | undefined>()
  const fileInputRef = useRef<HTMLInputElement>(null)
  const bottomRef = useRef<HTMLDivElement>(null)

  // Sync history into local state when it loads/refetches
  useEffect(() => {
    setLocalMessages(history)
  }, [history])

  // SSE: append new replies as they arrive
  const handleReply = useCallback((msg: ChatMessage) => {
    setLocalMessages((prev) => [...prev, msg])
    setIsProcessing(false)
  }, [])
  useLocalChatStream(sessionId, handleReply)

  // Auto-scroll to bottom on new messages
  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: "smooth" })
  }, [localMessages, isProcessing])

  const handleSend = async () => {
    const content = input.trim()
    if (!content && !pendingMediaPath) return

    const userMsg: ChatMessage = { role: "user", content, ts: Date.now() }
    setLocalMessages((prev) => [...prev, userMsg])
    setInput("")
    setIsProcessing(true)

    await sendMessage.mutateAsync({ content: content || " ", mediaPath: pendingMediaPath })
    setPendingMediaPath(undefined)
  }

  const handleFileChange = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0]
    if (!file) return
    const { path } = await api.localChat.uploadMedia(sessionId, file)
    setPendingMediaPath(path)
  }

  return (
    <div className="flex flex-col h-[calc(100vh-8rem)]">
      {/* Header */}
      <div className="flex items-center gap-3 mb-4">
        <Link to="/local-chat" className="text-sm text-muted-foreground hover:underline">
          ← {t("localChat.title")}
        </Link>
      </div>

      {/* Messages */}
      <div className="flex-1 overflow-y-auto space-y-3 pb-2">
        {localMessages.map((msg, i) => (
          <div
            key={i}
            className={`flex ${msg.role === "user" ? "justify-end" : "justify-start"}`}
          >
            <div
              className={`max-w-[70%] rounded-xl px-4 py-2 text-sm whitespace-pre-wrap ${
                msg.role === "user"
                  ? "bg-primary text-primary-foreground rounded-br-sm"
                  : "bg-muted rounded-bl-sm"
              }`}
            >
              {msg.role === "bee" && (
                <p className="text-xs text-muted-foreground mb-1">🤖 bee</p>
              )}
              {msg.content}
            </div>
          </div>
        ))}

        {isProcessing && (
          <div className="flex justify-start">
            <div className="bg-muted rounded-xl px-4 py-2 text-sm text-muted-foreground">
              {t("localChat.processing")}
            </div>
          </div>
        )}
        <div ref={bottomRef} />
      </div>

      {/* Input */}
      <div className="border-t pt-3">
        {pendingMediaPath && (
          <p className="text-xs text-muted-foreground mb-1 truncate">
            📎 {pendingMediaPath}
          </p>
        )}
        <div className="flex gap-2 items-end">
          <Textarea
            className="flex-1 min-h-[40px] max-h-[120px] resize-none"
            placeholder={t("localChat.inputPlaceholder")}
            value={input}
            onChange={(e) => setInput(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter" && !e.shiftKey) {
                e.preventDefault()
                handleSend()
              }
            }}
          />
          <input
            ref={fileInputRef}
            type="file"
            className="hidden"
            onChange={handleFileChange}
          />
          <Button variant="outline" size="icon" onClick={() => fileInputRef.current?.click()}>
            📎
          </Button>
          <Button onClick={handleSend} disabled={sendMessage.isPending}>
            {t("localChat.send")}
          </Button>
        </div>
      </div>
    </div>
  )
}
```

- [ ] **Step 2: Add routes to app.tsx**

Add two lazy imports after the existing `SessionDetail` import:

```tsx
const LocalChat = lazy(() => import("@/pages/local-chat").then(m => ({ default: m.LocalChat })))
const LocalChatDetail = lazy(() => import("@/pages/local-chat-detail").then(m => ({ default: m.LocalChatDetail })))
```

Add two routes inside the `<Route element={<Layout />}>` block, after the `sessions` route:

```tsx
<Route path="/local-chat" element={<LocalChat />} />
<Route path="/local-chat/:id" element={<LocalChatDetail />} />
```

- [ ] **Step 3: Add nav item to nav.tsx**

Replace the `links` array and the active-state className in `web/src/components/nav.tsx`:

```tsx
const links = [
  { href: "/", label: t("nav.dashboard") },
  { href: "/workers", label: t("nav.workers") },
  { href: "/executions", label: t("nav.executions") },
  { href: "/local-chat", label: t("localChat.title") },
]
```

Update the `className` on the `<Link>` to use prefix matching for `/local-chat`:

```tsx
className={cn(
  "text-sm font-medium transition-colors hover:text-primary",
  (pathname === link.href || (link.href !== "/" && pathname.startsWith(link.href)))
    ? "text-foreground"
    : "text-muted-foreground"
)}
```

This keeps exact matching for `/` (dashboard) while using prefix matching for all other paths, so `/local-chat/:id` correctly highlights "Local Chat" in the nav.

- [ ] **Step 4: Build and verify**

```bash
cd /Users/tengteng/work/robobee/core/web && pnpm build
```

Expected: builds with no errors.

- [ ] **Step 5: Commit**

```bash
cd /Users/tengteng/work/robobee/core
git add web/src/pages/local-chat-detail.tsx web/src/app.tsx web/src/components/nav.tsx
git commit -m "feat(web): add local chat detail page, nav item, and routes"
```

---

## Final Verification

- [ ] **Start the server and smoke-test manually**

```bash
cd /Users/tengteng/work/robobee/core
go run ./cmd/server
```

Open `http://localhost:<port>` (from config.yaml) and:
1. Navigate to "本地对话 / Local Chat"
2. Create a new session named "Test"
3. Send a text message — verify it appears in the chat
4. Upload a file and send — verify `[文件] /path/...` appears in message
5. Watch for bee's reply to appear in real time
6. Delete the session — verify it disappears from the list
7. Disconnect/reconnect the browser tab — verify history reloads

- [ ] **Run all Go tests one final time**

```bash
go test ./...
```

Expected: all PASS.

- [ ] **Final commit if any cleanup needed**

```bash
git add -p  # stage only intentional changes
git commit -m "chore: final cleanup for local chat feature"
```
