package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVersionHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name    string
		version string
		bundled bool
		want    string
	}{
		{"standalone", "1.0.5", false, `{"version":"1.0.5","bundled":false}`},
		{"bundled", "1.0.6", true, `{"version":"1.0.6","bundled":true}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := gin.New()
			r.GET("/api/version", NewVersionHandler(tc.version, tc.bundled).Get)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/version", nil))
			require.Equal(t, http.StatusOK, w.Code, w.Body.String())
			assert.JSONEq(t, tc.want, w.Body.String())
		})
	}
}
