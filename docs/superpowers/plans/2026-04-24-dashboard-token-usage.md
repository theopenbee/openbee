# Dashboard Token Usage Stats Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add token consumption stats (total, today, yesterday, 7/15/30-day trend) to the dashboard page.

**Architecture:** Extend `StatsStore.GetOverview()` with three concurrent token SQL queries against `bee_token_stats`, keyed by session via `bee_executions.completed_at`. Add a `GetTokenTrend()` method + new API endpoint. On the frontend, extend the types/API/hook, add a `TokenTrendChart` component, and update the dashboard to show a new 6th system stat, a 5th activity card, and a side-by-side chart layout.

**Tech Stack:** Go (SQLite, `errgroup`), React, TypeScript, recharts, react-i18next, TailwindCSS

---

## File Map

| File | Action | Responsibility |
|------|--------|---------------|
| `internal/infra/store/stats_store.go` | Modify | Add token fields to `StatsOverview`; add token goroutines to `GetOverview`; add `TokenTrendPoint` type and `GetTokenTrend` method |
| `internal/infra/store/stats_store_test.go` | Modify | Tests for token overview queries and token trend |
| `internal/api/stats_handler.go` | Modify | Add `GetTokenTrend` handler method |
| `internal/api/stats_handler_test.go` | Modify | Tests for token trend endpoint |
| `internal/routes/api.go` | Modify | Register `GET /stats/token-trend` route |
| `web/src/lib/types.ts` | Modify | Extend `StatsOverview`; add `TokenTrendPoint` and `TokenTrend` |
| `web/src/lib/api.ts` | Modify | Add `stats.tokenTrend` call |
| `web/src/hooks/use-stats.ts` | Modify | Add `useTokenTrend` hook |
| `web/src/components/token-trend-chart.tsx` | Create | Token trend line chart component |
| `web/src/pages/dashboard.tsx` | Modify | System Status (6th stat), Today Activity (5th card), side-by-side charts |
| `web/src/locales/en.json` | Modify | English i18n keys |
| `web/src/locales/zh.json` | Modify | Chinese i18n keys |

---

### Task 1: Extend `StatsOverview` with token fields and add overview queries

**Files:**
- Modify: `internal/infra/store/stats_store.go`
- Modify: `internal/infra/store/stats_store_test.go`

- [ ] **Step 1: Write the failing test**

Add this test to `internal/infra/store/stats_store_test.go`:

```go
func TestStatsStore_GetOverview_TokenStats(t *testing.T) {
	ss, ws, _, _, _, _, cleanup := newStatsTestDB(t)
	defer cleanup()
	ctx := context.Background()

	ws.Create(model.Worker{Name: "W1", WorkDir: "/tmp/w1"})
	db := ss.db

	todayStart, todayEnd := dayBounds(0)
	yestStart, _ := dayBounds(-1)
	todayMid := (todayStart + todayEnd) / 2
	yestMid := yestStart + 1000

	// Session A: execution completed today
	if _, err := db.Exec(`INSERT INTO bee_executions
		(id,worker_id,session_id,trigger_input,status,result,ai_process_pid,started_at,completed_at)
		VALUES (?,?,?,?,?,?,?,?,?)`,
		uuid.New().String(), "w1", "sess-today", "hi", "completed", "", 0, todayMid-100, todayMid); err != nil {
		t.Fatalf("insert exec: %v", err)
	}
	// Session B: execution completed yesterday
	if _, err := db.Exec(`INSERT INTO bee_executions
		(id,worker_id,session_id,trigger_input,status,result,ai_process_pid,started_at,completed_at)
		VALUES (?,?,?,?,?,?,?,?,?)`,
		uuid.New().String(), "w1", "sess-yest", "hi", "completed", "", 0, yestMid-100, yestMid); err != nil {
		t.Fatalf("insert exec: %v", err)
	}

	// Token stats: sess-today: 100 total (60 input, 40 output)
	if _, err := db.Exec(`INSERT INTO bee_token_stats
		(id,session_id,agent_type,model,input_tokens,output_tokens,cache_creation_tokens,cache_read_tokens,total_tokens,synced_at)
		VALUES (?,?,?,?,?,?,?,?,?,?)`,
		uuid.New().String(), "sess-today", "bee", "claude-3", 60, 40, 0, 0, 100, todayMid); err != nil {
		t.Fatalf("insert token stats: %v", err)
	}
	// Token stats: sess-yest: 200 total (120 input, 80 output)
	if _, err := db.Exec(`INSERT INTO bee_token_stats
		(id,session_id,agent_type,model,input_tokens,output_tokens,cache_creation_tokens,cache_read_tokens,total_tokens,synced_at)
		VALUES (?,?,?,?,?,?,?,?,?,?)`,
		uuid.New().String(), "sess-yest", "bee", "claude-3", 120, 80, 0, 0, 200, yestMid); err != nil {
		t.Fatalf("insert token stats: %v", err)
	}

	ov, err := ss.GetOverview(ctx)
	if err != nil {
		t.Fatalf("GetOverview: %v", err)
	}

	if ov.TokensTotal != 300 {
		t.Errorf("TokensTotal: want 300, got %d", ov.TokensTotal)
	}
	if ov.TokensTodayTotal != 100 {
		t.Errorf("TokensTodayTotal: want 100, got %d", ov.TokensTodayTotal)
	}
	if ov.TokensTodayInput != 60 {
		t.Errorf("TokensTodayInput: want 60, got %d", ov.TokensTodayInput)
	}
	if ov.TokensTodayOutput != 40 {
		t.Errorf("TokensTodayOutput: want 40, got %d", ov.TokensTodayOutput)
	}
	if ov.TokensYestTotal != 200 {
		t.Errorf("TokensYestTotal: want 200, got %d", ov.TokensYestTotal)
	}
	if ov.TokensYestInput != 120 {
		t.Errorf("TokensYestInput: want 120, got %d", ov.TokensYestInput)
	}
	if ov.TokensYestOutput != 80 {
		t.Errorf("TokensYestOutput: want 80, got %d", ov.TokensYestOutput)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee2
go test ./internal/infra/store/... -run TestStatsStore_GetOverview_TokenStats -v
```

