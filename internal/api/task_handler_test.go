package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/theopenbee/openbee/internal/infra/auth"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/infra/store"
)

func newTestServerWithTasks(t *testing.T) (*gin.Engine, *store.TaskStore) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	db, err := store.InitDB(t.TempDir() + "/test.db")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	for _, q := range []string{
		`INSERT INTO bee_workers (id,name,work_dir,status,created_at,updated_at) VALUES ('w1','Worker1','/','idle',1,1)`,
		`INSERT INTO bee_workers (id,name,work_dir,status,created_at,updated_at) VALUES ('w2','Worker2','/','idle',1,1)`,
		`INSERT INTO bee_platform_messages (id,session_key,platform,content,raw,platform_msg_id,received_at,created_at,updated_at) VALUES ('m1','s1','feishu','hi','','',1,1,1)`,
		`INSERT INTO bee_platform_messages (id,session_key,platform,content,raw,platform_msg_id,received_at,created_at,updated_at) VALUES ('m2','s1','feishu','again','','',2,2,2)`,
		`INSERT INTO bee_platform_messages (id,session_key,platform,content,raw,platform_msg_id,received_at,created_at,updated_at) VALUES ('m3','local:user-1','local','mine','','',3,3,3)`,
		`INSERT INTO bee_platform_messages (id,session_key,platform,content,raw,platform_msg_id,received_at,created_at,updated_at) VALUES ('m4','local:user-2','local','theirs','','',4,4,4)`,
	} {
		_, err := db.Exec(q)
		require.NoError(t, err)
	}

	taskStore := store.NewTaskStore(db)
	workerStore := store.NewWorkerStore(db)
	msgStore := store.NewMessageStore(db)

	h := NewTaskHandler(taskStore, workerStore, msgStore, taskStore)
	router := gin.New()
	api := router.Group("/api", func(c *gin.Context) {
		auth.SetUserID(c, testUserID)
	})
	api.GET("/tasks", h.List)
	api.GET("/tasks/targets", h.Targets)
	api.POST("/tasks", h.Create)
	api.PUT("/tasks/:id", h.Update)
	api.DELETE("/tasks/:id", h.Cancel)
	api.POST("/workers/:id/tasks/cancel-all", h.CancelByWorker)

	return router, taskStore
}

func TestListTasks_FiltersByTypeAndStatus(t *testing.T) {
	router, ts := newTestServerWithTasks(t)

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
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp struct {
		Items []map[string]any `json:"items"`
		Total int              `json:"total"`
	}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(t, 2, resp.Total)
	for _, item := range resp.Items {
		assert.NotEqual(t, "immediate", item["type"], "immediate task should not appear in default list")
	}
}

// TestCancelTask_Succeeds merges the pending- and running-task cancellation
// variants: both seed a single task at the given starting status, cancel it
// over HTTP, and expect it to land in TaskStatusCancelled.
func TestCancelTask_Succeeds(t *testing.T) {
	cases := []struct {
		name   string
		status string
	}{
		{name: "Pending", status: model.TaskStatusPending},
		{name: "Running", status: model.TaskStatusRunning},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			router, ts := newTestServerWithTasks(t)

			ctx := context.Background()
			now := time.Now().UnixMilli()
			id, _ := ts.Create(ctx, model.Task{
				MessageID: "m1", WorkerID: "w1", Instruction: "x",
				Type: model.TaskTypeScheduled, Status: model.TaskStatusPending,
				CreatedAt: now, UpdatedAt: now,
			})
			if tc.status != model.TaskStatusPending {
				ts.UpdateStatus(ctx, id, tc.status)
			}

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodDelete, "/api/tasks/"+id, nil)
			router.ServeHTTP(w, req)
			require.Equal(t, http.StatusNoContent, w.Code, w.Body.String())

			task, _ := ts.GetByID(ctx, id)
			assert.Equal(t, model.TaskStatusCancelled, task.Status)
		})
	}
}

func TestCancelTask_CompletedReturns409(t *testing.T) {
	router, ts := newTestServerWithTasks(t)

	ctx := context.Background()
	now := time.Now().UnixMilli()
	id, _ := ts.Create(ctx, model.Task{
		MessageID: "m1", WorkerID: "w1", Instruction: "x",
		Type: model.TaskTypeScheduled, Status: model.TaskStatusPending,
		CreatedAt: now, UpdatedAt: now,
	})
	ts.UpdateStatus(ctx, id, model.TaskStatusCompleted)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/tasks/"+id, nil)
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusConflict, w.Code)
}

func TestCancelWorkerTasks_CancelsAllPending(t *testing.T) {
	router, ts := newTestServerWithTasks(t)

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
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusNoContent, w.Code, w.Body.String())

	tasks, _ := ts.List(ctx, store.TaskFilter{WorkerID: "w1", Status: "cancelled"})
	assert.Len(t, tasks, 3)
}

