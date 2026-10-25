package configcmd

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/theopenbee/openbee/internal/infra/config"
	"github.com/theopenbee/openbee/internal/plugin"
)

func renderToFile(t *testing.T, vals configValues) string {
	t.Helper()
	out, err := renderConfig(vals)
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, out, 0o644))
	return path
}

func TestRenderConfig_WithPluginSection(t *testing.T) {
	vals := defaultValues()
	vals.LinearEnabled = true
	vals.LinearAPIKey = "lin_x"
	vals.FeederMaxConcurrentBee = 7
	vals.PluginSections = map[string]map[string]any{
		"discord": {"enabled": true, "bot_name": "DBot", "token": "x", "allowed_guilds": []string{"1", "22"}, "max_media_size": 1024},
	}

	cfg, err := config.Load(renderToFile(t, vals))
	require.NoError(t, err)
	assert.True(t, cfg.Bee.Platforms.Linear.Enabled)
	assert.Equal(t, "lin_x", cfg.Bee.Platforms.Linear.APIKey)
	assert.Equal(t, 7, cfg.Bee.Feeder.MaxConcurrentBee)
	d := cfg.Bee.Platforms.Plugins["discord"]
	assert.Equal(t, true, d["enabled"])
	assert.Equal(t, "DBot", d["bot_name"])
	assert.Equal(t, "x", d["token"])
	assert.Equal(t, []any{"1", "22"}, d["allowed_guilds"])
	assert.Equal(t, 1024, d["max_media_size"])
}

func TestRenderConfig_PreservesUnknownPluginSection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`
language: en
server:
  port: 8080
bee:
  platforms:
    ghost:
      enabled: true
      token: "007"
      nested:
        level: 2
        tags: [a, b]
      ratio: 1.5
    feishu:
      enabled: true
      app_id: "cli_x"
      app_secret: "s"
      bot_name: "FeishuBot"
`), 0o644))
	original, err := config.Load(path)
	require.NoError(t, err)

	vals := loadExistingConfig(path)
	require.NotNil(t, vals)
	reloaded, err := config.Load(renderToFile(t, *vals))
	require.NoError(t, err)
	assert.Equal(t, original.Bee.Platforms.Plugins["ghost"], reloaded.Bee.Platforms.Plugins["ghost"])
	assert.Equal(t, "007", reloaded.Bee.Platforms.Plugins["ghost"]["token"])
	assert.Equal(t, original.Bee.Platforms.Plugins["feishu"], reloaded.Bee.Platforms.Plugins["feishu"])
	assert.Equal(t, "cli_x", reloaded.Bee.Platforms.Plugins["feishu"]["app_id"])
}

func TestRenderPluginSections_Empty(t *testing.T) {
	out, err := renderPluginSections(nil)
	require.NoError(t, err)
	assert.Equal(t, "", out)

	vals := defaultValues()
	rendered, err := renderConfig(vals)
	require.NoError(t, err)
	assert.Contains(t, string(rendered), "max_media_size: "+vals.LinearMaxMediaSize+"\n  feeder:")
}

func TestRenderPluginSections_Indentation(t *testing.T) {
	out, err := renderPluginSections(map[string]map[string]any{
		"zeta":  {"enabled": false},
		"alpha": {"enabled": true, "list": []string{"x"}},
	})
	require.NoError(t, err)
	lines := strings.Split(out, "\n")
	assert.Equal(t, "    alpha:", lines[0])
	assert.Equal(t, "      enabled: true", lines[1])
	assert.Contains(t, out, "\n    zeta:\n      enabled: false")
	assert.False(t, strings.HasSuffix(out, "\n"))
}

func TestApplyPluginSelection_DeselectedPluginKeepsValues(t *testing.T) {
	vals := defaultValues()
	vals.PluginSections = map[string]map[string]any{
		"discord": {"enabled": true, "token": "x"},
		"slack":   {"enabled": false, "token": "y"},
		"ghost":   {"enabled": true, "token": "z"},
	}
	applyPluginSelection(&vals, []string{"discord", "slack"}, []string{"slack"})

	assert.Equal(t, map[string]any{"enabled": false, "token": "x"}, vals.PluginSections["discord"])
	assert.Equal(t, map[string]any{"enabled": false, "token": "y"}, vals.PluginSections["slack"])
	assert.Equal(t, map[string]any{"enabled": true, "token": "z"}, vals.PluginSections["ghost"])
}

func setupTestManifest() plugin.Manifest {
	return plugin.Manifest{
		ID:    "weixin",
		Setup: &plugin.Setup{Command: []string{"node", "setup.js"}},
		Config: []plugin.ConfigField{
			{Key: "token", Type: plugin.FieldSecret, Required: true},
			{Key: "user_id", Type: plugin.FieldString},
		},
	}
}

func stubSetupCommand(t *testing.T, values map[string]any, err error) *int {
	t.Helper()
	calls := 0
	orig := runPluginSetupCommand
	runPluginSetupCommand = func(plugin.Manifest, string) (map[string]any, error) {
		calls++
		return values, err
	}
	t.Cleanup(func() { runPluginSetupCommand = orig })
	return &calls
}

func TestRunPluginSetup_MergesValues(t *testing.T) {
	calls := stubSetupCommand(t, map[string]any{"token": "tok", "user_id": "u1"}, nil)
	section := map[string]any{"bot_name": "Bee"}

	provided, err := runPluginSetup(setupTestManifest(), "en", section)
	require.NoError(t, err)
	assert.Equal(t, 1, *calls)
	assert.Equal(t, map[string]any{"token": "tok", "user_id": "u1"}, provided)
	assert.Equal(t, map[string]any{"bot_name": "Bee", "token": "tok", "user_id": "u1"}, section)
}

func TestRunPluginSetup_FailureFallsBackToPrompts(t *testing.T) {
	stubSetupCommand(t, nil, errors.New("expired"))
	section := map[string]any{}

	provided, err := runPluginSetup(setupTestManifest(), "en", section)
	require.NoError(t, err)
	assert.Empty(t, provided)
	assert.Empty(t, section)
}

func TestRunPluginSetup_NoSetup(t *testing.T) {
	calls := stubSetupCommand(t, nil, nil)
	m := setupTestManifest()
	m.Setup = nil

	provided, err := runPluginSetup(m, "en", map[string]any{})
	require.NoError(t, err)
	assert.Empty(t, provided)
	assert.Equal(t, 0, *calls)
}

func TestPluginConfigured(t *testing.T) {
	m := setupTestManifest()
	assert.False(t, pluginConfigured(m, map[string]any{}))
	assert.False(t, pluginConfigured(m, map[string]any{"enabled": false, "token": "", "user_id": "u1"}))
	assert.True(t, pluginConfigured(m, map[string]any{"token": "tok"}))

	m.Config = []plugin.ConfigField{{Key: "region", Type: plugin.FieldString}}
	assert.False(t, pluginConfigured(m, map[string]any{"enabled": true}))
	assert.True(t, pluginConfigured(m, map[string]any{"region": "cn"}))
}
