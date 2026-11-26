package pluginmgr

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/theopenbee/openbee/internal/plugin"
)

func writeAvatarManifest(t *testing.T, dir, id string, names ...string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "avatars"), 0o755))
	data, err := json.Marshal(map[string]any{
		"protocol_version": plugin.AvatarProtocolVersion,
		"type":             plugin.TypeAvatar,
		"id":               id,
		"name":             map[string]string{"en": id + " avatars"},
		"avatars":          "avatars",
	})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, plugin.ManifestFile), data, 0o644))
	for _, n := range names {
		require.NoError(t, os.WriteFile(filepath.Join(dir, "avatars", n+".webp"), []byte(n), 0o644))
	}
}

func (h *harness) installAvatars(id string, names ...string) string {
	h.t.Helper()
	dir := filepath.Join(h.reg.dir, "pkgs", id)
	writeAvatarManifest(h.t, dir, id, names...)
	h.reg.put(plugin.Entry{ID: id, Source: plugin.SourcePath, Path: dir, Version: "0.1.0"})
	return dir
}

func (h *harness) loadAvatars() {
	h.t.Helper()
	h.m.LoadAvatars(context.Background())
}

func TestAvatars_InstallAutoEnables(t *testing.T) {
	h := newHarness(t)
	h.start()

	done := h.waitOp(h.m.Install("@theopenbee/avatars-mfers@0.1.0").ID)
	require.Equal(t, OpSucceeded, done.Status)
	h.waitState("mfers", StateReady)

	st := h.state("mfers")
	assert.True(t, st.Enabled)
	assert.Equal(t, plugin.TypeAvatar, st.Type)
	s, found := h.settings("mfers")
	require.True(t, found)
	assert.True(t, s.Enabled)
	assert.Equal(t, []AvatarPack{{ID: "mfers", Name: plugin.LocalizedText{"en": "mfers avatars"}, Avatars: []string{"mfers/a", "mfers/b"}}}, h.m.AvatarPacks())
	assert.Equal(t, 0, h.factory.startCount("mfers"))
	_, ok := h.senders.Get("mfers")
	assert.False(t, ok)
}

func TestAvatars_LoadOnlyEnabledPacks(t *testing.T) {
	h := newHarness(t)
	h.installAvatars("on", "x")
	h.installAvatars("off", "y")
	h.install("discord")
	h.save("on", true, "", map[string]any{})
	h.save("off", false, "", map[string]any{})
	h.save("discord", true, "", map[string]any{"token": "t"})

	h.loadAvatars()

	assert.True(t, h.m.HasAvatar("on/x"))
	assert.False(t, h.m.HasAvatar("off/y"))
	assert.Equal(t, StateReady, h.state("on").State)
	assert.Equal(t, StateDisabled, h.state("off").State)
	assert.Equal(t, 0, h.factory.startCount("discord"))
}

func TestAvatars_StartSkipsAvatarPacks(t *testing.T) {
	h := newHarness(t)
	h.installAvatars("mfers", "a")
	h.save("mfers", true, "", map[string]any{})
	h.start()

	assert.False(t, h.m.HasAvatar("mfers/a"))
	h.loadAvatars()
	assert.True(t, h.m.HasAvatar("mfers/a"))
}

func TestAvatars_DisableEnableRestart(t *testing.T) {
	h := newHarness(t)
	dir := h.installAvatars("mfers", "a")
	h.save("mfers", true, "", map[string]any{})
	h.loadAvatars()
	ctx := context.Background()

	require.NoError(t, h.m.Disable(ctx, "mfers"))
	assert.False(t, h.m.HasAvatar("mfers/a"))
	assert.Empty(t, h.m.AvatarPacks())
	assert.Equal(t, StateDisabled, h.state("mfers").State)

	require.NoError(t, h.m.Enable(ctx, "mfers"))
	assert.True(t, h.m.HasAvatar("mfers/a"))

	require.NoError(t, os.WriteFile(filepath.Join(dir, "avatars", "b.png"), []byte("b"), 0o644))
	require.NoError(t, h.m.Restart(ctx, "mfers"))
	assert.True(t, h.m.HasAvatar("mfers/b"))
}

func TestAvatars_EnableWithoutSettingsSavesDefaults(t *testing.T) {
	h := newHarness(t)
	h.installAvatars("mfers", "a")

	require.NoError(t, h.m.Enable(context.Background(), "mfers"))
	assert.True(t, h.m.HasAvatar("mfers/a"))
	s, found := h.settings("mfers")
	require.True(t, found)
	assert.True(t, s.Enabled)
}

func TestAvatars_BadFolderFails(t *testing.T) {
	h := newHarness(t)
	dir := h.installAvatars("mfers", "a")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "avatars", "notes.txt"), []byte("x"), 0o644))
	h.save("mfers", true, "", map[string]any{})
	h.loadAvatars()

	st := h.state("mfers")
	assert.Equal(t, StateFailed, st.State)
	assert.Contains(t, st.Error, "notes.txt")
	assert.False(t, h.m.HasAvatar("mfers/a"))
}

func TestAvatars_UninstallDropsPack(t *testing.T) {
	h := newHarness(t)
	h.start()
	require.Equal(t, OpSucceeded, h.waitOp(h.m.Install("@theopenbee/avatars-mfers@0.1.0").ID).Status)
	h.waitState("mfers", StateReady)

	op, err := h.m.Uninstall(context.Background(), "mfers")
	require.NoError(t, err)
	require.Equal(t, OpSucceeded, h.waitOp(op.ID).Status)
	assert.False(t, h.m.HasAvatar("mfers/a"))
	_, found := h.settings("mfers")
	assert.False(t, found)
}

func TestAvatars_BotNameNotSupported(t *testing.T) {
	h := newHarness(t)
	h.installAvatars("mfers", "a")
	h.save("mfers", true, "", map[string]any{})
	name := "bot"

	err := h.m.UpdateSettings(context.Background(), "mfers", &name, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no bot name")
}

func TestAvatars_FileLookup(t *testing.T) {
	h := newHarness(t)
	dir := h.installAvatars("mfers", "a")
	h.save("mfers", true, "", map[string]any{})
	h.loadAvatars()

	a, ok := h.m.AvatarFile("mfers/a")
	require.True(t, ok)
	real, err := filepath.EvalSymlinks(filepath.Join(dir, "avatars", "a.webp"))
	require.NoError(t, err)
	assert.Equal(t, real, a.Path)
	assert.Equal(t, "image/webp", a.ContentType)

	for _, ref := range []string{"mfers/zz", "other/a", "mfers/../a", "", "mfers"} {
		_, ok := h.m.AvatarFile(ref)
		assert.False(t, ok, ref)
	}
}

func TestAvatars_RandomPrefersUnused(t *testing.T) {
	h := newHarness(t)
	h.installAvatars("p1", "a", "b")
	h.installAvatars("p2", "c")
	h.save("p1", true, "", map[string]any{})
	h.save("p2", true, "", map[string]any{})
	h.loadAvatars()

	used := map[string]bool{"p1/a": true, "p2/c": true}
	for range 20 {
		assert.Equal(t, "p1/b", h.m.RandomAvatar(used))
	}

	used["p1/b"] = true
	seen := map[string]bool{}
	for range 200 {
		seen[h.m.RandomAvatar(used)] = true
	}
	assert.Equal(t, map[string]bool{"p1/a": true, "p1/b": true, "p2/c": true}, seen)
}

func TestAvatars_RandomWithoutPacks(t *testing.T) {
	h := newHarness(t)
	assert.Empty(t, h.m.RandomAvatar(nil))
}
