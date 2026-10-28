package plugin

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegistry_PutGetListRemove(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "plugins")
	r := OpenRegistry(dir)

	list, err := r.List()
	require.NoError(t, err)
	assert.Empty(t, list)

	require.NoError(t, r.Put(Entry{ID: "slack", Source: SourcePath, Path: "/p/slack", Version: "0.2.0"}))
	require.NoError(t, r.Put(Entry{ID: "discord", Source: SourceNpm, Package: "@x/openbee-discord", Path: "/p/discord", Version: "0.1.0"}))

	info, err := os.Stat(filepath.Join(dir, "plugins.json"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())

	list, err = r.List()
	require.NoError(t, err)
	require.Len(t, list, 2)
	assert.Equal(t, "discord", list[0].ID)
	assert.Equal(t, "slack", list[1].ID)

	got, ok, err := r.Get("discord")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, Entry{ID: "discord", Source: SourceNpm, Package: "@x/openbee-discord", Path: "/p/discord", Version: "0.1.0"}, got)

	require.NoError(t, r.Remove("discord"))
	_, ok, err = r.Get("discord")
	require.NoError(t, err)
	assert.False(t, ok)

	assert.Equal(t, filepath.Join(dir, "data", "slack"), r.DataDir("slack"))
	assert.Equal(t, dir, r.Dir())
}

func TestRegistry_CorruptFile(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "plugins.json"), []byte("{"), 0o600))
	_, err := OpenRegistry(dir).List()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "plugins.json")
}
