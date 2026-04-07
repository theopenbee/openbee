# Local Chat Session Refactor Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove the explicit session creation step from local chat so users open `/local-chat` and start chatting immediately, with all messages stored under the fixed key `local:default`.

**Architecture:** Eliminate `bee_local_sessions` table usage and the session CRUD layer. The handler hardcodes `session_key = "local:default"` for all operations. The frontend collapses the two-page flow (list → detail) into a single chat page at `/local-chat`.

**Tech Stack:** Go 1.23 + gin (backend), React + TanStack Query (frontend), SQLite via `database/sql`

**Design spec:** `docs/superpowers/specs/2026-04-06-local-chat-session-refactor-design.md`

---

## File Map

| File | Action |
|---|---|
| `internal/infra/store/db.go` | Add migration v27 to mark bee_local_sessions deprecated |
| `internal/infra/store/local_session_store.go` | **Delete** |
| `internal/api/local_chat_handler.go` | Rewrite — remove session CRUD, hardcode `local:default` |
| `internal/api/router.go` | Simplify `registerLocalChatRoutes` |
| `internal/app/app.go` | Remove `localSessionStore` field and usage |
| `web/src/lib/api.ts` | Simplify `localChat` API object |
| `web/src/hooks/use-local-chat.ts` | Remove session hooks, simplify remaining hooks |
| `web/src/pages/local-chat.tsx` | Rewrite as single chat page |
| `web/src/pages/local-chat-detail.tsx` | **Delete** |
| `web/src/app.tsx` | Remove `/local-chat/:id` route, update import |

**Out of scope:** The AI reset mechanism (user sends "clear" → Bee calls existing CLI tool to clear `bee_session_contexts`) is a Bee brain/prompt configuration change, not a code change. That is handled separately in Bee's tool registration.

---

## Task 1: Add DB Migration to Mark bee_local_sessions Deprecated

**Files:**
- Modify: `internal/infra/store/db.go:249` (append after the last migration `v26`)

- [ ] **Step 1: Add migration v27**

In `internal/infra/store/db.go`, append the following entry to the `migrations` slice, immediately before the closing `}` of the slice (currently at line 250):

```go
	{
		version: 27,
		name:    "deprecate_bee_local_sessions",
		sql: `-- bee_local_sessions is deprecated. Local chat now uses the fixed
-- session_key "local:default" and no longer reads or writes this table.
-- The table is preserved for historical data only.
SELECT 1`,
	},
```

- [ ] **Step 2: Verify the app starts without error**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee2
go build ./...
```

Expected: no compilation errors.

- [ ] **Step 3: Commit**

```bash
git add internal/infra/store/db.go
git commit -m "feat(store): add migration v27 to mark bee_local_sessions deprecated"
```

---

## Task 2: Rewrite LocalChatHandler (Remove Session Layer)

**Files:**
- Modify: `internal/api/local_chat_handler.go` (full rewrite)

- [ ] **Step 1: Replace the handler file**

Replace the entire contents of `internal/api/local_chat_handler.go` with:

```go
package api

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/theopenbee/openbee/internal/infra/logger"
	"github.com/theopenbee/openbee/internal/platform"
	"github.com/theopenbee/openbee/internal/platform/local"
	"github.com/theopenbee/openbee/internal/infra/store"
)

var log = logger.With(zap.String("component", "api"))

// fileMediaMarker is the protocol prefix embedded in message content to carry a media path.
// It is shared between the write path (encodeMediaPaths) and the read path (decodeMediaPaths).
// The leading \x00 (NUL byte) prevents collision with ordinary user text, which cannot contain NUL.
const fileMediaMarker = "\x00[file]"

// legacyFileMediaMarker is the old marker written before the NUL prefix was added.
// decodeMediaPaths still recognises it so existing stored messages are decoded correctly.
const legacyFileMediaMarker = "[file]"

const fileMediaPrefix = fileMediaMarker + " "
const legacyFileMediaPrefix = legacyFileMediaMarker + " "

// defaultSessionKey is the fixed session key used for all local chat messages.
const defaultSessionKey = "local:default"

type LocalChatHandler struct {
	receiver      *local.LocalReceiver
	hub           *local.SSEHub
	outboundStore *store.OutboundMessageStore
	msgStore      *store.MessageStore
	sessionCtx    *store.SessionStore
}

