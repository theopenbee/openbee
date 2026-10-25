package plugin

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const validManifest = `{
  "protocol_version": 1,
  "id": "discord",
  "name": {"en": "Discord", "zh": "Discord"},
  "description": "Discord bot channel",
  "command": ["node", "dist/index.js"],
  "config": [
    {"key": "token", "type": "secret", "required": true, "label": {"en": "Bot token", "zh": "机器人 Token"}},
    {"key": "allowed_guilds", "type": "list"},
    {"key": "max_media_size", "type": "int", "default": 10485760}
  ]
}`

func writePlugin(t *testing.T, dir, manifest string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ManifestFile), []byte(manifest), 0o644))
}

func TestLoadManifest_Valid(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, validManifest)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"version":"0.3.1"}`), 0o644))

	m, err := LoadManifest(dir)
	require.NoError(t, err)
	assert.Equal(t, "discord", m.ID)
	assert.Equal(t, "0.3.1", m.Version)
	assert.Equal(t, "Discord", m.Name.Get("zh"))
	assert.Equal(t, []string{"node", "dist/index.js"}, m.Command)
	assert.True(t, filepath.IsAbs(m.Dir))
	require.Len(t, m.Config, 3)
	assert.Equal(t, FieldSecret, m.Config[0].Type)
	assert.True(t, m.Config[0].Required)
	assert.Equal(t, "机器人 Token", m.Config[0].Label.Get("zh"))
}

func TestLoadManifest_VersionFromManifestWithoutPackageJSON(t *testing.T) {
	dir := t.TempDir()
	var raw map[string]any
	require.NoError(t, json.Unmarshal([]byte(validManifest), &raw))
	raw["version"] = "1.2.3"
	b, _ := json.Marshal(raw)
	writePlugin(t, dir, string(b))

	m, err := LoadManifest(dir)
	require.NoError(t, err)
	assert.Equal(t, "1.2.3", m.Version)
}

func TestLoadManifest_Missing(t *testing.T) {
	_, err := LoadManifest(t.TempDir())
	require.Error(t, err)
	assert.Contains(t, err.Error(), ManifestFile)
}

func TestLocalizedText_StringOrObject(t *testing.T) {
	var a LocalizedText
	require.NoError(t, json.Unmarshal([]byte(`"Bot token"`), &a))
	assert.Equal(t, "Bot token", a.Get("zh"))

	var b LocalizedText
	require.NoError(t, json.Unmarshal([]byte(`{"en":"A","zh":"甲"}`), &b))
	assert.Equal(t, "甲", b.Get("zh"))
	assert.Equal(t, "A", b.Get("fr"))

	var empty LocalizedText
	assert.Equal(t, "", empty.Get("en"))
}

func TestManifest_DisplayName(t *testing.T) {
	m := Manifest{ID: "discord"}
	assert.Equal(t, "discord", m.DisplayName("zh"))
	m.Name = LocalizedText{"en": "Discord"}
	assert.Equal(t, "Discord", m.DisplayName("zh"))
}

func TestManifestValidate(t *testing.T) {
	base := func() Manifest {
		return Manifest{
			ProtocolVersion: 1,
			ID:              "discord",
			Command:         []string{"node", "index.js"},
			Config:          []ConfigField{{Key: "token", Type: FieldSecret}},
		}
	}
	cases := []struct {
		name    string
		mutate  func(m *Manifest)
		wantErr string
	}{
		{"valid", func(m *Manifest) {}, ""},
		{"protocol version", func(m *Manifest) { m.ProtocolVersion = 2 }, "protocol_version"},
		{"uppercase id", func(m *Manifest) { m.ID = "Discord" }, "id"},
		{"reserved id", func(m *Manifest) { m.ID = "weixin" }, "reserved"},
		{"empty command", func(m *Manifest) { m.Command = nil }, "command"},
		{"reserved key enabled", func(m *Manifest) { m.Config = append(m.Config, ConfigField{Key: "enabled", Type: FieldBool}) }, "enabled"},
		{"reserved key bot_name", func(m *Manifest) { m.Config = append(m.Config, ConfigField{Key: "bot_name", Type: FieldString}) }, "bot_name"},
		{"duplicate key", func(m *Manifest) { m.Config = append(m.Config, ConfigField{Key: "token", Type: FieldString}) }, "duplicate"},
		{"unknown type", func(m *Manifest) { m.Config = append(m.Config, ConfigField{Key: "ratio", Type: "float"}) }, "float"},
		{"empty key", func(m *Manifest) { m.Config = append(m.Config, ConfigField{Key: "", Type: FieldString}) }, "key"},
		{"random default on secret", func(m *Manifest) {
			m.Config = append(m.Config, ConfigField{Key: "code", Type: FieldSecret, RandomDefault: true})
		}, "random_default"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := base()
			tc.mutate(&m)
			err := m.Validate()
			if tc.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

func TestValidateID(t *testing.T) {
	for _, ok := range []string{"discord", "slack-bot", "qq_2", "ab"} {
		assert.NoError(t, ValidateID(ok), ok)
	}
	for _, bad := range []string{"", "a", "2fa", "has:colon", "UPPER", "local", "weixin", "abcdefghijklmnopqrstuvwxyz0123456"} {
		assert.Error(t, ValidateID(bad), bad)
	}
}

func TestLoadManifest_RandomDefault(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, `{"protocol_version":1,"id":"chat","command":["node","x.js"],"config":[{"key":"code","type":"string","random_default":true}]}`)
	m, err := LoadManifest(dir)
	require.NoError(t, err)
	require.Len(t, m.Config, 1)
	assert.True(t, m.Config[0].RandomDefault)
}
