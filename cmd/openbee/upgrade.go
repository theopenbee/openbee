package main

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

const (
	githubAPILatest = "https://api.github.com/repos/theopenbee/openbee/releases/latest"
	githubRelBase   = "https://github.com/theopenbee/openbee/releases/download"

	upgradeBinaryName    = "openbee"
	upgradeBinaryNameWin = "openbee.exe"
	maxDownloadBytes     = 512 * 1024 * 1024 // 512 MB guard against runaway responses
	executablePerm       = 0o755
)

type githubRelease struct {
	TagName string `json:"tag_name"`
}

var upgradeCheckOnly bool

var upgradeCmd = &cobra.Command{
	Use:   "upgrade",
	Short: "Upgrade openbee to the latest version",
	Long:  "Check for a new version and replace the current binary if one is available.",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runUpgrade(upgradeCheckOnly)
	},
}

func init() {
	upgradeCmd.Flags().BoolVar(&upgradeCheckOnly, "check", false, "check for updates only, do not upgrade")
	rootCmd.AddCommand(upgradeCmd)
}

func runUpgrade(checkOnly bool) error {
	current := version

	fmt.Printf("Current version: %s\n", current)
	fmt.Println("Checking for latest version...")

	latest, err := fetchLatestVersion()
	if err != nil {
		return fmt.Errorf("fetch latest version: %w", err)
	}

	fmt.Printf("Latest version: %s\n", latest)

	if !isNewer(latest, current) {
		fmt.Println("Already up to date.")
		return nil
	}

	fmt.Printf("New version available: %s\n", latest)

	if checkOnly {
		fmt.Printf("Run 'openbee upgrade' to upgrade.\n")
		return nil
	}

	return doUpgrade(latest)
}

func fetchLatestVersion() (string, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(githubAPILatest)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub API returned %d", resp.StatusCode)
	}

	var rel githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return "", fmt.Errorf("parse response: %w", err)
	}

	tag := strings.TrimSpace(rel.TagName)
	if tag == "" {
		return "", fmt.Errorf("empty version tag")
	}
	if !strings.HasPrefix(tag, "v") {
		tag = "v" + tag
	}
	return tag, nil
}

// isNewer returns true when latest is strictly newer than current.
// Falls back to string comparison for non-semver tags (e.g. "dev").
func isNewer(latest, current string) bool {
	lv := parseSemver(latest)
	cv := parseSemver(current)
	if lv == nil || cv == nil {
		return latest != current
	}
	for i := range min(len(lv), len(cv)) {
		if lv[i] > cv[i] {
			return true
		}
		if lv[i] < cv[i] {
			return false
		}
	}
	// All common parts equal: longer version (e.g. 1.0.1 vs 1.0) is newer.
	return len(lv) > len(cv)
}

func parseSemver(v string) []int {
	v = strings.TrimPrefix(v, "v")
	// Drop pre-release / build-metadata suffixes
	v = strings.SplitN(v, "-", 2)[0]
	v = strings.SplitN(v, "+", 2)[0]
	parts := strings.Split(v, ".")
	nums := make([]int, len(parts))
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return nil
		}
		nums[i] = n
	}
	return nums
}

func doUpgrade(newVersion string) error {
	versionNum := strings.TrimPrefix(newVersion, "v")
	archiveName := fmt.Sprintf("%s-%s-%s-%s.tar.gz", upgradeBinaryName, versionNum, runtime.GOOS, runtime.GOARCH)
	archiveURL := fmt.Sprintf("%s/%s/%s", githubRelBase, newVersion, archiveName)
	checksumURL := fmt.Sprintf("%s/%s/checksums.txt", githubRelBase, newVersion)

	fmt.Printf("Downloading %s...\n", archiveName)

	tmpDir, err := os.MkdirTemp("", "openbee-upgrade-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// Download checksums first (small file), then the archive while hashing it.
	// This avoids a second read of the archive for checksum verification.
	checksumPath := filepath.Join(tmpDir, "checksums.txt")
	checksumAvailable := true
	if err := downloadFile(checksumURL, checksumPath, nil); err != nil {
		checksumAvailable = false
		fmt.Printf("warning: failed to download checksums.txt, skipping verification (%v)\n", err)
	}

	h := sha256.New()
	archivePath := filepath.Join(tmpDir, archiveName)
	if err := downloadFile(archiveURL, archivePath, h); err != nil {
		return fmt.Errorf("download: %w", err)
	}

	if checksumAvailable {
		fmt.Println("Verifying SHA256...")
		data, err := os.ReadFile(checksumPath)
		if err != nil {
			return fmt.Errorf("read checksums: %w", err)
		}
		var expected string
		for line := range strings.SplitSeq(string(data), "\n") {
			if parts := strings.Fields(line); len(parts) == 2 && parts[1] == archiveName {
				expected = parts[0]
				break
			}
		}
		if expected == "" {
			return fmt.Errorf("no checksum for %s in checksums.txt", archiveName)
		}
		if actual := hex.EncodeToString(h.Sum(nil)); actual != expected {
			return fmt.Errorf("SHA256 mismatch\n  expected: %s\n  got:      %s", expected, actual)
		}
		fmt.Println("SHA256 verified.")
	}

	// Locate the current executable.
	execPath, err := resolveExecutable()
	if err != nil {
		return err
	}

	// Atomic replace: extract directly into a temp file next to the target, then rename.
	dir := filepath.Dir(execPath)
	tmpBin, err := os.CreateTemp(dir, ".openbee-new-*")
	if err != nil {
		// May lack write permission — try sudo-less approach with a clear message
		return fmt.Errorf("create temp file in %s (may need sudo): %w", dir, err)
	}
	tmpBinPath := tmpBin.Name()
	defer os.Remove(tmpBinPath)

	if err := extractBinary(archivePath, tmpBin); err != nil {
		return fmt.Errorf("extract: %w", err)
	}
	if err := os.Chmod(tmpBinPath, executablePerm); err != nil {
		return fmt.Errorf("set permissions: %w", err)
	}
	if err := os.Rename(tmpBinPath, execPath); err != nil {
		return fmt.Errorf("replace binary (may need sudo): %w", err)
	}

	fmt.Printf("Successfully upgraded openbee to %s.\n", newVersion)
	return nil
}

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

func extractBinary(archivePath string, dest *os.File) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		name := filepath.Base(hdr.Name)
		if name == upgradeBinaryName || name == upgradeBinaryNameWin {
			if _, err := io.Copy(dest, tr); err != nil {
				dest.Close()
				return err
			}
			return dest.Close()
		}
	}
	return fmt.Errorf("%s binary not found in archive", upgradeBinaryName)
}
