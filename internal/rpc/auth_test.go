package rpc_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/theopenbee/openbee/internal/infra/auth"
	"github.com/theopenbee/openbee/internal/rpc"
)

func init() {
	gin.SetMode(gin.TestMode)
}

const testSecret = "test-secret-xyz"

func newRouter(secret string, extra ...gin.HandlerFunc) *gin.Engine {
	r := gin.New()
	r.Use(append([]gin.HandlerFunc{rpc.JWTAuthMiddleware(secret)}, extra...)...)
	r.GET("/test", func(c *gin.Context) { c.Status(http.StatusOK) })
	return r
}

func TestJWTAuthMiddleware(t *testing.T) {
	cases := []struct {
		name     string
		token    func(t *testing.T) string
		via      string
		extra    []gin.HandlerFunc
		wantCode int
	}{
		{
			name:     "NoToken",
			token:    func(t *testing.T) string { return "" },
			via:      "header",
			wantCode: http.StatusUnauthorized,
		},
		{
			name:     "InvalidToken",
			token:    func(t *testing.T) string { return "not-a-jwt" },
			via:      "header",
			wantCode: http.StatusUnauthorized,
		},
		{
			name: "ValidBeeToken",
			token: func(t *testing.T) string {
				tok, err := auth.GenerateBeeToken(testSecret, time.Hour)
				require.NoError(t, err)
				return tok
			},
			via:      "header",
			wantCode: http.StatusOK,
		},
		{
			name: "ValidWorkerToken",
			token: func(t *testing.T) string {
				tok, err := auth.GenerateWorkerToken(testSecret, "wid-1", nil, time.Hour)
				require.NoError(t, err)
				return tok
			},
			via:      "header",
			wantCode: http.StatusOK,
		},
		{
			name: "TokenViaQueryParam",
			token: func(t *testing.T) string {
				tok, err := auth.GenerateBeeToken(testSecret, time.Hour)
				require.NoError(t, err)
				return tok
			},
			via:      "query",
			wantCode: http.StatusOK,
		},
		{
			name: "AllowsBeeToken",
			token: func(t *testing.T) string {
				tok, err := auth.GenerateBeeToken(testSecret, time.Hour)
				require.NoError(t, err)
				return tok
			},
			via:      "header",
			extra:    []gin.HandlerFunc{rpc.RequireBeeOrWorker()},
			wantCode: http.StatusOK,
		},
		{
			name: "AllowsWorkerToken",
			token: func(t *testing.T) string {
				tok, err := auth.GenerateWorkerToken(testSecret, "wid-1", nil, time.Hour)
				require.NoError(t, err)
				return tok
			},
			via:      "header",
			extra:    []gin.HandlerFunc{rpc.RequireBeeOrWorker()},
			wantCode: http.StatusOK,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newRouter(testSecret, tc.extra...)
			w := httptest.NewRecorder()
			tok := tc.token(t)

			var req *http.Request
			if tc.via == "query" {
				req, _ = http.NewRequest(http.MethodGet, "/test?api_key="+tok, nil)
			} else {
				req, _ = http.NewRequest(http.MethodGet, "/test", nil)
				if tok != "" {
					req.Header.Set("X-API-Key", tok)
				}
			}
			r.ServeHTTP(w, req)
			assert.Equal(t, tc.wantCode, w.Code)
		})
	}
}

func TestWorkerIDStoredInContext(t *testing.T) {
	tok, err := auth.GenerateWorkerToken(testSecret, "worker-999", nil, time.Hour)
	require.NoError(t, err)
	r := gin.New()
	r.Use(rpc.JWTAuthMiddleware(testSecret))
	r.GET("/test", func(c *gin.Context) {
		wid, _ := c.Get(rpc.CtxKeyWorkerID)
		if wid != "worker-999" {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "wrong worker id"})
			return
		}
		c.Status(http.StatusOK)
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-API-Key", tok)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestWorkerScopesStoredInContext(t *testing.T) {
	scopes := []string{auth.ScopeReadWorkers, auth.ScopeReadTasks}
	tok, err := auth.GenerateWorkerToken(testSecret, "worker-scoped", scopes, time.Hour)
	require.NoError(t, err)
	r := gin.New()
	r.Use(rpc.JWTAuthMiddleware(testSecret))
	r.GET("/test", func(c *gin.Context) {
		raw, _ := c.Get(rpc.CtxKeyScopes)
		got, _ := raw.([]string)
		if len(got) != 2 || got[0] != auth.ScopeReadWorkers || got[1] != auth.ScopeReadTasks {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "wrong scopes"})
			return
		}
		c.Status(http.StatusOK)
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-API-Key", tok)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}
