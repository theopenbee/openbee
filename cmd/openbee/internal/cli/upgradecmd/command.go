package upgradecmd

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

	"github.com/spf13/cobra"
	"github.com/theopenbee/openbee/internal/infra/i18n"
	"github.com/theopenbee/openbee/internal/infra/utils"
)

const (
	githubAPILatest = "https://api.github.com/repos/theopenbee/openbee/releases/latest"
	githubRelBase   = "https://github.com/theopenbee/openbee/releases/download"

	defaultCDNBaseURL = "https://dl.theopenbee.cn"

	upgradeBinaryName    = "openbee"
	upgradeBinaryNameWin = "openbee.exe"
	executablePerm       = 0o755
)

type githubRelease struct {
	TagName string `json:"tag_name"`
}

var (
	upgradeCheckOnly bool
	upgradeCDNURL    string
	upgradeCN        bool
)

func resolveCDNURL(cdnURL string, useCN bool) string {
	if cdnURL == "" && useCN {
		return defaultCDNBaseURL
	}
	return cdnURL
}

// NewCommand returns the "upgrade" cobra command.
// currentVersion is the version string injected at build time.
func NewCommand(currentVersion string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "upgrade",
		Short: i18n.M.Cmd.Upgrade.Short,
		Long:  i18n.M.Cmd.Upgrade.Long,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUpgrade(currentVersion, upgradeCheckOnly, resolveCDNURL(upgradeCDNURL, upgradeCN))
		},
	}
	cmd.Flags().BoolVar(&upgradeCheckOnly, "check", false, i18n.M.Flag.UpgradeCheck)
	cmd.Flags().StringVar(&upgradeCDNURL, "cdn-url", "", i18n.M.Flag.UpgradeCDNURL)
	cmd.Flags().BoolVar(&upgradeCN, "cn", false, i18n.M.Flag.UpgradeCN)
	return cmd
}

func runUpgrade(current string, checkOnly bool, cdnURL string) error {
	fmt.Printf(i18n.M.Output.Upgrade.CurrentVersion+"\n", current)

	if cdnURL != "" {
		fmt.Printf(i18n.M.Output.Upgrade.UsingCDN+"\n", cdnURL)
	}

	fmt.Println(i18n.M.Output.Upgrade.Checking)

	latest, err := fetchLatestVersion(cdnURL)
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

	return doUpgrade(latest, cdnURL)
}

func fetchLatestVersion(cdnURL string) (string, error) {
	if cdnURL != "" {
		return utils.FetchPlainTextVersion(cdnURL + "/releases/latest.txt")
	}

	resp, err := utils.APIClient.Get(githubAPILatest)
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
	return utils.NormalizeVersionTag(rel.TagName)
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

func doUpgrade(newVersion string, cdnURL string) error {
	versionNum := strings.TrimPrefix(newVersion, "v")
	archiveName := fmt.Sprintf("%s-%s-%s-%s.tar.gz", upgradeBinaryName, versionNum, runtime.GOOS, runtime.GOARCH)

	var relBase string
	if cdnURL != "" {
		relBase = fmt.Sprintf("%s/releases/%s", cdnURL, newVersion)
	} else {
		relBase = fmt.Sprintf("%s/%s", githubRelBase, newVersion)
	}
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
	if err := utils.DownloadFile(checksumURL, checksumPath, nil); err != nil {
		checksumAvailable = false
		fmt.Printf(i18n.M.Output.Upgrade.ChecksumWarning+"\n", err)
	}

	h := sha256.New()
	archivePath := filepath.Join(tmpDir, archiveName)
	if err := utils.DownloadFile(archiveURL, archivePath, h); err != nil {
		return fmt.Errorf("download: %w", err)
	}

	if checksumAvailable {
		fmt.Println(i18n.M.Output.Upgrade.Verifying)
		data, err := os.ReadFile(checksumPath)
		if err != nil {
			return fmt.Errorf("read checksums: %w", err)
		}
		expected, err := utils.ParseChecksumFile(data, archiveName)
		if err != nil {
			return fmt.Errorf("%w in checksums.txt", err)
		}
		if actual := hex.EncodeToString(h.Sum(nil)); actual != expected {
			return fmt.Errorf("SHA256 mismatch\n  expected: %s\n  got:      %s", expected, actual)
		}
		fmt.Println(i18n.M.Output.Upgrade.Verified)
	}

	// Locate the current executable.
	// resolveExecutable is duplicated here to avoid an import cycle with the cli
	// package (cli imports upgradecmd via NewRoot). This matches daemoncmd's pattern.
	execPath, err := resolveExecutable()
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
		return fmt.Errorf("extract: %w", err)
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

// resolveExecutable returns the real path of the running binary, following symlinks.
// Also defined in daemoncmd; kept local in each leaf package to avoid an import
// cycle with the cli package (cli imports upgradecmd via NewRoot).
func resolveExecutable() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolve executable: %w", err)
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return "", fmt.Errorf("eval symlinks: %w", err)
	}
	return exe, nil
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
