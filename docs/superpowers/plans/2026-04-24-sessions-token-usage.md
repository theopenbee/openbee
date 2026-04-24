# Sessions Page Token Usage Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a "Tokens" column to the `/sessions` list page that shows total token count per session, with a per-model breakdown tooltip on hover.

**Architecture:** Extend `GET /executions` to batch-fetch `bee_token_stats` for all sessions on the current page and embed them as a top-level `token_stats` map in the response. The frontend reads this map by `session_id` when rendering each row and displays a compact total with a shadcn Tooltip showing per-model input/output/cache breakdown.

**Tech Stack:** Go (gin, database/sql, SQLite), React + TypeScript, shadcn/ui Tooltip (via base-ui), lucide-react, react-i18next

---

## File Map

| File | Change |
|------|--------|
| `internal/infra/store/token_stats_store.go` | Add `GetBySessionIDs` |
| `internal/infra/store/token_stats_store_test.go` | Tests for `GetBySessionIDs` |
| `internal/api/execution_handler.go` | Add `tokenStats` field + `buildTokenStatsMap` + modify `List` |
| `internal/api/execution_handler_test.go` | New: handler integration tests |
| `internal/app/app.go` | Pass `tokenStatsStore` to `NewExecutionHandler` |
| `web/src/lib/types.ts` | Add `ModelTokenStats`, `SessionTokenStats`; extend `PaginatedResponse` |
| `web/src/lib/format.ts` | Add `formatTokenCount` |
| `web/src/locales/en.json` | Add `sessions.columns.tokens` |
| `web/src/locales/zh.json` | Add `sessions.columns.tokens` |
| `web/src/pages/sessions.tsx` | Add Tokens column + `TokenStatsTooltip` component |

---

## Task 1: Add `GetBySessionIDs` to `TokenStatsStore`

**Files:**
- Modify: `internal/infra/store/token_stats_store.go`
- Modify: `internal/infra/store/token_stats_store_test.go`

- [ ] **Step 1: Write failing tests**

Add to `internal/infra/store/token_stats_store_test.go`:

```go
func TestTokenStatsStore_GetBySessionIDs_ReturnsMatchingRows(t *testing.T) {
	s, cleanup := newTokenStatsTestDB(t)
	defer cleanup()

	now := time.Now().UnixMilli()
	for _, stat := range []model.TokenStats{
		{SessionID: "session-1", AgentType: "claude", Model: "claude-sonnet-4-6", InputTokens: 100, OutputTokens: 200, TotalTokens: 300, SyncedAt: now},
		{SessionID: "session-1", AgentType: "claude", Model: "claude-opus-4-7", InputTokens: 50, OutputTokens: 100, TotalTokens: 150, SyncedAt: now},
		{SessionID: "session-2", AgentType: "claude", Model: "claude-sonnet-4-6", InputTokens: 10, OutputTokens: 20, TotalTokens: 30, SyncedAt: now},
	} {
		if err := s.Upsert(stat); err != nil {
			t.Fatalf("Upsert: %v", err)
		}
	}

	rows, err := s.GetBySessionIDs([]string{"session-1", "session-99"})
	if err != nil {
		t.Fatalf("GetBySessionIDs: %v", err)
	}
	if len(rows) != 2 {
		t.Errorf("expected 2 rows for session-1, got %d", len(rows))
	}
	for _, r := range rows {
		if r.SessionID != "session-1" {
			t.Errorf("unexpected session_id %q", r.SessionID)
		}
	}
}

func TestTokenStatsStore_GetBySessionIDs_NilSlice(t *testing.T) {
	s, cleanup := newTokenStatsTestDB(t)
	defer cleanup()

	rows, err := s.GetBySessionIDs(nil)
	if err != nil {
		t.Fatalf("GetBySessionIDs: %v", err)
	}
	if rows != nil {
		t.Errorf("expected nil result for empty input, got %v", rows)
	}
}
```

