package utils

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

const maxDownloadBytes = 512 * 1024 * 1024 // 512 MB guard against runaway responses

// DownloadFile fetches url and writes the response body to dest.
// If extra is non-nil, all downloaded bytes are also written to it (e.g. for hashing).
func DownloadFile(url, dest string, extra io.Writer) error {
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
