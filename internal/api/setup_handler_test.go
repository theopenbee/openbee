package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/theopenbee/openbee/internal/api"
	"github.com/theopenbee/openbee/internal/infra/auth"
	"github.com/theopenbee/openbee/internal/infra/store"
)

func newSetupServer(t *testing.T) (*gin.Engine, *store.UserStore) {
	t.Helper()
	db, err := store.InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	us := store.NewUserStore(db)
	jwtSvc := auth.NewJWTService("secret", time.Hour, time.Hour)
	h := api.NewSetupHandler(us, jwtSvc)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/api/setup/status", h.Status)
	r.POST("/api/setup", h.Create)
	return r, us
}

func TestSetup_StatusFalseThenTrue(t *testing.T) {
	r, _ := newSetupServer(t)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/setup/status", nil))
	var resp map[string]bool
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["initialized"] {
		t.Fatal("expected uninitialized")
	}

	body, _ := json.Marshal(map[string]string{"username": "root", "password": "rootpw"})
	req := httptest.NewRequest(http.MethodPost, "/api/setup", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec2 := httptest.NewRecorder()
	r.ServeHTTP(rec2, req)
	if rec2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (%s)", rec2.Code, rec2.Body.String())
	}

	rec3 := httptest.NewRecorder()
	r.ServeHTTP(rec3, httptest.NewRequest(http.MethodGet, "/api/setup/status", nil))
	_ = json.Unmarshal(rec3.Body.Bytes(), &resp)
	if !resp["initialized"] {
		t.Fatal("expected initialized after create")
	}
}

func TestSetup_SecondCreateRejected(t *testing.T) {
	r, _ := newSetupServer(t)
	body, _ := json.Marshal(map[string]string{"username": "root", "password": "rootpw"})
	// first create succeeds
	req0 := httptest.NewRequest(http.MethodPost, "/api/setup", bytes.NewReader(body))
	req0.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(httptest.NewRecorder(), req0)
	// second attempt
	req := httptest.NewRequest(http.MethodPost, "/api/setup", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 on re-init, got %d", rec.Code)
	}
}