func doJSON(t *testing.T, router *gin.Engine, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)
	return w
}

func decodeBody[T any](t *testing.T, w *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &v), w.Body.String())
	return v
}

func TestTaskTargets_ListsOwnLocalSessionOnly(t *testing.T) {
	router, _ := newTestServerWithTasks(t)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/tasks/targets", nil))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	resp := decodeBody[struct {
		Items []taskTargetResponse `json:"items"`
	}](t, w)
	assert.Equal(t, []taskTargetResponse{
		{SessionKey: "local:user-1", Platform: "local", LastActiveAt: 3},
		{SessionKey: "s1", Platform: "feishu", LastActiveAt: 2},
	}, resp.Items)
}

func TestCreateTask_Scheduled(t *testing.T) {
	router, ts := newTestServerWithTasks(t)

	w := doJSON(t, router, http.MethodPost, "/api/tasks", map[string]any{
		"session_key": "s1",
		"worker_id":   "w1",
		"instruction": "  daily report  ",
		"type":        "scheduled",
		"cron_expr":   "0 9 * * *",
	})
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	resp := decodeBody[taskResponse](t, w)
	assert.Equal(t, "Worker1", resp.WorkerName)
	assert.Equal(t, "daily report", resp.Instruction)
	assert.Equal(t, model.TaskStatusPending, resp.Status)
	require.NotNil(t, resp.NextRunAt)
	assert.Greater(t, *resp.NextRunAt, time.Now().UnixMilli())

	task, err := ts.GetByID(context.Background(), resp.ID)
	require.NoError(t, err)
	assert.Equal(t, "m2", task.MessageID)
	assert.Equal(t, model.TaskTypeScheduled, task.Type)
	assert.Equal(t, "0 9 * * *", task.CronExpr)
	assert.Nil(t, task.ScheduledAt)
}

func TestCreateTask_Countdown(t *testing.T) {
	router, ts := newTestServerWithTasks(t)
	at := time.Now().Add(time.Hour).UnixMilli()

	w := doJSON(t, router, http.MethodPost, "/api/tasks", map[string]any{
		"session_key":  "local:user-1",
		"worker_id":    "w2",
		"instruction":  "remind me",
		"type":         "countdown",
		"scheduled_at": at,
		"cron_expr":    "0 9 * * *",
	})
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	resp := decodeBody[taskResponse](t, w)
	task, err := ts.GetByID(context.Background(), resp.ID)
	require.NoError(t, err)
	assert.Equal(t, "m3", task.MessageID)
	assert.Equal(t, "w2", task.WorkerID)
	require.NotNil(t, task.ScheduledAt)
	assert.Equal(t, at, *task.ScheduledAt)
	assert.Empty(t, task.CronExpr)
	assert.Nil(t, task.NextRunAt)
}

func TestCreateTask_RejectsInvalidInput(t *testing.T) {
	past := time.Now().Add(-time.Minute).UnixMilli()
	valid := map[string]any{
		"session_key": "s1",
		"worker_id":   "w1",
		"instruction": "x",
		"type":        "scheduled",
		"cron_expr":   "0 9 * * *",
	}
	cases := []struct {
		name     string
		override map[string]any
		code     string
	}{
		{"EmptyInstruction", map[string]any{"instruction": "   "}, "task_instruction_required"},
		{"ImmediateType", map[string]any{"type": "immediate"}, "task_type_invalid"},
		{"BadCron", map[string]any{"cron_expr": "every day"}, "task_cron_invalid"},
		{"PastCountdown", map[string]any{"type": "countdown", "scheduled_at": past}, "task_scheduled_at_invalid"},
		{"MissingCountdownTime", map[string]any{"type": "countdown"}, "task_scheduled_at_invalid"},
		{"UnknownWorker", map[string]any{"worker_id": "nope"}, "worker_not_found"},
		{"MissingSession", map[string]any{"session_key": ""}, "task_target_not_found"},
		{"UnknownSession", map[string]any{"session_key": "missing"}, "task_target_not_found"},
		{"OtherUsersLocalSession", map[string]any{"session_key": "local:user-2"}, "task_target_not_found"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			router, ts := newTestServerWithTasks(t)
			body := map[string]any{}
			for k, v := range valid {
				body[k] = v
			}
			for k, v := range tc.override {
				body[k] = v
			}

			w := doJSON(t, router, http.MethodPost, "/api/tasks", body)
			require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
			assert.Equal(t, tc.code, decodeBody[map[string]any](t, w)["code"])

			count, err := ts.CountTasks(context.Background(), store.TaskFilter{})
			require.NoError(t, err)
			assert.Zero(t, count)
		})
	}
}

