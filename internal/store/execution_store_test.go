package store

import (
	"testing"

	"github.com/google/uuid"
	"github.com/theopenbee/openbee/internal/model"
)

func TestExecutionStore_CreateAndGet(t *testing.T) {
	db, err := InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ws := NewWorkerStore(db)
	es := NewExecutionStore(db)

	w, _ := ws.Create(model.Worker{Name: "Bot", WorkDir: "/tmp/bot"})

	exec, err := es.Create(w.ID, "test message", uuid.New().String())
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
}

func TestExecutionStore_UpdateStatus(t *testing.T) {
	db, err := InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ws := NewWorkerStore(db)
	es := NewExecutionStore(db)

	w, _ := ws.Create(model.Worker{Name: "Bot", WorkDir: "/tmp/bot"})
	exec, _ := es.Create(w.ID, "test message", uuid.New().String())

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
	es := NewExecutionStore(db)

	w, _ := ws.Create(model.Worker{Name: "Bot", WorkDir: "/tmp/bot"})
	exec, err := es.Create(w.ID, "test", uuid.New().String())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	var startedAt int64
	err = db.QueryRow(`SELECT started_at FROM executions WHERE id = ?`, exec.ID).Scan(&startedAt)
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
	es := NewExecutionStore(db)

	w, _ := ws.Create(model.Worker{Name: "Bot", WorkDir: "/tmp/bot"})
	exec, _ := es.Create(w.ID, "test", uuid.New().String())

	if err := es.UpdateResult(exec.ID, "output", model.ExecStatusCompleted); err != nil {
		t.Fatalf("UpdateResult: %v", err)
	}

	var completedAt int64
	err = db.QueryRow(`SELECT completed_at FROM executions WHERE id = ?`, exec.ID).Scan(&completedAt)
	if err != nil {
		t.Fatalf("scan completed_at: %v", err)
	}
	if completedAt <= 0 {
		t.Errorf("completed_at %d: want positive Unix millisecond timestamp", completedAt)
	}
}

func TestExecutionStore_GetBySessionID(t *testing.T) {
	db, err := InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ws := NewWorkerStore(db)
	es := NewExecutionStore(db)

	w, _ := ws.Create(model.Worker{Name: "Bot", WorkDir: "/tmp/bot"})
	exec, _ := es.Create(w.ID, "test message", uuid.New().String())

	got, err := es.GetBySessionID(exec.SessionID)
	if err != nil {
		t.Fatalf("GetBySessionID: %v", err)
	}
	if got.ID != exec.ID {
		t.Errorf("expected ID %s, got %s", exec.ID, got.ID)
	}
}

func TestExecutionStore_ListBeeExecutions(t *testing.T) {
	db, err := InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	es := NewExecutionStore(db)

	// Create a bee execution (worker_id = NULL)
	bee1, err := es.CreateBeeExecution("session1", "user said hello")
	if err != nil {
		t.Fatal(err)
	}
	_ = bee1

	// Create a worker execution (should not appear)
	db.Exec(`INSERT INTO workers (id, name, work_dir, status, created_at, updated_at) VALUES ('w1','test','/tmp','idle',0,0)`)
	_, err = es.Create("w1", "worker task", "session2")
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

func TestExecutionStore_GetLogsByID(t *testing.T) {
	db, err := InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	es := NewExecutionStore(db)
	exec, _ := es.CreateBeeExecution("session1", "test")

	// Update logs with multiline content
	logs := "line1\nline2\nline3\nline4\nline5"
	es.UpdateLogs(exec.ID, logs)

	// Get logs
	result, err := es.GetLogsByID(exec.ID)
	if err != nil {
		t.Fatal(err)
	}
	if result.Logs != logs {
		t.Errorf("expected full logs, got %q", result.Logs)
	}

	// Non-existent returns nil
	result2, err := es.GetLogsByID("nonexistent")
	if err != nil {
		t.Fatal(err)
	}
	if result2 != nil {
		t.Error("expected nil for non-existent execution")
	}
}

func TestExecutionStore_CreateBeeExecution(t *testing.T) {
	db, err := InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	es := NewExecutionStore(db)

	sessionID := uuid.New().String()
	exec, err := es.CreateBeeExecution(sessionID, "test prompt")
	if err != nil {
		t.Fatalf("CreateBeeExecution: %v", err)
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

	// GetByID must scan NULL worker_id without error
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
}
