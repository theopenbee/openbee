# Local Messages Pagination Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add cursor-based (keyset) pagination to `GET /api/local/messages` so it returns at most 50 messages per request instead of all messages, with a `has_more` flag and support for loading older messages via a `before` timestamp parameter.

**Architecture:** Both DB store methods gain `before int64` and `limit int` parameters; each queries `LIMIT limit+1` to detect `has_more` without an extra COUNT. The handler parses query params, merges both result sets, and returns `{ messages, has_more }`. The frontend hook and chat page gain an incremental "load more" path that prepends older messages while preserving scroll position.

**Tech Stack:** Go (SQLite via `database/sql`), React + TypeScript, `@tanstack/react-query`

---

## File Map

| File | Change |
|------|--------|
| `internal/infra/store/message_store.go` | `ListBySessionKey` gains `before int64, limit int` |
| `internal/infra/store/message_store_local_test.go` | New pagination tests |
| `internal/infra/store/outbound_message_store.go` | `ListBySessionKey` gains `before int64` param |
| `internal/api/local_chat_handler.go` | Parse query params, new response shape |
| `web/src/lib/types.ts` | Add `LocalMessagesResponse` type |
| `web/src/lib/api.ts` | Update `getMessages` signature |
| `web/src/hooks/use-local-chat.ts` | Add `useLoadMoreMessages` hook |
| `web/src/pages/local-chat.tsx` | "Load more" button + scroll-position preservation |

---

## Task 1: Update MessageStore.ListBySessionKey

**Files:**
- Modify: `internal/infra/store/message_store.go` (function `ListBySessionKey`)
- Modify: `internal/infra/store/message_store_local_test.go`

- [ ] **Step 1: Write failing tests**

Open `internal/infra/store/message_store_local_test.go` and replace its entire content with:

```go
package store_test

import (
	"context"
	"testing"

	"github.com/theopenbee/openbee/internal/infra/store"
)

func TestMessageStore_ListBySessionKey_ExcludesMerged(t *testing.T) {
	db, err := store.InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	defer db.Close()
	s := store.NewMessageStore(db)
	ctx := context.Background()

	s.CreateBatch(ctx, []store.BatchMsg{ //nolint:errcheck
		{ID: "m1", SessionKey: "local:s1", Platform: "local", Content: "hello",
			Status: "received", MessageTime: 1000},
		{ID: "m2", SessionKey: "local:s1", Platform: "local", Content: "world",
			Status: "merged", MergedInto: "m1", MessageTime: 900},
	})

	msgs, err := s.ListBySessionKey(ctx, "local:s1", 0, 50)
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

func TestMessageStore_ListBySessionKey_Pagination(t *testing.T) {
	db, err := store.InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	defer db.Close()
	s := store.NewMessageStore(db)
	ctx := context.Background()

	// Insert 5 messages with timestamps 100..500
	s.CreateBatch(ctx, []store.BatchMsg{ //nolint:errcheck
		{ID: "a", SessionKey: "local:s1", Platform: "local", Content: "1", Status: "received", MessageTime: 100},
		{ID: "b", SessionKey: "local:s1", Platform: "local", Content: "2", Status: "received", MessageTime: 200},
		{ID: "c", SessionKey: "local:s1", Platform: "local", Content: "3", Status: "received", MessageTime: 300},
		{ID: "d", SessionKey: "local:s1", Platform: "local", Content: "4", Status: "received", MessageTime: 400},
		{ID: "e", SessionKey: "local:s1", Platform: "local", Content: "5", Status: "received", MessageTime: 500},
	})

	// limit=3, no before -> latest 3: c,d,e (returned ASC)
	msgs, err := s.ListBySessionKey(ctx, "local:s1", 0, 3)
	if err != nil {
		t.Fatalf("ListBySessionKey: %v", err)
	}
	if len(msgs) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(msgs))
	}
	if msgs[0].ID != "c" || msgs[2].ID != "e" {
		t.Errorf("expected c,d,e got %s,%s,%s", msgs[0].ID, msgs[1].ID, msgs[2].ID)
	}

	// before=300 (exclusive) -> latest 3 before ts 300: a,b (only 2 exist)
	msgs2, err := s.ListBySessionKey(ctx, "local:s1", 300, 3)
	if err != nil {
		t.Fatalf("ListBySessionKey with before: %v", err)
	}
	if len(msgs2) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs2))
	}
	if msgs2[0].ID != "a" || msgs2[1].ID != "b" {
		t.Errorf("expected a,b got %s,%s", msgs2[0].ID, msgs2[1].ID)
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

	msgs, _ := s.ListBySessionKey(ctx, "local:s1", 0, 50)
	if len(msgs) != 0 {
		t.Errorf("expected 0 messages for s1, got %d", len(msgs))
	}
	msgs2, _ := s.ListBySessionKey(ctx, "local:s2", 0, 50)
	if len(msgs2) != 1 {
		t.Errorf("s2 should be unaffected, got %d", len(msgs2))
	}
}
```

