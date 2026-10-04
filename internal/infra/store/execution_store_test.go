package store

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/theopenbee/openbee/internal/infra/model"
)

func newTestExecutionStore(t *testing.T) *ExecutionStore {
	t.Helper()
	db := newTestDB(t, seedWorkerSQL("w1"))
	return NewExecutionStore(db, t.TempDir())
}

func TestExecutionStore_GetRunningByTaskID(t *testing.T) {
	s := newTestExecutionStore(t)
	running, _ := s.Create(ExecutionCreate{WorkerID: "w1", TaskID: "task-1", TriggerInput: "in", SessionID: "sess-1", Engine: "claude"})
	require.NoError(t, s.UpdateStatus(running.ID, model.ExecStatusRunning))

	got, err := s.GetRunningByTaskID(context.Background(), "task-1")
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, running.ID, got.ID)

	none, err := s.GetRunningByTaskID(context.Background(), "task-x")
	require.NoError(t, err)
	assert.Nil(t, none, "want nil for unknown task")

	// A pending (never-running) execution must not be returned.
	_, _ = s.Create(ExecutionCreate{WorkerID: "w1", TaskID: "task-2", TriggerInput: "in", SessionID: "sess-2", Engine: "claude"})
	got2, err := s.GetRunningByTaskID(context.Background(), "task-2")
	require.NoError(t, err)
	assert.Nil(t, got2, "want nil for task with only a pending execution")
}

func TestExecutionStore_ListByTaskIDs(t *testing.T) {
	s := newTestExecutionStore(t)
	e1, _ := s.Create(ExecutionCreate{WorkerID: "w1", TaskID: "task-1", TriggerInput: "first", SessionID: "sess-1", Engine: "claude"})
	e2, _ := s.Create(ExecutionCreate{WorkerID: "w1", TaskID: "task-1", TriggerInput: "second", SessionID: "sess-2", Engine: "claude"})
	_, _ = s.Create(ExecutionCreate{WorkerID: "w1", TaskID: "task-2", TriggerInput: "other", SessionID: "sess-3", Engine: "claude"})

	m, err := s.ListByTaskIDs(context.Background(), []string{"task-1", "task-2"}, 0)
	require.NoError(t, err)
	require.Len(t, m["task-1"], 2)
	// Newest-first: e2 was inserted after e1; the rowid DESC tiebreak makes this
	// deterministic even when both rows share the same started_at millisecond.
	assert.Equal(t, e2.ID, m["task-1"][0].ID)
	assert.Equal(t, e1.ID, m["task-1"][1].ID)
	assert.Len(t, m["task-2"], 1)

	// Task ids with no executions are absent from the returned map.
	withMissing, err := s.ListByTaskIDs(context.Background(), []string{"task-1", "task-none"}, 0)
	require.NoError(t, err)
	_, ok := withMissing["task-none"]
	assert.False(t, ok, "want task-none absent")
}

func TestExecutionStore_ListByTaskIDs_LimitsExecutionsPerTask(t *testing.T) {
	s := newTestExecutionStore(t)
	_, err := s.Create(ExecutionCreate{WorkerID: "w1", TaskID: "task-1", TriggerInput: "first", SessionID: "sess-1", Engine: "claude"})
	require.NoError(t, err)
	e2, _ := s.Create(ExecutionCreate{WorkerID: "w1", TaskID: "task-1", TriggerInput: "second", SessionID: "sess-2", Engine: "claude"})
	e3, _ := s.Create(ExecutionCreate{WorkerID: "w1", TaskID: "task-1", TriggerInput: "third", SessionID: "sess-3", Engine: "claude"})
	_, err = s.Create(ExecutionCreate{WorkerID: "w1", TaskID: "task-2", TriggerInput: "other", SessionID: "sess-4", Engine: "claude"})
	require.NoError(t, err)

	got, err := s.ListByTaskIDs(context.Background(), []string{"task-1", "task-2"}, 2)
	require.NoError(t, err)
	require.Len(t, got["task-1"], 2)
	require.Equal(t, e3.ID, got["task-1"][0].ID)
	require.Equal(t, e2.ID, got["task-1"][1].ID)
	require.Len(t, got["task-2"], 1)
}

