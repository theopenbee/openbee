# Session Detail Token Usage — Design Spec

**Date:** 2026-04-24
**Branch:** feat/token-usage-stats

## Overview

Add token usage information to the session detail page (`/sessions/detail?session_id=<id>`). Token usage is already displayed on the sessions list page; this feature brings the same information into the session detail view.

The implementation also refactors the existing `GET /sessions/executions?session_id=<id>` endpoint into a proper RESTful `GET /sessions/:id` endpoint that includes token stats alongside executions.

---

## Goals

- Display session total token count in the Overview Stats grid (5th stat)
- Show per-model breakdown (input / output / cache_creation / cache_read) in a hover tooltip
- Refactor `GET /sessions/executions?session_id=<id>` → `GET /sessions/:id`
- Reuse existing `TokenStatsTooltip` as a shared component

---

## Backend

### New Endpoint

**Route:** `GET /sessions/:id`

**Response:**
```json
{
  "executions": [...WorkerExecution...],
  "token_stats": {
    "total_tokens": 10400,
    "by_model": [
      {
        "model": "claude-sonnet-4-6",
        "total_tokens": 8000,
        "input_tokens": 3200,
        "output_tokens": 4800,
        "cache_creation_tokens": 200,
        "cache_read_tokens": 400
      }
    ]
  }
}
```

- `token_stats` is `null` when no token data exists for the session
- Reuses `tokenStatsStore.GetBySessionID(sessionID)` to fetch per-model rows
- Aggregates rows into `SessionTokenStats` (same logic as existing `buildTokenStatsMap`)

### Removed Endpoint

`GET /sessions/executions?session_id=<id>` is deleted. All callers must migrate to the new endpoint.

### Handler Placement

New `GetSession` method on the existing `ExecutionHandler` (already has access to both `ExecutionStore` and `TokenStatsStore`). No new handler struct needed.

---

## Frontend

### New Type (`types.ts`)

```typescript
export interface SessionDetail {
  executions: WorkerExecution[]
  token_stats: SessionTokenStats | null
}
```

### API Client (`api.ts`)

```typescript
// Remove:
sessions.executions(sessionId: string): Promise<WorkerExecution[] | null>

// Add:
sessions.get(sessionId: string): Promise<SessionDetail>
// → fetchAPI<SessionDetail>(`/sessions/${encodeURIComponent(sessionId)}`)
```

### Hook (`use-session-detail.ts`)

```typescript
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

Replaces `useSessionExecutions`. File: `hooks/use-session-detail.ts` (delete `use-executions.ts`'s `useSessionExecutions` export or keep for other callers — check usage first).

### Shared Component (`components/token-stats-tooltip.tsx`)

Extract `TokenStatsTooltip` from `pages/sessions.tsx` into a standalone shared component. Update `sessions.tsx` to import from the new location.

Props remain unchanged:
```typescript
interface TokenStatsTooltipProps {
  stats: SessionTokenStats
}
```

Tooltip content (per-model rows, cache rows hidden when both cache values are 0):
```
Total: 10.4K
─────────────────────────
claude-sonnet-4-6
  Input:        3,200
  Output:       4,800
  Cache Write:    200
  Cache Read:     400
```

### Session Detail Page (`pages/session-detail.tsx`)

Add a 5th `DetailOverviewStat` to the existing hero grid:

- **Label:** `t("sessions.detail.tokens")` (i18n key)
- **Value:** `formatTokenCount(tokenStats.total_tokens)` + `TokenStatsTooltip` on ℹ️ button
- **Empty state:** `—` when `token_stats` is null

Update data source: replace `useSessionExecutions` with `useSessionDetail`, destructure `executions` and `token_stats`.

---

## Localization

Add to `en.json` and `zh.json` under `sessions.detail`:

```json
// en.json
"sessions": {
  "detail": {
    "tokens": "Tokens"
  }
}

// zh.json
"sessions": {
  "detail": {
    "tokens": "Token 用量"
  }
}
```

---

## Error & Empty States

| State | Behavior |
|-------|----------|
| No token data for session | `token_stats: null` → Overview stat shows `—`, tooltip hidden |
| Session not found | 404 from backend, existing error handling in detail page covers it |
| Token data partially synced | Shows whatever is available (sync runs every 10 min) |

---

## Testing

- **Backend:** Add test case to `execution_handler_test.go` — `GET /sessions/:id` returns `executions` + `token_stats` correctly; returns null `token_stats` when none exist
- **Manual:** Verify session `219e0ca5-2c06-4c37-b171-d5ce60167e93` detail page shows Tokens stat; verify tooltip shows per-model breakdown

---

## Files Changed

| File | Change |
|------|--------|
| `internal/api/execution_handler.go` | Add `GetSession` handler |
| `internal/api/router.go` (or equivalent) | Register `GET /sessions/:id`, remove old route |
| `internal/api/execution_handler_test.go` | Add new test |
| `web/src/lib/types.ts` | Add `SessionDetail` type |
| `web/src/lib/api.ts` | Replace `sessions.executions` with `sessions.get` |
| `web/src/hooks/use-session-detail.ts` | New file, replaces `useSessionExecutions` |
| `web/src/hooks/use-executions.ts` | Remove `useSessionExecutions` export |
| `web/src/components/token-stats-tooltip.tsx` | Extract from sessions.tsx |
| `web/src/pages/sessions.tsx` | Import from shared component |
| `web/src/pages/session-detail.tsx` | Use new hook, add Tokens stat |
| `web/src/locales/en.json` | Add `sessions.detail.tokens` key |
| `web/src/locales/zh.json` | Add `sessions.detail.tokens` key |
