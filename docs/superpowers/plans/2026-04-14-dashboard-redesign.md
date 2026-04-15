# Dashboard Redesign Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Redesign the dashboard to remove "今日会话", add total message count, add an execution duration card (today / yesterday / cumulative), and add an execution duration trend chart.

**Architecture:** Extend `StatsOverview` with four new fields and add a new `GET /api/stats/execution-duration-trend` endpoint backed by a new store method. On the frontend, add a `formatTotalDuration` utility, expand the Today Activity grid from 3 to 4 columns, and add an `ExecutionDurationTrendChart` component below the existing activity trend chart.

**Tech Stack:** Go (gin, SQLite via database/sql, errgroup), React + TypeScript, recharts, react-query, i18next

**Spec:** `docs/superpowers/specs/2026-04-14-dashboard-redesign-design.md`

---

## File Map

| File | Change |
|---|---|
| `internal/infra/store/stats_store.go` | Remove `SessionsNewToday`; add `MessagesTotalToday`, `ExecDurationTodayMS`, `ExecDurationYesterdayMS`, `ExecDurationTotalMS`; add `ExecDurationTrendPoint` type and `GetExecutionDurationTrend` method |
| `internal/infra/store/stats_store_test.go` | Update overview test (remove `SessionsNewToday` assertion, add duration assertions); add `GetExecutionDurationTrend` tests |
| `internal/api/stats_handler.go` | Add `GetExecutionDurationTrend` handler method |
| `internal/api/stats_handler_test.go` | Add handler test for the new endpoint |
| `internal/routes/api.go` | Register `GET /api/stats/execution-duration-trend` |
| `web/src/lib/types.ts` | Extend `StatsOverview`; add `ExecDurationTrendPoint`, `ExecDurationTrend` |
| `web/src/lib/api.ts` | Add `api.stats.executionDurationTrend(days)` |
| `web/src/lib/format.ts` | Add `formatTotalDuration(ms: number): string` |
| `web/src/lib/__tests__/format.test.ts` | Add `formatTotalDuration` tests |
| `web/src/hooks/use-stats.ts` | Add `useExecutionDurationTrend` hook |
| `web/src/pages/dashboard.tsx` | Remove sessions stat; update Messages card; add Execution Duration card; include new chart |
| `web/src/components/execution-duration-trend-chart.tsx` | New component (mirrors `ActivityTrendChart`) |
| `web/src/locales/zh.json` | Add new i18n keys |
| `web/src/locales/en.json` | Add new i18n keys |

---

## Task 1: Backend — Extend StatsOverview with duration and total-message fields

**Files:**
- Modify: `internal/infra/store/stats_store.go`
- Modify: `internal/infra/store/stats_store_test.go`

### Steps

- [ ] **Step 1: Update `StatsOverview` struct — remove `SessionsNewToday`, add four new fields**

In `internal/infra/store/stats_store.go`, replace the `StatsOverview` struct:

```go
// StatsOverview holds all numeric dashboard card data.
type StatsOverview struct {
	Departments             int       `json:"departments"`
	Workers                 int       `json:"workers"`
	ActiveWorkersToday      int       `json:"active_workers_today"`
	ActiveWorkersYesterday  int       `json:"active_workers_yesterday"`
	ActiveWorkersChange     *float64  `json:"active_workers_change"`
	MessagesReceivedToday   int       `json:"messages_received_today"`
	MessagesSentToday       int       `json:"messages_sent_today"`
	MessagesTotalToday      int       `json:"messages_total_today"`
	ExecutionsToday         ExecStats `json:"executions_today"`
	ExecDurationTodayMS     int64     `json:"exec_duration_today_ms"`
	ExecDurationYesterdayMS int64     `json:"exec_duration_yesterday_ms"`
	ExecDurationTotalMS     int64     `json:"exec_duration_total_ms"`
	ScheduledTasks          int       `json:"scheduled_tasks"`
}
```

- [ ] **Step 2: Add duration queries to `GetOverview`**

In `GetOverview`, add three new `eg.Go` blocks immediately after the existing outbound-message goroutine (around line 103). Also remove the `SessionsNewToday` goroutine entirely. The three new goroutines are:

```go
durationQuery := `
    SELECT COALESCE(SUM(completed_at - started_at), 0)
    FROM bee_executions
    WHERE status = 'completed'
      AND completed_at IS NOT NULL
      AND started_at >= ? AND started_at < ?`

eg.Go(func() error {
    return s.db.QueryRowContext(egc, durationQuery, todayStart, todayEnd).Scan(&ov.ExecDurationTodayMS)
})

eg.Go(func() error {
    return s.db.QueryRowContext(egc, durationQuery, yestStart, yestEnd).Scan(&ov.ExecDurationYesterdayMS)
})

eg.Go(func() error {
    return s.db.QueryRowContext(egc, `
        SELECT COALESCE(SUM(completed_at - started_at), 0)
        FROM bee_executions
        WHERE status = 'completed'
          AND completed_at IS NOT NULL`).Scan(&ov.ExecDurationTotalMS)
})
```