func TestExecutionStore_ListByTaskIDs_ZeroLimitReturnsAll(t *testing.T) {
	s := newTestExecutionStore(t)
	e1, _ := s.Create(ExecutionCreate{WorkerID: "w1", TaskID: "task-1", TriggerInput: "first", SessionID: "sess-1", Engine: "claude"})
	e2, _ := s.Create(ExecutionCreate{WorkerID: "w1", TaskID: "task-1", TriggerInput: "second", SessionID: "sess-2", Engine: "claude"})
	e3, _ := s.Create(ExecutionCreate{WorkerID: "w1", TaskID: "task-1", TriggerInput: "third", SessionID: "sess-3", Engine: "claude"})

	got, err := s.ListByTaskIDs(context.Background(), []string{"task-1"}, 0)
	require.NoError(t, err)
	require.Len(t, got["task-1"], 3)
	require.Equal(t, e3.ID, got["task-1"][0].ID)
	require.Equal(t, e2.ID, got["task-1"][1].ID)
	require.Equal(t, e1.ID, got["task-1"][2].ID)
}

func TestExecutionStore_RunningExecIDsByTaskIDs_ChunksLargeInput(t *testing.T) {
	s := newTestExecutionStore(t)
	ctx := context.Background()

	const n = inListChunkSize*3 + 17
	taskIDs := make([]string, n)
	for i := range taskIDs {
		taskIDs[i] = fmt.Sprintf("task-%05d", i)
	}

	hitA, _ := s.Create(ExecutionCreate{WorkerID: "w1", TaskID: taskIDs[1], TriggerInput: "a", SessionID: "sess-a", Engine: "claude"})
	require.NoError(t, s.UpdateStatus(hitA.ID, model.ExecStatusRunning))
	hitB, _ := s.Create(ExecutionCreate{WorkerID: "w1", TaskID: taskIDs[n-2], TriggerInput: "b", SessionID: "sess-b", Engine: "claude"})
	require.NoError(t, s.UpdateStatus(hitB.ID, model.ExecStatusRunning))

	got, err := s.RunningExecIDsByTaskIDs(ctx, taskIDs)
	require.NoError(t, err)
	assert.Equal(t, hitA.ID, got[taskIDs[1]])
	assert.Equal(t, hitB.ID, got[taskIDs[n-2]])
}

func TestExecutionStore_ListByTaskIDs_ChunksLargeInput(t *testing.T) {
	s := newTestExecutionStore(t)
	ctx := context.Background()

	const n = inListChunkSize*2 + 5
	taskIDs := make([]string, n)
	for i := range taskIDs {
		taskIDs[i] = fmt.Sprintf("task-%05d", i)
	}

	e1, _ := s.Create(ExecutionCreate{WorkerID: "w1", TaskID: taskIDs[0], TriggerInput: "first", SessionID: "sess-1", Engine: "claude"})
	e2, _ := s.Create(ExecutionCreate{WorkerID: "w1", TaskID: taskIDs[n-1], TriggerInput: "last", SessionID: "sess-2", Engine: "claude"})

	got, err := s.ListByTaskIDs(ctx, taskIDs, 5)
	require.NoError(t, err)
	if assert.Len(t, got[taskIDs[0]], 1) {
		assert.Equal(t, e1.ID, got[taskIDs[0]][0].ID)
	}
	if assert.Len(t, got[taskIDs[n-1]], 1) {
		assert.Equal(t, e2.ID, got[taskIDs[n-1]][0].ID)
	}
}