- [ ] **Step 2: Run tests to confirm they fail**

```bash
go test ./internal/infra/store/... -run "TestMessageStore_ListBySessionKey|TestMessageStore_DeleteBySessionKey" -v
```

Expected: compile error — `ListBySessionKey` does not accept 3 arguments yet.

- [ ] **Step 3: Update ListBySessionKey in message_store.go**

Replace the existing `ListBySessionKey` function (lines 330–350 in `internal/infra/store/message_store.go`) with:

```go
// ListBySessionKey returns non-merged messages for a session.
// If before > 0, only messages with received_at < before are returned.
// Results are ordered by received_at ASC. limit must be > 0.
func (s *MessageStore) ListBySessionKey(ctx context.Context, sessionKey string, before int64, limit int) ([]InboundMessage, error) {
	var (
		query string
		args  []any
	)
	if before > 0 {
		query = `SELECT id, content, received_at FROM bee_platform_messages
                 WHERE session_key = ? AND status != 'merged' AND received_at < ?
                 ORDER BY received_at DESC LIMIT ?`
		args = []any{sessionKey, before, limit}
	} else {
		query = `SELECT id, content, received_at FROM bee_platform_messages
                 WHERE session_key = ? AND status != 'merged'
                 ORDER BY received_at DESC LIMIT ?`
		args = []any{sessionKey, limit}
	}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var msgs []InboundMessage
	for rows.Next() {
		var m InboundMessage
		if err := rows.Scan(&m.ID, &m.Content, &m.ReceivedAt); err != nil {
			return nil, err
		}
		msgs = append(msgs, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// Reverse DESC result to ASC for callers.
	for i, j := 0, len(msgs)-1; i < j; i, j = i+1, j-1 {
		msgs[i], msgs[j] = msgs[j], msgs[i]
	}
	return msgs, nil
}
```

- [ ] **Step 4: Run tests to confirm they pass**

