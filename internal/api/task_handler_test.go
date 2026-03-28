package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/theopenbee/openbee/internal/model"
	"github.com/theopenbee/openbee/internal/store"
)

func newTestServerWithTasks(t *testing.T) (*Server, *store.TaskStore, func()) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	db, err := store.InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	db.Exec(`INSERT INTO bee_workers (id,name,work_dir,status,created_at,updated_at) VALUES ('w1','Worker1','/','idle',1,1)`)
	db.Exec(`INSERT INTO bee_platform_messages (id,session_key,platform,content,raw,platform_msg_id,received_at,created_at,updated_at) VALUES ('m1','s1','feishu','hi','','',1,1,1)`)

	taskStore := store.NewTaskStore(db)
	workerStore := store.NewWorkerStore(db)

	router := gin.New()
	s := &Server{
		router: router,
		ServerParams: ServerParams{
			TaskStore:   taskStore,
			WorkerStore: workerStore,
		},
	}
	s.registerTaskRoutes(router.Group("/api"))
	return s, taskStore, func() { db.Close() }
}

func TestListTasks_FiltersByTypeAndStatus(t *testing.T) {
	s, ts, cleanup := newTestServerWithTasks(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now().UnixMilli()

	// Create scheduled + countdown tasks
	ts.Create(ctx, model.Task{
		MessageID: "m1", WorkerID: "w1", Instruction: "cron job",
		Type: model.TaskTypeScheduled, Status: model.TaskStatusPending,
		CronExpr: "0 * * * *", CreatedAt: now, UpdatedAt: now,
	})
	ts.Create(ctx, model.Task{
		MessageID: "m1", WorkerID: "w1", Instruction: "countdown job",
		Type: model.TaskTypeCountdown, Status: model.TaskStatusPending,
		CreatedAt: now, UpdatedAt: now,
	})
	// Immediate task — should NOT appear in default list
	ts.Create(ctx, model.Task{
		MessageID: "m1", WorkerID: "w1", Instruction: "immediate",
		Type: model.TaskTypeImmediate, Status: model.TaskStatusPending,
		CreatedAt: now, UpdatedAt: now,
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/tasks", nil)
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Items []map[string]any `json:"items"`
		Total int              `json:"total"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Total != 2 {
		t.Errorf("want total=2, got %d", resp.Total)
	}
	for _, item := range resp.Items {
		if item["type"] == "immediate" {
			t.Error("immediate task should not appear in default list")
		}
	}
}

func TestCancelTask_PendingSucceeds(t *testing.T) {
	s, ts, cleanup := newTestServerWithTasks(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now().UnixMilli()
	id, _ := ts.Create(ctx, model.Task{
		MessageID: "m1", WorkerID: "w1", Instruction: "x",
		Type: model.TaskTypeScheduled, Status: model.TaskStatusPending,
		CreatedAt: now, UpdatedAt: now,
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/tasks/"+id, nil)
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
	}

	task, _ := ts.GetByID(ctx, id)
	if task.Status != model.TaskStatusCancelled {
		t.Errorf("want cancelled, got %s", task.Status)
	}
}

func TestCancelTask_NonPendingReturns409(t *testing.T) {
	s, ts, cleanup := newTestServerWithTasks(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now().UnixMilli()
	id, _ := ts.Create(ctx, model.Task{
		MessageID: "m1", WorkerID: "w1", Instruction: "x",
		Type: model.TaskTypeScheduled, Status: model.TaskStatusPending,
		CreatedAt: now, UpdatedAt: now,
	})
	// Mark it running
	ts.UpdateStatus(ctx, id, model.TaskStatusRunning)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/tasks/"+id, nil)
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", w.Code)
	}
}

func TestCancelWorkerTasks_CancelsAllPending(t *testing.T) {
	s, ts, cleanup := newTestServerWithTasks(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now().UnixMilli()
	for i := 0; i < 3; i++ {
		ts.Create(ctx, model.Task{
			MessageID: "m1", WorkerID: "w1", Instruction: "x",
			Type: model.TaskTypeScheduled, Status: model.TaskStatusPending,
			CreatedAt: now, UpdatedAt: now,
		})
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/workers/w1/tasks/cancel-all", nil)
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	tasks, _ := ts.List(ctx, store.TaskFilter{WorkerID: "w1", Status: "cancelled"})
	if len(tasks) != 3 {
		t.Errorf("want 3 cancelled tasks, got %d", len(tasks))
	}
}
