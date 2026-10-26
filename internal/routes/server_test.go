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
