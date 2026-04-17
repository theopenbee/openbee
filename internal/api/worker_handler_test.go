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
