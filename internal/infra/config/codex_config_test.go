package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEngineConfigRaw_Codex(t *testing.T) {
	cfg := BeeConfig{
		Engine:  EngineDefaultConfig{Default: "codex"},
		Engines: EnginesConfig{Codex: EngineItemConfig{Path: "/usr/local/bin/codex"}},
	}
	raw := cfg.EngineConfigRaw()
	require.NotNil(t, raw)
	path, ok := raw["path"].(string)
	require.True(t, ok)
	require.Equal(t, "/usr/local/bin/codex", path)
}

func TestEngineConfigRaw_CodexEmptyPath(t *testing.T) {
	cfg := BeeConfig{Engine: EngineDefaultConfig{Default: "codex"}}
	raw := cfg.EngineConfigRaw()
	require.Nil(t, raw)
}
