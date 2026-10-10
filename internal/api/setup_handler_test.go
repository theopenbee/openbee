package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/theopenbee/openbee/internal/api"
	"github.com/theopenbee/openbee/internal/infra/auth"
	"github.com/theopenbee/openbee/internal/infra/store"
)

func newSetupServer(t *testing.T) (*gin.Engine, *store.UserStore) {
	t.Helper()
	return newSetupServerWithClaim(t, func(context.Context, string) error { return nil })
}

func newSetupServerWithClaim(t *testing.T, claimLegacyChat func(context.Context, string) error) (*gin.Engine, *store.UserStore) {
	t.Helper()
	db, err := store.InitDB(t.TempDir() + "/test.db")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	us := store.NewUserStore(db)
	jwtSvc := auth.NewJWTService("secret", time.Hour, time.Hour)
	h := api.NewSetupHandler(us, jwtSvc, claimLegacyChat)
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
	require.False(t, resp["initialized"], "expected uninitialized")

	body, _ := json.Marshal(map[string]string{"username": "root", "password": "rootpw"})
	req := httptest.NewRequest(http.MethodPost, "/api/setup", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec2 := httptest.NewRecorder()
	r.ServeHTTP(rec2, req)
	require.Equal(t, http.StatusOK, rec2.Code, rec2.Body.String())

	rec3 := httptest.NewRecorder()
	r.ServeHTTP(rec3, httptest.NewRequest(http.MethodGet, "/api/setup/status", nil))
	_ = json.Unmarshal(rec3.Body.Bytes(), &resp)
	require.True(t, resp["initialized"], "expected initialized after create")
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
	require.Equal(t, http.StatusConflict, rec.Code)
}

func TestSetup_ClaimsLegacyChatForFirstAdmin(t *testing.T) {
	var claimed []string
	r, us := newSetupServerWithClaim(t, func(_ context.Context, userID string) error {
		claimed = append(claimed, userID)
		return errors.New("claim failed")
	})

	body, _ := json.Marshal(map[string]string{"username": "root", "password": "rootpw"})
	req := httptest.NewRequest(http.MethodPost, "/api/setup", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	ownerID, ok, err := us.FirstActiveSuperAdminID()
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, []string{ownerID}, claimed)
}