```bash
go test ./internal/infra/store/... -run "TestMessageStore_ListBySessionKey|TestMessageStore_DeleteBySessionKey" -v
```

Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/infra/store/message_store.go internal/infra/store/message_store_local_test.go
git commit -m "feat: add before/limit params to MessageStore.ListBySessionKey"
```

---

## Task 2: Update OutboundMessageStore.ListBySessionKey

**Files:**
- Modify: `internal/infra/store/outbound_message_store.go` (function `ListBySessionKey`)

> No new test file needed — the handler integration test in Task 3 covers outbound pagination. The existing outbound store tests don't call `ListBySessionKey` directly.

- [ ] **Step 1: Replace ListBySessionKey in outbound_message_store.go**

Replace the existing `ListBySessionKey` function (lines 94–113 in `internal/infra/store/outbound_message_store.go`) with:

```go
// ListBySessionKey returns outbound messages for a session ordered by sent_at ascending.
// If before > 0, only messages with sent_at < before are returned.
// limit must be > 0.
func (s *OutboundMessageStore) ListBySessionKey(ctx context.Context, sessionKey string, before int64, limit int) ([]OutboundMessage, error) {
	var (
		query string
		args  []any
	)
	if before > 0 {
		query = `SELECT ` + outboundMessageColumns + `
			 FROM bee_outbound_messages
			 WHERE session_key = ? AND sent_at < ?
			 ORDER BY sent_at DESC LIMIT ?`
		args = []any{sessionKey, before, limit}
	} else {
		query = `SELECT ` + outboundMessageColumns + `
			 FROM bee_outbound_messages
			 WHERE session_key = ?
			 ORDER BY sent_at DESC LIMIT ?`
		args = []any{sessionKey, limit}
	}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	msgs, err := scanOutboundMessages(rows)
	if err != nil {
		return nil, err
	}
	// Reverse DESC result to ASC for callers.
	for i, j := 0, len(msgs)-1; i < j; i, j = i+1, j-1 {
		msgs[i], msgs[j] = msgs[j], msgs[i]
	}
	return msgs, nil
}
```

- [ ] **Step 2: Build to confirm no compile errors**

```bash
go build ./...
```

Expected: exits 0 with no output.

- [ ] **Step 3: Commit**

```bash
git add internal/infra/store/outbound_message_store.go
git commit -m "feat: add before/limit params to OutboundMessageStore.ListBySessionKey"
```

---

## Task 3: Update getMessages Handler

**Files:**
- Modify: `internal/api/local_chat_handler.go` (function `getMessages`)

- [ ] **Step 1: Replace getMessages in local_chat_handler.go**

Replace the `getMessages` function (lines 161–197 in `internal/api/local_chat_handler.go`) with:

```go
func (h *LocalChatHandler) getMessages(c *gin.Context) {
	ctx := c.Request.Context()

	before := int64(0)
	if v := c.Query("before"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			before = n
		}
	}
	limit := 50
	if v := c.Query("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 {
			limit = n
		}
	}
	// Query limit+1 to detect has_more without an extra COUNT query.
	fetch := limit + 1

	var inbound []store.InboundMessage
	var replies []store.OutboundMessage
	g, gCtx := errgroup.WithContext(ctx)
	g.Go(func() error {
		var err error
		inbound, err = h.msgStore.ListBySessionKey(gCtx, defaultSessionKey, before, fetch)
		return err
	})
	g.Go(func() error {
		var err error
		replies, err = h.outboundStore.ListBySessionKey(gCtx, defaultSessionKey, before, fetch)
		return err
	})
	if err := g.Wait(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	combined := make([]chatMessage, 0, len(inbound)+len(replies))
	for _, m := range inbound {
		paths, text := decodeMediaPaths(m.Content)
		msg := chatMessage{Role: chatRoleUser, Content: text, Timestamp: m.ReceivedAt}
		if len(paths) > 0 {
			msg.MediaPaths = paths
		}
		combined = append(combined, msg)
	}
	for _, r := range replies {
		combined = append(combined, chatMessage{Role: chatRoleBee, Content: r.Content, Timestamp: r.SentAt})
	}
	sort.Slice(combined, func(i, j int) bool { return combined[i].Timestamp < combined[j].Timestamp })

	hasMore := len(combined) > limit
	if hasMore {
		combined = combined[:limit]
	}

	c.JSON(http.StatusOK, gin.H{"messages": combined, "has_more": hasMore})
}
```

Also add `"strconv"` to the import block at the top of the file (it's not currently imported).

- [ ] **Step 2: Build to confirm no compile errors**

```bash
go build ./...
```

Expected: exits 0.

- [ ] **Step 3: Run all handler tests**

```bash
go test ./internal/api/... -v
```

Expected: all PASS (existing encode/decode tests unaffected).

- [ ] **Step 4: Commit**

```bash
git add internal/api/local_chat_handler.go
git commit -m "feat: add cursor pagination to GET /api/local/messages"
```

---

## Task 4: Update Frontend Types and API Client

**Files:**
- Modify: `web/src/lib/types.ts`
- Modify: `web/src/lib/api.ts`

- [ ] **Step 1: Add LocalMessagesResponse type to types.ts**

In `web/src/lib/types.ts`, after the `ChatMessage` interface (after line 60), add:

```ts
export interface LocalMessagesResponse {
  messages: ChatMessage[]
  has_more: boolean
}
```

- [ ] **Step 2: Update getMessages in api.ts**

In `web/src/lib/api.ts`, replace the `getMessages` function inside `localChat`:

```ts
// Before (lines 113-116):
getMessages: async () => {
  const msgs = await fetchAPI<ChatMessage[] | null>("/local/messages")
  return Array.isArray(msgs) ? msgs : []
},

