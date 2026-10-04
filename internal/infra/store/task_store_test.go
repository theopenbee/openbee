package store

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/theopenbee/openbee/internal/infra/model"
)

func newTaskStoreForTest(t *testing.T) *TaskStore {
	t.Helper()
	return NewTaskStore(newTestDB(t, seedWorkerSQL("w1"), seedMessageSQL("m1", "feishu:c:u")))
}

func newTaskStoreWithTwoSessions(t *testing.T) *TaskStore {
	t.Helper()
	return NewTaskStore(newTestDB(t, seedWorkerSQL("w1"),
		seedMessageSQL("m1", "session-A"), seedMessageSQL("m2", "session-B")))
}

// newTaskStoreWithTwoWorkers sets up: w1 and w2 workers; m1 (session-A) and m2 (session-B) messages.
func newTaskStoreWithTwoWorkers(t *testing.T) *TaskStore {
	t.Helper()
	return NewTaskStore(newTestDB(t, seedWorkerSQL("w1"), seedWorkerSQL("w2"),
		seedMessageSQL("m1", "session-A"), seedMessageSQL("m2", "session-B")))
}

func TestTaskStore_Create_And_Get(t *testing.T) {
	ts := newTaskStoreForTest(t)

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
	require.NoError(t, err)
	require.NotEmpty(t, id)

	got, err := ts.GetByID(context.Background(), id)
	require.NoError(t, err)
	assert.Equal(t, "do it", got.Instruction)
	assert.Equal(t, model.TaskTypeImmediate, got.Type)
}

func TestTaskStore_ClaimDueTasks_ImmediateOnly(t *testing.T) {
	ts := newTaskStoreForTest(t)

	now := time.Now().UnixMilli()
	seedTask(t, ts, model.Task{})

	tasks, err := ts.ClaimDueTasks(context.Background(), now, nil)
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	assert.Equal(t, model.TaskStatusRunning, tasks[0].Status)
}

func TestTaskStore_ClaimDueTasks_Idempotent(t *testing.T) {
	ts := newTaskStoreForTest(t)

	now := time.Now().UnixMilli()
	seedTask(t, ts, model.Task{})

	tasks1, _ := ts.ClaimDueTasks(context.Background(), now, nil)
	tasks2, _ := ts.ClaimDueTasks(context.Background(), now, nil)
	assert.Len(t, tasks1, 1, "first claim")
	assert.Empty(t, tasks2, "second claim should be empty (already running)")
}

func TestTaskStore_DeleteByMessageIDs(t *testing.T) {
	ts := newTaskStoreForTest(t)

	seedTask(t, ts, model.Task{})

	err := ts.DeletePendingByMessageIDs(context.Background(), []string{"m1"})
	require.NoError(t, err)

	// Verify no pending tasks remain
	tasks, _ := ts.ClaimDueTasks(context.Background(), time.Now().UnixMilli(), nil)
	assert.Empty(t, tasks)
}

func TestTaskStore_List_ByMessageIDFilter(t *testing.T) {
	ts := newTaskStoreForTest(t)

	seedTask(t, ts, model.Task{Instruction: "a"})
	seedTask(t, ts, model.Task{Instruction: "b", Type: model.TaskTypeCountdown})

	tasks, err := ts.List(context.Background(), TaskFilter{MessageID: "m1"})
	require.NoError(t, err)
	assert.Len(t, tasks, 2)
}

func TestTaskStore_UpdateStatus(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name        string
		from        string
		ifRunning   bool // exercise UpdateStatusIfRunning instead of UpdateStatus
		to          string
		wantChanged bool
		want        string
	}{
		{"SetsCompleted", model.TaskStatusRunning, false, model.TaskStatusCompleted, true, model.TaskStatusCompleted},
		{"SetsFailed", model.TaskStatusRunning, false, model.TaskStatusFailed, true, model.TaskStatusFailed},
		{"IfRunning_SkipsWhenNotRunning", model.TaskStatusPending, true, model.TaskStatusCompleted, false, model.TaskStatusPending},
		{"IfRunning_Transitions", model.TaskStatusRunning, true, model.TaskStatusFailed, true, model.TaskStatusFailed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ts := newTaskStoreForTest(t)
			id := seedTask(t, ts, model.Task{Status: tc.from})
			if tc.ifRunning {
				changed, err := ts.UpdateStatusIfRunning(ctx, id, tc.to)
				require.NoError(t, err)
				require.Equal(t, tc.wantChanged, changed)
			} else {
				require.NoError(t, ts.UpdateStatus(ctx, id, tc.to))
			}
			got, err := ts.GetByID(ctx, id)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got.Status)
		})
	}
}

