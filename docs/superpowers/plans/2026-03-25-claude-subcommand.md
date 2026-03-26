# `openbee claude` Subcommand Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Extract Claude Code download and provider-config logic from `config_claude.go` into `internal/claude/` package, then add `openbee claude download` and `openbee claude env` subcommands.

**Architecture:** Move download helpers to `internal/claude/download.go` and provider helpers to `internal/claude/provider.go`. The existing `config_claude.go` becomes a thin wrapper that delegates to the new package. A new `cmd/openbee/claude.go` file registers the `claude` parent command and its two sub-subcommands.

**Tech Stack:** Go 1.25, Cobra (github.com/spf13/cobra), survey/v2 (github.com/AlecAivazis/survey/v2)

---

## File Map

| Action | Path | Responsibility |
|--------|------|----------------|
| Create | `internal/claude/download.go` | Platform detection, URL building, `Download()` public func |
| Create | `internal/claude/download_test.go` | Tests for download logic (moved from config_claude_test.go) |
| Create | `internal/claude/provider.go` | Env map builders, JSON merge helpers, `ConfigureProvider()` |
| Create | `internal/claude/provider_test.go` | Tests for provider logic (moved from config_claude_test.go) |
| Modify | `cmd/openbee/config_claude.go` | Remove extracted code; delegate to `internal/claude` |
| Modify | `cmd/openbee/config_claude_test.go` | Remove tests that moved to `internal/claude` |
| Create | `cmd/openbee/claude.go` | `openbee claude`, `claude download`, `claude env` Cobra commands |

---

## Task 1: Create `internal/claude/download.go`

**Files:**
- Create: `internal/claude/download.go`

- [ ] **Step 1: Write `internal/claude/download.go`**

```go
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
	h := sha256.New()
	if err := downloadFile(binaryURL, tmpPath, h); err != nil {
		return "", fmt.Errorf("download: %w", err)
	}

	if checksumAvailable {
		fmt.Println("Verifying SHA256...")
		data, err := os.ReadFile(checksumPath)
		if err != nil {
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
```

- [ ] **Step 2: Verify it compiles**

```bash
cd /path/to/openbee && go build ./internal/claude/...
```

Expected: no output (clean build).

---

## Task 2: Create `internal/claude/download_test.go`

**Files:**
- Create: `internal/claude/download_test.go`

These tests are moved from `cmd/openbee/config_claude_test.go`. They test private functions so the package declaration must be `package claude` (white-box).

- [ ] **Step 1: Write `internal/claude/download_test.go`**

```go
package claude

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMapArch(t *testing.T) {
	tests := []struct {
		goarch string
		want   string
	}{
		{"amd64", "x64"},
		{"arm64", "arm64"},
		{"386", "386"},
		{"riscv64", "riscv64"},
	}
	for _, tt := range tests {
		if got := mapArch(tt.goarch); got != tt.want {
			t.Errorf("mapArch(%q) = %q, want %q", tt.goarch, got, tt.want)
		}
	}
}

func TestIsMuslWith(t *testing.T) {
	found := isMuslWith(func(pattern string) ([]string, error) {
		return []string{"/lib/ld-musl-x86_64.so.1"}, nil
	})
	if !found {
		t.Error("expected true when musl linker found")
	}

	notFound := isMuslWith(func(pattern string) ([]string, error) {
		return nil, nil
	})
	if notFound {
		t.Error("expected false when no musl linker")
	}

	errCase := isMuslWith(func(pattern string) ([]string, error) {
		return nil, fmt.Errorf("permission denied")
	})
	if errCase {
		t.Error("expected false (fail-open) when glob errors")
	}
}

func TestIsSupportedPlatform(t *testing.T) {
	supported := []claudePlatform{
		{"darwin", "arm64", ""},
		{"darwin", "x64", ""},
		{"linux", "arm64", ""},
		{"linux", "x64", ""},
		{"linux", "arm64", "musl"},
		{"linux", "x64", "musl"},
	}
	for _, p := range supported {
		if !isSupportedPlatform(p) {
			t.Errorf("expected supported: %+v", p)
		}
	}

	unsupported := []claudePlatform{
		{"windows", "x64", ""},
		{"windows", "arm64", ""},
		{"freebsd", "x64", ""},
		{"linux", "386", ""},
		{"darwin", "x64", "musl"},
		{"darwin", "arm64", "musl"},
	}
	for _, p := range unsupported {
		if isSupportedPlatform(p) {
			t.Errorf("expected unsupported: %+v", p)
		}
	}
}

func TestBuildClaudeDownloadURL(t *testing.T) {
	const version = "v1.2.3"
	tests := []struct {
		platform claudePlatform
		want     string
	}{
		{
			claudePlatform{"darwin", "arm64", ""},
			gitHubRelBase + "/v1.2.3/claude-1.2.3-darwin-arm64",
		},
		{
			claudePlatform{"darwin", "x64", ""},
			gitHubRelBase + "/v1.2.3/claude-1.2.3-darwin-x64",
		},
		{
			claudePlatform{"linux", "x64", ""},
			gitHubRelBase + "/v1.2.3/claude-1.2.3-linux-x64",
		},
		{
			claudePlatform{"linux", "arm64", "musl"},
			gitHubRelBase + "/v1.2.3/claude-1.2.3-linux-arm64-musl",
		},
	}
	for _, tt := range tests {
		got := buildClaudeDownloadURL(tt.platform, version)
		if got != tt.want {
			t.Errorf("buildClaudeDownloadURL(%+v, %q) = %q, want %q", tt.platform, version, got, tt.want)
		}
	}
}

func TestFetchLatestClaudeVersion(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		status  int
		want    string
		wantErr bool
	}{
		{
			name:   "v-prefixed tag",
			body:   `{"tag_name": "v1.2.3"}`,
			status: http.StatusOK,
			want:   "v1.2.3",
		},
		{
			name:   "tag without v prefix",
			body:   `{"tag_name": "2.0.0"}`,
			status: http.StatusOK,
			want:   "v2.0.0",
		},
		{
			name:    "empty tag",
			body:    `{"tag_name": ""}`,
			status:  http.StatusOK,
			wantErr: true,
		},
		{
			name:    "non-200 status",
			body:    `{}`,
			status:  http.StatusNotFound,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
				fmt.Fprint(w, tt.body)
			}))
			defer srv.Close()

			orig := GitHubAPI
			GitHubAPI = srv.URL
			defer func() { GitHubAPI = orig }()

			got, err := fetchLatestClaudeVersion()
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDetectPlatform(t *testing.T) {
	p := detectPlatform()
	if p.os == "" {
		t.Error("detectPlatform() returned empty os")
	}
	if p.arch == "" {
		t.Error("detectPlatform() returned empty arch")
	}
	if !isSupportedPlatform(p) {
		t.Errorf("current platform %+v should be supported", p)
	}
}

// Ensure TestFetchLatestClaudeVersion uses json import
var _ = json.Marshal
```