Expected: FAIL — `ov.TokensTotal` field does not exist (compile error).

- [ ] **Step 3: Add token fields to `StatsOverview` struct**

In `internal/infra/store/stats_store.go`, extend the `StatsOverview` struct. Append after `ScheduledTasks`:

```go
// Token usage stats — attributed by bee_executions.completed_at
TokensTotal     int64 `json:"tokens_total"`
TokensTodayTotal int64 `json:"tokens_today_total"`
TokensTodayInput int64 `json:"tokens_today_input"`
TokensTodayOutput int64 `json:"tokens_today_output"`
TokensYestTotal  int64 `json:"tokens_yesterday_total"`
TokensYestInput  int64 `json:"tokens_yesterday_input"`
TokensYestOutput int64 `json:"tokens_yesterday_output"`
```

- [ ] **Step 4: Add token queries to `GetOverview()`**

In `internal/infra/store/stats_store.go`, inside `GetOverview()`, add three new `eg.Go` calls before `if err := eg.Wait()`. Use `mu` for the cross-goroutine writes. Add after the scheduled tasks goroutine:

```go
eg.Go(func() error {
	return s.db.QueryRowContext(egc,
		`SELECT COALESCE(SUM(total_tokens),0),
		        COALESCE(SUM(input_tokens),0),
		        COALESCE(SUM(output_tokens),0)
		 FROM bee_token_stats`,
	).Scan(&ov.TokensTotal, &ov.TokensTodayInput, &ov.TokensTodayOutput)
	// Reuse today fields as temporaries — overwritten by next goroutines.
	// Actually scan into dedicated locals:
})
```

Wait — scan all-time totals into separate locals to avoid races. Here is the correct implementation: declare three locals before the goroutines and scan into them, then assign after `eg.Wait()`. Replace the above stub with the complete, correct addition below.

Add these variable declarations inside `GetOverview()`, right after `var globalReceived, globalSent int` (around line 115):

```go
var (
	tokensTotal, tokensTotalInput, tokensTotalOutput         int64
	tokensTodayTotal, tokensTodayInput, tokensTodayOutput    int64
	tokensYestTotal, tokensYestInput, tokensYestOutput       int64
)
```

Then add three goroutines before `if err := eg.Wait()`:

```go
eg.Go(func() error {
	return s.db.QueryRowContext(egc,
		`SELECT COALESCE(SUM(total_tokens),0),
		        COALESCE(SUM(input_tokens),0),
		        COALESCE(SUM(output_tokens),0)
		 FROM bee_token_stats`,
	).Scan(&tokensTotal, &tokensTotalInput, &tokensTotalOutput)
})

eg.Go(func() error {
	return s.db.QueryRowContext(egc,
		`SELECT COALESCE(SUM(ts.total_tokens),0),
		        COALESCE(SUM(ts.input_tokens),0),
		        COALESCE(SUM(ts.output_tokens),0)
		 FROM bee_token_stats ts
		 WHERE ts.session_id IN (
		   SELECT DISTINCT session_id FROM bee_executions
		   WHERE completed_at >= ? AND completed_at < ?
		     AND session_id IS NOT NULL
		 )`,
		todayStart, todayEnd,
	).Scan(&tokensTodayTotal, &tokensTodayInput, &tokensTodayOutput)
})

eg.Go(func() error {
	_, yestEnd := dayBounds(-1)
	yestStart, _ := dayBounds(-1)
	return s.db.QueryRowContext(egc,
		`SELECT COALESCE(SUM(ts.total_tokens),0),
		        COALESCE(SUM(ts.input_tokens),0),
		        COALESCE(SUM(ts.output_tokens),0)
		 FROM bee_token_stats ts
		 WHERE ts.session_id IN (
		   SELECT DISTINCT session_id FROM bee_executions
		   WHERE completed_at >= ? AND completed_at < ?
		     AND session_id IS NOT NULL
		 )`,
		yestStart, yestEnd,
	).Scan(&tokensYestTotal, &tokensYestInput, &tokensYestOutput)
})
```

