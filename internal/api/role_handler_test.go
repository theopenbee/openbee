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

func newRoleServer(t *testing.T) (*gin.Engine, *store.RoleStore) {
	t.Helper()
	db, err := store.InitDB(t.TempDir() + "/test.db")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	rs := store.NewRoleStore(db)
	resolver := auth.NewPermissionResolver(func(string) ([]string, error) { return nil, nil })
	h := api.NewRoleHandler(rs, resolver)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/api/permissions", h.Catalog)
	r.GET("/api/roles", h.List)
	r.POST("/api/roles", h.Create)
	r.PUT("/api/roles/:id", h.Update)
	r.DELETE("/api/roles/:id", h.Delete)
	return r, rs
}

func TestRoleHandler_Catalog(t *testing.T) {
	r, _ := newRoleServer(t)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/permissions", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	var groups []auth.PermissionGroup
	_ = json.Unmarshal(rec.Body.Bytes(), &groups)
	require.NotEmpty(t, groups)
}

func TestRoleHandler_CreateAndDelete(t *testing.T) {
	r, _ := newRoleServer(t)
	body, _ := json.Marshal(map[string]any{
		"name": "ops", "permissions": []string{"contacts:read"},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/roles", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var role model.RoleWithPermissions
	_ = json.Unmarshal(rec.Body.Bytes(), &role)

	del := httptest.NewRequest(http.MethodDelete, "/api/roles/"+role.ID, nil)
	recDel := httptest.NewRecorder()
	r.ServeHTTP(recDel, del)
	require.Equal(t, http.StatusNoContent, recDel.Code)
}

func TestRoleHandler_CannotDeleteSystemRole(t *testing.T) {
	r, _ := newRoleServer(t)
	del := httptest.NewRequest(http.MethodDelete, "/api/roles/"+model.RoleIDSuperAdmin, nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, del)
	require.Equal(t, http.StatusBadRequest, rec.Code)
}
