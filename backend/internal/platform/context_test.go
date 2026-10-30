package platform_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/theopenbee/openbee/internal/platform"
)

func TestExtractContext_Registered(t *testing.T) {
	platform.RegisterExtractor("testplatform", func(_ string) string {
		return `{"testplatform":{"key":"value"}}`
	})
	got := platform.ExtractContext("testplatform", "ignored-raw")
	assert.Equal(t, `{"testplatform":{"key":"value"}}`, got)
}

func TestExtractContext_Unregistered(t *testing.T) {
	got := platform.ExtractContext("no-such-platform", "{}")
	assert.Empty(t, got)
}

func TestUnregisterExtractor(t *testing.T) {
	platform.RegisterExtractor("removable", func(_ string) string { return `{"removable":1}` })
	assert.NotEmpty(t, platform.ExtractContext("removable", "raw"))
	platform.UnregisterExtractor("removable")
	assert.Empty(t, platform.ExtractContext("removable", "raw"))
}
