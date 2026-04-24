# Session Detail Token Usage Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add token usage stats (total + per-model breakdown via tooltip) to the session detail page, while refactoring `GET /sessions/executions?session_id=<id>` into `GET /sessions/:id`.

**Architecture:** New `GetSession` backend handler returns executions + token_stats in one response. Frontend extracts `TokenStatsTooltip` into a shared component, adds a `useSessionDetail` hook, and adds a 5th Overview Stat to the session detail hero grid.

**Tech Stack:** Go/Gin (backend), React/TypeScript + TanStack Query (frontend), i18next (i18n)

---

## File Map

| File | Change |
|------|--------|
| `internal/api/execution_handler.go` | Add `GetSession`, `buildSessionTokenStats`; remove `ListBySession` |
| `internal/api/execution_handler_test.go` | Add `newTestServerWithSessions`, 2 new test cases |
| `internal/routes/api.go` | Replace `GET /sessions/executions` with `GET /sessions/:id` |
| `web/src/lib/types.ts` | Add `SessionDetail` interface |
| `web/src/lib/api.ts` | Replace `sessions.executions` with `sessions.get` |
| `web/src/components/token-stats-tooltip.tsx` | New file — extract from sessions.tsx |
| `web/src/pages/sessions.tsx` | Import `TokenStatsTooltip` from shared component |
| `web/src/hooks/use-session-detail.ts` | New file — `useSessionDetail` hook |
| `web/src/hooks/use-executions.ts` | Remove `useSessionExecutions` |
| `web/src/pages/session-detail.tsx` | Use `useSessionDetail`, add Tokens stat |
| `web/src/locales/en.json` | Add `sessionDetail.tokens` |
| `web/src/locales/zh.json` | Add `sessionDetail.tokens` |

---

## Task 1: Backend — `GetSession` handler + route refactor

**Files:**
- Modify: `internal/api/execution_handler.go`
- Modify: `internal/api/execution_handler_test.go`
- Modify: `internal/routes/api.go`

- [ ] **Step 1: Write failing tests**

Add to `internal/api/execution_handler_test.go` after the existing tests:

```go
func newTestServerWithSessions(t *testing.T) (*gin.Engine, *store.ExecutionStore, *store.TokenStatsStore, func()) {
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
	api.GET("/sessions/:id", h.GetSession)
	return router, es, ts, func() { db.Close() }
}

func TestGetSession_IncludesTokenStats(t *testing.T) {
	router, es, ts, cleanup := newTestServerWithSessions(t)
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
	req := httptest.NewRequest(http.MethodGet, "/api/sessions/session-abc", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if _, ok := resp["executions"]; !ok {
		t.Fatal("expected executions field")
	}
	stats, ok := resp["token_stats"].(map[string]any)
	if !ok {
		t.Fatalf("expected token_stats as object, got %T", resp["token_stats"])
	}
	if _, found := stats["total_tokens"]; !found {
		t.Error("expected total_tokens in token_stats")
	}
}

func TestGetSession_NullTokenStats_WhenNoneExist(t *testing.T) {
	router, es, _, cleanup := newTestServerWithSessions(t)
	defer cleanup()

	if _, err := es.Create("worker-1", "hello", "session-xyz", "claude"); err != nil {
		t.Fatalf("Create execution: %v", err)
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/sessions/session-xyz", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["token_stats"] != nil {
		t.Error("token_stats must be null when no stats exist")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee
go test ./internal/api/... -run "TestGetSession" -v
```

Expected: FAIL — `h.GetSession undefined`

- [ ] **Step 3: Add `GetSession` and `buildSessionTokenStats` to handler**

Add to `internal/api/execution_handler.go` (before the closing of the file, after `buildTokenStatsMap`):

```go
func (h *ExecutionHandler) GetSession(c *gin.Context) {
	sessionID := c.Param("id")
	execs, err := h.executions.ListBySessionID(sessionID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if execs == nil {
		execs = []model.WorkerExecution{}
	}
	c.JSON(http.StatusOK, gin.H{
		"executions":  execs,
		"token_stats": h.buildSessionTokenStats(sessionID),
	})
}

func (h *ExecutionHandler) buildSessionTokenStats(sessionID string) *sessionTokenStats {
	rows, err := h.tokenStats.GetBySessionID(sessionID)
	if err != nil || len(rows) == 0 {
		return nil
	}
	result := &sessionTokenStats{}
	for _, row := range rows {
		result.TotalTokens += row.TotalTokens
		result.ByModel = append(result.ByModel, modelTokenStats{
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

- [ ] **Step 4: Remove `ListBySession` from handler**

Delete the following method from `internal/api/execution_handler.go` (lines 107-115):

```go
func (h *ExecutionHandler) ListBySession(c *gin.Context) {
	sessionID := c.Query("session_id")
	execs, err := h.executions.ListBySessionID(sessionID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, execs)
}
```

- [ ] **Step 5: Update route registration**

In `internal/routes/api.go`, replace line 20:
```go
// Remove:
r.GET("/sessions/executions", s.Executions.ListBySession)