- [ ] **Step 2: Run the new tests**

```bash
go test ./internal/claude/... -run TestMapArch -v
go test ./internal/claude/... -run TestIsMuslWith -v
go test ./internal/claude/... -run TestIsSupportedPlatform -v
go test ./internal/claude/... -run TestBuildClaudeDownloadURL -v
go test ./internal/claude/... -run TestFetchLatestClaudeVersion -v
go test ./internal/claude/... -run TestDetectPlatform -v
```

Expected: all PASS.

---

## Task 3: Create `internal/claude/provider.go`

**Files:**
- Create: `internal/claude/provider.go`

- [ ] **Step 1: Write `internal/claude/provider.go`**

```go
package claude

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/AlecAivazis/survey/v2"
	"github.com/AlecAivazis/survey/v2/terminal"
)

// ErrInterrupted is returned when the user cancels an interactive prompt (Ctrl+C).
var ErrInterrupted = errors.New("interrupted")

// Provider display names used in the selection menu and switch cases.
const (
	ProviderMoonshot   = "Moonshot (Kimi)"
	ProviderDeepSeek   = "DeepSeek"
	ProviderGLM        = "Zhipu (GLM)"
	ProviderMiniMax    = "MiniMax"
	ProviderAliyun     = "Alibaba Cloud (Qwen)"
	ProviderVolcengine = "Volcengine (Doubao)"
	ProviderTencent    = "Tencent Cloud"
	ProviderCustom     = "Custom provider"
)

func handleSurveyErr(err error) error {
	if errors.Is(err, terminal.InterruptErr) {
		fmt.Println("\nCancelled.")
		return ErrInterrupted
	}
	return err
}

func promptAPIKey(message string) (string, error) {
	var apiKey string
	if err := survey.AskOne(&survey.Input{
		Message: message,
	}, &apiKey, survey.WithValidator(survey.Required)); err != nil {
		return "", handleSurveyErr(err)
	}
	return apiKey, nil
}

// Provider env map builders — only ANTHROPIC_AUTH_TOKEN comes from user input.

func moonshotEnv(apiKey string) map[string]string {
	return map[string]string{
		"ANTHROPIC_BASE_URL":                      "https://api.moonshot.cn/anthropic",
		"ANTHROPIC_AUTH_TOKEN":                    apiKey,
		"ANTHROPIC_MODEL":                         "kimi-k2.5",
		"ANTHROPIC_SMALL_FAST_MODEL":              "kimi-k2.5",
		"ANTHROPIC_DEFAULT_OPUS_MODEL":            "kimi-k2.5",
		"ANTHROPIC_DEFAULT_SONNET_MODEL":          "kimi-k2.5",
		"ANTHROPIC_DEFAULT_HAIKU_MODEL":           "kimi-k2.5",
		"CLAUDE_CODE_SUBAGENT_MODEL":              "kimi-k2.5",
		"CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1",
		"ENABLE_TOOL_SEARCH":                      "false",
		"API_TIMEOUT_MS":                          "600000",
	}
}

func glmEnv(apiKey string) map[string]string {
	return map[string]string{
		"ANTHROPIC_AUTH_TOKEN":                    apiKey,
		"ANTHROPIC_BASE_URL":                      "https://open.bigmodel.cn/api/anthropic",
		"API_TIMEOUT_MS":                          "3000000",
		"CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1",
	}
}

func minimaxEnv(apiKey string) map[string]string {
	return map[string]string{
		"ANTHROPIC_BASE_URL":                      "https://api.minimaxi.com/anthropic",
		"ANTHROPIC_AUTH_TOKEN":                    apiKey,
		"API_TIMEOUT_MS":                          "3000000",
		"CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1",
		"ANTHROPIC_MODEL":                         "MiniMax-M2.7",
		"ANTHROPIC_SMALL_FAST_MODEL":              "MiniMax-M2.7",
		"ANTHROPIC_DEFAULT_SONNET_MODEL":          "MiniMax-M2.7",
		"ANTHROPIC_DEFAULT_OPUS_MODEL":            "MiniMax-M2.7",
		"ANTHROPIC_DEFAULT_HAIKU_MODEL":           "MiniMax-M2.7",
	}
}

func deepseekEnv(apiKey string) map[string]string {
	return map[string]string{
		"ANTHROPIC_BASE_URL":                      "https://api.deepseek.com/anthropic",
		"ANTHROPIC_AUTH_TOKEN":                    apiKey,
		"ANTHROPIC_MODEL":                         "deepseek-chat",
		"ANTHROPIC_SMALL_FAST_MODEL":              "deepseek-chat",
		"CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1",
		"API_TIMEOUT_MS":                          "600000",
	}
}

func standardProviderEnv(baseURL, apiKey, model string) map[string]string {
	return map[string]string{
		"ANTHROPIC_AUTH_TOKEN":                    apiKey,
		"ANTHROPIC_BASE_URL":                      baseURL,
		"ANTHROPIC_MODEL":                         model,
		"CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1",
	}
}

func aliyunEnv(apiKey, model string) map[string]string {
	return standardProviderEnv("https://coding.dashscope.aliyuncs.com/apps/anthropic", apiKey, model)
}

func volcengineEnv(apiKey, model string) map[string]string {
	return standardProviderEnv("https://ark.cn-beijing.volces.com/api/coding", apiKey, model)
}

func tencentEnv(apiKey, model string) map[string]string {
	return standardProviderEnv("https://api.lkeap.cloud.tencent.com/coding/anthropic", apiKey, model)
}

func customEnv(baseURL, apiKey string) map[string]string {
	return map[string]string{
		"ANTHROPIC_BASE_URL":   baseURL,
		"ANTHROPIC_AUTH_TOKEN": apiKey,
	}
}

// providerEnvKeys lists all environment variable keys that any provider may write.
// These are cleared before writing new provider settings so that stale keys from a
// previous provider do not linger after switching.
var providerEnvKeys = []string{
	"ANTHROPIC_AUTH_TOKEN",
	"ANTHROPIC_BASE_URL",
	"ANTHROPIC_MODEL",
	"ANTHROPIC_SMALL_FAST_MODEL",
	"ANTHROPIC_DEFAULT_SONNET_MODEL",
	"ANTHROPIC_DEFAULT_OPUS_MODEL",
	"ANTHROPIC_DEFAULT_HAIKU_MODEL",
	"CLAUDE_CODE_SUBAGENT_MODEL",
	"ENABLE_TOOL_SEARCH",
	"API_TIMEOUT_MS",
	"CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC",
}

// mergeJSONFile reads the JSON file at path (if it exists) into a map,
// calls apply to mutate the map, then writes it back with indentation.
func mergeJSONFile(path string, apply func(map[string]any)) error {
	existing := make(map[string]any)
	if data, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(data, &existing); err != nil {
			fmt.Printf("warning: %s has invalid JSON, overwriting: %v\n", path, err)
		}
	}
	apply(existing)
	data, err := json.MarshalIndent(existing, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal JSON: %w", err)
	}
	return os.WriteFile(path, append(data, '\n'), 0644)
}

// mergeClaudeSettings reads existing ~/.claude/settings.json (if any),
// removes all known provider env keys, merges the provided env map, and writes back.
func mergeClaudeSettings(path string, env map[string]string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}
	return mergeJSONFile(path, func(m map[string]any) {
		envMap, ok := m["env"].(map[string]any)
		if !ok {
			envMap = make(map[string]any)
		}
		for _, k := range providerEnvKeys {
			delete(envMap, k)
		}
		for k, v := range env {
			envMap[k] = v
		}
		m["env"] = envMap
	})
}

// mergeClaudeJSON reads existing ~/.claude.json (if any),
// sets hasCompletedOnboarding=true, preserves all other keys, and writes back.
func mergeClaudeJSON(path string) error {
	return mergeJSONFile(path, func(m map[string]any) {
		m["hasCompletedOnboarding"] = true
	})
}

// ConfigureProvider runs an interactive survey to select a provider and API key,
// then writes the result to ~/.claude/settings.json (and ~/.claude.json where needed).
// Returns ErrInterrupted if the user cancels with Ctrl+C.
func ConfigureProvider() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home directory: %w", err)
	}

	settingsPath := filepath.Join(home, ".claude", "settings.json")
	claudeJSONPath := filepath.Join(home, ".claude.json")

	if _, err := os.Stat(settingsPath); err == nil {
		var skip bool
		if err := survey.AskOne(&survey.Confirm{
			Message: "Found ~/.claude/settings.json, skip model provider setup?",
			Default: true,
		}, &skip); err != nil {
			return handleSurveyErr(err)
		}
		if skip {
			return nil
		}
	}

	providerOptions := []string{
		ProviderMoonshot,
		ProviderDeepSeek,
		ProviderGLM,
		ProviderMiniMax,
		ProviderAliyun,
		ProviderVolcengine,
		ProviderTencent,
		ProviderCustom,
	}
	var provider string
	if err := survey.AskOne(&survey.Select{
		Message: "Select model provider:",
		Options: providerOptions,
	}, &provider); err != nil {
		return handleSurveyErr(err)
	}

	var env map[string]string
	needClaudeJSON := false

	switch provider {
	case ProviderMoonshot:
		apiKey, err := promptAPIKey("Moonshot API Key:")
		if err != nil {
			return err
		}
		env = moonshotEnv(apiKey)

	case ProviderDeepSeek:
		apiKey, err := promptAPIKey("DeepSeek API Key:")
		if err != nil {
			return err
		}
		env = deepseekEnv(apiKey)

	case ProviderGLM:
		apiKey, err := promptAPIKey("Zhipu API Key:")
		if err != nil {
			return err
		}
		env = glmEnv(apiKey)
		needClaudeJSON = true

	case ProviderMiniMax:
		apiKey, err := promptAPIKey("MiniMax API Key:")
		if err != nil {
			return err
		}
		env = minimaxEnv(apiKey)
		needClaudeJSON = true

	case ProviderAliyun:
		apiKey, err := promptAPIKey("Alibaba Cloud API Key:")
		if err != nil {
			return err
		}
		var model string
		if err := survey.AskOne(&survey.Select{
			Message: "Select model:",
			Options: []string{"qwen3.5-plus", "kimi-k2.5", "glm-5", "MiniMax-M2.5"},
			Default: "qwen3.5-plus",
		}, &model); err != nil {
			return handleSurveyErr(err)
		}
		env = aliyunEnv(apiKey, model)

	case ProviderVolcengine:
		apiKey, err := promptAPIKey("Volcengine API Key:")
		if err != nil {
			return err
		}
		var model string
		if err := survey.AskOne(&survey.Select{
			Message: "Select model:",
			Options: []string{
				"doubao-seed-2.0-code",
				"doubao-seed-2.0-pro",
				"doubao-seed-2.0-lite",
				"doubao-seed-code",
				"minimax-m2.5",
				"glm-4.7",
				"deepseek-v3.2",
				"kimi-k2.5",
			},
			Default: "doubao-seed-2.0-code",
		}, &model); err != nil {
			return handleSurveyErr(err)
		}
		env = volcengineEnv(apiKey, model)
		needClaudeJSON = true

	case ProviderTencent:
		apiKey, err := promptAPIKey("Tencent Cloud API Key:")
		if err != nil {
			return err
		}
		var model string
		if err := survey.AskOne(&survey.Select{
			Message: "Select model:",
			Options: []string{
				"tc-code-latest（auto）",
				"hunyuan-2.0-instruct",
				"hunyuan-2.0-thinking",
				"minimax-m2.5",
				"kimi-k2.5",
				"glm-5",
				"hunyuan-t1",
				"hunyuan-turbos",
			},
			Default: "tc-code-latest（auto）",
		}, &model); err != nil {
			return handleSurveyErr(err)
		}
		env = tencentEnv(apiKey, model)
		needClaudeJSON = true

	case ProviderCustom:
		baseURL, err := promptAPIKey("ANTHROPIC_BASE_URL:")
		if err != nil {
			return err
		}
		apiKey, err := promptAPIKey("ANTHROPIC_AUTH_TOKEN:")
		if err != nil {
			return err
		}
		env = customEnv(baseURL, apiKey)
	}

	if err := mergeClaudeSettings(settingsPath, env); err != nil {
		return fmt.Errorf("write settings.json: %w", err)
	}
	fmt.Println("Written ~/.claude/settings.json")

	if needClaudeJSON {
		if err := mergeClaudeJSON(claudeJSONPath); err != nil {
			return fmt.Errorf("write .claude.json: %w", err)
		}
		fmt.Println("Written ~/.claude.json")
	}

	return nil
}
```

