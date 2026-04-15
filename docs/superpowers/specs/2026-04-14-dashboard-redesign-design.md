# Dashboard Redesign — Design Spec

**Date:** 2026-04-14  
**Status:** Approved

## Overview

Redesign the OpenBee2 dashboard stats to:

1. Remove the "今日会话" (Sessions Today) stat from the System Status section.
2. Upgrade the Messages card: add a "总消息数" (Total Messages) primary metric with received/sent as sub-items.
3. Add a new "执行耗时" (Execution Duration) card showing today's, yesterday's, and cumulative total execution duration.
4. Add a new line chart showing the daily execution duration trend over 7/15/30 days.

---

## Decisions Log

| Question | Decision |
|---|---|
| Execution duration scope | Today + yesterday + all-time cumulative |
| Execution duration card placement | New 4th card in Today Activity (not merged into existing Executions card) |
| Messages total display style | Total as large primary number; received/sent as sub-items (mirrors Executions card style) |
| API design approach | Extend `overview` + new `execution-duration-trend` endpoint (Approach B) |

---

## Architecture

### Data Flow

```
GET /api/stats/overview  (polls every 30s)
  └─ StatsOverview {
       ...existing fields (sessions_new_today removed),
       messages_total_today,           ← new (= received + sent, computed backend-side)
       exec_duration_today_ms,         ← new
       exec_duration_yesterday_ms,     ← new
       exec_duration_total_ms,         ← new (all-time)
     }

GET /api/stats/execution-duration-trend?days=7|15|30  (new endpoint, on demand)
  └─ ExecDurationTrend { days, data: [{date, total_duration_ms}] }
```

### Files Changed

```
Backend
├── internal/infra/store/stats_store.go   — extend StatsOverview, new GetExecutionDurationTrend
├── internal/api/stats_handler.go         — new GetExecutionDurationTrend handler method
└── internal/routes/api.go               — register new route GET /api/stats/execution-duration-trend

Frontend
├── web/src/lib/types.ts                                        — extend StatsOverview, new trend types
├── web/src/lib/api.ts                                          — new executionDurationTrend call
├── web/src/lib/format.ts                                       — new formatDuration utility
├── web/src/hooks/use-stats.ts                                  — new useExecutionDurationTrend hook
├── web/src/pages/dashboard.tsx                                 — layout changes
├── web/src/components/execution-duration-trend-chart.tsx       — new chart component
└── web/src/locales/{zh,en}.json                                — new i18n keys
```

---

## Backend

### StatsOverview Changes (stats_store.go)

Remove `SessionsNewToday`. Add:

```go
type StatsOverview struct {
    Departments             int       `json:"departments"`
    Workers                 int       `json:"workers"`
    ActiveWorkersToday      int       `json:"active_workers_today"`
    ActiveWorkersYesterday  int       `json:"active_workers_yesterday"`
    ActiveWorkersChange     *float64  `json:"active_workers_change"`
    MessagesReceivedToday   int       `json:"messages_received_today"`
    MessagesSentToday       int       `json:"messages_sent_today"`
    MessagesTotalToday      int       `json:"messages_total_today"`      // new, = received + sent
    ExecutionsToday         ExecStats `json:"executions_today"`
    ExecDurationTodayMS     int64     `json:"exec_duration_today_ms"`     // new
    ExecDurationYesterdayMS int64     `json:"exec_duration_yesterday_ms"` // new
    ExecDurationTotalMS     int64     `json:"exec_duration_total_ms"`     // new, all-time
    ScheduledTasks          int       `json:"scheduled_tasks"`
}
```

`MessagesTotalToday` is computed after all parallel goroutines finish (no extra SQL):
```go
ov.MessagesTotalToday = ov.MessagesReceivedToday + ov.MessagesSentToday
```

### New SQL Queries

**Today's / yesterday's execution duration** (reuses existing `dayBounds`):
```sql
SELECT COALESCE(SUM(completed_at - started_at), 0)
FROM bee_executions
WHERE status = 'completed'
  AND started_at >= ? AND started_at < ?
  AND completed_at IS NOT NULL
```

**All-time cumulative execution duration:**
```sql
SELECT COALESCE(SUM(completed_at - started_at), 0)
FROM bee_executions
WHERE status = 'completed'
  AND completed_at IS NOT NULL
```

All three queries run concurrently via `errgroup` alongside existing queries.

### New Trend Type + Query

```go
type ExecDurationTrendPoint struct {
    Date            string `json:"date"`
    TotalDurationMS int64  `json:"total_duration_ms"`
}

func (s *StatsStore) GetExecutionDurationTrend(ctx context.Context, days int) ([]ExecDurationTrendPoint, error)
```

SQL mirrors `GetTrend` but aggregates duration instead of counting workers:
```sql
SELECT DATE(started_at/1000, 'unixepoch', 'localtime') AS day,
       COALESCE(SUM(completed_at - started_at), 0) AS total_ms
FROM bee_executions
WHERE status = 'completed'
  AND completed_at IS NOT NULL
  AND started_at >= ? AND started_at < ?
GROUP BY day
ORDER BY day ASC
```