func TestExecutionStore_CreateAndGet(t *testing.T) {
	s := newTestExecutionStore(t)
	exec, err := s.Create(ExecutionCreate{WorkerID: "w1", TaskID: "task-1", TriggerInput: "test message", SessionID: uuid.New().String(), Engine: "claude"})
	require.NoError(t, err)
	assert.Equal(t, model.ExecStatusPending, exec.Status)
	assert.NotEmpty(t, exec.SessionID)

	got, err := s.GetByID(exec.ID)
	require.NoError(t, err)
	require.NotNil(t, got.WorkerID)
	assert.Equal(t, "w1", *got.WorkerID)
	assert.Equal(t, "claude", got.Engine)
	assert.Equal(t, "task-1", got.TaskID)
}

func TestExecutionStore_UpdateStatus(t *testing.T) {
	db := newTestDB(t)
	ws := NewWorkerStore(db)
	es := NewExecutionStore(db, t.TempDir())

	w, _ := ws.Create(model.Worker{Name: "Bot", WorkDir: "/tmp/bot"})
	exec, _ := es.Create(ExecutionCreate{WorkerID: w.ID, TriggerInput: "test message", SessionID: uuid.New().String(), Engine: "claude"})

	err := es.UpdateStatus(exec.ID, model.ExecStatusRunning)
	require.NoError(t, err)
	got, _ := es.GetByID(exec.ID)
	assert.Equal(t, model.ExecStatusRunning, got.Status)
}

func TestExecutionStore_Create_StartedAtMillisecondPrecision(t *testing.T) {
	db := newTestDB(t)
	ws := NewWorkerStore(db)
	es := NewExecutionStore(db, t.TempDir())

	w, _ := ws.Create(model.Worker{Name: "Bot", WorkDir: "/tmp/bot"})
	exec, err := es.Create(ExecutionCreate{WorkerID: w.ID, TriggerInput: "test", SessionID: uuid.New().String(), Engine: "claude"})
	require.NoError(t, err)

	var startedAt int64
	err = db.QueryRow(`SELECT started_at FROM bee_executions WHERE id = ?`, exec.ID).Scan(&startedAt)
	require.NoError(t, err)
	assert.Positive(t, startedAt, "want positive Unix millisecond timestamp")

	assert.NotNil(t, exec.StartedAt, "exec.StartedAt must not be nil")
}

func TestExecutionStore_UpdateResult_CompletedAtMillisecondPrecision(t *testing.T) {
	db := newTestDB(t)
	ws := NewWorkerStore(db)
	es := NewExecutionStore(db, t.TempDir())

	w, _ := ws.Create(model.Worker{Name: "Bot", WorkDir: "/tmp/bot"})
	exec, _ := es.Create(ExecutionCreate{WorkerID: w.ID, TriggerInput: "test", SessionID: uuid.New().String(), Engine: "claude"})

	require.NoError(t, es.UpdateResult(exec.ID, "output", model.ExecStatusCompleted))

	var completedAt int64
	err := db.QueryRow(`SELECT completed_at FROM bee_executions WHERE id = ?`, exec.ID).Scan(&completedAt)
	require.NoError(t, err)
	assert.Positive(t, completedAt, "want positive Unix millisecond timestamp")
}

func TestExecutionStore_ListBySessionID(t *testing.T) {
	db := newTestDB(t)
	ws := NewWorkerStore(db)
	es := NewExecutionStore(db, t.TempDir())

	w, _ := ws.Create(model.Worker{Name: "Bot", WorkDir: "/tmp/bot"})
	exec, _ := es.Create(ExecutionCreate{WorkerID: w.ID, TriggerInput: "test message", SessionID: uuid.New().String(), Engine: "claude"})

	got, err := es.ListBySessionID(exec.SessionID)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, exec.ID, got[0].ID)
}

