package pluginchannel

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/theopenbee/openbee/internal/plugin"
)

func installTestPlugin(t *testing.T, reg *plugin.Registry, registryID, manifestID string) string {
	t.Helper()
	dir := t.TempDir()
	m := map[string]any{
		"protocol_version": 1,
		"id":               manifestID,
		"command":          fakePluginCommand(),
		"config": []map[string]any{
			{"key": "token", "type": "secret", "required": true},
			{"key": "max_media_size", "type": "int", "default": 1024},
		},
	}
	b, err := json.Marshal(m)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, plugin.ManifestFile), b, 0o644))
	require.NoError(t, reg.Put(plugin.Entry{ID: registryID, Source: plugin.SourcePath, Path: dir}))
	return dir
}

func TestBuildChannels_SkipsBrokenSections(t *testing.T) {
	reg := plugin.OpenRegistry(filepath.Join(t.TempDir(), "plugins"))
	goodDir := installTestPlugin(t, reg, "good", "good")
	installTestPlugin(t, reg, "incomplete", "incomplete")
	installTestPlugin(t, reg, "mismatch", "other")
	require.NoError(t, reg.Put(plugin.Entry{ID: "vanished", Source: plugin.SourcePath, Path: filepath.Join(t.TempDir(), "gone")}))
	svc := newTestMedia(t)

	built := BuildChannels(map[string]map[string]any{
		"good":       {"enabled": true, "bot_name": "GoodBot", "token": "x"},
		"missing":    {"enabled": true, "token": "x"},
		"incomplete": {"enabled": true},
		"mismatch":   {"enabled": true, "token": "x"},
		"vanished":   {"enabled": true, "token": "x"},
		"off":        {"enabled": false, "token": "x"},
		"stringy":    {"enabled": "true", "token": "x"},
	}, BuildOptions{Registry: reg, MediaSvc: svc, Language: "zh"})

	require.Len(t, built, 1)
	ch := built[0]
	assert.Equal(t, "good", ch.Channel.Platform.ID())
	require.NotNil(t, ch.Channel.ExtractContext)
	assert.NotNil(t, ch.Done)

	p, ok := ch.Channel.Platform.(*Platform)
	require.True(t, ok)
	spec := p.sup.spec
	assert.Equal(t, goodDir, spec.Dir)
	assert.Equal(t, fakePluginCommand(), spec.Command)
	assert.Equal(t, 1, spec.Init.ProtocolVersion)
	assert.Equal(t, "good", spec.Init.ChannelID)
	assert.Equal(t, "GoodBot", spec.Init.BotName)
	assert.Equal(t, "zh", spec.Init.Language)
	assert.Equal(t, svc.InboundDir(), spec.Init.MediaDir)
	assert.Equal(t, map[string]any{"token": "x", "max_media_size": 1024}, spec.Init.Config)
	assert.Equal(t, reg.DataDir("good"), spec.Init.DataDir)
	info, err := os.Stat(spec.Init.DataDir)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o700), info.Mode().Perm())
}

func TestBuildChannels_SortedAndEmpty(t *testing.T) {
	reg := plugin.OpenRegistry(filepath.Join(t.TempDir(), "plugins"))
	installTestPlugin(t, reg, "zeta", "zeta")
	installTestPlugin(t, reg, "alpha", "alpha")
	built := BuildChannels(map[string]map[string]any{
		"zeta":  {"enabled": true, "token": "x"},
		"alpha": {"enabled": true, "token": "x"},
	}, BuildOptions{Registry: reg, MediaSvc: newTestMedia(t)})
	require.Len(t, built, 2)
	assert.Equal(t, "alpha", built[0].Channel.Platform.ID())
	assert.Equal(t, "zeta", built[1].Channel.Platform.ID())

	assert.Empty(t, BuildChannels(nil, BuildOptions{Registry: reg, MediaSvc: newTestMedia(t)}))
}

func TestBuildChannels_YAMLStyleEnabled(t *testing.T) {
	reg := plugin.OpenRegistry(filepath.Join(t.TempDir(), "plugins"))
	installTestPlugin(t, reg, "yesplug", "yesplug")
	installTestPlugin(t, reg, "badplug", "badplug")
	built := BuildChannels(map[string]map[string]any{
		"yesplug": {"enabled": "yes", "token": "x"},
		"badplug": {"enabled": "maybe", "token": "x"},
	}, BuildOptions{Registry: reg, MediaSvc: newTestMedia(t)})
	require.Len(t, built, 1)
	assert.Equal(t, "yesplug", built[0].Channel.Platform.ID())
}
