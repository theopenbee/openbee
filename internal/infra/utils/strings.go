package utils

import (
	"strings"
	"unicode/utf8"
)

// SplitAndTrim splits a comma-separated string, trims whitespace from each part,
// and returns only non-empty parts.
func SplitAndTrim(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// TruncateRunes returns s unchanged when it is at most maxContentRunes runes;
// otherwise it returns the first maxContentRunes runes followed by "…". The
// ellipsis is appended on top, so a truncated result is one rune longer than
// maxContentRunes — callers with a hard ceiling should size accordingly.
// Slices by rune so multi-byte UTF-8 characters are not split.
func TruncateRunes(s string, maxContentRunes int) string {
	if utf8.RuneCountInString(s) <= maxContentRunes {
		return s
	}
	return string([]rune(s)[:maxContentRunes]) + "…"
}