Then after `if err := eg.Wait()` and before `return ov, nil`, assign:

```go
ov.TokensTotal = tokensTotal
// tokensTotalInput/Output are informational only; not in StatsOverview for now.
// (all-time input/output not shown in spec)
ov.TokensTodayTotal = tokensTodayTotal
ov.TokensTodayInput = tokensTodayInput
ov.TokensTodayOutput = tokensTodayOutput
ov.TokensYestTotal = tokensYestTotal
ov.TokensYestInput = tokensYestInput
ov.TokensYestOutput = tokensYestOutput
```

Wait — the spec actually says `TokensTotal` in overview needs all-time input/output for a hover tooltip. Update `StatsOverview` to also include `TokensTotalInput` and `TokensTotalOutput`:

Add two more fields to the struct:

```go
TokensTotalInput  int64 `json:"tokens_total_input"`
TokensTotalOutput int64 `json:"tokens_total_output"`
```

And in the assignment block:

```go
ov.TokensTotal = tokensTotal
ov.TokensTotalInput = tokensTotalInput
ov.TokensTotalOutput = tokensTotalOutput
```

Also fix the Yesterday query — `dayBounds(-1)` returns `(startMS, endMS)` so the variable order matters. Replace the yesterday goroutine with:

```go
eg.Go(func() error {
	yestStart, yestEnd := dayBounds(-1)
	return s.db.QueryRowContext(egc,
		`SELECT COALESCE(SUM(ts.total_tokens),0),
		        COALESCE(SUM(ts.input_tokens),0),
		        COALESCE(SUM(ts.output_tokens),0)
		 FROM bee_token_stats ts
		 WHERE ts.session_id IN (
		   SELECT DISTINCT session_id FROM bee_executions
		   WHERE completed_at >= ? AND completed_at < ?
		     AND session_id IS NOT NULL
		 )`,
		yestStart, yestEnd,
	).Scan(&tokensYestTotal, &tokensYestInput, &tokensYestOutput)
})
```

The `tokensTotalInput`/`tokensTotalOutput` variables scanned in the first goroutine are safe because they're written in their own goroutine (no other goroutine touches them), and read only after `eg.Wait()`.

Update the `StatsOverview` struct to have all 9 fields total:

```go
TokensTotal       int64 `json:"tokens_total"`
TokensTotalInput  int64 `json:"tokens_total_input"`
TokensTotalOutput int64 `json:"tokens_total_output"`
TokensTodayTotal  int64 `json:"tokens_today_total"`
TokensTodayInput  int64 `json:"tokens_today_input"`
TokensTodayOutput int64 `json:"tokens_today_output"`
TokensYestTotal   int64 `json:"tokens_yesterday_total"`
TokensYestInput   int64 `json:"tokens_yesterday_input"`
TokensYestOutput  int64 `json:"tokens_yesterday_output"`
```

Update the test to also check `TokensTotalInput` and `TokensTotalOutput`:

```go
if ov.TokensTotalInput != 180 {
	t.Errorf("TokensTotalInput: want 180, got %d", ov.TokensTotalInput)
}
if ov.TokensTotalOutput != 120 {
	t.Errorf("TokensTotalOutput: want 120, got %d", ov.TokensTotalOutput)
}
```

- [ ] **Step 5: Run tests to verify they pass**

```bash
go test ./internal/infra/store/... -run TestStatsStore_GetOverview -v
```

