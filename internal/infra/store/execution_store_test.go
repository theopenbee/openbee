package store

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/theopenbee/openbee/internal/infra/model"
)

func newTestExecutionStore(t *testing.T) *ExecutionStore {
	t.Helper()
	db, err := InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	// Insert a worker so FK constraints are satisfied
	if _, err := db.Exec(`INSERT INTO bee_workers (id,name,work_dir,status,created_at,updated_at) VALUES ('w1','bot','/','idle',0,0)`); err != nil {
		t.Fatalf("seed worker: %v", err)
	}
	return NewExecutionStore(db, t.TempDir())
}

func TestExecutionStore_CreateWritesTaskID(t *testing.T) {
	s := newTestExecutionStore(t)
	exec, err := s.Create("w1", "task-1", "trigger", "sess-1", "claude")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, err := s.GetByID(exec.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.TaskID != "task-1" {
		t.Errorf("task_id: want task-1 got %q", got.TaskID)
	}
}

func TestExecutionStore_GetRunningByTaskID(t *testing.T) {
	s := newTestExecutionStore(t)
	running, _ := s.Create("w1", "task-1", "in", "sess-1", "claude")
	if err := s.UpdateStatus(running.ID, model.ExecStatusRunning); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}
	got, err := s.GetRunningByTaskID(context.Background(), "task-1")
	if err != nil {
		t.Fatalf("GetRunningByTaskID: %v", err)
	}
	if got == nil || got.ID != running.ID {
		t.Fatalf("want running exec %s, got %+v", running.ID, got)
	}
	none, err := s.GetRunningByTaskID(context.Background(), "task-x")
	if err != nil {
		t.Fatalf("GetRunningByTaskID(none): %v", err)
	}
	if none != nil {
		t.Errorf("want nil for unknown task, got %+v", none)
	}
	// A pending (never-running) execution must not be returned.
	pending, _ := s.Create("w1", "task-2", "in", "sess-2", "claude")
	got2, err := s.GetRunningByTaskID(context.Background(), "task-2")
	if err != nil {
		t.Fatalf("GetRunningByTaskID(pending): %v", err)
	}
	if got2 != nil {
		t.Errorf("want nil for task with only a pending execution, got %+v", got2)
	}
	_ = pending
}

func TestExecutionStore_ListByTaskIDs(t *testing.T) {
	s := newTestExecutionStore(t)
	e1, _ := s.Create("w1", "task-1", "first", "sess-1", "claude")
	e2, _ := s.Create("w1", "task-1", "second", "sess-2", "claude")
	_, _ = s.Create("w1", "task-2", "other", "sess-3", "claude")
	m, err := s.ListByTaskIDs(context.Background(), []string{"task-1", "task-2"}, 0)
	if err != nil {
		t.Fatalf("ListByTaskIDs: %v", err)
	}
	if len(m["task-1"]) != 2 {
		t.Fatalf("task-1 want 2 execs, got %d", len(m["task-1"]))
	}
	// Newest-first: e2 was inserted after e1; the rowid DESC tiebreak makes this
	// deterministic even when both rows share the same started_at millisecond.
	if m["task-1"][0].ID != e2.ID || m["task-1"][1].ID != e1.ID {
		t.Errorf("expected newest-first ordering; got %s,%s", m["task-1"][0].ID, m["task-1"][1].ID)
	}
	if len(m["task-2"]) != 1 {
		t.Errorf("task-2 want 1 exec, got %d", len(m["task-2"]))
	}
	// Task ids with no executions are absent from the returned map.
	withMissing, err := s.ListByTaskIDs(context.Background(), []string{"task-1", "task-none"}, 0)
	if err != nil {
		t.Fatalf("ListByTaskIDs(missing): %v", err)
	}
	if got, ok := withMissing["task-none"]; ok {
		t.Errorf("want task-none absent, got %#v", got)
	}
}

