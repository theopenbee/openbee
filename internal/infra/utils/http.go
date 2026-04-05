package utils

import (
	"fmt"
	"io"
	"net/http"
	"time"
)

// APIClient is the shared HTTP client for short API/version-check calls.
var APIClient = &http.Client{Timeout: 15 * time.Second}

// FetchPlainTextVersion fetches a plain-text version tag from url and normalizes it.
func FetchPlainTextVersion(url string) (string, error) {
	resp, err := APIClient.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, resp.Body)
		return "", fmt.Errorf("CDN returned %d for %s", resp.StatusCode, url)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 256))
	if err != nil {
		return "", fmt.Errorf("read CDN response: %w", err)
	}
	return NormalizeVersionTag(string(body))
}
