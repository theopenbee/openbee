package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/theopenbee/openbee/internal/domain/localchat"
	"github.com/theopenbee/openbee/internal/domain/worker"
	"github.com/theopenbee/openbee/internal/infra/config"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/infra/store"
	"github.com/theopenbee/openbee/internal/platform/local"
)

func TestToWorkerResponse_ParsesEngineArgs(t *testing.T) {
	resp, err := toWorkerResponse(model.Worker{
		ID:         "w1",
		Name:       "worker",
		EngineArgs: `{"claude":"--model claude-sonnet-4-5"}`,
	})
	require.NoError(t, err)
	require.Equal(t, "--model claude-sonnet-4-5", resp.EngineArgs["claude"])
}

func TestToWorkerResponse_EmptyEngineArgsUsesEmptyMap(t *testing.T) {
	resp, err := toWorkerResponse(model.Worker{ID: "w1", Name: "worker"})
	require.NoError(t, err)
	require.NotNil(t, resp.EngineArgs)
	require.Empty(t, resp.EngineArgs)
}

func TestToWorkerResponse_InvalidEngineArgsJSON(t *testing.T) {
	_, err := toWorkerResponse(model.Worker{
		ID:         "w1",
		Name:       "worker",
		EngineArgs: `{"claude":`,
	})
	require.Error(t, err)
}

type recordingWorkerChatCleaner struct {
	deleted []string
	err     error
}

func (r *recordingWorkerChatCleaner) DeleteWorkerChats(_ context.Context, w model.Worker) error {
	r.deleted = append(r.deleted, w.ID)
	return r.err
}

type workerDeleteEnv struct {
	router  *gin.Engine
	workers *store.WorkerStore
	msgs    *store.MessageStore
	chats   *store.LocalChatStore
	tasks   *store.TaskStore
	clear   *fakeClearer
}

func newWorkerDeleteEnv(t *testing.T, cleaner func(env *workerDeleteEnv) WorkerChatCleaner) *workerDeleteEnv {
	t.Helper()
	db, err := store.InitDB(t.TempDir() + "/test.db")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	env := &workerDeleteEnv{
		workers: store.NewWorkerStore(db),
		msgs:    store.NewMessageStore(db),
		chats:   store.NewLocalChatStore(db),
		tasks:   store.NewTaskStore(db),
		clear:   &fakeClearer{},
	}
	mgr := worker.NewManager(t.TempDir(), config.BeeConfig{}, env.workers, nil, nil, nil, nil, nil)
	h := NewWorkerHandler(env.workers, mgr, cleaner(env), "en")
	gin.SetMode(gin.TestMode)
	env.router = gin.New()
	env.router.DELETE("/workers/:id", h.Delete)
	return env
}

func (e *workerDeleteEnv) delete(id string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	e.router.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/workers/"+id, nil))
	return rec
}

func TestDeleteWorkerRemovesChats(t *testing.T) {
	env := newWorkerDeleteEnv(t, func(env *workerDeleteEnv) WorkerChatCleaner {
		return localchat.NewChatCleanup(env.chats, env.clear, env.tasks)
	})
	w, err := env.workers.Create(model.Worker{Name: "worker"})
	require.NoError(t, err)
	other, err := env.workers.Create(model.Worker{Name: "other"})
	require.NoError(t, err)
	ctx := context.Background()
	key := local.WorkerSessionKey("u1", w.ID)
	otherKey := local.WorkerSessionKey("u1", other.ID)
	for _, k := range []string{key, otherKey} {
		require.NoError(t, env.msgs.InsertProcessed(ctx, store.ProcessedMsg{ID: "m-" + k, SessionKey: k, Platform: local.PlatformID, Content: "hello", SenderType: store.SenderTypeUser}))
	}
	_, ok, err := env.chats.LastMessage(ctx, key)
	require.NoError(t, err)
	require.True(t, ok)

	rec := env.delete(w.ID)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	_, err = env.workers.GetByID(w.ID)
	require.Error(t, err)
	_, ok, err = env.chats.LastMessage(ctx, key)
	require.NoError(t, err)
	assert.False(t, ok)
	_, ok, err = env.chats.LastMessage(ctx, otherKey)
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, []string{"stop:" + w.ID}, env.clear.callList())
	assert.Equal(t, []string{key}, env.clear.keyList())
}

func TestDeleteWorkerKeepsWorkerWhenChatCleanupFails(t *testing.T) {
	cleaner := &recordingWorkerChatCleaner{err: errors.New("cleanup failed")}
	env := newWorkerDeleteEnv(t, func(*workerDeleteEnv) WorkerChatCleaner { return cleaner })
	w, err := env.workers.Create(model.Worker{Name: "worker"})
	require.NoError(t, err)

	rec := env.delete(w.ID)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Equal(t, []string{w.ID}, cleaner.deleted)
	_, err = env.workers.GetByID(w.ID)
	assert.NoError(t, err)
}

func TestDeleteWorkerNotFound(t *testing.T) {
	cleaner := &recordingWorkerChatCleaner{}
	env := newWorkerDeleteEnv(t, func(*workerDeleteEnv) WorkerChatCleaner { return cleaner })

	rec := env.delete("missing")

	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Empty(t, cleaner.deleted)
}
