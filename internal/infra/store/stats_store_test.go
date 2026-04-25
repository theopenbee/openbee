// internal/infra/store/stats_store_test.go
package store

import (
	"context"
	"strconv"
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


func TestStatsStore_GetOverview_Counts(t *testing.T) {
	ss, ws, es, ms, oms, ts, cleanup := newStatsTestDB(t)
	defer cleanup()
	ctx := context.Background()

	// Create 2 workers
	w1, _ := ws.Create(model.Worker{Name: "W1", WorkDir: "/tmp/w1"})
	w2, _ := ws.Create(model.Worker{Name: "W2", WorkDir: "/tmp/w2"})

	// Both workers have executions today
	todayStartMS, _ := dayBounds(0)
	todayMS := todayStartMS + 1000
	db := ss.db
	if _, err := db.Exec(`INSERT INTO bee_executions (id,worker_id,session_id,trigger_input,status,result,ai_process_pid,started_at) VALUES (?,?,?,?,?,?,?,?)`,
		uuid.New().String(), w1.ID, "sess1", "hi", "completed", "", 0, todayMS); err != nil {
		t.Fatalf("insert execution: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO bee_executions (id,worker_id,session_id,trigger_input,status,result,ai_process_pid,started_at) VALUES (?,?,?,?,?,?,?,?)`,
		uuid.New().String(), w2.ID, "sess2", "hi", "failed", "", 0, todayMS); err != nil {
		t.Fatalf("insert execution: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO bee_executions (id,worker_id,session_id,trigger_input,status,result,ai_process_pid,started_at) VALUES (?,?,?,?,?,?,?,?)`,
		uuid.New().String(), w1.ID, "sess3", "hi", "completed", "", 0, todayMS); err != nil {
		t.Fatalf("insert execution: %v", err)
	}

	// One inbound message today
	ms.Create(ctx, uuid.New().String(), "sk1", "feishu", "hello", "{}", "", todayMS)

	// One outbound message today
	if _, err := db.Exec(`INSERT INTO bee_outbound_messages (id,session_key,platform,content,media_path,status,platform_msg_id,source_type,source_id,inbound_msg_id,error,retry_count,sent_at,created_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		uuid.New().String(), "sk1", "feishu", "reply", "", "sent", "", "worker", w1.ID, "", "", 0, todayMS, todayMS); err != nil {
		t.Fatalf("insert outbound message: %v", err)
	}

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
	if ov.MessagesTotalToday != 2 {
		t.Errorf("MessagesTotalToday: want 2, got %d", ov.MessagesTotalToday)
	}
	if ov.ExecDurationTodayMS != 0 {
		t.Errorf("ExecDurationTodayMS: want 0 (no completed_at set), got %d", ov.ExecDurationTodayMS)
	}
	if ov.ExecutionsToday != 3 {
		t.Errorf("ExecutionsToday: want 3, got %d", ov.ExecutionsToday)
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
	if _, err := db.Exec(`INSERT INTO bee_executions (id,worker_id,session_id,trigger_input,status,result,ai_process_pid,started_at) VALUES (?,?,?,?,?,?,?,?)`,
		uuid.New().String(), w1.ID, "sess1", "hi", "completed", "", 0, threeDaysAgo); err != nil {
		t.Fatalf("insert execution: %v", err)
	}

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
	todayStartMS, _ := dayBounds(0)
	todayMS := todayStartMS + 1000
	if _, err := db.Exec(`INSERT INTO bee_executions (id,worker_id,session_id,trigger_input,status,result,ai_process_pid,started_at) VALUES (?,?,?,?,?,?,?,?)`,
		uuid.New().String(), w1.ID, "sess1", "hi", "completed", "", 0, todayMS); err != nil {
		t.Fatalf("insert execution: %v", err)
	}

	ov, err := ss.GetOverview(ctx)
	if err != nil {
		t.Fatalf("GetOverview: %v", err)
	}

	if ov.ActiveWorkersChange != nil {
		t.Errorf("ActiveWorkersChange: want nil (yesterday=0), got %v", *ov.ActiveWorkersChange)
	}
}

func TestStatsStore_GetOverview_ExecDuration(t *testing.T) {
	ss, ws, _, _, _, _, cleanup := newStatsTestDB(t)
	defer cleanup()
	ctx := context.Background()

	w1, _ := ws.Create(model.Worker{Name: "W1", WorkDir: "/tmp/w1"})
	db := ss.db

	todayStart, _ := dayBounds(0)
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

}

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
	if ov.TokensYestTotal != 200 {
		t.Errorf("TokensYestTotal: want 200, got %d", ov.TokensYestTotal)
	}
}

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
}

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
	for _, row := range []struct {
		model  string
		tokens int64
	}{
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
			uuid.New().String()+strconv.Itoa(i), "w1", sessID, "hi", "completed", "", 0,
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
