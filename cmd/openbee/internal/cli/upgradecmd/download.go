package upgradecmd

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const maxDownloadBytes = 512 * 1024 * 1024 // 512 MB guard against runaway responses

// downloadFile fetches url and writes the response body to dest.
// If extra is non-nil, all downloaded bytes are also written to it (e.g. for hashing).
func downloadFile(url, dest string, extra io.Writer) error {
	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, url)
	}
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()
	w := io.Writer(f)
	if extra != nil {
		w = io.MultiWriter(f, extra)
	}
	n, err := io.Copy(w, io.LimitReader(resp.Body, maxDownloadBytes))
	if err != nil {
		return err
	}
	if n == maxDownloadBytes {
		return fmt.Errorf("download exceeded %d byte limit", maxDownloadBytes)
	}
	return nil
}

// parseChecksumFile looks up assetName in a checksum file (one "hash  filename" pair per line)
// and returns the expected hex digest. Returns an error if the entry is not found.
func parseChecksumFile(data []byte, assetName string) (string, error) {
	for line := range strings.SplitSeq(string(data), "\n") {
		if parts := strings.Fields(line); len(parts) == 2 && parts[1] == assetName {
			return parts[0], nil
		}
	}
	return "", fmt.Errorf("no checksum for %s found", assetName)
}
