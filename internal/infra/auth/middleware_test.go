package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

type fakeUserLoader struct {
	status string
	err    error
}

func (f fakeUserLoader) UserStatus(uid string) (string, error) { return f.status, f.err }

func newTestContext(jwt *JWTService, loader UserStatusLoader, resolver *PermissionResolver, token string) (*gin.Engine, *httptest.ResponseRecorder) {
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
	if rec.Code != http.StatusForbidden && rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected disabled user rejected, got %d", rec.Code)
	}
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
	if rec.Code != http.StatusOK {
		t.Fatalf("expected users:manage to read roles (200), got %d", rec.Code)
	}

	// Neither perm -> 403.
	resolverNone := NewPermissionResolver(func(string) ([]string, error) { return []string{PermContactsRead}, nil })
	r2, rec2 := newAnyOfCtx(resolverNone)
	req2 := httptest.NewRequest(http.MethodGet, "/api/roles", nil)
	req2.Header.Set("Authorization", "Bearer "+pair.AccessToken)
	r2.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusForbidden {
		t.Fatalf("expected 403 without roles:manage or users:manage, got %d", rec2.Code)
	}
}

func TestRequirePermission_AllowsAndDenies(t *testing.T) {
	jwt := NewJWTService("s", time.Hour, time.Hour)
	pair, _ := jwt.GenerateUserTokenPair("u1")
	loader := fakeUserLoader{status: "active"}

	// has permission
	resolverYes := NewPermissionResolver(func(string) ([]string, error) { return []string{PermContactsRead}, nil })
	r, rec := newTestContext(jwt, loader, resolverYes, pair.AccessToken)
	req := httptest.NewRequest(http.MethodGet, "/api/secured", nil)
	req.Header.Set("Authorization", "Bearer "+pair.AccessToken)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	// lacks permission
	resolverNo := NewPermissionResolver(func(string) ([]string, error) { return []string{PermTasksRead}, nil })
	r2, rec2 := newTestContext(jwt, loader, resolverNo, pair.AccessToken)
	req2 := httptest.NewRequest(http.MethodGet, "/api/secured", nil)
	req2.Header.Set("Authorization", "Bearer "+pair.AccessToken)
	r2.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec2.Code)
	}
}