- [ ] **Step 2: Verify it compiles**

```bash
go build ./internal/claude/...
```

Expected: no output (clean build).

---

## Task 4: Create `internal/claude/provider_test.go`

**Files:**
- Create: `internal/claude/provider_test.go`

- [ ] **Step 1: Write `internal/claude/provider_test.go`**

```go
package claude

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestProviderEnvMap_Moonshot(t *testing.T) {
	env := moonshotEnv("test-key-123")
	if env["ANTHROPIC_AUTH_TOKEN"] != "test-key-123" {
		t.Errorf("want test-key-123, got %q", env["ANTHROPIC_AUTH_TOKEN"])
	}
	if env["ANTHROPIC_BASE_URL"] != "https://api.moonshot.cn/anthropic" {
		t.Errorf("unexpected base url: %q", env["ANTHROPIC_BASE_URL"])
	}
	if env["ANTHROPIC_MODEL"] != "kimi-k2.5" {
		t.Errorf("unexpected model: %q", env["ANTHROPIC_MODEL"])
	}
}

func TestProviderEnvMap_GLM(t *testing.T) {
	env := glmEnv("glm-key")
	if env["ANTHROPIC_AUTH_TOKEN"] != "glm-key" {
		t.Errorf("want glm-key, got %q", env["ANTHROPIC_AUTH_TOKEN"])
	}
	if env["ANTHROPIC_BASE_URL"] != "https://open.bigmodel.cn/api/anthropic" {
		t.Errorf("unexpected base url: %q", env["ANTHROPIC_BASE_URL"])
	}
}

func TestProviderEnvMap_MiniMax(t *testing.T) {
	env := minimaxEnv("mm-key")
	if env["ANTHROPIC_AUTH_TOKEN"] != "mm-key" {
		t.Errorf("want mm-key, got %q", env["ANTHROPIC_AUTH_TOKEN"])
	}
	if env["ANTHROPIC_MODEL"] != "MiniMax-M2.7" {
		t.Errorf("unexpected model: %q", env["ANTHROPIC_MODEL"])
	}
	if env["ANTHROPIC_DEFAULT_HAIKU_MODEL"] != "MiniMax-M2.7" {
		t.Errorf("unexpected haiku model: %q", env["ANTHROPIC_DEFAULT_HAIKU_MODEL"])
	}
}

func TestProviderEnvMap_DeepSeek(t *testing.T) {
	env := deepseekEnv("ds-key-456")
	if env["ANTHROPIC_AUTH_TOKEN"] != "ds-key-456" {
		t.Errorf("want ds-key-456, got %q", env["ANTHROPIC_AUTH_TOKEN"])
	}
	if env["ANTHROPIC_BASE_URL"] != "https://api.deepseek.com/anthropic" {
		t.Errorf("unexpected base url: %q", env["ANTHROPIC_BASE_URL"])
	}
	if env["ANTHROPIC_MODEL"] != "deepseek-chat" {
		t.Errorf("unexpected model: %q", env["ANTHROPIC_MODEL"])
	}
	if env["ANTHROPIC_SMALL_FAST_MODEL"] != "deepseek-chat" {
		t.Errorf("unexpected small fast model: %q", env["ANTHROPIC_SMALL_FAST_MODEL"])
	}
	if env["API_TIMEOUT_MS"] != "600000" {
		t.Errorf("unexpected timeout: %q", env["API_TIMEOUT_MS"])
	}
	if env["CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC"] != "1" {
		t.Errorf("unexpected traffic flag: %q", env["CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC"])
	}
}

func TestProviderEnvMap_Aliyun(t *testing.T) {
	env := aliyunEnv("ali-key-789", "qwen3.5-plus")
	if env["ANTHROPIC_AUTH_TOKEN"] != "ali-key-789" {
		t.Errorf("want ali-key-789, got %q", env["ANTHROPIC_AUTH_TOKEN"])
	}
	if env["ANTHROPIC_BASE_URL"] != "https://coding.dashscope.aliyuncs.com/apps/anthropic" {
		t.Errorf("unexpected base url: %q", env["ANTHROPIC_BASE_URL"])
	}
	if env["ANTHROPIC_MODEL"] != "qwen3.5-plus" {
		t.Errorf("unexpected model: %q", env["ANTHROPIC_MODEL"])
	}
	if env["CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC"] != "1" {
		t.Errorf("unexpected traffic flag: %q", env["CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC"])
	}
	if len(env) != 4 {
		t.Errorf("aliyun env should have exactly 4 keys, got %d", len(env))
	}
	env2 := aliyunEnv("ali-key", "MiniMax-M2.5")
	if env2["ANTHROPIC_MODEL"] != "MiniMax-M2.5" {
		t.Errorf("model not propagated: want MiniMax-M2.5, got %q", env2["ANTHROPIC_MODEL"])
	}
}

func TestProviderEnvMap_Volcengine(t *testing.T) {
	env := volcengineEnv("volc-key-123", "doubao-seed-2.0-code")
	if env["ANTHROPIC_AUTH_TOKEN"] != "volc-key-123" {
		t.Errorf("want volc-key-123, got %q", env["ANTHROPIC_AUTH_TOKEN"])
	}
	if env["ANTHROPIC_BASE_URL"] != "https://ark.cn-beijing.volces.com/api/coding" {
		t.Errorf("unexpected base url: %q", env["ANTHROPIC_BASE_URL"])
	}
	if env["ANTHROPIC_MODEL"] != "doubao-seed-2.0-code" {
		t.Errorf("unexpected model: %q", env["ANTHROPIC_MODEL"])
	}
	if env["CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC"] != "1" {
		t.Errorf("unexpected traffic flag: %q", env["CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC"])
	}
	if len(env) != 4 {
		t.Errorf("volcengine env should have exactly 4 keys, got %d", len(env))
	}
	env2 := volcengineEnv("volc-key", "kimi-k2.5")
	if env2["ANTHROPIC_MODEL"] != "kimi-k2.5" {
		t.Errorf("model not propagated: want kimi-k2.5, got %q", env2["ANTHROPIC_MODEL"])
	}
}

func TestProviderEnvMap_Tencent(t *testing.T) {
	env := tencentEnv("tc-key-456", "tc-code-latest（auto）")
	if env["ANTHROPIC_AUTH_TOKEN"] != "tc-key-456" {
		t.Errorf("want tc-key-456, got %q", env["ANTHROPIC_AUTH_TOKEN"])
	}
	if env["ANTHROPIC_BASE_URL"] != "https://api.lkeap.cloud.tencent.com/coding/anthropic" {
		t.Errorf("unexpected base url: %q", env["ANTHROPIC_BASE_URL"])
	}
	if env["ANTHROPIC_MODEL"] != "tc-code-latest（auto）" {
		t.Errorf("unexpected model: %q", env["ANTHROPIC_MODEL"])
	}
	if env["CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC"] != "1" {
		t.Errorf("unexpected traffic flag: %q", env["CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC"])
	}
	if len(env) != 4 {
		t.Errorf("tencent env should have exactly 4 keys, got %d", len(env))
	}
	env2 := tencentEnv("tc-key", "hunyuan-2.0-instruct")
	if env2["ANTHROPIC_MODEL"] != "hunyuan-2.0-instruct" {
		t.Errorf("model not propagated: want hunyuan-2.0-instruct, got %q", env2["ANTHROPIC_MODEL"])
	}
}

func TestProviderEnvMap_Custom(t *testing.T) {
	env := customEnv("https://my.api/v1", "my-key")
	if env["ANTHROPIC_BASE_URL"] != "https://my.api/v1" {
		t.Errorf("unexpected base url: %q", env["ANTHROPIC_BASE_URL"])
	}
	if env["ANTHROPIC_AUTH_TOKEN"] != "my-key" {
		t.Errorf("want my-key, got %q", env["ANTHROPIC_AUTH_TOKEN"])
	}
	if len(env) != 2 {
		t.Errorf("custom env should have exactly 2 keys, got %d", len(env))
	}
}

func TestMergeSettingsJSON_NewFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".claude", "settings.json")

	env := moonshotEnv("key1")
	if err := mergeClaudeSettings(path, env); err != nil {
		t.Fatalf("mergeClaudeSettings: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	envMap, ok := result["env"].(map[string]any)
	if !ok {
		t.Fatal("env key missing or wrong type")
	}
	if envMap["ANTHROPIC_AUTH_TOKEN"] != "key1" {
		t.Errorf("want key1, got %v", envMap["ANTHROPIC_AUTH_TOKEN"])
	}
}

func TestMergeSettingsJSON_PreservesExistingKeys(t *testing.T) {
	dir := t.TempDir()
	claudeDir := filepath.Join(dir, ".claude")
	os.MkdirAll(claudeDir, 0755)
	path := filepath.Join(claudeDir, "settings.json")

	existing := map[string]any{
		"allowedTools": []string{"Read", "Write"},
		"env": map[string]any{
			"SOME_OTHER_VAR": "keep-me",
		},
	}
	data, _ := json.MarshalIndent(existing, "", "  ")
	os.WriteFile(path, data, 0644)

	env := customEnv("https://api.test.com", "new-key")
	if err := mergeClaudeSettings(path, env); err != nil {
		t.Fatalf("mergeClaudeSettings: %v", err)
	}

	data, _ = os.ReadFile(path)
	var result map[string]any
	json.Unmarshal(data, &result)

	if result["allowedTools"] == nil {
		t.Error("allowedTools was lost during merge")
	}
	envMap := result["env"].(map[string]any)
	if envMap["ANTHROPIC_AUTH_TOKEN"] != "new-key" {
		t.Errorf("want new-key, got %v", envMap["ANTHROPIC_AUTH_TOKEN"])
	}
	if envMap["SOME_OTHER_VAR"] != "keep-me" {
		t.Errorf("SOME_OTHER_VAR was lost during merge")
	}
}

func TestMergeClaudeJSON_NewFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".claude.json")

	if err := mergeClaudeJSON(path); err != nil {
		t.Fatalf("mergeClaudeJSON: %v", err)
	}

	data, _ := os.ReadFile(path)
	var result map[string]any
	json.Unmarshal(data, &result)

	if result["hasCompletedOnboarding"] != true {
		t.Errorf("want true, got %v", result["hasCompletedOnboarding"])
	}
}

func TestMergeClaudeJSON_PreservesExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".claude.json")

	existing := map[string]any{"someKey": "someValue"}
	data, _ := json.MarshalIndent(existing, "", "  ")
	os.WriteFile(path, data, 0644)

	if err := mergeClaudeJSON(path); err != nil {
		t.Fatalf("mergeClaudeJSON: %v", err)
	}

	data, _ = os.ReadFile(path)
	var result map[string]any
	json.Unmarshal(data, &result)

	if result["someKey"] != "someValue" {
		t.Error("existing key was lost")
	}
	if result["hasCompletedOnboarding"] != true {
		t.Error("hasCompletedOnboarding not set")
	}
}

func TestMergeSettingsJSON_CleansOldProviderKeys(t *testing.T) {
	dir := t.TempDir()
	claudeDir := filepath.Join(dir, ".claude")
	os.MkdirAll(claudeDir, 0755)
	path := filepath.Join(claudeDir, "settings.json")

	existing := map[string]any{
		"allowedTools": []string{"Read", "Write"},
		"env": map[string]any{
			"ANTHROPIC_AUTH_TOKEN":                    "minimax-key",
			"ANTHROPIC_BASE_URL":                      "https://api.minimaxi.com/anthropic",
			"ANTHROPIC_MODEL":                         "MiniMax-M2.7",
			"ANTHROPIC_SMALL_FAST_MODEL":              "MiniMax-M2.7",
			"ANTHROPIC_DEFAULT_SONNET_MODEL":          "MiniMax-M2.7",
			"ANTHROPIC_DEFAULT_OPUS_MODEL":            "MiniMax-M2.7",
			"ANTHROPIC_DEFAULT_HAIKU_MODEL":           "MiniMax-M2.7",
			"API_TIMEOUT_MS":                          "3000000",
			"CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1",
			"SOME_CUSTOM_VAR":                         "keep-me",
		},
	}
	data, _ := json.MarshalIndent(existing, "", "  ")
	os.WriteFile(path, data, 0644)

	env := glmEnv("glm-key")
	if err := mergeClaudeSettings(path, env); err != nil {
		t.Fatalf("mergeClaudeSettings: %v", err)
	}

	data, _ = os.ReadFile(path)
	var result map[string]any
	json.Unmarshal(data, &result)

	if result["allowedTools"] == nil {
		t.Error("allowedTools was lost during merge")
	}
	envMap := result["env"].(map[string]any)

	if envMap["ANTHROPIC_AUTH_TOKEN"] != "glm-key" {
		t.Errorf("want glm-key, got %v", envMap["ANTHROPIC_AUTH_TOKEN"])
	}
	if envMap["ANTHROPIC_BASE_URL"] != "https://open.bigmodel.cn/api/anthropic" {
		t.Errorf("want GLM base URL, got %v", envMap["ANTHROPIC_BASE_URL"])
	}

	for _, key := range []string{
		"ANTHROPIC_MODEL",
		"ANTHROPIC_SMALL_FAST_MODEL",
		"ANTHROPIC_DEFAULT_SONNET_MODEL",
		"ANTHROPIC_DEFAULT_OPUS_MODEL",
		"ANTHROPIC_DEFAULT_HAIKU_MODEL",
	} {
		if _, exists := envMap[key]; exists {
			t.Errorf("stale key %s should have been removed after switching to GLM", key)
		}
	}

	if envMap["SOME_CUSTOM_VAR"] != "keep-me" {
		t.Errorf("SOME_CUSTOM_VAR was lost during merge")
	}
}
```

