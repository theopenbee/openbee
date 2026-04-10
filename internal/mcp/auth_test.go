package mcp_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/theopenbee/openbee/internal/infra/auth"
	"github.com/theopenbee/openbee/internal/mcp"
)

func init() {
	gin.SetMode(gin.TestMode)
}

const testSecret = "test-secret-xyz"

func newRouter(secret string, extra ...gin.HandlerFunc) *gin.Engine {
	r := gin.New()
	r.Use(append([]gin.HandlerFunc{mcp.JWTAuthMiddleware(secret)}, extra...)...)
	r.GET("/test", func(c *gin.Context) { c.Status(http.StatusOK) })
	return r
}

func TestJWTAuthMiddleware_NoToken(t *testing.T) {
	r := newRouter(testSecret)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/test", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestJWTAuthMiddleware_InvalidToken(t *testing.T) {
	r := newRouter(testSecret)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-API-Key", "not-a-jwt")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestJWTAuthMiddleware_ValidBeeToken(t *testing.T) {
	tok, _ := auth.GenerateBeeToken(testSecret, time.Hour)
	r := newRouter(testSecret)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-API-Key", tok)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestJWTAuthMiddleware_ValidWorkerToken(t *testing.T) {
	tok, _ := auth.GenerateWorkerToken(testSecret, "wid-1", nil, time.Hour)
	r := newRouter(testSecret)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-API-Key", tok)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestJWTAuthMiddleware_TokenViaQueryParam(t *testing.T) {
	tok, _ := auth.GenerateBeeToken(testSecret, time.Hour)
	r := newRouter(testSecret)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/test?api_key="+tok, nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 via query param, got %d", w.Code)
	}
}


func TestRequireBeeOrWorker_AllowsBeeToken(t *testing.T) {
	tok, _ := auth.GenerateBeeToken(testSecret, time.Hour)
	r := newRouter(testSecret, mcp.RequireBeeOrWorker())
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-API-Key", tok)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestRequireBeeOrWorker_AllowsWorkerToken(t *testing.T) {
	tok, _ := auth.GenerateWorkerToken(testSecret, "wid-1", nil, time.Hour)
	r := newRouter(testSecret, mcp.RequireBeeOrWorker())
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-API-Key", tok)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestWorkerIDStoredInContext(t *testing.T) {
	tok, _ := auth.GenerateWorkerToken(testSecret, "worker-999", nil, time.Hour)
	r := gin.New()
	r.Use(mcp.JWTAuthMiddleware(testSecret))
	r.GET("/test", func(c *gin.Context) {
		wid, _ := c.Get(mcp.CtxKeyWorkerID)
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
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestWorkerScopesStoredInContext(t *testing.T) {
	scopes := []string{auth.ScopeReadWorkers, auth.ScopeReadTasks}
	tok, _ := auth.GenerateWorkerToken(testSecret, "worker-scoped", scopes, time.Hour)
	r := gin.New()
	r.Use(mcp.JWTAuthMiddleware(testSecret))
	r.GET("/test", func(c *gin.Context) {
		raw, _ := c.Get(mcp.CtxKeyScopesKey)
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
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}
