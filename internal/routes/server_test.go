package routes

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/theopenbee/openbee/internal/api"
	"github.com/theopenbee/openbee/internal/infra/auth"
)

type activeUsers struct{}

func (activeUsers) PasswordChangedAt(string) (int64, error) {
	return 0, nil
}

func TestServer_AdminTokenReachesOnlyPluginRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	jwt := auth.NewJWTService("s", time.Hour, time.Hour)
	s, err := NewServer(ServerParams{
		Plugins:        api.NewPluginHandler(nil),
		Workers:        &api.WorkerHandler{},
		AuthMiddleware: auth.AuthMiddleware(jwt, activeUsers{}),
		PluginsAuth: func(c *gin.Context) {
			c.AbortWithStatus(http.StatusTeapot)
		},
		StaticFS: fstest.MapFS{"dist/index.html": {Data: []byte("ok")}},
	})
	require.NoError(t, err)

	for _, path := range []string{"/api/plugins", "/api/plugins/catalog", "/api/plugins/telegram", "/api/plugins/operations/x"} {
		rec := httptest.NewRecorder()
		s.router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		assert.Equal(t, http.StatusTeapot, rec.Code, path)
	}
	rec := httptest.NewRecorder()
	s.router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/workers", nil))
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestServer_LocalChatRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	jwt := auth.NewJWTService("s", time.Hour, time.Hour)
	s, err := NewServer(ServerParams{
		LocalChat:      api.NewLocalChatHandler(api.LocalChatDeps{}),
		AuthMiddleware: auth.AuthMiddleware(jwt, activeUsers{}),
		StaticFS:       fstest.MapFS{"dist/index.html": {Data: []byte("spa")}},
	})
	require.NoError(t, err)

	for _, route := range []struct{ method, path string }{
		{http.MethodGet, "/api/local/conversations"},
		{http.MethodGet, "/api/local/conversations/worker:w1/messages"},
		{http.MethodPost, "/api/local/conversations/worker:w1/messages"},
		{http.MethodPost, "/api/local/conversations/bee/read"},
		{http.MethodPost, "/api/local/conversations/bee/clear"},
		{http.MethodPost, "/api/local/conversations/worker:w1/stop"},
		{http.MethodPost, "/api/local/media"},
		{http.MethodGet, "/api/local/media/a.png"},
		{http.MethodGet, "/api/local/stream"},
	} {
		rec := httptest.NewRecorder()
		s.router.ServeHTTP(rec, httptest.NewRequest(route.method, route.path, nil))
		assert.Equal(t, http.StatusUnauthorized, rec.Code, route.method+" "+route.path)
	}

	for _, route := range []struct{ method, path string }{
		{http.MethodGet, "/api/local/messages"},
		{http.MethodPost, "/api/local/messages"},
	} {
		rec := httptest.NewRecorder()
		s.router.ServeHTTP(rec, httptest.NewRequest(route.method, route.path, nil))
		assert.Equal(t, "spa", rec.Body.String(), route.method+" "+route.path)
	}
}