- [ ] **Step 2: Run tests to confirm they fail**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee
go test ./internal/infra/store/... -run TestTokenStatsStore_GetBySessionIDs -v
```

Expected: FAIL — `s.GetBySessionIDs undefined`

- [ ] **Step 3: Implement `GetBySessionIDs`**

Add to `internal/infra/store/token_stats_store.go` (add `"strings"` to imports):

```go
func (s *TokenStatsStore) GetBySessionIDs(sessionIDs []string) ([]model.TokenStats, error) {
	if len(sessionIDs) == 0 {
		return nil, nil
	}
	placeholders := strings.Repeat("?,", len(sessionIDs))
	placeholders = placeholders[:len(placeholders)-1]
	query := fmt.Sprintf(
		`SELECT id, session_id, agent_type, model, input_tokens, output_tokens,
		        cache_creation_tokens, cache_read_tokens, total_tokens, synced_at
		 FROM bee_token_stats WHERE session_id IN (%s)`,
		placeholders,
	)
	args := make([]any, len(sessionIDs))
	for i, id := range sessionIDs {
		args[i] = id
	}
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("get token stats by session ids: %w", err)
	}
	defer rows.Close()
	var stats []model.TokenStats
	for rows.Next() {
		var st model.TokenStats
		if err := rows.Scan(
			&st.ID, &st.SessionID, &st.AgentType, &st.Model,
			&st.InputTokens, &st.OutputTokens,
			&st.CacheCreationTokens, &st.CacheReadTokens,
			&st.TotalTokens, &st.SyncedAt,
		); err != nil {
			return nil, fmt.Errorf("scan token stats: %w", err)
		}
		stats = append(stats, st)
	}
	return stats, rows.Err()
}
```

- [ ] **Step 4: Run tests to confirm they pass**

```bash
go test ./internal/infra/store/... -run TestTokenStatsStore_GetBySessionIDs -v
```

Expected: PASS both tests

- [ ] **Step 5: Commit**

```bash
git add internal/infra/store/token_stats_store.go internal/infra/store/token_stats_store_test.go
git commit -m "feat(tokenstat): add GetBySessionIDs for batch session lookup"
```

---

## Task 2: Extend `ExecutionHandler.List` with token stats

**Files:**
- Modify: `internal/api/execution_handler.go`
- Create: `internal/api/execution_handler_test.go`

- [ ] **Step 1: Write failing handler tests**

Create `internal/api/execution_handler_test.go`:

```go
package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/infra/store"
)

func newTestServerWithExecutions(t *testing.T) (*gin.Engine, *store.ExecutionStore, *store.TokenStatsStore, func()) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := store.InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	es := store.NewExecutionStore(db, t.TempDir())
	ts := store.NewTokenStatsStore(db)
	h := NewExecutionHandler(es, ts)
	router := gin.New()
	api := router.Group("/api")
	api.GET("/executions", h.List)
	return router, es, ts, func() { db.Close() }
}

func TestExecutionsList_IncludesTokenStats(t *testing.T) {
	router, es, ts, cleanup := newTestServerWithExecutions(t)
	defer cleanup()

	if _, err := es.Create("worker-1", "hello", "session-abc", "claude"); err != nil {
		t.Fatalf("Create execution: %v", err)
	}
	if err := ts.Upsert(model.TokenStats{
		SessionID:    "session-abc",
		AgentType:    "claude",
		Model:        "claude-sonnet-4-6",
		InputTokens:  100,
		OutputTokens: 200,
		TotalTokens:  300,
		SyncedAt:     time.Now().UnixMilli(),
	}); err != nil {
		t.Fatalf("Upsert token stats: %v", err)
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/executions", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	statsRaw, ok := resp["token_stats"]
	if !ok {
		t.Fatal("expected token_stats field in response")
	}
	statsMap, ok := statsRaw.(map[string]any)
	if !ok {
		t.Fatalf("token_stats must be a map, got %T", statsRaw)
	}
	if _, found := statsMap["session-abc"]; !found {
		t.Errorf("expected session-abc in token_stats, got keys: %v", statsMap)
	}
}

func TestExecutionsList_NoTokenStats_WhenNoneExist(t *testing.T) {
	router, es, _, cleanup := newTestServerWithExecutions(t)
	defer cleanup()

	if _, err := es.Create("worker-1", "hello", "session-xyz", "claude"); err != nil {
		t.Fatalf("Create execution: %v", err)
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/executions", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	if ts, ok := resp["token_stats"].(map[string]any); ok {
		if _, found := ts["session-xyz"]; found {
			t.Error("session-xyz must not appear in token_stats when no stats were upserted")
		}
	}
}
```

- [ ] **Step 2: Run to confirm they fail**

```bash
go test ./internal/api/... -run TestExecutionsList -v
```

Expected: FAIL — `NewExecutionHandler` has wrong signature (tests pass 2 args, handler takes 1)

- [ ] **Step 3: Update `ExecutionHandler` with token stats support**

Replace the entire `internal/api/execution_handler.go` with:

```go
package api

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/infra/store"
)

