package ai

import (
	"bufio"
	"io"
	"strings"
)

// ScanJSONLines reads r line by line and calls fn for each line that starts
// with '{'. fn returns true to keep scanning or false to stop early.
func ScanJSONLines(r io.Reader, fn func(line string) bool) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(nil, 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "{") && !fn(line) {
			return
		}
	}
}
