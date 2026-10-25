package plugincmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/theopenbee/openbee/internal/infra/i18n"
	"github.com/theopenbee/openbee/internal/plugin"
)

const testManifest = `{
  "protocol_version": 1,
  "id": "%s",
  "name": {"en": "%s"},
  "command": ["node", "index.js"],
  "version": "%s"
}`

func writePlugin(t *testing.T, id, name, version string) string {
	t.Helper()
	dir := t.TempDir()
	content := strings.Replace(strings.Replace(strings.Replace(testManifest, "%s", id, 1), "%s", name, 1), "%s", version, 1)
	require.NoError(t, os.WriteFile(filepath.Join(dir, plugin.ManifestFile), []byte(content), 0o644))
	return dir
}

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
	return path
}

func run(t *testing.T, reg *plugin.Registry, args ...string) (string, error) {
	t.Helper()
	require.NoError(t, i18n.Load("en"))
	var out bytes.Buffer
	cmd := newCommand(deps{
		Installer: &plugin.Installer{Registry: reg, Run: func(context.Context, string, string, ...string) ([]byte, error) {
			return nil, nil
		}},
		Out: &out,
	})
	cmd.SetArgs(args)
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	err := cmd.Execute()
	return out.String(), err
}

func newRegistry(t *testing.T) *plugin.Registry {
	return plugin.OpenRegistry(filepath.Join(t.TempDir(), "plugins"))
}

func TestPluginInstall_LocalPath(t *testing.T) {
	reg := newRegistry(t)
	dir := writePlugin(t, "discord", "Discord", "0.1.0")
	out, err := run(t, reg, "install", dir)
	require.NoError(t, err)
	assert.Contains(t, out, "discord")
	assert.Contains(t, out, "0.1.0")
	assert.Contains(t, out, firstLine(i18n.M.Output.Plugin.TrustWarning))
	assert.Contains(t, out, firstLine(i18n.M.Output.Plugin.NextSteps))
	_, ok, err := reg.Get("discord")
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestPluginInstall_Invalid(t *testing.T) {
	reg := newRegistry(t)
	dir := writePlugin(t, "linear", "Linear", "0.1.0")
	_, err := run(t, reg, "install", dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reserved")
}

func TestPluginList(t *testing.T) {
	reg := newRegistry(t)
	_, err := run(t, reg, "install", writePlugin(t, "discord", "Discord", "0.1.0"))
	require.NoError(t, err)
	_, err = run(t, reg, "install", writePlugin(t, "slack", "Slack", "0.2.0"))
	require.NoError(t, err)
	cfg := writeConfig(t, "bee:\n  platforms:\n    discord:\n      enabled: true\n      token: x\n")

	out, err := run(t, reg, "list", "-c", cfg)
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(out), "\n")
	require.Len(t, lines, 3)
	assert.Contains(t, lines[1], "discord")
	assert.Contains(t, lines[1], "Discord")
	assert.Contains(t, lines[1], "0.1.0")
	assert.Contains(t, lines[1], plugin.SourcePath)
	assert.Contains(t, lines[1], i18n.M.Output.Plugin.StatusEnabled)
	assert.Contains(t, lines[2], "slack")
	assert.Contains(t, lines[2], i18n.M.Output.Plugin.StatusDisabled)
}

func TestPluginList_Empty(t *testing.T) {
	out, err := run(t, newRegistry(t), "list", "-c", filepath.Join(t.TempDir(), "missing.yaml"))
	require.NoError(t, err)
	assert.Contains(t, out, i18n.M.Output.Plugin.ListEmpty)
}

func TestPluginRemove_WarnsWhenStillEnabled(t *testing.T) {
	reg := newRegistry(t)
	_, err := run(t, reg, "install", writePlugin(t, "discord", "Discord", "0.1.0"))
	require.NoError(t, err)
	cfg := writeConfig(t, "bee:\n  platforms:\n    discord:\n      enabled: true\n")

	out, err := run(t, reg, "remove", "discord", "-c", cfg)
	require.NoError(t, err)
	assert.Contains(t, out, strings.Split(i18n.M.Output.Plugin.Removed, "%s")[0])
	assert.Contains(t, out, strings.Split(i18n.M.Output.Plugin.StillEnabled, "%s")[0])
	_, ok, err := reg.Get("discord")
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestPluginRemove_NotInstalled(t *testing.T) {
	_, err := run(t, newRegistry(t), "remove", "nope")
	require.Error(t, err)
	assert.ErrorIs(t, err, plugin.ErrNotInstalled)
}

func firstLine(s string) string {
	return strings.SplitN(s, "\n", 2)[0]
}

func TestPluginList_YAMLStyleEnabled(t *testing.T) {
	reg := newRegistry(t)
	_, err := run(t, reg, "install", writePlugin(t, "discord", "Discord", "0.1.0"))
	require.NoError(t, err)
	_, err = run(t, reg, "install", writePlugin(t, "slack", "Slack", "0.2.0"))
	require.NoError(t, err)
	cfg := writeConfig(t, "bee:\n  platforms:\n    discord:\n      enabled: yes\n    slack:\n      enabled: maybe\n")

	out, err := run(t, reg, "list", "-c", cfg)
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(out), "\n")
	require.Len(t, lines, 3)
	assert.Contains(t, lines[1], i18n.M.Output.Plugin.StatusEnabled)
	assert.Contains(t, lines[2], i18n.M.Output.Plugin.StatusInvalid)
}
