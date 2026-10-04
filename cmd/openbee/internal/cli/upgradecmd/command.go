package upgradecmd

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
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/theopenbee/openbee/internal/infra/i18n"
	"github.com/theopenbee/openbee/internal/infra/utils"
)

const (
	githubAPILatest      = "https://api.github.com/repos/theopenbee/openbee/releases/latest"
	githubRelBase        = "https://github.com/theopenbee/openbee/releases/download"
	upgradeBinaryName    = "openbee"
	upgradeBinaryNameWin = "openbee.exe"
)

const executablePerm = 0o755

// apiClient is the HTTP client for short GitHub API calls (version check).
var apiClient = &http.Client{Timeout: 15 * time.Second}

type githubRelease struct {
	TagName string `json:"tag_name"`
}

// NewCommand returns the "upgrade" cobra command. currentVersion is injected at build time.
func NewCommand(currentVersion string) *cobra.Command {
	var checkOnly bool
	cmd := &cobra.Command{
		Use:   "upgrade",
		Short: i18n.M.Cmd.Upgrade.Short,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUpgrade(currentVersion, checkOnly)
		},
	}
	cmd.Flags().BoolVar(&checkOnly, "check", false, i18n.M.Flag.UpgradeCheck)
	return cmd
}

func runUpgrade(current string, checkOnly bool) error {
	fmt.Printf(i18n.M.Output.Upgrade.CurrentVersion+"\n", current)
	fmt.Println(i18n.M.Output.Upgrade.Checking)

	latest, err := fetchLatestVersion()
	if err != nil {
		return fmt.Errorf("fetch latest version: %w", err)
	}

	fmt.Printf(i18n.M.Output.Upgrade.LatestVersion+"\n", latest)

	if !isNewer(latest, current) {
		fmt.Println(i18n.M.Output.Upgrade.UpToDate)
		return nil
	}

	fmt.Printf(i18n.M.Output.Upgrade.NewVersion+"\n", latest)

	if checkOnly {
		fmt.Println(i18n.M.Output.Upgrade.RunCmd)
		return nil
	}

	return doUpgrade(latest)
}

func fetchLatestVersion() (string, error) {
	resp, err := apiClient.Get(githubAPILatest)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, resp.Body)
		return "", fmt.Errorf("GitHub API returned %d", resp.StatusCode)
	}
	var rel githubRelease
	if err := json.NewDecoder(io.LimitReader(resp.Body, 65536)).Decode(&rel); err != nil {
		return "", fmt.Errorf("parse response: %w", err)
	}
	return normalizeVersionTag(rel.TagName)
}

// normalizeVersionTag trims whitespace, validates the tag is non-empty, and
// ensures it carries a "v" prefix (e.g. "1.2.3" → "v1.2.3").
func normalizeVersionTag(tag string) (string, error) {
	tag = strings.TrimSpace(tag)
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
	archiveName := releaseArchiveName(versionNum, runtime.GOOS, runtime.GOARCH)

	relBase := fmt.Sprintf("%s/%s", githubRelBase, newVersion)
	archiveURL := fmt.Sprintf("%s/%s", relBase, archiveName)
	checksumURL := fmt.Sprintf("%s/checksums.txt", relBase)

	fmt.Printf(i18n.M.Output.Upgrade.Downloading+"\n", archiveName)

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
		fmt.Printf(i18n.M.Output.Upgrade.ChecksumWarning+"\n", err)
	}

	h := sha256.New()
	archivePath := filepath.Join(tmpDir, archiveName)
	if err := downloadFile(archiveURL, archivePath, h); err != nil {
		return fmt.Errorf("download: %w", err)
	}

	if checksumAvailable {
		fmt.Println(i18n.M.Output.Upgrade.Verifying)
		data, err := os.ReadFile(checksumPath)
		if err != nil {
			return fmt.Errorf("read checksums: %w", err)
		}
		expected, err := parseChecksumFile(data, archiveName)
		if err != nil {
			return fmt.Errorf("%w in checksums.txt", err)
		}
		if actual := hex.EncodeToString(h.Sum(nil)); actual != expected {
			return fmt.Errorf("SHA256 mismatch\n  expected: %s\n  got:      %s", expected, actual)
		}
		fmt.Println(i18n.M.Output.Upgrade.Verified)
	}

	execPath, err := utils.ResolveExecutable()
	if err != nil {
		return err
	}
	fmt.Printf(i18n.M.Output.Upgrade.BinaryAt+"\n", execPath)

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
		tmpBin.Close()
		return fmt.Errorf("extract: %w", err)
	}
	if err := tmpBin.Close(); err != nil {
		return fmt.Errorf("write new binary: %w", err)
	}
	if err := os.Chmod(tmpBinPath, executablePerm); err != nil {
		return fmt.Errorf("set permissions: %w", err)
	}
	if err := os.Rename(tmpBinPath, execPath); err != nil {
		return fmt.Errorf("replace binary (may need sudo): %w", err)
	}

	fmt.Printf(i18n.M.Output.Upgrade.Success+"\n", newVersion)
	return nil
}
