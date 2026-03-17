# Config Claude 配置流程重设计 Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Move Claude configuration out of optional "advanced config" into a dedicated Step 2 after basic config, adding executable auto-detection/download and model provider configuration.

**Architecture:** Add two new functions `configureClaudeExecutable` and `configureClaudeProvider` to `config.go`. The executable config writes to `config.yaml` via `configValues`; the provider config writes directly to `~/.claude/settings.json` (and optionally `~/.claude.json`) using JSON merge strategy.

**Tech Stack:** Go, survey/v2 (interactive prompts), os/exec (LookPath), net/http (download), encoding/json (settings.json merge)

**Spec:** `docs/superpowers/specs/2026-03-16-config-claude-redesign-design.md`

---

### File Structure

| File | Action | Responsibility |
|------|--------|----------------|
| `cmd/robobee/config.go` | Modify | Main config flow, new Step 2, adjusted advanced config |
| `cmd/robobee/config_claude.go` | Create | `configureClaudeExecutable` and `configureClaudeProvider` functions |
| `cmd/robobee/config_claude_test.go` | Create | Tests for JSON merge, provider env maps, download path construction |

---

### Task 1: Extract Claude provider env constants and JSON merge helpers

**Files:**
- Create: `cmd/robobee/config_claude.go`
- Create: `cmd/robobee/config_claude_test.go`

- [ ] **Step 1: Write the test for provider env maps**

```go
// cmd/robobee/config_claude_test.go
package main

import (
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
	if env["ANTHROPIC_MODEL"] != "MiniMax-M2.5" {
		t.Errorf("unexpected model: %q", env["ANTHROPIC_MODEL"])
	}
	if env["ANTHROPIC_DEFAULT_HAIKU_MODEL"] != "MiniMax-M2.5" {
		t.Errorf("unexpected haiku model: %q", env["ANTHROPIC_DEFAULT_HAIKU_MODEL"])
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/tengyongzhi/work/bot-workspaces/robobee && go test ./cmd/robobee/ -run TestProviderEnvMap -v`
Expected: FAIL — functions not defined

- [ ] **Step 3: Implement provider env map functions**

In `cmd/robobee/config_claude.go`:

Note: Add imports incrementally per task. Task 1 only needs `package main`. Task 2 adds `encoding/json`, `fmt`, `os`, `path/filepath`. Task 3 adds `io`, `net/http`, `os/exec`, `runtime`, `survey/v2`. Task 4 adds nothing new.

```go
package main

const claudeDownloadURL = "https://example.com/claude/download"

// Provider env map builders — only ANTHROPIC_AUTH_TOKEN comes from user input.

func moonshotEnv(apiKey string) map[string]string {
	return map[string]string{
		"ANTHROPIC_BASE_URL":                      "https://api.moonshot.cn/anthropic",
		"ANTHROPIC_AUTH_TOKEN":                     apiKey,
		"ANTHROPIC_MODEL":                          "kimi-k2.5",
		"ANTHROPIC_SMALL_FAST_MODEL":               "kimi-k2.5",
		"CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1",
		"API_TIMEOUT_MS":                           "600000",
	}
}

func glmEnv(apiKey string) map[string]string {
	return map[string]string{
		"ANTHROPIC_AUTH_TOKEN":                     apiKey,
		"ANTHROPIC_BASE_URL":                       "https://open.bigmodel.cn/api/anthropic",
		"API_TIMEOUT_MS":                           "3000000",
		"CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1",
	}
}

func minimaxEnv(apiKey string) map[string]string {
	return map[string]string{
		"ANTHROPIC_BASE_URL":                       "https://api.minimaxi.com/anthropic",
		"ANTHROPIC_AUTH_TOKEN":                     apiKey,
		"API_TIMEOUT_MS":                           "3000000",
		"CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1",
		"ANTHROPIC_MODEL":                          "MiniMax-M2.5",
		"ANTHROPIC_SMALL_FAST_MODEL":               "MiniMax-M2.5",
		"ANTHROPIC_DEFAULT_SONNET_MODEL":           "MiniMax-M2.5",
		"ANTHROPIC_DEFAULT_OPUS_MODEL":             "MiniMax-M2.5",
		"ANTHROPIC_DEFAULT_HAIKU_MODEL":            "MiniMax-M2.5",
	}
}

func customEnv(baseURL, apiKey string) map[string]string {
	return map[string]string{
		"ANTHROPIC_BASE_URL":   baseURL,
		"ANTHROPIC_AUTH_TOKEN": apiKey,
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /Users/tengyongzhi/work/bot-workspaces/robobee && go test ./cmd/robobee/ -run TestProviderEnvMap -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add cmd/robobee/config_claude.go cmd/robobee/config_claude_test.go
git commit -m "feat(config): add claude provider env map functions"
```

