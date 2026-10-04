package api

import (
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

func newTestServer(t *testing.T, register func(*gin.RouterGroup, *ExecutionHandler)) (*gin.Engine, *store.ExecutionStore, *store.TokenStatsStore) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := store.InitDB(t.TempDir() + "/test.db")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	es := store.NewExecutionStore(db, t.TempDir())
	ts := store.NewTokenStatsStore(db)
	h := NewExecutionHandler(es, ts)
	router := gin.New()
	api := router.Group("/api")
	register(api, h)
	return router, es, ts
}

func newTestServerWithExecutions(t *testing.T) (*gin.Engine, *store.ExecutionStore, *store.TokenStatsStore) {
	return newTestServer(t, func(api *gin.RouterGroup, h *ExecutionHandler) {
		api.GET("/sessions", h.List)
	})
}

// TestTokenStatsIncluded merges the ExecutionsList and GetSession variants of
// "response includes token stats": both seed the same execution + token
// stats row and differ only in which endpoint is hit and where the expected
// key lives in the decoded response.
func TestTokenStatsIncluded(t *testing.T) {
	cases := []struct {
		name    string
		path    string
		extract func(stats map[string]any) any
	}{
		{
			name: "ExecutionsList",
			path: "/api/sessions",
			extract: func(stats map[string]any) any {
				return stats["session-abc"]
			},
		},
		{
			name: "GetSession",
			path: "/api/sessions/session-abc",
			extract: func(stats map[string]any) any {
				return stats["total_tokens"]
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			router, es, ts := newTestServer(t, func(api *gin.RouterGroup, h *ExecutionHandler) {
				api.GET("/sessions", h.List)
				api.GET("/sessions/:id", h.GetSession)
			})

			_, err := es.Create(store.ExecutionCreate{WorkerID: "worker-1", TriggerInput: "hello", SessionID: "session-abc", Engine: "claude"})
			require.NoError(t, err)
			err = ts.Upsert(model.TokenStats{
				SessionID:    "session-abc",
				AgentType:    "claude",
				Model:        "claude-sonnet-4-6",
				InputTokens:  100,
				OutputTokens: 200,
				TotalTokens:  300,
				SyncedAt:     time.Now().UnixMilli(),
			})
			require.NoError(t, err)

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			router.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code, w.Body.String())

			var resp map[string]any
			require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
			statsRaw, ok := resp["token_stats"]
			require.True(t, ok, "expected token_stats field in response")
			statsMap, ok := statsRaw.(map[string]any)
			require.True(t, ok, "token_stats must be a map, got %T", statsRaw)

			assert.NotNil(t, tc.extract(statsMap))
		})
	}
}

func TestExecutionsList_NoTokenStats_WhenNoneExist(t *testing.T) {
	router, es, _ := newTestServerWithExecutions(t)

	_, err := es.Create(store.ExecutionCreate{WorkerID: "worker-1", TriggerInput: "hello", SessionID: "session-xyz", Engine: "claude"})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/sessions", nil)
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	ts, ok := resp["token_stats"].(map[string]any)
	if ok {
		_, found := ts["session-xyz"]
		assert.False(t, found, "session-xyz must not appear in token_stats when no stats were upserted")
	}
}

func newTestServerWithSessions(t *testing.T) (*gin.Engine, *store.ExecutionStore, *store.TokenStatsStore) {
	return newTestServer(t, func(api *gin.RouterGroup, h *ExecutionHandler) {
		api.GET("/sessions/:id", h.GetSession)
	})
}

func TestGetSession_NullTokenStats_WhenNoneExist(t *testing.T) {
	router, es, _ := newTestServerWithSessions(t)

	_, err := es.Create(store.ExecutionCreate{WorkerID: "worker-1", TriggerInput: "hello", SessionID: "session-xyz", Engine: "claude"})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/sessions/session-xyz", nil)
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	assert.Nil(t, resp["token_stats"], "token_stats must be null when no stats exist")
}