- [ ] **Step 2: Run the new tests**

```bash
go test ./internal/claude/... -v
```

Expected: all tests PASS (including the invoker tests that were already there).

---

## Task 5: Refactor `cmd/openbee/config_claude.go`

**Files:**
- Modify: `cmd/openbee/config_claude.go`

The file keeps only the cmd-layer interaction logic. All extracted functions are removed and replaced with calls to `internal/claude`.

- [ ] **Step 1: Replace the entire content of `cmd/openbee/config_claude.go`**

```go
package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"

	"github.com/AlecAivazis/survey/v2"
	claude "github.com/theopenbee/openbee/internal/claude"
)

// configureClaudeExecutable handles Step 2a:
// 1. Auto-detect claude in PATH
// 2. If not found: manual input or download
// 3. Prompt for timeout
func configureClaudeExecutable(vals *configValues) error {
	if claudePath, err := exec.LookPath("claude"); err == nil {
		fmt.Printf("Found Claude in PATH: %s, using it automatically.\n", claudePath)
		vals.ClaudePath = claudePath
	} else {
		var method string
		if err := survey.AskOne(&survey.Select{
			Message: "Claude not found, how would you like to get it?",
			Options: []string{"Enter path manually", "Download Claude"},
		}, &method); err != nil {
			return handleSurveyErr(err)
		}

		switch method {
		case "Enter path manually":
			if err := promptClaudeManualPath(vals); err != nil {
				return err
			}
		case "Download Claude":
			path, err := claude.Download(openbeeStateDir(), false)
			if err != nil {
				fmt.Printf("Download failed: %v\n", err)
				fmt.Println("Please enter the Claude path manually.")
				if err := promptClaudeManualPath(vals); err != nil {
					return err
				}
			} else {
				vals.ClaudePath = path
			}
		}
	}

	if err := survey.AskOne(&survey.Input{
		Message: "Claude timeout:",
		Default: vals.ClaudeTimeout,
	}, &vals.ClaudeTimeout); err != nil {
		return handleSurveyErr(err)
	}

	return nil
}

func promptClaudeManualPath(vals *configValues) error {
	if err := survey.AskOne(&survey.Input{
		Message: "Claude executable path:",
		Default: vals.ClaudePath,
	}, &vals.ClaudePath, survey.WithValidator(func(val any) error {
		path, _ := val.(string)
		info, err := os.Stat(path)
		if err != nil {
			return fmt.Errorf("file not found: %s", path)
		}
		if info.IsDir() {
			return fmt.Errorf("path is a directory, not a file: %s", path)
		}
		if info.Mode()&0111 == 0 {
			return fmt.Errorf("file is not executable: %s", path)
		}
		return nil
	})); err != nil {
		return handleSurveyErr(err)
	}
	return nil
}

func configureClaudeProvider(_ *configValues) error {
	if err := claude.ConfigureProvider(); err != nil {
		if errors.Is(err, claude.ErrInterrupted) {
			return errInterrupted
		}
		return err
	}
	return nil
}
```

