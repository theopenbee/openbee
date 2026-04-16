package api

import (
	"testing"
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
		{"empty string is invalid", "", true},
		{"unknown engine is invalid", "gpt-4", true},
		{"case sensitive", "Claude", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateEngine(tt.engine)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateEngine(%q) error = %v, wantErr %v", tt.engine, err, tt.wantErr)
			}
		})
	}
}
