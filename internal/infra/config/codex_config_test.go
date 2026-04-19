package config

import (
	"testing"
)

func TestEngineConfigRaw_Codex(t *testing.T) {
	cfg := BeeConfig{
		Engine:  EngineDefaultConfig{Default: "codex"},
		Engines: EnginesConfig{Codex: EngineItemConfig{Path: "/usr/local/bin/codex"}},
	}
	raw := cfg.EngineConfigRaw()
	if raw == nil {
		t.Fatal("expected non-nil raw config for codex engine")
	}
	path, ok := raw["path"].(string)
	if !ok || path != "/usr/local/bin/codex" {
		t.Fatalf("expected path=/usr/local/bin/codex, got %v", raw["path"])
	}
}

func TestEngineConfigRaw_CodexEmptyPath(t *testing.T) {
	cfg := BeeConfig{Engine: EngineDefaultConfig{Default: "codex"}}
	raw := cfg.EngineConfigRaw()
	if raw != nil {
		t.Fatalf("expected nil raw config for codex engine with empty path, got %v", raw)
	}
}
