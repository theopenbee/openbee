package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/theopenbee/openbee/internal/apperr"
	"github.com/theopenbee/openbee/internal/domain/pluginmgr"
	"github.com/theopenbee/openbee/internal/plugin"
	"github.com/theopenbee/openbee/internal/plugin/catalog"
)

type fakePluginService struct {
	statuses   []pluginmgr.Status
	detail     pluginmgr.Detail
	view       pluginmgr.CatalogView
	ops        map[string]pluginmgr.Operation
	err        error
	refresh    bool
	installed  string
	settingsID string
	botName    *string
	config     map[string]any
	calls      []string
}

func (f *fakePluginService) List(context.Context) ([]pluginmgr.Status, error) {
	return f.statuses, f.err
}

func (f *fakePluginService) Get(_ context.Context, id string) (pluginmgr.Detail, error) {
	if f.err != nil {
		return pluginmgr.Detail{}, f.err
	}
	return f.detail, nil
}

func (f *fakePluginService) Catalog(_ context.Context, refresh bool) (pluginmgr.CatalogView, error) {
	f.refresh = refresh
	return f.view, f.err
}

func (f *fakePluginService) Install(spec string) pluginmgr.Operation {
	f.installed = spec
	return pluginmgr.Operation{ID: "op1", Kind: "install", Spec: spec, Status: pluginmgr.OpQueued, CreatedAt: time.Unix(100, 0)}
}

func (f *fakePluginService) Uninstall(_ context.Context, id string) (pluginmgr.Operation, error) {
	if f.err != nil {
		return pluginmgr.Operation{}, f.err
	}
	return pluginmgr.Operation{ID: "op2", Kind: "uninstall", PluginID: id, Status: pluginmgr.OpQueued}, nil
}

func (f *fakePluginService) Operations() []pluginmgr.Operation {
	out := make([]pluginmgr.Operation, 0, len(f.ops))
	for _, op := range f.ops {
		out = append(out, op)
	}
	return out
}

func (f *fakePluginService) Operation(id string) (pluginmgr.Operation, bool) {
	op, ok := f.ops[id]
	return op, ok
}

func (f *fakePluginService) UpdateSettings(_ context.Context, id string, botName *string, config map[string]any) error {
	f.settingsID, f.botName, f.config = id, botName, config
	return f.err
}

func (f *fakePluginService) Enable(_ context.Context, id string) error {
	f.calls = append(f.calls, "enable:"+id)
	return f.err
}

func (f *fakePluginService) Disable(_ context.Context, id string) error {
	f.calls = append(f.calls, "disable:"+id)
	return f.err
}

func (f *fakePluginService) Restart(_ context.Context, id string) error {
	f.calls = append(f.calls, "restart:"+id)
	return f.err
}

func newPluginRouter(svc *fakePluginService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	h := NewPluginHandler(svc)
	r := gin.New()
	g := r.Group("/api")
	g.GET("/plugins", h.List)
	g.GET("/plugins/catalog", h.Catalog)
	g.POST("/plugins/install", h.Install)
	g.GET("/plugins/operations/:id", h.Operation)
	g.GET("/plugins/:id", h.Get)
	g.PUT("/plugins/:id/settings", h.UpdateSettings)
	g.POST("/plugins/:id/enable", h.Enable)
	g.POST("/plugins/:id/disable", h.Disable)
	g.POST("/plugins/:id/restart", h.Restart)
	g.DELETE("/plugins/:id", h.Uninstall)
	return r
}

func doJSON(t *testing.T, r http.Handler, method, path string, body any) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		require.NoError(t, json.NewEncoder(&buf).Encode(body))
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var out map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return w, out
}

func TestPluginHandler_List(t *testing.T) {
	svc := &fakePluginService{statuses: []pluginmgr.Status{{
		ID: "telegram", Package: "@theopenbee/channel-telegram", Source: "npm", Version: "0.1.0",
		Name: plugin.LocalizedText{"en": "Telegram"}, Enabled: true, BotName: "bee", State: pluginmgr.StateRunning, HasSetup: false,
	}}}
	w, body := doJSON(t, newPluginRouter(svc), http.MethodGet, "/api/plugins", nil)
	require.Equal(t, http.StatusOK, w.Code)
	items := body["plugins"].([]any)
	require.Len(t, items, 1)
	p := items[0].(map[string]any)
	assert.Equal(t, "telegram", p["id"])
	assert.Equal(t, "running", p["state"])
	assert.Equal(t, "bee", p["bot_name"])
	assert.Equal(t, map[string]any{"en": "Telegram"}, p["name"])
	assert.Equal(t, false, p["has_setup"])
}