---

### Task 2: Implement JSON merge helpers for settings.json and .claude.json

**Files:**
- Modify: `cmd/robobee/config_claude.go`
- Modify: `cmd/robobee/config_claude_test.go`

- [ ] **Step 1: Write tests for JSON merge functions**

Append to `cmd/robobee/config_claude_test.go`:

```go
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

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	envMap, ok := result["env"].(map[string]interface{})
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

	// Write existing file with extra keys
	existing := map[string]interface{}{
		"allowedTools": []string{"Read", "Write"},
		"env": map[string]interface{}{
			"SOME_OTHER_VAR": "keep-me",
		},
	}
	data, _ := json.MarshalIndent(existing, "", "  ")
	os.WriteFile(path, data, 0644)

	// Merge new env
	env := customEnv("https://api.test.com", "new-key")
	if err := mergeClaudeSettings(path, env); err != nil {
		t.Fatalf("mergeClaudeSettings: %v", err)
	}

	data, _ = os.ReadFile(path)
	var result map[string]interface{}
	json.Unmarshal(data, &result)

	// allowedTools preserved
	if result["allowedTools"] == nil {
		t.Error("allowedTools was lost during merge")
	}

	envMap := result["env"].(map[string]interface{})
	// New keys written
	if envMap["ANTHROPIC_AUTH_TOKEN"] != "new-key" {
		t.Errorf("want new-key, got %v", envMap["ANTHROPIC_AUTH_TOKEN"])
	}
	// Old env keys preserved
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
	var result map[string]interface{}
	json.Unmarshal(data, &result)

	if result["hasCompletedOnboarding"] != true {
		t.Errorf("want true, got %v", result["hasCompletedOnboarding"])
	}
}

func TestMergeClaudeJSON_PreservesExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".claude.json")

	existing := map[string]interface{}{"someKey": "someValue"}
	data, _ := json.MarshalIndent(existing, "", "  ")
	os.WriteFile(path, data, 0644)

	if err := mergeClaudeJSON(path); err != nil {
		t.Fatalf("mergeClaudeJSON: %v", err)
	}

	data, _ = os.ReadFile(path)
	var result map[string]interface{}
	json.Unmarshal(data, &result)

	if result["someKey"] != "someValue" {
		t.Error("existing key was lost")
	}
	if result["hasCompletedOnboarding"] != true {
		t.Error("hasCompletedOnboarding not set")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/tengyongzhi/work/bot-workspaces/robobee && go test ./cmd/robobee/ -run "TestMerge" -v`
Expected: FAIL — functions not defined

- [ ] **Step 3: Implement merge functions**

Add to `cmd/robobee/config_claude.go`:

```go
// mergeClaudeSettings reads existing ~/.claude/settings.json (if any),
// merges the provided env map into the "env" key, preserves all other keys,
// and writes back. Creates parent directories if needed.
func mergeClaudeSettings(path string, env map[string]string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("创建目录失败: %w", err)
	}

	existing := make(map[string]interface{})
	if data, err := os.ReadFile(path); err == nil {
		// Ignore malformed JSON — treat as empty
		json.Unmarshal(data, &existing)
	}

	// Get or create the env map
	envMap, ok := existing["env"].(map[string]interface{})
	if !ok {
		envMap = make(map[string]interface{})
	}

	// Merge new env values
	for k, v := range env {
		envMap[k] = v
	}
	existing["env"] = envMap

	data, err := json.MarshalIndent(existing, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化 JSON 失败: %w", err)
	}
	return os.WriteFile(path, append(data, '\n'), 0644)
}

// mergeClaudeJSON reads existing ~/.claude.json (if any),
// sets hasCompletedOnboarding=true, preserves all other keys, and writes back.
func mergeClaudeJSON(path string) error {
	existing := make(map[string]interface{})
	if data, err := os.ReadFile(path); err == nil {
		json.Unmarshal(data, &existing)
	}

	existing["hasCompletedOnboarding"] = true

	data, err := json.MarshalIndent(existing, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化 JSON 失败: %w", err)
	}
	return os.WriteFile(path, append(data, '\n'), 0644)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/tengyongzhi/work/bot-workspaces/robobee && go test ./cmd/robobee/ -run "TestMerge" -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add cmd/robobee/config_claude.go cmd/robobee/config_claude_test.go
git commit -m "feat(config): add JSON merge helpers for settings.json and .claude.json"
```

---

### Task 3: Implement `configureClaudeExecutable` function

**Files:**
- Modify: `cmd/robobee/config_claude.go`

- [ ] **Step 1: Implement `configureClaudeExecutable`**

Add to `cmd/robobee/config_claude.go`:

```go
// configureClaudeExecutable handles Step 2a:
// 1. Auto-detect claude in PATH
// 2. If not found: manual input or download
// 3. Prompt for timeout
func configureClaudeExecutable(vals *configValues) error {
	// Try auto-detect
	if claudePath, err := exec.LookPath("claude"); err == nil {
		fmt.Printf("已检测到系统安装的 Claude: %s，将自动使用。\n", claudePath)
		vals.ClaudePath = claudePath
	} else {
		// Not found — offer choices
		var method string
		if err := survey.AskOne(&survey.Select{
			Message: "未检测到 Claude，请选择获取方式:",
			Options: []string{"手动输入路径", "下载 Claude"},
		}, &method); err != nil {
			return handleSurveyErr(err)
		}

		switch method {
		case "手动输入路径":
			if err := promptClaudeManualPath(vals); err != nil {
				return err
			}
		case "下载 Claude":
			if err := downloadClaude(vals); err != nil {
				// Download failed — fallback to manual
				fmt.Printf("下载失败: %v\n", err)
				fmt.Println("请手动输入 Claude 路径。")
				if err := promptClaudeManualPath(vals); err != nil {
					return err
				}
			}
		}
	}

	// Claude timeout
	if err := survey.AskOne(&survey.Input{
		Message: "Claude 超时:",
		Default: vals.ClaudeTimeout,
	}, &vals.ClaudeTimeout); err != nil {
		return handleSurveyErr(err)
	}

	return nil
}

func promptClaudeManualPath(vals *configValues) error {
	if err := survey.AskOne(&survey.Input{
		Message: "Claude 可执行文件路径:",
		Default: vals.ClaudePath,
	}, &vals.ClaudePath, survey.WithValidator(func(val interface{}) error {
		path, _ := val.(string)
		info, err := os.Stat(path)
		if err != nil {
			return fmt.Errorf("文件不存在: %s", path)
		}
		if info.IsDir() {
			return fmt.Errorf("路径是目录而非文件: %s", path)
		}
		// Check executable bit (Unix)
		if info.Mode()&0111 == 0 {
			return fmt.Errorf("文件不可执行: %s", path)
		}
		return nil
	})); err != nil {
		return handleSurveyErr(err)
	}
	return nil
}

func downloadClaude(vals *configValues) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("获取用户目录失败: %w", err)
	}

	binDir := filepath.Join(home, ".robobee", "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		return fmt.Errorf("创建目录失败: %w", err)
	}

	destPath := filepath.Join(binDir, "claude")
	arch := runtime.GOARCH
	url := fmt.Sprintf("%s?arch=%s", claudeDownloadURL, arch)

	fmt.Printf("正在下载 Claude (%s)...\n", arch)

	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("请求下载地址失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("下载失败，状态码: %d", resp.StatusCode)
	}

	f, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("创建文件失败: %w", err)
	}
	defer f.Close()

	if _, err := io.Copy(f, resp.Body); err != nil {
		return fmt.Errorf("写入文件失败: %w", err)
	}

	if err := os.Chmod(destPath, 0755); err != nil {
		return fmt.Errorf("设置可执行权限失败: %w", err)
	}

	vals.ClaudePath = destPath
	fmt.Printf("Claude 已下载到: %s\n", destPath)
	return nil
}
```

