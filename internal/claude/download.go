package claude

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// GitHubAPI is the GitHub Releases API endpoint for cc-download.
// It is a var (not const) so tests can override it with a local httptest server.
var GitHubAPI = "https://api.github.com/repos/theopenbee/cc-download/releases/latest"

const gitHubRelBase = "https://github.com/theopenbee/cc-download/releases/download"
const maxDownloadBytes = 512 * 1024 * 1024 // 512 MB guard

// claudePlatform represents a target platform for Claude Code download.
type claudePlatform struct {
	os      string // "darwin" or "linux"
	arch    string // "arm64" or "x64"
	variant string // "" or "musl"
}

// supportedPlatforms lists all platforms that support Claude Code download.
var supportedPlatforms = map[claudePlatform]struct{}{
	{"darwin", "arm64", ""}:    {},
	{"darwin", "x64", ""}:      {},
	{"linux", "arm64", ""}:     {},
	{"linux", "x64", ""}:       {},
	{"linux", "arm64", "musl"}: {},
	{"linux", "x64", "musl"}:   {},
}

func isSupportedPlatform(p claudePlatform) bool {
	_, ok := supportedPlatforms[p]
	return ok
}

func detectPlatform() claudePlatform {
	p := claudePlatform{
		os:   runtime.GOOS,
		arch: mapArch(runtime.GOARCH),
	}
	if runtime.GOOS == "linux" && isMusl() {
		p.variant = "musl"
	}
	return p
}

func mapArch(goarch string) string {
	if goarch == "amd64" {
		return "x64"
	}
	return goarch
}

// isMuslWith checks for the musl dynamic linker using the provided glob function.
// The globFunc parameter allows dependency injection for testing.
func isMuslWith(globFunc func(string) ([]string, error)) bool {
	matches, err := globFunc("/lib/ld-musl-*.so*")
	if err != nil {
		return false // fail-open: treat errors as non-musl
	}
	return len(matches) > 0
}

func isMusl() bool {
	return isMuslWith(filepath.Glob)
}

func buildClaudeDownloadURL(p claudePlatform, version string) string {
	versionNum := strings.TrimPrefix(version, "v")
	platformStr := p.os + "-" + p.arch
	if p.variant != "" {
		platformStr += "-" + p.variant
	}
	assetName := fmt.Sprintf("claude-%s-%s", versionNum, platformStr)
	return fmt.Sprintf("%s/%s/%s", gitHubRelBase, version, assetName)
}

func fetchLatestClaudeVersion() (string, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(GitHubAPI)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub API returned %d", resp.StatusCode)
	}
	var rel struct {
		TagName string `json:"tag_name"`
	}
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
	if n >= maxDownloadBytes {
		return fmt.Errorf("download exceeded %d byte limit", maxDownloadBytes)
	}
	return nil
}

// Download downloads Claude Code to stateDir/bin/claude and returns the installed path.
// If the binary already exists and force is false, it returns the existing path immediately
// without re-downloading.
func Download(stateDir string, force bool) (string, error) {
	destPath := filepath.Join(stateDir, "bin", "claude")

	if !force {
		if _, err := os.Stat(destPath); err == nil {
			return destPath, nil
		}
	}

	platform := detectPlatform()
	if !isSupportedPlatform(platform) {
		return "", fmt.Errorf(
			"current platform (%s/%s) does not support automatic Claude Code download.\n"+
				"Supported platforms: darwin-arm64, darwin-x64, linux-arm64, linux-x64, linux-arm64-musl, linux-x64-musl\n"+
				"Please install manually.",
			runtime.GOOS, runtime.GOARCH,
		)
	}

	binDir := filepath.Join(stateDir, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		return "", fmt.Errorf("create directory: %w", err)
	}

	fmt.Println("Checking for latest Claude version...")
	version, err := fetchLatestClaudeVersion()
	if err != nil {
		return "", fmt.Errorf("fetch latest Claude version: %w", err)
	}
	fmt.Printf("Latest Claude version: %s\n", version)

	versionNum := strings.TrimPrefix(version, "v")
	platformStr := platform.os + "-" + platform.arch
	if platform.variant != "" {
		platformStr += "-" + platform.variant
	}

	checksumURL := fmt.Sprintf("%s/%s/checksums-sha256.txt", gitHubRelBase, version)
	binaryURL := buildClaudeDownloadURL(platform, version)
	assetName := fmt.Sprintf("claude-%s-%s", versionNum, platformStr)

	tmpDir, err := os.MkdirTemp("", "openbee-claude-*")
	if err != nil {
		return "", fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	checksumPath := filepath.Join(tmpDir, "checksums-sha256.txt")
	checksumAvailable := true
	if err := downloadFile(checksumURL, checksumPath, nil); err != nil {
		checksumAvailable = false
		fmt.Printf("warning: failed to download checksums-sha256.txt, skipping verification (%v)\n", err)
	}

	fmt.Printf("Downloading Claude %s (%s)...\n", version, platformStr)

	tmpPath := destPath + ".tmp"
	os.Remove(tmpPath) // clean up any stale partial download from a previous interrupted run
	h := sha256.New()
	if err := downloadFile(binaryURL, tmpPath, h); err != nil {
		os.Remove(tmpPath)
		return "", fmt.Errorf("download: %w", err)
	}

	if checksumAvailable {
		fmt.Println("Verifying SHA256...")
		data, err := os.ReadFile(checksumPath)
		if err != nil {
			os.Remove(tmpPath)
			return "", fmt.Errorf("read checksums: %w", err)
		}
		var expected string
		for line := range strings.SplitSeq(string(data), "\n") {
			if parts := strings.Fields(line); len(parts) == 2 && parts[1] == assetName {
				expected = parts[0]
				break
			}
		}
		if expected == "" {
			os.Remove(tmpPath)
			return "", fmt.Errorf("no checksum for %s in checksums-sha256.txt", assetName)
		}
		if actual := hex.EncodeToString(h.Sum(nil)); actual != expected {
			os.Remove(tmpPath)
			return "", fmt.Errorf("SHA256 mismatch\n  expected: %s\n  got:      %s", expected, actual)
		}
		fmt.Println("SHA256 verified.")
	}

	if err := os.Chmod(tmpPath, 0755); err != nil {
		os.Remove(tmpPath)
		return "", fmt.Errorf("set executable permission: %w", err)
	}

	if err := os.Rename(tmpPath, destPath); err != nil {
		os.Remove(tmpPath)
		return "", fmt.Errorf("move file: %w", err)
	}

	fmt.Printf("Claude downloaded to: %s\n", destPath)
	return destPath, nil
}
