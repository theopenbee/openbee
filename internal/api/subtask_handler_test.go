package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/infra/store"
)

// fakeSubtaskDispatcher captures NotifyAllSubtasksTerminal calls.
type fakeSubtaskDispatcher struct {
	lastResumeRootID string
}

func (d *fakeSubtaskDispatcher) NotifySubtaskProgress(_ context.Context, _ model.Task, _ string) {}
func (d *fakeSubtaskDispatcher) NotifyAllSubtasksTerminal(_ context.Context, rootID string) {
	d.lastResumeRootID = rootID
}
func (d *fakeSubtaskDispatcher) LastResumeWasFor(rootID string) bool {
	return d.lastResumeRootID == rootID
}

type subtaskTestHarness struct {
	taskStore  *store.TaskStore
	groupStore *store.GroupStore
	dispatcher *fakeSubtaskDispatcher
	msgID      string
	db         interface {
		Exec(string, ...any) (interface{}, error)
	}
}

func newTestSubtaskHandler(t *testing.T) (*subtaskTestHarness, *gin.Engine) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	db, err := store.InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	// Seed platform message
	msgID := uuid.New().String()
	db.Exec(`INSERT INTO bee_platform_messages (id, session_key, platform, content, raw, platform_msg_id, received_at, created_at, updated_at)
             VALUES (?, 'sk', 'feishu', 'hi', '', '', 1, 1, 1)`, msgID)
	// Seed a worker
	db.Exec(`INSERT INTO bee_workers (id,name,work_dir,status,created_at,updated_at) VALUES ('w1','Worker1','/','idle',1,1)`)

	ts := store.NewTaskStore(db)
	gs := store.NewGroupStore(db)
	d := &fakeSubtaskDispatcher{}

	h := NewSubtaskHandler(ts, gs, nil, d)

	router := gin.New()
	r := router.Group("/api/tasks")
	r.POST("/dispatch-subtask", h.Dispatch)
	r.GET("/subtasks", h.ListSubtasks)
	r.POST("/suspend", h.Suspend)
	r.POST("/mark-success", h.MarkSuccess)
	r.POST("/mark-failed", h.MarkFailed)

	harness := &subtaskTestHarness{
		taskStore:  ts,
		groupStore: gs,
		dispatcher: d,
		msgID:      msgID,
	}
	return harness, router
}

func (h *subtaskTestHarness) seedGroupRootTask(t *testing.T) string {
	t.Helper()
	ctx := context.Background()
	id, err := h.taskStore.Create(ctx, model.Task{
		MessageID:   h.msgID,
		WorkerID:    "g1",
		Instruction: "root task",
		Type:        model.TaskTypeImmediate,
		Status:      model.TaskStatusRunning,
		AgentKind:   model.AgentKindGroup,
	})
	if err != nil {
		t.Fatalf("seedGroupRootTask: %v", err)
	}
	return id
}

func (h *subtaskTestHarness) seedWorker(_ *testing.T) string {
	return "w1"
}

func (h *subtaskTestHarness) seedTerminalSubtask(t *testing.T, rootID, status string) string {
	t.Helper()
	ctx := context.Background()
	id, err := h.taskStore.Create(ctx, model.Task{
		MessageID:    h.msgID,
		WorkerID:     "w1",
		Instruction:  "sub",
		Type:         model.TaskTypeImmediate,
		Status:       model.TaskStatusPending,
		ParentTaskID: rootID,
		RootTaskID:   rootID,
		AgentKind:    model.AgentKindWorker,
	})
	if err != nil {
		t.Fatalf("seedTerminalSubtask create: %v", err)
	}
	// Update to the desired terminal status
	if err := h.taskStore.UpdateStatus(ctx, id, status); err != nil {
		t.Fatalf("seedTerminalSubtask update: %v", err)
	}
	return id
}

func testCtxAPI() context.Context { return context.Background() }

func TestSubtaskHandler_DispatchSubtask(t *testing.T) {
	h, router := newTestSubtaskHandler(t)
	rootID := h.seedGroupRootTask(t)
	workerID := h.seedWorker(t)

	body, _ := json.Marshal(map[string]any{
		"parent_task_id": rootID,
		"worker_id":      workerID,
		"instruction":    "fetch X",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/tasks/dispatch-subtask", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, body=%s", rec.Code, rec.Body.String())
	}
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if out["subtask_id"] == nil {
		t.Errorf("expected subtask_id in response, got %v", out)
	}
}

func TestSubtaskHandler_Suspend(t *testing.T) {
	h, router := newTestSubtaskHandler(t)
	rootID := h.seedGroupRootTask(t)

	body, _ := json.Marshal(map[string]any{"task_id": rootID})
	req := httptest.NewRequest(http.MethodPost, "/api/tasks/suspend", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d", rec.Code)
	}
	got, _ := h.taskStore.GetByID(testCtxAPI(), rootID)
	if got.Status != "waiting_subtasks" {
		t.Errorf("expected waiting_subtasks, got %s", got.Status)
	}
}

func TestSubtaskHandler_ListSubtasks(t *testing.T) {
	h, router := newTestSubtaskHandler(t)
	rootID := h.seedGroupRootTask(t)
	// Create 2 subtasks
	for i := 0; i < 2; i++ {
		h.taskStore.Create(testCtxAPI(), model.Task{
			MessageID:    h.msgID,
			WorkerID:     "w1",
			Instruction:  "sub",
			Type:         model.TaskTypeImmediate,
			Status:       model.TaskStatusPending,
			ParentTaskID: rootID,
			RootTaskID:   rootID,
			AgentKind:    model.AgentKindWorker,
		})
	}
	req := httptest.NewRequest(http.MethodGet, "/api/tasks/subtasks?task_id="+rootID, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d", rec.Code)
	}
	var list []any
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	if len(list) != 3 { // root + 2 subs
		t.Errorf("expected 3 tasks, got %d", len(list))
	}
}

func TestSubtaskHandler_MarkSuccess(t *testing.T) {
	h, router := newTestSubtaskHandler(t)
	rootID := h.seedGroupRootTask(t)

	body, _ := json.Marshal(map[string]any{"task_id": rootID})
	req := httptest.NewRequest(http.MethodPost, "/api/tasks/mark-success", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d", rec.Code)
	}
	got, _ := h.taskStore.GetByID(testCtxAPI(), rootID)
	if got.Status != model.TaskStatusCompleted {
		t.Errorf("expected completed, got %s", got.Status)
	}
}

func TestSubtaskHandler_Suspend_AllSubtasksDone_TriggersImmediateResume(t *testing.T) {
	h, router := newTestSubtaskHandler(t)
	rootID := h.seedGroupRootTask(t)
	h.seedTerminalSubtask(t, rootID, "completed")
	h.seedTerminalSubtask(t, rootID, "completed")

	body, _ := json.Marshal(map[string]any{"task_id": rootID})
	req := httptest.NewRequest(http.MethodPost, "/api/tasks/suspend", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d", rec.Code)
	}
	if !h.dispatcher.LastResumeWasFor(rootID) {
		t.Error("expected immediate resume because all subtasks already terminal")
	}
}