type modelTokenStats struct {
	Model               string `json:"model"`
	TotalTokens         int64  `json:"total_tokens"`
	InputTokens         int64  `json:"input_tokens"`
	OutputTokens        int64  `json:"output_tokens"`
	CacheCreationTokens int64  `json:"cache_creation_tokens"`
	CacheReadTokens     int64  `json:"cache_read_tokens"`
}

type sessionTokenStats struct {
	TotalTokens int64             `json:"total_tokens"`
	ByModel     []modelTokenStats `json:"by_model"`
}

type ExecutionHandler struct {
	executions *store.ExecutionStore
	tokenStats *store.TokenStatsStore
}

func NewExecutionHandler(es *store.ExecutionStore, ts *store.TokenStatsStore) *ExecutionHandler {
	return &ExecutionHandler{executions: es, tokenStats: ts}
}

func (h *ExecutionHandler) ListByWorker(c *gin.Context) {
	workerID := c.Param("id")
	page, pageSize, offset := parsePagination(c)

	total, err := h.executions.CountSessionsByWorkerID(workerID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	execs, err := h.executions.ListPaginatedByWorkerID(workerID, pageSize, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, paginatedResponse(execs, total, page, pageSize))
}

func (h *ExecutionHandler) List(c *gin.Context) {
	page, pageSize, offset := parsePagination(c)

	f := store.ExecutionFilter{
		WorkerID:      c.Query("worker_id"),
		SessionID:     c.Query("session_id"),
		Status:        c.Query("status"),
		StartedFrom:   parseInt64Query(c, "started_at_from"),
		StartedTo:     parseInt64Query(c, "started_at_to"),
		CompletedFrom: parseInt64Query(c, "completed_at_from"),
		CompletedTo:   parseInt64Query(c, "completed_at_to"),
	}

	if f == (store.ExecutionFilter{}) {
		total, err := h.executions.CountSessions()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		execs, err := h.executions.ListPaginated(pageSize, offset)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"items":       execs,
			"total":       total,
			"page":        page,
			"page_size":   pageSize,
			"token_stats": h.buildTokenStatsMap(execs),
		})
		return
	}

	execs, total, err := h.executions.ListFiltered(c.Request.Context(), f, pageSize, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, paginatedResponse(execs, total, page, pageSize))
}

func (h *ExecutionHandler) Get(c *gin.Context) {
	exec, err := h.executions.GetByID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "execution not found"})
		return
	}
	c.JSON(http.StatusOK, exec)
}

func (h *ExecutionHandler) ListBySession(c *gin.Context) {
	sessionID := c.Query("session_id")
	execs, err := h.executions.ListBySessionID(sessionID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, execs)
}

func (h *ExecutionHandler) GetLogs(c *gin.Context) {
	id := c.Param("id")

	var since int64
	if raw := c.Query("since"); raw != "" {
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || n < 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid since parameter"})
			return
		}
		since = n
	}

	slice, err := h.executions.ReadLogSince(id, since)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if slice.Status == model.ExecStatusCompleted || slice.Status == model.ExecStatusFailed {
		c.Header("Cache-Control", "public, max-age=3600")
	}
	c.JSON(http.StatusOK, gin.H{
		"content":   slice.Content,
		"size":      slice.Size,
		"truncated": slice.Truncated,
	})
}