- [ ] **Step 3: Compute `MessagesTotalToday` after `eg.Wait()`**

In `GetOverview`, after the `if err := eg.Wait(); err != nil` block and before the `ActiveWorkersChange` calculation, add:

```go
ov.MessagesTotalToday = ov.MessagesReceivedToday + ov.MessagesSentToday
```

- [ ] **Step 4: Run existing tests — expect one failure**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee2
go test ./internal/infra/store/... -run TestStatsStore -v
```

Expected: `TestStatsStore_GetOverview_Counts` FAIL because it still references `ov.SessionsNewToday`.

- [ ] **Step 5: Update `TestStatsStore_GetOverview_Counts`**

In `internal/infra/store/stats_store_test.go`:

1. Remove the `SessionsNewToday` assertion block (lines 89–91):
```go
// DELETE these lines:
if ov.SessionsNewToday != 3 {
    t.Errorf("SessionsNewToday: want 3, got %d", ov.SessionsNewToday)
}
```

2. After the `MessagesSentToday` assertion, add assertions for the new fields. The existing test inserts don't have `completed_at`, so duration values will be 0. Add:
```go
if ov.MessagesTotalToday != 2 {
    t.Errorf("MessagesTotalToday: want 2, got %d", ov.MessagesTotalToday)
}
// Duration fields are 0 because test executions have no completed_at set
if ov.ExecDurationTodayMS != 0 {
    t.Errorf("ExecDurationTodayMS: want 0 (no completed_at set), got %d", ov.ExecDurationTodayMS)
}
```

- [ ] **Step 6: Add a focused duration test**

Append to `internal/infra/store/stats_store_test.go`:

```go
func TestStatsStore_GetOverview_ExecDuration(t *testing.T) {
	ss, ws, _, _, _, _, cleanup := newStatsTestDB(t)
	defer cleanup()
	ctx := context.Background()

	w1, _ := ws.Create(model.Worker{Name: "W1", WorkDir: "/tmp/w1"})
	db := ss.db

	todayStart, todayEnd := dayBounds(0)
	yestStart, _ := dayBounds(-1)

	// Today: two completed executions with known durations (1000ms + 2000ms = 3000ms)
	for _, pair := range [][2]int64{
		{todayStart + 100, todayStart + 1100},  // 1000ms
		{todayStart + 200, todayStart + 2200},  // 2000ms
	} {
		if _, err := db.Exec(`INSERT INTO bee_executions
			(id,worker_id,session_id,trigger_input,status,result,ai_process_pid,started_at,completed_at)
			VALUES (?,?,?,?,?,?,?,?,?)`,
			uuid.New().String(), w1.ID, "s1", "hi", "completed", "", 0, pair[0], pair[1]); err != nil {
			t.Fatalf("insert: %v", err)
		}
	}

	// Yesterday: one completed execution of 500ms
	if _, err := db.Exec(`INSERT INTO bee_executions
		(id,worker_id,session_id,trigger_input,status,result,ai_process_pid,started_at,completed_at)
		VALUES (?,?,?,?,?,?,?,?,?)`,
		uuid.New().String(), w1.ID, "s2", "hi", "completed", "", 0, yestStart+100, yestStart+600); err != nil {
		t.Fatalf("insert: %v", err)
	}

	// A failed execution today (should NOT count toward duration)
	if _, err := db.Exec(`INSERT INTO bee_executions
		(id,worker_id,session_id,trigger_input,status,result,ai_process_pid,started_at,completed_at)
		VALUES (?,?,?,?,?,?,?,?,?)`,
		uuid.New().String(), w1.ID, "s3", "hi", "failed", "", 0, todayStart+300, todayStart+800); err != nil {
		t.Fatalf("insert: %v", err)
	}

	ov, err := ss.GetOverview(ctx)
	if err != nil {
		t.Fatalf("GetOverview: %v", err)
	}

	if ov.ExecDurationTodayMS != 3000 {
		t.Errorf("ExecDurationTodayMS: want 3000, got %d", ov.ExecDurationTodayMS)
	}
	if ov.ExecDurationYesterdayMS != 500 {
		t.Errorf("ExecDurationYesterdayMS: want 500, got %d", ov.ExecDurationYesterdayMS)
	}
	// Cumulative = today(3000) + yesterday(500) = 3500 (failed exec not counted)
	if ov.ExecDurationTotalMS != 3500 {
		t.Errorf("ExecDurationTotalMS: want 3500, got %d", ov.ExecDurationTotalMS)
	}

	_ = todayEnd
}
```

- [ ] **Step 7: Run store tests — all pass**

```bash
go test ./internal/infra/store/... -run TestStatsStore -v
```

Expected: all `TestStatsStore_*` tests PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/infra/store/stats_store.go internal/infra/store/stats_store_test.go
git commit -m "feat: extend StatsOverview with exec duration and total message fields"
```