- [ ] **Step 2: Verify the whole module builds**

```bash
go build ./...
```

Expected: no output (clean build). If you see "declared and not used" or "undefined" errors, fix them before continuing.

---

## Task 6: Update `cmd/openbee/config_claude_test.go`

**Files:**
- Modify: `cmd/openbee/config_claude_test.go`

All tests in this file have moved to `internal/claude/`. The file should be replaced with a minimal stub that keeps the package declaration.

- [ ] **Step 1: Replace `cmd/openbee/config_claude_test.go` with an empty test file**

```go
package main
```

- [ ] **Step 2: Run all tests to confirm nothing is broken**

```bash
go test ./...
```

Expected: all tests PASS, no failures.

---

## Task 7: Create `cmd/openbee/claude.go`

**Files:**
- Create: `cmd/openbee/claude.go`

- [ ] **Step 1: Write `cmd/openbee/claude.go`**

```go
package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	claude "github.com/theopenbee/openbee/internal/claude"
)

var claudeCmd = &cobra.Command{
	Use:   "claude",
	Short: "Manage Claude Code installation and provider configuration",
}

var claudeDownloadForce bool

var claudeDownloadCmd = &cobra.Command{
	Use:   "download",
	Short: "Download Claude Code binary to ~/.openbee/bin/claude",
	RunE: func(cmd *cobra.Command, args []string) error {
		stateDir := openbeeStateDir()
		destPath := filepath.Join(stateDir, "bin", "claude")
		if !claudeDownloadForce {
			if _, err := os.Stat(destPath); err == nil {
				fmt.Printf("Claude is already installed at %s\n", destPath)
				fmt.Println("Use --force to re-download.")
				return nil
			}
		}
		path, err := claude.Download(stateDir, claudeDownloadForce)
		if err != nil {
			return err
		}
		fmt.Printf("Claude installed at: %s\n", path)
		return nil
	},
}

var claudeEnvCmd = &cobra.Command{
	Use:   "env",
	Short: "Configure Claude Code provider and environment settings",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := claude.ConfigureProvider(); err != nil {
			if errors.Is(err, claude.ErrInterrupted) {
				return nil
			}
			return err
		}
		return nil
	},
}

func init() {
	claudeDownloadCmd.Flags().BoolVar(&claudeDownloadForce, "force", false, "Force re-download even if already installed")
	claudeCmd.AddCommand(claudeDownloadCmd)
	claudeCmd.AddCommand(claudeEnvCmd)
	rootCmd.AddCommand(claudeCmd)
}
```