- [ ] **Step 2: Verify compilation**

Run: `cd /Users/tengyongzhi/work/bot-workspaces/robobee && go build ./cmd/robobee/`
Expected: Build succeeds

- [ ] **Step 3: Commit**

```bash
git add cmd/robobee/config_claude.go
git commit -m "feat(config): implement configureClaudeExecutable with auto-detect/manual/download"
```

---

### Task 4: Implement `configureClaudeProvider` function

**Files:**
- Modify: `cmd/robobee/config_claude.go`

- [ ] **Step 1: Implement `configureClaudeProvider`**

Add to `cmd/robobee/config_claude.go`:

```go
// configureClaudeProvider handles Step 2b:
// 1. Check if ~/.claude/settings.json exists
// 2. If exists: offer to skip or reconfigure
// 3. Select provider, collect API key, merge into settings.json
// 4. For GLM/MiniMax: also merge into ~/.claude.json
func configureClaudeProvider() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("获取用户目录失败: %w", err)
	}

	settingsPath := filepath.Join(home, ".claude", "settings.json")
	claudeJSONPath := filepath.Join(home, ".claude.json")

	// Check existing settings
	if _, err := os.Stat(settingsPath); err == nil {
		var skip bool
		if err := survey.AskOne(&survey.Confirm{
			Message: "已检测到 Claude 配置文件 (~/.claude/settings.json)，是否跳过模型服务商配置？",
			Default: true,
		}, &skip); err != nil {
			return handleSurveyErr(err)
		}
		if skip {
			return nil
		}
	}

	// Select provider
	var provider string
	if err := survey.AskOne(&survey.Select{
		Message: "选择模型服务商:",
		Options: []string{
			"月之暗面（Kimi）",
			"智谱清言（GLM）",
			"稀宇科技（MiniMax）",
			"自定义服务商",
		},
	}, &provider); err != nil {
		return handleSurveyErr(err)
	}

	var env map[string]string
	needClaudeJSON := false

	switch provider {
	case "月之暗面（Kimi）":
		var apiKey string
		if err := survey.AskOne(&survey.Input{
			Message: "Moonshot API Key:",
		}, &apiKey, survey.WithValidator(survey.Required)); err != nil {
			return handleSurveyErr(err)
		}
		env = moonshotEnv(apiKey)

	case "智谱清言（GLM）":
		var apiKey string
		if err := survey.AskOne(&survey.Input{
			Message: "智谱 API Key:",
		}, &apiKey, survey.WithValidator(survey.Required)); err != nil {
			return handleSurveyErr(err)
		}
		env = glmEnv(apiKey)
		needClaudeJSON = true

	case "稀宇科技（MiniMax）":
		var apiKey string
		if err := survey.AskOne(&survey.Input{
			Message: "MiniMax API Key:",
		}, &apiKey, survey.WithValidator(survey.Required)); err != nil {
			return handleSurveyErr(err)
		}
		env = minimaxEnv(apiKey)
		needClaudeJSON = true

	case "自定义服务商":
		var baseURL, apiKey string
		if err := survey.AskOne(&survey.Input{
			Message: "ANTHROPIC_BASE_URL:",
		}, &baseURL, survey.WithValidator(survey.Required)); err != nil {
			return handleSurveyErr(err)
		}
		if err := survey.AskOne(&survey.Input{
			Message: "ANTHROPIC_AUTH_TOKEN:",
		}, &apiKey, survey.WithValidator(survey.Required)); err != nil {
			return handleSurveyErr(err)
		}
		env = customEnv(baseURL, apiKey)
	}

	// Write settings.json
	if err := mergeClaudeSettings(settingsPath, env); err != nil {
		return fmt.Errorf("写入 settings.json 失败: %w", err)
	}
	fmt.Println("已写入 ~/.claude/settings.json")

	// Write .claude.json if needed
	if needClaudeJSON {
		if err := mergeClaudeJSON(claudeJSONPath); err != nil {
			return fmt.Errorf("写入 .claude.json 失败: %w", err)
		}
		fmt.Println("已写入 ~/.claude.json")
	}

	return nil
}
```