---

## Task 2: Backend — Add `GetExecutionDurationTrend` store method + API endpoint

**Files:**
- Modify: `internal/infra/store/stats_store.go`
- Modify: `internal/api/stats_handler.go`
- Modify: `internal/api/stats_handler_test.go`
- Modify: `internal/routes/api.go`

### Steps

- [ ] **Step 1: Write the failing test for `GetExecutionDurationTrend`**

Append to `internal/infra/store/stats_store_test.go`:

```go
func TestStatsStore_GetExecutionDurationTrend_FillsMissingDays(t *testing.T) {
	ss, ws, _, _, _, _, cleanup := newStatsTestDB(t)
	defer cleanup()
	ctx := context.Background()

	w1, _ := ws.Create(model.Worker{Name: "W1", WorkDir: "/tmp/w1"})
	db := ss.db

	// One completed execution 3 days ago: 2000ms
	threeDaysAgo := time.Now().AddDate(0, 0, -3).UnixMilli()
	if _, err := db.Exec(`INSERT INTO bee_executions
		(id,worker_id,session_id,trigger_input,status,result,ai_process_pid,started_at,completed_at)
		VALUES (?,?,?,?,?,?,?,?,?)`,
		uuid.New().String(), w1.ID, "s1", "hi", "completed", "", 0, threeDaysAgo, threeDaysAgo+2000); err != nil {
		t.Fatalf("insert: %v", err)
	}

	// A failed execution 3 days ago (must NOT appear in totals)
	if _, err := db.Exec(`INSERT INTO bee_executions
		(id,worker_id,session_id,trigger_input,status,result,ai_process_pid,started_at,completed_at)
		VALUES (?,?,?,?,?,?,?,?,?)`,
		uuid.New().String(), w1.ID, "s1", "hi", "failed", "", 0, threeDaysAgo+100, threeDaysAgo+5000); err != nil {
		t.Fatalf("insert: %v", err)
	}

	points, err := ss.GetExecutionDurationTrend(ctx, 7)
	if err != nil {
		t.Fatalf("GetExecutionDurationTrend: %v", err)
	}

	if len(points) != 7 {
		t.Fatalf("want 7 points, got %d", len(points))
	}

	target := time.Now().AddDate(0, 0, -3).Format("2006-01-02")
	found := false
	for _, p := range points {
		if p.Date == target {
			found = true
			if p.TotalDurationMS != 2000 {
				t.Errorf("date %s: want TotalDurationMS=2000, got %d", target, p.TotalDurationMS)
			}
		} else {
			if p.TotalDurationMS != 0 {
				t.Errorf("date %s: want 0, got %d", p.Date, p.TotalDurationMS)
			}
		}
	}
	if !found {
		t.Errorf("date %s not found in trend points", target)
	}

	_ = w1
}
```

- [ ] **Step 2: Run the test — expect FAIL (method not defined)**

```bash
go test ./internal/infra/store/... -run TestStatsStore_GetExecutionDurationTrend -v
```

Expected: compile error — `ss.GetExecutionDurationTrend` undefined.

- [ ] **Step 3: Add `ExecDurationTrendPoint` type and `GetExecutionDurationTrend` to `stats_store.go`**

Append after the `TrendPoint` struct and `GetTrend` method in `internal/infra/store/stats_store.go`:

```go
// ExecDurationTrendPoint is one day's total execution duration.
type ExecDurationTrendPoint struct {
	Date            string `json:"date"`
	TotalDurationMS int64  `json:"total_duration_ms"`
}

// GetExecutionDurationTrend returns the sum of completed execution durations
// for each of the last `days` days (local time), filling missing days with zero.
func (s *StatsStore) GetExecutionDurationTrend(ctx context.Context, days int) ([]ExecDurationTrendPoint, error) {
	now := time.Now()
	y, m, d := now.Date()
	loc := now.Location()

	startOfToday := time.Date(y, m, d, 0, 0, 0, 0, loc)
	startOfRange := startOfToday.AddDate(0, 0, -(days - 1))
	startMS := startOfRange.UnixMilli()
	endMS := startOfToday.Add(24 * time.Hour).UnixMilli()

	rows, err := s.db.QueryContext(ctx, `
		SELECT DATE(started_at/1000, 'unixepoch', 'localtime') AS day,
		       COALESCE(SUM(completed_at - started_at), 0) AS total_ms
		FROM bee_executions
		WHERE status = 'completed'
		  AND completed_at IS NOT NULL
		  AND started_at >= ? AND started_at < ?
		GROUP BY day
		ORDER BY day ASC`, startMS, endMS)
	if err != nil {
		return nil, fmt.Errorf("execution duration trend query: %w", err)
	}
	defer rows.Close()

	dbTotals := make(map[string]int64, days)
	for rows.Next() {
		var day string
		var total int64
		if err := rows.Scan(&day, &total); err != nil {
			return nil, err
		}
		dbTotals[day] = total
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("execution duration trend rows: %w", err)
	}

	points := make([]ExecDurationTrendPoint, days)
	for i := 0; i < days; i++ {
		date := startOfRange.AddDate(0, 0, i).Format("2006-01-02")
		points[i] = ExecDurationTrendPoint{Date: date, TotalDurationMS: dbTotals[date]}
	}
	return points, nil
}
```

