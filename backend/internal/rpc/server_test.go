package rpc_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/theopenbee/openbee/internal/infra/utils"
)

func TestHandleCall_ListWorkers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s := setupServerWithMessaging(t)
	r := gin.New()
	r.POST("/rpc/bee/call", s.HandleCall)

	body := `{"name":"` + utils.ListWorkers + `","arguments":{}}`
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/rpc/bee/call", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	require.Equal(t, 200, w.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	_, ok := resp["result"]
	assert.True(t, ok, "response should have 'result' key, got: %s", w.Body.String())
}

func TestHandleCall_UnknownTool(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s := setupServerWithMessaging(t)
	r := gin.New()
	r.POST("/rpc/bee/call", s.HandleCall)

	body := `{"name":"no_such_tool","arguments":{}}`
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/rpc/bee/call", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	require.Equal(t, 200, w.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	errMsg, ok := resp["error"]
	assert.True(t, ok, "response should have 'error' key, got: %s", w.Body.String())
	assert.Contains(t, errMsg.(string), "no_such_tool")
}

func TestHandleCall_BadJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s := setupServerWithMessaging(t)
	r := gin.New()
	r.POST("/rpc/bee/call", s.HandleCall)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/rpc/bee/call", strings.NewReader("not-json"))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, 400, w.Code)
}
