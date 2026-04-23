package ai

import (
	"bufio"
	"io"
	"strings"
)

// ScannerBufSize is the max token size for all JSON-line scanners in this package.
const ScannerBufSize = 2 * 1024 * 1024

// ScanJSONLines reads r line by line and calls fn for each line that starts
// with '{'. fn returns true to keep scanning or false to stop early.
func ScanJSONLines(r io.Reader, fn func(line string) bool) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(nil, ScannerBufSize)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "{") && !fn(line) {
			return
		}
	}
}
