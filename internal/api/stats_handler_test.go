package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/theopenbee/openbee/internal/infra/store"
)

func newTestServerWithStats(t *testing.T) (*gin.Engine, *store.StatsStore, func()) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	db, err := store.InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}

	ss := store.NewStatsStore(db)

	h := NewStatsHandler(ss)
	router := gin.New()
	api := router.Group("/api")
	api.GET("/stats/overview", h.GetOverview)
	api.GET("/stats/trend", h.GetTrend)
	api.GET("/stats/execution-duration-trend", h.GetExecutionDurationTrend)
	api.GET("/stats/token-trend", h.GetTokenTrend)

	return router, ss, func() { db.Close() }
}

func TestGetStatsOverview_ReturnsOK(t *testing.T) {
	router, _, cleanup := newTestServerWithStats(t)
	defer cleanup()

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/stats/overview", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if _, ok := resp["workers"]; !ok {
		t.Error("response missing 'workers' field")
	}
	if _, ok := resp["executions_today"]; !ok {
		t.Error("response missing 'executions_today' field")
	}
}

func TestGetStatsTrend_ValidDays(t *testing.T) {
	router, _, cleanup := newTestServerWithStats(t)
	defer cleanup()

	for _, days := range []int{7, 15, 30} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/stats/trend?days="+strconv.Itoa(days), nil)
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("days=%d: expected 200, got %d: %s", days, w.Code, w.Body.String())
		}

		var resp struct {
			Days int              `json:"days"`
			Data []map[string]any `json:"data"`
		}
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("days=%d: decode: %v", days, err)
		}
		if len(resp.Data) != days {
			t.Errorf("days=%d: want %d points, got %d", days, days, len(resp.Data))
		}
	}
}

func TestGetStatsTrend_InvalidDays_Returns400(t *testing.T) {
	router, _, cleanup := newTestServerWithStats(t)
	defer cleanup()

	for _, bad := range []string{"99", "0", "abc", "-1"} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/stats/trend?days="+bad, nil)
		router.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("days=%q: expected 400, got %d", bad, w.Code)
		}
	}
}

func TestGetExecutionDurationTrend_ValidDays(t *testing.T) {
	router, _, cleanup := newTestServerWithStats(t)
	defer cleanup()

	for _, days := range []int{7, 15, 30} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/stats/execution-duration-trend?days="+strconv.Itoa(days), nil)
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("days=%d: expected 200, got %d: %s", days, w.Code, w.Body.String())
		}

		var resp struct {
			Days int              `json:"days"`
			Data []map[string]any `json:"data"`
		}
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("days=%d: decode: %v", days, err)
		}
		if len(resp.Data) != days {
			t.Errorf("days=%d: want %d points, got %d", days, days, len(resp.Data))
		}
		// Each point must have total_duration_ms
		for _, pt := range resp.Data {
			if _, ok := pt["total_duration_ms"]; !ok {
				t.Errorf("days=%d: point missing total_duration_ms: %v", days, pt)
			}
		}
	}
}

func TestGetExecutionDurationTrend_InvalidDays_Returns400(t *testing.T) {
	router, _, cleanup := newTestServerWithStats(t)
	defer cleanup()

	for _, bad := range []string{"99", "0", "abc", "-1"} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/stats/execution-duration-trend?days="+bad, nil)
		router.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("days=%q: expected 400, got %d", bad, w.Code)
		}
	}
}

func TestGetTokenTrend_ValidDays(t *testing.T) {
	router, _, cleanup := newTestServerWithStats(t)
	defer cleanup()

	for _, days := range []int{7, 15, 30} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/stats/token-trend?days="+strconv.Itoa(days), nil)
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("days=%d: expected 200, got %d: %s", days, w.Code, w.Body.String())
		}

		var resp struct {
			Days int              `json:"days"`
			Data []map[string]any `json:"data"`
		}
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("days=%d: decode: %v", days, err)
		}
		if len(resp.Data) != days {
			t.Errorf("days=%d: want %d points, got %d", days, days, len(resp.Data))
		}
		for _, pt := range resp.Data {
			if _, ok := pt["total_tokens"]; !ok {
				t.Errorf("days=%d: point missing total_tokens: %v", days, pt)
			}
		}
	}
}

func TestGetTokenTrend_InvalidDays_Returns400(t *testing.T) {
	router, _, cleanup := newTestServerWithStats(t)
	defer cleanup()

	for _, bad := range []string{"99", "0", "abc", "-1"} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/stats/token-trend?days="+bad, nil)
		router.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("days=%q: expected 400, got %d", bad, w.Code)
		}
	}
}