func TestExecutionStore_ListByTaskIDs_LimitsExecutionsPerTask(t *testing.T) {
	s := newTestExecutionStore(t)
	if _, err := s.Create("w1", "task-1", "first", "sess-1", "claude"); err != nil {
		t.Fatalf("Create: %v", err)
	}
	e2, _ := s.Create("w1", "task-1", "second", "sess-2", "claude")
	e3, _ := s.Create("w1", "task-1", "third", "sess-3", "claude")
	if _, err := s.Create("w1", "task-2", "other", "sess-4", "claude"); err != nil {
		t.Fatalf("Create task-2 execution: %v", err)
	}

	got, err := s.ListByTaskIDs(context.Background(), []string{"task-1", "task-2"}, 2)
	if err != nil {
		t.Fatalf("ListByTaskIDs: %v", err)
	}
	if len(got["task-1"]) != 2 {
		t.Fatalf("task-1 expected 2 executions, got %d", len(got["task-1"]))
	}
	if got["task-1"][0].ID != e3.ID || got["task-1"][1].ID != e2.ID {
		t.Fatalf("expected newest two executions %s,%s; got %+v", e3.ID, e2.ID, got["task-1"])
	}
	if len(got["task-2"]) != 1 {
		t.Fatalf("task-2 expected 1 execution, got %d", len(got["task-2"]))
	}
}

func TestExecutionStore_ListByTaskIDs_ZeroLimitReturnsAll(t *testing.T) {
	s := newTestExecutionStore(t)
	e1, _ := s.Create("w1", "task-1", "first", "sess-1", "claude")
	e2, _ := s.Create("w1", "task-1", "second", "sess-2", "claude")
	e3, _ := s.Create("w1", "task-1", "third", "sess-3", "claude")

	got, err := s.ListByTaskIDs(context.Background(), []string{"task-1"}, 0)
	if err != nil {
		t.Fatalf("ListByTaskIDs: %v", err)
	}
	if len(got["task-1"]) != 3 {
		t.Fatalf("expected all 3 executions, got %d", len(got["task-1"]))
	}
	if got["task-1"][0].ID != e3.ID || got["task-1"][1].ID != e2.ID || got["task-1"][2].ID != e1.ID {
		t.Fatalf("unexpected newest-first order: %+v", got["task-1"])
	}
}

func TestExecutionStore_CreateAndGet(t *testing.T) {
	db, err := InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ws := NewWorkerStore(db)
	es := NewExecutionStore(db, t.TempDir())

	w, _ := ws.Create(model.Worker{Name: "Bot", WorkDir: "/tmp/bot"})

	exec, err := es.Create(w.ID, "", "test message", uuid.New().String(), "claude")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if exec.Status != model.ExecStatusPending {
		t.Errorf("expected pending, got %s", exec.Status)
	}
	if exec.SessionID == "" {
		t.Error("expected non-empty session_id")
	}

	got, err := es.GetByID(exec.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.WorkerID == nil || *got.WorkerID != w.ID {
		gotStr := "<nil>"
		if got.WorkerID != nil {
			gotStr = *got.WorkerID
		}
		t.Errorf("expected worker_id %s, got %s", w.ID, gotStr)
	}
	if got.Engine != "claude" {
		t.Errorf("expected engine claude, got %q", got.Engine)
	}
}

func TestExecutionStore_UpdateStatus(t *testing.T) {
	db, err := InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ws := NewWorkerStore(db)
	es := NewExecutionStore(db, t.TempDir())

	w, _ := ws.Create(model.Worker{Name: "Bot", WorkDir: "/tmp/bot"})
	exec, _ := es.Create(w.ID, "", "test message", uuid.New().String(), "claude")

	err = es.UpdateStatus(exec.ID, model.ExecStatusRunning)
	if err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}
	got, _ := es.GetByID(exec.ID)
	if got.Status != model.ExecStatusRunning {
		t.Errorf("expected running, got %s", got.Status)
	}
}

func TestExecutionStore_Create_StartedAtMillisecondPrecision(t *testing.T) {
	db, err := InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ws := NewWorkerStore(db)
	es := NewExecutionStore(db, t.TempDir())

	w, _ := ws.Create(model.Worker{Name: "Bot", WorkDir: "/tmp/bot"})
	exec, err := es.Create(w.ID, "", "test", uuid.New().String(), "claude")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	var startedAt int64
	err = db.QueryRow(`SELECT started_at FROM bee_executions WHERE id = ?`, exec.ID).Scan(&startedAt)
	if err != nil {
		t.Fatalf("scan started_at: %v", err)
	}
	if startedAt <= 0 {
		t.Errorf("started_at %d: want positive Unix millisecond timestamp", startedAt)
	}

	if exec.StartedAt == nil {
		t.Error("exec.StartedAt must not be nil")
	}
}