func TestTaskStore_List_ByWorker(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name             string
		seed             func(t *testing.T, ts *TaskStore)
		filter           TaskFilter
		wantInstructions []string
	}{
		{
			name: "ByWorkerID",
			seed: func(t *testing.T, ts *TaskStore) {
				// w1 has tasks in session-A and session-B; w2 has a task in session-A
				seedTask(t, ts, model.Task{WorkerID: "w1", MessageID: "m1", Instruction: "w1-sessA"})
				seedTask(t, ts, model.Task{WorkerID: "w1", MessageID: "m2", Instruction: "w1-sessB", Status: model.TaskStatusCompleted})
				seedTask(t, ts, model.Task{WorkerID: "w2", MessageID: "m1", Instruction: "w2-sessA"})
			},
			filter:           TaskFilter{WorkerID: "w1"},
			wantInstructions: []string{"w1-sessA", "w1-sessB"},
		},
		{
			name: "ByWorkerIDAndSessionKey",
			seed: func(t *testing.T, ts *TaskStore) {
				seedTask(t, ts, model.Task{WorkerID: "w1", MessageID: "m1", Instruction: "w1-sessA"})
				seedTask(t, ts, model.Task{WorkerID: "w1", MessageID: "m2", Instruction: "w1-sessB"})
				seedTask(t, ts, model.Task{WorkerID: "w2", MessageID: "m1", Instruction: "w2-sessA"})
			},
			// w1 + session-A: should return only the 1 task that is both w1 AND in session-A
			filter:           TaskFilter{WorkerID: "w1", SessionKey: "session-A"},
			wantInstructions: []string{"w1-sessA"},
		},
		{
			name: "ByWorkerIDAndStatus",
			seed: func(t *testing.T, ts *TaskStore) {
				seedTask(t, ts, model.Task{WorkerID: "w1", MessageID: "m1", Instruction: "pending"})
				seedTask(t, ts, model.Task{WorkerID: "w1", MessageID: "m2", Instruction: "completed", Status: model.TaskStatusCompleted})
				seedTask(t, ts, model.Task{WorkerID: "w2", MessageID: "m1", Instruction: "w2-pending"})
			},
			filter:           TaskFilter{WorkerID: "w1", Status: "pending"},
			wantInstructions: []string{"pending"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ts := newTaskStoreWithTwoWorkers(t)
			tc.seed(t, ts)

			tasks, err := ts.List(ctx, tc.filter)
			require.NoError(t, err)
			instructions := make([]string, len(tasks))
			for i, task := range tasks {
				instructions[i] = task.Instruction
				assert.Equal(t, "w1", task.WorkerID, "expected all tasks to belong to w1")
			}
			assert.ElementsMatch(t, tc.wantInstructions, instructions)
		})
	}
}

func TestTaskStore_ListBySessionKey(t *testing.T) {
	ts := newTaskStoreWithTwoSessions(t)
	ctx := context.Background()

	// Create tasks in session-A: one pending, one running
	seedTask(t, ts, model.Task{Instruction: "a"})
	seedTask(t, ts, model.Task{Instruction: "b", Status: model.TaskStatusRunning})
	// Create task in session-B
	seedTask(t, ts, model.Task{MessageID: "m2", Instruction: "c"})

	// List all tasks for session-A
	tasks, err := ts.List(ctx, TaskFilter{SessionKey: "session-A"})
	require.NoError(t, err)
	assert.Len(t, tasks, 2)

	// List only pending tasks for session-A
	tasks, err = ts.List(ctx, TaskFilter{SessionKey: "session-A", Status: "pending"})
	require.NoError(t, err)
	assert.Len(t, tasks, 1)

	// List with comma-separated status
	tasks, err = ts.List(ctx, TaskFilter{SessionKey: "session-A", Status: "pending,running"})
	require.NoError(t, err)
	assert.Len(t, tasks, 2)

	// List for session-B
	tasks, err = ts.List(ctx, TaskFilter{SessionKey: "session-B"})
	require.NoError(t, err)
	assert.Len(t, tasks, 1)
}

