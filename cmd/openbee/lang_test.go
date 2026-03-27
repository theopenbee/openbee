package main

import (
	"os"
	"testing"
)

func TestParseLangFlag(t *testing.T) {
	tests := []struct {
		args []string
		want string
	}{
		{[]string{"openbee", "--lang", "en", "config"}, "en"},
		{[]string{"openbee", "--lang=zh", "server"}, "zh"},
		{[]string{"openbee", "server"}, ""},
		{[]string{}, ""},
	}
	for _, tt := range tests {
		if got := parseLangFlag(tt.args); got != tt.want {
			t.Errorf("parseLangFlag(%v) = %q, want %q", tt.args, got, tt.want)
		}
	}
}

func TestDetectLang_flag(t *testing.T) {
	os.Unsetenv("OPENBEE_LANG")
	if got := detectLang("en"); got != "en" {
		t.Errorf("detectLang flag: got %q, want en", got)
	}
}

func TestDetectLang_env(t *testing.T) {
	os.Setenv("OPENBEE_LANG", "en")
	defer os.Unsetenv("OPENBEE_LANG")
	if got := detectLang(""); got != "en" {
		t.Errorf("detectLang env: got %q, want en", got)
	}
}

func TestDetectLang_default(t *testing.T) {
	os.Unsetenv("OPENBEE_LANG")
	if got := detectLang(""); got != "zh" {
		t.Errorf("detectLang default: got %q, want zh", got)
	}
}
