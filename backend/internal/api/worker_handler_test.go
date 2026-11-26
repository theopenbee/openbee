package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	ai "github.com/theopenbee/openbee/internal/ai"
	"github.com/theopenbee/openbee/internal/ai/aitest"
	"github.com/theopenbee/openbee/internal/domain/enginecfg"
	"github.com/theopenbee/openbee/internal/domain/env"
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

func TestUpdateWorker_UninstalledEngineFormEdit(t *testing.T) {
	db, err := store.InitDB(t.TempDir() + "/test.db")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	workers := store.NewWorkerStore(db)
	src := aitest.NewSource(map[string]ai.EngineAdapter{"pi": &aitest.Engine{}})
	mgr := worker.NewManager(t.TempDir(), config.BeeConfig{}, workers, nil, src, nil, nil, nil)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.PUT("/workers/:id", NewWorkerHandler(workers, mgr, nil, "en").Update)
	w, err := workers.Create(model.Worker{Name: "coder", Engine: "codex", EngineArgs: `{"codex":"--model o3"}`})
	require.NoError(t, err)
	put := func(body string) (int, map[string]any) {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodPut, "/workers/"+w.ID, strings.NewReader(body)))
		var out map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out), rec.Body.String())
		return rec.Code, out
	}

	code, body := put(`{"description":"edited","engine":"codex","engine_args":{"codex":"--model o3"}}`)
	require.Equal(t, http.StatusOK, code, body)
	assert.Equal(t, "edited", body["description"])
	assert.Equal(t, "codex", body["engine"])

	code, body = put(`{"engine":"pi"}`)
	require.Equal(t, http.StatusOK, code, body)

	code, body = put(`{"engine":"codex"}`)
	assert.Equal(t, http.StatusBadRequest, code)
	assert.Contains(t, body["error"], `engine "codex" is not installed or enabled`)
	assert.Equal(t, "worker_validation", body["code"])
}

type engineModelsEnv struct {
	router  *gin.Engine
	workers *store.WorkerStore
	envs    *env.Service
	lister  *aitest.ModelEngine
	src     *aitest.Source
}

func newEngineModelsEnv(t *testing.T) *engineModelsEnv {
	t.Helper()
	db, err := store.InitDB(t.TempDir() + "/test.db")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	envs, err := env.NewService(store.NewEnvConfigStore(db), "0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f20")
	require.NoError(t, err)
	lister := &aitest.ModelEngine{}
	src := aitest.NewSource(map[string]ai.EngineAdapter{"opencode": lister, "pi": &aitest.Engine{}})
	workers := store.NewWorkerStore(db)
	mgr := worker.NewManager(t.TempDir(), config.BeeConfig{}, workers, nil, src, enginecfg.NewStore("pi", "", src.IDs), envs, nil)
	h := NewWorkerHandler(workers, mgr, nil, "en")
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/workers", h.Create)
	router.GET("/workers/:id", h.Get)
	router.PUT("/workers/:id", h.Update)
	router.GET("/engines/:id/models", h.EngineModels)
	return &engineModelsEnv{router: router, workers: workers, envs: envs, lister: lister, src: src}
}

func (e *engineModelsEnv) do(t *testing.T, method, path, body string) (int, map[string]any) {
	t.Helper()
	rec := httptest.NewRecorder()
	e.router.ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader(body)))
	var out map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out), rec.Body.String())
	return rec.Code, out
}

