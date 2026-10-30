package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/theopenbee/openbee/internal/infra/auth"
	"github.com/theopenbee/openbee/internal/infra/store"
)

func newAuthTestServer(t *testing.T, maxAttempts int) (*gin.Engine, *store.UserStore, *auth.JWTService) {
	t.Helper()
	db, err := store.InitDB(t.TempDir() + "/test.db")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	us := store.NewUserStore(db)
	_, err = us.CreateFirst("alice", "s3cret", "Alice")
	require.NoError(t, err)
	jwtSvc := auth.NewJWTService("secret", time.Hour, 24*time.Hour)
	rl := auth.NewLoginRateLimiter(maxAttempts, time.Minute)
	h := auth.NewAuthHandler(us, jwtSvc, rl)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/api/auth/login", h.Login)
	return r, us, jwtSvc
}

func TestAuthHandler_LoginSuccess(t *testing.T) {
	r, _, _ := newAuthTestServer(t, 50)
	body, _ := json.Marshal(map[string]string{"username": "alice", "password": "s3cret"})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var pair auth.TokenPair
	_ = json.Unmarshal(rec.Body.Bytes(), &pair)
	require.NotEmpty(t, pair.AccessToken)
}

func TestAuthHandler_LoginBadPassword(t *testing.T) {
	r, _, _ := newAuthTestServer(t, 50)
	require.Equal(t, http.StatusUnauthorized, login(t, r, "alice", "nope"))
}

func login(t *testing.T, r *gin.Engine, username, password string) int {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"username": username, "password": password})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec.Code
}

// A successful login must clear the rate-limit budget, so repeated successful
// logins from the same client never trip the limiter.
func TestAuthHandler_SuccessfulLoginsNeverRateLimited(t *testing.T) {
	r, _, _ := newAuthTestServer(t, 3)
	for i := 0; i < 10; i++ {
		require.Equal(t, http.StatusOK, login(t, r, "alice", "s3cret"), "successful login %d", i+1)
	}
}

// Repeated failed logins must eventually be blocked with 429.
func TestAuthHandler_RepeatedFailuresRateLimited(t *testing.T) {
	r, _, _ := newAuthTestServer(t, 3)
	for i := 0; i < 3; i++ {
		require.Equal(t, http.StatusUnauthorized, login(t, r, "alice", "wrong"), "failed login %d", i+1)
	}
	require.Equal(t, http.StatusTooManyRequests, login(t, r, "alice", "wrong"), "expected 429 after exhausting attempts")
}

func TestAuthHandler_MeReturnsUserWithoutRoles(t *testing.T) {
	db, err := store.InitDB(t.TempDir() + "/test.db")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	us := store.NewUserStore(db)
	u, err := us.CreateFirst("alice", "s3cret", "Alice")
	require.NoError(t, err)
	jwtSvc := auth.NewJWTService("secret", time.Hour, 24*time.Hour)
	h := auth.NewAuthHandler(us, jwtSvc, auth.NewLoginRateLimiter(5, time.Minute))

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/api/me", auth.AuthMiddleware(jwtSvc, us), h.Me)

	pair, err := jwtSvc.GenerateUserTokenPair(u.ID)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	req.Header.Set("Authorization", "Bearer "+pair.AccessToken)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, u.ID, body["id"])
	require.Equal(t, "alice", body["username"])
	require.Equal(t, "Alice", body["display_name"])
	for _, key := range []string{"roles", "permissions", "status", "created_by", "password_hash"} {
		require.NotContains(t, body, key)
	}
}
