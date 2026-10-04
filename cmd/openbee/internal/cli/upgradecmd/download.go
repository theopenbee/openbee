package upgradecmd

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/theopenbee/openbee/internal/infra/i18n"
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

// parseChecksumFile looks up assetName in a sha256sum-style checksum file and returns
// the expected hex digest. Lines are "hash  name" (text mode) or "hash *name" (binary
// mode, as written by sha256sum -b). Returns an error if the entry is not found.
func parseChecksumFile(data []byte, assetName string) (string, error) {
	for line := range strings.SplitSeq(string(data), "\n") {
		parts := strings.Fields(line)
		if len(parts) == 2 && strings.TrimPrefix(parts[1], "*") == assetName {
			return parts[0], nil
		}
	}
	return "", fmt.Errorf("no checksum for %s found", assetName)
}

// verifyChecksum checks sum (a raw SHA256 digest) against assetName's entry in
// checksums. Hex digits are compared case-insensitively.
func verifyChecksum(checksums []byte, assetName string, sum []byte) error {
	expected, err := parseChecksumFile(checksums, assetName)
	if err != nil {
		return fmt.Errorf("%w in checksums.txt", err)
	}
	if actual := hex.EncodeToString(sum); !strings.EqualFold(actual, expected) {
		return fmt.Errorf("SHA256 mismatch\n  expected: %s\n  got:      %s", expected, actual)
	}
	return nil
}

// fetchVerifiedArchive downloads checksums.txt and archiveName from relBase into dir,
// verifies the archive's SHA256, and returns the archive path. Any failure aborts,
// including checksums.txt being unavailable: an unverified archive is never returned.
func fetchVerifiedArchive(relBase, archiveName, dir string) (string, error) {
	// Download checksums first (small file), then the archive while hashing it.
	// This avoids a second read of the archive for checksum verification.
	checksumPath := filepath.Join(dir, "checksums.txt")
	if err := downloadFile(relBase+"/checksums.txt", checksumPath, nil); err != nil {
		return "", fmt.Errorf("download checksums.txt: %w", err)
	}

	h := sha256.New()
	archivePath := filepath.Join(dir, archiveName)
	if err := downloadFile(relBase+"/"+archiveName, archivePath, h); err != nil {
		return "", fmt.Errorf("download: %w", err)
	}

	fmt.Println(i18n.M.Output.Upgrade.Verifying)
	checksums, err := os.ReadFile(checksumPath)
	if err != nil {
		return "", fmt.Errorf("read checksums: %w", err)
	}
	if err := verifyChecksum(checksums, archiveName, h.Sum(nil)); err != nil {
		return "", err
	}
	fmt.Println(i18n.M.Output.Upgrade.Verified)
	return archivePath, nil
}
