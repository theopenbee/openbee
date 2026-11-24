package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	ai "github.com/theopenbee/openbee/internal/ai"
	"github.com/theopenbee/openbee/internal/ai/aitest"
	"github.com/theopenbee/openbee/internal/domain/execution"
	"github.com/theopenbee/openbee/internal/infra/auth"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/infra/store"
	"github.com/theopenbee/openbee/internal/platform/local"
)

type executionEnv struct {
	router  *gin.Engine
	execs   *store.ExecutionStore
	tasks   *store.TaskStore
	engine  *aitest.TranscriptEngine
	worker  model.Worker
	message string
}

func newExecutionEnv(t *testing.T) *executionEnv {
	t.Helper()
	db, err := store.InitDB(t.TempDir() + "/test.db")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	engine := &aitest.TranscriptEngine{
		TranscriptFunc: func(context.Context, string) ([]ai.TranscriptStep, error) {
			return []ai.TranscriptStep{
				{Type: "text", Text: "working on it"},
				{Type: "tool", Kind: "command", Name: "Bash", Summary: "ls", Output: "a.txt", Status: "done", DurationMS: 12},
			}, nil
		},
	}
	e := &executionEnv{
		execs:  store.NewExecutionStore(db, t.TempDir()),
		tasks:  store.NewTaskStore(db),
		engine: engine,
	}
	e.worker, err = store.NewWorkerStore(db).Create(model.Worker{Name: "coder"})
	require.NoError(t, err)
	e.message = "m1"
	_, err = db.Exec(`INSERT INTO bee_platform_messages
		(id, session_key, platform, content, raw, platform_msg_id, received_at, created_at, updated_at)
		VALUES (?, ?, 'local', 'hi', '', '', 1, 1, 1)`, e.message, local.SessionKey(testUserID))
	require.NoError(t, err)

	svc := execution.New(e.execs, e.tasks, aitest.NewSource(map[string]ai.EngineAdapter{"claude": engine}))
	gin.SetMode(gin.TestMode)
	e.router = gin.New()
	e.router.Use(func(c *gin.Context) {
		auth.SetUserID(c, c.GetHeader(testUserHeader))
		c.Next()
	})
	e.router.GET("/executions/:id", NewExecutionHandler(svc).Get)
	return e
}

func (e *executionEnv) newTask(t *testing.T, sessionKey string) string {
	t.Helper()
	id, err := e.tasks.Create(context.Background(), model.Task{
		MessageID: e.message, SessionKey: sessionKey, Platform: "local", WorkerID: e.worker.ID,
		Instruction: "list files", Type: model.TaskTypeImmediate, Status: model.TaskStatusRunning,
	})
	require.NoError(t, err)
	return id
}

func (e *executionEnv) newExecution(t *testing.T, taskID string) model.WorkerExecution {
	t.Helper()
	exec, err := e.execs.Create(store.ExecutionCreate{WorkerID: e.worker.ID, TaskID: taskID, Engine: "claude", SessionID: "s1"})
	require.NoError(t, err)
	return exec
}

func (e *executionEnv) get(userID, id string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/executions/"+id, nil)
	req.Header.Set(testUserHeader, userID)
	rec := httptest.NewRecorder()
	e.router.ServeHTTP(rec, req)
	return rec
}

func TestExecutionGetCompleted(t *testing.T) {
	e := newExecutionEnv(t)
	taskID := e.newTask(t, local.WorkerSessionKey(testUserID, e.worker.ID))
	exec := e.newExecution(t, taskID)
	logPath, err := e.execs.PrepareLogPath(exec.ID, exec.StartedAt)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(logPath, []byte("{}\n"), 0o644))
	require.NoError(t, e.execs.UpdateResult(exec.ID, "done", model.ExecStatusCompleted))
	stored, err := e.execs.GetByID(exec.ID)
	require.NoError(t, err)

	rec := e.get(testUserID, exec.ID)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.JSONEq(t, fmt.Sprintf(`{
		"id": %q, "task_id": %q, "worker_id": %q, "engine": "claude",
		"status": "completed", "instruction": "list files",
		"started_at": %d, "completed_at": %d, "error": "",
		"steps": [
			{"type": "text", "text": "working on it"},
			{"type": "tool", "kind": "command", "name": "Bash", "summary": "ls", "output": "a.txt", "status": "done", "duration_ms": 12}
		],
		"steps_error": ""
	}`, exec.ID, taskID, e.worker.ID, *stored.StartedAt, *stored.CompletedAt), rec.Body.String())
}

