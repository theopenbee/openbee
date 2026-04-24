# Dashboard Token Usage Stats — Design Spec

**Date:** 2026-04-24  
**Branch:** feat/token-usage-stats  
**Status:** Approved

---

## Overview

Add token consumption visibility to the dashboard page. Users will be able to see total all-time token usage, today's and yesterday's token usage with input/output breakdowns, and a daily token trend chart for 7/15/30-day windows.

---

## UI Layout

### System Status Section (6 items, was 5)

Grid changes from `sm:grid-cols-5` to `sm:grid-cols-3 lg:grid-cols-6`.

New sixth item: **Total Tokens** — all-time sum of `total_tokens` across all sessions.  
Hovering the number shows a tooltip with all-time input tokens and output tokens.

### Today Activity Section (5 cards, was 4)

Grid changes from `sm:grid-cols-4` to `sm:grid-cols-5`.

New fifth card: **Tokens**
- Primary number: today's total tokens (formatted with `formatTokenCount`, e.g. `12.4K`)
- Sub-stats row:
  - Yesterday's total tokens
  - Day-over-day percentage change with trend icon
- Hovering the primary number shows a tooltip: today's input tokens + output tokens
- An info icon with tooltip noting: "Cross-day sessions are counted in each active day"

### Charts Section (2-column side by side, was 1 full-width)

The chart area becomes a responsive two-column grid (`grid-cols-1 lg:grid-cols-2`). Each chart takes 50% width on large screens, stacks vertically on small screens.

- **Left: Token Trend** (new) — single line `total_tokens` per day, 7/15/30-day selector
- **Right: Activity Trend** (existing) — unchanged, dual-line active workers + execution duration

---

## Backend

### Data Source & Time Attribution

Token usage is attributed to a day based on `bee_executions.completed_at` for sessions associated with that day.

**Known caveat:** A session whose executions span midnight will be counted in both the earlier and later day. This is an accepted approximation; the UI displays a note.

### `StatsOverview` Struct Extensions (`internal/infra/store/stats_store.go`)

```go
TokensTotal          int64 `json:"tokens_total"`
TokensTodayTotal     int64 `json:"tokens_today_total"`
TokensTodayInput     int64 `json:"tokens_today_input"`
TokensTodayOutput    int64 `json:"tokens_today_output"`
TokensYestTotal      int64 `json:"tokens_yesterday_total"`
TokensYestInput      int64 `json:"tokens_yesterday_input"`
TokensYestOutput     int64 `json:"tokens_yesterday_output"`
```

### `GetOverview()` New Queries

Three new concurrent goroutines added to the existing `errgroup`:

```sql
-- All-time totals
SELECT COALESCE(SUM(total_tokens), 0),
       COALESCE(SUM(input_tokens), 0),
       COALESCE(SUM(output_tokens), 0)
FROM bee_token_stats

-- Today totals: sessions with any execution completed today
SELECT COALESCE(SUM(ts.total_tokens), 0),
       COALESCE(SUM(ts.input_tokens), 0),
       COALESCE(SUM(ts.output_tokens), 0)
FROM bee_token_stats ts
WHERE ts.session_id IN (
  SELECT DISTINCT session_id FROM bee_executions
  WHERE completed_at >= :todayStart AND completed_at < :todayEnd
    AND session_id IS NOT NULL
)

-- Yesterday totals: same pattern with yestStart / yestEnd
```

### New `TokenTrendPoint` Type

```go
type TokenTrendPoint struct {
    Date        string `json:"date"`
    TotalTokens int64  `json:"total_tokens"`
}
```

### New `GetTokenTrend(ctx, days)` Method

Query: group by calendar day using `bee_executions.completed_at` as the date anchor (same approach as `GetExecutionDurationTrend`). A session spanning multiple days contributes its full `total_tokens` to each day it appears.

To avoid double-counting when a session has multiple executions on the same day, first derive distinct (session_id, day) pairs, then join with token stats:

```sql
SELECT day, COALESCE(SUM(ts.total_tokens), 0) AS tokens
FROM (
  SELECT DISTINCT session_id,
         DATE(completed_at/1000, 'unixepoch', 'localtime') AS day
  FROM bee_executions
  WHERE completed_at >= :startMS AND completed_at < :endMS
    AND session_id IS NOT NULL
) sessions
JOIN bee_token_stats ts ON ts.session_id = sessions.session_id
GROUP BY day
ORDER BY day ASC
```

