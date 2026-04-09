package config

import (
	"testing"
)

func TestEngineConfigRaw_Codex(t *testing.T) {
	cfg := BeeConfig{
		Engine: "codex",
		Codex:  CodexConfig{Path: "/usr/local/bin/codex"},
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
	cfg := BeeConfig{Engine: "codex"}
	raw := cfg.EngineConfigRaw()
	if raw == nil {
		t.Fatal("expected non-nil raw config for codex engine with empty path")
	}
	path, _ := raw["path"].(string)
	if path != "" {
		t.Fatalf("expected empty path, got %q", path)
	}
}
