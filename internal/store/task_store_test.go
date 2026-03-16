package store

import (
	"context"
	"testing"
	"time"

	"github.com/robobee/core/internal/model"
)

func newTaskStoreForTest(t *testing.T) (*TaskStore, func()) {
	t.Helper()
	db, err := InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	// Insert prerequisite rows matching the actual schema (raw, platform_msg_id required)
	db.Exec(`INSERT INTO workers (id,name,work_dir,status,created_at,updated_at) VALUES ('w1','W','/','idle',1,1)`)
	db.Exec(`INSERT INTO platform_messages
        (id, session_key, platform, content, raw, platform_msg_id, received_at, created_at, updated_at)
        VALUES ('m1','feishu:c:u','feishu','hi','','',1,1,1)`)
	return NewTaskStore(db), func() { db.Close() }
}

func TestTaskStore_Create_And_Get(t *testing.T) {
	ts, cleanup := newTaskStoreForTest(t)
	defer cleanup()

	now := time.Now().UnixMilli()
	task := model.Task{
		MessageID:   "m1",
		WorkerID:    "w1",
		Instruction: "do it",
		Type:        model.TaskTypeImmediate,
		Status:      model.TaskStatusPending,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	id, err := ts.Create(context.Background(), task)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if id == "" {
		t.Fatal("expected non-empty task ID")
	}

	got, err := ts.GetByID(context.Background(), id)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Instruction != "do it" {
		t.Errorf("instruction: want %q got %q", "do it", got.Instruction)
	}
	if got.Type != model.TaskTypeImmediate {
		t.Errorf("type: want immediate got %q", got.Type)
	}
}

func TestTaskStore_ClaimDueTasks_ImmediateOnly(t *testing.T) {
	ts, cleanup := newTaskStoreForTest(t)
	defer cleanup()

	now := time.Now().UnixMilli()
	ts.Create(context.Background(), model.Task{
		MessageID: "m1", WorkerID: "w1", Instruction: "go",
		Type: model.TaskTypeImmediate, Status: model.TaskStatusPending,
		CreatedAt: now, UpdatedAt: now,
	})

	tasks, err := ts.ClaimDueTasks(context.Background(), now)
	if err != nil {
		t.Fatalf("ClaimDueTasks: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected 1 due task, got %d", len(tasks))
	}
	if tasks[0].Status != model.TaskStatusRunning {
		t.Errorf("claimed task should have status running, got %q", tasks[0].Status)
	}
}

func TestTaskStore_ClaimDueTasks_Idempotent(t *testing.T) {
	ts, cleanup := newTaskStoreForTest(t)
	defer cleanup()

	now := time.Now().UnixMilli()
	ts.Create(context.Background(), model.Task{
		MessageID: "m1", WorkerID: "w1", Instruction: "go",
		Type: model.TaskTypeImmediate, Status: model.TaskStatusPending,
		CreatedAt: now, UpdatedAt: now,
	})

	tasks1, _ := ts.ClaimDueTasks(context.Background(), now)
	tasks2, _ := ts.ClaimDueTasks(context.Background(), now)
	if len(tasks1) != 1 {
		t.Errorf("first claim: want 1, got %d", len(tasks1))
	}
	if len(tasks2) != 0 {
		t.Errorf("second claim should be empty (already running), got %d", len(tasks2))
	}
}

func TestTaskStore_SetExecution(t *testing.T) {
	ts, cleanup := newTaskStoreForTest(t)
	defer cleanup()

	now := time.Now().UnixMilli()
	id, _ := ts.Create(context.Background(), model.Task{
		MessageID: "m1", WorkerID: "w1", Instruction: "go",
		Type: model.TaskTypeImmediate, Status: model.TaskStatusPending,
		CreatedAt: now, UpdatedAt: now,
	})

	err := ts.SetExecution(context.Background(), id, "exec-1", model.TaskStatusCompleted)
	if err != nil {
		t.Fatalf("SetExecution: %v", err)
	}

	got, _ := ts.GetByID(context.Background(), id)
	if got.ExecutionID != "exec-1" {
		t.Errorf("execution_id: want exec-1 got %q", got.ExecutionID)
	}
	if got.Status != model.TaskStatusCompleted {
		t.Errorf("status: want completed got %q", got.Status)
	}
}

func TestTaskStore_DeleteByMessageIDs(t *testing.T) {
	ts, cleanup := newTaskStoreForTest(t)
	defer cleanup()

	now := time.Now().UnixMilli()
	ts.Create(context.Background(), model.Task{
		MessageID: "m1", WorkerID: "w1", Instruction: "go",
		Type: model.TaskTypeImmediate, Status: model.TaskStatusPending,
		CreatedAt: now, UpdatedAt: now,
	})

	err := ts.DeletePendingByMessageIDs(context.Background(), []string{"m1"})
	if err != nil {
		t.Fatalf("DeletePendingByMessageIDs: %v", err)
	}

	// Verify no pending tasks remain
	tasks, _ := ts.ClaimDueTasks(context.Background(), now)
	if len(tasks) != 0 {
		t.Errorf("expected 0 tasks after delete, got %d", len(tasks))
	}
}

func TestTaskStore_ListByMessageID(t *testing.T) {
	ts, cleanup := newTaskStoreForTest(t)
	defer cleanup()

	now := time.Now().UnixMilli()
	ts.Create(context.Background(), model.Task{
		MessageID: "m1", WorkerID: "w1", Instruction: "a",
		Type: model.TaskTypeImmediate, Status: model.TaskStatusPending,
		CreatedAt: now, UpdatedAt: now,
	})
	ts.Create(context.Background(), model.Task{
		MessageID: "m1", WorkerID: "w1", Instruction: "b",
		Type: model.TaskTypeCountdown, Status: model.TaskStatusPending,
		CreatedAt: now, UpdatedAt: now,
	})

	tasks, err := ts.ListByMessageID(context.Background(), "m1", "", "")
	if err != nil {
		t.Fatalf("ListByMessageID: %v", err)
	}
	if len(tasks) != 2 {
		t.Errorf("expected 2 tasks, got %d", len(tasks))
	}
}

func TestTaskStore_UpdateStatus_SetsCompleted(t *testing.T) {
	ts, cleanup := newTaskStoreForTest(t)
	defer cleanup()

	now := time.Now().UnixMilli()
	id, err := ts.Create(context.Background(), model.Task{
		MessageID: "m1", WorkerID: "w1", Instruction: "go",
		Type: model.TaskTypeImmediate, Status: model.TaskStatusRunning,
		CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := ts.UpdateStatus(context.Background(), id, model.TaskStatusCompleted); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}

	got, err := ts.GetByID(context.Background(), id)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Status != model.TaskStatusCompleted {
		t.Errorf("status: want completed, got %q", got.Status)
	}
	// execution_id must be untouched (different from SetExecution)
	if got.ExecutionID != "" {
		t.Errorf("execution_id should be empty, got %q", got.ExecutionID)
	}
}

func TestTaskStore_UpdateStatus_SetsFailed(t *testing.T) {
	ts, cleanup := newTaskStoreForTest(t)
	defer cleanup()

	now := time.Now().UnixMilli()
	id, err := ts.Create(context.Background(), model.Task{
		MessageID: "m1", WorkerID: "w1", Instruction: "go",
		Type: model.TaskTypeImmediate, Status: model.TaskStatusRunning,
		CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := ts.UpdateStatus(context.Background(), id, model.TaskStatusFailed); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}

	got, _ := ts.GetByID(context.Background(), id)
	if got.Status != model.TaskStatusFailed {
		t.Errorf("status: want failed, got %q", got.Status)
	}
}

func newTaskStoreWithTwoSessions(t *testing.T) (*TaskStore, func()) {
	t.Helper()
	db, err := InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	db.Exec(`INSERT INTO workers (id,name,work_dir,status,created_at,updated_at) VALUES ('w1','W','/','idle',1,1)`)
	db.Exec(`INSERT INTO platform_messages
		(id, session_key, platform, content, raw, platform_msg_id, received_at, created_at, updated_at)
		VALUES ('m1','session-A','feishu','hi','','',1,1,1)`)
	db.Exec(`INSERT INTO platform_messages
		(id, session_key, platform, content, raw, platform_msg_id, received_at, created_at, updated_at)
		VALUES ('m2','session-B','feishu','bye','','',1,1,1)`)
	return NewTaskStore(db), func() { db.Close() }
}

func TestTaskStore_ListBySessionKey(t *testing.T) {
	ts, cleanup := newTaskStoreWithTwoSessions(t)
	defer cleanup()
	ctx := context.Background()
	now := time.Now().UnixMilli()

	// Create tasks in session-A: one pending, one running
	ts.Create(ctx, model.Task{
		MessageID: "m1", WorkerID: "w1", Instruction: "a",
		Type: model.TaskTypeImmediate, Status: model.TaskStatusPending,
		CreatedAt: now, UpdatedAt: now,
	})
	ts.Create(ctx, model.Task{
		MessageID: "m1", WorkerID: "w1", Instruction: "b",
		Type: model.TaskTypeImmediate, Status: model.TaskStatusRunning,
		CreatedAt: now, UpdatedAt: now,
	})
	// Create task in session-B
	ts.Create(ctx, model.Task{
		MessageID: "m2", WorkerID: "w1", Instruction: "c",
		Type: model.TaskTypeImmediate, Status: model.TaskStatusPending,
		CreatedAt: now, UpdatedAt: now,
	})

	// List all tasks for session-A
	tasks, err := ts.ListBySessionKey(ctx, "session-A", "", "")
	if err != nil {
		t.Fatalf("ListBySessionKey: %v", err)
	}
	if len(tasks) != 2 {
		t.Errorf("expected 2 tasks for session-A, got %d", len(tasks))
	}

	// List only pending tasks for session-A
	tasks, err = ts.ListBySessionKey(ctx, "session-A", "pending", "")
	if err != nil {
		t.Fatalf("ListBySessionKey (pending): %v", err)
	}
	if len(tasks) != 1 {
		t.Errorf("expected 1 pending task for session-A, got %d", len(tasks))
	}

	// List with comma-separated status
	tasks, err = ts.ListBySessionKey(ctx, "session-A", "pending,running", "")
	if err != nil {
		t.Fatalf("ListBySessionKey (pending,running): %v", err)
	}
	if len(tasks) != 2 {
		t.Errorf("expected 2 tasks for session-A with pending,running, got %d", len(tasks))
	}

	// List for session-B
	tasks, err = ts.ListBySessionKey(ctx, "session-B", "", "")
	if err != nil {
		t.Fatalf("ListBySessionKey session-B: %v", err)
	}
	if len(tasks) != 1 {
		t.Errorf("expected 1 task for session-B, got %d", len(tasks))
	}
}

func TestTaskStore_ListByMessageID_CommaSeparatedStatus(t *testing.T) {
	ts, cleanup := newTaskStoreForTest(t)
	defer cleanup()
	ctx := context.Background()
	now := time.Now().UnixMilli()

	ts.Create(ctx, model.Task{
		MessageID: "m1", WorkerID: "w1", Instruction: "a",
		Type: model.TaskTypeImmediate, Status: model.TaskStatusPending,
		CreatedAt: now, UpdatedAt: now,
	})
	ts.Create(ctx, model.Task{
		MessageID: "m1", WorkerID: "w1", Instruction: "b",
		Type: model.TaskTypeImmediate, Status: model.TaskStatusRunning,
		CreatedAt: now, UpdatedAt: now,
	})
	ts.Create(ctx, model.Task{
		MessageID: "m1", WorkerID: "w1", Instruction: "c",
		Type: model.TaskTypeImmediate, Status: model.TaskStatusCompleted,
		CreatedAt: now, UpdatedAt: now,
	})

	tasks, err := ts.ListByMessageID(ctx, "m1", "pending,running", "")
	if err != nil {
		t.Fatalf("ListByMessageID: %v", err)
	}
	if len(tasks) != 2 {
		t.Errorf("expected 2 tasks (pending+running), got %d", len(tasks))
	}
}

func TestTaskStore_CancelBySessionKey(t *testing.T) {
	ts, cleanup := newTaskStoreWithTwoSessions(t)
	defer cleanup()
	ctx := context.Background()
	now := time.Now().UnixMilli()

	// Create tasks in session-A: pending + running + completed
	ts.Create(ctx, model.Task{
		MessageID: "m1", WorkerID: "w1", Instruction: "a",
		Type: model.TaskTypeImmediate, Status: model.TaskStatusPending,
		CreatedAt: now, UpdatedAt: now,
	})
	ts.Create(ctx, model.Task{
		MessageID: "m1", WorkerID: "w1", Instruction: "b",
		Type: model.TaskTypeImmediate, Status: model.TaskStatusRunning,
		CreatedAt: now, UpdatedAt: now,
	})
	ts.Create(ctx, model.Task{
		MessageID: "m1", WorkerID: "w1", Instruction: "c",
		Type: model.TaskTypeImmediate, Status: model.TaskStatusCompleted,
		CreatedAt: now, UpdatedAt: now,
	})
	// Task in session-B (should not be affected)
	ts.Create(ctx, model.Task{
		MessageID: "m2", WorkerID: "w1", Instruction: "d",
		Type: model.TaskTypeImmediate, Status: model.TaskStatusPending,
		CreatedAt: now, UpdatedAt: now,
	})

	n, err := ts.CancelBySessionKey(ctx, "session-A")
	if err != nil {
		t.Fatalf("CancelBySessionKey: %v", err)
	}
	if n != 2 {
		t.Errorf("expected 2 cancelled (pending+running), got %d", n)
	}

	// Verify: session-A completed task untouched
	tasksA, _ := ts.ListBySessionKey(ctx, "session-A", "completed", "")
	if len(tasksA) != 1 {
		t.Errorf("completed task should be untouched, got %d", len(tasksA))
	}

	// Verify: session-A cancelled tasks
	cancelledA, _ := ts.ListBySessionKey(ctx, "session-A", "cancelled", "")
	if len(cancelledA) != 2 {
		t.Errorf("expected 2 cancelled tasks, got %d", len(cancelledA))
	}

	// Verify: session-B unaffected
	tasksB, _ := ts.ListBySessionKey(ctx, "session-B", "pending", "")
	if len(tasksB) != 1 {
		t.Errorf("session-B task should be unaffected, got %d", len(tasksB))
	}
}

func TestTaskStore_ResetRunningToPending(t *testing.T) {
	ts, cleanup := newTaskStoreForTest(t)
	defer cleanup()

	now := time.Now().UnixMilli()
	id, _ := ts.Create(context.Background(), model.Task{
		MessageID: "m1", WorkerID: "w1", Instruction: "go",
		Type: model.TaskTypeImmediate, Status: model.TaskStatusRunning,
		CreatedAt: now, UpdatedAt: now,
	})

	n, err := ts.ResetRunningToPending(context.Background())
	if err != nil {
		t.Fatalf("ResetRunningToPending: %v", err)
	}
	if n != 1 {
		t.Errorf("expected 1 reset, got %d", n)
	}

	got, _ := ts.GetByID(context.Background(), id)
	if got.Status != model.TaskStatusPending {
		t.Errorf("expected pending, got %q", got.Status)
	}
}

func TestTaskStore_FailTask_RegularTask_MarksAsFailed(t *testing.T) {
	ts, cleanup := newTaskStoreForTest(t)
	defer cleanup()

	ctx := context.Background()
	id, _ := ts.Create(ctx, model.Task{
		MessageID: "m1", WorkerID: "w1", Instruction: "x",
		Type: model.TaskTypeImmediate, Status: model.TaskStatusRunning,
		CreatedAt: 1, UpdatedAt: 1,
	})

	if err := ts.FailTask(ctx, id); err != nil {
		t.Fatalf("FailTask: %v", err)
	}

	got, _ := ts.GetByID(ctx, id)
	if got.Status != model.TaskStatusFailed {
		t.Errorf("expected status=failed, got %q", got.Status)
	}
}

func TestTaskStore_FailTask_ScheduledTask_WithCron_ResetsToPending(t *testing.T) {
	ts, cleanup := newTaskStoreForTest(t)
	defer cleanup()

	ctx := context.Background()
	id, _ := ts.Create(ctx, model.Task{
		MessageID: "m1", WorkerID: "w1", Instruction: "x",
		Type: model.TaskTypeScheduled, CronExpr: "* * * * *",
		Status: model.TaskStatusRunning,
		CreatedAt: 1, UpdatedAt: 1,
	})

	if err := ts.FailTask(ctx, id); err != nil {
		t.Fatalf("FailTask: %v", err)
	}

	got, _ := ts.GetByID(ctx, id)
	if got.Status != model.TaskStatusPending {
		t.Errorf("expected status=pending (reset for next run), got %q", got.Status)
	}
}

func TestTaskStore_FailTask_ScheduledTask_NoCron_MarksAsFailed(t *testing.T) {
	ts, cleanup := newTaskStoreForTest(t)
	defer cleanup()

	ctx := context.Background()
	id, _ := ts.Create(ctx, model.Task{
		MessageID: "m1", WorkerID: "w1", Instruction: "x",
		Type: model.TaskTypeScheduled, CronExpr: "",
		Status: model.TaskStatusRunning,
		CreatedAt: 1, UpdatedAt: 1,
	})

	if err := ts.FailTask(ctx, id); err != nil {
		t.Fatalf("FailTask: %v", err)
	}

	got, _ := ts.GetByID(ctx, id)
	if got.Status != model.TaskStatusFailed {
		t.Errorf("expected status=failed (no cron), got %q", got.Status)
	}
}

func TestTaskStore_FailTask_ScheduledTask_Cancelled_NoChange(t *testing.T) {
	ts, cleanup := newTaskStoreForTest(t)
	defer cleanup()

	ctx := context.Background()
	id, _ := ts.Create(ctx, model.Task{
		MessageID: "m1", WorkerID: "w1", Instruction: "x",
		Type: model.TaskTypeScheduled, CronExpr: "* * * * *",
		Status: model.TaskStatusCancelled,
		CreatedAt: 1, UpdatedAt: 1,
	})

	// FailTask on a cancelled scheduled task should not error
	if err := ts.FailTask(ctx, id); err != nil {
		t.Fatalf("FailTask on cancelled task: %v", err)
	}

	got, _ := ts.GetByID(ctx, id)
	if got.Status != model.TaskStatusCancelled {
		t.Errorf("expected status=cancelled (preserved), got %q", got.Status)
	}
}