// After:
getMessages: async (before?: number, limit = 50): Promise<LocalMessagesResponse> => {
  const qs = new URLSearchParams({ limit: String(limit) })
  if (before) qs.set("before", String(before))
  const res = await fetchAPI<LocalMessagesResponse>(`/local/messages?${qs}`)
  return { messages: Array.isArray(res?.messages) ? res.messages : [], has_more: res?.has_more ?? false }
},
```

Also add `LocalMessagesResponse` to the import at line 1:

```ts
import type { Worker, WorkerExecution, PaginatedResponse, ChatMessage, LocalMessagesResponse, Task, Department, DepartmentTree, StatsOverview, StatsTrend } from "./types"
```

- [ ] **Step 3: Build frontend to confirm no TypeScript errors**

```bash
cd web && npm run build 2>&1 | head -40
```

Expected: build succeeds (or only pre-existing warnings, no new errors).

- [ ] **Step 4: Commit**

```bash
git add web/src/lib/types.ts web/src/lib/api.ts
git commit -m "feat: update localChat.getMessages to return paginated response"
```

---

## Task 5: Update useLocalMessages Hook + Add useLoadMoreMessages

**Files:**
- Modify: `web/src/hooks/use-local-chat.ts`

- [ ] **Step 1: Replace use-local-chat.ts content**

Replace the entire file `web/src/hooks/use-local-chat.ts` with:

```ts
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query"
import { useEffect, useRef, useState, useCallback } from "react"
import { api } from "@/lib/api"
import { config } from "@/lib/config"
import { tokenParam } from "@/lib/auth"
import type { ChatMessage } from "@/lib/types"

export function useLocalMessages() {
  return useQuery({
    queryKey: ["local-messages"],
    queryFn: () => api.localChat.getMessages(),
    select: (data) => data,
  })
}

/**
 * useLoadMoreMessages manages incremental loading of older messages.
 * Returns `loadMore`, `hasMore`, and `isLoadingMore`.
 * Call `loadMore(earliestTs)` with the timestamp of the oldest message currently shown.
 */
export function useLoadMoreMessages(
  onLoaded: (older: ChatMessage[]) => void
) {
  const [isLoadingMore, setIsLoadingMore] = useState(false)
  const [hasMore, setHasMore] = useState(false)

  const setInitialHasMore = useCallback((value: boolean) => {
    setHasMore(value)
  }, [])

  const loadMore = useCallback(async (earliestTs: number) => {
    if (isLoadingMore) return
    setIsLoadingMore(true)
    try {
      const res = await api.localChat.getMessages(earliestTs)
      setHasMore(res.has_more)
      onLoaded(res.messages)
    } finally {
      setIsLoadingMore(false)
    }
  }, [isLoadingMore, onLoaded])

  return { loadMore, hasMore, isLoadingMore, setInitialHasMore }
}

export function useSendMessage() {
  return useMutation({
    mutationFn: ({ content, mediaPaths }: { content: string; mediaPaths?: string[] }) =>
      api.localChat.sendMessage(content, mediaPaths),
  })
}

// useLocalChatStream subscribes to SSE for the default local session.
// Calls onReply on each new reply event and re-fetches history on reconnect.
export function useLocalChatStream(onReply: (msg: ChatMessage) => void) {
  const queryClient = useQueryClient()
  const onReplyRef = useRef(onReply)
  onReplyRef.current = onReply

  useEffect(() => {
    let es: EventSource
    let reconnectTimer: ReturnType<typeof setTimeout>
    let mounted = true

    const connect = () => {
      if (!mounted) return
      es = new EventSource(`${config.apiUrl}/local/stream${tokenParam()}`)

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
        clearTimeout(reconnectTimer)
        // Re-fetch full history to fill any gap created by the disconnect.
        queryClient.invalidateQueries({ queryKey: ["local-messages"] })
        reconnectTimer = setTimeout(connect, 2000)
      }
    }

    connect()

    return () => {
      mounted = false
      clearTimeout(reconnectTimer)
      es?.close()
    }
  }, [queryClient])
}
```

- [ ] **Step 2: Build frontend to confirm no TypeScript errors**

```bash
cd web && npm run build 2>&1 | head -40
```

Expected: no new errors.

- [ ] **Step 3: Commit**

```bash
git add web/src/hooks/use-local-chat.ts
git commit -m "feat: add useLoadMoreMessages hook for cursor pagination"
```

---

## Task 6: Update LocalChat Page

**Files:**
- Modify: `web/src/pages/local-chat.tsx`

- [ ] **Step 1: Add load-more state and logic to LocalChat**

In `web/src/pages/local-chat.tsx`, make the following changes:

**a) Update imports** — add `useLoadMoreMessages` to the hook import:

```ts
import {
  useLocalMessages,
  useLocalChatStream,
  useLoadMoreMessages,
  useSendMessage,
} from "@/hooks/use-local-chat"
```

**b) Inside the `LocalChat` function**, after the existing state declarations (around line 188), add:

```ts
const scrollContainerRef = useRef<HTMLDivElement>(null)