func TestUpdateTask_ChangesScheduleAndKeepsTarget(t *testing.T) {
	router, ts := newTestServerWithTasks(t)
	ctx := context.Background()
	id, err := ts.Create(ctx, model.Task{
		MessageID: "m1", WorkerID: "w1", Instruction: "old",
		Type: model.TaskTypeScheduled, Status: model.TaskStatusPending, CronExpr: "0 * * * *",
	})
	require.NoError(t, err)
	at := time.Now().Add(2 * time.Hour).UnixMilli()

	w := doJSON(t, router, http.MethodPut, "/api/tasks/"+id, map[string]any{
		"worker_id":    "w2",
		"instruction":  "new",
		"type":         "countdown",
		"scheduled_at": at,
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, "Worker2", decodeBody[taskResponse](t, w).WorkerName)

	task, err := ts.GetByID(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, "m1", task.MessageID)
	assert.Equal(t, "w2", task.WorkerID)
	assert.Equal(t, "new", task.Instruction)
	assert.Equal(t, model.TaskTypeCountdown, task.Type)
	assert.Empty(t, task.CronExpr)
	assert.Nil(t, task.NextRunAt)
	require.NotNil(t, task.ScheduledAt)
	assert.Equal(t, at, *task.ScheduledAt)
}

func TestUpdateTask_RecomputesNextRun(t *testing.T) {
	router, ts := newTestServerWithTasks(t)
	ctx := context.Background()
	stale := time.Now().Add(-time.Hour).UnixMilli()
	id, err := ts.Create(ctx, model.Task{
		MessageID: "m1", WorkerID: "w1", Instruction: "x",
		Type: model.TaskTypeScheduled, Status: model.TaskStatusPending,
		CronExpr: "0 * * * *", NextRunAt: &stale,
	})
	require.NoError(t, err)

	w := doJSON(t, router, http.MethodPut, "/api/tasks/"+id, map[string]any{
		"worker_id":   "w1",
		"instruction": "x",
		"type":        "scheduled",
		"cron_expr":   "*/10 * * * *",
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	task, err := ts.GetByID(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, "*/10 * * * *", task.CronExpr)
	require.NotNil(t, task.NextRunAt)
	assert.Greater(t, *task.NextRunAt, time.Now().UnixMilli())
}

func TestUpdateTask_Errors(t *testing.T) {
	body := map[string]any{"worker_id": "w1", "instruction": "x", "type": "scheduled", "cron_expr": "0 * * * *"}

	t.Run("NotFound", func(t *testing.T) {
		router, _ := newTestServerWithTasks(t)
		w := doJSON(t, router, http.MethodPut, "/api/tasks/missing", body)
		require.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
		assert.Equal(t, "task_not_found", decodeBody[map[string]any](t, w)["code"])
	})

	for _, tc := range []struct {
		name     string
		taskType string
		status   string
	}{
		{"Running", model.TaskTypeScheduled, model.TaskStatusRunning},
		{"Cancelled", model.TaskTypeScheduled, model.TaskStatusCancelled},
		{"Immediate", model.TaskTypeImmediate, model.TaskStatusPending},
	} {
		t.Run(tc.name, func(t *testing.T) {
			router, ts := newTestServerWithTasks(t)
			ctx := context.Background()
			id, err := ts.Create(ctx, model.Task{
				MessageID: "m1", WorkerID: "w1", Instruction: "old",
				Type: tc.taskType, Status: tc.status, CronExpr: "0 * * * *",
			})
			require.NoError(t, err)

			w := doJSON(t, router, http.MethodPut, "/api/tasks/"+id, body)
			require.Equal(t, http.StatusConflict, w.Code, w.Body.String())
			assert.Equal(t, "task_not_editable", decodeBody[map[string]any](t, w)["code"])

			task, err := ts.GetByID(ctx, id)
			require.NoError(t, err)
			assert.Equal(t, "old", task.Instruction)
		})
	}

	t.Run("InvalidCron", func(t *testing.T) {
		router, ts := newTestServerWithTasks(t)
		ctx := context.Background()
		id, err := ts.Create(ctx, model.Task{
			MessageID: "m1", WorkerID: "w1", Instruction: "old",
			Type: model.TaskTypeScheduled, Status: model.TaskStatusPending, CronExpr: "0 * * * *",
		})
		require.NoError(t, err)

		w := doJSON(t, router, http.MethodPut, "/api/tasks/"+id, map[string]any{
			"worker_id": "w1", "instruction": "x", "type": "scheduled", "cron_expr": "61 * * * *",
		})
		require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
		assert.Equal(t, "task_cron_invalid", decodeBody[map[string]any](t, w)["code"])
	})
}