- [ ] **Step 4: Run the test — expect PASS**

```bash
go test ./internal/infra/store/... -run TestStatsStore_GetExecutionDurationTrend -v
```

Expected: PASS.

- [ ] **Step 5: Write failing handler test**

Append to `internal/api/stats_handler_test.go`:

```go
func TestGetExecutionDurationTrend_ValidDays(t *testing.T) {
	router, _, cleanup := newTestServerWithStats(t)
	defer cleanup()

	for _, days := range []int{7, 15, 30} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/stats/execution-duration-trend?days="+strconv.Itoa(days), nil)
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("days=%d: expected 200, got %d: %s", days, w.Code, w.Body.String())
		}

		var resp struct {
			Days int              `json:"days"`
			Data []map[string]any `json:"data"`
		}
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("days=%d: decode: %v", days, err)
		}
		if len(resp.Data) != days {
			t.Errorf("days=%d: want %d points, got %d", days, days, len(resp.Data))
		}
		// Each point must have total_duration_ms
		for _, pt := range resp.Data {
			if _, ok := pt["total_duration_ms"]; !ok {
				t.Errorf("days=%d: point missing total_duration_ms: %v", days, pt)
			}
		}
	}
}

func TestGetExecutionDurationTrend_InvalidDays_Returns400(t *testing.T) {
	router, _, cleanup := newTestServerWithStats(t)
	defer cleanup()

	for _, bad := range []string{"99", "0", "abc", "-1"} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/stats/execution-duration-trend?days="+bad, nil)
		router.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("days=%q: expected 400, got %d", bad, w.Code)
		}
	}
}
```

- [ ] **Step 6: Run handler tests — expect FAIL (route not registered)**

```bash
go test ./internal/api/... -run TestGetExecutionDurationTrend -v
```

Expected: FAIL — 404 because the route doesn't exist yet.

- [ ] **Step 7: Add handler method to `stats_handler.go`**

Append to `internal/api/stats_handler.go`:

```go
func (h *StatsHandler) GetExecutionDurationTrend(c *gin.Context) {
	daysStr := c.DefaultQuery("days", "7")
	days, err := strconv.Atoi(daysStr)
	if err != nil || (days != 7 && days != 15 && days != 30) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "days must be 7, 15, or 30"})
		return
	}

	points, err := h.stats.GetExecutionDurationTrend(c.Request.Context(), days)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"days": days, "data": points})
}
```

- [ ] **Step 8: Register route in `api.go`**

In `internal/routes/api.go`, add after the existing trend route:

```go
r.GET("/stats/execution-duration-trend", s.Stats.GetExecutionDurationTrend)
```

Also add `GetExecutionDurationTrend` to the test server setup in `stats_handler_test.go`. Update `newTestServerWithStats`:

```go
func newTestServerWithStats(t *testing.T) (*gin.Engine, *store.StatsStore, func()) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	db, err := store.InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}

	ss := store.NewStatsStore(db)

	h := NewStatsHandler(ss)
	router := gin.New()
	api := router.Group("/api")
	api.GET("/stats/overview", h.GetOverview)
	api.GET("/stats/trend", h.GetTrend)
	api.GET("/stats/execution-duration-trend", h.GetExecutionDurationTrend)

	return router, ss, func() { db.Close() }
}
```

- [ ] **Step 9: Run all handler tests — all pass**

```bash
go test ./internal/api/... -run TestGet -v
```

Expected: all `TestGet*` tests PASS.

- [ ] **Step 10: Run full backend test suite**

```bash
go test ./... 2>&1 | tail -20
```

Expected: no failures.

- [ ] **Step 11: Commit**

```bash
git add internal/infra/store/stats_store.go internal/infra/store/stats_store_test.go \
        internal/api/stats_handler.go internal/api/stats_handler_test.go \
        internal/routes/api.go
git commit -m "feat: add GetExecutionDurationTrend store method and API endpoint"
```