func TestExecutionStore_UpdateResult_CompletedAtMillisecondPrecision(t *testing.T) {
	db, err := InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ws := NewWorkerStore(db)
	es := NewExecutionStore(db, t.TempDir())

	w, _ := ws.Create(model.Worker{Name: "Bot", WorkDir: "/tmp/bot"})
	exec, _ := es.Create(w.ID, "", "test", uuid.New().String(), "claude")

	if err := es.UpdateResult(exec.ID, "output", model.ExecStatusCompleted); err != nil {
		t.Fatalf("UpdateResult: %v", err)
	}

	var completedAt int64
	err = db.QueryRow(`SELECT completed_at FROM bee_executions WHERE id = ?`, exec.ID).Scan(&completedAt)
	if err != nil {
		t.Fatalf("scan completed_at: %v", err)
	}
	if completedAt <= 0 {
		t.Errorf("completed_at %d: want positive Unix millisecond timestamp", completedAt)
	}
}

func TestExecutionStore_ListBySessionID(t *testing.T) {
	db, err := InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ws := NewWorkerStore(db)
	es := NewExecutionStore(db, t.TempDir())

	w, _ := ws.Create(model.Worker{Name: "Bot", WorkDir: "/tmp/bot"})
	exec, _ := es.Create(w.ID, "", "test message", uuid.New().String(), "claude")

	got, err := es.ListBySessionID(exec.SessionID)
	if err != nil {
		t.Fatalf("ListBySessionID: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 execution, got %d", len(got))
	}
	if got[0].ID != exec.ID {
		t.Errorf("expected ID %s, got %s", exec.ID, got[0].ID)
	}
}

func TestExecutionStore_ListBeeExecutions(t *testing.T) {
	db, err := InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	es := NewExecutionStore(db, t.TempDir())

	// Create a bee execution (worker_id = NULL)
	bee1, err := es.Create("", "", "user said hello", "session1", "")
	if err != nil {
		t.Fatal(err)
	}
	_ = bee1

	// Create a worker execution (should not appear)
	db.Exec(`INSERT INTO bee_workers (id, name, work_dir, status, created_at, updated_at) VALUES ('w1','test','/tmp','idle',0,0)`)
	_, err = es.Create("w1", "", "worker task", "session2", "claude")
	if err != nil {
		t.Fatal(err)
	}

	results, err := es.ListBeeExecutions(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 bee execution, got %d", len(results))
	}
}

