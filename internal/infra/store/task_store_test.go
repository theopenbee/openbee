package store

import (
	"context"
	"testing"
	"time"

	"github.com/theopenbee/openbee/internal/infra/model"
)

func newTaskStoreForTest(t *testing.T) (*TaskStore, func()) {
	t.Helper()
	db, err := InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	// Insert prerequisite rows matching the actual schema (raw, platform_msg_id required)
	db.Exec(`INSERT INTO bee_workers (id,name,work_dir,status,created_at,updated_at) VALUES ('w1','W','/','idle',1,1)`)
	db.Exec(`INSERT INTO bee_platform_messages
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

	tasks, err := ts.ClaimDueTasks(context.Background(), now, nil)
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

	tasks1, _ := ts.ClaimDueTasks(context.Background(), now, nil)
	tasks2, _ := ts.ClaimDueTasks(context.Background(), now, nil)
	if len(tasks1) != 1 {
		t.Errorf("first claim: want 1, got %d", len(tasks1))
	}
	if len(tasks2) != 0 {
		t.Errorf("second claim should be empty (already running), got %d", len(tasks2))
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
	tasks, _ := ts.ClaimDueTasks(context.Background(), now, nil)
	if len(tasks) != 0 {
		t.Errorf("expected 0 tasks after delete, got %d", len(tasks))
	}
}

func TestTaskStore_List_ByMessageIDFilter(t *testing.T) {
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

	tasks, err := ts.List(context.Background(), TaskFilter{MessageID: "m1"})
	if err != nil {
		t.Fatalf("List: %v", err)
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

func TestTaskStore_UpdateStatusIfRunning_SkipsWhenNotRunning(t *testing.T) {
	ts, cleanup := newTaskStoreForTest(t)
	defer cleanup()

	now := time.Now().UnixMilli()
	id, err := ts.Create(context.Background(), model.Task{
		MessageID: "m1", WorkerID: "w1", Instruction: "go",
		Type: model.TaskTypeImmediate, Status: model.TaskStatusPending,
		CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	changed, err := ts.UpdateStatusIfRunning(context.Background(), id, model.TaskStatusCompleted)
	if err != nil {
		t.Fatalf("UpdateStatusIfRunning: %v", err)
	}
	if changed {
		t.Fatal("expected no-op when current status is pending")
	}
	got, _ := ts.GetByID(context.Background(), id)
	if got.Status != model.TaskStatusPending {
		t.Errorf("status mutated: got %q", got.Status)
	}
}

func TestTaskStore_UpdateStatusIfRunning_Transitions(t *testing.T) {
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

	changed, err := ts.UpdateStatusIfRunning(context.Background(), id, model.TaskStatusFailed)
	if err != nil {
		t.Fatalf("UpdateStatusIfRunning: %v", err)
	}
	if !changed {
		t.Fatal("expected transition from running")
	}
	got, _ := ts.GetByID(context.Background(), id)
	if got.Status != model.TaskStatusFailed {
		t.Errorf("status not updated: %q", got.Status)
	}
}

func newTaskStoreWithTwoSessions(t *testing.T) (*TaskStore, func()) {
	t.Helper()
	db, err := InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	db.Exec(`INSERT INTO bee_workers (id,name,work_dir,status,created_at,updated_at) VALUES ('w1','W','/','idle',1,1)`)
	db.Exec(`INSERT INTO bee_platform_messages
		(id, session_key, platform, content, raw, platform_msg_id, received_at, created_at, updated_at)
		VALUES ('m1','session-A','feishu','hi','','',1,1,1)`)
	db.Exec(`INSERT INTO bee_platform_messages
		(id, session_key, platform, content, raw, platform_msg_id, received_at, created_at, updated_at)
		VALUES ('m2','session-B','feishu','bye','','',1,1,1)`)
	return NewTaskStore(db), func() { db.Close() }
}

// newTaskStoreWithTwoWorkers sets up: w1 and w2 workers; m1 (session-A) and m2 (session-B) messages.
func newTaskStoreWithTwoWorkers(t *testing.T) (*TaskStore, func()) {
	t.Helper()
	db, err := InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	db.Exec(`INSERT INTO bee_workers (id,name,work_dir,status,created_at,updated_at) VALUES ('w1','W1','/','idle',1,1)`)
	db.Exec(`INSERT INTO bee_workers (id,name,work_dir,status,created_at,updated_at) VALUES ('w2','W2','/','idle',1,1)`)
	db.Exec(`INSERT INTO bee_platform_messages
		(id, session_key, platform, content, raw, platform_msg_id, received_at, created_at, updated_at)
		VALUES ('m1','session-A','feishu','hi','','',1,1,1)`)
	db.Exec(`INSERT INTO bee_platform_messages
		(id, session_key, platform, content, raw, platform_msg_id, received_at, created_at, updated_at)
		VALUES ('m2','session-B','feishu','bye','','',1,1,1)`)
	return NewTaskStore(db), func() { db.Close() }
}

func TestTaskStore_List_ByWorkerID(t *testing.T) {
	ts, cleanup := newTaskStoreWithTwoWorkers(t)
	defer cleanup()
	ctx := context.Background()
	now := time.Now().UnixMilli()

	// w1 has tasks in session-A and session-B; w2 has a task in session-A
	ts.Create(ctx, model.Task{MessageID: "m1", WorkerID: "w1", Instruction: "w1-sessA", Type: model.TaskTypeImmediate, Status: model.TaskStatusPending, CreatedAt: now, UpdatedAt: now})
	ts.Create(ctx, model.Task{MessageID: "m2", WorkerID: "w1", Instruction: "w1-sessB", Type: model.TaskTypeImmediate, Status: model.TaskStatusCompleted, CreatedAt: now, UpdatedAt: now})
	ts.Create(ctx, model.Task{MessageID: "m1", WorkerID: "w2", Instruction: "w2-sessA", Type: model.TaskTypeImmediate, Status: model.TaskStatusPending, CreatedAt: now, UpdatedAt: now})

	tasks, err := ts.List(ctx, TaskFilter{WorkerID: "w1"})
	if err != nil {
		t.Fatalf("List by worker_id: %v", err)
	}
	if len(tasks) != 2 {
		t.Errorf("expected 2 tasks for w1 across sessions, got %d", len(tasks))
	}
	for _, task := range tasks {
		if task.WorkerID != "w1" {
			t.Errorf("expected all tasks to belong to w1, got worker_id=%q", task.WorkerID)
		}
	}
}

func TestTaskStore_List_ByWorkerIDAndSessionKey(t *testing.T) {
	ts, cleanup := newTaskStoreWithTwoWorkers(t)
	defer cleanup()
	ctx := context.Background()
	now := time.Now().UnixMilli()

	ts.Create(ctx, model.Task{MessageID: "m1", WorkerID: "w1", Instruction: "w1-sessA", Type: model.TaskTypeImmediate, Status: model.TaskStatusPending, CreatedAt: now, UpdatedAt: now})
	ts.Create(ctx, model.Task{MessageID: "m2", WorkerID: "w1", Instruction: "w1-sessB", Type: model.TaskTypeImmediate, Status: model.TaskStatusPending, CreatedAt: now, UpdatedAt: now})
	ts.Create(ctx, model.Task{MessageID: "m1", WorkerID: "w2", Instruction: "w2-sessA", Type: model.TaskTypeImmediate, Status: model.TaskStatusPending, CreatedAt: now, UpdatedAt: now})

	// w1 + session-A: should return only the 1 task that is both w1 AND in session-A
	tasks, err := ts.List(ctx, TaskFilter{WorkerID: "w1", SessionKey: "session-A"})
	if err != nil {
		t.Fatalf("List by worker_id+session_key: %v", err)
	}
	if len(tasks) != 1 {
		t.Errorf("expected 1 task for w1 in session-A, got %d", len(tasks))
	}
	if len(tasks) == 1 && tasks[0].Instruction != "w1-sessA" {
		t.Errorf("expected instruction 'w1-sessA', got %q", tasks[0].Instruction)
	}
}

func TestTaskStore_List_ByWorkerIDAndStatus(t *testing.T) {
	ts, cleanup := newTaskStoreWithTwoWorkers(t)
	defer cleanup()
	ctx := context.Background()
	now := time.Now().UnixMilli()

	ts.Create(ctx, model.Task{MessageID: "m1", WorkerID: "w1", Instruction: "pending", Type: model.TaskTypeImmediate, Status: model.TaskStatusPending, CreatedAt: now, UpdatedAt: now})
	ts.Create(ctx, model.Task{MessageID: "m2", WorkerID: "w1", Instruction: "completed", Type: model.TaskTypeImmediate, Status: model.TaskStatusCompleted, CreatedAt: now, UpdatedAt: now})
	ts.Create(ctx, model.Task{MessageID: "m1", WorkerID: "w2", Instruction: "w2-pending", Type: model.TaskTypeImmediate, Status: model.TaskStatusPending, CreatedAt: now, UpdatedAt: now})

	tasks, err := ts.List(ctx, TaskFilter{WorkerID: "w1", Status: "pending"})
	if err != nil {
		t.Fatalf("List by worker_id+status: %v", err)
	}
	if len(tasks) != 1 {
		t.Errorf("expected 1 pending task for w1, got %d", len(tasks))
	}
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
	tasks, err := ts.List(ctx, TaskFilter{SessionKey: "session-A"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(tasks) != 2 {
		t.Errorf("expected 2 tasks for session-A, got %d", len(tasks))
	}

	// List only pending tasks for session-A
	tasks, err = ts.List(ctx, TaskFilter{SessionKey: "session-A", Status: "pending"})
	if err != nil {
		t.Fatalf("List (pending): %v", err)
	}
	if len(tasks) != 1 {
		t.Errorf("expected 1 pending task for session-A, got %d", len(tasks))
	}

	// List with comma-separated status
	tasks, err = ts.List(ctx, TaskFilter{SessionKey: "session-A", Status: "pending,running"})
	if err != nil {
		t.Fatalf("List (pending,running): %v", err)
	}
	if len(tasks) != 2 {
		t.Errorf("expected 2 tasks for session-A with pending,running, got %d", len(tasks))
	}

	// List for session-B
	tasks, err = ts.List(ctx, TaskFilter{SessionKey: "session-B"})
	if err != nil {
		t.Fatalf("List session-B: %v", err)
	}
	if len(tasks) != 1 {
		t.Errorf("expected 1 task for session-B, got %d", len(tasks))
	}
}

func TestTaskStore_List_MessageIDFilter_CommaSeparatedStatus(t *testing.T) {
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

	tasks, err := ts.List(ctx, TaskFilter{MessageID: "m1", Status: "pending,running"})
	if err != nil {
		t.Fatalf("List: %v", err)
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

	n, err := ts.Cancel(ctx, CancelFilter{SessionKey: "session-A"})
	if err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if n != 2 {
		t.Errorf("expected 2 cancelled (pending+running), got %d", n)
	}

	// Verify: session-A completed task untouched
	tasksA, _ := ts.List(ctx, TaskFilter{SessionKey: "session-A", Status: "completed"})
	if len(tasksA) != 1 {
		t.Errorf("completed task should be untouched, got %d", len(tasksA))
	}

	// Verify: session-A cancelled tasks
	cancelledA, _ := ts.List(ctx, TaskFilter{SessionKey: "session-A", Status: "cancelled"})
	if len(cancelledA) != 2 {
		t.Errorf("expected 2 cancelled tasks, got %d", len(cancelledA))
	}

	// Verify: session-B unaffected
	tasksB, _ := ts.List(ctx, TaskFilter{SessionKey: "session-B", Status: "pending"})
	if len(tasksB) != 1 {
		t.Errorf("session-B task should be unaffected, got %d", len(tasksB))
	}
}

func TestTaskStore_ListBySessionAndWorker(t *testing.T) {
	ts, cleanup := newTaskStoreWithTwoSessions(t)
	defer cleanup()
	ctx := context.Background()
	now := time.Now().UnixMilli()

	// session-A: w1 (pending+running), w2 (running)
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
		MessageID: "m1", WorkerID: "w2", Instruction: "c",
		Type: model.TaskTypeImmediate, Status: model.TaskStatusRunning,
		CreatedAt: now, UpdatedAt: now,
	})
	// session-B: w1 running (should be excluded)
	ts.Create(ctx, model.Task{
		MessageID: "m2", WorkerID: "w1", Instruction: "d",
		Type: model.TaskTypeImmediate, Status: model.TaskStatusRunning,
		CreatedAt: now, UpdatedAt: now,
	})

	tasks, err := ts.List(ctx, TaskFilter{
		SessionKey: "session-A", WorkerID: "w1",
		Status: model.TaskStatusRunning, Type: model.TaskTypeImmediate,
	})
	if err != nil {
		t.Fatalf("List session+worker: %v", err)
	}
	if len(tasks) != 1 || tasks[0].Instruction != "b" {
		t.Errorf("expected only session-A/w1 running task 'b', got %+v", tasks)
	}

	all, err := ts.List(ctx, TaskFilter{SessionKey: "session-A", WorkerID: "w1"})
	if err != nil {
		t.Fatalf("List session+worker (all): %v", err)
	}
	if len(all) != 2 {
		t.Errorf("expected 2 tasks for session-A/w1, got %d", len(all))
	}
}

func TestTaskStore_CancelBySessionAndWorker(t *testing.T) {
	ts, cleanup := newTaskStoreWithTwoSessions(t)
	defer cleanup()
	ctx := context.Background()
	now := time.Now().UnixMilli()

	// session-A/w1: pending + running + completed
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
	// session-A/w2 running: must survive
	ts.Create(ctx, model.Task{
		MessageID: "m1", WorkerID: "w2", Instruction: "other-worker",
		Type: model.TaskTypeImmediate, Status: model.TaskStatusRunning,
		CreatedAt: now, UpdatedAt: now,
	})
	// session-B/w1 running: must survive
	ts.Create(ctx, model.Task{
		MessageID: "m2", WorkerID: "w1", Instruction: "other-session",
		Type: model.TaskTypeImmediate, Status: model.TaskStatusRunning,
		CreatedAt: now, UpdatedAt: now,
	})

	n, err := ts.Cancel(ctx, CancelFilter{
		SessionKey: "session-A", WorkerID: "w1", Type: model.TaskTypeImmediate,
	})
	if err != nil {
		t.Fatalf("Cancel session+worker: %v", err)
	}
	if n != 2 {
		t.Errorf("expected 2 cancelled (pending+running for session-A/w1), got %d", n)
	}

	// session-A/w2 untouched
	w2, _ := ts.List(ctx, TaskFilter{SessionKey: "session-A", WorkerID: "w2", Status: model.TaskStatusRunning})
	if len(w2) != 1 {
		t.Errorf("session-A/w2 running task should be untouched, got %d", len(w2))
	}
	// session-B/w1 untouched
	sb, _ := ts.List(ctx, TaskFilter{SessionKey: "session-B", WorkerID: "w1", Status: model.TaskStatusRunning})
	if len(sb) != 1 {
		t.Errorf("session-B/w1 running task should be untouched, got %d", len(sb))
	}
}

func TestTaskStore_CancelBySessionKey_ImmediateOnly(t *testing.T) {
	ts, cleanup := newTaskStoreWithTwoSessions(t)
	defer cleanup()
	ctx := context.Background()
	now := time.Now().UnixMilli()

	// immediate pending + running → should be cancelled
	ts.Create(ctx, model.Task{
		MessageID: "m1", WorkerID: "w1", Instruction: "imm-pending",
		Type: model.TaskTypeImmediate, Status: model.TaskStatusPending,
		CreatedAt: now, UpdatedAt: now,
	})
	ts.Create(ctx, model.Task{
		MessageID: "m1", WorkerID: "w1", Instruction: "imm-running",
		Type: model.TaskTypeImmediate, Status: model.TaskStatusRunning,
		CreatedAt: now, UpdatedAt: now,
	})
	// countdown pending → should survive
	ts.Create(ctx, model.Task{
		MessageID: "m1", WorkerID: "w1", Instruction: "countdown-pending",
		Type: model.TaskTypeCountdown, Status: model.TaskStatusPending,
		CreatedAt: now, UpdatedAt: now,
	})
	// scheduled pending → should survive
	ts.Create(ctx, model.Task{
		MessageID: "m1", WorkerID: "w1", Instruction: "scheduled-pending",
		Type: model.TaskTypeScheduled, Status: model.TaskStatusPending,
		CreatedAt: now, UpdatedAt: now,
	})

	n, err := ts.Cancel(ctx, CancelFilter{SessionKey: "session-A", Type: model.TaskTypeImmediate})
	if err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if n != 2 {
		t.Errorf("expected 2 cancelled (immediate only), got %d", n)
	}

	// countdown and scheduled tasks must still be pending
	surviving, _ := ts.List(ctx, TaskFilter{SessionKey: "session-A", Status: "pending"})
	if len(surviving) != 2 {
		t.Errorf("expected 2 surviving pending tasks (countdown+scheduled), got %d", len(surviving))
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
		Status:    model.TaskStatusRunning,
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
		Status:    model.TaskStatusRunning,
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
		Status:    model.TaskStatusCancelled,
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

func TestTaskStore_CountPendingByWorkerID(t *testing.T) {
	ts, cleanup := newTaskStoreForTest(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now().UnixMilli()

	ts.Create(ctx, model.Task{
		MessageID: "m1", WorkerID: "w1", Instruction: "do something",
		Type: model.TaskTypeImmediate, Status: model.TaskStatusPending,
		CreatedAt: now, UpdatedAt: now,
	})
	// Also create a completed task (should not count)
	ts.Create(ctx, model.Task{
		MessageID: "m1", WorkerID: "w1", Instruction: "done",
		Type: model.TaskTypeImmediate, Status: model.TaskStatusCompleted,
		CreatedAt: now, UpdatedAt: now,
	})

	count, err := ts.CountPendingByWorkerID(ctx, "w1")
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("expected 1 pending task, got %d", count)
	}
}

func TestTaskStore_CountAllByStatus(t *testing.T) {
	ts, cleanup := newTaskStoreForTest(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now().UnixMilli()

	ts.Create(ctx, model.Task{
		MessageID: "m1", WorkerID: "w1", Instruction: "task1",
		Type: model.TaskTypeImmediate, Status: model.TaskStatusPending,
		CreatedAt: now, UpdatedAt: now,
	})
	ts.Create(ctx, model.Task{
		MessageID: "m1", WorkerID: "w1", Instruction: "task2",
		Type: model.TaskTypeImmediate, Status: model.TaskStatusPending,
		CreatedAt: now, UpdatedAt: now,
	})

	counts, err := ts.CountAllByStatus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if counts["pending"] != 2 {
		t.Errorf("expected 2 pending, got %d", counts["pending"])
	}
}

func TestTaskStore_PeekDueScheduledTasks_ReturnsDueOnly(t *testing.T) {
	ts, cleanup := newTaskStoreForTest(t)
	defer cleanup()

	now := time.Now().UnixMilli()
	pastRun := now - 1000
	futureRun := now + 60_000
	expr := "0 * * * *"

	// Due: next_run_at in the past
	ts.Create(context.Background(), model.Task{
		MessageID: "m1", WorkerID: "w1", Instruction: "due",
		Type: model.TaskTypeScheduled, Status: model.TaskStatusPending,
		CronExpr: expr, NextRunAt: &pastRun,
		CreatedAt: now, UpdatedAt: now,
	})
	// Not due: next_run_at in the future
	ts.Create(context.Background(), model.Task{
		MessageID: "m1", WorkerID: "w1", Instruction: "future",
		Type: model.TaskTypeScheduled, Status: model.TaskStatusPending,
		CronExpr: expr, NextRunAt: &futureRun,
		CreatedAt: now, UpdatedAt: now,
	})
	// Due: next_run_at IS NULL
	ts.Create(context.Background(), model.Task{
		MessageID: "m1", WorkerID: "w1", Instruction: "null-run-at",
		Type: model.TaskTypeScheduled, Status: model.TaskStatusPending,
		CronExpr:  expr,
		CreatedAt: now, UpdatedAt: now,
	})

	tasks, err := ts.PeekDueScheduledTasks(context.Background(), now)
	if err != nil {
		t.Fatalf("PeekDueScheduledTasks: %v", err)
	}
	if len(tasks) != 2 {
		t.Fatalf("expected 2 due scheduled tasks, got %d", len(tasks))
	}
	for _, task := range tasks {
		if task.CronExpr != expr {
			t.Errorf("expected cron_expr %q, got %q", expr, task.CronExpr)
		}
	}
}

func TestTaskStore_ClaimDueTasks_ScheduledUsesProvidedNextRunAt(t *testing.T) {
	ts, cleanup := newTaskStoreForTest(t)
	defer cleanup()

	now := time.Now().UnixMilli()
	pastRun := now - 1000
	ts.Create(context.Background(), model.Task{
		MessageID: "m1", WorkerID: "w1", Instruction: "recurring",
		Type: model.TaskTypeScheduled, Status: model.TaskStatusPending,
		CronExpr: "0 * * * *", NextRunAt: &pastRun,
		CreatedAt: now, UpdatedAt: now,
	})

	// Peek to get the task ID
	peeked, err := ts.PeekDueScheduledTasks(context.Background(), now)
	if err != nil || len(peeked) != 1 {
		t.Fatalf("peek: %v, got %d tasks", err, len(peeked))
	}
	taskID := peeked[0].ID
	realNext := now + 3600_000 // 1h from now

	tasks, err := ts.ClaimDueTasks(context.Background(), now, map[string]int64{taskID: realNext})
	if err != nil {
		t.Fatalf("ClaimDueTasks: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected 1 claimed task, got %d", len(tasks))
	}

	// next_run_at should be the provided value, NOT +24h
	got, err := ts.GetByID(context.Background(), taskID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.NextRunAt == nil || *got.NextRunAt != realNext {
		t.Errorf("expected next_run_at=%d, got %v", realNext, got.NextRunAt)
	}
	// status should still be pending (scheduled tasks stay pending)
	if got.Status != model.TaskStatusPending {
		t.Errorf("scheduled task should remain pending, got %q", got.Status)
	}
}

func TestTaskStore_ClaimDueTasks_ImmediateUnaffectedByScheduledMap(t *testing.T) {
	ts, cleanup := newTaskStoreForTest(t)
	defer cleanup()

	now := time.Now().UnixMilli()
	ts.Create(context.Background(), model.Task{
		MessageID: "m1", WorkerID: "w1", Instruction: "now",
		Type: model.TaskTypeImmediate, Status: model.TaskStatusPending,
		CreatedAt: now, UpdatedAt: now,
	})

	tasks, err := ts.ClaimDueTasks(context.Background(), now, nil)
	if err != nil {
		t.Fatalf("ClaimDueTasks: %v", err)
	}
	if len(tasks) != 1 || tasks[0].Status != model.TaskStatusRunning {
		t.Fatalf("expected 1 running immediate task, got %+v", tasks)
	}
}

func TestTaskStore_CountTasks(t *testing.T) {
	ts, cleanup := newTaskStoreForTest(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now().UnixMilli()

	for i := 0; i < 3; i++ {
		ts.Create(ctx, model.Task{
			MessageID: "m1", WorkerID: "w1", Instruction: "task",
			Type: model.TaskTypeScheduled, Status: model.TaskStatusPending,
			CreatedAt: now, UpdatedAt: now,
		})
	}
	ts.Create(ctx, model.Task{
		MessageID: "m1", WorkerID: "w1", Instruction: "countdown",
		Type: model.TaskTypeCountdown, Status: model.TaskStatusPending,
		CreatedAt: now, UpdatedAt: now,
	})

	count, err := ts.CountTasks(ctx, TaskFilter{Type: "scheduled"})
	if err != nil {
		t.Fatalf("CountTasks: %v", err)
	}
	if count != 3 {
		t.Errorf("want 3, got %d", count)
	}

	count, err = ts.CountTasks(ctx, TaskFilter{Type: "scheduled,countdown"})
	if err != nil {
		t.Fatalf("CountTasks: %v", err)
	}
	if count != 4 {
		t.Errorf("want 4, got %d", count)
	}
}

func TestTaskStore_List_Pagination(t *testing.T) {
	ts, cleanup := newTaskStoreForTest(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now().UnixMilli()

	for i := 0; i < 5; i++ {
		ts.Create(ctx, model.Task{
			MessageID: "m1", WorkerID: "w1", Instruction: "task",
			Type: model.TaskTypeScheduled, Status: model.TaskStatusPending,
			CreatedAt: now, UpdatedAt: now,
		})
	}

	page1, err := ts.List(ctx, TaskFilter{Type: "scheduled", Limit: 2, Offset: 0})
	if err != nil {
		t.Fatalf("List page1: %v", err)
	}
	if len(page1) != 2 {
		t.Errorf("want 2, got %d", len(page1))
	}

	page2, err := ts.List(ctx, TaskFilter{Type: "scheduled", Limit: 2, Offset: 2})
	if err != nil {
		t.Fatalf("List page2: %v", err)
	}
	if len(page2) != 2 {
		t.Errorf("want 2, got %d", len(page2))
	}
}

func TestTaskStore_CompleteTask_Regular(t *testing.T) {
	ts, cleanup := newTaskStoreForTest(t)
	defer cleanup()

	ctx := context.Background()
	id, err := ts.Create(ctx, model.Task{
		MessageID:   "m1",
		WorkerID:    "w1",
		Instruction: "do it",
		Type:        model.TaskTypeImmediate,
		Status:      model.TaskStatusRunning,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := ts.CompleteTask(ctx, id); err != nil {
		t.Fatalf("CompleteTask: %v", err)
	}

	got, err := ts.GetByID(ctx, id)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Status != model.TaskStatusCompleted {
		t.Errorf("want status %q got %q", model.TaskStatusCompleted, got.Status)
	}
}

func TestTaskStore_CompleteTask_Scheduled(t *testing.T) {
	ts, cleanup := newTaskStoreForTest(t)
	defer cleanup()

	ctx := context.Background()
	cronExpr := "0 * * * *"
	id, err := ts.Create(ctx, model.Task{
		MessageID:   "m1",
		WorkerID:    "w1",
		Instruction: "do it",
		Type:        model.TaskTypeScheduled,
		Status:      model.TaskStatusRunning,
		CronExpr:    cronExpr,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := ts.CompleteTask(ctx, id); err != nil {
		t.Fatalf("CompleteTask: %v", err)
	}

	got, err := ts.GetByID(ctx, id)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	// Scheduled tasks reset to pending for the next cron run.
	if got.Status != model.TaskStatusPending {
		t.Errorf("want status %q got %q", model.TaskStatusPending, got.Status)
	}
}

func TestTaskStore_CompleteTask_Scheduled_NoCron_MarksAsCompleted(t *testing.T) {
	ts, cleanup := newTaskStoreForTest(t)
	defer cleanup()

	ctx := context.Background()
	id, err := ts.Create(ctx, model.Task{
		MessageID:   "m1",
		WorkerID:    "w1",
		Instruction: "do it",
		Type:        model.TaskTypeScheduled,
		Status:      model.TaskStatusRunning,
		// CronExpr intentionally empty
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := ts.CompleteTask(ctx, id); err != nil {
		t.Fatalf("CompleteTask: %v", err)
	}

	got, err := ts.GetByID(ctx, id)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Status != model.TaskStatusCompleted {
		t.Errorf("want status %q got %q", model.TaskStatusCompleted, got.Status)
	}
}

func TestTaskStore_CompleteTask_Scheduled_Cancelled_NoChange(t *testing.T) {
	ts, cleanup := newTaskStoreForTest(t)
	defer cleanup()

	ctx := context.Background()
	id, err := ts.Create(ctx, model.Task{
		MessageID:   "m1",
		WorkerID:    "w1",
		Instruction: "do it",
		Type:        model.TaskTypeScheduled,
		Status:      model.TaskStatusCancelled,
		CronExpr:    "0 * * * *",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := ts.CompleteTask(ctx, id); err != nil {
		t.Fatalf("CompleteTask: %v", err)
	}

	got, err := ts.GetByID(ctx, id)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Status != model.TaskStatusCancelled {
		t.Errorf("want status %q got %q", model.TaskStatusCancelled, got.Status)
	}
}

func TestTaskStore_List_ByTaskID(t *testing.T) {
	ts, cleanup := newTaskStoreForTest(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now().UnixMilli()

	id1, err := ts.Create(ctx, model.Task{
		MessageID: "m1", WorkerID: "w1", Instruction: "target",
		Type: model.TaskTypeImmediate, Status: model.TaskStatusCompleted,
		CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("Create target: %v", err)
	}
	if _, err := ts.Create(ctx, model.Task{
		MessageID: "m1", WorkerID: "w1", Instruction: "other",
		Type: model.TaskTypeImmediate, Status: model.TaskStatusCompleted,
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("Create other: %v", err)
	}

	tasks, err := ts.List(ctx, TaskFilter{TaskID: id1})
	if err != nil {
		t.Fatalf("List by task_id: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}
	if tasks[0].ID != id1 || tasks[0].Instruction != "target" {
		t.Fatalf("unexpected task: %+v", tasks[0])
	}

	total, err := ts.CountTasks(ctx, TaskFilter{TaskID: id1})
	if err != nil {
		t.Fatalf("CountTasks by task_id: %v", err)
	}
	if total != 1 {
		t.Fatalf("expected total 1, got %d", total)
	}
}

func TestTaskStore_HasActiveImmediateTasksByWorkerID(t *testing.T) {
	db, err := InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	defer db.Close()

	// Two workers, two messages
	db.Exec(`INSERT INTO bee_workers (id,name,work_dir,status,created_at,updated_at) VALUES ('w1','alice','/','idle',1,1)`)
	db.Exec(`INSERT INTO bee_workers (id,name,work_dir,status,created_at,updated_at) VALUES ('w2','bob','/','idle',1,1)`)
	db.Exec(`INSERT INTO bee_platform_messages (id,session_key,platform,content,raw,platform_msg_id,received_at,created_at,updated_at) VALUES ('m1','s','feishu','hi','','',1,1,1)`)
	db.Exec(`INSERT INTO bee_platform_messages (id,session_key,platform,content,raw,platform_msg_id,received_at,created_at,updated_at) VALUES ('m2','s2','feishu','hi','','',1,1,1)`)

	ts := NewTaskStore(db)
	ctx := context.Background()

	// no tasks → false
	active, err := ts.HasActiveImmediateTasksByWorkerID(ctx, "w1")
	if err != nil {
		t.Fatalf("HasActiveImmediateTasksByWorkerID: %v", err)
	}
	if active {
		t.Error("expected false with no tasks")
	}

	// create pending immediate task for w1
	now := time.Now().UnixMilli()
	id1, _ := ts.Create(ctx, model.Task{
		MessageID: "m1", WorkerID: "w1", Instruction: "go",
		Type: model.TaskTypeImmediate, Status: model.TaskStatusPending,
		CreatedAt: now, UpdatedAt: now,
	})

	active, err = ts.HasActiveImmediateTasksByWorkerID(ctx, "w1")
	if err != nil {
		t.Fatalf("HasActiveImmediateTasksByWorkerID: %v", err)
	}
	if !active {
		t.Error("expected true for w1 with pending immediate task")
	}

	// w2 must not be affected by w1's task
	active, err = ts.HasActiveImmediateTasksByWorkerID(ctx, "w2")
	if err != nil {
		t.Fatalf("HasActiveImmediateTasksByWorkerID w2: %v", err)
	}
	if active {
		t.Error("w2 should not be affected by w1's task")
	}

	// complete w1's task → false
	_ = ts.UpdateStatus(ctx, id1, model.TaskStatusCompleted)
	active, err = ts.HasActiveImmediateTasksByWorkerID(ctx, "w1")
	if err != nil {
		t.Fatalf("HasActiveImmediateTasksByWorkerID: %v", err)
	}
	if active {
		t.Error("expected false after completing w1 task")
	}

	// scheduled task for w1 must NOT count
	_, _ = ts.Create(ctx, model.Task{
		MessageID: "m2", WorkerID: "w1", Instruction: "cron",
		Type: model.TaskTypeScheduled, Status: model.TaskStatusPending,
		CreatedAt: now, UpdatedAt: now,
	})
	active, err = ts.HasActiveImmediateTasksByWorkerID(ctx, "w1")
	if err != nil {
		t.Fatalf("HasActiveImmediateTasksByWorkerID: %v", err)
	}
	if active {
		t.Error("scheduled task must not affect HasActiveImmediateTasksByWorkerID")
	}
}
