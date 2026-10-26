package plugincmd

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/theopenbee/openbee/internal/infra/i18n"
	"github.com/theopenbee/openbee/internal/plugin"
)

func TestList_ShowsTableAndErrors(t *testing.T) {
	api := newFakeAPI()
	api.addPlugin(Detail{Plugin: Plugin{ID: "telegram", Name: plugin.LocalizedText{"en": "Telegram"}, Version: "0.1.0", Enabled: true, State: "running", BotName: "TBot"}})
	api.addPlugin(Detail{Plugin: Plugin{ID: "discord", Name: plugin.LocalizedText{"en": "Discord"}, Version: "0.1.0", Enabled: true, State: "failed", Error: "plugin rejected its config: bad token"}})
	out, err := runCmd(t, api, &fakePrompter{}, "list")
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(out), "\n")
	require.GreaterOrEqual(t, len(lines), 4)
	assert.Contains(t, lines[0], "STATE")
	assert.Contains(t, out, "TBot")
	assert.Contains(t, out, "running")
	assert.Contains(t, out, "discord: plugin rejected its config: bad token")
}

func TestList_Empty(t *testing.T) {
	out, err := runCmd(t, newFakeAPI(), &fakePrompter{}, "list")
	require.NoError(t, err)
	assert.Contains(t, out, "No channel plugins installed")
}

func TestEnable_WaitsForRunning(t *testing.T) {
	api := newFakeAPI()
	api.addPlugin(Detail{Plugin: Plugin{ID: "telegram", State: "disabled"}})
	api.states["telegram"] = []string{"starting", "running"}
	out, err := runCmd(t, api, &fakePrompter{}, "enable", "telegram")
	require.NoError(t, err)
	assert.Equal(t, []string{"enable:telegram"}, api.actions)
	assert.Contains(t, out, "telegram is running")
}

func TestDisable(t *testing.T) {
	api := newFakeAPI()
	api.addPlugin(Detail{Plugin: Plugin{ID: "telegram", State: "running", Enabled: true}})
	out, err := runCmd(t, api, &fakePrompter{}, "disable", "telegram")
	require.NoError(t, err)
	assert.Equal(t, []string{"disable:telegram"}, api.actions)
	assert.Contains(t, out, "Disabled telegram")
}

func TestRestart_ReportsFailure(t *testing.T) {
	api := newFakeAPI()
	api.addPlugin(Detail{Plugin: Plugin{ID: "telegram", State: "running", Enabled: true}})
	api.states["telegram"] = []string{"failed"}
	_, err := runCmd(t, api, &fakePrompter{}, "restart", "telegram")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bad token")
	assert.Equal(t, []string{"restart:telegram"}, api.actions)
}

func TestRemove_ConfirmDeclined(t *testing.T) {
	api := newFakeAPI()
	api.addPlugin(Detail{Plugin: Plugin{ID: "telegram"}})
	_, err := runCmd(t, api, &fakePrompter{interactive: true, answers: []any{false}}, "remove", "telegram")
	require.NoError(t, err)
	assert.Empty(t, api.actions)
}

func TestRemove_Confirmed(t *testing.T) {
	api := newFakeAPI()
	api.addPlugin(Detail{Plugin: Plugin{ID: "telegram"}})
	out, err := runCmd(t, api, &fakePrompter{interactive: true, answers: []any{true}}, "remove", "telegram")
	require.NoError(t, err)
	assert.Equal(t, []string{"remove:telegram"}, api.actions)
	assert.Contains(t, out, "Removed plugin telegram")
}

func TestRemove_YesSkipsConfirm(t *testing.T) {
	api := newFakeAPI()
	api.addPlugin(Detail{Plugin: Plugin{ID: "telegram"}})
	_, err := runCmd(t, api, &fakePrompter{}, "remove", "telegram", "--yes")
	require.NoError(t, err)
	assert.Equal(t, []string{"remove:telegram"}, api.actions)
}

func TestRemove_NonInteractiveNeedsYes(t *testing.T) {
	api := newFakeAPI()
	api.addPlugin(Detail{Plugin: Plugin{ID: "telegram"}})
	_, err := runCmd(t, api, &fakePrompter{}, "remove", "telegram")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--yes")
	assert.Empty(t, api.actions)
}

func runWithDeps(t *testing.T, d deps, args ...string) (string, error) {
	t.Helper()
	require.NoError(t, i18n.Load("en"))
	var out bytes.Buffer
	d.Out = &out
	if d.Prompter == nil {
		d.Prompter = &fakePrompter{}
	}
	d.Lang, d.Interval, d.OpTimeout, d.RunTimeout = "en", time.Millisecond, time.Second, 100*time.Millisecond
	if d.Getenv == nil {
		d.Getenv = noEnv
	}
	cmd := newCommand(d)
	cmd.SetArgs(args)
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	err := cmd.Execute()
	return out.String(), err
}

func TestAgentEnvironmentRefused(t *testing.T) {
	called := false
	d := deps{
		Getenv: func(k string) string {
			if k == "OPENBEE_API_KEY" {
				return "worker-token"
			}
			return ""
		},
		NewClient: func(string) (*Client, error) { called = true; return nil, errors.New("unreachable") },
	}
	for _, args := range [][]string{{"list"}, {"install", "telegram"}, {"remove", "telegram", "--yes"}} {
		_, err := runWithDeps(t, d, args...)
		require.Error(t, err, args)
		assert.Contains(t, err.Error(), "agents", args)
	}
	assert.False(t, called)
}

func TestFriendlyErrors(t *testing.T) {
	cases := map[error]string{
		fmt.Errorf("%w at http://127.0.0.1:9", ErrNotRunning): "openbee server -d",
		errNoTokenSecret: "bee.rpc.token_secret",
	}
	for cause, want := range cases {
		d := deps{NewClient: func(string) (*Client, error) { return nil, cause }}
		_, err := runWithDeps(t, d, "list")
		require.Error(t, err)
		assert.Contains(t, err.Error(), want)
	}
}

func TestNotRunningFromRequest(t *testing.T) {
	d := deps{NewClient: func(string) (*Client, error) {
		return &Client{baseURL: "http://127.0.0.1:1", token: "t", http: &httpClientNoRetry}, nil
	}}
	_, err := runWithDeps(t, d, "list")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "openbee server -d")
	assert.Contains(t, err.Error(), "127.0.0.1:1")
}

var httpClientNoRetry = http.Client{Timeout: time.Second}

func TestFriendlyErrors_Unauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
	}))
	t.Cleanup(srv.Close)
	d := deps{NewClient: func(string) (*Client, error) { return &Client{baseURL: srv.URL, token: "t", http: srv.Client()}, nil }}
	_, err := runWithDeps(t, d, "list")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bee.rpc.token_secret")
	assert.Contains(t, err.Error(), srv.URL)
}

func TestInstallByIDWhenOpenbeeIsDown(t *testing.T) {
	d := deps{NewClient: func(string) (*Client, error) {
		return &Client{baseURL: "http://127.0.0.1:1", token: "t", http: &httpClientNoRetry}, nil
	}}
	_, err := runWithDeps(t, d, "install", "lark")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "openbee server -d")
}

func TestLoadLanguageDefaultsToEnglish(t *testing.T) {
	path := writeClientConfig(t, 12999, clientSecret)
	require.NoError(t, os.WriteFile(path, []byte("server:\n  port: 1\n"), 0o600))
	assert.Equal(t, i18n.LangEN, loadLanguage(path))
}