func NewLocalChatHandler(
	receiver *local.LocalReceiver,
	hub *local.SSEHub,
	outboundStore *store.OutboundMessageStore,
	msgStore *store.MessageStore,
	sessionCtx *store.SessionStore,
) *LocalChatHandler {
	return &LocalChatHandler{
		receiver:      receiver,
		hub:           hub,
		outboundStore: outboundStore,
		msgStore:      msgStore,
		sessionCtx:    sessionCtx,
	}
}

func (h *LocalChatHandler) StreamReplies(c *gin.Context) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")

	ch, unsub := h.hub.Subscribe(defaultSessionKey)
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

func (h *LocalChatHandler) sendMessage(c *gin.Context) {
	var body struct {
		Content    string   `json:"content" binding:"required"`
		MediaPaths []string `json:"media_paths"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	for _, p := range body.MediaPaths {
		if strings.ContainsAny(p, "/\\") || p == ".." || p == "." {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid filename in media_paths"})
			return
		}
	}

	content := encodeMediaPaths(body.MediaPaths, body.Content)

	h.receiver.Enqueue(platform.InboundMessage{
		Platform:    local.PlatformID,
		SenderID:    "web",
		SessionKey:  defaultSessionKey,
		Content:     content,
		RawContent:  content,
		MessageTime: time.Now().UnixMilli(),
	})

	c.JSON(http.StatusAccepted, gin.H{"status": "queued"})
}

// encodeMediaPaths prepends zero or more "[file] name\n" lines to text.
func encodeMediaPaths(paths []string, text string) string {
	if len(paths) == 0 {
		return text
	}
	var sb strings.Builder
	for _, p := range paths {
		sb.WriteString(fileMediaMarker)
		sb.WriteByte(' ')
		sb.WriteString(p)
		sb.WriteByte('\n')
	}
	sb.WriteString(text)
	return sb.String()
}

// decodeMediaPaths extracts leading "<marker> name\n" lines from content.
// It recognises the current marker (\x00[file]) and the legacy marker ([file])
// so messages stored before the NUL prefix was introduced are still decoded correctly.
// Returns the list of filenames and the remaining text.
func decodeMediaPaths(content string) ([]string, string) {
	var paths []string
	for {
		var rest string
		switch {
		case strings.HasPrefix(content, fileMediaPrefix):
			rest = content[len(fileMediaPrefix):]
		case strings.HasPrefix(content, legacyFileMediaPrefix):
			rest = content[len(legacyFileMediaPrefix):]
		default:
			return paths, content
		}
		filename, after, ok := strings.Cut(rest, "\n")
		if !ok {
			break
		}
		paths = append(paths, filename)
		content = after
	}
	return paths, content
}

type chatMessage struct {
	Role       string   `json:"role"`
	Content    string   `json:"content"`
	MediaPaths []string `json:"media_paths,omitempty"`
	Timestamp  int64    `json:"ts"`
}

func (h *LocalChatHandler) getMessages(c *gin.Context) {
	ctx := c.Request.Context()

	inbound, err := h.msgStore.ListBySessionKey(ctx, defaultSessionKey)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	replies, err := h.outboundStore.ListBySessionKey(ctx, defaultSessionKey, 0)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	combined := make([]chatMessage, 0, len(inbound)+len(replies))
	for _, m := range inbound {
		paths, text := decodeMediaPaths(m.Content)
		msg := chatMessage{Role: "user", Content: text, Timestamp: m.ReceivedAt}
		if len(paths) > 0 {
			msg.MediaPaths = paths
		}
		combined = append(combined, msg)
	}
	for _, r := range replies {
		combined = append(combined, chatMessage{Role: "bee", Content: r.Content, Timestamp: r.SentAt})
	}
	sort.Slice(combined, func(i, j int) bool { return combined[i].Timestamp < combined[j].Timestamp })

	c.JSON(http.StatusOK, combined)
}

func (h *LocalChatHandler) uploadMedia(c *gin.Context) {
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing 'file' field"})
		return
	}
	defer file.Close()

	uploadDir, err := localUploadDir()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := os.MkdirAll(uploadDir, 0o755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	filename := uuid.New().String() + "_" + filepath.Base(header.Filename)
	destPath := filepath.Join(uploadDir, filename)
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

	c.JSON(http.StatusOK, gin.H{"path": filename})
}

func (h *LocalChatHandler) serveMedia(c *gin.Context) {
	filename := filepath.Base(c.Param("filename"))
	if filename == "." || filename == ".." {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid filename"})
		return
	}

	uploadDir, err := localUploadDir()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.File(filepath.Join(uploadDir, filename))
}

func localUploadDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get home dir: %w", err)
	}
	return filepath.Join(home, ".openbee", "local-uploads", "default"), nil
}
```

> Note: `uploadMedia` now prefixes each filename with a UUID to avoid collisions, since all uploads share a single directory.

- [ ] **Step 2: Verify compilation**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee2
go build ./internal/api/...
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add internal/api/local_chat_handler.go
git commit -m "feat(api): rewrite local chat handler to use fixed local:default session key"
```

---

## Task 3: Update Routes and App Initialization

**Files:**
- Modify: `internal/api/router.go:100-109`
- Modify: `internal/app/app.go:148-152` and `app.go:171,188`

- [ ] **Step 1: Update registerLocalChatRoutes in router.go**

Replace lines 100-109 in `internal/api/router.go`:

```go
func (s *Server) registerLocalChatRoutes(api *gin.RouterGroup) {
	api.POST("/local/messages", s.LocalChatHandler.sendMessage)
	api.GET("/local/messages", s.LocalChatHandler.getMessages)
	api.POST("/local/media", s.LocalChatHandler.uploadMedia)
	api.GET("/local/media/:filename", s.LocalChatHandler.serveMedia)
	api.GET("/local/stream", s.LocalChatHandler.StreamReplies)
}
```

- [ ] **Step 2: Remove localSessionStore from NewLocalChatHandler call in app.go**

In `internal/app/app.go`, replace lines 148-152:

```go
	localChatHandler := api.NewLocalChatHandler(
		localReceiver, localHub,
		s.outboundMsgStore, s.msgStore, s.sessionStore,
	)
```

- [ ] **Step 3: Remove localSessionStore from appStores struct in app.go**

In `internal/app/app.go`, remove `localSessionStore *store.LocalSessionStore` from the `appStores` struct (line 171) and remove `localSessionStore: store.NewLocalSessionStore(db),` from `buildStores` (line 188).

- [ ] **Step 4: Verify compilation**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee2
go build ./...
```

Expected: no errors. If you see `undefined: store.LocalSessionStore`, you haven't removed all references yet — search with `grep -r "LocalSessionStore" internal/`.

- [ ] **Step 5: Commit**

```bash
git add internal/api/router.go internal/app/app.go
git commit -m "feat(api): simplify local chat routes and remove session store dependency"
```

---

## Task 4: Delete local_session_store.go

**Files:**
- Delete: `internal/infra/store/local_session_store.go`

- [ ] **Step 1: Remove the file**

```bash
rm /Users/tengyongzhi/work/bot-workspaces/openbee2/internal/infra/store/local_session_store.go
```

- [ ] **Step 2: Verify no remaining references**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee2
go build ./...
```

Expected: no errors. If `LocalSession` or `LocalSessionStore` is referenced anywhere, fix those references before proceeding.

- [ ] **Step 3: Commit**

```bash
git add -u internal/infra/store/local_session_store.go
git commit -m "chore(store): delete deprecated LocalSessionStore"
```

---

## Task 5: Update Frontend API Client

**Files:**
- Modify: `web/src/lib/api.ts:107-138`

- [ ] **Step 1: Replace the localChat API object**

In `web/src/lib/api.ts`, replace the entire `localChat` block (lines 107-138) with:

```typescript
  localChat: {
    sendMessage: (content: string, mediaPaths?: string[]) =>
      fetchAPI("/local/messages", {
        method: "POST",
        body: JSON.stringify({ content, media_paths: mediaPaths }),
      }),
    getMessages: async () => {
      const msgs = await fetchAPI<ChatMessage[] | null>("/local/messages")
      return Array.isArray(msgs) ? msgs : []
    },
    uploadMedia: async (file: File) => {
      const form = new FormData()
      form.append("file", file)
      const res = await fetchWithAuth(`${API_BASE}/local/media`, {
        method: "POST",
        body: form,
      })
      if (!res.ok) throw new Error(await res.text())
      return res.json() as Promise<{ path: string }>
    },
  },
```

- [ ] **Step 2: Remove LocalChatSession from imports (if it becomes unused)**

Check if `LocalChatSession` is used anywhere else in the codebase:

```bash
grep -r "LocalChatSession" /Users/tengyongzhi/work/bot-workspaces/openbee2/web/src/
```

If it only appears in `types.ts` and is not used anywhere, remove it from `web/src/lib/types.ts`. If still used, leave it.

- [ ] **Step 3: Commit**

```bash
git add web/src/lib/api.ts web/src/lib/types.ts
git commit -m "feat(web): simplify local chat API client — remove session endpoints"
```

---

## Task 6: Simplify Frontend Hooks

**Files:**
- Modify: `web/src/hooks/use-local-chat.ts` (full rewrite)

- [ ] **Step 1: Replace the hooks file**

Replace the entire contents of `web/src/hooks/use-local-chat.ts` with:

```typescript
import { useQuery, useMutation } from "@tanstack/react-query"
import { useEffect, useRef } from "react"
import { api } from "@/lib/api"
import { config } from "@/lib/config"
import { tokenParam } from "@/lib/auth"
import type { ChatMessage } from "@/lib/types"

export function useLocalMessages() {
  return useQuery({
    queryKey: ["local-messages"],
    queryFn: () => api.localChat.getMessages(),
  })
}

export function useSendMessage() {
  return useMutation({
    mutationFn: ({ content, mediaPaths }: { content: string; mediaPaths?: string[] }) =>
      api.localChat.sendMessage(content, mediaPaths),
  })
}

// useLocalChatStream subscribes to SSE for the default local session.
// Calls onReply on each new reply event.
export function useLocalChatStream(onReply: (msg: ChatMessage) => void) {
  const onReplyRef = useRef(onReply)
  onReplyRef.current = onReply

  useEffect(() => {
    let es: EventSource
    let reconnectTimer: ReturnType<typeof setTimeout>

    const connect = () => {
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
        reconnectTimer = setTimeout(connect, 2000)
      }
    }

    connect()

    return () => {
      clearTimeout(reconnectTimer)
      es?.close()
    }
  }, [])
}
```

- [ ] **Step 2: Commit**

```bash
git add web/src/hooks/use-local-chat.ts
git commit -m "feat(web): simplify local chat hooks — remove session management"
```

---

## Task 7: Rewrite local-chat.tsx as the Single Chat Page

**Files:**
- Modify: `web/src/pages/local-chat.tsx` (full rewrite)

The new page is a port of `local-chat-detail.tsx`, with session-specific references removed.

- [ ] **Step 1: Rewrite the file**

Replace the entire contents of `web/src/pages/local-chat.tsx` with:

```tsx
import {
  memo,
  useCallback,
  useEffect,
  useRef,
  useState,
} from "react"
import { Streamdown } from "streamdown"
import { useTranslation } from "react-i18next"
import {
  ArrowUpRight,
  MessageSquareText,
  Paperclip,
  Send,
  X,
} from "lucide-react"
import {
  useLocalMessages,
  useLocalChatStream,
  useSendMessage,
} from "@/hooks/use-local-chat"
import { DetailSection } from "@/components/detail-primitives"
import { FadeIn } from "@/components/fade-in"
import { PageHeader } from "@/components/page-header"
import { Button } from "@/components/ui/button"
import { api } from "@/lib/api"
import { config } from "@/lib/config"
import { tokenParam } from "@/lib/auth"
import type { ChatMessage } from "@/lib/types"
import { basename, cn, isImage } from "@/lib/utils"
import { isSameDay } from "@/lib/format"

function formatMessageTimestamp(timestamp: number | null | undefined, language: string) {
  if (!timestamp) return "—"
  return new Intl.DateTimeFormat(language, isSameDay(timestamp, Date.now())
    ? {
        hour: "numeric",
        minute: "2-digit",
      }
    : {
        month: "short",
        day: "numeric",
        hour: "numeric",
        minute: "2-digit",
      }).format(new Date(timestamp))
}

function mediaUrl(mediaPath: string) {
  const filename = basename(mediaPath)
  return `${config.apiUrl}/local/media/${encodeURIComponent(filename)}${tokenParam()}`
}

const AttachmentPreview = memo(function AttachmentPreview({
  mediaPath,
  tone,
}: {
  mediaPath: string
  tone: "user" | "bee"
}) {
  const filename = basename(mediaPath)
  const url = mediaUrl(mediaPath)
  const frameClass = tone === "user"
    ? "border-primary-foreground/15 bg-primary-foreground/10"
    : "border-border/70 bg-background/70"

  if (isImage(mediaPath)) {
    return (
      <a
        href={url}
        target="_blank"
        rel="noreferrer"
        className={cn("block overflow-hidden rounded-2xl border", frameClass)}
      >
        <img
          src={url}
          alt={filename}
          className="max-h-80 w-full object-contain"
        />
      </a>
    )
  }

  return (
    <a
      href={url}
      target="_blank"
      rel="noreferrer"
      className={cn(
        "inline-flex items-center gap-2 rounded-2xl border px-3 py-2 text-sm transition-colors",
        frameClass,
        tone === "user"
          ? "text-primary-foreground/90 hover:bg-primary-foreground/10"
          : "text-foreground hover:bg-muted/40"
      )}
    >
      <Paperclip className="size-4 shrink-0" />
      <span className="break-all">{filename}</span>
      <ArrowUpRight className="size-4 shrink-0 opacity-70" />
    </a>
  )
})

export function LocalChat() {
  const { t, i18n } = useTranslation()

  const { data: history = [], isLoading } = useLocalMessages()
  const sendMessage = useSendMessage()

  const [localMessages, setLocalMessages] = useState<ChatMessage[]>([])
  const [input, setInput] = useState("")
  const [isProcessing, setIsProcessing] = useState(false)
  const [pendingMediaPaths, setPendingMediaPaths] = useState<string[]>([])
  const [uploadError, setUploadError] = useState<string | null>(null)
  const fileInputRef = useRef<HTMLInputElement>(null)
  const textareaRef = useRef<HTMLTextAreaElement>(null)
  const bottomRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    setLocalMessages(history)
  }, [history])

  useEffect(() => {
    const textarea = textareaRef.current
    if (!textarea) return
    textarea.style.height = "0px"
    textarea.style.height = `${Math.min(textarea.scrollHeight, 220)}px`
  }, [input])

  const handleReply = useCallback((message: ChatMessage) => {
    setLocalMessages((prev) => [...prev, message])
    setIsProcessing(false)
  }, [])
  useLocalChatStream(handleReply)

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: "smooth" })
  }, [localMessages, isProcessing])

  const handleSend = useCallback(async () => {
    const content = input.trim()
    if (!content && pendingMediaPaths.length === 0) return

    const paths = [...pendingMediaPaths]
    const userMessage: ChatMessage = {
      role: "user",
      content,
      media_paths: paths.length > 0 ? paths : undefined,
      ts: Date.now(),
    }

    setLocalMessages((prev) => [...prev, userMessage])
    setInput("")
    setPendingMediaPaths([])
    setUploadError(null)
    setIsProcessing(true)

    try {
      await sendMessage.mutateAsync({
        content: content || " ",
        mediaPaths: paths.length > 0 ? paths : undefined,
      })
    } catch {
      setLocalMessages((prev) => prev.filter((message) => message !== userMessage))
      setPendingMediaPaths((prev) => [...paths, ...prev])
      setIsProcessing(false)
    }
  }, [input, pendingMediaPaths, sendMessage])

  const handleFileChange = useCallback(async (event: React.ChangeEvent<HTMLInputElement>) => {
    const files = Array.from(event.target.files ?? [])
    if (files.length === 0) return

    event.target.value = ""
    setUploadError(null)

    const results = await Promise.allSettled(
      files.map((file) => api.localChat.uploadMedia(file))
    )
    const succeeded = results.filter(
      (result): result is PromiseFulfilledResult<{ path: string }> => result.status === "fulfilled"
    )
    const failedCount = results.length - succeeded.length

    if (failedCount > 0) {
      setUploadError(t("localChat.uploadError", { count: failedCount }))
    }

    setPendingMediaPaths((prev) => [...prev, ...succeeded.map((result) => result.value.path)])
  }, [t])

  const messageCount = localMessages.length
  const canSend = input.trim().length > 0 || pendingMediaPaths.length > 0
  const isEmpty = !isLoading && messageCount === 0

  return (
    <FadeIn>
      <div className="space-y-6">
        <PageHeader
          title={t("localChat.title")}
          subtitle={t("localChat.detailDescription")}
          actions={
            <div className="flex flex-wrap items-center justify-end gap-2">
              <span className="inline-flex items-center rounded-full border border-border bg-background px-3 py-1.5 text-xs text-muted-foreground">
                {isProcessing ? t("localChat.processing") : t("localChat.idleStatus")}
              </span>
              <span className="inline-flex items-center rounded-full border border-border bg-background px-3 py-1.5 text-xs text-muted-foreground">
                {t("localChat.queuedUploadsLabel")}: {pendingMediaPaths.length}
              </span>
            </div>
          }
        />

        <DetailSection className="flex min-h-[34rem] flex-col xl:h-[calc(100vh-12rem)]">
          <div className="border-b border-border/70 px-5 py-4 sm:px-6">
            <div className="flex flex-col gap-3 lg:flex-row lg:items-end lg:justify-between">
              <div>
                <p className="text-xs font-medium uppercase tracking-[0.18em] text-muted-foreground">
                  {t("localChat.timelineLabel")}
                </p>
                <p className="mt-2 max-w-2xl text-sm leading-6 text-muted-foreground">
                  {t("localChat.timelineHint")}
                </p>
              </div>

              <div className="flex flex-wrap items-center gap-2 text-xs">
                <span className="inline-flex items-center rounded-full border border-border/70 bg-background/80 px-3 py-1 text-muted-foreground">
                  {isProcessing ? t("localChat.processing") : t("localChat.idleStatus")}
                </span>
                <span className="inline-flex items-center rounded-full border border-border/70 bg-background/80 px-3 py-1 text-muted-foreground">
                  {t("localChat.queuedUploadsLabel")}: {pendingMediaPaths.length}
                </span>
              </div>
            </div>
          </div>

          <div className="flex-1 overflow-y-auto px-4 py-5 sm:px-6 sm:py-6">
            {isLoading ? (
              <div className="space-y-4">
                {Array.from({ length: 3 }).map((_, index) => (
                  <div
                    key={index}
                    className="rounded-[1.6rem] border border-border/70 bg-background/80 px-4 py-4"
                  >
                    <div className="h-4 w-28 rounded bg-muted" />
                    <div className="mt-4 h-4 w-full rounded bg-muted" />
                    <div className="mt-2 h-4 w-4/5 rounded bg-muted" />
                  </div>
                ))}
              </div>
            ) : isEmpty ? (
              <div className="flex h-full min-h-[18rem] items-center justify-center">
                <div className="max-w-md rounded-[1.75rem] border border-dashed border-border/80 bg-background/78 px-6 py-8 text-left">
                  <div className="inline-flex size-12 items-center justify-center rounded-2xl border border-border/70 bg-muted/35">
                    <MessageSquareText className="size-5 text-muted-foreground" />
                  </div>
                  <h2 className="mt-5 text-lg font-semibold tracking-tight text-foreground">
                    {t("localChat.noMessagesTitle")}
                  </h2>
                  <p className="mt-2 text-sm leading-6 text-muted-foreground">
                    {t("localChat.noMessagesDescription")}
                  </p>
                </div>
              </div>
            ) : (
              <div className="space-y-4">
                {localMessages.map((message, index) => {
                  const isUser = message.role === "user"
                  const hasContent = message.content.trim().length > 0

                  return (
                    <div
                      key={`${message.role}-${message.ts}-${index}`}
                      className={cn("flex", isUser ? "justify-end" : "justify-start")}
                    >
                      <article
                        className={cn(
                          "w-full rounded-[1.6rem] border px-4 py-4 sm:px-5",
                          isUser
                            ? "max-w-[min(100%,44rem)] border-primary/15 bg-primary text-primary-foreground"
                            : "max-w-[min(100%,52rem)] border-border/70 bg-background/82"
                        )}
                      >
                        <div className="flex flex-wrap items-center justify-between gap-3">
                          <div className="flex items-center gap-2 text-[11px] font-medium uppercase tracking-[0.18em]">
                            <span className={cn(
                              "size-2 rounded-full",
                              isUser ? "bg-primary-foreground/70" : "bg-primary/70"
                            )}
                            />
                            <span className={isUser ? "text-primary-foreground/78" : "text-muted-foreground"}>
                              {isUser ? t("localChat.operatorLabel") : t("localChat.beeLabel")}
                            </span>
                          </div>

                          <time className={cn(
                            "text-xs",
                            isUser ? "text-primary-foreground/72" : "text-muted-foreground"
                          )}
                          >
                            {formatMessageTimestamp(message.ts, i18n.language)}
                          </time>
                        </div>

                        {message.media_paths && message.media_paths.length > 0 && (
                          <div className="mt-4 space-y-3">
                            {message.media_paths.map((path) => (
                              <AttachmentPreview
                                key={path}
                                mediaPath={path}
                                tone={message.role}
                              />
                            ))}
                          </div>
                        )}

                        {hasContent && (
                          message.role === "bee" ? (
                            <div className="prose prose-sm mt-4 max-w-none dark:prose-invert prose-p:my-3 prose-pre:rounded-2xl prose-pre:border prose-pre:border-border/70 prose-pre:bg-muted/35 prose-pre:px-4 prose-pre:py-3">
                              <Streamdown mode="static">{message.content}</Streamdown>
                            </div>
                          ) : (
                            <p className="mt-4 whitespace-pre-wrap text-sm leading-7">
                              {message.content}
                            </p>
                          )
                        )}
                      </article>
                    </div>
                  )
                })}

                {isProcessing && (
                  <div className="flex justify-start">
                    <div className="w-full max-w-[38rem] rounded-[1.6rem] border border-border/70 bg-background/82 px-4 py-4 sm:px-5">
                      <div className="flex items-center justify-between gap-3">
                        <div className="flex items-center gap-2 text-[11px] font-medium uppercase tracking-[0.18em] text-muted-foreground">
                          <span className="size-2 rounded-full bg-primary/70" />
                          <span>{t("localChat.beeLabel")}</span>
                        </div>
                        <span className="text-xs text-muted-foreground">
                          {t("localChat.processing")}
                        </span>
                      </div>
                      <div className="mt-4 flex gap-1.5">
                        <span className="h-2 w-2 rounded-full bg-primary animate-pulse-amber" style={{ animationDelay: "0ms" }} />
                        <span className="h-2 w-2 rounded-full bg-primary animate-pulse-amber" style={{ animationDelay: "300ms" }} />
                        <span className="h-2 w-2 rounded-full bg-primary animate-pulse-amber" style={{ animationDelay: "600ms" }} />
                      </div>
                    </div>
                  </div>
                )}

                <div ref={bottomRef} />
              </div>
            )}
          </div>

          <div className="border-t border-border/70 bg-card p-4 sm:p-5">
            {uploadError && (
              <div className="mb-4 rounded-2xl border border-destructive/20 bg-destructive/10 px-4 py-3 text-sm text-destructive">
                {uploadError}
              </div>
            )}

            {pendingMediaPaths.length > 0 && (
              <div className="mb-4 flex flex-wrap gap-2">
                {pendingMediaPaths.map((path) => (
                  <span
                    key={path}
                    className="inline-flex items-center gap-2 rounded-full border border-border/70 bg-background/80 px-3 py-1.5 text-xs text-foreground"
                  >
                    <Paperclip className="size-3.5 text-muted-foreground" />
                    <span className="max-w-52 truncate">{basename(path)}</span>
                    <button
                      type="button"
                      className="inline-flex size-5 items-center justify-center rounded-full text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
                      aria-label={`${t("localChat.removeAttachment")}: ${basename(path)}`}
                      onClick={() =>
                        setPendingMediaPaths((prev) => prev.filter((entry) => entry !== path))
                      }
                    >
                      <X className="size-3.5" />
                    </button>
                  </span>
                ))}
              </div>
            )}

            <div className="rounded-[1.6rem] border border-border/70 bg-background/82 p-3">
              <textarea
                ref={textareaRef}
                className="max-h-[220px] min-h-[120px] w-full resize-none bg-transparent px-3 py-2 text-sm leading-7 placeholder:text-muted-foreground focus:outline-none"
                placeholder={t("localChat.inputPlaceholder")}
                value={input}
                onChange={(event) => setInput(event.target.value)}
                onKeyDown={(event) => {
                  if (event.key === "Enter" && !event.shiftKey) {
                    event.preventDefault()
                    void handleSend()
                  }
                }}
              />

              <div className="mt-3 flex flex-col gap-3 border-t border-border/70 pt-3 sm:flex-row sm:items-center sm:justify-between">
                <p className="text-xs leading-5 text-muted-foreground">
                  {t("localChat.composerHint")}
                </p>

                <div className="flex flex-wrap items-center gap-2">
                  <input
                    ref={fileInputRef}
                    type="file"
                    multiple
                    className="hidden"
                    onChange={handleFileChange}
                  />
                  <Button
                    variant="outline"
                    className="h-10 rounded-xl"
                    onClick={() => fileInputRef.current?.click()}
                    aria-label={t("localChat.uploadFile")}
                  >
                    <Paperclip className="size-4" />
                    <span className="hidden sm:inline">{t("localChat.uploadFile")}</span>
                  </Button>
                  <Button
                    className="h-10 rounded-xl"
                    onClick={() => void handleSend()}
                    disabled={!canSend || sendMessage.isPending}
                    aria-label={t("localChat.send")}
                  >
                    <Send className="size-4" />
                    <span>{t("localChat.send")}</span>
                  </Button>
                </div>
              </div>
            </div>
          </div>
        </DetailSection>
      </div>
    </FadeIn>
  )
}
```

- [ ] **Step 2: Check TypeScript compiles**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee2/web
npx tsc --noEmit
```