func TestTaskStore_List_MessageIDFilter_CommaSeparatedStatus(t *testing.T) {
	ts := newTaskStoreForTest(t)
	ctx := context.Background()

	seedTask(t, ts, model.Task{Instruction: "a"})
	seedTask(t, ts, model.Task{Instruction: "b", Status: model.TaskStatusRunning})
	seedTask(t, ts, model.Task{Instruction: "c", Status: model.TaskStatusCompleted})

	tasks, err := ts.List(ctx, TaskFilter{MessageID: "m1", Status: "pending,running"})
	require.NoError(t, err)
	assert.Len(t, tasks, 2)
}

func TestTaskStore_CancelBySessionKey(t *testing.T) {
	ts := newTaskStoreWithTwoSessions(t)
	ctx := context.Background()

	// Create tasks in session-A: pending + running + completed
	seedTask(t, ts, model.Task{Instruction: "a"})
	seedTask(t, ts, model.Task{Instruction: "b", Status: model.TaskStatusRunning})
	seedTask(t, ts, model.Task{Instruction: "c", Status: model.TaskStatusCompleted})
	// Task in session-B (should not be affected)
	seedTask(t, ts, model.Task{MessageID: "m2", Instruction: "d"})

	n, err := ts.Cancel(ctx, CancelFilter{SessionKey: "session-A"})
	require.NoError(t, err)
	assert.EqualValues(t, 2, n, "expected 2 cancelled (pending+running)")

	// Verify: session-A completed task untouched
	tasksA, _ := ts.List(ctx, TaskFilter{SessionKey: "session-A", Status: "completed"})
	assert.Len(t, tasksA, 1, "completed task should be untouched")

	// Verify: session-A cancelled tasks
	cancelledA, _ := ts.List(ctx, TaskFilter{SessionKey: "session-A", Status: "cancelled"})
	assert.Len(t, cancelledA, 2)

	// Verify: session-B unaffected
	tasksB, _ := ts.List(ctx, TaskFilter{SessionKey: "session-B", Status: "pending"})
	assert.Len(t, tasksB, 1, "session-B task should be unaffected")
}

func TestTaskStore_ListBySessionAndWorker(t *testing.T) {
	ts := newTaskStoreWithTwoWorkers(t)
	ctx := context.Background()

	// session-A: w1 (pending+running), w2 (running)
	seedTask(t, ts, model.Task{Instruction: "a"})
	seedTask(t, ts, model.Task{Instruction: "b", Status: model.TaskStatusRunning})
	seedTask(t, ts, model.Task{WorkerID: "w2", Instruction: "c", Status: model.TaskStatusRunning})
	// session-B: w1 running (should be excluded)
	seedTask(t, ts, model.Task{MessageID: "m2", Instruction: "d", Status: model.TaskStatusRunning})

	tasks, err := ts.List(ctx, TaskFilter{
		SessionKey: "session-A", WorkerID: "w1",
		Status: model.TaskStatusRunning, Type: model.TaskTypeImmediate,
	})
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	assert.Equal(t, "b", tasks[0].Instruction)

	all, err := ts.List(ctx, TaskFilter{SessionKey: "session-A", WorkerID: "w1"})
	require.NoError(t, err)
	assert.Len(t, all, 2)
}