Missing days are filled with `0` using the same date-filling loop as `GetTrend`.

### New API Endpoint

- Handler: `StatsHandler.GetExecutionDurationTrend`
- Route: `GET /api/stats/execution-duration-trend?days=7|15|30`
- Validation: identical to existing `GetTrend` (days must be 7, 15, or 30)
- Response: `{ "days": N, "data": [{date, total_duration_ms}] }`

---

## Frontend

### types.ts

```ts
export interface StatsOverview {
  departments: number
  workers: number
  active_workers_today: number
  active_workers_yesterday: number
  active_workers_change: number | null
  messages_received_today: number
  messages_sent_today: number
  messages_total_today: number          // new
  executions_today: ExecStats
  exec_duration_today_ms: number        // new
  exec_duration_yesterday_ms: number    // new
  exec_duration_total_ms: number        // new
  scheduled_tasks: number
  // sessions_new_today removed
}

export interface ExecDurationTrendPoint {
  date: string
  total_duration_ms: number
}

export interface ExecDurationTrend {
  days: number
  data: ExecDurationTrendPoint[]
}
```

### format.ts — formatDuration

Converts milliseconds to a human-readable string. Smart unit switching:

| Range | Example output |
|---|---|
| < 60 000 ms | `"45s"` |
| 60 000 – 3 599 999 ms | `"12m 30s"` |
| 3 600 000 – 86 399 999 ms | `"2h 15m"` |
| ≥ 86 400 000 ms | `"2d 3h"` |

### use-stats.ts — New Hook

```ts
export function useExecutionDurationTrend(days: 7 | 15 | 30) {
  return useQuery({
    queryKey: ["stats", "execution-duration-trend", days],
    queryFn: () => api.stats.executionDurationTrend(days),
    staleTime: 60_000,
  })
}
```

### dashboard.tsx Layout Changes

**System Status** (4 cols → 3 cols):
```
[ 部门数 ]  [ Worker 数 ]  [ 定时任务数 ]
```
The `sessions_new_today` item and its i18n key `dashboard.sessionsToday` are removed from this grid.

**Today Activity** (3 cols → 4 cols):

```
┌──────────────┬──────────────┬──────────────┬──────────────┐
│ 活跃 Worker  │    消息       │   执行次数    │   执行耗时    │
│              │              │              │              │
│  今日（大字） │ 总数（大字）  │ 总数（大字）  │ 今日（大字）  │
│              │ 收到 / 发送   │ 成功 / 失败   │ 昨日 / 累计   │
│ 昨日 / 变化  │              │              │              │
└──────────────┴──────────────┴──────────────┴──────────────┘
```

Grid class changes from `sm:grid-cols-3` to `sm:grid-cols-4`.

**Charts section** (bottom of page, in order):
1. `<ActivityTrendChart />` — existing, unchanged
2. `<ExecutionDurationTrendChart />` — new, below the first chart

### execution-duration-trend-chart.tsx

Mirrors `ActivityTrendChart` structure exactly:
- Card with header (label + day selector buttons 7/15/30)
- `useExecutionDurationTrend(days)` hook
- `ResponsiveContainer` → `LineChart` from recharts
- Y-axis tick formatter: convert ms to minutes (`v / 60000`)
- Tooltip formatter: `formatDuration(value)` for human-readable display
- Line `dataKey="total_duration_ms"`
- Title i18n key: `dashboard.executionDurationTrend`

### i18n Keys

```json
// zh.json
"dashboard.executionDuration": "执行耗时",
"dashboard.execDurationToday": "今日耗时",
"dashboard.execDurationYesterday": "昨日耗时",
"dashboard.execDurationTotal": "累计耗时",
"dashboard.executionDurationTrend": "执行耗时趋势",
"dashboard.executionDurationTrendAriaLabel": "最近 {{days}} 天执行耗时趋势"

// en.json
"dashboard.executionDuration": "Execution Duration",
"dashboard.execDurationToday": "Today",
"dashboard.execDurationYesterday": "Yesterday",
"dashboard.execDurationTotal": "All Time",
"dashboard.executionDurationTrend": "Execution Duration Trend",
"dashboard.executionDurationTrendAriaLabel": "Execution duration trend over {{days}} days"
```

---

## Error Handling

- `completed_at IS NOT NULL` guard prevents null arithmetic in SQL.
- `COALESCE(..., 0)` ensures zero is returned when no completed executions exist.
- Duration trend fills missing days with `0` (same pattern as active-workers trend).
- `formatDuration(0)` returns `"0s"`.

---

## Out of Scope

- No database migrations needed (derived from existing `started_at` / `completed_at` fields).
- No changes to execution or message write paths.
- No changes to the existing `ActivityTrendChart` component or its API.
