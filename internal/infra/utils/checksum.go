package utils

import (
	"fmt"
	"strings"
)

// ParseChecksumFile looks up assetName in a checksum file (one "hash  filename" pair per line)
// and returns the expected hex digest. Returns an error if the entry is not found.
func ParseChecksumFile(data []byte, assetName string) (string, error) {
	for line := range strings.SplitSeq(string(data), "\n") {
		if parts := strings.Fields(line); len(parts) == 2 && parts[1] == assetName {
			return parts[0], nil
		}
	}
	return "", fmt.Errorf("no checksum for %s found", assetName)
}