Expected: All `TestStatsStore_GetOverview_*` tests PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/infra/store/stats_store.go internal/infra/store/stats_store_test.go
git commit -m "feat(tokenstat): add token usage fields to StatsOverview"
```

---

### Task 2: Add `GetTokenTrend` to `StatsStore`

**Files:**
- Modify: `internal/infra/store/stats_store.go`
- Modify: `internal/infra/store/stats_store_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/infra/store/stats_store_test.go`:

```go
func TestStatsStore_GetTokenTrend_FillsMissingDays(t *testing.T) {
	ss, _, _, _, _, _, cleanup := newStatsTestDB(t)
	defer cleanup()
	ctx := context.Background()
	db := ss.db

	// Insert an execution completed 2 days ago with 2 models in token stats
	twoDaysAgo := time.Now().AddDate(0, 0, -2).UnixMilli()
	sessID := "sess-trend-1"
	if _, err := db.Exec(`INSERT INTO bee_executions
		(id,worker_id,session_id,trigger_input,status,result,ai_process_pid,started_at,completed_at)
		VALUES (?,?,?,?,?,?,?,?,?)`,
		uuid.New().String(), "w1", sessID, "hi", "completed", "", 0, twoDaysAgo-100, twoDaysAgo); err != nil {
		t.Fatalf("insert exec: %v", err)
	}
	// Two model rows for the same session — should count once per day, summed
	for _, row := range []struct{ model string; tokens int64 }{
		{"claude-3", 300},
		{"claude-3.5", 200},
	} {
		if _, err := db.Exec(`INSERT INTO bee_token_stats
			(id,session_id,agent_type,model,input_tokens,output_tokens,cache_creation_tokens,cache_read_tokens,total_tokens,synced_at)
			VALUES (?,?,?,?,?,?,?,?,?,?)`,
			uuid.New().String(), sessID, "bee", row.model, 0, 0, 0, 0, row.tokens, twoDaysAgo); err != nil {
			t.Fatalf("insert token stats: %v", err)
		}
	}

	points, err := ss.GetTokenTrend(ctx, 7)
	if err != nil {
		t.Fatalf("GetTokenTrend: %v", err)
	}

	if len(points) != 7 {
		t.Fatalf("want 7 points, got %d", len(points))
	}

	target := time.Now().AddDate(0, 0, -2).Format("2006-01-02")
	found := false
	for _, p := range points {
		if p.Date == target {
			found = true
			if p.TotalTokens != 500 {
				t.Errorf("date %s: want TotalTokens=500, got %d", target, p.TotalTokens)
			}
		} else {
			if p.TotalTokens != 0 {
				t.Errorf("date %s: want 0, got %d", p.Date, p.TotalTokens)
			}
		}
	}
	if !found {
		t.Errorf("date %s not found in trend points", target)
	}
}

func TestStatsStore_GetTokenTrend_MultipleExecutionsSameDay_NoDuplication(t *testing.T) {
	ss, _, _, _, _, _, cleanup := newStatsTestDB(t)
	defer cleanup()
	ctx := context.Background()
	db := ss.db

	// Session with TWO executions both completed today — tokens should be counted once
	todayStart, todayEnd := dayBounds(0)
	todayMid := (todayStart + todayEnd) / 2
	sessID := "sess-dedup"
	for i, offset := range []int64{1000, 2000} {
		if _, err := db.Exec(`INSERT INTO bee_executions
			(id,worker_id,session_id,trigger_input,status,result,ai_process_pid,started_at,completed_at)
			VALUES (?,?,?,?,?,?,?,?,?)`,
			uuid.New().String()+"x"+strconv.Itoa(i), "w1", sessID, "hi", "completed", "", 0,
			todayMid-100, todayMid+offset); err != nil {
			t.Fatalf("insert exec: %v", err)
		}
	}
	if _, err := db.Exec(`INSERT INTO bee_token_stats
		(id,session_id,agent_type,model,input_tokens,output_tokens,cache_creation_tokens,cache_read_tokens,total_tokens,synced_at)
		VALUES (?,?,?,?,?,?,?,?,?,?)`,
		uuid.New().String(), sessID, "bee", "claude-3", 0, 0, 0, 0, 1000, todayMid); err != nil {
		t.Fatalf("insert token stats: %v", err)
	}

	points, err := ss.GetTokenTrend(ctx, 7)
	if err != nil {
		t.Fatalf("GetTokenTrend: %v", err)
	}
	today := time.Now().Format("2006-01-02")
	for _, p := range points {
		if p.Date == today && p.TotalTokens != 1000 {
			t.Errorf("today: want TotalTokens=1000 (no duplication), got %d", p.TotalTokens)
		}
	}
}
```

Add `"strconv"` to the imports in `stats_store_test.go`.

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/infra/store/... -run TestStatsStore_GetTokenTrend -v
```

Expected: FAIL — `ss.GetTokenTrend` undefined.

- [ ] **Step 3: Add `TokenTrendPoint` type and `GetTokenTrend` method**

Append to `internal/infra/store/stats_store.go`:

```go
// TokenTrendPoint is one day's total token usage.
type TokenTrendPoint struct {
	Date        string `json:"date"`
	TotalTokens int64  `json:"total_tokens"`
}

// GetTokenTrend returns total token usage per day for the last `days` days,
// attributed by bee_executions.completed_at. Sessions with multiple executions
// on the same day are counted once per day. Zero-fills missing days.
func (s *StatsStore) GetTokenTrend(ctx context.Context, days int) ([]TokenTrendPoint, error) {
	startOfRange, startMS, endMS := trendRange(days)

	rows, err := s.db.QueryContext(ctx, `
		SELECT day, COALESCE(SUM(ts.total_tokens), 0) AS tokens
		FROM (
		  SELECT DISTINCT session_id,
		         DATE(completed_at/1000, 'unixepoch', 'localtime') AS day
		  FROM bee_executions
		  WHERE completed_at >= ? AND completed_at < ?
		    AND session_id IS NOT NULL
		) sessions
		JOIN bee_token_stats ts ON ts.session_id = sessions.session_id
		GROUP BY day
		ORDER BY day ASC`, startMS, endMS)
	if err != nil {
		return nil, fmt.Errorf("token trend query: %w", err)
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
		return nil, fmt.Errorf("token trend rows: %w", err)
	}

	points := make([]TokenTrendPoint, days)
	for i := range days {
		date := startOfRange.AddDate(0, 0, i).Format("2006-01-02")
		points[i] = TokenTrendPoint{Date: date, TotalTokens: dbTotals[date]}
	}
	return points, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/infra/store/... -run TestStatsStore_GetTokenTrend -v
```