func TestExecutionStore_Create_EmptyWorkerID(t *testing.T) {
	db := newTestDB(t)
	es := NewExecutionStore(db, t.TempDir())

	sessionID := uuid.New().String()
	exec, err := es.Create(ExecutionCreate{TriggerInput: "test prompt", SessionID: sessionID, Engine: "claude-sonnet-4-5"})
	require.NoError(t, err)
	assert.NotEmpty(t, exec.ID)
	assert.Nil(t, exec.WorkerID, "expected nil WorkerID for bee execution")
	assert.Equal(t, model.ExecStatusPending, exec.Status)
	assert.Equal(t, "claude-sonnet-4-5", exec.Engine)

	// GetByID must scan NULL worker_id without error and preserve engine
	got, err := es.GetByID(exec.ID)
	require.NoError(t, err)
	assert.Nil(t, got.WorkerID, "expected nil WorkerID from DB")
	assert.Equal(t, sessionID, got.SessionID)
	assert.Equal(t, "claude-sonnet-4-5", got.Engine)
}

func TestExecutionStore_ReadLogSince(t *testing.T) {
	db := newTestDB(t)
	logsDir := t.TempDir()
	es := NewExecutionStore(db, logsDir)

	exec, _ := es.Create(ExecutionCreate{TriggerInput: "test prompt", SessionID: "session1"})

	// No log path yet → zero slice, no error.
	slice, err := es.ReadLogSince(exec.ID, 0)
	require.NoError(t, err)
	assert.Empty(t, slice.Content)
	assert.Zero(t, slice.Size)
	assert.False(t, slice.Truncated)

	logPath, err := es.PrepareLogPath(exec.ID, exec.StartedAt)
	require.NoError(t, err)

	// File not yet created → zero slice.
	slice, err = es.ReadLogSince(exec.ID, 0)
	require.NoError(t, err)
	assert.Empty(t, slice.Content)
	assert.Zero(t, slice.Size)
	assert.False(t, slice.Truncated)

	// Write initial content; since=0 must return everything.
	initial := []byte("line1\nline2\n")
	require.NoError(t, os.WriteFile(logPath, initial, 0o644))
	slice, err = es.ReadLogSince(exec.ID, 0)
	require.NoError(t, err)
	assert.Equal(t, string(initial), slice.Content)
	assert.EqualValues(t, len(initial), slice.Size)
	assert.False(t, slice.Truncated)

	// Append; since=len(initial) must return only the tail.
	tail := []byte("line3\n")
	f, err := os.OpenFile(logPath, os.O_WRONLY|os.O_APPEND, 0o644)
	require.NoError(t, err)
	_, err = f.Write(tail)
	require.NoError(t, err)
	f.Close()

	slice, err = es.ReadLogSince(exec.ID, int64(len(initial)))
	require.NoError(t, err)
	assert.Equal(t, string(tail), slice.Content)
	assert.EqualValues(t, len(initial)+len(tail), slice.Size)
	assert.False(t, slice.Truncated, "should not be truncated")

	// since == size → empty content.
	slice, err = es.ReadLogSince(exec.ID, int64(len(initial)+len(tail)))
	require.NoError(t, err)
	assert.Empty(t, slice.Content)
	assert.False(t, slice.Truncated)
	assert.EqualValues(t, len(initial)+len(tail), slice.Size, "size should still match")

	// since > size → truncated=true with full content.
	slice, err = es.ReadLogSince(exec.ID, 99999)
	require.NoError(t, err)
	assert.True(t, slice.Truncated, "expected truncated=true when since > size")
	assert.Equal(t, string(initial)+string(tail), slice.Content)
}

func TestExecutionStore_PrepareLogPath(t *testing.T) {
	db := newTestDB(t)
	logsDir := t.TempDir()
	es := NewExecutionStore(db, logsDir)

	exec, _ := es.Create(ExecutionCreate{TriggerInput: "test prompt", SessionID: "session1"})

	logPath, err := es.PrepareLogPath(exec.ID, exec.StartedAt)
	require.NoError(t, err)
	require.NotEmpty(t, logPath)

	// Directory must exist
	_, err = os.Stat(filepath.Dir(logPath))
	assert.NoError(t, err, "log directory should exist")

	// DB must have log_path set
	got, err := es.GetByID(exec.ID)
	require.NoError(t, err)
	assert.Equal(t, logPath, got.LogPath)
}

