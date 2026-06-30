package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/theopenbee/openbee/internal/api"
	"github.com/theopenbee/openbee/internal/infra/auth"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/infra/store"
)

func newUserServer(t *testing.T) (*gin.Engine, *store.UserStore) {
	t.Helper()
	db, err := store.InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	us := store.NewUserStore(db)
	resolver := auth.NewPermissionResolver(us.PermissionsForUser)
	h := api.NewUserHandler(us, resolver)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/api/users", h.List)
	r.POST("/api/users", h.Create)
	r.PUT("/api/users/:id/roles", h.SetRoles)
	r.PUT("/api/users/:id/status", h.SetStatus)
	r.POST("/api/users/:id/password", h.ResetPassword)
	r.DELETE("/api/users/:id", h.Delete)
	return r, us
}

func TestUserHandler_CreateAndList(t *testing.T) {
	r, _ := newUserServer(t)
	body, _ := json.Marshal(map[string]any{
		"username": "bob", "password": "bobpw1", "display_name": "Bob",
		"role_ids": []string{model.RoleIDSuperAdmin},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/users", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d (%s)", rec.Code, rec.Body.String())
	}

	rec2 := httptest.NewRecorder()
	r.ServeHTTP(rec2, httptest.NewRequest(http.MethodGet, "/api/users", nil))
	var users []model.UserWithRoles
	_ = json.Unmarshal(rec2.Body.Bytes(), &users)
	if len(users) != 1 || users[0].Username != "bob" {
		t.Fatalf("unexpected users: %+v", users)
	}
}

func TestUserHandler_CannotDeleteLastSuperAdmin(t *testing.T) {
	r, us := newUserServer(t)
	su, _ := us.Create("root", "rootpw", "Root", "", []string{model.RoleIDSuperAdmin})
	req := httptest.NewRequest(http.MethodDelete, "/api/users/"+su.ID, nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 deleting last super-admin, got %d", rec.Code)
	}
}
