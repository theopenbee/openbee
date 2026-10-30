package plugincmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/theopenbee/openbee/internal/infra/auth"
)

const clientSecret = "client-test-secret"

func noEnv(string) string { return "" }

func writeClientConfig(t *testing.T, port int, secret string) string {
	t.Helper()
	body := fmt.Sprintf("language: en\nserver:\n  host: 127.0.0.1\n  port: %d\n", port)
	if secret != "" {
		body += fmt.Sprintf("bee:\n  rpc:\n    token_secret: %q\n", secret)
	}
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	return path
}

type recorded struct {
	method string
	path   string
	body   string
	key    string
}

func fakeServer(t *testing.T, handler func(w http.ResponseWriter, r *http.Request)) (*Client, *[]recorded) {
	t.Helper()
	var calls []recorded
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		calls = append(calls, recorded{method: r.Method, path: r.URL.RequestURI(), body: string(b), key: r.Header.Get("X-API-Key")})
		handler(w, r)
	}))
	t.Cleanup(srv.Close)
	return &Client{baseURL: srv.URL, token: "tok", http: srv.Client()}, &calls
}

func TestNewClientFromConfig_SignsAdminToken(t *testing.T) {
	c, err := newClientFromConfig(writeClientConfig(t, 12999, clientSecret), noEnv)
	require.NoError(t, err)
	assert.Equal(t, "http://127.0.0.1:12999", c.baseURL)
	key, err := c.apiKey()
	require.NoError(t, err)
	claims, err := auth.ValidateToken(key, clientSecret)
	require.NoError(t, err)
	assert.Equal(t, auth.TokenTypeAdmin, claims.Type)
}

func TestNewClientFromConfig_RefusesAgentEnvironment(t *testing.T) {
	env := func(k string) string {
		if k == "OPENBEE_API_KEY" {
			return "worker-token"
		}
		return ""
	}
	_, err := newClientFromConfig(writeClientConfig(t, 12999, clientSecret), env)
	assert.ErrorIs(t, err, errAgentEnvironment)
}

func TestNewClientFromConfig_RequiresTokenSecret(t *testing.T) {
	_, err := newClientFromConfig(writeClientConfig(t, 12999, ""), noEnv)
	assert.ErrorIs(t, err, errNoTokenSecret)
}

func TestNewClientFromConfig_MissingConfig(t *testing.T) {
	_, err := newClientFromConfig(filepath.Join(t.TempDir(), "missing.yaml"), noEnv)
	assert.Error(t, err)
}

func TestClient_SendsTokenAndDecodesList(t *testing.T) {
	c, calls := fakeServer(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"plugins":[{"id":"telegram","name":{"en":"Telegram"},"version":"0.1.0","enabled":true,"bot_name":"TBot","state":"running","has_setup":false}]}`))
	})
	list, err := c.List(context.Background())
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, "telegram", list[0].ID)
	assert.Equal(t, "Telegram", list[0].Name.Get("en"))
	assert.Equal(t, "running", list[0].State)
	assert.Equal(t, "TBot", list[0].BotName)
	assert.Equal(t, "tok", (*calls)[0].key)
	assert.Equal(t, "/api/plugins", (*calls)[0].path)
}

func TestClient_Detail(t *testing.T) {
	c, _ := fakeServer(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"id":"telegram","state":"disabled","path":"/p","has_setup":true,"setup_label":{"en":"QR login"},"config":{"token":"****abcd"},"fields":[{"key":"token","type":"secret","required":true,"label":{"en":"Token","zh":"令牌"}},{"key":"auth_code","type":"string","random_default":true}]}`))
	})
	d, err := c.Get(context.Background(), "telegram")
	require.NoError(t, err)
	assert.Equal(t, "/p", d.Path)
	assert.True(t, d.HasSetup)
	assert.Equal(t, "QR login", d.SetupLabel.Get("en"))
	assert.Equal(t, "****abcd", d.Config["token"])
	require.Len(t, d.Fields, 2)
	assert.Equal(t, "令牌", d.Fields[0].Label.Get("zh"))
	assert.True(t, d.Fields[1].RandomDefault)
}