func TestTaskStore_CancelBySessionAndWorker(t *testing.T) {
	ts := newTaskStoreWithTwoWorkers(t)
	ctx := context.Background()

	// session-A/w1: pending + running + completed
	seedTask(t, ts, model.Task{Instruction: "a"})
	seedTask(t, ts, model.Task{Instruction: "b", Status: model.TaskStatusRunning})
	seedTask(t, ts, model.Task{Instruction: "c", Status: model.TaskStatusCompleted})
	// session-A/w2 running: must survive
	seedTask(t, ts, model.Task{WorkerID: "w2", Instruction: "other-worker", Status: model.TaskStatusRunning})
	// session-B/w1 running: must survive
	seedTask(t, ts, model.Task{MessageID: "m2", Instruction: "other-session", Status: model.TaskStatusRunning})

	n, err := ts.Cancel(ctx, CancelFilter{
		SessionKey: "session-A", WorkerID: "w1", Type: model.TaskTypeImmediate,
	})
	require.NoError(t, err)
	assert.EqualValues(t, 2, n, "expected 2 cancelled (pending+running for session-A/w1)")

	// session-A/w2 untouched
	w2, _ := ts.List(ctx, TaskFilter{SessionKey: "session-A", WorkerID: "w2", Status: model.TaskStatusRunning})
	assert.Len(t, w2, 1, "session-A/w2 running task should be untouched")
	// session-B/w1 untouched
	sb, _ := ts.List(ctx, TaskFilter{SessionKey: "session-B", WorkerID: "w1", Status: model.TaskStatusRunning})
	assert.Len(t, sb, 1, "session-B/w1 running task should be untouched")
}

func TestTaskStore_CancelBySessionKey_ImmediateOnly(t *testing.T) {
	ts := newTaskStoreWithTwoSessions(t)
	ctx := context.Background()

	// immediate pending + running → should be cancelled
	seedTask(t, ts, model.Task{Instruction: "imm-pending"})
	seedTask(t, ts, model.Task{Instruction: "imm-running", Status: model.TaskStatusRunning})
	// countdown pending → should survive
	seedTask(t, ts, model.Task{Instruction: "countdown-pending", Type: model.TaskTypeCountdown})
	// scheduled pending → should survive
	seedTask(t, ts, model.Task{Instruction: "scheduled-pending", Type: model.TaskTypeScheduled})

	n, err := ts.Cancel(ctx, CancelFilter{SessionKey: "session-A", Type: model.TaskTypeImmediate})
	require.NoError(t, err)
	assert.EqualValues(t, 2, n, "expected 2 cancelled (immediate only)")

	// countdown and scheduled tasks must still be pending
	surviving, _ := ts.List(ctx, TaskFilter{SessionKey: "session-A", Status: "pending"})
	assert.Len(t, surviving, 2, "expected 2 surviving pending tasks (countdown+scheduled)")
}

func TestTaskStore_ResetRunningToPending(t *testing.T) {
	ts := newTaskStoreForTest(t)

	id := seedTask(t, ts, model.Task{Status: model.TaskStatusRunning})

	n, err := ts.ResetRunningToPending(context.Background())
	require.NoError(t, err)
	assert.EqualValues(t, 1, n)

	got, _ := ts.GetByID(context.Background(), id)
	assert.Equal(t, model.TaskStatusPending, got.Status)
}

func TestTaskStore_FailTask(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name       string
		seed       model.Task
		wantStatus string
	}{
		{"RegularTask_MarksAsFailed", model.Task{Instruction: "x", Status: model.TaskStatusRunning}, model.TaskStatusFailed},
		{"ScheduledTask_WithCron_ResetsToPending", model.Task{Instruction: "x", Type: model.TaskTypeScheduled, CronExpr: "* * * * *", Status: model.TaskStatusRunning}, model.TaskStatusPending},
		{"ScheduledTask_NoCron_MarksAsFailed", model.Task{Instruction: "x", Type: model.TaskTypeScheduled, Status: model.TaskStatusRunning}, model.TaskStatusFailed},
		{"ScheduledTask_Cancelled_NoChange", model.Task{Instruction: "x", Type: model.TaskTypeScheduled, CronExpr: "* * * * *", Status: model.TaskStatusCancelled}, model.TaskStatusCancelled},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ts := newTaskStoreForTest(t)
			id := seedTask(t, ts, tc.seed)

			require.NoError(t, ts.FailTask(ctx, id))

			got, err := ts.GetByID(ctx, id)
			require.NoError(t, err)
			assert.Equal(t, tc.wantStatus, got.Status)
		})
	}
}

