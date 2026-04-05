package ctlclient_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/theopenbee/openbee/internal/ctlclient"
	"github.com/theopenbee/openbee/internal/infra/utils"
)

func TestCall_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/mcp/bee/call", r.URL.Path)
		assert.Equal(t, "testkey", r.Header.Get("X-API-Key"))
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"result": []any{}})
	}))
	defer srv.Close()

	c := &ctlclient.Client{BaseURL: srv.URL, APIKey: "testkey", HTTPClient: &http.Client{}}
	result, err := c.Call(utils.ListWorkers, map[string]any{})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestCall_ToolError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"error": "unknown tool: bad_tool"})
	}))
	defer srv.Close()

	c := &ctlclient.Client{BaseURL: srv.URL, APIKey: "key", HTTPClient: &http.Client{}}
	_, err := c.Call("bad_tool", map[string]any{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown tool")
}

func TestCall_Unauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer srv.Close()

	c := &ctlclient.Client{BaseURL: srv.URL, APIKey: "wrong", HTTPClient: &http.Client{}}
	_, err := c.Call(utils.ListWorkers, map[string]any{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unauthorized")
}

func TestCall_ConnectionRefused(t *testing.T) {
	c := &ctlclient.Client{BaseURL: "http://127.0.0.1:19999", APIKey: "key", HTTPClient: &http.Client{}}
	_, err := c.Call(utils.ListWorkers, map[string]any{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot connect")
}
