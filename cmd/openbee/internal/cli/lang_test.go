package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDetectLang_default(t *testing.T) {
	assert.Equal(t, "en", DetectLang())
}
