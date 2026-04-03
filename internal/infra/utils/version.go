package utils

import (
	"fmt"
	"strings"
)

// NormalizeVersionTag trims whitespace, validates the tag is non-empty, and
// ensures it carries a "v" prefix (e.g. "1.2.3" → "v1.2.3").
func NormalizeVersionTag(tag string) (string, error) {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return "", fmt.Errorf("empty version tag")
	}
	if !strings.HasPrefix(tag, "v") {
		tag = "v" + tag
	}
	return tag, nil
}
