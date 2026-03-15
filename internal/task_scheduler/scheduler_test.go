package task_scheduler_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/robobee/core/internal/task_dispatcher"
	"github.com/robobee/core/internal/model"
	"github.com/robobee/core/internal/store"
	"github.com/robobee/core/internal/task_scheduler"
)

func setupDB(t *testing.T) (*sql.DB, *store.TaskStore) {
	t.Helper()
	db, err := store.InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	db.Exec(`INSERT INTO workers (id,name,work_dir,status,created_at,updated_at) VALUES ('w1','W','/','idle',1,1)`)
	db.Exec(`INSERT INTO platform_messages (id,session_key,platform,content,received_at,created_at,updated_at) VALUES ('m1','sk','feishu','hi',1,1,1)`)
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