func (h *ExecutionHandler) buildTokenStatsMap(execs []model.WorkerExecution) map[string]*sessionTokenStats {
	seen := make(map[string]struct{})
	var sessionIDs []string
	for _, e := range execs {
		if _, ok := seen[e.SessionID]; !ok {
			seen[e.SessionID] = struct{}{}
			sessionIDs = append(sessionIDs, e.SessionID)
		}
	}
	if len(sessionIDs) == 0 {
		return nil
	}
	rows, err := h.tokenStats.GetBySessionIDs(sessionIDs)
	if err != nil {
		return nil
	}
	result := make(map[string]*sessionTokenStats)
	for _, row := range rows {
		entry := result[row.SessionID]
		if entry == nil {
			entry = &sessionTokenStats{}
			result[row.SessionID] = entry
		}
		entry.TotalTokens += row.TotalTokens
		entry.ByModel = append(entry.ByModel, modelTokenStats{
			Model:               row.Model,
			TotalTokens:         row.TotalTokens,
			InputTokens:         row.InputTokens,
			OutputTokens:        row.OutputTokens,
			CacheCreationTokens: row.CacheCreationTokens,
			CacheReadTokens:     row.CacheReadTokens,
		})
	}
	return result
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/api/... -run TestExecutionsList -v
go test ./internal/... -v 2>&1 | tail -20
```

Expected: all pass, no compile errors

- [ ] **Step 5: Commit**

```bash
git add internal/api/execution_handler.go internal/api/execution_handler_test.go
git commit -m "feat(tokenstat): include token stats in executions list response"
```

---

## Task 3: Wire `TokenStatsStore` into `ExecutionHandler`

**Files:**
- Modify: `internal/app/app.go` (line ~335)

- [ ] **Step 1: Update `NewExecutionHandler` call**

In `internal/app/app.go`, find the line:
```go
Executions:        api.NewExecutionHandler(s.execStore),
```

Replace it with:
```go
Executions:        api.NewExecutionHandler(s.execStore, s.tokenStatsStore),
```

- [ ] **Step 2: Build to confirm it compiles**

```bash
go build ./...
```

Expected: successful build, no errors

- [ ] **Step 3: Commit**

```bash
git add internal/app/app.go
git commit -m "chore: wire tokenStatsStore into ExecutionHandler"
```

---

## Task 4: Add TypeScript types for token stats

**Files:**
- Modify: `web/src/lib/types.ts`

- [ ] **Step 1: Add the new interfaces and extend `PaginatedResponse`**

In `web/src/lib/types.ts`, add the following two new interfaces after the `WorkerExecution` interface (after line 48):

```typescript
export interface ModelTokenStats {
  model: string
  total_tokens: number
  input_tokens: number
  output_tokens: number
  cache_creation_tokens: number
  cache_read_tokens: number
}

export interface SessionTokenStats {
  total_tokens: number
  by_model: ModelTokenStats[]
}
```

Then replace the existing `PaginatedResponse<T>` interface:

```typescript
export interface PaginatedResponse<T> {
  items: T[]
  total: number
  page: number
  page_size: number
  token_stats?: Record<string, SessionTokenStats | null>
}
```

- [ ] **Step 2: Type-check**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee/web
npm run type-check 2>&1 | tail -20
```

If `npm run type-check` doesn't exist, use:
```bash
npx tsc --noEmit 2>&1 | tail -20
```

Expected: no errors

- [ ] **Step 3: Commit**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee
git add web/src/lib/types.ts
git commit -m "feat(tokenstat): add ModelTokenStats and SessionTokenStats TS types"
```

---

## Task 5: Add `formatTokenCount` helper

**Files:**
- Modify: `web/src/lib/format.ts`

- [ ] **Step 1: Add the function**

Append to `web/src/lib/format.ts`:

```typescript
export function formatTokenCount(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}K`
  return String(n)
}
```

- [ ] **Step 2: Type-check**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee/web
npx tsc --noEmit 2>&1 | tail -10
```

Expected: no errors

- [ ] **Step 3: Commit**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee
git add web/src/lib/format.ts
git commit -m "feat(tokenstat): add formatTokenCount for compact display"
```

---

## Task 6: Add i18n keys for Tokens column

**Files:**
- Modify: `web/src/locales/en.json`
- Modify: `web/src/locales/zh.json`

- [ ] **Step 1: Add key to `en.json`**

In `web/src/locales/en.json`, find:
```json
      "duration": "Duration"
```
Replace with:
```json
      "duration": "Duration",
      "tokens": "Tokens"
```

- [ ] **Step 2: Add key to `zh.json`**

In `web/src/locales/zh.json`, find:
```json
      "duration": "耗时"
```
Replace with:
```json
      "duration": "耗时",
      "tokens": "Token 用量"
```

- [ ] **Step 3: Commit**

```bash
git add web/src/locales/en.json web/src/locales/zh.json
git commit -m "i18n: add sessions.columns.tokens translation key"
```

---

## Task 7: Add Tokens column to sessions page

**Files:**
- Modify: `web/src/pages/sessions.tsx`

- [ ] **Step 1: Update imports**

Replace the existing import block at the top of `web/src/pages/sessions.tsx` with:

```typescript
import { useMemo, useState } from "react"
import { Link } from "react-router-dom"
import { useTranslation } from "react-i18next"
import { Info } from "lucide-react"
import { useExecutions } from "@/hooks/use-executions"
import type { WorkerExecution, SessionTokenStats } from "@/lib/types"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { Tooltip, TooltipTrigger, TooltipContent } from "@/components/ui/tooltip"
import { StatusBadge } from "@/components/status-badge"
import { EmptyState } from "@/components/empty-state"
import { PageHeader } from "@/components/page-header"
import { FadeIn } from "@/components/fade-in"
import { SkeletonTable } from "@/components/skeleton-loader"
import { PaginationControls } from "@/components/pagination-controls"
import { cn } from "@/lib/utils"
import { formatDuration, formatRelative, formatTokenCount, groupExecutionsBySession, isActiveStatus, STATUS_ROW_BORDER } from "@/lib/format"
```

- [ ] **Step 2: Add `TokenStatsTooltip` component**

Add the following component after the `TurnPips` function (before `export function Sessions`):

```tsx
function TokenStatsTooltip({ stats }: { stats: SessionTokenStats }) {
  const sorted = [...stats.by_model].sort((a, b) => b.total_tokens - a.total_tokens)
  return (
    <div className="flex flex-col gap-1.5 font-mono text-xs min-w-[160px]">
      <div className="flex justify-between gap-4 font-semibold">
        <span>Total</span>
        <span>{stats.total_tokens.toLocaleString()}</span>
      </div>
      <div className="border-t border-background/20 pt-1 flex flex-col gap-2">
        {sorted.map((m) => (
          <div key={m.model} className="flex flex-col gap-0.5">
            <div className="flex justify-between gap-4">
              <span className="opacity-90">{m.model}</span>
              <span>{m.total_tokens.toLocaleString()}</span>
            </div>
            <div className="flex gap-3 opacity-60 pl-1">
              <span>In {m.input_tokens.toLocaleString()}</span>
              <span>Out {m.output_tokens.toLocaleString()}</span>
            </div>
            {(m.cache_creation_tokens > 0 || m.cache_read_tokens > 0) && (
              <div className="flex gap-3 opacity-60 pl-1">
                <span>Cache↑ {m.cache_creation_tokens.toLocaleString()}</span>
                <span>Cache↓ {m.cache_read_tokens.toLocaleString()}</span>
              </div>
            )}
          </div>
        ))}
      </div>
    </div>
  )
}
```

- [ ] **Step 3: Add Tokens column header**

In the `<TableHeader>` section, find:
```tsx
                  <TableHead className="w-20">{t("sessions.columns.duration")}</TableHead>
```
Replace with:
```tsx
                  <TableHead className="w-20">{t("sessions.columns.duration")}</TableHead>
                  <TableHead className="w-24">{t("sessions.columns.tokens")}</TableHead>
```

- [ ] **Step 4: Add Tokens cell to each row**

Inside the row map function, look up token stats for the current session:

After the line:
```tsx
                  const duration = formatDuration(oldest.started_at, lastCompleted?.completed_at ?? null)
```

Add:
```tsx
                  const tokenStats = data?.token_stats?.[latest.session_id] ?? null
```

Then add the Tokens cell after the Duration cell. Find:
```tsx
                      <TableCell className="text-xs font-mono">
                        {isActive ? (
                          <span className="text-status-working animate-pulse-amber">live</span>
                        ) : (
                          <span className="text-muted-foreground">{duration}</span>
                        )}
                      </TableCell>
```
Replace with:
```tsx
                      <TableCell className="text-xs font-mono">
                        {isActive ? (
                          <span className="text-status-working animate-pulse-amber">live</span>
                        ) : (
                          <span className="text-muted-foreground">{duration}</span>
                        )}
                      </TableCell>

                      <TableCell>
                        {tokenStats ? (
                          <div className="flex items-center gap-1 text-xs font-mono">
                            <span className="text-muted-foreground">{formatTokenCount(tokenStats.total_tokens)}</span>
                            <Tooltip>
                              <TooltipTrigger asChild>
                                <button className="flex items-center text-muted-foreground/40 hover:text-muted-foreground transition-colors">
                                  <Info className="size-3" />
                                </button>
                              </TooltipTrigger>
                              <TooltipContent side="left" align="center">
                                <TokenStatsTooltip stats={tokenStats} />
                              </TooltipContent>
                            </Tooltip>
                          </div>
                        ) : (
                          <span className="text-xs text-muted-foreground/40">—</span>
                        )}
                      </TableCell>
```

- [ ] **Step 5: Type-check**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee/web
npx tsc --noEmit 2>&1 | tail -20
```

Expected: no type errors

- [ ] **Step 6: Start dev server and verify manually**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee
npm --prefix web run dev &
```

Open the browser to the sessions page. Verify:
- A "Tokens" column header appears at the far right
- Sessions with synced token data show a compact number (e.g. `12.4K`) + ℹ icon
- Sessions without data show `—`
- Hovering the ℹ icon shows the per-model breakdown tooltip
- Cache rows are hidden when both cache values are 0

- [ ] **Step 7: Commit**

```bash
git add web/src/pages/sessions.tsx
git commit -m "feat(tokenstat): add Tokens column with per-model tooltip to sessions list"
```

---

## Self-Review

**Spec coverage check:**
- ✅ Total tokens shown in Tokens column (Task 7)
- ✅ Info icon hover reveals per-model breakdown (Task 7 `TokenStatsTooltip`)
- ✅ Sessions without data show `—` (Task 7 null branch)
- ✅ Per-model breakdown sorted by total_tokens descending (Task 7)
- ✅ Cache rows hidden when both 0 (Task 7)
- ✅ Compact number format: K/M suffix (Task 5)
- ✅ Full numbers in tooltip (toLocaleString) (Task 7)
- ✅ Backend batch fetch via `GetBySessionIDs` (Tasks 1–3)
- ✅ Response shape matches TS types (Tasks 2 and 4)

**No placeholders found.**

**Type consistency:**
- `SessionTokenStats` defined in Task 4, used by name in Task 7 ✅
- `ModelTokenStats` defined in Task 4, referenced in `SessionTokenStats.by_model` ✅
- `formatTokenCount` defined in Task 5, imported in Task 7 ✅
- `GetBySessionIDs` defined in Task 1, called in Task 2 ✅
- `NewExecutionHandler(es, ts)` signature set in Task 2, call site updated in Task 3 ✅
