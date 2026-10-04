package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/theopenbee/openbee/internal/infra/store"
)

func newTestServerWithStats(t *testing.T) (*gin.Engine, *store.StatsStore) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	db, err := store.InitDB(t.TempDir() + "/test.db")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	ss := store.NewStatsStore(db)

	h := NewStatsHandler(ss)
	router := gin.New()
	api := router.Group("/api")
	api.GET("/stats/overview", h.GetOverview)
	api.GET("/stats/token-trend", h.GetTokenTrend)

	return router, ss
}

func TestGetStatsOverview_ReturnsOK(t *testing.T) {
	router, _ := newTestServerWithStats(t)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/stats/overview", nil)
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))

	_, ok := resp["workers"]
	assert.True(t, ok, "response missing 'workers' field")
	_, ok = resp["tokens_today_total"]
	assert.True(t, ok, "response missing 'tokens_today_total' field")
}

func TestGetTokenTrend_ValidDays(t *testing.T) {
	router, _ := newTestServerWithStats(t)

	for _, days := range []int{7, 15, 30} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/stats/token-trend?days="+strconv.Itoa(days), nil)
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code, "days=%d: %s", days, w.Body.String())

		var resp struct {
			Days int              `json:"days"`
			Data []map[string]any `json:"data"`
		}
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp), "days=%d", days)
		assert.Len(t, resp.Data, days, "days=%d", days)
		for _, pt := range resp.Data {
			_, ok := pt["total_tokens"]
			assert.True(t, ok, "days=%d: point missing total_tokens: %v", days, pt)
		}
	}
}

func TestGetTokenTrend_InvalidDays_Returns400(t *testing.T) {
	router, _ := newTestServerWithStats(t)

	for _, bad := range []string{"99", "0", "abc", "-1"} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/stats/token-trend?days="+bad, nil)
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code, "days=%q", bad)
	}
}
