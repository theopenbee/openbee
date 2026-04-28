package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/theopenbee/openbee/internal/domain/group"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/infra/store"
)

func newTestGroupHandler(t *testing.T) (*GroupHandler, *gin.Engine) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := store.InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	gs := store.NewGroupStore(db)
	ws := store.NewWorkerStore(db)
	ts := store.NewTaskStore(db)
	mgr := group.NewManager(t.TempDir(), gs, ws, ts, nil, nil, nil)
	h := NewGroupHandler(mgr, gs, ws)

	router := gin.New()
	g := router.Group("/api/groups")
	g.POST("", h.Create)
	g.GET("", h.List)
	g.GET("/:id", h.Get)
	g.PUT("/:id", h.Update)
	g.DELETE("/:id", h.Delete)
	g.GET("/:id/members", h.ListMembers)
	g.POST("/:id/members", h.AddMember)
	g.DELETE("/:id/members/:worker_id", h.RemoveMember)

	return h, router
}

func TestGroupHandler_CreateAndGet(t *testing.T) {
	_, router := newTestGroupHandler(t)

	body, _ := json.Marshal(map[string]any{"name": "g1"})
	req := httptest.NewRequest(http.MethodPost, "/api/groups", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("create: got %d, body=%s", rec.Code, rec.Body.String())
	}
	var created map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &created)
	id := created["id"].(string)

	req = httptest.NewRequest(http.MethodGet, "/api/groups/"+id, nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("get: got %d", rec.Code)
	}
}

func TestGroupHandler_AddRemoveMember(t *testing.T) {
	h, router := newTestGroupHandler(t)
	g, _ := h.manager.CreateGroup(group.CreateGroupParams{Name: "g"})
	w, _ := h.workerStore.Create(model.Worker{Name: "w", WorkDir: "/tmp/w"})

	body, _ := json.Marshal(map[string]any{"worker_id": w.ID})
	req := httptest.NewRequest(http.MethodPost, "/api/groups/"+g.ID+"/members", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("add: got %d, body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodDelete, "/api/groups/"+g.ID+"/members/"+w.ID, nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("remove: got %d", rec.Code)
	}
}

func TestGroupHandler_List(t *testing.T) {
	_, router := newTestGroupHandler(t)

	// Create two groups
	for _, name := range []string{"gA", "gB"} {
		body, _ := json.Marshal(map[string]any{"name": name})
		req := httptest.NewRequest(http.MethodPost, "/api/groups", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("create %s: got %d", name, rec.Code)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/api/groups", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list: got %d", rec.Code)
	}
	var list []any
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	if len(list) != 2 {
		t.Errorf("expected 2 groups, got %d", len(list))
	}
}
