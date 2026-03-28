package main

import (
	"testing"
)

func TestDetectLang_default(t *testing.T) {
	if got := detectLang(); got != "en" {
		t.Errorf("detectLang default: got %q, want en", got)
	}
}
