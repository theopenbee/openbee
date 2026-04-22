package platform_test

import (
	"testing"

	"github.com/theopenbee/openbee/internal/platform"
)

func TestExtractContext_Registered(t *testing.T) {
	platform.RegisterExtractor("testplatform", func(_ string) string {
		return `{"testplatform":{"key":"value"}}`
	})
	got := platform.ExtractContext("testplatform", "ignored-raw")
	if got != `{"testplatform":{"key":"value"}}` {
		t.Errorf("unexpected result: %q", got)
	}
}

func TestExtractContext_Unregistered(t *testing.T) {
	got := platform.ExtractContext("no-such-platform", "{}")
	if got != "" {
		t.Errorf("expected empty string for unregistered platform, got %q", got)
	}
}
