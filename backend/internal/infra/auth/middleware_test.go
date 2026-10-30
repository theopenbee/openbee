package auth

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type fakeUserLoader struct {
	passwordChangedAt int64
	err               error
}

func (f fakeUserLoader) PasswordChangedAt(uid string) (int64, error) {
	return f.passwordChangedAt, f.err
}

func newTestContext(jwt *JWTService, loader PasswordChangeLoader) (*gin.Engine, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	grp := r.Group("/api")
	grp.Use(AuthMiddleware(jwt, loader))
	grp.GET("/secured", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	rec := httptest.NewRecorder()
	return r, rec
}

func TestAuthMiddleware_RejectsUnknownUser(t *testing.T) {
	jwt := NewJWTService("s", time.Hour, time.Hour)
	pair, _ := jwt.GenerateUserTokenPair("u1")
	loader := fakeUserLoader{err: errors.New("no rows")}

	r, rec := newTestContext(jwt, loader)
	req := httptest.NewRequest(http.MethodGet, "/api/secured", nil)
	req.Header.Set("Authorization", "Bearer "+pair.AccessToken)
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestAuthMiddleware_RejectsTokenIssuedBeforePasswordChange(t *testing.T) {
	jwt := NewJWTService("s", time.Hour, time.Hour)
	pair, _ := jwt.GenerateUserTokenPair("u1")

	// Password changed one hour in the future relative to the token's iat: the
	// existing token must be rejected, forcing a re-login.
	future := time.Now().Add(time.Hour).UnixMilli()
	loader := fakeUserLoader{passwordChangedAt: future}
	r, rec := newTestContext(jwt, loader)
	req := httptest.NewRequest(http.MethodGet, "/api/secured", nil)
	req.Header.Set("Authorization", "Bearer "+pair.AccessToken)
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusUnauthorized, rec.Code)

	// A password change well in the past leaves the current token valid.
	past := time.Now().Add(-time.Hour).UnixMilli()
	loaderOK := fakeUserLoader{passwordChangedAt: past}
	r2, rec2 := newTestContext(jwt, loaderOK)
	req2 := httptest.NewRequest(http.MethodGet, "/api/secured", nil)
	req2.Header.Set("Authorization", "Bearer "+pair.AccessToken)
	r2.ServeHTTP(rec2, req2)
	require.Equal(t, http.StatusOK, rec2.Code)
}
