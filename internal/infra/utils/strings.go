package utils

import "strings"

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