// Add:
r.GET("/sessions/:id", s.Executions.GetSession)
```

- [ ] **Step 6: Run tests to verify they pass**

```bash
go test ./internal/api/... -run "TestGetSession" -v
```

Expected: PASS

- [ ] **Step 7: Run all backend tests**

```bash
go test ./internal/... -v 2>&1 | tail -20
```

Expected: all PASS (no regressions)

- [ ] **Step 8: Commit**

```bash
git add internal/api/execution_handler.go internal/api/execution_handler_test.go internal/routes/api.go
git commit -m "feat(api): add GET /sessions/:id with token_stats, remove legacy executions route"
```

---

## Task 2: Frontend Types + API Client

**Files:**
- Modify: `web/src/lib/types.ts`
- Modify: `web/src/lib/api.ts`

- [ ] **Step 1: Add `SessionDetail` type**

In `web/src/lib/types.ts`, after the `SessionTokenStats` interface (line 62), add:

```typescript
export interface SessionDetail {
  executions: WorkerExecution[]
  token_stats: SessionTokenStats | null
}
```

- [ ] **Step 2: Update API client**

In `web/src/lib/api.ts`:

1. Add `SessionDetail` to the import on line 1:
```typescript
import type { Worker, WorkerExecution, PaginatedResponse, ChatMessage, LocalMessagesResponse, Task, Department, DepartmentTree, StatsOverview, StatsTrend, EnvConfig, ExecDurationTrend, AppConfig, Engine, SessionDetail } from "./types"
```

2. Replace the `sessions` object (lines 108-113):
```typescript
sessions: {
  get: async (sessionId: string) => {
    const detail = await fetchAPI<SessionDetail>(`/sessions/${encodeURIComponent(sessionId)}`)
    return {
      executions: Array.isArray(detail?.executions) ? detail.executions : [],
      token_stats: detail?.token_stats ?? null,
    }
  },
},
```

- [ ] **Step 3: Commit**

```bash
git add web/src/lib/types.ts web/src/lib/api.ts
git commit -m "feat(types): add SessionDetail type and update api.sessions.get"
```

---

## Task 3: Extract `TokenStatsTooltip` Shared Component

**Files:**
- Create: `web/src/components/token-stats-tooltip.tsx`
- Modify: `web/src/pages/sessions.tsx`

- [ ] **Step 1: Create shared component**

Create `web/src/components/token-stats-tooltip.tsx`:

```tsx
import type { SessionTokenStats } from "@/lib/types"

