package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/infra/store"
)

func newTestServer(t *testing.T, register func(*gin.RouterGroup, *ExecutionHandler)) (*gin.Engine, *store.ExecutionStore, *store.TokenStatsStore, func()) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := store.InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	es := store.NewExecutionStore(db, t.TempDir())
	ts := store.NewTokenStatsStore(db)
	h := NewExecutionHandler(es, ts)
	router := gin.New()
	api := router.Group("/api")
	register(api, h)
	return router, es, ts, func() { db.Close() }
}

func newTestServerWithExecutions(t *testing.T) (*gin.Engine, *store.ExecutionStore, *store.TokenStatsStore, func()) {
	return newTestServer(t, func(api *gin.RouterGroup, h *ExecutionHandler) {
		api.GET("/executions", h.List)
	})
}

func TestExecutionsList_IncludesTokenStats(t *testing.T) {
	router, es, ts, cleanup := newTestServerWithExecutions(t)
	defer cleanup()

	if _, err := es.Create("worker-1", "hello", "session-abc", "claude"); err != nil {
		t.Fatalf("Create execution: %v", err)
	}
	if err := ts.Upsert(model.TokenStats{
		SessionID:    "session-abc",
		AgentType:    "claude",
		Model:        "claude-sonnet-4-6",
		InputTokens:  100,
		OutputTokens: 200,
		TotalTokens:  300,
		SyncedAt:     time.Now().UnixMilli(),
	}); err != nil {
		t.Fatalf("Upsert token stats: %v", err)
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/executions", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	statsRaw, ok := resp["token_stats"]
	if !ok {
		t.Fatal("expected token_stats field in response")
	}
	statsMap, ok := statsRaw.(map[string]any)
	if !ok {
		t.Fatalf("token_stats must be a map, got %T", statsRaw)
	}
	if _, found := statsMap["session-abc"]; !found {
		t.Errorf("expected session-abc in token_stats, got keys: %v", statsMap)
	}
}

func TestExecutionsList_NoTokenStats_WhenNoneExist(t *testing.T) {
	router, es, _, cleanup := newTestServerWithExecutions(t)
	defer cleanup()

	if _, err := es.Create("worker-1", "hello", "session-xyz", "claude"); err != nil {
		t.Fatalf("Create execution: %v", err)
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/executions", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	if ts, ok := resp["token_stats"].(map[string]any); ok {
		if _, found := ts["session-xyz"]; found {
			t.Error("session-xyz must not appear in token_stats when no stats were upserted")
		}
	}
}

func newTestServerWithSessions(t *testing.T) (*gin.Engine, *store.ExecutionStore, *store.TokenStatsStore, func()) {
	return newTestServer(t, func(api *gin.RouterGroup, h *ExecutionHandler) {
		api.GET("/sessions/:id", h.GetSession)
	})
}

func TestGetSession_IncludesTokenStats(t *testing.T) {
	router, es, ts, cleanup := newTestServerWithSessions(t)
	defer cleanup()

	if _, err := es.Create("worker-1", "hello", "session-abc", "claude"); err != nil {
		t.Fatalf("Create execution: %v", err)
	}
	if err := ts.Upsert(model.TokenStats{
		SessionID:    "session-abc",
		AgentType:    "claude",
		Model:        "claude-sonnet-4-6",
		InputTokens:  100,
		OutputTokens: 200,
		TotalTokens:  300,
		SyncedAt:     time.Now().UnixMilli(),
	}); err != nil {
		t.Fatalf("Upsert token stats: %v", err)
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/sessions/session-abc", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if _, ok := resp["executions"]; !ok {
		t.Fatal("expected executions field")
	}
	stats, ok := resp["token_stats"].(map[string]any)
	if !ok {
		t.Fatalf("expected token_stats as object, got %T", resp["token_stats"])
	}
	if _, found := stats["total_tokens"]; !found {
		t.Error("expected total_tokens in token_stats")
	}
}

func TestGetSession_NullTokenStats_WhenNoneExist(t *testing.T) {
	router, es, _, cleanup := newTestServerWithSessions(t)
	defer cleanup()

	if _, err := es.Create("worker-1", "hello", "session-xyz", "claude"); err != nil {
		t.Fatalf("Create execution: %v", err)
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/sessions/session-xyz", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["token_stats"] != nil {
		t.Error("token_stats must be null when no stats exist")
	}
}
