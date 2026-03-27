package task_scheduler_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/theopenbee/openbee/internal/task_dispatcher"
	"github.com/theopenbee/openbee/internal/model"
	"github.com/theopenbee/openbee/internal/store"
	"github.com/theopenbee/openbee/internal/task_scheduler"
)

func setupDB(t *testing.T) (*sql.DB, *store.TaskStore) {
	t.Helper()
	db, err := store.InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	db.Exec(`INSERT INTO bee_workers (id,name,work_dir,status,created_at,updated_at) VALUES ('w1','W','/','idle',1,1)`)
	db.Exec(`INSERT INTO bee_platform_messages (id,session_key,platform,content,received_at,created_at,updated_at) VALUES ('m1','sk','feishu','hi',1,1,1)`)
	return db, store.NewTaskStore(db)
}

func TestScheduler_ImmediateTask_Dispatched(t *testing.T) {
	db, ts := setupDB(t)
	defer db.Close()

	now := time.Now().UnixMilli()
	ts.Create(context.Background(), model.Task{
		MessageID: "m1", WorkerID: "w1", Instruction: "go",
		Type: model.TaskTypeImmediate, Status: model.TaskStatusPending,
		CreatedAt: now, UpdatedAt: now,
	})

	dispCh := make(chan task_dispatcher.DispatchTask, 10)
	sched := task_scheduler.New(ts, dispCh, 50*time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	go sched.Run(ctx)

	select {
	case task := <-dispCh:
		if task.WorkerID != "w1" {
			t.Errorf("unexpected worker: %s", task.WorkerID)
		}
		if task.TaskType != model.TaskTypeImmediate {
			t.Errorf("unexpected task type: %s", task.TaskType)
		}
	case <-ctx.Done():
		t.Fatal("timeout: no task dispatched")
	}
}

func TestScheduler_ScheduledTask_NextRunAtSetCorrectly(t *testing.T) {
	db, ts := setupDB(t)
	defer db.Close()

	now := time.Now().UnixMilli()
	pastRun := now - 1000
	cronExpr := "0 * * * *" // every hour

	taskID, err := ts.Create(context.Background(), model.Task{
		MessageID: "m1", WorkerID: "w1", Instruction: "tick",
		Type: model.TaskTypeScheduled, Status: model.TaskStatusPending,
		CronExpr: cronExpr, NextRunAt: &pastRun,
		CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	dispCh := make(chan task_dispatcher.DispatchTask, 10)
	sched := task_scheduler.New(ts, dispCh, 50*time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	go sched.Run(ctx)

	// Wait for dispatch
	select {
	case <-dispCh:
	case <-ctx.Done():
		t.Fatal("timeout: scheduled task not dispatched")
	}

	// next_run_at should NOT be +24h sentinel; it should be cron-computed (~1h from now)
	got, err := ts.GetByID(context.Background(), taskID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.NextRunAt == nil {
		t.Fatal("next_run_at should not be nil after dispatch")
	}
	sentinel := now + 24*60*60*1000
	if *got.NextRunAt == sentinel {
		t.Errorf("next_run_at is the old 24h sentinel (%d); expected real cron value", sentinel)
	}
	// Should be cron-computed: next top-of-hour, which is at most 1 hour away.
	// Verify it's in the future and no more than 1h+2min from now.
	twoMin := int64(2 * 60 * 1000)
	oneHour := int64(60 * 60 * 1000)
	if *got.NextRunAt <= now || *got.NextRunAt > now+oneHour+twoMin {
		t.Errorf("next_run_at=%d not a valid cron value relative to now=%d (expected in range (now, now+1h+2min])", *got.NextRunAt, now)
	}
}

func TestScheduler_CountdownTask_NotDispatchedBeforeTime(t *testing.T) {
	db, ts := setupDB(t)
	defer db.Close()

	now := time.Now().UnixMilli()
	future := now + 60_000 // 1 minute from now
	ts.Create(context.Background(), model.Task{
		MessageID: "m1", WorkerID: "w1", Instruction: "go",
		Type: model.TaskTypeCountdown, Status: model.TaskStatusPending,
		ScheduledAt: &future,
		CreatedAt:   now, UpdatedAt: now,
	})

	dispCh := make(chan task_dispatcher.DispatchTask, 10)
	sched := task_scheduler.New(ts, dispCh, 50*time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	go sched.Run(ctx)

	select {
	case task := <-dispCh:
		t.Errorf("should not have dispatched future task, got: %+v", task)
	case <-ctx.Done():
		// Expected: no dispatch
	}
}
