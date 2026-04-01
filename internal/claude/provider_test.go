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
	} {
		if _, exists := envMap[key]; exists {
			t.Errorf("stale key %s should have been removed after switching to GLM", key)
		}
	}

	if envMap["ANTHROPIC_DEFAULT_SONNET_MODEL"] != "glm-5-turbo" {
		t.Errorf("want glm-5-turbo for ANTHROPIC_DEFAULT_SONNET_MODEL, got %v", envMap["ANTHROPIC_DEFAULT_SONNET_MODEL"])
	}
	if envMap["ANTHROPIC_DEFAULT_OPUS_MODEL"] != "glm-5.1" {
		t.Errorf("want glm-5.1 for ANTHROPIC_DEFAULT_OPUS_MODEL, got %v", envMap["ANTHROPIC_DEFAULT_OPUS_MODEL"])
	}
	if envMap["ANTHROPIC_DEFAULT_HAIKU_MODEL"] != "glm-4.5-air" {
		t.Errorf("want glm-4.5-air for ANTHROPIC_DEFAULT_HAIKU_MODEL, got %v", envMap["ANTHROPIC_DEFAULT_HAIKU_MODEL"])
	}

	if envMap["SOME_CUSTOM_VAR"] != "keep-me" {
		t.Errorf("SOME_CUSTOM_VAR was lost during merge")
	}
}