Zero-fill missing days server-side (same pattern as existing trend methods).

### New API Route

```
GET /stats/token-trend?days=7|15|30
```

Response: `{ "days": 7, "data": [{ "date": "2026-04-18", "total_tokens": 12400 }, ...] }`

Handler method: `StatsHandler.GetTokenTrend()` — follows the same pattern as `GetExecutionDurationTrend`.

---

## Frontend

### Types (`web/src/lib/types.ts`)

Extend `StatsOverview`:
```ts
tokens_total: number
tokens_today_total: number
tokens_today_input: number
tokens_today_output: number
tokens_yesterday_total: number
tokens_yesterday_input: number
tokens_yesterday_output: number
```

Add new types:
```ts
interface TokenTrendPoint {
  date: string
  total_tokens: number
}
interface TokenTrend {
  days: number
  data: TokenTrendPoint[]
}
```

### API Client (`web/src/lib/api.ts`)

```ts
stats.tokenTrend: (days: 7 | 15 | 30) =>
  fetchAPI<TokenTrend>(`/stats/token-trend?days=${days}`)
```

### Hook (`web/src/hooks/use-stats.ts`)

```ts
export const useTokenTrend = (days: 7 | 15 | 30) =>
  useStatsDayTrend("token-trend", api.stats.tokenTrend, days)
```

### New Component: `TokenTrendChart` (`web/src/components/token-trend-chart.tsx`)

- Mirrors `CombinedTrendChart` structure
- Uses `useTokenTrend(days)` hook
- Single `<Line>` for `total_tokens`
- Single Y-axis with `formatTokenCount` tick formatter
- Reuses `TrendLineCard`, `CHART_TOOLTIP_STYLE`
- Tooltip formatter shows compact token count

### Dashboard Page (`web/src/pages/dashboard.tsx`)

1. **`EMPTY` constant**: add 7 token fields defaulting to `0`
2. **System Status grid**: `grid-cols-2 sm:grid-cols-3 lg:grid-cols-6`, add Total Tokens item with hover tooltip (input + output)
3. **Today Activity grid**: `grid-cols-1 sm:grid-cols-5`, add Tokens card with:
   - Primary: `tokens_today_total` via `formatTokenCount`
   - Hover tooltip: `tokens_today_input` / `tokens_today_output`
   - Secondary: yesterday total + day-over-day % change
   - Info icon with cross-day caveat note
4. **Charts area**: wrap in `grid grid-cols-1 lg:grid-cols-2 gap-6`, place `<TokenTrendChart />` left, `<CombinedTrendChart />` right

### i18n (`web/src/locales/`)

New keys needed:
- `dashboard.totalTokens`
- `dashboard.tokensToday`
- `dashboard.tokensYesterday`  
- `dashboard.tokensTodayInput`
- `dashboard.tokensTodayOutput`
- `dashboard.tokensTrend`
- `dashboard.tokensTrendAriaLabel`
- `dashboard.tokensCrossDayNote`

---

## Scope Boundaries

- No per-worker or per-model breakdown on the dashboard (out of scope for this iteration)
- No cache tokens displayed (input + output only in tooltips)
- Token trend chart shows only `total_tokens` per day (no input/output split lines)
- Cross-day double-counting is acknowledged and noted in the UI, not engineered away

---

## File Change Summary

| File | Change |
|------|--------|
| `internal/infra/store/stats_store.go` | Extend `StatsOverview`, add token queries to `GetOverview`, add `GetTokenTrend` |
| `internal/api/stats_handler.go` | Add `GetTokenTrend` handler method |
| `internal/routes/api.go` | Register `GET /stats/token-trend` route |
| `web/src/lib/types.ts` | Extend `StatsOverview`, add `TokenTrendPoint`, `TokenTrend` |
| `web/src/lib/api.ts` | Add `stats.tokenTrend` |
| `web/src/hooks/use-stats.ts` | Add `useTokenTrend` |
| `web/src/components/token-trend-chart.tsx` | New component |
| `web/src/pages/dashboard.tsx` | Update all three sections |
| `web/src/locales/*.json` | Add i18n keys |