Expected: no errors. Fix any type errors before proceeding.

- [ ] **Step 3: Commit**

```bash
git add web/src/pages/local-chat.tsx
git commit -m "feat(web): rewrite local-chat page as single chat UI (no session management)"
```

---

## Task 8: Update Router and Delete Detail Page

**Files:**
- Modify: `web/src/app.tsx:42-43`
- Delete: `web/src/pages/local-chat-detail.tsx`

- [ ] **Step 1: Update app.tsx routes**

In `web/src/app.tsx`, find the local chat routes (around lines 42-43):

```tsx
<Route path="/local-chat" element={<LocalChat />} />
<Route path="/local-chat/:id" element={<LocalChatDetail />} />
```

Replace with just:

```tsx
<Route path="/local-chat" element={<LocalChat />} />
```

Also remove the `LocalChatDetail` import at the top of the file.

- [ ] **Step 2: Delete the detail page**

```bash
rm /Users/tengyongzhi/work/bot-workspaces/openbee2/web/src/pages/local-chat-detail.tsx
```

- [ ] **Step 3: Verify TypeScript compiles**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee2/web
npx tsc --noEmit
```

Expected: no errors. If `LocalChatDetail` is still imported anywhere, remove those imports.

- [ ] **Step 4: Verify the frontend builds**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee2/web
npm run build
```

