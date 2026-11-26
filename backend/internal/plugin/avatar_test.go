package plugin

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const avatarManifest = `{
  "protocol_version": 1,
  "type": "avatar",
  "id": "mfers",
  "name": "mfers",
  "avatars": "avatars"
}`

func writeAvatarPlugin(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	writePlugin(t, dir, avatarManifest)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "avatars"), 0o755))
	for name, body := range files {
		require.NoError(t, os.WriteFile(filepath.Join(dir, "avatars", name), []byte(body), 0o644))
	}
	return dir
}

func TestManifest_AvatarValid(t *testing.T) {
	dir := writeAvatarPlugin(t, map[string]string{"00001.webp": "x"})

	m, err := LoadManifest(dir)
	require.NoError(t, err)
	assert.Equal(t, TypeAvatar, m.Kind())
	assert.Equal(t, "avatars", m.Avatars)
	assert.Empty(t, m.Command)
}

func TestManifest_AvatarRules(t *testing.T) {
	base := Manifest{ProtocolVersion: AvatarProtocolVersion, Type: TypeAvatar, ID: "mfers", Avatars: "avatars"}
	require.NoError(t, base.Validate())

	cases := map[string]func(m *Manifest){
		"cannot declare command": func(m *Manifest) { m.Command = []string{"node", "index.js"} },
		"cannot declare setup":   func(m *Manifest) { m.Setup = &Setup{Command: []string{"node"}} },
		"cannot declare config":  func(m *Manifest) { m.Config = []ConfigField{{Key: "k", Type: FieldString}} },
		"must declare avatars":   func(m *Manifest) { m.Avatars = "" },
		"relative path":          func(m *Manifest) { m.Avatars = "../elsewhere" },
		"protocol_version":       func(m *Manifest) { m.ProtocolVersion = AvatarProtocolVersion + 1 },
	}
	for want, mutate := range cases {
		m := base
		mutate(&m)
		err := m.Validate()
		require.Error(t, err, want)
		assert.Contains(t, err.Error(), want)
	}
}

func TestManifest_OnlyAvatarPluginsDeclareAvatars(t *testing.T) {
	m := Manifest{ProtocolVersion: EngineProtocolVersion, Type: TypeEngine, ID: "claude", Command: []string{"node"}, Avatars: "avatars"}
	err := m.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "avatars")
}

func TestLoadAvatars_SortedWithContentTypes(t *testing.T) {
	dir := writeAvatarPlugin(t, map[string]string{
		"00002.png":  "p",
		"00001.webp": "w",
		"00003.JPG":  "j",
		".DS_Store":  "ignored",
	})
	m, err := LoadManifest(dir)
	require.NoError(t, err)

	got, err := LoadAvatars(m)
	require.NoError(t, err)
	require.Len(t, got, 3)
	assert.Equal(t, []string{"00001", "00002", "00003"}, []string{got[0].Name, got[1].Name, got[2].Name})
	assert.Equal(t, "image/webp", got[0].ContentType)
	assert.Equal(t, "image/png", got[1].ContentType)
	assert.Equal(t, "image/jpeg", got[2].ContentType)
	real, err := filepath.EvalSymlinks(filepath.Join(m.Dir, "avatars", "00001.webp"))
	require.NoError(t, err)
	assert.Equal(t, real, got[0].Path)
}

func TestLoadAvatars_Rejects(t *testing.T) {
	cases := map[string]struct {
		files map[string]string
		want  string
	}{
		"empty":     {map[string]string{}, "no images"},
		"bad type":  {map[string]string{"a.gif": "x"}, ".webp, .png or .jpg"},
		"bad name":  {map[string]string{"Big Face.png": "x"}, "invalid avatar name"},
		"duplicate": {map[string]string{"a.png": "x", "a.webp": "y"}, "duplicate avatar name"},
		"too large": {map[string]string{"a.png": strings.Repeat("x", MaxAvatarSize+1)}, "larger than"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			dir := writeAvatarPlugin(t, tc.files)
			m, err := LoadManifest(dir)
			require.NoError(t, err)
			_, err = LoadAvatars(m)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}

func TestLoadAvatars_RejectsSymlinkAndMissingFolder(t *testing.T) {
	dir := writeAvatarPlugin(t, map[string]string{"a.png": "x"})
	outside := filepath.Join(t.TempDir(), "secret.png")
	require.NoError(t, os.WriteFile(outside, []byte("s"), 0o644))
	require.NoError(t, os.Symlink(outside, filepath.Join(dir, "avatars", "b.png")))
	m, err := LoadManifest(dir)
	require.NoError(t, err)
	_, err = LoadAvatars(m)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a regular file")

	require.NoError(t, os.RemoveAll(filepath.Join(dir, "avatars")))
	_, err = LoadAvatars(m)
	require.Error(t, err)
}

func TestLoadAvatars_RejectsFolderLinkedOutside(t *testing.T) {
	dir := writeAvatarPlugin(t, map[string]string{"a.png": "x"})
	outside := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(outside, "b.png"), []byte("b"), 0o644))
	require.NoError(t, os.RemoveAll(filepath.Join(dir, "avatars")))
	require.NoError(t, os.Symlink(outside, filepath.Join(dir, "avatars")))
	m, err := LoadManifest(dir)
	require.NoError(t, err)

	_, err = LoadAvatars(m)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "inside the plugin")
}

func TestLoadAvatars_FolderLinkedInsideIsFine(t *testing.T) {
	dir := writeAvatarPlugin(t, map[string]string{})
	require.NoError(t, os.RemoveAll(filepath.Join(dir, "avatars")))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "images"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "images", "a.png"), []byte("a"), 0o644))
	require.NoError(t, os.Symlink("images", filepath.Join(dir, "avatars")))
	m, err := LoadManifest(dir)
	require.NoError(t, err)

	got, err := LoadAvatars(m)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "a", got[0].Name)
}

func TestParseAvatarRef(t *testing.T) {
	id, name, ok := ParseAvatarRef("mfers/00042")
	require.True(t, ok)
	assert.Equal(t, "mfers", id)
	assert.Equal(t, "00042", name)
	assert.Equal(t, "mfers/00042", AvatarRef(id, name))

	for _, bad := range []string{"", "mfers", "mfers/", "/00042", "mfers/../x", "mfers/a/b", "Mfers/1", "mfers/A"} {
		_, _, ok := ParseAvatarRef(bad)
		assert.False(t, ok, bad)
	}
}