func TestExecutionGetBeeRun(t *testing.T) {
	e := newExecutionEnv(t)
	exec, err := e.execs.Create(store.ExecutionCreate{
		SessionID: "s2", SessionKey: local.SessionKey(testUserID), Instruction: "plan my day", Engine: "claude",
	})
	require.NoError(t, err)
	logPath, err := e.execs.PrepareLogPath(exec.ID, exec.StartedAt)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(logPath, []byte("{}\n"), 0o644))

	rec := e.get(testUserID, exec.ID)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.JSONEq(t, fmt.Sprintf(`{
		"id": %q, "task_id": "", "worker_id": "", "engine": "claude",
		"status": "running", "instruction": "plan my day",
		"started_at": %d, "completed_at": null, "error": "",
		"steps": [
			{"type": "text", "text": "working on it"},
			{"type": "tool", "kind": "command", "name": "Bash", "summary": "ls", "output": "a.txt", "status": "done", "duration_ms": 12}
		],
		"steps_error": ""
	}`, exec.ID, *exec.StartedAt), rec.Body.String())

	other := e.get("someone-else", exec.ID)
	require.Equal(t, http.StatusNotFound, other.Code, other.Body.String())
}

func TestExecutionGetPendingWithRuns(t *testing.T) {
	e := newExecutionEnv(t)
	taskID := e.newTask(t, local.SessionKey(testUserID))
	first := e.newExecution(t, taskID)
	require.NoError(t, e.execs.UpdateResult(first.ID, "network down", model.ExecStatusFailed))
	second := e.newExecution(t, taskID)

	rec := e.get(testUserID, second.ID)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "running", body["status"])
	assert.Nil(t, body["completed_at"])
	assert.Contains(t, body, "completed_at")
	assert.Equal(t, []any{}, body["steps"])
	assert.Equal(t, "", body["steps_error"])
	runs, ok := body["runs"].([]any)
	require.True(t, ok, "runs missing: %v", body)
	require.Len(t, runs, 2)
	assert.Equal(t, second.ID, runs[0].(map[string]any)["id"])
	assert.Equal(t, "running", runs[0].(map[string]any)["status"])
	assert.Equal(t, first.ID, runs[1].(map[string]any)["id"])
	assert.Equal(t, "failed", runs[1].(map[string]any)["status"])

	rec = e.get(testUserID, first.ID)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "failed", body["status"])
	assert.Equal(t, "network down", body["error"])
}

func TestExecutionGetSingleRunOmitsRuns(t *testing.T) {
	e := newExecutionEnv(t)
	exec := e.newExecution(t, e.newTask(t, local.SessionKey(testUserID)))

	rec := e.get(testUserID, exec.ID)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.NotContains(t, body, "runs")
}

func TestExecutionGetNotFound(t *testing.T) {
	e := newExecutionEnv(t)
	otherTask := e.newTask(t, local.SessionKey("someone-else"))
	otherExec := e.newExecution(t, otherTask)
	beeExec, err := e.execs.Create(store.ExecutionCreate{SessionID: "s2", Engine: "claude"})
	require.NoError(t, err)

	for name, id := range map[string]string{
		"Missing":    "no-such-execution",
		"LegacyBee":  beeExec.ID,
		"OtherUsers": otherExec.ID,
	} {
		t.Run(name, func(t *testing.T) {
			rec := e.get(testUserID, id)
			require.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
			assert.JSONEq(t, `{"error":"execution not found","code":"execution_not_found"}`, rec.Body.String())
		})
	}
}
