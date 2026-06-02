package cli

import "testing"

func TestDetectLang_default(t *testing.T) {
	if got := DetectLang(); got != "en" {
		t.Errorf("DetectLang default: got %q, want en", got)
	}
}