- [ ] **Step 2: Build and verify the binary**

```bash
go build ./... && go build -o /tmp/openbee-test ./cmd/openbee && /tmp/openbee-test claude --help
```

Expected output should include:
```
Manage Claude Code installation and provider configuration

Usage:
  openbee claude [command]

Available Commands:
  download    Download Claude Code binary to ~/.openbee/bin/claude
  env         Configure Claude Code provider and environment settings
```

- [ ] **Step 3: Verify subcommand help**

```bash
/tmp/openbee-test claude download --help
```

Expected: shows `--force` flag.

```bash
/tmp/openbee-test claude env --help
```

Expected: shows the command description with no extra flags.

- [ ] **Step 4: Run the full test suite one final time**

```bash
go test ./...
```

Expected: all tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/claude/download.go internal/claude/download_test.go \
        internal/claude/provider.go internal/claude/provider_test.go \
        cmd/openbee/claude.go cmd/openbee/config_claude.go \
        cmd/openbee/config_claude_test.go
git commit -m "feat(claude): add openbee claude download/env subcommands

Extract download and provider-config logic from config_claude.go into
internal/claude package. Add openbee claude download and openbee claude
env subcommands as independent entry points.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>"
```

---

## Self-Review

**Spec coverage check:**
- ✅ `internal/claude/download.go` — `Download()` public func, all helpers
- ✅ `internal/claude/provider.go` — `ConfigureProvider()`, `ErrInterrupted`, provider constants
- ✅ `cmd/openbee/claude.go` — `openbee claude`, `claude download --force`, `claude env`
- ✅ `config_claude.go` refactored to delegate to `internal/claude`
- ✅ `config` command behavior unchanged (still calls `claude.Download` + `claude.ConfigureProvider` via the same wrappers)
- ✅ Tests moved to `internal/claude` where they test the same logic

**Type consistency check:**
- `claude.Download(stateDir string, force bool) (string, error)` — used consistently in Task 5 and Task 7
- `claude.ConfigureProvider() error` — used consistently in Task 5 and Task 7
- `claude.ErrInterrupted` — used consistently in Task 5 and Task 7
- `gitHubRelBase` (private const) — referenced in `download_test.go` Task 2, defined in Task 1 ✅
- `GitHubAPI` (public var) — referenced in `download_test.go` Task 2 as `GitHubAPI`, defined in Task 1 ✅

**No placeholders** — all steps contain actual code.
