package codex_test

import (
	"testing"

	ai "github.com/theopenbee/openbee/internal/ai"
	"github.com/theopenbee/openbee/internal/ai/engine/codex"
)

func TestAdapter_ExtraEnvInBaseEnv(t *testing.T) {
	a, err := codex.NewAdapter("echo", map[string]string{
		"CODEX_CUSTOM": "value",
	})
	if err != nil {
		t.Fatalf("NewAdapter: %v", err)
	}
	var _ ai.EngineAdapter = a
}
