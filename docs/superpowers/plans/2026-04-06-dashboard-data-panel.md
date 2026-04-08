# Dashboard Data Panel Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rewrite the dashboard page from a worker card list into a data panel showing departments, workers, active-worker trends, messages, sessions, executions, and scheduled tasks.

**Architecture:** Two new backend API endpoints (`GET /api/stats/overview` and `GET /api/stats/trend`) backed by a new `StatsStore`; the frontend dashboard is rewritten to call these endpoints and render stat cards plus a Recharts line chart.

**Tech Stack:** Go (SQLite via `database/sql`), Gin, React 19, TypeScript, TanStack Query, Recharts, shadcn/ui, Tailwind CSS 4

---

## File Map

| File | Action | Purpose |
|------|--------|---------|
| `internal/infra/store/stats_store.go` | **Create** | All stats DB queries |
| `internal/infra/store/stats_store_test.go` | **Create** | Unit tests for StatsStore |
| `internal/api/stats_handler.go` | **Create** | HTTP handlers for /api/stats/* |
| `internal/api/stats_handler_test.go` | **Create** | Handler tests |
| `internal/api/router.go` | **Modify** | Add StatsStore to ServerParams; register routes |
| `internal/app/app.go` | **Modify** | Add statsStore to appStores; wire into buildAPIServer |
| `web/src/lib/types.ts` | **Modify** | Add StatsOverview and StatsTrend types |
| `web/src/lib/api.ts` | **Modify** | Add stats namespace to api object |
| `web/src/hooks/use-stats.ts` | **Create** | useStatsOverview and useStatsTrend hooks |
| `web/src/components/stat-card.tsx` | **Create** | Generic single-metric card |
| `web/src/components/active-workers-card.tsx` | **Create** | Today/yesterday/change card |
| `web/src/components/messages-card.tsx` | **Create** | Received/sent messages card |
| `web/src/components/executions-card.tsx` | **Create** | Total/success/failed executions card |
| `web/src/components/activity-trend-chart.tsx` | **Create** | Recharts line chart with day-range tabs |
| `web/src/pages/dashboard.tsx` | **Rewrite** | Data panel page |
| `web/src/locales/en.json` | **Modify** | Add dashboard.* i18n keys |
| `web/src/locales/zh.json` | **Modify** | Add dashboard.* i18n keys |

---

### Task 1: StatsStore — DB queries

**Files:**
- Create: `internal/infra/store/stats_store.go`
- Create: `internal/infra/store/stats_store_test.go`

- [ ] **Step 1: Write the failing tests**

```go
// internal/infra/store/stats_store_test.go
package store

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/theopenbee/openbee/internal/infra/model"
)

func newStatsTestDB(t *testing.T) (*StatsStore, *WorkerStore, *ExecutionStore, *MessageStore, *OutboundMessageStore, *TaskStore, func()) {
	t.Helper()
	db, err := InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	return NewStatsStore(db),
		NewWorkerStore(db),
		NewExecutionStore(db, t.TempDir()),
		NewMessageStore(db),
		NewOutboundMessageStore(db),
		NewTaskStore(db),
		func() { db.Close() }
}

func todayStartMS() int64 {
	now := time.Now()
	y, m, d := now.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, now.Location()).UnixMilli()
}

func TestStatsStore_GetOverview_Counts(t *testing.T) {
	ss, ws, es, ms, oms, ts, cleanup := newStatsTestDB(t)
	defer cleanup()
	ctx := context.Background()

	// Create 2 workers
	w1, _ := ws.Create(model.Worker{Name: "W1", WorkDir: "/tmp/w1"})
	w2, _ := ws.Create(model.Worker{Name: "W2", WorkDir: "/tmp/w2"})

	// Both workers have executions today
	todayMS := todayStartMS() + 1000
	db := ss.db
	db.Exec(`INSERT INTO bee_executions (id,worker_id,session_id,trigger_input,status,result,ai_process_pid,started_at) VALUES (?,?,?,?,?,?,?,?)`,
		uuid.New().String(), w1.ID, "sess1", "hi", "completed", "", 0, todayMS)
	db.Exec(`INSERT INTO bee_executions (id,worker_id,session_id,trigger_input,status,result,ai_process_pid,started_at) VALUES (?,?,?,?,?,?,?,?)`,
		uuid.New().String(), w2.ID, "sess2", "hi", "failed", "", 0, todayMS)
	db.Exec(`INSERT INTO bee_executions (id,worker_id,session_id,trigger_input,status,result,ai_process_pid,started_at) VALUES (?,?,?,?,?,?,?,?)`,
		uuid.New().String(), w1.ID, "sess3", "hi", "completed", "", 0, todayMS)

	// One inbound message today
	ms.Create(ctx, uuid.New().String(), "sk1", "feishu", "hello", "{}", "", todayMS)

	// One outbound message today
	db.Exec(`INSERT INTO bee_outbound_messages (id,session_key,platform,content,media_path,status,platform_msg_id,source_type,source_id,inbound_msg_id,error,retry_count,sent_at,created_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		uuid.New().String(), "sk1", "feishu", "reply", "", "sent", "", "worker", w1.ID, "", "", 0, todayMS, todayMS)

	// New session today: first message for "sk1" is today
	// Active countdown task
	now := time.Now().UnixMilli()
	ts.Create(ctx, model.Task{
		MessageID: "m1", WorkerID: w1.ID, Instruction: "count",
		Type: model.TaskTypeCountdown, Status: model.TaskStatusPending,
		CreatedAt: now, UpdatedAt: now,
	})

	ov, err := ss.GetOverview(ctx)
	if err != nil {
		t.Fatalf("GetOverview: %v", err)
	}

	if ov.Workers != 2 {
		t.Errorf("Workers: want 2, got %d", ov.Workers)
	}
	if ov.ActiveWorkersToday != 2 {
		t.Errorf("ActiveWorkersToday: want 2, got %d", ov.ActiveWorkersToday)
	}
	if ov.MessagesReceivedToday != 1 {
		t.Errorf("MessagesReceivedToday: want 1, got %d", ov.MessagesReceivedToday)
	}
	if ov.MessagesSentToday != 1 {
		t.Errorf("MessagesSentToday: want 1, got %d", ov.MessagesSentToday)
	}
	if ov.SessionsNewToday != 1 {
		t.Errorf("SessionsNewToday: want 1, got %d", ov.SessionsNewToday)
	}
	if ov.ExecutionsToday.Total != 3 {
		t.Errorf("ExecutionsToday.Total: want 3, got %d", ov.ExecutionsToday.Total)
	}
	if ov.ExecutionsToday.Success != 2 {
		t.Errorf("ExecutionsToday.Success: want 2, got %d", ov.ExecutionsToday.Success)
	}
	if ov.ExecutionsToday.Failed != 1 {
		t.Errorf("ExecutionsToday.Failed: want 1, got %d", ov.ExecutionsToday.Failed)
	}
	if ov.ScheduledTasks != 1 {
		t.Errorf("ScheduledTasks: want 1, got %d", ov.ScheduledTasks)
	}

	_ = es
	_ = oms
}

func TestStatsStore_GetTrend_FillsMissingDays(t *testing.T) {
	ss, ws, _, _, _, _, cleanup := newStatsTestDB(t)
	defer cleanup()
	ctx := context.Background()

	w1, _ := ws.Create(model.Worker{Name: "W1", WorkDir: "/tmp/w1"})
	db := ss.db

	// Add an execution 3 days ago only
	threeDaysAgo := time.Now().AddDate(0, 0, -3).UnixMilli()
	db.Exec(`INSERT INTO bee_executions (id,worker_id,session_id,trigger_input,status,result,ai_process_pid,started_at) VALUES (?,?,?,?,?,?,?,?)`,
		uuid.New().String(), w1.ID, "sess1", "hi", "completed", "", 0, threeDaysAgo)

	points, err := ss.GetTrend(ctx, 7)
	if err != nil {
		t.Fatalf("GetTrend: %v", err)
	}

	if len(points) != 7 {
		t.Fatalf("want 7 points, got %d", len(points))
	}

	// Find the day 3 days ago
	target := time.Now().AddDate(0, 0, -3).Format("2006-01-02")
	found := false
	for _, p := range points {
		if p.Date == target {
			found = true
			if p.ActiveWorkers != 1 {
				t.Errorf("day %s: want 1 active worker, got %d", target, p.ActiveWorkers)
			}
		} else if p.ActiveWorkers != 0 {
			// Days without executions should be 0
			// (today might have 0 too — that's fine)
		}
	}
	if !found {
		t.Errorf("date %s not found in trend points", target)
	}
}

func TestStatsStore_ActiveWorkersChange_NullWhenYesterdayZero(t *testing.T) {
	ss, ws, _, _, _, _, cleanup := newStatsTestDB(t)
	defer cleanup()
	ctx := context.Background()

	w1, _ := ws.Create(model.Worker{Name: "W1", WorkDir: "/tmp/w1"})
	db := ss.db

	// Only today's execution, none yesterday
	todayMS := todayStartMS() + 1000
	db.Exec(`INSERT INTO bee_executions (id,worker_id,session_id,trigger_input,status,result,ai_process_pid,started_at) VALUES (?,?,?,?,?,?,?,?)`,
		uuid.New().String(), w1.ID, "sess1", "hi", "completed", "", 0, todayMS)

	ov, err := ss.GetOverview(ctx)
	if err != nil {
		t.Fatalf("GetOverview: %v", err)
	}

	if ov.ActiveWorkersChange != nil {
		t.Errorf("ActiveWorkersChange: want nil (yesterday=0), got %v", *ov.ActiveWorkersChange)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd /path/to/openbee
go test ./internal/infra/store/ -run TestStatsStore -v
```

Expected: compilation error — `StatsStore` undefined.

- [ ] **Step 3: Implement StatsStore**

```go
// internal/infra/store/stats_store.go
package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// StatsStore provides aggregated statistics queries.
type StatsStore struct {
	db *sql.DB
}

// NewStatsStore constructs a StatsStore.
func NewStatsStore(db *sql.DB) *StatsStore {
	return &StatsStore{db: db}
}

// ExecStats holds today's execution counts.
type ExecStats struct {
	Total   int `json:"total"`
	Success int `json:"success"`
	Failed  int `json:"failed"`
}

// StatsOverview holds all numeric dashboard card data.
type StatsOverview struct {
	Departments            int      `json:"departments"`
	Workers                int      `json:"workers"`
	ActiveWorkersToday     int      `json:"active_workers_today"`
	ActiveWorkersYesterday int      `json:"active_workers_yesterday"`
	ActiveWorkersChange    *float64 `json:"active_workers_change"`
	MessagesReceivedToday  int      `json:"messages_received_today"`
	MessagesSentToday      int      `json:"messages_sent_today"`
	SessionsNewToday       int      `json:"sessions_new_today"`
	ExecutionsToday        ExecStats `json:"executions_today"`
	ScheduledTasks         int      `json:"scheduled_tasks"`
}

// TrendPoint is one day's data point in the activity trend.
type TrendPoint struct {
	Date          string `json:"date"`
	ActiveWorkers int    `json:"active_workers"`
}

// todayBounds returns Unix-millisecond boundaries for today (local time).
func todayBounds() (startMS, endMS int64) {
	now := time.Now()
	y, m, d := now.Date()
	loc := now.Location()
	start := time.Date(y, m, d, 0, 0, 0, 0, loc)
	end := start.Add(24 * time.Hour)
	return start.UnixMilli(), end.UnixMilli()
}

// yesterdayBounds returns Unix-millisecond boundaries for yesterday (local time).
func yesterdayBounds() (startMS, endMS int64) {
	now := time.Now()
	y, m, d := now.Date()
	loc := now.Location()
	start := time.Date(y, m, d-1, 0, 0, 0, 0, loc)
	end := time.Date(y, m, d, 0, 0, 0, 0, loc)
	return start.UnixMilli(), end.UnixMilli()
}

// GetOverview returns all numeric card statistics.
func (s *StatsStore) GetOverview(ctx context.Context) (StatsOverview, error) {
	var ov StatsOverview

	todayStart, todayEnd := todayBounds()
	yestStart, yestEnd := yesterdayBounds()

	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM bee_departments`).Scan(&ov.Departments); err != nil {
		return ov, fmt.Errorf("count departments: %w", err)
	}

	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM bee_workers`).Scan(&ov.Workers); err != nil {
		return ov, fmt.Errorf("count workers: %w", err)
	}

	activeWorkerQuery := `
		SELECT COUNT(DISTINCT worker_id)
		FROM bee_executions
		WHERE worker_id IS NOT NULL
		  AND started_at >= ? AND started_at < ?`

	if err := s.db.QueryRowContext(ctx, activeWorkerQuery, todayStart, todayEnd).Scan(&ov.ActiveWorkersToday); err != nil {
		return ov, fmt.Errorf("active workers today: %w", err)
	}

	if err := s.db.QueryRowContext(ctx, activeWorkerQuery, yestStart, yestEnd).Scan(&ov.ActiveWorkersYesterday); err != nil {
		return ov, fmt.Errorf("active workers yesterday: %w", err)
	}

	if ov.ActiveWorkersYesterday > 0 {
		change := float64(ov.ActiveWorkersToday-ov.ActiveWorkersYesterday) / float64(ov.ActiveWorkersYesterday)
		ov.ActiveWorkersChange = &change
	}

	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM bee_platform_messages WHERE received_at >= ? AND received_at < ?`,
		todayStart, todayEnd,
	).Scan(&ov.MessagesReceivedToday); err != nil {
		return ov, fmt.Errorf("messages received today: %w", err)
	}

	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM bee_outbound_messages WHERE sent_at >= ? AND sent_at < ?`,
		todayStart, todayEnd,
	).Scan(&ov.MessagesSentToday); err != nil {
		return ov, fmt.Errorf("messages sent today: %w", err)
	}

	if err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM (
			SELECT session_key
			FROM bee_platform_messages
			GROUP BY session_key
			HAVING MIN(received_at) >= ? AND MIN(received_at) < ?
		)`, todayStart, todayEnd,
	).Scan(&ov.SessionsNewToday); err != nil {
		return ov, fmt.Errorf("new sessions today: %w", err)
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT status, COUNT(*) FROM bee_executions
		WHERE worker_id IS NOT NULL
		  AND started_at >= ? AND started_at < ?
		GROUP BY status`, todayStart, todayEnd)
	if err != nil {
		return ov, fmt.Errorf("executions today: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var status string
		var cnt int
		if err := rows.Scan(&status, &cnt); err != nil {
			return ov, err
		}
		ov.ExecutionsToday.Total += cnt
		switch status {
		case "completed":
			ov.ExecutionsToday.Success += cnt
		case "failed":
			ov.ExecutionsToday.Failed += cnt
		}
	}
	if err := rows.Err(); err != nil {
		return ov, fmt.Errorf("executions today rows: %w", err)
	}

	if err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM bee_tasks
		WHERE type IN ('countdown','scheduled')
		  AND status NOT IN ('completed','cancelled','failed')`,
	).Scan(&ov.ScheduledTasks); err != nil {
		return ov, fmt.Errorf("scheduled tasks: %w", err)
	}

	return ov, nil
}

// GetTrend returns active-worker counts for each of the last `days` days (local time),
// filling missing days with zero.
func (s *StatsStore) GetTrend(ctx context.Context, days int) ([]TrendPoint, error) {
	now := time.Now()
	y, m, d := now.Date()
	loc := now.Location()

	startOfToday := time.Date(y, m, d, 0, 0, 0, 0, loc)
	startOfRange := startOfToday.AddDate(0, 0, -(days - 1))
	startMS := startOfRange.UnixMilli()
	endMS := startOfToday.Add(24 * time.Hour).UnixMilli()

	rows, err := s.db.QueryContext(ctx, `
		SELECT DATE(started_at/1000, 'unixepoch', 'localtime') AS day,
		       COUNT(DISTINCT worker_id) AS active
		FROM bee_executions
		WHERE worker_id IS NOT NULL
		  AND started_at >= ? AND started_at < ?
		GROUP BY day
		ORDER BY day ASC`, startMS, endMS)
	if err != nil {
		return nil, fmt.Errorf("trend query: %w", err)
	}
	defer rows.Close()

	dbCounts := make(map[string]int, days)
	for rows.Next() {
		var day string
		var cnt int
		if err := rows.Scan(&day, &cnt); err != nil {
			return nil, err
		}
		dbCounts[day] = cnt
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("trend rows: %w", err)
	}

	points := make([]TrendPoint, days)
	for i := 0; i < days; i++ {
		date := startOfRange.AddDate(0, 0, i).Format("2006-01-02")
		points[i] = TrendPoint{Date: date, ActiveWorkers: dbCounts[date]}
	}
	return points, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/infra/store/ -run TestStatsStore -v
```

Expected: all 3 `TestStatsStore_*` tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/infra/store/stats_store.go internal/infra/store/stats_store_test.go
git commit -m "feat(store): add StatsStore with overview and trend queries"
```

---

### Task 2: stats_handler.go + HTTP tests

**Files:**
- Create: `internal/api/stats_handler.go`
- Create: `internal/api/stats_handler_test.go`

- [ ] **Step 1: Write the failing handler tests**

```go
// internal/api/stats_handler_test.go
package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/infra/store"
)

func newTestServerWithStats(t *testing.T) (*Server, *store.StatsStore, *store.WorkerStore, *sql.DB, func()) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	db, err := store.InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}

	ss := store.NewStatsStore(db)
	ws := store.NewWorkerStore(db)

	router := gin.New()
	s := &Server{
		router: router,
		ServerParams: ServerParams{
			StatsStore: ss,
		},
	}
	s.registerStatsRoutes(router.Group("/api"))
	return s, ss, ws, db, func() { db.Close() }
}

func TestGetStatsOverview_ReturnsOK(t *testing.T) {
	s, _, _, _, cleanup := newTestServerWithStats(t)
	defer cleanup()

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/stats/overview", nil)
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, ok := resp["workers"]; !ok {
		t.Error("response missing 'workers' field")
	}
	if _, ok := resp["executions_today"]; !ok {
		t.Error("response missing 'executions_today' field")
	}
}

func TestGetStatsTrend_ValidDays(t *testing.T) {
	s, _, ws, db, cleanup := newTestServerWithStats(t)
	defer cleanup()

	w1, _ := ws.Create(model.Worker{Name: "W1", WorkDir: "/tmp"})
	todayMS := time.Now().UnixMilli()
	db.Exec(`INSERT INTO bee_executions (id,worker_id,session_id,trigger_input,status,result,ai_process_pid,started_at) VALUES (?,?,?,?,?,?,?,?)`,
		uuid.New().String(), w1.ID, "s1", "hi", "completed", "", 0, todayMS)

	for _, days := range []string{"7", "15", "30"} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/stats/trend?days="+days, nil)
		s.router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("days=%s: expected 200, got %d", days, rec.Code)
		}

		var resp map[string]any
		json.NewDecoder(rec.Body).Decode(&resp)
		data, _ := resp["data"].([]any)
		wantLen := map[string]int{"7": 7, "15": 15, "30": 30}[days]
		if len(data) != wantLen {
			t.Errorf("days=%s: want %d points, got %d", days, wantLen, len(data))
		}
	}
}

func TestGetStatsTrend_InvalidDays_Returns400(t *testing.T) {
	s, _, _, _, cleanup := newTestServerWithStats(t)
	defer cleanup()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/stats/trend?days=99", nil)
	s.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/api/ -run TestGetStats -v
```

Expected: compilation error — `StatsStore`, `registerStatsRoutes` undefined.

- [ ] **Step 3: Implement stats_handler.go**

```go
// internal/api/stats_handler.go
package api

import (
	"context"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/theopenbee/openbee/internal/infra/store"
)

type statsHandlerStore interface {
	GetOverview(ctx context.Context) (store.StatsOverview, error)
	GetTrend(ctx context.Context, days int) ([]store.TrendPoint, error)
}

func (s *Server) registerStatsRoutes(api *gin.RouterGroup) {
	api.GET("/stats/overview", s.getStatsOverview)
	api.GET("/stats/trend", s.getStatsTrend)
}

func (s *Server) getStatsOverview(c *gin.Context) {
	ov, err := s.StatsStore.GetOverview(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, ov)
}

func (s *Server) getStatsTrend(c *gin.Context) {
	daysStr := c.DefaultQuery("days", "7")
	days, err := strconv.Atoi(daysStr)
	if err != nil || (days != 7 && days != 15 && days != 30) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "days must be 7, 15, or 30"})
		return
	}

	points, err := s.StatsStore.GetTrend(c.Request.Context(), days)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"days": days, "data": points})
}
```

- [ ] **Step 4: Add `StatsStore` field to `ServerParams` and register routes**

In `internal/api/router.go`, add the field to `ServerParams`:

```go
// Add after DepartmentStore field:
StatsStore *store.StatsStore
```

In the `setupRoutes` method, inside the JWT-protected `api` group, add:

```go
s.registerStatsRoutes(api)
```

- [ ] **Step 5: Fix the test import — add `database/sql`**

The test file imports `database/sql` for the `*sql.DB` parameter. Verify the import block in `stats_handler_test.go` includes:

```go
import (
    "context"
    "database/sql"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"
    "time"

    "github.com/gin-gonic/gin"
    "github.com/google/uuid"
    "github.com/theopenbee/openbee/internal/infra/model"
    "github.com/theopenbee/openbee/internal/infra/store"
)
```

- [ ] **Step 6: Run tests to verify they pass**

```bash
go test ./internal/api/ -run TestGetStats -v
go test ./internal/infra/store/ -run TestStatsStore -v
```

Expected: all tests PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/api/stats_handler.go internal/api/stats_handler_test.go internal/api/router.go
git commit -m "feat(api): add /api/stats/overview and /api/stats/trend endpoints"
```

---

### Task 3: Wire StatsStore into the app

**Files:**
- Modify: `internal/app/app.go`

- [ ] **Step 1: Add `statsStore` to `appStores` struct**

In `internal/app/app.go`, find the `appStores` struct (around line 165) and add:

```go
statsStore *store.StatsStore
```

- [ ] **Step 2: Initialize `statsStore` in `buildStores`**

In the `buildStores` function return statement, add:

```go
statsStore: store.NewStatsStore(db),
```

- [ ] **Step 3: Pass `statsStore` to `buildAPIServer`**

In `buildAPIServer`, add `StatsStore` to the `api.ServerParams{}` literal:

```go
StatsStore: s.statsStore,
```

- [ ] **Step 4: Verify it compiles**

```bash
go build ./...
```

Expected: no errors.

- [ ] **Step 5: Commit**

```bash
git add internal/app/app.go
git commit -m "feat(app): wire StatsStore into API server"
```

---

### Task 4: Frontend types, API client, and hooks

**Files:**
- Modify: `web/src/lib/types.ts`
- Modify: `web/src/lib/api.ts`
- Create: `web/src/hooks/use-stats.ts`

- [ ] **Step 1: Add types**

In `web/src/lib/types.ts`, append:

```typescript
export interface ExecStats {
  total: number
  success: number
  failed: number
}

export interface StatsOverview {
  departments: number
  workers: number
  active_workers_today: number
  active_workers_yesterday: number
  active_workers_change: number | null
  messages_received_today: number
  messages_sent_today: number
  sessions_new_today: number
  executions_today: ExecStats
  scheduled_tasks: number
}

export interface TrendPoint {
  date: string
  active_workers: number
}

export interface StatsTrend {
  days: number
  data: TrendPoint[]
}
```

- [ ] **Step 2: Add stats to API client**

In `web/src/lib/api.ts`, add `stats` namespace to the `api` object (after the `tasks` entry):

```typescript
stats: {
  overview: () => fetchAPI<StatsOverview>("/stats/overview"),
  trend: (days: 7 | 15 | 30) => fetchAPI<StatsTrend>(`/stats/trend?days=${days}`),
},
```

Also add `StatsOverview` and `StatsTrend` to the import line at the top of `api.ts`:

```typescript
import type { Worker, WorkerExecution, PaginatedResponse, LocalChatSession, ChatMessage, Task, Department, DepartmentTree, StatsOverview, StatsTrend } from "./types"
```

- [ ] **Step 3: Create hooks**

```typescript
// web/src/hooks/use-stats.ts
import { useQuery } from "@tanstack/react-query"
import { api } from "@/lib/api"

export function useStatsOverview() {
  return useQuery({
    queryKey: ["stats", "overview"],
    queryFn: () => api.stats.overview(),
    refetchInterval: 30_000,
  })
}

export function useStatsTrend(days: 7 | 15 | 30) {
  return useQuery({
    queryKey: ["stats", "trend", days],
    queryFn: () => api.stats.trend(days),
  })
}
```

- [ ] **Step 4: Verify TypeScript compiles**

```bash
cd web && npx tsc --noEmit
```

Expected: no errors.

- [ ] **Step 5: Commit**

```bash
git add web/src/lib/types.ts web/src/lib/api.ts web/src/hooks/use-stats.ts
git commit -m "feat(web): add stats types, API client methods, and query hooks"
```

---

### Task 5: StatCard component

**Files:**
- Create: `web/src/components/stat-card.tsx`

- [ ] **Step 1: Create the component**

```tsx
// web/src/components/stat-card.tsx
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Skeleton } from "@/components/ui/skeleton"

interface StatCardProps {
  title: string
  value: number | string
  loading?: boolean
}

export function StatCard({ title, value, loading }: StatCardProps) {
  return (
    <Card>
      <CardHeader className="pb-2">
        <CardTitle className="text-sm font-medium text-muted-foreground">{title}</CardTitle>
      </CardHeader>
      <CardContent>
        {loading ? (
          <Skeleton className="h-8 w-24" />
        ) : (
          <p className="text-3xl font-bold">{value}</p>
        )}
      </CardContent>
    </Card>
  )
}
```

- [ ] **Step 2: Verify it builds**

```bash
cd web && npx tsc --noEmit
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add web/src/components/stat-card.tsx
git commit -m "feat(web): add StatCard component"
```

---

### Task 6: ActiveWorkersCard component

**Files:**
- Create: `web/src/components/active-workers-card.tsx`

- [ ] **Step 1: Create the component**

```tsx
// web/src/components/active-workers-card.tsx
import { useTranslation } from "react-i18next"
import { TrendingUp, TrendingDown, Minus } from "lucide-react"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Skeleton } from "@/components/ui/skeleton"

interface ActiveWorkersCardProps {
  today: number
  yesterday: number
  change: number | null
  loading?: boolean
}

export function ActiveWorkersCard({ today, yesterday, change, loading }: ActiveWorkersCardProps) {
  const { t } = useTranslation()

  const changeLabel = (() => {
    if (change === null) return null
    const pct = (change * 100).toFixed(1)
    return change >= 0 ? `+${pct}%` : `${pct}%`
  })()

  const ChangeIcon = change === null ? null : change > 0 ? TrendingUp : change < 0 ? TrendingDown : Minus
  const changeColor = change === null ? "" : change > 0 ? "text-green-500" : change < 0 ? "text-red-500" : "text-muted-foreground"

  return (
    <Card>
      <CardHeader className="pb-2">
        <CardTitle className="text-sm font-medium text-muted-foreground">
          {t("dashboard.activeWorkers")}
        </CardTitle>
      </CardHeader>
      <CardContent>
        {loading ? (
          <div className="flex gap-6">
            <Skeleton className="h-8 w-16" />
            <Skeleton className="h-8 w-16" />
            <Skeleton className="h-8 w-16" />
          </div>
        ) : (
          <div className="flex items-end gap-6">
            <div>
              <p className="text-xs text-muted-foreground">{t("dashboard.today")}</p>
              <p className="text-3xl font-bold">{today}</p>
            </div>
            <div>
              <p className="text-xs text-muted-foreground">{t("dashboard.yesterday")}</p>
              <p className="text-3xl font-bold text-muted-foreground">{yesterday}</p>
            </div>
            {changeLabel !== null && ChangeIcon && (
              <div className={`flex items-center gap-1 pb-1 ${changeColor}`}>
                <ChangeIcon className="h-4 w-4" />
                <span className="text-sm font-medium">{changeLabel}</span>
              </div>
            )}
          </div>
        )}
      </CardContent>
    </Card>
  )
}
```

- [ ] **Step 2: Verify**

```bash
cd web && npx tsc --noEmit
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add web/src/components/active-workers-card.tsx
git commit -m "feat(web): add ActiveWorkersCard component"
```

---

### Task 7: MessagesCard and ExecutionsCard components

**Files:**
- Create: `web/src/components/messages-card.tsx`
- Create: `web/src/components/executions-card.tsx`

- [ ] **Step 1: Create MessagesCard**

```tsx
// web/src/components/messages-card.tsx
import { useTranslation } from "react-i18next"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Skeleton } from "@/components/ui/skeleton"

interface MessagesCardProps {
  received: number
  sent: number
  loading?: boolean
}

export function MessagesCard({ received, sent, loading }: MessagesCardProps) {
  const { t } = useTranslation()
  return (
    <Card>
      <CardHeader className="pb-2">
        <CardTitle className="text-sm font-medium text-muted-foreground">
          {t("dashboard.messages")}
        </CardTitle>
      </CardHeader>
      <CardContent>
        {loading ? (
          <div className="flex gap-6">
            <Skeleton className="h-8 w-16" />
            <Skeleton className="h-8 w-16" />
          </div>
        ) : (
          <div className="flex gap-6">
            <div>
              <p className="text-xs text-muted-foreground">{t("dashboard.messagesReceived")}</p>
              <p className="text-3xl font-bold">{received}</p>
            </div>
            <div>
              <p className="text-xs text-muted-foreground">{t("dashboard.messagesSent")}</p>
              <p className="text-3xl font-bold">{sent}</p>
            </div>
          </div>
        )}
      </CardContent>
    </Card>
  )
}
```

- [ ] **Step 2: Create ExecutionsCard**

```tsx
// web/src/components/executions-card.tsx
import { useTranslation } from "react-i18next"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Skeleton } from "@/components/ui/skeleton"
import type { ExecStats } from "@/lib/types"

interface ExecutionsCardProps {
  stats: ExecStats
  loading?: boolean
}

export function ExecutionsCard({ stats, loading }: ExecutionsCardProps) {
  const { t } = useTranslation()
  return (
    <Card>
      <CardHeader className="pb-2">
        <CardTitle className="text-sm font-medium text-muted-foreground">
          {t("dashboard.executions")}
        </CardTitle>
      </CardHeader>
      <CardContent>
        {loading ? (
          <div className="flex gap-4">
            <Skeleton className="h-8 w-12" />
            <Skeleton className="h-8 w-12" />
            <Skeleton className="h-8 w-12" />
          </div>
        ) : (
          <div className="flex gap-6">
            <div>
              <p className="text-xs text-muted-foreground">{t("dashboard.executionsTotal")}</p>
              <p className="text-3xl font-bold">{stats.total}</p>
            </div>
            <div>
              <p className="text-xs text-muted-foreground">{t("dashboard.executionsSuccess")}</p>
              <p className="text-3xl font-bold text-green-500">{stats.success}</p>
            </div>
            <div>
              <p className="text-xs text-muted-foreground">{t("dashboard.executionsFailed")}</p>
              <p className="text-3xl font-bold text-red-500">{stats.failed}</p>
            </div>
          </div>
        )}
      </CardContent>
    </Card>
  )
}
```

- [ ] **Step 3: Verify**

```bash
cd web && npx tsc --noEmit
```

Expected: no errors.

- [ ] **Step 4: Commit**

```bash
git add web/src/components/messages-card.tsx web/src/components/executions-card.tsx
git commit -m "feat(web): add MessagesCard and ExecutionsCard components"
```

---

### Task 8: ActivityTrendChart with Recharts

**Files:**
- Modify: `web/package.json` (install recharts)
- Create: `web/src/components/activity-trend-chart.tsx`

- [ ] **Step 1: Install recharts**

```bash
cd web && npm install recharts
```

Expected: recharts appears in `package.json` dependencies, `node_modules/recharts` exists.

- [ ] **Step 2: Create the chart component**

```tsx
// web/src/components/activity-trend-chart.tsx
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
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { useStatsTrend } from "@/hooks/use-stats"

const DAY_OPTIONS = [7, 15, 30] as const

export function ActivityTrendChart() {
  const { t } = useTranslation()
  const [days, setDays] = useState<7 | 15 | 30>(7)
  const { data, isLoading } = useStatsTrend(days)

  return (
    <Card>
      <CardHeader className="pb-2">
        <div className="flex items-center justify-between">
          <CardTitle className="text-sm font-medium text-muted-foreground">
            {t("dashboard.activityTrend")}
          </CardTitle>
          <div className="flex gap-1">
            {DAY_OPTIONS.map((d) => (
              <Button
                key={d}
                variant={days === d ? "default" : "ghost"}
                size="sm"
                className="h-7 px-2 text-xs"
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
        ) : (
          <ResponsiveContainer width="100%" height={200}>
            <LineChart data={data?.data ?? []} margin={{ top: 4, right: 8, left: -20, bottom: 0 }}>
              <CartesianGrid strokeDasharray="3 3" className="stroke-border" />
              <XAxis
                dataKey="date"
                tick={{ fontSize: 11 }}
                tickFormatter={(v: string) => v.slice(5)}
                className="text-muted-foreground"
              />
              <YAxis tick={{ fontSize: 11 }} allowDecimals={false} className="text-muted-foreground" />
              <Tooltip
                labelFormatter={(label: string) => label}
                formatter={(value: number) => [value, t("dashboard.activeWorkers")]}
              />
              <Line
                type="monotone"
                dataKey="active_workers"
                strokeWidth={2}
                dot={false}
                className="stroke-primary"
                stroke="hsl(var(--primary))"
              />
            </LineChart>
          </ResponsiveContainer>
        )}
      </CardContent>
    </Card>
  )
}
```

- [ ] **Step 3: Verify**

```bash
cd web && npx tsc --noEmit
```

Expected: no errors.

- [ ] **Step 4: Commit**

```bash
git add web/package.json web/package-lock.json web/src/components/activity-trend-chart.tsx
git commit -m "feat(web): add ActivityTrendChart with Recharts line chart"
```

---

### Task 9: Rewrite dashboard.tsx

**Files:**
- Rewrite: `web/src/pages/dashboard.tsx`

- [ ] **Step 1: Rewrite the file**

```tsx
// web/src/pages/dashboard.tsx
import { useTranslation } from "react-i18next"
import { PageHeader } from "@/components/page-header"
import { FadeIn } from "@/components/fade-in"
import { StatCard } from "@/components/stat-card"
import { ActiveWorkersCard } from "@/components/active-workers-card"
import { MessagesCard } from "@/components/messages-card"
import { ExecutionsCard } from "@/components/executions-card"
import { ActivityTrendChart } from "@/components/activity-trend-chart"
import { useStatsOverview } from "@/hooks/use-stats"

export function Dashboard() {
  const { t } = useTranslation()
  const { data, isLoading } = useStatsOverview()

  const empty: import("@/lib/types").StatsOverview = {
    departments: 0,
    workers: 0,
    active_workers_today: 0,
    active_workers_yesterday: 0,
    active_workers_change: null,
    messages_received_today: 0,
    messages_sent_today: 0,
    sessions_new_today: 0,
    executions_today: { total: 0, success: 0, failed: 0 },
    scheduled_tasks: 0,
  }
  const ov = data ?? empty

  return (
    <FadeIn>
      <PageHeader title={t("dashboard.title")} />

      {/* Row 1: 4 stat cards */}
      <div className="grid grid-cols-2 sm:grid-cols-4 gap-4 mb-4">
        <StatCard title={t("dashboard.departments")} value={ov.departments} loading={isLoading} />
        <StatCard title={t("dashboard.workers")} value={ov.workers} loading={isLoading} />
        <StatCard title={t("dashboard.scheduledTasks")} value={ov.scheduled_tasks} loading={isLoading} />
        <StatCard title={t("dashboard.sessionsToday")} value={ov.sessions_new_today} loading={isLoading} />
      </div>

      {/* Row 2: active workers full width */}
      <div className="mb-4">
        <ActiveWorkersCard
          today={ov.active_workers_today}
          yesterday={ov.active_workers_yesterday}
          change={ov.active_workers_change}
          loading={isLoading}
        />
      </div>

      {/* Row 3: messages + executions */}
      <div className="grid grid-cols-1 sm:grid-cols-2 gap-4 mb-4">
        <MessagesCard
          received={ov.messages_received_today}
          sent={ov.messages_sent_today}
          loading={isLoading}
        />
        <ExecutionsCard stats={ov.executions_today} loading={isLoading} />
      </div>

      {/* Row 4: trend chart */}
      <ActivityTrendChart />
    </FadeIn>
  )
}
```

- [ ] **Step 2: Verify**

```bash
cd web && npx tsc --noEmit
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add web/src/pages/dashboard.tsx
git commit -m "feat(web): rewrite dashboard as data panel"
```

---

### Task 10: i18n keys

**Files:**
- Modify: `web/src/locales/en.json`
- Modify: `web/src/locales/zh.json`

- [ ] **Step 1: Update en.json dashboard section**

Replace the existing `"dashboard"` object in `web/src/locales/en.json` with:

```json
"dashboard": {
  "title": "Dashboard",
  "departments": "Departments",
  "workers": "Workers",
  "scheduledTasks": "Scheduled Tasks",
  "sessionsToday": "Sessions Today",
  "activeWorkers": "Active Workers",
  "today": "Today",
  "yesterday": "Yesterday",
  "messages": "Messages",
  "messagesReceived": "Received",
  "messagesSent": "Sent",
  "executions": "Executions",
  "executionsTotal": "Total",
  "executionsSuccess": "Success",
  "executionsFailed": "Failed",
  "activityTrend": "Worker Activity Trend",
  "days": "d"
}
```

- [ ] **Step 2: Update zh.json dashboard section**

Replace the existing `"dashboard"` object in `web/src/locales/zh.json` with:

```json
"dashboard": {
  "title": "仪表盘",
  "departments": "部门数",
  "workers": "员工数",
  "scheduledTasks": "定时任务",
  "sessionsToday": "今日会话",
  "activeWorkers": "活跃员工",
  "today": "今日",
  "yesterday": "昨日",
  "messages": "消息",
  "messagesReceived": "收到",
  "messagesSent": "发送",
  "executions": "执行数",
  "executionsTotal": "总计",
  "executionsSuccess": "成功",
  "executionsFailed": "失败",
  "activityTrend": "员工活跃趋势",
  "days": "天"
}
```

- [ ] **Step 3: Run full build to verify everything compiles**

```bash
cd web && npm run build
```

Expected: build succeeds with no TypeScript or bundler errors.

- [ ] **Step 4: Run Go tests one final time**

```bash
cd .. && go test ./internal/...
```

Expected: all tests PASS.

- [ ] **Step 5: Final commit**

```bash
git add web/src/locales/en.json web/src/locales/zh.json
git commit -m "feat(i18n): add dashboard data panel translation keys"
```

---

## Self-Review

**Spec coverage check:**

| Spec requirement | Task |
|-----------------|------|
| 部门数 | Task 1 (StatsStore), Task 9 (dashboard) |
| 员工数 | Task 1, Task 9 |
| 今日/昨日活跃员工/同比 | Task 1, Task 6, Task 9 |
| 折线图 7/15/30天 | Task 1 (GetTrend), Task 8 (chart), Task 9 |
| 今日消息收/发 | Task 1, Task 7, Task 9 |
| 今日会话数 | Task 1, Task 9 |
| 今日执行总/成/失败 | Task 1, Task 7, Task 9 |
| 定时任务数量 | Task 1, Task 9 |
| 自动刷新 30s | Task 4 (useStatsOverview refetchInterval) |
| JWT 鉴权 | Task 2 (routes inside JWT group) |
| i18n | Task 10 |

**Placeholder scan:** No TBD, TODO, or "similar to" references found. Every step has code.

**Type consistency:**
- `ExecStats` defined in `types.ts` (Task 4), used in `ExecutionsCard` (Task 7) and `dashboard.tsx` (Task 9) ✅
- `StatsOverview.executions_today` → `ExecStats` ✅
- `useStatsTrend(days: 7 | 15 | 30)` matches `api.stats.trend(days: 7 | 15 | 30)` ✅
- `StatsStore.GetOverview` / `GetTrend` return types match handler usage ✅
- `dashboard.executionsTotal` i18n key added in Task 10, used in Task 7 ✅
