package upgradecmd

import (
	"encoding/json"
	"errors"
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
	// githubTokenEnv names the env var whose token, if set, authenticates GitHub API
	// calls (anonymous requests are limited to 60 per hour per IP).
	githubTokenEnv = "GITHUB_TOKEN"
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

	latest, err := fetchLatestVersion(githubAPILatest)
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

func fetchLatestVersion(apiURL string) (string, error) {
	token := strings.TrimSpace(os.Getenv(githubTokenEnv))
	req, err := http.NewRequest(http.MethodGet, apiURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := apiClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, resp.Body)
		return "", apiStatusError(resp, token != "")
	}
	var rel githubRelease
	if err := json.NewDecoder(io.LimitReader(resp.Body, 65536)).Decode(&rel); err != nil {
		return "", fmt.Errorf("parse response: %w", err)
	}
	return normalizeVersionTag(rel.TagName)
}

// apiStatusError explains a non-200 GitHub API response, calling out rate limiting
// and rejected tokens, which would otherwise surface only as a bare 403 or 401.
func apiStatusError(resp *http.Response, withToken bool) error {
	switch {
	case isRateLimited(resp):
		msg := fmt.Sprintf("GitHub API rate limit exceeded (HTTP %d)", resp.StatusCode)
		if reset, err := strconv.ParseInt(resp.Header.Get("X-RateLimit-Reset"), 10, 64); err == nil {
			msg += ", resets at " + time.Unix(reset, 0).Format(time.DateTime)
		}
		if !withToken {
			msg += "; set " + githubTokenEnv + " to raise the limit"
		}
		return errors.New(msg)
	case resp.StatusCode == http.StatusUnauthorized && withToken:
		return fmt.Errorf("GitHub API returned 401: check %s", githubTokenEnv)
	default:
		return fmt.Errorf("GitHub API returned %d", resp.StatusCode)
	}
}

// isRateLimited reports whether resp is a GitHub rate-limit rejection. The primary
// limit answers 403 with X-RateLimit-Remaining: 0; secondary limits answer 403 or
// 429, usually with Retry-After.
func isRateLimited(resp *http.Response) bool {
	switch resp.StatusCode {
	case http.StatusTooManyRequests:
		return true
	case http.StatusForbidden:
		return resp.Header.Get("X-RateLimit-Remaining") == "0" || resp.Header.Get("Retry-After") != ""
	}
	return false
}

// normalizeVersionTag trims whitespace, validates the tag is non-empty, and
// ensures it carries a lowercase "v" prefix ("1.2.3" and "V1.2.3" → "v1.2.3").
func normalizeVersionTag(tag string) (string, error) {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return "", fmt.Errorf("empty version tag")
	}
	if tag[0] == 'v' || tag[0] == 'V' {
		tag = tag[1:]
	}
	return "v" + tag, nil
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

	fmt.Printf(i18n.M.Output.Upgrade.Downloading+"\n", archiveName)

	tmpDir, err := os.MkdirTemp("", "openbee-upgrade-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	archivePath, err := fetchVerifiedArchive(relBase, archiveName, tmpDir)
	if err != nil {
		return err
	}

	execPath, err := utils.ResolveExecutable()
	if err != nil {
		return err
	}
	fmt.Printf(i18n.M.Output.Upgrade.BinaryAt+"\n", execPath)

	// Extract into a temp file next to the target (same filesystem), then swap it in.
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
	if err := replaceExecutable(tmpBinPath, execPath); err != nil {
		return fmt.Errorf("replace binary (may need sudo): %w", err)
	}

	fmt.Printf(i18n.M.Output.Upgrade.Success+"\n", newVersion)
	return nil
}