func TestTaskStore_Count(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name  string
		seed  func(t *testing.T, ts *TaskStore)
		count func(*TaskStore) (int, error)
		want  int
	}{
		{
			name: "PendingByWorkerID",
			seed: func(t *testing.T, ts *TaskStore) {
				seedTask(t, ts, model.Task{Instruction: "do something"})
				// Also create a completed task (should not count)
				seedTask(t, ts, model.Task{Instruction: "done", Status: model.TaskStatusCompleted})
			},
			count: func(ts *TaskStore) (int, error) { return ts.CountPendingByWorkerID(ctx, "w1") },
			want:  1,
		},
		{
			name: "AllByStatus",
			seed: func(t *testing.T, ts *TaskStore) {
				seedTask(t, ts, model.Task{Instruction: "task1"})
				seedTask(t, ts, model.Task{Instruction: "task2"})
			},
			count: func(ts *TaskStore) (int, error) {
				counts, err := ts.CountAllByStatus(ctx)
				return counts[model.TaskStatusPending], err
			},
			want: 2,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ts := newTaskStoreForTest(t)
			tc.seed(t, ts)

			got, err := tc.count(ts)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestTaskStore_PeekDueScheduledTasks_ReturnsDueOnly(t *testing.T) {
	ts := newTaskStoreForTest(t)

	now := time.Now().UnixMilli()
	pastRun := now - 1000
	futureRun := now + 60_000
	expr := "0 * * * *"

	// Due: next_run_at in the past
	seedTask(t, ts, model.Task{Instruction: "due", Type: model.TaskTypeScheduled, CronExpr: expr, NextRunAt: &pastRun})
	// Not due: next_run_at in the future
	seedTask(t, ts, model.Task{Instruction: "future", Type: model.TaskTypeScheduled, CronExpr: expr, NextRunAt: &futureRun})
	// Due: next_run_at IS NULL
	seedTask(t, ts, model.Task{Instruction: "null-run-at", Type: model.TaskTypeScheduled, CronExpr: expr})

	tasks, err := ts.PeekDueScheduledTasks(context.Background(), now)
	require.NoError(t, err)
	require.Len(t, tasks, 2)
	for _, task := range tasks {
		assert.Equal(t, expr, task.CronExpr)
	}
}

func TestTaskStore_ClaimDueTasks_ScheduledUsesProvidedNextRunAt(t *testing.T) {
	ts := newTaskStoreForTest(t)

	now := time.Now().UnixMilli()
	pastRun := now - 1000
	seedTask(t, ts, model.Task{Instruction: "recurring", Type: model.TaskTypeScheduled, CronExpr: "0 * * * *", NextRunAt: &pastRun})

	// Peek to get the task ID
	peeked, err := ts.PeekDueScheduledTasks(context.Background(), now)
	require.NoError(t, err)
	require.Len(t, peeked, 1)
	taskID := peeked[0].ID
	realNext := now + 3600_000 // 1h from now

	tasks, err := ts.ClaimDueTasks(context.Background(), now, map[string]int64{taskID: realNext})
	require.NoError(t, err)
	require.Len(t, tasks, 1)

	// next_run_at should be the provided value, NOT +24h
	got, err := ts.GetByID(context.Background(), taskID)
	require.NoError(t, err)
	if assert.NotNil(t, got.NextRunAt) {
		assert.Equal(t, realNext, *got.NextRunAt)
	}
	// status should still be pending (scheduled tasks stay pending)
	assert.Equal(t, model.TaskStatusPending, got.Status)
}

func TestTaskStore_ClaimDueTasks_ImmediateUnaffectedByScheduledMap(t *testing.T) {
	ts := newTaskStoreForTest(t)

	now := time.Now().UnixMilli()
	seedTask(t, ts, model.Task{Instruction: "now"})

	tasks, err := ts.ClaimDueTasks(context.Background(), now, nil)
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	require.Equal(t, model.TaskStatusRunning, tasks[0].Status)
}

func TestTaskStore_CountTasks(t *testing.T) {
	ts := newTaskStoreForTest(t)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		seedTask(t, ts, model.Task{Instruction: "task", Type: model.TaskTypeScheduled})
	}
	seedTask(t, ts, model.Task{Instruction: "countdown", Type: model.TaskTypeCountdown})

	count, err := ts.CountTasks(ctx, TaskFilter{Type: "scheduled"})
	require.NoError(t, err)
	assert.Equal(t, 3, count)

	count, err = ts.CountTasks(ctx, TaskFilter{Type: "scheduled,countdown"})
	require.NoError(t, err)
	assert.Equal(t, 4, count)
}

func TestTaskStore_List_Pagination(t *testing.T) {
	ts := newTaskStoreForTest(t)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		seedTask(t, ts, model.Task{Instruction: "task", Type: model.TaskTypeScheduled})
	}

	page1, err := ts.List(ctx, TaskFilter{Type: "scheduled", Limit: 2, Offset: 0})
	require.NoError(t, err)
	assert.Len(t, page1, 2)

	page2, err := ts.List(ctx, TaskFilter{Type: "scheduled", Limit: 2, Offset: 2})
	require.NoError(t, err)
	assert.Len(t, page2, 2)
}