func TestClient_CatalogAndInstall(t *testing.T) {
	c, calls := fakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/plugins/catalog"):
			_, _ = w.Write([]byte(`{"schema_version":1,"categories":[],"tags":[],"plugins":[{"category":"channel","id":"telegram","package":"@theopenbee/channel-telegram","version":"0.2.0","name":{"en":"Telegram"},"installed":true,"installed_version":"0.1.0","update_available":true,"installable":true}]}`))
		case r.URL.Path == "/api/plugins/install":
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"id":"op1","kind":"install","status":"queued"}`))
		case r.URL.Path == "/api/plugins/operations/op1":
			_, _ = w.Write([]byte(`{"id":"op1","kind":"install","plugin_id":"telegram","status":"succeeded"}`))
		}
	})
	cat, err := c.Catalog(context.Background(), true)
	require.NoError(t, err)
	require.Len(t, cat.Plugins, 1)
	assert.True(t, cat.Plugins[0].UpdateAvailable)
	assert.Equal(t, "0.1.0", cat.Plugins[0].InstalledVersion)
	assert.Equal(t, "/api/plugins/catalog?refresh=1", (*calls)[0].path)

	op, err := c.Install(context.Background(), "@theopenbee/channel-telegram@0.2.0")
	require.NoError(t, err)
	assert.Equal(t, "op1", op.ID)
	assert.JSONEq(t, `{"spec":"@theopenbee/channel-telegram@0.2.0"}`, (*calls)[1].body)

	op, err = c.Operation(context.Background(), "op1")
	require.NoError(t, err)
	assert.Equal(t, "succeeded", op.Status)
	assert.Equal(t, "telegram", op.PluginID)
}

func TestClient_SettingsAndLifecycle(t *testing.T) {
	c, calls := fakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"id":"op2","kind":"uninstall","status":"queued"}`))
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	name := "TBot"
	require.NoError(t, c.UpdateSettings(context.Background(), "telegram", &name, map[string]any{"token": "t"}))
	var body map[string]any
	require.NoError(t, json.Unmarshal([]byte((*calls)[0].body), &body))
	assert.Equal(t, map[string]any{"bot_name": "TBot", "config": map[string]any{"token": "t"}}, body)

	require.NoError(t, c.UpdateSettings(context.Background(), "telegram", nil, map[string]any{}))
	assert.JSONEq(t, `{"config":{}}`, (*calls)[1].body)

	require.NoError(t, c.Enable(context.Background(), "telegram"))
	require.NoError(t, c.Disable(context.Background(), "telegram"))
	require.NoError(t, c.Restart(context.Background(), "telegram"))
	op, err := c.Uninstall(context.Background(), "telegram")
	require.NoError(t, err)
	assert.Equal(t, "op2", op.ID)
	var paths []string
	for _, call := range (*calls)[2:] {
		paths = append(paths, call.method+" "+call.path)
	}
	assert.Equal(t, []string{
		"POST /api/plugins/telegram/enable",
		"POST /api/plugins/telegram/disable",
		"POST /api/plugins/telegram/restart",
		"DELETE /api/plugins/telegram",
	}, paths)
}

func TestClient_APIError(t *testing.T) {
	c, _ := fakeServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"config \"token\" is required","code":"plugin_config_invalid","params":{"reason":"config \"token\" is required"}}`))
	})
	err := c.Enable(context.Background(), "telegram")
	var apiErr *APIError
	require.True(t, errors.As(err, &apiErr))
	assert.Equal(t, http.StatusBadRequest, apiErr.Status)
	assert.Equal(t, "plugin_config_invalid", apiErr.Code)
	assert.Contains(t, apiErr.Error(), "token")
}

func TestClient_NotRunning(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()
	ln.Close()
	c := &Client{baseURL: "http://" + addr, token: "tok", http: http.DefaultClient}
	_, err = c.List(context.Background())
	assert.ErrorIs(t, err, ErrNotRunning)
	assert.Contains(t, err.Error(), addr)
}

func TestClient_SignsFreshTokenPerRequest(t *testing.T) {
	var keys []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		keys = append(keys, r.Header.Get("X-API-Key"))
		_, _ = w.Write([]byte(`{"plugins":[]}`))
	}))
	t.Cleanup(srv.Close)
	c := &Client{baseURL: srv.URL, secret: clientSecret, http: srv.Client(), now: func() time.Time { return time.Now().Add(-time.Hour) }}
	_, err := c.List(context.Background())
	require.NoError(t, err)
	c.now = time.Now
	_, err = c.List(context.Background())
	require.NoError(t, err)
	require.Len(t, keys, 2)
	assert.NotEqual(t, keys[0], keys[1])
	_, err = auth.ValidateToken(keys[0], clientSecret)
	assert.Error(t, err)
	claims, err := auth.ValidateToken(keys[1], clientSecret)
	require.NoError(t, err)
	assert.Equal(t, auth.TokenTypeAdmin, claims.Type)
}
