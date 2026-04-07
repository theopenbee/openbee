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
	todayMS := todayStartMS() + 1000
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