---

## Task 3: Frontend — Types, API client, and hooks

**Files:**
- Modify: `web/src/lib/types.ts`
- Modify: `web/src/lib/api.ts`
- Modify: `web/src/hooks/use-stats.ts`

### Steps

- [ ] **Step 1: Update `StatsOverview` in `types.ts`**

Replace the `StatsOverview` interface:

```ts
export interface StatsOverview {
  departments: number
  workers: number
  active_workers_today: number
  active_workers_yesterday: number
  active_workers_change: number | null
  messages_received_today: number
  messages_sent_today: number
  messages_total_today: number
  executions_today: ExecStats
  exec_duration_today_ms: number
  exec_duration_yesterday_ms: number
  exec_duration_total_ms: number
  scheduled_tasks: number
}
```

Also add the new trend types after `StatsTrend`:

```ts
export interface ExecDurationTrendPoint {
  date: string
  total_duration_ms: number
}

export interface ExecDurationTrend {
  days: number
  data: ExecDurationTrendPoint[]
}
```

- [ ] **Step 2: Update `api.ts` — add import and new method**

At the top of `api.ts`, add `ExecDurationTrend` to the import:

```ts
import type { Worker, WorkerExecution, PaginatedResponse, ChatMessage, LocalMessagesResponse, Task, Department, DepartmentTree, StatsOverview, StatsTrend, ExecDurationTrend } from "./types"
```

In the `stats` section of the `api` object, add:

```ts
executionDurationTrend: (days: 7 | 15 | 30) =>
  fetchAPI<ExecDurationTrend>(`/stats/execution-duration-trend?days=${days}`),
```

- [ ] **Step 3: Add hook to `use-stats.ts`**

Append to `web/src/hooks/use-stats.ts`:

```ts
export function useExecutionDurationTrend(days: 7 | 15 | 30) {
  return useQuery({
    queryKey: ["stats", "execution-duration-trend", days],
    queryFn: () => api.stats.executionDurationTrend(days),
    staleTime: 60_000,
  })
}
```

- [ ] **Step 4: Verify TypeScript compiles**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee2/web
npm run build 2>&1 | grep -E "error|Error" | head -20
```

Expected: no type errors related to the changed files.

- [ ] **Step 5: Commit**

```bash
git add web/src/lib/types.ts web/src/lib/api.ts web/src/hooks/use-stats.ts
git commit -m "feat: add exec duration trend types, API client, and hook"
```

---

## Task 4: Frontend — `formatTotalDuration` utility

**Files:**
- Modify: `web/src/lib/format.ts`
- Modify: `web/src/lib/__tests__/format.test.ts`

### Steps

- [ ] **Step 1: Write failing tests for `formatTotalDuration`**

Append the following `describe` block to the end of `web/src/lib/__tests__/format.test.ts`. Do not touch the existing `extractMessageContent` tests. Also update the import line at the top of that file to add `formatTotalDuration`:

```ts
// Updated import at the top of the file:
import { extractMessageContent, formatTotalDuration } from "../format"
```

Then append at the end of the file:

```ts
describe("formatTotalDuration", () => {
  it("returns 0s for zero milliseconds", () => {
    expect(formatTotalDuration(0)).toBe("0s")
  })

  it("formats sub-minute durations as seconds", () => {
    expect(formatTotalDuration(45_000)).toBe("45s")
    expect(formatTotalDuration(1_000)).toBe("1s")
    expect(formatTotalDuration(59_999)).toBe("59s")
  })

  it("formats minute-range durations as m s", () => {
    expect(formatTotalDuration(90_000)).toBe("1m 30s")
    expect(formatTotalDuration(750_000)).toBe("12m 30s")
    expect(formatTotalDuration(3_599_999)).toBe("59m 59s")
  })

  it("formats hour-range durations as h m", () => {
    expect(formatTotalDuration(3_600_000)).toBe("1h 0m")
    expect(formatTotalDuration(8_100_000)).toBe("2h 15m")
    expect(formatTotalDuration(86_399_999)).toBe("23h 59m")
  })

  it("formats day-range durations as d h", () => {
    expect(formatTotalDuration(86_400_000)).toBe("1d 0h")
    expect(formatTotalDuration(97_200_000)).toBe("1d 3h")
  })
})
```

- [ ] **Step 2: Run tests — expect FAIL**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee2/web
npx vitest run src/lib/__tests__/format.test.ts 2>&1 | tail -20
```

Expected: FAIL — `formatTotalDuration` is not exported.

- [ ] **Step 3: Implement `formatTotalDuration` in `format.ts`**

Append to `web/src/lib/format.ts`:

