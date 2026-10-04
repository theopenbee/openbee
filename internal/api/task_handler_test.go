package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/infra/store"
)

func newTestServerWithTasks(t *testing.T) (*gin.Engine, *store.TaskStore) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	db, err := store.InitDB(t.TempDir() + "/test.db")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	db.Exec(`INSERT INTO bee_workers (id,name,work_dir,status,created_at,updated_at) VALUES ('w1','Worker1','/','idle',1,1)`)
	db.Exec(`INSERT INTO bee_platform_messages (id,session_key,platform,content,raw,platform_msg_id,received_at,created_at,updated_at) VALUES ('m1','s1','feishu','hi','','',1,1,1)`)

	taskStore := store.NewTaskStore(db)
	workerStore := store.NewWorkerStore(db)

	h := NewTaskHandler(taskStore, workerStore, taskStore)
	router := gin.New()
	api := router.Group("/api")
	api.GET("/tasks", h.List)
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
