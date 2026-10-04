package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/theopenbee/openbee/internal/api"
	"github.com/theopenbee/openbee/internal/infra/auth"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/infra/store"
)

func newUserServer(t *testing.T) (*gin.Engine, *store.UserStore) {
	t.Helper()
	db, err := store.InitDB(t.TempDir() + "/test.db")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	us := store.NewUserStore(db)
	resolver := auth.NewPermissionResolver(us.PermissionsForUser)
	h := api.NewUserHandler(us, resolver)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/api/users", h.List)
	r.POST("/api/users", h.Create)
	r.PUT("/api/users/:id/profile", h.UpdateProfile)
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
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())

	rec2 := httptest.NewRecorder()
	r.ServeHTTP(rec2, httptest.NewRequest(http.MethodGet, "/api/users", nil))
	var users []model.UserWithRoles
	_ = json.Unmarshal(rec2.Body.Bytes(), &users)
	require.Len(t, users, 1, "unexpected users: %+v", users)
	require.Equal(t, "bob", users[0].Username, "unexpected users: %+v", users)
}

func TestUserHandler_UpdateProfile(t *testing.T) {
	r, us := newUserServer(t)
	u, _ := us.Create("alice", "alicepw", "Alice", "", nil)

	body, _ := json.Marshal(map[string]any{"username": "alice2", "display_name": "Alice Two"})
	req := httptest.NewRequest(http.MethodPut, "/api/users/"+u.ID+"/profile", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
	got, _ := us.GetByID(u.ID)
	require.Equal(t, "alice2", got.Username, "profile not updated: %+v", got.User)
	require.Equal(t, "Alice Two", got.DisplayName, "profile not updated: %+v", got.User)
}

func TestUserHandler_UpdateProfile_UsernameTaken(t *testing.T) {
	r, us := newUserServer(t)
	_, _ = us.Create("taken", "pw1234", "Taken", "", nil)
	u, _ := us.Create("free", "pw1234", "Free", "", nil)

	body, _ := json.Marshal(map[string]any{"username": "taken"})
	req := httptest.NewRequest(http.MethodPut, "/api/users/"+u.ID+"/profile", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
	var resp struct {
		Code string `json:"code"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	require.Equal(t, "username_taken", resp.Code)
}

func TestUserHandler_UpdateProfile_BlankUsername(t *testing.T) {
	r, us := newUserServer(t)
	u, _ := us.Create("blanktest", "pw1234", "Blank", "", nil)

	body, _ := json.Marshal(map[string]any{"username": "   "})
	req := httptest.NewRequest(http.MethodPut, "/api/users/"+u.ID+"/profile", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusBadRequest, rec.Code, "expected 400 for blank username")
}

func TestUserHandler_CannotDeleteLastSuperAdmin(t *testing.T) {
	r, us := newUserServer(t)
	su, _ := us.Create("root", "rootpw", "Root", "", []string{model.RoleIDSuperAdmin})
	req := httptest.NewRequest(http.MethodDelete, "/api/users/"+su.ID, nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusBadRequest, rec.Code, "expected 400 deleting last super-admin")
	var body struct {
		Code  string `json:"code"`
		Error string `json:"error"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, "last_super_admin", body.Code)
	require.NotEmpty(t, body.Error, "expected non-empty fallback error message")
}