```ts
/**
 * Formats a total duration in milliseconds to a human-readable string.
 * Examples: 45000 → "45s", 90000 → "1m 30s", 8100000 → "2h 15m", 97200000 → "1d 3h"
 */
export function formatTotalDuration(ms: number): string {
  if (ms <= 0) return "0s"
  const totalSec = Math.floor(ms / 1000)
  const days = Math.floor(totalSec / 86400)
  const hours = Math.floor((totalSec % 86400) / 3600)
  const minutes = Math.floor((totalSec % 3600) / 60)
  const seconds = totalSec % 60
  if (days > 0) return `${days}d ${hours}h`
  if (hours > 0) return `${hours}h ${minutes}m`
  if (minutes > 0) return `${minutes}m ${seconds}s`
  return `${seconds}s`
}
```

- [ ] **Step 4: Run tests — expect PASS**

```bash
npx vitest run src/lib/__tests__/format.test.ts 2>&1 | tail -20
```

Expected: all tests PASS.

- [ ] **Step 5: Commit**

```bash
git add web/src/lib/format.ts web/src/lib/__tests__/format.test.ts
git commit -m "feat: add formatTotalDuration utility"
```

---

## Task 5: Frontend — i18n keys

**Files:**
- Modify: `web/src/locales/zh.json`
- Modify: `web/src/locales/en.json`

### Steps

- [ ] **Step 1: Add keys to `zh.json`**

In the `"dashboard"` section of `web/src/locales/zh.json`, add the following new keys after `"activityTrendAriaLabel"`:

```json
"messagesTotal": "总计",
"executionDuration": "执行耗时",
"execDurationToday": "今日耗时",
"execDurationYesterday": "昨日耗时",
"execDurationTotal": "累计耗时",
"executionDurationTrend": "执行耗时趋势",
"noExecDurationData": "暂无耗时数据",
"noExecDurationDataDesc": "任务执行完成后，执行耗时趋势将显示在此处。",
"executionDurationTrendAriaLabel": "最近 {{days}} 天执行耗时趋势"
```

- [ ] **Step 2: Add keys to `en.json`**

In the `"dashboard"` section of `web/src/locales/en.json`, add the same keys:

```json
"messagesTotal": "Total",
"executionDuration": "Exec Duration",
"execDurationToday": "Today",
"execDurationYesterday": "Yesterday",
"execDurationTotal": "All Time",
"executionDurationTrend": "Execution Duration Trend",
"noExecDurationData": "No duration data",
"noExecDurationDataDesc": "Execution duration trend will appear here once tasks have completed.",
"executionDurationTrendAriaLabel": "Execution duration trend over {{days}} days"
```

- [ ] **Step 3: Commit**

```bash
git add web/src/locales/zh.json web/src/locales/en.json
git commit -m "feat: add i18n keys for exec duration card and trend chart"
```

---

## Task 6: Frontend — Dashboard layout changes (System Status + Messages card)

**Files:**
- Modify: `web/src/pages/dashboard.tsx`

### Steps

- [ ] **Step 1: Update `EMPTY` constant — remove `sessions_new_today`, add new fields**

In `dashboard.tsx`, replace the `EMPTY` constant:

```ts
const EMPTY: StatsOverview = {
  departments: 0,
  workers: 0,
  active_workers_today: 0,
  active_workers_yesterday: 0,
  active_workers_change: null,
  messages_received_today: 0,
  messages_sent_today: 0,
  messages_total_today: 0,
  executions_today: { total: 0, success: 0, failed: 0 },
  exec_duration_today_ms: 0,
  exec_duration_yesterday_ms: 0,
  exec_duration_total_ms: 0,
  scheduled_tasks: 0,
}
```

- [ ] **Step 2: Update the import line at the top of `dashboard.tsx`**

Add `formatTotalDuration` to the format import:

```ts
import { formatChange, formatTotalDuration } from "@/lib/format"
```

- [ ] **Step 3: Remove `sessionsToday` from the System Status grid**

In the System Status section, replace the four-item array with three items:

```tsx
[
  { label: t("dashboard.departments"), value: ov.departments },
  { label: t("dashboard.workers"), value: ov.workers },
  { label: t("dashboard.scheduledTasks"), value: ov.scheduled_tasks },
].map(({ label, value }, i) => (
  <div
    key={i}
    className={[
      i % 2 !== 0 ? "pl-6 border-l border-border/70" : "",
      i > 0 ? "sm:pl-8 sm:border-l sm:border-border/70" : "",
      i < 2 ? "pb-6 sm:pb-0" : "",
    ].join(" ")}
    aria-label={label}
  >
    <p className="text-[11px] font-medium uppercase tracking-[0.18em] text-muted-foreground mb-2.5">
      {label}
    </p>
    <p className="text-3xl font-semibold tabular-nums leading-none">{value}</p>
  </div>
))
```

