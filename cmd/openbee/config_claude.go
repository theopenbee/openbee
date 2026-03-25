package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/AlecAivazis/survey/v2"
)

// claudeGitHubAPI is a var so tests can override it with a local httptest server.
var claudeGitHubAPI = "https://api.github.com/repos/theopenbee/cc-download/releases/latest"

const claudeGitHubRelBase = "https://github.com/theopenbee/cc-download/releases/download"

// Provider display names used in the selection menu and switch cases.
const (
	providerMoonshot   = "Moonshot (Kimi)"
	providerDeepSeek   = "DeepSeek"
	providerGLM        = "Zhipu (GLM)"
	providerMiniMax    = "MiniMax"
	providerAliyun     = "Alibaba Cloud (Qwen)"
	providerVolcengine = "Volcengine (Doubao)"
	providerTencent    = "Tencent Cloud"
	providerCustom     = "Custom provider"
)

// promptAPIKey asks the user for an API key with the given message.
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
		"ANTHROPIC_AUTH_TOKEN":                     apiKey,
		"ANTHROPIC_MODEL":                          "kimi-k2.5",
		"ANTHROPIC_SMALL_FAST_MODEL":               "kimi-k2.5",
		"ANTHROPIC_DEFAULT_OPUS_MODEL":             "kimi-k2.5",
		"ANTHROPIC_DEFAULT_SONNET_MODEL":           "kimi-k2.5",
		"ANTHROPIC_DEFAULT_HAIKU_MODEL":            "kimi-k2.5",
		"CLAUDE_CODE_SUBAGENT_MODEL":               "kimi-k2.5",
		"CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1",
		"ENABLE_TOOL_SEARCH":                       "false",
		"API_TIMEOUT_MS":                           "600000",
	}
}

func glmEnv(apiKey string) map[string]string {
	return map[string]string{
		"ANTHROPIC_AUTH_TOKEN":                     apiKey,
		"ANTHROPIC_BASE_URL":                      "https://open.bigmodel.cn/api/anthropic",
		"API_TIMEOUT_MS":                           "3000000",
		"CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1",
	}
}

func minimaxEnv(apiKey string) map[string]string {
	return map[string]string{
		"ANTHROPIC_BASE_URL":                      "https://api.minimaxi.com/anthropic",
		"ANTHROPIC_AUTH_TOKEN":                     apiKey,
		"API_TIMEOUT_MS":                           "3000000",
		"CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1",
		"ANTHROPIC_MODEL":                          "MiniMax-M2.7",
		"ANTHROPIC_SMALL_FAST_MODEL":               "MiniMax-M2.7",
		"ANTHROPIC_DEFAULT_SONNET_MODEL":           "MiniMax-M2.7",
		"ANTHROPIC_DEFAULT_OPUS_MODEL":             "MiniMax-M2.7",
		"ANTHROPIC_DEFAULT_HAIKU_MODEL":            "MiniMax-M2.7",
	}
}

func deepseekEnv(apiKey string) map[string]string {
	return map[string]string{
		"ANTHROPIC_BASE_URL":                      "https://api.deepseek.com/anthropic",
		"ANTHROPIC_AUTH_TOKEN":                     apiKey,
		"ANTHROPIC_MODEL":                          "deepseek-chat",
		"ANTHROPIC_SMALL_FAST_MODEL":               "deepseek-chat",
		"CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1",
		"API_TIMEOUT_MS":                           "600000",
	}
}