Expected: build completes with no errors.

- [ ] **Step 5: Commit**

```bash
git add web/src/app.tsx
git add -u web/src/pages/local-chat-detail.tsx
git commit -m "feat(web): remove local chat session list route and detail page"
```

---

## Task 9: End-to-End Verification

- [ ] **Step 1: Start the backend**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee2
go run ./cmd/openbee/...
```

Expected: server starts, migration v27 applied in logs.

- [ ] **Step 2: Verify new endpoints respond**

```bash
# Should return empty array on fresh install
curl -s -H "Authorization: Bearer <token>" http://localhost:8080/api/local/messages | jq .

# Should return { "status": "queued" }
curl -s -X POST -H "Authorization: Bearer <token>" -H "Content-Type: application/json" \
  -d '{"content":"hello"}' http://localhost:8080/api/local/messages | jq .
```

Expected: first returns `[]`, second returns `{"status":"queued"}`.

- [ ] **Step 3: Verify old endpoints are gone**

```bash
curl -s -o /dev/null -w "%{http_code}" \
  -H "Authorization: Bearer <token>" http://localhost:8080/api/local/sessions
```

Expected: `404`.

- [ ] **Step 4: Open the frontend and chat**

Navigate to `/local-chat`. The chat interface should load immediately with no session creation step. Send a test message and verify the AI replies via SSE.

- [ ] **Step 5: Final commit (if any fixes were needed)**

```bash
git add -A
git commit -m "fix: address issues found during end-to-end verification"
```
