package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/AlecAivazis/survey/v2"
)

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
		"ANTHROPIC_MODEL":                          "MiniMax-M2.5",
		"ANTHROPIC_SMALL_FAST_MODEL":               "MiniMax-M2.5",
		"ANTHROPIC_DEFAULT_SONNET_MODEL":           "MiniMax-M2.5",
		"ANTHROPIC_DEFAULT_OPUS_MODEL":             "MiniMax-M2.5",
		"ANTHROPIC_DEFAULT_HAIKU_MODEL":            "MiniMax-M2.5",
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

func customEnv(baseURL, apiKey string) map[string]string {
	return map[string]string{
		"ANTHROPIC_BASE_URL":   baseURL,
		"ANTHROPIC_AUTH_TOKEN": apiKey,
	}
}

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
			"深度求索（DeepSeek）",
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

	case "深度求索（DeepSeek）":
		var apiKey string
		if err := survey.AskOne(&survey.Input{
			Message: "DeepSeek API Key:",
		}, &apiKey, survey.WithValidator(survey.Required)); err != nil {
			return handleSurveyErr(err)
		}
		env = deepseekEnv(apiKey)

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