// standardProviderEnv builds an env map for providers that need only base URL, API key, and model.
func standardProviderEnv(baseURL, apiKey, model string) map[string]string {
	return map[string]string{
		"ANTHROPIC_AUTH_TOKEN":                     apiKey,
		"ANTHROPIC_BASE_URL":                       baseURL,
		"ANTHROPIC_MODEL":                          model,
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

// mergeJSONFile reads the JSON file at path (if it exists) into a map,
// calls apply to mutate the map, then writes it back with indentation.
// If the file contains invalid JSON it is overwritten with a warning.
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

// providerEnvKeys lists all environment variable keys that any provider may
// write. These are cleared before writing new provider settings so that stale
// keys from a previous provider do not linger after switching.
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

// mergeClaudeSettings reads existing ~/.claude/settings.json (if any),
// removes all known provider env keys, merges the provided env map into
// the "env" key, preserves all other keys, and writes back.
// Creates parent directories if needed.
func mergeClaudeSettings(path string, env map[string]string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}
	return mergeJSONFile(path, func(m map[string]any) {
		envMap, ok := m["env"].(map[string]any)
		if !ok {
			envMap = make(map[string]any)
		}
		// Remove all known provider keys to avoid stale values from
		// a previously configured provider.
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

// configureClaudeExecutable handles Step 2a:
// 1. Auto-detect claude in PATH
// 2. If not found: manual input or download
// 3. Prompt for timeout
func configureClaudeExecutable(vals *configValues) error {
	// Try auto-detect
	if claudePath, err := exec.LookPath("claude"); err == nil {
		fmt.Printf("Found Claude in PATH: %s, using it automatically.\n", claudePath)
		vals.ClaudePath = claudePath
	} else {
		// Not found — offer choices
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
			if err := downloadClaude(vals); err != nil {
				// Download failed — fallback to manual
				fmt.Printf("Download failed: %v\n", err)
				fmt.Println("Please enter the Claude path manually.")
				if err := promptClaudeManualPath(vals); err != nil {
					return err
				}
			}
		}
	}

	// Claude timeout
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
		// Check executable bit (Unix)
		if info.Mode()&0111 == 0 {
			return fmt.Errorf("file is not executable: %s", path)
		}
		return nil
	})); err != nil {
		return handleSurveyErr(err)
	}
	return nil
}