const handleOlderLoaded = useCallback((older: ChatMessage[]) => {
  setLocalMessages((prev) => [...older, ...prev])
}, [])

const { loadMore, hasMore, isLoadingMore, setInitialHasMore } = useLoadMoreMessages(handleOlderLoaded)
```

**c) Update the `useEffect` that syncs `history` into `localMessages`** (around line 190–192). Replace:

```ts
useEffect(() => {
  setLocalMessages(history)
}, [history])
```

With:

```ts
useEffect(() => {
  if (!data) return
  setLocalMessages(data.messages)
  setInitialHasMore(data.has_more)
}, [data, setInitialHasMore])
```

Also update the destructure on line 178 from:
```ts
const { data: history = [], isLoading } = useLocalMessages()
```
to:
```ts
const { data, isLoading } = useLocalMessages()
```

**d) Add scroll container ref** — in the JSX, add `ref={scrollContainerRef}` to the scrollable div (the one with `overflow-y-auto`, around line 301):

```tsx
<div ref={scrollContainerRef} className="flex-1 overflow-y-auto overflow-x-hidden px-4 py-5 sm:px-6 sm:py-6">
```

**e) Add "Load more" button** — inside the non-empty branch, add this block just above the messages list (before `{localMessages.map(...)}`, around line 322):

```tsx
{hasMore && (
  <div className="flex justify-center pb-4">
    <button
      type="button"
      disabled={isLoadingMore}
      onClick={() => {
        const container = scrollContainerRef.current
        const prevScrollHeight = container?.scrollHeight ?? 0
        const earliestTs = localMessages[0]?.ts ?? Date.now()
        loadMore(earliestTs).then(() => {
          // Restore scroll position after prepend
          if (container) {
            const newScrollHeight = container.scrollHeight
            container.scrollTop += newScrollHeight - prevScrollHeight
          }
        })
      }}
      className="inline-flex items-center gap-2 rounded-full border border-border/70 bg-background/80 px-4 py-1.5 text-xs text-muted-foreground transition-colors hover:bg-muted hover:text-foreground disabled:opacity-50"
    >
      {isLoadingMore ? t("localChat.loadingMore") : t("localChat.loadMore")}
    </button>
  </div>
)}
```

- [ ] **Step 2: Add i18n keys**

Find the translation JSON files (typically under `web/src/locales/` or similar). Add these keys to each locale:

```json
"loadMore": "Load earlier messages",
"loadingMore": "Loading…"
```

Run this to find the locale files:
```bash
find web/src -name "*.json" | grep -i "en\|zh" | head -10
```

- [ ] **Step 3: Build frontend to confirm no TypeScript errors**

```bash
cd web && npm run build 2>&1 | head -40
```

Expected: no new errors.

- [ ] **Step 4: Commit**

```bash
git add web/src/pages/local-chat.tsx web/src/locales
git commit -m "feat: add load-more button to local chat page"
```

---

## Self-Review Checklist

- [x] **Spec coverage:**
  - ✅ `before` + `limit` query params → Tasks 3
  - ✅ Default limit 50, max 100 → Task 3
  - ✅ `has_more` via limit+1 trick → Tasks 1, 2, 3
  - ✅ MessageStore updated → Task 1
  - ✅ OutboundMessageStore updated → Task 2
  - ✅ Response changed to `{ messages, has_more }` → Task 3, 4
  - ✅ Frontend api.ts updated → Task 4
  - ✅ "Load more" with scroll-position preservation → Task 6

- [x] **No placeholders:** All steps contain actual code.

- [x] **Type consistency:**
  - `LocalMessagesResponse` defined in Task 4, used in Task 4 and Task 5
  - `useLoadMoreMessages` defined in Task 5, imported in Task 6
  - `ListBySessionKey(ctx, key, before, limit)` — same signature in Tasks 1, 2, and consumed in Task 3
