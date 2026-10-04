package task_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/theopenbee/openbee/internal/domain/task"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/infra/store"
)

func setupDB(t *testing.T) (*sql.DB, *store.TaskStore) {
	t.Helper()
	db, err := store.InitDB(t.TempDir() + "/test.db")
	require.NoError(t, err)
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

	dispCh := make(chan task.DispatchTask, 10)
	sched := task.NewScheduler(ts, dispCh, 50*time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	go sched.Run(ctx)

	select {
	case dt := <-dispCh:
		assert.Equal(t, "w1", dt.WorkerID)
		assert.Equal(t, model.TaskTypeImmediate, dt.TaskType)
	case <-ctx.Done():
		require.Fail(t, "timeout: no task dispatched")
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
	require.NoError(t, err)

	dispCh := make(chan task.DispatchTask, 10)
	sched := task.NewScheduler(ts, dispCh, 50*time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	go sched.Run(ctx)

	// Wait for dispatch
	select {
	case <-dispCh:
	case <-ctx.Done():
		require.Fail(t, "timeout: scheduled task not dispatched")
	}

	// next_run_at should NOT be +24h sentinel; it should be cron-computed (~1h from now)
	got, err := ts.GetByID(context.Background(), taskID)
	require.NoError(t, err)
	require.NotNil(t, got.NextRunAt, "next_run_at should not be nil after dispatch")

	sentinel := now + 24*60*60*1000
	assert.NotEqual(t, sentinel, *got.NextRunAt, "next_run_at must not be the old 24h sentinel")

	// Should be cron-computed: next top-of-hour, which is at most 1 hour away.
	twoMin := int64(2 * 60 * 1000)
	oneHour := int64(60 * 60 * 1000)
	assert.Greater(t, *got.NextRunAt, now, "next_run_at must be after now")
	assert.LessOrEqual(t, *got.NextRunAt, now+oneHour+twoMin, "next_run_at must be within 1h+2min of now")
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

	dispCh := make(chan task.DispatchTask, 10)
	sched := task.NewScheduler(ts, dispCh, 50*time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	go sched.Run(ctx)

	select {
	case dt := <-dispCh:
		assert.Failf(t, "should not have dispatched future task", "got: %+v", dt)
	case <-ctx.Done():
		// Expected: no dispatch
	}
}
