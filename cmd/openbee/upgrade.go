package main

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
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
	goos := runtime.GOOS

	versionNum := strings.TrimPrefix(newVersion, "v")
	archiveName := fmt.Sprintf("openbee-%s-%s-%s.tar.gz", versionNum, goos, runtime.GOARCH)
	archiveURL := fmt.Sprintf("%s/%s/%s", githubRelBase, newVersion, archiveName)
	checksumURL := fmt.Sprintf("%s/%s/checksums.txt", githubRelBase, newVersion)

	fmt.Printf("Downloading %s...\n", archiveName)

	tmpDir, err := os.MkdirTemp("", "openbee-upgrade-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	archivePath := filepath.Join(tmpDir, archiveName)
	if err := downloadFile(archiveURL, archivePath); err != nil {
		return fmt.Errorf("download: %w", err)
	}

	// Verify checksum if available
	checksumPath := filepath.Join(tmpDir, "checksums.txt")
	if err := downloadFile(checksumURL, checksumPath); err != nil {
		fmt.Printf("warning: failed to download checksums.txt, skipping verification (%v)\n", err)
	} else {
		fmt.Println("Verifying SHA256...")
		if err := verifyChecksum(archivePath, archiveName, checksumPath); err != nil {
			return fmt.Errorf("checksum verification: %w", err)
		}
		fmt.Println("SHA256 verified.")
	}

	// Extract binary from tarball
	binName := upgradeBinaryName
	if goos == "windows" {
		binName = upgradeBinaryNameWin
	}
	newBinPath := filepath.Join(tmpDir, binName)
	if err := extractBinary(archivePath, newBinPath); err != nil {
		return fmt.Errorf("extract: %w", err)
	}

	// Locate the current executable
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("determine executable path: %w", err)
	}
	execPath, err = filepath.EvalSymlinks(execPath)
	if err != nil {
		return fmt.Errorf("resolve symlink: %w", err)
	}

	// Atomic replace: write to a temp file next to the target, then rename
	dir := filepath.Dir(execPath)
	tmpBin, err := os.CreateTemp(dir, ".openbee-new-*")
	if err != nil {
		// May lack write permission — try sudo-less approach with a clear message
		return fmt.Errorf("create temp file in %s (may need sudo): %w", dir, err)
	}
	tmpBinPath := tmpBin.Name()
	tmpBin.Close()
	defer os.Remove(tmpBinPath)

	if err := copyFile(newBinPath, tmpBinPath); err != nil {
		return fmt.Errorf("copy new binary: %w", err)
	}
	if err := os.Chmod(tmpBinPath, 0755); err != nil {
		return fmt.Errorf("set permissions: %w", err)
	}
	if err := os.Rename(tmpBinPath, execPath); err != nil {
		return fmt.Errorf("replace binary (may need sudo): %w", err)
	}

	fmt.Printf("Successfully upgraded openbee to %s.\n", newVersion)
	return nil
}

func downloadFile(url, dest string) error {
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
	_, err = io.Copy(f, io.LimitReader(resp.Body, maxDownloadBytes))
	return err
}

func verifyChecksum(archivePath, archiveName, checksumPath string) error {
	data, err := os.ReadFile(checksumPath)
	if err != nil {
		return err
	}

	var expected string
	for line := range strings.SplitSeq(string(data), "\n") {
		parts := strings.Fields(line)
		if len(parts) == 2 && parts[1] == archiveName {
			expected = parts[0]
			break
		}
	}
	if expected == "" {
		return fmt.Errorf("no checksum for %s in checksums.txt", archiveName)
	}

	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	actual := fmt.Sprintf("%x", h.Sum(nil))
	if actual != expected {
		return fmt.Errorf("SHA256 mismatch\n  expected: %s\n  got:      %s", expected, actual)
	}
	return nil
}

func extractBinary(archivePath, destPath string) error {
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
		name := filepath.Base(hdr.Name)
		if name == upgradeBinaryName || name == upgradeBinaryNameWin {
			out, err := os.Create(destPath)
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return err
			}
			return out.Close()
		}
	}
	return fmt.Errorf("openbee binary not found in archive")
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}
