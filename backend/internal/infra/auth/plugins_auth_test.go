package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testRPCSecret = "rpc-secret"

func pluginsRouter(jwt *JWTService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	loader := fakeUserLoader{}
	r := gin.New()
	r.Group("/api/plugins", PluginsAuth(jwt, loader, testRPCSecret)).
		POST("/install", func(c *gin.Context) { c.Status(http.StatusOK) })
	api := r.Group("/api", AuthMiddleware(jwt, loader))
	api.GET("/workers", func(c *gin.Context) { c.Status(http.StatusOK) })
	return r
}

func send(r *gin.Engine, method, path string, header map[string]string) int {
	req := httptest.NewRequest(method, path, nil)
	for k, v := range header {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec.Code
}

func TestPluginsAuth_AdminTokenAllowed(t *testing.T) {
	jwt := NewJWTService("s", time.Hour, time.Hour)
	tok, err := GenerateAdminToken(testRPCSecret, time.Minute)
	require.NoError(t, err)
	r := pluginsRouter(jwt)
	assert.Equal(t, http.StatusOK, send(r, http.MethodPost, "/api/plugins/install", map[string]string{"X-API-Key": tok}))
}

func TestPluginsAuth_RejectsBeeAndWorkerTokens(t *testing.T) {
	jwt := NewJWTService("s", time.Hour, time.Hour)
	r := pluginsRouter(jwt)
	bee, err := GenerateBeeToken(testRPCSecret, time.Minute)
	require.NoError(t, err)
	worker, err := GenerateWorkerToken(testRPCSecret, "w1", nil, time.Minute)
	require.NoError(t, err)
	for _, tok := range []string{bee, worker} {
		code := send(r, http.MethodPost, "/api/plugins/install", map[string]string{"X-API-Key": tok})
		assert.Contains(t, []int{http.StatusUnauthorized, http.StatusForbidden}, code)
	}
}

func TestPluginsAuth_RejectsBadAndExpiredTokens(t *testing.T) {
	jwt := NewJWTService("s", time.Hour, time.Hour)
	r := pluginsRouter(jwt)
	other, err := GenerateAdminToken("other-secret", time.Minute)
	require.NoError(t, err)
	expired, err := GenerateAdminToken(testRPCSecret, -time.Minute)
	require.NoError(t, err)
	for _, tok := range []string{"garbage", other, expired} {
		assert.Equal(t, http.StatusUnauthorized, send(r, http.MethodPost, "/api/plugins/install", map[string]string{"X-API-Key": tok}))
	}
	assert.Equal(t, http.StatusUnauthorized, send(r, http.MethodPost, "/api/plugins/install", nil))
}

func TestPluginsAuth_UserTokenAllowed(t *testing.T) {
	jwt := NewJWTService("s", time.Hour, time.Hour)
	pair, err := jwt.GenerateUserTokenPair("u1")
	require.NoError(t, err)
	bearer := map[string]string{"Authorization": "Bearer " + pair.AccessToken}
	assert.Equal(t, http.StatusOK, send(pluginsRouter(jwt), http.MethodPost, "/api/plugins/install", bearer))
}

func TestAdminTokenCannotReachOtherAPIRoutes(t *testing.T) {
	jwt := NewJWTService("s", time.Hour, time.Hour)
	tok, err := GenerateAdminToken(testRPCSecret, time.Minute)
	require.NoError(t, err)
	r := pluginsRouter(jwt)
	assert.Equal(t, http.StatusUnauthorized, send(r, http.MethodGet, "/api/workers", map[string]string{"X-API-Key": tok}))
	assert.Equal(t, http.StatusUnauthorized, send(r, http.MethodGet, "/api/workers", map[string]string{"Authorization": "Bearer " + tok}))
}
