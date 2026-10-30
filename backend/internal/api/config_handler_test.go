package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/theopenbee/openbee/internal/ai/aitest"
)

func TestConfigHandler_LiveEngines(t *testing.T) {
	gin.SetMode(gin.TestMode)
	src := aitest.NewSource(nil)
	r := gin.New()
	r.GET("/api/config", NewConfigHandler("zh", src).Get)
	get := func() string {
		t.Helper()
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/config", nil))
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		return w.Body.String()
	}

	assert.JSONEq(t, `{"language":"zh","enabled_engines":[],"engines":[]}`, get())

	src.Add("pi", map[string]string{"en": "Pi"}, &aitest.Engine{})
	src.Add("claude", map[string]string{"en": "Claude Code", "zh": "Claude Code"}, &aitest.Engine{})
	assert.JSONEq(t, `{
		"language": "zh",
		"enabled_engines": ["claude", "pi"],
		"engines": [
			{"id": "claude", "name": {"en": "Claude Code", "zh": "Claude Code"}},
			{"id": "pi", "name": {"en": "Pi"}}
		]
	}`, get())

	src.Remove("claude")
	assert.JSONEq(t, `{"language":"zh","enabled_engines":["pi"],"engines":[{"id":"pi","name":{"en":"Pi"}}]}`, get())
}

func TestConfigHandler_DefaultsLanguage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/api/config", NewConfigHandler("", aitest.NewSource(nil)).Get)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/config", nil))
	assert.JSONEq(t, `{"language":"en","enabled_engines":[],"engines":[]}`, w.Body.String())
}