func TestExecutionStore_CreateBeeExecution(t *testing.T) {
	db, err := InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	es := NewExecutionStore(db, t.TempDir())

	sessionID := uuid.New().String()
	exec, err := es.Create("", "", "test prompt", sessionID, "claude-sonnet-4-5")
	if err != nil {
		t.Fatalf("Create bee execution: %v", err)
	}
	if exec.ID == "" {
		t.Error("expected non-empty ID")
	}
	if exec.WorkerID != nil {
		t.Errorf("expected nil WorkerID for bee execution, got %v", exec.WorkerID)
	}
	if exec.Status != model.ExecStatusPending {
		t.Errorf("expected pending, got %s", exec.Status)
	}
	if exec.Engine != "claude-sonnet-4-5" {
		t.Errorf("expected engine claude-sonnet-4-5, got %s", exec.Engine)
	}

	// GetByID must scan NULL worker_id without error and preserve engine
	got, err := es.GetByID(exec.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.WorkerID != nil {
		t.Errorf("expected nil WorkerID from DB, got %v", got.WorkerID)
	}
	if got.SessionID != sessionID {
		t.Errorf("expected session_id %s, got %s", sessionID, got.SessionID)
	}
	if got.Engine != "claude-sonnet-4-5" {
		t.Errorf("expected engine claude-sonnet-4-5 from DB, got %s", got.Engine)
	}
}

func TestExecutionStore_ReadLogSince(t *testing.T) {
	db, err := InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	logsDir := t.TempDir()
	es := NewExecutionStore(db, logsDir)

	exec, _ := es.Create("", "", "test prompt", "session1", "")

	// No log path yet → zero slice, no error.
	slice, err := es.ReadLogSince(exec.ID, 0)
	if err != nil {
		t.Fatalf("ReadLogSince (no log_path): %v", err)
	}
	if slice.Content != "" || slice.Size != 0 || slice.Truncated {
		t.Errorf("expected zero slice, got %+v", slice)
	}

	logPath, err := es.PrepareLogPath(exec.ID, exec.StartedAt)
	if err != nil {
		t.Fatal(err)
	}

	// File not yet created → zero slice.
	slice, err = es.ReadLogSince(exec.ID, 0)
	if err != nil {
		t.Fatalf("ReadLogSince (file missing): %v", err)
	}
	if slice.Content != "" || slice.Size != 0 || slice.Truncated {
		t.Errorf("expected zero slice, got %+v", slice)
	}

	// Write initial content; since=0 must return everything.
	initial := []byte("line1\nline2\n")
	if err := os.WriteFile(logPath, initial, 0o644); err != nil {
		t.Fatal(err)
	}
	slice, err = es.ReadLogSince(exec.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if slice.Content != string(initial) || slice.Size != int64(len(initial)) || slice.Truncated {
		t.Errorf("full read mismatch: %+v", slice)
	}

	// Append; since=len(initial) must return only the tail.
	tail := []byte("line3\n")
	f, err := os.OpenFile(logPath, os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write(tail); err != nil {
		t.Fatal(err)
	}
	f.Close()

	slice, err = es.ReadLogSince(exec.ID, int64(len(initial)))
	if err != nil {
		t.Fatal(err)
	}
	if slice.Content != string(tail) {
		t.Errorf("tail content mismatch: got %q want %q", slice.Content, string(tail))
	}
	if slice.Size != int64(len(initial)+len(tail)) {
		t.Errorf("size mismatch: got %d want %d", slice.Size, len(initial)+len(tail))
	}
	if slice.Truncated {
		t.Error("should not be truncated")
	}

	// since == size → empty content.
	slice, err = es.ReadLogSince(exec.ID, int64(len(initial)+len(tail)))
	if err != nil {
		t.Fatal(err)
	}
	if slice.Content != "" || slice.Truncated {
		t.Errorf("caught-up read mismatch: %+v", slice)
	}
	if slice.Size != int64(len(initial)+len(tail)) {
		t.Errorf("size should still match: got %d", slice.Size)
	}

	// since > size → truncated=true with full content.
	slice, err = es.ReadLogSince(exec.ID, 99999)
	if err != nil {
		t.Fatal(err)
	}
	if !slice.Truncated {
		t.Error("expected truncated=true when since > size")
	}
	if slice.Content != string(initial)+string(tail) {
		t.Errorf("truncated content mismatch: got %q", slice.Content)
	}
}

func TestExecutionStore_PrepareLogPath(t *testing.T) {
	db, err := InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	logsDir := t.TempDir()
	es := NewExecutionStore(db, logsDir)

	exec, _ := es.Create("", "", "test prompt", "session1", "")

	logPath, err := es.PrepareLogPath(exec.ID, exec.StartedAt)
	if err != nil {
		t.Fatalf("PrepareLogPath: %v", err)
	}
	if logPath == "" {
		t.Fatal("expected non-empty logPath")
	}

	// Directory must exist
	if _, err := os.Stat(filepath.Dir(logPath)); err != nil {
		t.Errorf("log directory should exist: %v", err)
	}

	// DB must have log_path set
	got, err := es.GetByID(exec.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.LogPath != logPath {
		t.Errorf("DB log_path mismatch: want %q got %q", logPath, got.LogPath)
	}
}

func TestExecutionStore_HasActiveBeeExecutions(t *testing.T) {
	db, err := InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	es := NewExecutionStore(db, t.TempDir())
	ctx := context.Background()

	// no executions → false
	active, err := es.HasActiveBeeExecutions(ctx)
	if err != nil {
		t.Fatalf("HasActiveBeeExecutions: %v", err)
	}
	if active {
		t.Error("expected false with no executions")
	}

	// create a bee execution (worker_id IS NULL), status pending
	bee, _ := es.Create("", "", "prompt", "s1", "claude")
	active, err = es.HasActiveBeeExecutions(ctx)
	if err != nil {
		t.Fatalf("HasActiveBeeExecutions: %v", err)
	}
	if !active {
		t.Error("expected true with pending bee execution")
	}

	// transition to running → still true
	_ = es.UpdateStatus(bee.ID, model.ExecStatusRunning)
	active, err = es.HasActiveBeeExecutions(ctx)
	if err != nil {
		t.Fatalf("HasActiveBeeExecutions: %v", err)
	}
	if !active {
		t.Error("expected true with running bee execution")
	}

	// complete the bee execution → false again
	_ = es.UpdateStatus(bee.ID, model.ExecStatusCompleted)
	active, err = es.HasActiveBeeExecutions(ctx)
	if err != nil {
		t.Fatalf("HasActiveBeeExecutions: %v", err)
	}
	if active {
		t.Error("expected false after completing bee execution")
	}

	// worker execution (worker_id NOT NULL) must not count
	db.Exec(`INSERT INTO bee_workers (id,name,work_dir,status,created_at,updated_at) VALUES ('w1','bot','/','idle',0,0)`)
	_, _ = es.Create("w1", "", "task", "s2", "claude")
	active, err = es.HasActiveBeeExecutions(ctx)
	if err != nil {
		t.Fatalf("HasActiveBeeExecutions: %v", err)
	}
	if active {
		t.Error("worker execution must not affect HasActiveBeeExecutions")
	}
}

func TestExecutionStore_MarkAbandoned_OnlyUpdatesActive(t *testing.T) {
	db, err := InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	es := NewExecutionStore(db, t.TempDir())
	ctx := context.Background()
	db.Exec(`INSERT INTO bee_workers (id,name,work_dir,status,created_at,updated_at) VALUES ('w1','bot','/','idle',0,0)`)

	pending, _ := es.Create("w1", "", "p", uuid.New().String(), "claude")
	running, _ := es.Create("w1", "", "r", uuid.New().String(), "claude")
	_ = es.UpdateStatus(running.ID, model.ExecStatusRunning)
	completed, _ := es.Create("w1", "", "c", uuid.New().String(), "claude")
	_ = es.UpdateResult(completed.ID, "done", model.ExecStatusCompleted)
	failed, _ := es.Create("w1", "", "f", uuid.New().String(), "claude")
	_ = es.UpdateResult(failed.ID, "boom", model.ExecStatusFailed)

	ok, err := es.MarkAbandoned(ctx, pending.ID, "cancelled by user")
	if err != nil || !ok {
		t.Fatalf("pending: ok=%v err=%v", ok, err)
	}
	got, _ := es.GetByID(pending.ID)
	if got.Status != model.ExecStatusFailed {
		t.Errorf("pending → expected failed, got %s", got.Status)
	}
	if got.Result != "cancelled by user" {
		t.Errorf("pending result: got %q", got.Result)
	}
	if got.CompletedAt == nil || *got.CompletedAt <= 0 {
		t.Error("pending completed_at should be set")
	}

	ok, _ = es.MarkAbandoned(ctx, running.ID, "process exited")
	if !ok {
		t.Error("running should be updated")
	}

	// Terminal states must be left untouched and the call must report no update.
	ok, _ = es.MarkAbandoned(ctx, completed.ID, "should not change")
	if ok {
		t.Error("completed row must not be updated")
	}
	got, _ = es.GetByID(completed.ID)
	if got.Result != "done" {
		t.Errorf("completed result clobbered: got %q", got.Result)
	}

	ok, _ = es.MarkAbandoned(ctx, failed.ID, "should not change")
	if ok {
		t.Error("failed row must not be updated")
	}
	got, _ = es.GetByID(failed.ID)
	if got.Result != "boom" {
		t.Errorf("failed result clobbered: got %q", got.Result)
	}
}

func TestExecutionStore_ResetRunningExecutions(t *testing.T) {
	db, err := InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	es := NewExecutionStore(db, t.TempDir())
	ctx := context.Background()
	db.Exec(`INSERT INTO bee_workers (id,name,work_dir,status,created_at,updated_at) VALUES ('w1','bot','/','idle',0,0)`)

	p, _ := es.Create("w1", "", "p", uuid.New().String(), "claude")
	r, _ := es.Create("w1", "", "r", uuid.New().String(), "claude")
	_ = es.UpdateStatus(r.ID, model.ExecStatusRunning)
	c, _ := es.Create("w1", "", "c", uuid.New().String(), "claude")
	_ = es.UpdateResult(c.ID, "done", model.ExecStatusCompleted)

	n, err := es.ResetRunningExecutions(ctx)
	if err != nil {
		t.Fatalf("ResetRunningExecutions: %v", err)
	}
	if n != 2 {
		t.Errorf("expected 2 rows updated (pending+running), got %d", n)
	}

	pg, _ := es.GetByID(p.ID)
	if pg.Status != model.ExecStatusFailed {
		t.Errorf("pending → failed: got %s", pg.Status)
	}
	if pg.Result != "abandoned: server restarted" {
		t.Errorf("pending result: got %q", pg.Result)
	}
	if pg.CompletedAt == nil {
		t.Error("pending completed_at must be set")
	}

	rg, _ := es.GetByID(r.ID)
	if rg.Status != model.ExecStatusFailed {
		t.Errorf("running → failed: got %s", rg.Status)
	}

	cg, _ := es.GetByID(c.ID)
	if cg.Status != model.ExecStatusCompleted {
		t.Errorf("completed must be untouched: got %s", cg.Status)
	}
	if cg.Result != "done" {
		t.Errorf("completed result clobbered: got %q", cg.Result)
	}
}

func TestExecutionStore_HasActiveExecutionsByWorkerID(t *testing.T) {
	db, err := InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	es := NewExecutionStore(db, t.TempDir())
	ctx := context.Background()

	db.Exec(`INSERT INTO bee_workers (id,name,work_dir,status,created_at,updated_at) VALUES ('w1','alice','/','idle',0,0)`)
	db.Exec(`INSERT INTO bee_workers (id,name,work_dir,status,created_at,updated_at) VALUES ('w2','bob','/','idle',0,0)`)

	// no executions → false for both workers
	active, err := es.HasActiveExecutionsByWorkerID(ctx, "w1")
	if err != nil {
		t.Fatalf("HasActiveExecutionsByWorkerID: %v", err)
	}
	if active {
		t.Error("expected false with no executions")
	}

	// create pending execution for w1
	exec1, _ := es.Create("w1", "", "task", "s1", "claude")
	active, err = es.HasActiveExecutionsByWorkerID(ctx, "w1")
	if err != nil {
		t.Fatalf("HasActiveExecutionsByWorkerID: %v", err)
	}
	if !active {
		t.Error("expected true for w1 with pending execution")
	}

	// w2 must not be affected by w1's execution
	active, err = es.HasActiveExecutionsByWorkerID(ctx, "w2")
	if err != nil {
		t.Fatalf("HasActiveExecutionsByWorkerID w2: %v", err)
	}
	if active {
		t.Error("w2 should not be affected by w1's execution")
	}

	// transition to running → still true
	_ = es.UpdateStatus(exec1.ID, model.ExecStatusRunning)
	active, err = es.HasActiveExecutionsByWorkerID(ctx, "w1")
	if err != nil {
		t.Fatalf("HasActiveExecutionsByWorkerID (running): %v", err)
	}
	if !active {
		t.Error("expected true for w1 with running execution")
	}

	// complete w1's execution → false
	_ = es.UpdateStatus(exec1.ID, model.ExecStatusCompleted)
	active, err = es.HasActiveExecutionsByWorkerID(ctx, "w1")
	if err != nil {
		t.Fatalf("HasActiveExecutionsByWorkerID: %v", err)
	}
	if active {
		t.Error("expected false after completing w1 execution")
	}
}
