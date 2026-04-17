package api

import (
	"testing"

	ai "github.com/theopenbee/openbee/internal/ai"
)

func TestValidateEngine(t *testing.T) {
	tests := []struct {
		name    string
		engine  string
		wantErr bool
	}{
		{"claude is valid", "claude", false},
		{"codex is valid", "codex", false},
		{"pi is valid", "pi", false},
		{"kimi is valid", "kimi", false},
		{"empty string is invalid", "", true},
		{"unknown engine is invalid", "gpt-4", true},
		{"case sensitive", "Claude", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ai.ValidateEngine(tt.engine)
			if (err != nil) != tt.wantErr {
				t.Errorf("ai.ValidateEngine(%q) error = %v, wantErr %v", tt.engine, err, tt.wantErr)
			}
		})
	}
}

func TestWorkerHandlerEngineEnabled(t *testing.T) {
	h := &WorkerHandler{
		enabledEngines: map[string]bool{"claude": true},
	}

	tests := []struct {
		name    string
		engine  string
		enabled bool
	}{
		{"claude is enabled", "claude", true},
		{"codex is not enabled", "codex", false},
		{"pi is not enabled", "pi", false},
		{"kimi is not enabled", "kimi", false},
		{"unknown engine is not enabled", "gpt-4", false},
		{"empty string is not enabled", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := h.enabledEngines[tt.engine]
			if got != tt.enabled {
				t.Errorf("enabledEngines[%q] = %v, want %v", tt.engine, got, tt.enabled)
			}
		})
	}
}
