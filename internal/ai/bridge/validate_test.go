package bridge

import (
	"errors"
	"testing"

	ai "github.com/theopenbee/openbee/internal/ai"
)

func TestValidateEngineArgsDelegatesToInternalAI(t *testing.T) {
	if err := ValidateEngineArgs(`--ok "value"`); err != nil {
		t.Fatalf("expected ok, got %v", err)
	}
	if err := ValidateEngineArgs(`--bad "unterminated`); err == nil {
		t.Fatalf("expected error for unterminated quote")
	}
}

func TestValidateEngineAllowsEmptyAndChecksEnabled(t *testing.T) {
	b := &bridgeImpl{engines: map[string]ai.EngineAdapter{ai.EngineClaude: nil}}
	if err := b.ValidateEngine(""); err != nil {
		t.Fatalf("empty should be ok: %v", err)
	}
	if err := b.ValidateEngine(ai.EngineClaude); err != nil {
		t.Fatalf("enabled should be ok: %v", err)
	}
	err := b.ValidateEngine(ai.EngineCodex)
	if err == nil {
		t.Fatalf("disabled engine should error")
	}
	if !errors.Is(err, ErrEngineNotEnabled) {
		t.Fatalf("expected ErrEngineNotEnabled, got %v", err)
	}
}