Expected: PASS.

- [ ] **Step 5: Run all stats store tests**

```bash
go test ./internal/infra/store/... -v
```

Expected: All tests PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/infra/store/stats_store.go internal/infra/store/stats_store_test.go
git commit -m "feat(tokenstat): add GetTokenTrend to StatsStore"
```

---

### Task 3: Add `GetTokenTrend` handler and register route

**Files:**
- Modify: `internal/api/stats_handler.go`
- Modify: `internal/api/stats_handler_test.go`
- Modify: `internal/routes/api.go`

- [ ] **Step 1: Write the failing handler test**

Add to `internal/api/stats_handler_test.go`:

```go
func TestGetTokenTrend_ValidDays(t *testing.T) {
	router, _, cleanup := newTestServerWithStats(t)
	defer cleanup()

	for _, days := range []int{7, 15, 30} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/stats/token-trend?days="+strconv.Itoa(days), nil)
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
		for _, pt := range resp.Data {
			if _, ok := pt["total_tokens"]; !ok {
				t.Errorf("days=%d: point missing total_tokens: %v", days, pt)
			}
		}
	}
}

func TestGetTokenTrend_InvalidDays_Returns400(t *testing.T) {
	router, _, cleanup := newTestServerWithStats(t)
	defer cleanup()

	for _, bad := range []string{"99", "0", "abc", "-1"} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/stats/token-trend?days="+bad, nil)
		router.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("days=%q: expected 400, got %d", bad, w.Code)
		}
	}
}
```

Update `newTestServerWithStats` in `stats_handler_test.go` to register the new route:

```go
api.GET("/stats/token-trend", h.GetTokenTrend)
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/api/... -run TestGetTokenTrend -v
```

Expected: FAIL — `h.GetTokenTrend` undefined.

- [ ] **Step 3: Add `GetTokenTrend` handler**

Append to `internal/api/stats_handler.go`:

```go
func (h *StatsHandler) GetTokenTrend(c *gin.Context) {
	days, ok := parseDaysParam(c)
	if !ok {
		return
	}

	points, err := h.stats.GetTokenTrend(c.Request.Context(), days)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"days": days, "data": points})
}
```

- [ ] **Step 4: Register route in `internal/routes/api.go`**

After the line `r.GET("/stats/execution-duration-trend", s.Stats.GetExecutionDurationTrend)`, add:

```go
r.GET("/stats/token-trend", s.Stats.GetTokenTrend)
```

- [ ] **Step 5: Run tests to verify they pass**

```bash
go test ./internal/api/... -run TestGetTokenTrend -v
```

Expected: All `TestGetTokenTrend_*` tests PASS.

- [ ] **Step 6: Run all backend tests**

```bash
go test ./... 2>&1 | tail -20
```

Expected: All tests PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/api/stats_handler.go internal/api/stats_handler_test.go internal/routes/api.go
git commit -m "feat(tokenstat): add /stats/token-trend endpoint"
```

---

### Task 4: Update frontend types, API client, and hook

**Files:**
- Modify: `web/src/lib/types.ts`
- Modify: `web/src/lib/api.ts`
- Modify: `web/src/hooks/use-stats.ts`

- [ ] **Step 1: Extend `StatsOverview` in `types.ts`**

In `web/src/lib/types.ts`, find the `StatsOverview` interface (line ~108) and add after `scheduled_tasks: number`:

```ts
tokens_total: number
tokens_total_input: number
tokens_total_output: number
tokens_today_total: number
tokens_today_input: number
tokens_today_output: number
tokens_yesterday_total: number
tokens_yesterday_input: number
tokens_yesterday_output: number
```

Then add the new trend types after the `ExecDurationTrend` interface:

```ts
export interface TokenTrendPoint {
  date: string
  total_tokens: number
}

export interface TokenTrend {
  days: number
  data: TokenTrendPoint[]
}
```

- [ ] **Step 2: Add `stats.tokenTrend` to API client**

In `web/src/lib/api.ts`, inside the `stats` object (after the `executionDurationTrend` entry), add:

```ts
tokenTrend: (days: 7 | 15 | 30) =>
  fetchAPI<TokenTrend>(`/stats/token-trend?days=${days}`),
```

Add `TokenTrend` to the import from `@/lib/types` if needed (check existing imports).