func TestWorkerEngineModelsFields(t *testing.T) {
	e := newEngineModelsEnv(t)

	code, body := e.do(t, http.MethodPost, "/workers", `{"name":"plain"}`)
	require.Equal(t, http.StatusCreated, code, body)
	assert.Equal(t, map[string]any{}, body["engine_models"])

	code, body = e.do(t, http.MethodPost, "/workers", `{"name":"coder","engine_models":{"opencode":" deepseek/deepseek-flash ","pi":""}}`)
	require.Equal(t, http.StatusCreated, code, body)
	assert.Equal(t, map[string]any{"opencode": "deepseek/deepseek-flash"}, body["engine_models"])
	id := body["id"].(string)

	code, body = e.do(t, http.MethodGet, "/workers/"+id, "")
	require.Equal(t, http.StatusOK, code, body)
	assert.Equal(t, map[string]any{"opencode": "deepseek/deepseek-flash"}, body["engine_models"])

	code, body = e.do(t, http.MethodPut, "/workers/"+id, `{"description":"edited"}`)
	require.Equal(t, http.StatusOK, code, body)
	assert.Equal(t, map[string]any{"opencode": "deepseek/deepseek-flash"}, body["engine_models"])

	code, body = e.do(t, http.MethodPut, "/workers/"+id, `{"engine_models":{"pi":"openai/gpt-5","opencode":""}}`)
	require.Equal(t, http.StatusOK, code, body)
	assert.Equal(t, map[string]any{"pi": "openai/gpt-5"}, body["engine_models"])

	code, body = e.do(t, http.MethodPut, "/workers/"+id, `{"engine_models":{}}`)
	require.Equal(t, http.StatusOK, code, body)
	assert.Equal(t, map[string]any{}, body["engine_models"])

	for _, req := range []struct{ method, path, body string }{
		{http.MethodPost, "/workers", `{"name":"bad","engine_models":{"pi":"--yolo"}}`},
		{http.MethodPost, "/workers", `{"name":"bad","engine_models":{"codex":"o3"}}`},
		{http.MethodPut, "/workers/" + id, `{"engine_models":{"pi":"openai/gpt 5"}}`},
		{http.MethodPut, "/workers/" + id, `{"engine_models":{"claude":"sonnet"}}`},
	} {
		code, body = e.do(t, req.method, req.path, req.body)
		assert.Equal(t, http.StatusBadRequest, code, req.body)
		assert.Equal(t, "worker_validation", body["code"], req.body)
	}
}

func TestEngineModelsEndpoint(t *testing.T) {
	t.Run("ListsWithGlobalEnv", func(t *testing.T) {
		e := newEngineModelsEnv(t)
		_, err := e.envs.Create(env.ScopeGlobal, "", "DEEPSEEK_API_KEY", "sk-global")
		require.NoError(t, err)
		e.lister.ModelsFunc = func(context.Context, []string) ([]ai.ModelInfo, error) {
			return []ai.ModelInfo{{ID: "deepseek/deepseek-flash", Name: "DeepSeek Flash"}, {ID: "opencode/big-pickle"}}, nil
		}

		rec := httptest.NewRecorder()
		e.router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/engines/opencode/models", nil))

		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		assert.JSONEq(t, `{"models":[{"id":"deepseek/deepseek-flash","name":"DeepSeek Flash"},{"id":"opencode/big-pickle"}]}`, rec.Body.String())
		assert.Equal(t, [][]string{{"DEEPSEEK_API_KEY=sk-global"}}, e.lister.ModelCalls())
	})

	t.Run("UsesWorkerEnv", func(t *testing.T) {
		e := newEngineModelsEnv(t)
		w, err := e.workers.Create(model.Worker{Name: "coder"})
		require.NoError(t, err)
		_, err = e.envs.Create(env.ScopeGlobal, "", "DEEPSEEK_API_KEY", "sk-global")
		require.NoError(t, err)
		_, err = e.envs.Create(env.ScopeWorker, w.ID, "DEEPSEEK_API_KEY", "sk-worker")
		require.NoError(t, err)

		code, body := e.do(t, http.MethodGet, "/engines/opencode/models?worker_id="+w.ID, "")

		require.Equal(t, http.StatusOK, code, body)
		assert.Equal(t, []any{}, body["models"])
		assert.Equal(t, [][]string{{"DEEPSEEK_API_KEY=sk-worker"}}, e.lister.ModelCalls())
	})

	t.Run("EngineWithoutListReturnsEmpty", func(t *testing.T) {
		e := newEngineModelsEnv(t)
		code, body := e.do(t, http.MethodGet, "/engines/pi/models", "")
		require.Equal(t, http.StatusOK, code, body)
		assert.Equal(t, []any{}, body["models"])
	})

	t.Run("UnknownEngine", func(t *testing.T) {
		e := newEngineModelsEnv(t)
		e.src.Remove("opencode")
		code, body := e.do(t, http.MethodGet, "/engines/opencode/models", "")
		assert.Equal(t, http.StatusNotFound, code)
		assert.Equal(t, "engine_not_found", body["code"])
		assert.Empty(t, e.lister.ModelCalls())
	})

	t.Run("UnknownWorker", func(t *testing.T) {
		e := newEngineModelsEnv(t)
		code, body := e.do(t, http.MethodGet, "/engines/opencode/models?worker_id=missing", "")
		assert.Equal(t, http.StatusNotFound, code)
		assert.Equal(t, "worker_not_found", body["code"])
		assert.Empty(t, e.lister.ModelCalls())
	})

	t.Run("PluginErrorIsBadGateway", func(t *testing.T) {
		e := newEngineModelsEnv(t)
		e.lister.ModelsFunc = func(context.Context, []string) ([]ai.ModelInfo, error) {
			return nil, errors.New(`unknown method "models"`)
		}
		code, body := e.do(t, http.MethodGet, "/engines/opencode/models", "")
		assert.Equal(t, http.StatusBadGateway, code)
		assert.Equal(t, map[string]any{"error": `unknown method "models"`}, body)
	})
}