func TestTaskStore_CompleteTask(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name       string
		seed       model.Task
		wantStatus string
	}{
		{"Regular", model.Task{Instruction: "do it", Status: model.TaskStatusRunning}, model.TaskStatusCompleted},
		// Scheduled tasks reset to pending for the next cron run.
		{"Scheduled", model.Task{Instruction: "do it", Type: model.TaskTypeScheduled, Status: model.TaskStatusRunning, CronExpr: "0 * * * *"}, model.TaskStatusPending},
		{"Scheduled_NoCron_MarksAsCompleted", model.Task{Instruction: "do it", Type: model.TaskTypeScheduled, Status: model.TaskStatusRunning}, model.TaskStatusCompleted},
		{"Scheduled_Cancelled_NoChange", model.Task{Instruction: "do it", Type: model.TaskTypeScheduled, Status: model.TaskStatusCancelled, CronExpr: "0 * * * *"}, model.TaskStatusCancelled},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ts := newTaskStoreForTest(t)
			id := seedTask(t, ts, tc.seed)

			require.NoError(t, ts.CompleteTask(ctx, id))

			got, err := ts.GetByID(ctx, id)
			require.NoError(t, err)
			assert.Equal(t, tc.wantStatus, got.Status)
		})
	}
}

func TestTaskStore_List_ByTaskID(t *testing.T) {
	ts := newTaskStoreForTest(t)
	ctx := context.Background()

	id1 := seedTask(t, ts, model.Task{Instruction: "target", Status: model.TaskStatusCompleted})
	seedTask(t, ts, model.Task{Instruction: "other", Status: model.TaskStatusCompleted})

	tasks, err := ts.List(ctx, TaskFilter{TaskID: id1})
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	require.Equal(t, id1, tasks[0].ID)
	require.Equal(t, "target", tasks[0].Instruction)

	total, err := ts.CountTasks(ctx, TaskFilter{TaskID: id1})
	require.NoError(t, err)
	assert.Equal(t, 1, total)
}

func TestTaskStore_HasActiveImmediateTasksByWorkerID(t *testing.T) {
	// Two workers, two messages
	db := newTestDB(t, seedWorkerSQL("w1"), seedWorkerSQL("w2"), seedMessageSQL("m1", "s"), seedMessageSQL("m2", "s2"))
	ts := NewTaskStore(db)
	ctx := context.Background()

	// no tasks → false
	active, err := ts.HasActiveImmediateTasksByWorkerID(ctx, "w1")
	require.NoError(t, err)
	assert.False(t, active, "expected false with no tasks")

	// create pending immediate task for w1
	id1 := seedTask(t, ts, model.Task{})

	active, err = ts.HasActiveImmediateTasksByWorkerID(ctx, "w1")
	require.NoError(t, err)
	assert.True(t, active, "expected true for w1 with pending immediate task")

	// w2 must not be affected by w1's task
	active, err = ts.HasActiveImmediateTasksByWorkerID(ctx, "w2")
	require.NoError(t, err)
	assert.False(t, active, "w2 should not be affected by w1's task")

	// complete w1's task → false
	_ = ts.UpdateStatus(ctx, id1, model.TaskStatusCompleted)
	active, err = ts.HasActiveImmediateTasksByWorkerID(ctx, "w1")
	require.NoError(t, err)
	assert.False(t, active, "expected false after completing w1 task")

	// scheduled task for w1 must NOT count
	seedTask(t, ts, model.Task{MessageID: "m2", Instruction: "cron", Type: model.TaskTypeScheduled})
	active, err = ts.HasActiveImmediateTasksByWorkerID(ctx, "w1")
	require.NoError(t, err)
	assert.False(t, active, "scheduled task must not affect HasActiveImmediateTasksByWorkerID")
}