- [ ] **Step 2: Verify compilation**

Run: `cd /Users/tengyongzhi/work/bot-workspaces/robobee && go build ./cmd/robobee/`
Expected: Build succeeds

- [ ] **Step 3: Commit**

```bash
git add cmd/robobee/config_claude.go
git commit -m "feat(config): implement configureClaudeProvider with 4 provider options"
```

---

### Task 5: Integrate into `runConfig` flow and adjust step numbering

**Files:**
- Modify: `cmd/robobee/config.go:101-323`

This task modifies the `runConfig` function to:
1. Insert Step 2 (Claude config) after Step 1 (basic config)
2. Renumber all subsequent steps
3. Remove Claude path and timeout from advanced config

- [ ] **Step 1: Insert Step 2 — Claude config — after basic config**

In `cmd/robobee/config.go`, after the Step 1 block (after the DB path prompt, before the `// Step 2 — MCP config` comment), insert:

```go
	// Step 2 — Claude config
	fmt.Println("\n=== Claude 配置 ===")

	if err := configureClaudeExecutable(&vals); err != nil {
		return err
	}
	if err := configureClaudeProvider(); err != nil {
		return err
	}
```

- [ ] **Step 2: Renumber Step 2 → Step 3 (MCP config)**

Find `// Step 2 — MCP config` and change to `// Step 3 — MCP config`.

- [ ] **Step 3: Renumber Step 3 → Step 4 (Platform config)**

Find `// Step 3 — Platform config` and change to `// Step 4 — Platform config`.

- [ ] **Step 4: Renumber Step 4 → Step 5 (Advanced config) and remove Claude prompts**

Find `// Step 4 — Advanced config` and change to `// Step 5 — Advanced config`.

Remove the Claude path and Claude timeout prompts from the `if customAdvanced` block (the first two `survey.AskOne` calls for "Claude 可执行文件路径:" and "Claude 超时:"). The block should only contain: Feeder timeout, message debounce, FFprobe path, FFmpeg path.

- [ ] **Step 5: Renumber Step 5 → Step 6 (Confirm write)**

Find `// Step 5 — Confirm write` and change to `// Step 6 — Confirm write`.

- [ ] **Step 6: Verify compilation and run existing tests**

Run: `cd /Users/tengyongzhi/work/bot-workspaces/robobee && go build ./cmd/robobee/ && go test ./cmd/robobee/ -v && go test ./internal/config/ -v`
Expected: Build succeeds, all tests pass

- [ ] **Step 7: Commit**

```bash
git add cmd/robobee/config.go
git commit -m "feat(config): integrate Claude config as Step 2, renumber remaining steps"
```

---

### Task 6: Run full test suite and verify

**Files:** None (verification only)

- [ ] **Step 1: Run all tests**

Run: `cd /Users/tengyongzhi/work/bot-workspaces/robobee && go test ./... -v`
Expected: All tests pass

- [ ] **Step 2: Verify build**

Run: `cd /Users/tengyongzhi/work/bot-workspaces/robobee && go build -o /dev/null ./cmd/robobee/`
Expected: Build succeeds with no warnings

- [ ] **Step 3: Quick manual verification of flow structure**

Read `cmd/robobee/config.go` to confirm the step order is:
1. Basic config
2. Claude config (new)
3. MCP config
4. Platform config
5. Advanced config (no Claude path/timeout)
6. Confirm write