Also update the skeleton count from `length: 4` to `length: 3` and the grid class from `grid-cols-2 sm:grid-cols-4` to `grid-cols-2 sm:grid-cols-3`.

- [ ] **Step 4: Update the Messages card to show total as primary number**

Replace the existing Messages card `div` (the middle column):

```tsx
{/* Messages */}
<div className="p-6">
  <p className="text-[11px] font-medium uppercase tracking-[0.18em] text-muted-foreground mb-5">
    {t("dashboard.messages")}
  </p>
  {isLoading ? (
    <div className="space-y-4">
      <Skeleton className="h-12 w-16" />
      <div className="flex gap-6">
        <StatSkeleton />
        <StatSkeleton />
      </div>
    </div>
  ) : (
    <div>
      <p
        className="text-5xl font-semibold tabular-nums leading-none mb-4"
        aria-label={`${t("dashboard.messages")}: ${ov.messages_total_today}`}
        aria-live="polite"
      >
        {ov.messages_total_today}
      </p>
      <div className="flex gap-6">
        <div>
          <p className="text-[11px] font-medium uppercase tracking-[0.18em] text-muted-foreground mb-2">
            {t("dashboard.messagesReceived")}
          </p>
          <p
            className="text-xl font-semibold tabular-nums leading-none"
            aria-live="polite"
          >
            {ov.messages_received_today}
          </p>
        </div>
        <div>
          <p className="text-[11px] font-medium uppercase tracking-[0.18em] text-muted-foreground mb-2">
            {t("dashboard.messagesSent")}
          </p>
          <p
            className="text-xl font-semibold tabular-nums leading-none"
            aria-live="polite"
          >
            {ov.messages_sent_today}
          </p>
        </div>
      </div>
    </div>
  )}
</div>
```

- [ ] **Step 5: Check TypeScript**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee2/web
npm run build 2>&1 | grep -E "error TS" | head -20
```

Expected: no errors.

- [ ] **Step 6: Commit**

```bash
git add web/src/pages/dashboard.tsx
git commit -m "feat: remove sessions stat, update Messages card to show total"
```

---

## Task 7: Frontend — Execution Duration card and Duration Trend chart

**Files:**
- Modify: `web/src/pages/dashboard.tsx`
- Create: `web/src/components/execution-duration-trend-chart.tsx`

### Steps

- [ ] **Step 1: Add the Execution Duration card to the Today Activity grid**

In `dashboard.tsx`, change the Today Activity grid container class from `sm:grid-cols-3` to `sm:grid-cols-4`.

Then append a new 4th card after the Executions card (closing `</div>` of executions):

```tsx
{/* Execution Duration */}
<div className="p-6">
  <p className="text-[11px] font-medium uppercase tracking-[0.18em] text-muted-foreground mb-5">
    {t("dashboard.executionDuration")}
  </p>
  {isLoading ? (
    <div className="space-y-4">
      <Skeleton className="h-12 w-28" />
      <div className="flex gap-6">
        <StatSkeleton />
        <StatSkeleton />
      </div>
    </div>
  ) : (
    <div>
      <p
        className="text-5xl font-semibold tabular-nums leading-none mb-4"
        aria-label={`${t("dashboard.execDurationToday")}: ${formatTotalDuration(ov.exec_duration_today_ms)}`}
        aria-live="polite"
      >
        {formatTotalDuration(ov.exec_duration_today_ms)}
      </p>
      <div className="flex gap-6">
        <div>
          <p className="text-[11px] font-medium uppercase tracking-[0.18em] text-muted-foreground mb-2">
            {t("dashboard.execDurationYesterday")}
          </p>
          <p
            className="text-xl font-semibold tabular-nums leading-none text-muted-foreground"
            aria-live="polite"
          >
            {formatTotalDuration(ov.exec_duration_yesterday_ms)}
          </p>
        </div>
        <div>
          <p className="text-[11px] font-medium uppercase tracking-[0.18em] text-muted-foreground mb-2">
            {t("dashboard.execDurationTotal")}
          </p>
          <p
            className="text-xl font-semibold tabular-nums leading-none"
            aria-live="polite"
          >
            {formatTotalDuration(ov.exec_duration_total_ms)}
          </p>
        </div>
      </div>
    </div>
  )}
</div>
```

- [ ] **Step 2: Create `execution-duration-trend-chart.tsx`**

Create `web/src/components/execution-duration-trend-chart.tsx`:

```tsx
import { useState } from "react"
import { useTranslation } from "react-i18next"
import {
  LineChart,
  Line,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ResponsiveContainer,
} from "recharts"
import { Card, CardContent, CardHeader } from "@/components/ui/card"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { EmptyState } from "@/components/empty-state"
import { useExecutionDurationTrend } from "@/hooks/use-stats"
import { formatTotalDuration } from "@/lib/format"

