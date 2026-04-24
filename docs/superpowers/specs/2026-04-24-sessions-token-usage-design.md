# Sessions Page Token Usage Display

**Date:** 2026-04-24
**Branch:** feat/token-usage-stats
**Status:** Approved

## Overview

Add a "Tokens" column to the `/sessions` list page that shows total token consumption per session. Hovering over an info icon reveals a per-model breakdown tooltip with input/output/cache details.

## Data Source

`bee_token_stats` table (already populated by the background token syncer running every 10 minutes):

| Column | Type | Description |
|--------|------|-------------|
| session_id | TEXT | Foreign key to session |
| model | TEXT | Model name (e.g. `claude-sonnet-4-6`) |
| input_tokens | INTEGER | Prompt tokens |
| output_tokens | INTEGER | Completion tokens |
| cache_creation_tokens | INTEGER | Cache write tokens |
| cache_read_tokens | INTEGER | Cache read tokens |
| total_tokens | INTEGER | Sum of all token types |

Each row is unique on `(session_id, model)`. A session may have multiple rows if it used multiple models.

## Backend Changes

### 1. Extend the executions list handler

The `GET /executions` list handler (`internal/api/execution_handler.go`) currently returns a flat `PaginatedResponse<WorkerExecution>`. Frontend groups these by `session_id`.

Changes:
- Inject `TokenStatsStore` into `ExecutionHandler`
- After `executions.ListPaginated()` returns, collect the unique `session_id`s from the result
- Call `tokenStats.GetBySessionIDs(sessionIDs []string)` (new store method) to batch-fetch all rows in one query
- Build a `map[string]*SessionTokenStats` keyed by `session_id`
- Include the map as a `token_stats` field alongside `items` in the response

Add `GetBySessionIDs` to `TokenStatsStore` (`internal/infra/store/token_stats_store.go`):
- Query: `SELECT * FROM bee_token_stats WHERE session_id IN (?)`
- Return `[]TokenStats`, caller builds the map and aggregates

### 2. Response shape

The `/executions` list response gains a top-level `token_stats` map keyed by `session_id`. Sessions absent from the map have not been synced yet.

```json
{
  "items": [ ...WorkerExecution objects... ],
  "total": 100,
  "page": 1,
  "page_size": 20,
  "token_stats": {
    "abc12345": {
      "total_tokens": 12400,
      "by_model": [
        {
          "model": "claude-sonnet-4-6",
          "total_tokens": 8000,
          "input_tokens": 3200,
          "output_tokens": 4800,
          "cache_creation_tokens": 0,
          "cache_read_tokens": 0
        },
        {
          "model": "claude-opus-4-7",
          "total_tokens": 4400,
          "input_tokens": 2000,
          "output_tokens": 1800,
          "cache_creation_tokens": 400,
          "cache_read_tokens": 200
        }
      ]
    },
    "def67890": null
  }
}
```

`token_stats` entry is `null` (or absent) when the session has not been synced yet.

## Frontend Changes

### 1. Type definitions (`web/src/lib/types.ts`)

```typescript
interface ModelTokenStats {
  model: string
  total_tokens: number
  input_tokens: number
  output_tokens: number
  cache_creation_tokens: number
  cache_read_tokens: number
}

interface SessionTokenStats {
  total_tokens: number
  by_model: ModelTokenStats[]
}
```

Extend `PaginatedResponse<T>` to include an optional `token_stats` field:

```typescript
interface PaginatedResponse<T> {
  items: T[]
  total: number
  page: number
  page_size: number
  token_stats?: Record<string, SessionTokenStats | null>
}
```

The `sessions.tsx` page reads `data.token_stats?.[group_session_id]` when rendering each session row.

### 2. Sessions table (`web/src/pages/sessions.tsx`)

Add a 7th column after "Duration":

**Header:** `Tokens`

**Cell:**
- No data (`token_stats === null`): render `—` in muted style
- Has data: render formatted total (e.g. `12.4K`) followed by a small `ℹ` icon

The ℹ icon is the hover target for the tooltip. The total number itself is plain text.

### 3. Token number formatting

Format using compact notation:
- `< 1,000` → raw number (e.g. `842`)
- `≥ 1,000` → one decimal K (e.g. `12.4K`)
- `≥ 1,000,000` → one decimal M (e.g. `1.2M`)

### 4. Tooltip content (shadcn/ui `Tooltip`)

Rendered inside `TooltipContent`, triggered by hovering the ℹ icon:

```
Total  12,400
─────────────────────────────
claude-sonnet-4-6       8,000
  In 3,200  Out 4,800
claude-opus-4-7         4,400
  In 2,000  Out 1,800
  Cache↑ 400  Cache↓ 200
```

Rules:
- Cache rows (Cache↑ creation, Cache↓ read) are omitted when both are 0 for that model.
- Models are listed in descending order of `total_tokens`.
- Numbers are formatted with locale-aware comma separators (no compact notation inside tooltip — full precision).

## Edge Cases

| Scenario | Behavior |
|----------|----------|
| Session not yet synced | Cell shows `—` |
| Single model | Tooltip still uses per-model layout for consistency |
| All cache tokens are 0 | Cache row hidden in tooltip |
| Very large token counts (>1M) | Formatted as `1.2M` in cell, full number in tooltip |
| Session actively running | Tooltip may show partial counts (syncer runs every 10 min) |

## Implementation Scope

This spec covers only the `/sessions` list page. The `/sessions/detail` page is out of scope for this iteration.

No changes to the token syncer, parsers, or `bee_token_stats` schema are required — all data is already available.
