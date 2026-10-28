package model

import (
	"fmt"
	"strings"
	"time"
	"unicode"
)

func SetupLink(line string) bool {
	rest, ok := strings.CutPrefix(strings.TrimSpace(line), "http")
	if !ok {
		return false
	}
	rest = strings.TrimPrefix(rest, "s")
	rest, ok = strings.CutPrefix(rest, "://")
	return ok && rest != "" && !strings.ContainsFunc(rest, unicode.IsSpace)
}

func LatestLink(lines []string) string {
	for i := len(lines) - 1; i >= 0; i-- {
		if SetupLink(lines[i]) {
			return strings.TrimSpace(lines[i])
		}
	}
	return ""
}

func LastOutputLines(lines []string, n int) []string {
	var out []string
	for _, line := range lines {
		if !SetupLink(line) {
			out = append(out, line)
		}
	}
	if n <= 0 {
		return nil
	}
	if len(out) > n {
		out = out[len(out)-n:]
	}
	return out
}

func Elapsed(start, now time.Time) string {
	if start.IsZero() {
		return "0:00"
	}
	total := max(int64(now.Sub(start)/time.Second), 0)
	return fmt.Sprintf("%d:%02d", total/60, total%60)
}