export function TokenStatsTooltip({ stats }: { stats: SessionTokenStats }) {
  const sorted = [...stats.by_model].sort((a, b) => b.total_tokens - a.total_tokens)
  return (
    <div className="flex flex-col gap-1.5 font-mono text-xs min-w-[160px]">
      <div className="flex justify-between gap-4 font-semibold">
        <span>Total</span>
        <span>{stats.total_tokens.toLocaleString()}</span>
      </div>
      <div className="border-t border-background/20 pt-1 flex flex-col gap-2">
        {sorted.length === 0 ? (
          <span className="opacity-60">No model data</span>
        ) : sorted.map((m) => (
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

- [ ] **Step 2: Update `sessions.tsx` to use shared component**

In `web/src/pages/sessions.tsx`:

1. Remove the inline `TokenStatsTooltip` function definition (lines 52-84 — the entire function).

2. Add import at the top of the file (after existing imports):
```typescript
import { TokenStatsTooltip } from "@/components/token-stats-tooltip"
```

3. Remove `SessionTokenStats` from the import on line 6 (it's no longer needed in sessions.tsx since the type is used only by the extracted component):
```typescript
import { useExecutions } from "@/hooks/use-executions"
import type { WorkerExecution } from "@/lib/types"
```

- [ ] **Step 3: Commit**

```bash
git add web/src/components/token-stats-tooltip.tsx web/src/pages/sessions.tsx
git commit -m "refactor: extract TokenStatsTooltip to shared component"
```

---

## Task 4: New `useSessionDetail` Hook

**Files:**
- Create: `web/src/hooks/use-session-detail.ts`
- Modify: `web/src/hooks/use-executions.ts`

- [ ] **Step 1: Create new hook file**

Create `web/src/hooks/use-session-detail.ts`:

```typescript
import { useQuery } from "@tanstack/react-query"
import { api } from "@/lib/api"
import { isActiveStatus } from "@/lib/format"

export function useSessionDetail(sessionId: string) {
  return useQuery({
    queryKey: ["sessions", sessionId],
    queryFn: () => api.sessions.get(sessionId),
    enabled: !!sessionId,
    refetchInterval: (query) => {
      const executions = query.state.data?.executions ?? []
      return executions.some((e) => isActiveStatus(e.status)) ? 500 : false
    },
  })
}
```

- [ ] **Step 2: Remove `useSessionExecutions` from `use-executions.ts`**

In `web/src/hooks/use-executions.ts`, delete the entire `useSessionExecutions` function (lines 17-27):

```typescript
// DELETE THIS ENTIRE FUNCTION:
export function useSessionExecutions(sessionId: string) {
  return useQuery({
    queryKey: ["sessions", sessionId, "executions"],
    queryFn: () => api.sessions.executions(sessionId),
    enabled: !!sessionId,
    refetchInterval: (query) => {
      const executions = query.state.data ?? []
      return executions.some((e) => isActiveStatus(e.status)) ? 500 : false
    },
  })
}
```

- [ ] **Step 3: Commit**

```bash
git add web/src/hooks/use-session-detail.ts web/src/hooks/use-executions.ts
git commit -m "feat(hooks): add useSessionDetail, remove useSessionExecutions"
```

---

## Task 5: Session Detail Page UI + i18n

**Files:**
- Modify: `web/src/locales/en.json`
- Modify: `web/src/locales/zh.json`
- Modify: `web/src/pages/session-detail.tsx`

- [ ] **Step 1: Add i18n keys**

In `web/src/locales/en.json`, add `"tokens"` inside the `"sessionDetail"` object (after `"live": "Live"`):

```json
"live": "Live",
"tokens": "Tokens"
```

In `web/src/locales/zh.json`, add inside `"sessionDetail"`:

```json
"live": "实时",
"tokens": "Token 用量"
```

- [ ] **Step 2: Update session-detail.tsx imports**

In `web/src/pages/session-detail.tsx`:

1. Replace the `useSessionExecutions` import (line 6):
```typescript
// Remove:
import { useSessionExecutions } from "@/hooks/use-executions"
// Add:
import { useSessionDetail } from "@/hooks/use-session-detail"
```

2. Add `Zap` and `Info` to the lucide-react import (line 5):
```typescript
import { Activity, Bot, Clock3, Info, Logs, Zap } from "lucide-react"
```

3. Add `TokenStatsTooltip` import (after existing component imports):
```typescript
import { TokenStatsTooltip } from "@/components/token-stats-tooltip"
```

4. Add `Tooltip`, `TooltipTrigger`, `TooltipContent` import:
```typescript
import { Tooltip, TooltipTrigger, TooltipContent } from "@/components/ui/tooltip"
```

5. Add `formatTokenCount` to the format import (line 15):
```typescript
import { formatTimestamp, formatCompactTimestamp, formatDuration, formatTokenCount, statusTone, isActiveStatus, extractMessageContent } from "@/lib/format"
```

- [ ] **Step 3: Update data source in component**

In `web/src/pages/session-detail.tsx`, replace line 22:
```typescript
// Remove:
const { data: executions = [], error, isLoading } = useSessionExecutions(currentSessionId)

// Add:
const { data, error, isLoading } = useSessionDetail(currentSessionId)
const executions = data?.executions ?? []
const tokenStats = data?.token_stats ?? null
```

- [ ] **Step 4: Update grid from 4 to 5 columns and add Tokens stat**

In `web/src/pages/session-detail.tsx`, find the grid div (line 151):
```tsx
// Change:
<div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">

// To:
<div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-5">
```

Add the Tokens stat after the Duration `DetailOverviewStat` (after line ~191, after the closing `/>` of the Duration stat):

```tsx
<DetailOverviewStat
  icon={Zap}
  label={t("sessionDetail.tokens")}
  value={
    tokenStats ? (
      <div className="flex items-center gap-1">
        <span>{formatTokenCount(tokenStats.total_tokens)}</span>
        <Tooltip>
          <TooltipTrigger
            type="button"
            aria-label="Token breakdown"
            className="flex items-center text-muted-foreground/40 hover:text-muted-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring transition-colors"
          >
            <Info className="size-3" />
          </TooltipTrigger>
          <TooltipContent side="bottom" align="start">
            <TokenStatsTooltip stats={tokenStats} />
          </TooltipContent>
        </Tooltip>
      </div>
    ) : (
      "—"
    )
  }
/>
```

- [ ] **Step 5: Build and check for TypeScript errors**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee/web
npm run build 2>&1 | tail -30
```

Expected: 0 errors. If errors appear, fix them before proceeding.

- [ ] **Step 6: Commit**

```bash
git add web/src/locales/en.json web/src/locales/zh.json web/src/pages/session-detail.tsx
git commit -m "feat(session-detail): add Tokens overview stat with per-model tooltip"
```

---

## Task 6: Manual Verification

- [ ] **Step 1: Start the dev server**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee
make dev  # or the project's standard start command
```

- [ ] **Step 2: Open session detail page**

Navigate to: `/sessions/detail?session_id=219e0ca5-2c06-4c37-b171-d5ce60167e93`

- [ ] **Step 3: Verify**

- [ ] Overview stats grid shows 5 columns: Turns / Worker / Started / Duration / **Tokens**
- [ ] Tokens stat shows a formatted number (e.g. `12.4K`) or `—` if no data
- [ ] Clicking/hovering ℹ️ shows tooltip with per-model breakdown
- [ ] Tooltip shows model name, total, input, output, cache↑/↓ (cache rows hidden when 0)
- [ ] Sessions list (`/sessions`) still works — no regression in token display there
- [ ] No console errors