type stubAvatars struct{ refs map[string]bool }

func (s stubAvatars) HasAvatar(ref string) bool { return s.refs[ref] }

func (s stubAvatars) RandomAvatar(map[string]bool) string { return "mfers/b" }

func TestWorkerAvatarField(t *testing.T) {
	e := newEngineModelsEnv(t)
	avatars := stubAvatars{refs: map[string]bool{"mfers/a": true, "mfers/b": true}}
	mgr := worker.NewManager(t.TempDir(), config.BeeConfig{}, e.workers, nil, e.src, enginecfg.NewStore("pi", "", e.src.IDs), e.envs, nil)
	mgr.SetAvatarSource(avatars)
	h := NewWorkerHandler(e.workers, mgr, nil, "en")
	router := gin.New()
	router.POST("/workers", h.Create)
	router.GET("/workers", h.List)
	router.PUT("/workers/:id", h.Update)
	e.router = router

	code, body := e.do(t, http.MethodPost, "/workers", `{"name":"random"}`)
	require.Equal(t, http.StatusCreated, code, body)
	assert.Equal(t, "mfers/b", body["avatar"])

	code, body = e.do(t, http.MethodPost, "/workers", `{"name":"chosen","avatar":"mfers/a"}`)
	require.Equal(t, http.StatusCreated, code, body)
	assert.Equal(t, "mfers/a", body["avatar"])
	id := body["id"].(string)

	code, body = e.do(t, http.MethodPost, "/workers", `{"name":"bad","avatar":"mfers/zz"}`)
	assert.Equal(t, http.StatusBadRequest, code)
	assert.Equal(t, "worker_validation", body["code"])

	code, body = e.do(t, http.MethodPut, "/workers/"+id, `{"avatar":""}`)
	require.Equal(t, http.StatusOK, code, body)
	assert.Equal(t, "", body["avatar"])

	stored, err := e.workers.Create(model.Worker{Name: "orphan", Avatar: "gone/1"})
	require.NoError(t, err)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/workers", nil))
	var list []map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &list))
	for _, w := range list {
		if w["id"] == stored.ID {
			assert.Equal(t, "", w["avatar"])
		}
	}
	raw, err := e.workers.GetByID(stored.ID)
	require.NoError(t, err)
	assert.Equal(t, "gone/1", raw.Avatar)
}