func TestPluginHandler_Get(t *testing.T) {
	svc := &fakePluginService{detail: pluginmgr.Detail{
		Status: pluginmgr.Status{ID: "telegram", State: pluginmgr.StateDisabled},
		Config: map[string]any{"token": "1234****EFGH"},
		Fields: []plugin.ConfigField{{Key: "token", Type: plugin.FieldSecret, Required: true}, {Key: "auth_code", Type: plugin.FieldString, RandomDefault: true}},
	}}
	w, body := doJSON(t, newPluginRouter(svc), http.MethodGet, "/api/plugins/telegram", nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, map[string]any{"token": "1234****EFGH"}, body["config"])
	fields := body["fields"].([]any)
	require.Len(t, fields, 2)
	assert.Equal(t, "secret", fields[0].(map[string]any)["type"])
	assert.Equal(t, true, fields[1].(map[string]any)["random_default"])
}

func TestPluginHandler_UnknownPlugin404(t *testing.T) {
	svc := &fakePluginService{err: apperr.New(pluginmgr.CodeNotFound, "plugin \"x\" is not installed")}
	w, body := doJSON(t, newPluginRouter(svc), http.MethodGet, "/api/plugins/x", nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Equal(t, pluginmgr.CodeNotFound, body["code"])
}

func TestPluginHandler_Catalog(t *testing.T) {
	svc := &fakePluginService{view: pluginmgr.CatalogView{
		SchemaVersion: 1,
		Categories:    []catalog.Vocab{{ID: "channel", Name: plugin.LocalizedText{"en": "Channels"}}},
		Plugins: []pluginmgr.CatalogPlugin{{
			Entry:     catalog.Entry{Category: "channel", ID: "telegram", Package: "@theopenbee/channel-telegram", Version: "0.2.0", Official: true},
			Installed: true, InstalledVersion: "0.1.0", UpdateAvailable: true, Installable: true,
		}},
	}}
	w, body := doJSON(t, newPluginRouter(svc), http.MethodGet, "/api/plugins/catalog?refresh=1", nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.True(t, svc.refresh)
	p := body["plugins"].([]any)[0].(map[string]any)
	assert.Equal(t, "telegram", p["id"])
	assert.Equal(t, true, p["official"])
	assert.Equal(t, true, p["installed"])
	assert.Equal(t, "0.1.0", p["installed_version"])
	assert.Equal(t, true, p["update_available"])
	assert.Equal(t, true, p["installable"])
	assert.EqualValues(t, 1, body["schema_version"])
}

func TestPluginHandler_CatalogUnavailable502(t *testing.T) {
	svc := &fakePluginService{err: apperr.New(pluginmgr.CodeCatalogUnavailable, "plugin catalog is unavailable")}
	w, body := doJSON(t, newPluginRouter(svc), http.MethodGet, "/api/plugins/catalog", nil)
	assert.Equal(t, http.StatusBadGateway, w.Code)
	assert.Equal(t, pluginmgr.CodeCatalogUnavailable, body["code"])
}

func TestPluginHandler_InstallReturns202(t *testing.T) {
	svc := &fakePluginService{}
	w, body := doJSON(t, newPluginRouter(svc), http.MethodPost, "/api/plugins/install", map[string]string{"spec": " @theopenbee/channel-telegram@0.1.0 "})
	require.Equal(t, http.StatusAccepted, w.Code)
	assert.Equal(t, "@theopenbee/channel-telegram@0.1.0", svc.installed)
	assert.Equal(t, "op1", body["id"])
	assert.Equal(t, "install", body["kind"])
	assert.Equal(t, "queued", body["status"])
	assert.NotContains(t, body, "finished_at")
}

func TestPluginHandler_InstallRequiresSpec(t *testing.T) {
	svc := &fakePluginService{}
	w, _ := doJSON(t, newPluginRouter(svc), http.MethodPost, "/api/plugins/install", map[string]string{"spec": " "})
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Empty(t, svc.installed)
}

func TestPluginHandler_Operation(t *testing.T) {
	svc := &fakePluginService{ops: map[string]pluginmgr.Operation{"op1": {
		ID: "op1", Kind: "install", PluginID: "telegram", Status: pluginmgr.OpFailed, Error: "boom",
		CreatedAt: time.Unix(100, 0), FinishedAt: time.Unix(160, 0),
	}}}
	r := newPluginRouter(svc)
	w, body := doJSON(t, r, http.MethodGet, "/api/plugins/operations/op1", nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "failed", body["status"])
	assert.Equal(t, "boom", body["error"])
	assert.Equal(t, "telegram", body["plugin_id"])
	assert.NotEmpty(t, body["finished_at"])

	w, body = doJSON(t, r, http.MethodGet, "/api/plugins/operations/nope", nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Equal(t, pluginmgr.CodeOperationNotFound, body["code"])
}

func TestPluginHandler_UpdateSettings(t *testing.T) {
	svc := &fakePluginService{}
	w, _ := doJSON(t, newPluginRouter(svc), http.MethodPut, "/api/plugins/telegram/settings", map[string]any{
		"bot_name": "bee",
		"config":   map[string]any{"token": "t"},
	})
	require.Equal(t, http.StatusNoContent, w.Code)
	assert.Equal(t, "telegram", svc.settingsID)
	require.NotNil(t, svc.botName)
	assert.Equal(t, "bee", *svc.botName)
	assert.Equal(t, map[string]any{"token": "t"}, svc.config)

	svc = &fakePluginService{}
	w, _ = doJSON(t, newPluginRouter(svc), http.MethodPut, "/api/plugins/telegram/settings", map[string]any{"config": map[string]any{}})
	require.Equal(t, http.StatusNoContent, w.Code)
	assert.Nil(t, svc.botName)
}

func TestPluginHandler_SettingsValidationError(t *testing.T) {
	for code, status := range map[string]int{
		pluginmgr.CodeConfigInvalid:    http.StatusBadRequest,
		pluginmgr.CodeUnknownField:     http.StatusBadRequest,
		pluginmgr.CodeBotNameConflict:  http.StatusBadRequest,
		pluginmgr.CodeEnvSecretMissing: http.StatusBadRequest,
		pluginmgr.CodeNotConfigured:    http.StatusBadRequest,
	} {
		svc := &fakePluginService{err: apperr.New(code, "nope")}
		w, body := doJSON(t, newPluginRouter(svc), http.MethodPut, "/api/plugins/telegram/settings", map[string]any{"config": map[string]any{}})
		assert.Equal(t, status, w.Code, code)
		assert.Equal(t, code, body["code"])
	}
	svc := &fakePluginService{err: errors.New("disk full")}
	w, _ := doJSON(t, newPluginRouter(svc), http.MethodPut, "/api/plugins/telegram/settings", map[string]any{"config": map[string]any{}})
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestPluginHandler_Lifecycle(t *testing.T) {
	svc := &fakePluginService{}
	r := newPluginRouter(svc)
	for _, action := range []string{"enable", "disable", "restart"} {
		w, _ := doJSON(t, r, http.MethodPost, "/api/plugins/telegram/"+action, nil)
		assert.Equal(t, http.StatusNoContent, w.Code, action)
	}
	assert.Equal(t, []string{"enable:telegram", "disable:telegram", "restart:telegram"}, svc.calls)
}

func TestPluginHandler_Uninstall(t *testing.T) {
	svc := &fakePluginService{}
	w, body := doJSON(t, newPluginRouter(svc), http.MethodDelete, "/api/plugins/telegram", nil)
	require.Equal(t, http.StatusAccepted, w.Code)
	assert.Equal(t, "uninstall", body["kind"])
	assert.Equal(t, "telegram", body["plugin_id"])
}

func TestPluginHandler_Operations(t *testing.T) {
	svc := &fakePluginService{ops: map[string]pluginmgr.Operation{"op1": {ID: "op1", Kind: "install", Spec: "x@1", Status: pluginmgr.OpRunning}}}
	r := newPluginRouter(svc)
	r.GET("/api/plugins/operations", NewPluginHandler(svc).Operations)
	w, _ := doJSON(t, r, http.MethodGet, "/api/plugins/operations", nil)
	require.Equal(t, http.StatusOK, w.Code)
	var body struct {
		Operations []map[string]any `json:"operations"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Len(t, body.Operations, 1)
	assert.Equal(t, "running", body.Operations[0]["status"])
}
