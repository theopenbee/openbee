package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type fakeUserLoader struct {
	status            string
	passwordChangedAt int64
	err               error
}

func (f fakeUserLoader) UserAuthState(uid string) (string, int64, error) {
	return f.status, f.passwordChangedAt, f.err
}

func newTestContext(jwt *JWTService, loader UserAuthStateLoader, resolver *PermissionResolver, token string) (*gin.Engine, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	grp := r.Group("/api")
	grp.Use(AuthMiddleware(jwt, loader))
	grp.GET("/secured", RequirePermission(resolver, PermContactsRead), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	rec := httptest.NewRecorder()
	return r, rec
}

func TestAuthMiddleware_RejectsDisabledUser(t *testing.T) {
	jwt := NewJWTService("s", time.Hour, time.Hour)
	pair, _ := jwt.GenerateUserTokenPair("u1")
	loader := fakeUserLoader{status: "disabled"}
	resolver := NewPermissionResolver(func(string) ([]string, error) { return []string{"*"}, nil })

	r, rec := newTestContext(jwt, loader, resolver, pair.AccessToken)
	req := httptest.NewRequest(http.MethodGet, "/api/secured", nil)
	req.Header.Set("Authorization", "Bearer "+pair.AccessToken)
	r.ServeHTTP(rec, req)
	require.Contains(t, []int{http.StatusForbidden, http.StatusUnauthorized}, rec.Code)
}

func TestAuthMiddleware_RejectsTokenIssuedBeforePasswordChange(t *testing.T) {
	jwt := NewJWTService("s", time.Hour, time.Hour)
	pair, _ := jwt.GenerateUserTokenPair("u1")
	resolver := NewPermissionResolver(func(string) ([]string, error) { return []string{"*"}, nil })

	// Password changed one hour in the future relative to the token's iat: the
	// existing token must be rejected, forcing a re-login.
	future := time.Now().Add(time.Hour).UnixMilli()
	loader := fakeUserLoader{status: "active", passwordChangedAt: future}
	r, rec := newTestContext(jwt, loader, resolver, pair.AccessToken)
	req := httptest.NewRequest(http.MethodGet, "/api/secured", nil)
	req.Header.Set("Authorization", "Bearer "+pair.AccessToken)
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusUnauthorized, rec.Code)

	// A password change well in the past leaves the current token valid.
	past := time.Now().Add(-time.Hour).UnixMilli()
	loaderOK := fakeUserLoader{status: "active", passwordChangedAt: past}
	r2, rec2 := newTestContext(jwt, loaderOK, resolver, pair.AccessToken)
	req2 := httptest.NewRequest(http.MethodGet, "/api/secured", nil)
	req2.Header.Set("Authorization", "Bearer "+pair.AccessToken)
	r2.ServeHTTP(rec2, req2)
	require.Equal(t, http.StatusOK, rec2.Code)
}

func TestRequirePermission_AnyOf(t *testing.T) {
	jwt := NewJWTService("s", time.Hour, time.Hour)
	pair, _ := jwt.GenerateUserTokenPair("u1")
	loader := fakeUserLoader{status: "active"}

	newAnyOfCtx := func(resolver *PermissionResolver) (*gin.Engine, *httptest.ResponseRecorder) {
		gin.SetMode(gin.TestMode)
		r := gin.New()
		grp := r.Group("/api")
		grp.Use(AuthMiddleware(jwt, loader))
		grp.GET("/roles", RequirePermission(resolver, PermRolesManage, PermUsersManage), func(c *gin.Context) {
			c.Status(http.StatusOK)
		})
		return r, httptest.NewRecorder()
	}

	// Only users:manage -> still allowed to read the role list (any-of).
	resolverUsers := NewPermissionResolver(func(string) ([]string, error) { return []string{PermUsersManage}, nil })
	r, rec := newAnyOfCtx(resolverUsers)
	req := httptest.NewRequest(http.MethodGet, "/api/roles", nil)
	req.Header.Set("Authorization", "Bearer "+pair.AccessToken)
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	// Neither perm -> 403.
	resolverNone := NewPermissionResolver(func(string) ([]string, error) { return []string{PermContactsRead}, nil })
	r2, rec2 := newAnyOfCtx(resolverNone)
	req2 := httptest.NewRequest(http.MethodGet, "/api/roles", nil)
	req2.Header.Set("Authorization", "Bearer "+pair.AccessToken)
	r2.ServeHTTP(rec2, req2)
	require.Equal(t, http.StatusForbidden, rec2.Code)
}
