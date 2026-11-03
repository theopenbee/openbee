package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnsureFile_CreatesLoadableConfig(t *testing.T) {
	dir := filepath.Join(t.TempDir(), ".openbee")
	path := filepath.Join(dir, "config.yaml")

	created, err := EnsureFile(path, "zh")
	require.NoError(t, err)
	assert.True(t, created)

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())

	cfg, err := Load(path)
	require.NoError(t, err)
	assert.Equal(t, "zh", cfg.Language)
	assert.Equal(t, filepath.Join(dir, "data", "openbee.db"), cfg.Database.Path)
	assert.Equal(t, 8080, cfg.Server.Port)
	assert.Equal(t, "localhost", cfg.Server.Host)
	assert.Equal(t, 5*time.Minute, cfg.Bee.Engine.Timeout.Bee)
	assert.Equal(t, 30*time.Minute, cfg.Bee.Engine.Timeout.Worker)
	assert.False(t, cfg.Bee.RPC.TokenSecretGenerated)
	assert.False(t, cfg.Server.EnvSecretGenerated)
	assert.NotEmpty(t, cfg.Server.Auth.JWTSecret)
	secrets := []string{cfg.Server.Auth.JWTSecret, cfg.Bee.RPC.TokenSecret, cfg.Server.EnvSecret}
	assert.Len(t, map[string]bool{secrets[0]: true, secrets[1]: true, secrets[2]: true}, 3)
}

func TestEnsureFile_KeepsExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte("language: zh\n"), 0o644))

	created, err := EnsureFile(path, "en")
	require.NoError(t, err)
	assert.False(t, created)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "language: zh\n", string(data))
}

func TestEnsureFile_OmitsWebCredentials(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	_, err := EnsureFile(path, "en")
	require.NoError(t, err)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.NotContains(t, string(data), "username:")
	assert.NotContains(t, string(data), "password:")
}

func TestPrepare_GeneratesOnlyAtDefaultPath(t *testing.T) {
	dir := t.TempDir()
	defaultPath := filepath.Join(dir, ".openbee", "config.yaml")
	other := filepath.Join(dir, "elsewhere", "config.yaml")
	getenv := func(string) string { return "" }

	created, err := prepare(other, defaultPath, getenv)
	require.NoError(t, err)
	assert.False(t, created)
	assert.NoFileExists(t, other)

	created, err = prepare(defaultPath, defaultPath, getenv)
	require.NoError(t, err)
	assert.True(t, created)
	assert.FileExists(t, defaultPath)
}

func TestDetectLanguage(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
		want string
	}{
		{"unset", nil, "en"},
		{"lang chinese", map[string]string{"LANG": "zh_CN.UTF-8"}, "zh"},
		{"lang traditional chinese", map[string]string{"LANG": "zh_TW.UTF-8"}, "zh"},
		{"lang english", map[string]string{"LANG": "en_US.UTF-8"}, "en"},
		{"lc_messages beats lang", map[string]string{"LC_MESSAGES": "zh_CN.UTF-8", "LANG": "en_US.UTF-8"}, "zh"},
		{"lc_all beats lang", map[string]string{"LC_ALL": "C", "LANG": "zh_CN.UTF-8"}, "en"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := DetectLanguage(func(k string) string { return tc.env[k] })
			assert.Equal(t, tc.want, got)
		})
	}
}