func TestExecutionStore_HasActiveBeeExecutions(t *testing.T) {
	db := newTestDB(t)
	es := NewExecutionStore(db, t.TempDir())
	ctx := context.Background()

	// no executions → false
	active, err := es.HasActiveBeeExecutions(ctx)
	require.NoError(t, err)
	assert.False(t, active, "expected false with no executions")

	// create a bee execution (worker_id IS NULL), status pending
	bee, _ := es.Create(ExecutionCreate{TriggerInput: "prompt", SessionID: "s1", Engine: "claude"})
	active, err = es.HasActiveBeeExecutions(ctx)
	require.NoError(t, err)
	assert.True(t, active, "expected true with pending bee execution")

	// transition to running → still true
	_ = es.UpdateStatus(bee.ID, model.ExecStatusRunning)
	active, err = es.HasActiveBeeExecutions(ctx)
	require.NoError(t, err)
	assert.True(t, active, "expected true with running bee execution")

	// complete the bee execution → false again
	_ = es.UpdateStatus(bee.ID, model.ExecStatusCompleted)
	active, err = es.HasActiveBeeExecutions(ctx)
	require.NoError(t, err)
	assert.False(t, active, "expected false after completing bee execution")

	// worker execution (worker_id NOT NULL) must not count
	db.Exec(seedWorkerSQL("w1"))
	_, _ = es.Create(ExecutionCreate{WorkerID: "w1", TriggerInput: "task", SessionID: "s2", Engine: "claude"})
	active, err = es.HasActiveBeeExecutions(ctx)
	require.NoError(t, err)
	assert.False(t, active, "worker execution must not affect HasActiveBeeExecutions")
}

func TestExecutionStore_MarkAbandoned_OnlyUpdatesActive(t *testing.T) {
	db := newTestDB(t, seedWorkerSQL("w1"))
	es := NewExecutionStore(db, t.TempDir())
	ctx := context.Background()

	pending, _ := es.Create(ExecutionCreate{WorkerID: "w1", TriggerInput: "p", SessionID: uuid.New().String(), Engine: "claude"})
	running, _ := es.Create(ExecutionCreate{WorkerID: "w1", TriggerInput: "r", SessionID: uuid.New().String(), Engine: "claude"})
	_ = es.UpdateStatus(running.ID, model.ExecStatusRunning)
	completed, _ := es.Create(ExecutionCreate{WorkerID: "w1", TriggerInput: "c", SessionID: uuid.New().String(), Engine: "claude"})
	_ = es.UpdateResult(completed.ID, "done", model.ExecStatusCompleted)
	failed, _ := es.Create(ExecutionCreate{WorkerID: "w1", TriggerInput: "f", SessionID: uuid.New().String(), Engine: "claude"})
	_ = es.UpdateResult(failed.ID, "boom", model.ExecStatusFailed)

	ok, err := es.MarkAbandoned(ctx, pending.ID, "cancelled by user")
	require.NoError(t, err)
	require.True(t, ok)
	got, _ := es.GetByID(pending.ID)
	assert.Equal(t, model.ExecStatusFailed, got.Status, "pending → expected failed")
	assert.Equal(t, "cancelled by user", got.Result)
	if assert.NotNil(t, got.CompletedAt) {
		assert.Positive(t, *got.CompletedAt, "pending completed_at should be set")
	}

	ok, _ = es.MarkAbandoned(ctx, running.ID, "process exited")
	assert.True(t, ok, "running should be updated")

	// Terminal states must be left untouched and the call must report no update.
	ok, _ = es.MarkAbandoned(ctx, completed.ID, "should not change")
	assert.False(t, ok, "completed row must not be updated")
	got, _ = es.GetByID(completed.ID)
	assert.Equal(t, "done", got.Result, "completed result clobbered")

	ok, _ = es.MarkAbandoned(ctx, failed.ID, "should not change")
	assert.False(t, ok, "failed row must not be updated")
	got, _ = es.GetByID(failed.ID)
	assert.Equal(t, "boom", got.Result, "failed result clobbered")
}