- [ ] **Step 3: Add `useTokenTrend` hook**

In `web/src/hooks/use-stats.ts`, add:

```ts
export const useTokenTrend = (days: 7 | 15 | 30) =>
  useStatsDayTrend("token-trend", api.stats.tokenTrend, days)
```

- [ ] **Step 4: Verify TypeScript compiles**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee2/web
npm run type-check 2>&1 | tail -20
```

Expected: No type errors.

- [ ] **Step 5: Commit**

```bash
git add web/src/lib/types.ts web/src/lib/api.ts web/src/hooks/use-stats.ts
git commit -m "feat(tokenstat): add frontend types, API client, and hook for token trend"
```

---

### Task 5: Create `TokenTrendChart` component

**Files:**
- Create: `web/src/components/token-trend-chart.tsx`

- [ ] **Step 1: Create the component**

Create `web/src/components/token-trend-chart.tsx`:

```tsx
import { useState } from "react"
import { useTranslation } from "react-i18next"
import { TrendLineCard } from "@/components/trend-line-card"
import { useTokenTrend } from "@/hooks/use-stats"
import { formatTokenCount } from "@/lib/format"

export function TokenTrendChart() {
  const { t } = useTranslation()
  const [days, setDays] = useState<7 | 15 | 30>(7)
  const { data, isLoading } = useTokenTrend(days)

  const chartData = (data?.data ?? []).map((p) => ({
    date: p.date,
    total_tokens: p.total_tokens,
  }))

  return (
    <TrendLineCard
      title={t("dashboard.tokensTrend")}
      ariaLabel={t("dashboard.tokensTrendAriaLabel", { days })}
      emptyTitle={t("dashboard.noTokenData")}
      emptyDesc={t("dashboard.noTokenDataDesc")}
      dataKey="total_tokens"
      tooltipLabel={t("dashboard.tokens")}
      chartData={chartData}
      isLoading={isLoading}
      days={days}
      onDaysChange={setDays}
      yAxisFormatter={(v) => formatTokenCount(v)}
      tooltipFormatter={(v) => formatTokenCount(v)}
    />
  )
}
```

- [ ] **Step 2: Verify TypeScript compiles**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee2/web
npm run type-check 2>&1 | tail -20
```

Expected: No type errors.

- [ ] **Step 3: Commit**

```bash
git add web/src/components/token-trend-chart.tsx
git commit -m "feat(tokenstat): add TokenTrendChart component"
```

---

### Task 6: Add i18n keys

**Files:**
- Modify: `web/src/locales/en.json`
- Modify: `web/src/locales/zh.json`

- [ ] **Step 1: Add English keys**

In `web/src/locales/en.json`, inside the `"dashboard"` object, append before the closing `}`:

```json
"totalTokens": "Total Tokens",
"tokens": "Tokens",
"tokensToday": "Tokens Today",
"tokensYesterday": "Yesterday",
"tokensTodayInput": "Input",
"tokensTodayOutput": "Output",
"tokensTrend": "Token Usage Trend",
"tokensTrendAriaLabel": "Token usage trend for the last {{days}} days",
"tokensCrossDayNote": "Sessions active across midnight are counted in each day",
"noTokenData": "No token data",
"noTokenDataDesc": "Token usage trend will appear here once sessions have completed."
```

- [ ] **Step 2: Add Chinese keys**

In `web/src/locales/zh.json`, inside the `"dashboard"` object, append the same keys with Chinese values:

```json
"totalTokens": "总 Token 用量",
"tokens": "Token 用量",
"tokensToday": "今日 Token",
"tokensYesterday": "昨日",
"tokensTodayInput": "输入",
"tokensTodayOutput": "输出",
"tokensTrend": "Token 使用趋势",
"tokensTrendAriaLabel": "最近 {{days}} 天 Token 使用趋势",
"tokensCrossDayNote": "跨天活跃的会话会在每天中分别记录",
"noTokenData": "暂无 Token 数据",
"noTokenDataDesc": "有会话完成执行后，Token 趋势将显示在此处。"
```

- [ ] **Step 3: Commit**

```bash
git add web/src/locales/en.json web/src/locales/zh.json
git commit -m "i18n: add dashboard token usage translation keys"
```

---

### Task 7: Update dashboard page

**Files:**
- Modify: `web/src/pages/dashboard.tsx`

- [ ] **Step 1: Update imports and `EMPTY` constant**

At the top of `web/src/pages/dashboard.tsx`, add `Info` to the lucide-react import and add `TokenTrendChart` import:

```tsx
import { TrendingUp, TrendingDown, Minus, Info } from "lucide-react"
import { TokenTrendChart } from "@/components/token-trend-chart"
```

Also add `Tooltip, TooltipContent, TooltipTrigger` from the UI tooltip component (check existing imports — if `@/components/ui/tooltip` is not imported, add it):

```tsx
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
```

Update the `EMPTY` constant to include all 9 new token fields:

```tsx
const EMPTY: StatsOverview = {
  // ...existing fields...
  tokens_total: 0,
  tokens_total_input: 0,
  tokens_total_output: 0,
  tokens_today_total: 0,
  tokens_today_input: 0,
  tokens_today_output: 0,
  tokens_yesterday_total: 0,
  tokens_yesterday_input: 0,
  tokens_yesterday_output: 0,
}
```

- [ ] **Step 2: Add token day-over-day computation**

In the `Dashboard` function body, after the `durationChangeColor` declaration, add:

```tsx
const tokenDiff = ov.tokens_today_total - ov.tokens_yesterday_total
const tokenRatio =
  ov.tokens_yesterday_total > 0 ? tokenDiff / ov.tokens_yesterday_total : null
const tokenChangeLabel = formatChange(tokenRatio)
const tokenChangeColor =
  tokenDiff > 0
    ? "text-status-idle"
    : tokenDiff < 0
      ? "text-status-error"
      : "text-muted-foreground"
```

- [ ] **Step 3: Update System Status section (5 → 6 items)**

Replace the System Status grid `className`:

```tsx
// Before:
<div className="grid grid-cols-2 sm:grid-cols-5">

// After:
<div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-6">
```

Replace the items array (the `[{ label, value }, ...]` array passed to `.map()`) with one that includes Total Tokens as the 6th item. The last item uses a custom render, so replace the entire static items block with:

```tsx
{isLoading ? (
  Array.from({ length: 6 }).map((_, i) => (
    <div
      key={i}
      className={[
        i % 2 !== 0 ? "pl-6 border-l border-border/70" : "",
        i > 0 ? "sm:pl-8 sm:border-l sm:border-border/70" : "",
        i < 5 ? "pb-6 sm:pb-0" : "",
      ].join(" ")}
    >
      <StatSkeleton />
    </div>
  ))
) : (
  <>
    {[
      { label: t("dashboard.departments"), value: ov.departments },
      { label: t("dashboard.workers"), value: ov.workers },
      { label: t("dashboard.scheduledTasks"), value: ov.scheduled_tasks },
      { label: t("dashboard.totalMessages"), value: ov.messages_total_global },
      {
        label: t("dashboard.totalWorkDuration"),
        value: formatTotalDuration(ov.exec_duration_total_ms),
      },
    ].map(({ label, value }, i) => (
      <div
        key={i}
        className={[
          i % 2 !== 0 ? "pl-6 border-l border-border/70" : "",
          i > 0 ? "sm:pl-8 sm:border-l sm:border-border/70" : "",
          i < 5 ? "pb-6 lg:pb-0" : "",
        ].join(" ")}
        aria-label={label}
      >
        <p className="text-[11px] font-medium uppercase tracking-[0.18em] text-muted-foreground mb-2.5">
          {label}
        </p>
        <p className="text-3xl font-semibold tabular-nums leading-none">{value}</p>
      </div>
    ))}
    {/* Total Tokens — with hover tooltip */}
    <div
      className="pl-6 border-l border-border/70 sm:pl-8 sm:border-l sm:border-border/70"
      aria-label={t("dashboard.totalTokens")}
    >
      <p className="text-[11px] font-medium uppercase tracking-[0.18em] text-muted-foreground mb-2.5">
        {t("dashboard.totalTokens")}
      </p>
      <Tooltip>
        <TooltipTrigger asChild>
          <p className="text-3xl font-semibold tabular-nums leading-none cursor-default">
            {formatTokenCount(ov.tokens_total)}
          </p>
        </TooltipTrigger>
        <TooltipContent>
          <p>{t("dashboard.tokensTodayInput")}: {ov.tokens_total_input.toLocaleString()}</p>
          <p>{t("dashboard.tokensTodayOutput")}: {ov.tokens_total_output.toLocaleString()}</p>
        </TooltipContent>
      </Tooltip>
    </div>
  </>
)}
```

- [ ] **Step 4: Update Today Activity section (4 → 5 cards)**

Change the Today Activity inner grid from `sm:grid-cols-4` to `sm:grid-cols-5`:

```tsx
// Before:
<div className="grid grid-cols-1 sm:grid-cols-4 divide-y sm:divide-y-0 sm:divide-x divide-border">

// After:
<div className="grid grid-cols-1 sm:grid-cols-5 divide-y sm:divide-y-0 sm:divide-x divide-border">
```

Append a new fifth card inside the grid, after the closing `</div>` of the Execution Duration card:

```tsx
{/* Tokens */}
<div className="p-6">
  <p className="text-[11px] font-medium uppercase tracking-[0.18em] text-muted-foreground mb-5">
    {t("dashboard.tokensToday")}
  </p>
  {isLoading ? (
    <div className="space-y-4">
      <Skeleton className="h-12 w-16" />
      <div className="flex gap-4">
        <Skeleton className="h-5 w-20" />
        <Skeleton className="h-5 w-12" />
      </div>
    </div>
  ) : (
    <div>
      <Tooltip>
        <TooltipTrigger asChild>
          <p
            className="text-5xl font-semibold tabular-nums leading-none mb-4 cursor-default"
            aria-label={`${t("dashboard.tokensToday")}: ${formatTokenCount(ov.tokens_today_total)}`}
            aria-live="polite"
          >
            {formatTokenCount(ov.tokens_today_total)}
          </p>
        </TooltipTrigger>
        <TooltipContent>
          <p>{t("dashboard.tokensTodayInput")}: {ov.tokens_today_input.toLocaleString()}</p>
          <p>{t("dashboard.tokensTodayOutput")}: {ov.tokens_today_output.toLocaleString()}</p>
        </TooltipContent>
      </Tooltip>
      <div className="flex flex-wrap items-center gap-x-5 gap-y-1.5">
        <div className="flex items-baseline gap-2">
          <span className="text-[11px] font-medium uppercase tracking-[0.18em] text-muted-foreground">
            {t("dashboard.tokensYesterday")}
          </span>
          <span className="text-lg font-medium tabular-nums text-muted-foreground leading-none">
            {formatTokenCount(ov.tokens_yesterday_total)}
          </span>
        </div>
        {tokenChangeLabel !== null && (
          <div
            className={`flex items-center gap-1 ${tokenChangeColor}`}
            aria-label={tokenChangeLabel}
          >
            {tokenDiff > 0 ? (
              <TrendingUp className="h-3 w-3" aria-hidden />
            ) : tokenDiff < 0 ? (
              <TrendingDown className="h-3 w-3" aria-hidden />
            ) : (
              <Minus className="h-3 w-3" aria-hidden />
            )}
            <span className="text-xs font-semibold tabular-nums">{tokenChangeLabel}</span>
          </div>
        )}
        <Tooltip>
          <TooltipTrigger asChild>
            <button
              type="button"
              aria-label={t("dashboard.tokensCrossDayNote")}
              className="ml-auto text-muted-foreground/50 hover:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring rounded"
            >
              <Info className="h-3.5 w-3.5" aria-hidden />
            </button>
          </TooltipTrigger>
          <TooltipContent>
            <p>{t("dashboard.tokensCrossDayNote")}</p>
          </TooltipContent>
        </Tooltip>
      </div>
    </div>
  )}
</div>
```

- [ ] **Step 5: Update charts section to side-by-side layout**

Replace the final `<CombinedTrendChart />` line with:

```tsx
{/* ── Charts ─────────────────────────────────────────────── */}
<div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
  <TokenTrendChart />
  <CombinedTrendChart />
</div>
```

- [ ] **Step 6: Add `formatTokenCount` to the imports**

In `web/src/pages/dashboard.tsx`, update the import from `@/lib/format`:

```tsx
import { formatChange, formatTotalDuration, formatTokenCount } from "@/lib/format"
```

- [ ] **Step 7: Verify TypeScript compiles**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee2/web
npm run type-check 2>&1 | tail -20
```

Expected: No type errors.

- [ ] **Step 8: Commit**

```bash
git add web/src/pages/dashboard.tsx
git commit -m "feat(tokenstat): add token usage stats to dashboard page"
```

---

## Self-Review Checklist

### Spec Coverage

| Spec requirement | Task |
|-----------------|------|
| Total all-time tokens in System Status (6th stat) | Task 7 Step 3 |
| Hover on Total Tokens shows input/output | Task 7 Step 3 |
| Today/Yesterday tokens in Today Activity (5th card) | Task 7 Step 4 |
| Hover on today total shows input/output | Task 7 Step 4 |
| Day-over-day comparison + trend icon | Task 7 Step 4 |
| Cross-day sessions note (info icon + tooltip) | Task 7 Step 4 |
| Token trend chart (7/15/30 days) | Task 5 |
| Charts side-by-side (50%/50%) | Task 7 Step 5 |
| `bee_executions.completed_at` as time anchor | Tasks 1, 2 |
| Avoid double-counting multiple executions same day | Task 2 (dedup query) |
| Backend `StatsOverview` 9 token fields | Task 1 |
| Backend `GetTokenTrend` endpoint | Tasks 2, 3 |
| i18n keys (en + zh) | Task 6 |

### Type Consistency

- `StatsOverview` fields: `tokens_total`, `tokens_total_input`, `tokens_total_output`, `tokens_today_total`, `tokens_today_input`, `tokens_today_output`, `tokens_yesterday_total`, `tokens_yesterday_input`, `tokens_yesterday_output` — consistent across Go struct, TS interface, and dashboard usage.
- Go: `TokenTrendPoint.TotalTokens` / JSON `total_tokens` — matches TS `TokenTrendPoint.total_tokens`.
- `formatTokenCount` imported in dashboard: ✓ Task 7 Step 6.
- `TokenTrendChart` imported in dashboard: ✓ Task 7 Step 1.