// fetchLatestClaudeVersion queries the GitHub Releases API for the latest cc-download tag.
func fetchLatestClaudeVersion() (string, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(claudeGitHubAPI)
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

// isSupportedPlatform checks if the given platform is in the supported list.
func isSupportedPlatform(p claudePlatform) bool {
	_, ok := supportedPlatforms[p]
	return ok
}

// detectPlatform builds a claudePlatform from the current runtime environment.
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

// mapArch converts Go's runtime.GOARCH to the download platform arch name.
func mapArch(goarch string) string {
	if goarch == "amd64" {
		return "x64"
	}
	return goarch
}

// isMuslWith checks if the system uses musl libc by looking for the musl dynamic linker.
// The globFunc parameter allows dependency injection for testing.
func isMuslWith(globFunc func(string) ([]string, error)) bool {
	matches, err := globFunc("/lib/ld-musl-*.so*")
	if err != nil {
		return false // fail-open: treat errors as non-musl
	}
	return len(matches) > 0
}

// isMusl checks if the current system uses musl libc.
func isMusl() bool {
	return isMuslWith(filepath.Glob)
}

// buildClaudeDownloadURL constructs the GitHub release asset URL for the given version and platform.
func buildClaudeDownloadURL(p claudePlatform, version string) string {
	versionNum := strings.TrimPrefix(version, "v")
	platformStr := p.os + "-" + p.arch
	if p.variant != "" {
		platformStr += "-" + p.variant
	}
	assetName := fmt.Sprintf("claude-%s-%s", versionNum, platformStr)
	return fmt.Sprintf("%s/%s/%s", claudeGitHubRelBase, version, assetName)
}

func downloadClaude(vals *configValues) error {
	// Platform validation
	platform := detectPlatform()
	if !isSupportedPlatform(platform) {
		return fmt.Errorf(
			"current platform (%s/%s) does not support automatic Claude Code download.\n"+
				"Supported platforms: darwin-arm64, darwin-x64, linux-arm64, linux-x64, linux-arm64-musl, linux-x64-musl\n"+
				"Please install manually.",
			runtime.GOOS, runtime.GOARCH,
		)
	}

	binDir := filepath.Join(openbeeStateDir(), "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}

	fmt.Println("Checking for latest Claude version...")
	version, err := fetchLatestClaudeVersion()
	if err != nil {
		return fmt.Errorf("fetch latest Claude version: %w", err)
	}
	fmt.Printf("Latest Claude version: %s\n", version)

	versionNum := strings.TrimPrefix(version, "v")
	platformStr := platform.os + "-" + platform.arch
	if platform.variant != "" {
		platformStr += "-" + platform.variant
	}

	checksumURL := fmt.Sprintf("%s/%s/checksums-sha256.txt", claudeGitHubRelBase, version)
	binaryURL := buildClaudeDownloadURL(platform, version)
	assetName := fmt.Sprintf("claude-%s-%s", versionNum, platformStr)

	// Temp dir for checksums file; cleaned up automatically.
	tmpDir, err := os.MkdirTemp("", "openbee-claude-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// Download checksums first (small file).
	checksumPath := filepath.Join(tmpDir, "checksums-sha256.txt")
	checksumAvailable := true
	if err := downloadFile(checksumURL, checksumPath, nil); err != nil {
		checksumAvailable = false
		fmt.Printf("warning: failed to download checksums-sha256.txt, skipping verification (%v)\n", err)
	}

	fmt.Printf("Downloading Claude %s (%s)...\n", version, platformStr)

	// Download binary while computing SHA256 in one pass.
	destPath := filepath.Join(binDir, "claude")
	tmpPath := destPath + ".tmp"
	h := sha256.New()
	if err := downloadFile(binaryURL, tmpPath, h); err != nil {
		return fmt.Errorf("download: %w", err)
	}

	// Verify checksum if available.
	if checksumAvailable {
		fmt.Println("Verifying SHA256...")
		data, err := os.ReadFile(checksumPath)
		if err != nil {
			return fmt.Errorf("read checksums: %w", err)
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
			return fmt.Errorf("no checksum for %s in checksums-sha256.txt", assetName)
		}
		if actual := hex.EncodeToString(h.Sum(nil)); actual != expected {
			os.Remove(tmpPath)
			return fmt.Errorf("SHA256 mismatch\n  expected: %s\n  got:      %s", expected, actual)
		}
		fmt.Println("SHA256 verified.")
	}

	if err := os.Chmod(tmpPath, 0755); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("set executable permission: %w", err)
	}

	if err := os.Rename(tmpPath, destPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("move file: %w", err)
	}

	vals.ClaudePath = destPath
	fmt.Printf("Claude downloaded to: %s\n", destPath)
	return nil
}

// configureClaudeProvider handles Step 2b:
// 1. Check if ~/.claude/settings.json exists
// 2. If exists: offer to skip or reconfigure
// 3. Select provider, collect API key, merge into settings.json
// 4. For providers that need onboarding bypass: also merge into ~/.claude.json
func configureClaudeProvider(vals *configValues) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home directory: %w", err)
	}

	settingsPath := filepath.Join(home, ".claude", "settings.json")
	claudeJSONPath := filepath.Join(home, ".claude.json")

	// Check existing settings
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

	// Select provider
	providerOptions := []string{
		providerMoonshot,
		providerDeepSeek,
		providerGLM,
		providerMiniMax,
		providerAliyun,
		providerVolcengine,
		providerTencent,
		providerCustom,
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
	case providerMoonshot:
		apiKey, err := promptAPIKey("Moonshot API Key:")
		if err != nil {
			return err
		}
		env = moonshotEnv(apiKey)

	case providerDeepSeek:
		apiKey, err := promptAPIKey("DeepSeek API Key:")
		if err != nil {
			return err
		}
		env = deepseekEnv(apiKey)

	case providerGLM:
		apiKey, err := promptAPIKey("Zhipu API Key:")
		if err != nil {
			return err
		}
		env = glmEnv(apiKey)
		needClaudeJSON = true

	case providerMiniMax:
		apiKey, err := promptAPIKey("MiniMax API Key:")
		if err != nil {
			return err
		}
		env = minimaxEnv(apiKey)
		needClaudeJSON = true

	case providerAliyun:
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

	case providerVolcengine:
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

	case providerTencent:
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

	case providerCustom:
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

	// Write settings.json
	if err := mergeClaudeSettings(settingsPath, env); err != nil {
		return fmt.Errorf("write settings.json: %w", err)
	}
	fmt.Println("Written ~/.claude/settings.json")

	// Write .claude.json if needed
	if needClaudeJSON {
		if err := mergeClaudeJSON(claudeJSONPath); err != nil {
			return fmt.Errorf("write .claude.json: %w", err)
		}
		fmt.Println("Written ~/.claude.json")
	}

	return nil
}
