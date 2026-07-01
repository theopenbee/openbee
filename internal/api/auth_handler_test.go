package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/theopenbee/openbee/internal/infra/auth"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/infra/store"
)

func newAuthTestServer(t *testing.T, maxAttempts int) (*gin.Engine, *store.UserStore, *auth.JWTService) {
	t.Helper()
	db, err := store.InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	us := store.NewUserStore(db)
	if _, err := us.Create("alice", "s3cret", "Alice", "", []string{model.RoleIDSuperAdmin}); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	jwtSvc := auth.NewJWTService("secret", time.Hour, 24*time.Hour)
	rl := auth.NewLoginRateLimiter(maxAttempts, time.Minute)
	resolver := auth.NewPermissionResolver(us.PermissionsForUser)
	h := auth.NewAuthHandler(us, jwtSvc, rl, resolver)

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
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	var pair auth.TokenPair
	_ = json.Unmarshal(rec.Body.Bytes(), &pair)
	if pair.AccessToken == "" {
		t.Fatal("expected access token")
	}
}

func TestAuthHandler_LoginBadPassword(t *testing.T) {
	r, _, _ := newAuthTestServer(t, 50)
	if code := login(t, r, "alice", "nope"); code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", code)
	}
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
		if code := login(t, r, "alice", "s3cret"); code != http.StatusOK {
			t.Fatalf("successful login %d: expected 200, got %d", i+1, code)
		}
	}
}

// Repeated failed logins must eventually be blocked with 429.
func TestAuthHandler_RepeatedFailuresRateLimited(t *testing.T) {
	r, _, _ := newAuthTestServer(t, 3)
	for i := 0; i < 3; i++ {
		if code := login(t, r, "alice", "wrong"); code != http.StatusUnauthorized {
			t.Fatalf("failed login %d: expected 401, got %d", i+1, code)
		}
	}
	if code := login(t, r, "alice", "wrong"); code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 after exhausting attempts, got %d", code)
	}
}