const DAY_OPTIONS = [7, 15, 30] as const

export function ExecutionDurationTrendChart() {
  const { t } = useTranslation()
  const [days, setDays] = useState<7 | 15 | 30>(7)
  const { data, isLoading } = useExecutionDurationTrend(days)

  const chartData = data?.data ?? []

  return (
    <Card>
      <CardHeader className="pb-2">
        <div className="flex items-center justify-between gap-3">
          <div className="flex items-center gap-3 min-w-0 flex-1">
            <span className="text-[10px] font-semibold uppercase tracking-[0.16em] text-muted-foreground/70 whitespace-nowrap select-none">
              {t("dashboard.executionDurationTrend")}
            </span>
            <div className="flex-1 h-px bg-border" />
          </div>
          <div className="flex gap-1 shrink-0" role="group" aria-label={t("dashboard.executionDurationTrend")}>
            {DAY_OPTIONS.map((d) => (
              <Button
                key={d}
                variant={days === d ? "default" : "ghost"}
                size="sm"
                className="h-7 px-2 text-xs"
                aria-pressed={days === d}
                aria-label={t("dashboard.daysLabel", { count: d })}
                onClick={() => setDays(d)}
              >
                {d}{t("dashboard.days")}
              </Button>
            ))}
          </div>
        </div>
      </CardHeader>
      <CardContent>
        {isLoading ? (
          <Skeleton className="h-48 w-full" />
        ) : chartData.length === 0 ? (
          <EmptyState
            title={t("dashboard.noExecDurationData")}
            description={t("dashboard.noExecDurationDataDesc")}
          />
        ) : (
          <div
            role="img"
            aria-label={t("dashboard.executionDurationTrendAriaLabel", { days })}
          >
            <ResponsiveContainer width="100%" height={200}>
              <LineChart data={chartData} margin={{ top: 4, right: 8, left: -20, bottom: 0 }}>
                <CartesianGrid strokeDasharray="3 3" className="stroke-border" />
                <XAxis
                  dataKey="date"
                  tick={{ fontSize: 11 }}
                  tickFormatter={(v: string) => v.slice(5)}
                  className="text-muted-foreground"
                />
                <YAxis
                  tick={{ fontSize: 11 }}
                  allowDecimals={false}
                  tickFormatter={(v: number) => `${Math.round(v / 60000)}m`}
                  className="text-muted-foreground"
                />
                <Tooltip
                  labelFormatter={(label) => String(label)}
                  formatter={(value: number) => [formatTotalDuration(value), t("dashboard.executionDuration")]}
                  contentStyle={{
                    background: "var(--card)",
                    border: "1px solid var(--border)",
                    borderRadius: "var(--radius-md)",
                    color: "var(--card-foreground)",
                    fontSize: 12,
                  }}
                />
                <Line
                  type="monotone"
                  dataKey="total_duration_ms"
                  strokeWidth={2}
                  dot={false}
                  stroke="var(--primary)"
                />
              </LineChart>
            </ResponsiveContainer>
          </div>
        )}
      </CardContent>
    </Card>
  )
}
```

- [ ] **Step 3: Import and render `ExecutionDurationTrendChart` in `dashboard.tsx`**

At the top of `dashboard.tsx`, add the import:

```ts
import { ExecutionDurationTrendChart } from "@/components/execution-duration-trend-chart"
```

In the JSX, after `<ActivityTrendChart />`, add:

```tsx
<div className="mt-6">
  <ExecutionDurationTrendChart />
</div>
```

- [ ] **Step 4: Build to verify no type errors**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee2/web
npm run build 2>&1 | grep -E "error TS" | head -20
```

Expected: no TypeScript errors.

- [ ] **Step 5: Run frontend tests**

```bash
npx vitest run 2>&1 | tail -20
```

Expected: all tests pass.

- [ ] **Step 6: Commit**

```bash
git add web/src/pages/dashboard.tsx web/src/components/execution-duration-trend-chart.tsx
git commit -m "feat: add execution duration card and trend chart to dashboard"
```

---

## Task 8: Final verification

### Steps

- [ ] **Step 1: Run full backend test suite**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee2
go test ./... 2>&1 | grep -E "FAIL|ok" | head -30
```

Expected: all packages show `ok`, no `FAIL`.

- [ ] **Step 2: Run full frontend test suite**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee2/web
npx vitest run 2>&1 | tail -10
```

Expected: all tests pass.

- [ ] **Step 3: Build frontend**

```bash
npm run build 2>&1 | tail -10
```

Expected: build completes with no errors.

- [ ] **Step 4: Final commit if anything was missed**

If all tests pass and build succeeds, no additional commit needed. The feature is complete.