func TestExecutionStore_ResetRunningExecutions(t *testing.T) {
	db := newTestDB(t, seedWorkerSQL("w1"))
	es := NewExecutionStore(db, t.TempDir())
	ctx := context.Background()

	p, _ := es.Create(ExecutionCreate{WorkerID: "w1", TriggerInput: "p", SessionID: uuid.New().String(), Engine: "claude"})
	r, _ := es.Create(ExecutionCreate{WorkerID: "w1", TriggerInput: "r", SessionID: uuid.New().String(), Engine: "claude"})
	_ = es.UpdateStatus(r.ID, model.ExecStatusRunning)
	c, _ := es.Create(ExecutionCreate{WorkerID: "w1", TriggerInput: "c", SessionID: uuid.New().String(), Engine: "claude"})
	_ = es.UpdateResult(c.ID, "done", model.ExecStatusCompleted)

	n, err := es.ResetRunningExecutions(ctx)
	require.NoError(t, err)
	assert.EqualValues(t, 2, n, "expected 2 rows updated (pending+running)")

	pg, _ := es.GetByID(p.ID)
	assert.Equal(t, model.ExecStatusFailed, pg.Status, "pending → failed")
	assert.Equal(t, "abandoned: server restarted", pg.Result)
	assert.NotNil(t, pg.CompletedAt, "pending completed_at must be set")

	rg, _ := es.GetByID(r.ID)
	assert.Equal(t, model.ExecStatusFailed, rg.Status, "running → failed")

	cg, _ := es.GetByID(c.ID)
	assert.Equal(t, model.ExecStatusCompleted, cg.Status, "completed must be untouched")
	assert.Equal(t, "done", cg.Result, "completed result clobbered")
}

func TestExecutionStore_HasActiveExecutionsByWorkerID(t *testing.T) {
	db := newTestDB(t, seedWorkerSQL("w1"), seedWorkerSQL("w2"))
	es := NewExecutionStore(db, t.TempDir())
	ctx := context.Background()

	// no executions → false for both workers
	active, err := es.HasActiveExecutionsByWorkerID(ctx, "w1")
	require.NoError(t, err)
	assert.False(t, active, "expected false with no executions")

	// create pending execution for w1
	exec1, _ := es.Create(ExecutionCreate{WorkerID: "w1", TriggerInput: "task", SessionID: "s1", Engine: "claude"})
	active, err = es.HasActiveExecutionsByWorkerID(ctx, "w1")
	require.NoError(t, err)
	assert.True(t, active, "expected true for w1 with pending execution")

	// w2 must not be affected by w1's execution
	active, err = es.HasActiveExecutionsByWorkerID(ctx, "w2")
	require.NoError(t, err)
	assert.False(t, active, "w2 should not be affected by w1's execution")

	// transition to running → still true
	_ = es.UpdateStatus(exec1.ID, model.ExecStatusRunning)
	active, err = es.HasActiveExecutionsByWorkerID(ctx, "w1")
	require.NoError(t, err)
	assert.True(t, active, "expected true for w1 with running execution")

	// complete w1's execution → false
	_ = es.UpdateStatus(exec1.ID, model.ExecStatusCompleted)
	active, err = es.HasActiveExecutionsByWorkerID(ctx, "w1")
	require.NoError(t, err)
	assert.False(t, active, "expected false after completing w1 execution")
}
