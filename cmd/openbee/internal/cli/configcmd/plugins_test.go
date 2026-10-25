package configcmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/theopenbee/openbee/internal/infra/config"
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
	vals.WeixinEnabled = true
	vals.WeixinUserID = "wx_x"
	vals.FeederMaxConcurrentBee = 7
	vals.PluginSections = map[string]map[string]any{
		"discord": {"enabled": true, "bot_name": "DBot", "token": "x", "allowed_guilds": []string{"1", "22"}, "max_media_size": 1024},
	}

	cfg, err := config.Load(renderToFile(t, vals))
	require.NoError(t, err)
	assert.True(t, cfg.Bee.Platforms.Weixin.Enabled)
	assert.Equal(t, "wx_x", cfg.Bee.Platforms.Weixin.UserID)
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
