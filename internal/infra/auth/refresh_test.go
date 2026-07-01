package auth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/theopenbee/openbee/internal/infra/model"
)

type fakeAuthenticator struct {
	user model.UserWithRoles
	err  error
}

func (f fakeAuthenticator) Authenticate(string, string) (model.UserWithRoles, error) {
	return f.user, f.err
}
func (f fakeAuthenticator) GetByID(string) (model.UserWithRoles, error) { return f.user, f.err }
func (f fakeAuthenticator) SetPassword(string, string) error           { return nil }

func serveRefresh(h *AuthHandler, refreshToken string) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/auth/refresh", h.Refresh)
	rec := httptest.NewRecorder()
	body := `{"refresh_token":"` + refreshToken + `"}`
	req := httptest.NewRequest(http.MethodPost, "/auth/refresh", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	return rec
}

func TestRefresh_RejectsRefreshTokenIssuedBeforePasswordChange(t *testing.T) {
	jwt := NewJWTService("s", time.Hour, time.Hour)
	pair, _ := jwt.GenerateUserTokenPair("u1")

	// Password changed after the refresh token was minted -> refresh must fail so
	// the stale session cannot keep minting access tokens.
	changed := model.UserWithRoles{User: model.User{ID: "u1", PasswordChangedAt: time.Now().Add(time.Hour).UnixMilli()}}
	h := NewAuthHandler(fakeAuthenticator{user: changed}, jwt, nil, nil)
	if rec := serveRefresh(h, pair.RefreshToken); rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for refresh token issued before password change, got %d", rec.Code)
	}

	// Password change in the past -> the refresh token still works.
	ok := model.UserWithRoles{User: model.User{ID: "u1", PasswordChangedAt: time.Now().Add(-time.Hour).UnixMilli()}}
	h2 := NewAuthHandler(fakeAuthenticator{user: ok}, jwt, nil, nil)
	if rec := serveRefresh(h2, pair.RefreshToken); rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for valid refresh token, got %d", rec.Code)
	}
}
