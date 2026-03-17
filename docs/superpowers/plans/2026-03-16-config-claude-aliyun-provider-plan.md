# Config Claude: Add Aliyun (Qianwen) Provider — Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add Alibaba Cloud (Qianwen) as a model provider in the `robobee config` claude configuration wizard, with user-selectable model.

**Architecture:** Add `aliyunEnv(apiKey, model string)` env builder function and a new switch-case in `configureClaudeProvider()` that prompts for API key and model selection before calling the builder.

**Tech Stack:** Go, AlecAivazis/survey/v2

**Spec:** `docs/superpowers/specs/2026-03-16-config-claude-aliyun-provider-design.md`

---

### Task 1: Add `aliyunEnv` function and test

**Files:**
- Modify: `cmd/robobee/config_claude.go:63` (after `deepseekEnv`, before `customEnv`)
- Modify: `cmd/robobee/config_claude_test.go:66` (after `TestProviderEnvMap_DeepSeek`)

- [ ] **Step 1: Write the failing test**

Add to `cmd/robobee/config_claude_test.go` after `TestProviderEnvMap_DeepSeek` (line 66):

```go
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

	// Verify non-default model propagation
	env2 := aliyunEnv("ali-key", "MiniMax-M2.5")
	if env2["ANTHROPIC_MODEL"] != "MiniMax-M2.5" {
		t.Errorf("model not propagated: want MiniMax-M2.5, got %q", env2["ANTHROPIC_MODEL"])
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/robobee/ -run TestProviderEnvMap_Aliyun -v`
Expected: FAIL — `undefined: aliyunEnv`

- [ ] **Step 3: Write the `aliyunEnv` function**

Add to `cmd/robobee/config_claude.go` after `deepseekEnv` (line 63), before `customEnv`:

```go
func aliyunEnv(apiKey, model string) map[string]string {
	return map[string]string{
		"ANTHROPIC_AUTH_TOKEN":                     apiKey,
		"ANTHROPIC_BASE_URL":                      "https://coding.dashscope.aliyuncs.com/apps/anthropic",
		"ANTHROPIC_MODEL":                          model,
		"CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1",
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./cmd/robobee/ -run TestProviderEnvMap_Aliyun -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add cmd/robobee/config_claude.go cmd/robobee/config_claude_test.go
git commit -m "feat(config): add aliyunEnv function for Aliyun provider"
```

---

### Task 2: Add Aliyun provider to `configureClaudeProvider()` switch-case

**Files:**
- Modify: `cmd/robobee/config_claude.go:271-277` (provider options list)
- Modify: `cmd/robobee/config_claude.go:285-337` (switch-case block)

- [ ] **Step 1: Add `"阿里云（千问）"` to the provider options list**

In `configureClaudeProvider()`, find the `Options` slice (line 271-277) and add the new entry after `"稀宇科技（MiniMax）"` and before `"自定义服务商"`:

```go
Options: []string{
	"月之暗面（Kimi）",
	"深度求索（DeepSeek）",
	"智谱清言（GLM）",
	"稀宇科技（MiniMax）",
	"阿里云（千问）",
	"自定义服务商",
},
```

- [ ] **Step 2: Add the switch-case block for Aliyun**

After the `"稀宇科技（MiniMax）"` case block (line 321) and before the `"自定义服务商"` case (line 324), add:

```go
case "阿里云（千问）":
	var apiKey string
	if err := survey.AskOne(&survey.Input{
		Message: "阿里云 API Key:",
	}, &apiKey, survey.WithValidator(survey.Required)); err != nil {
		return handleSurveyErr(err)
	}
	var model string
	if err := survey.AskOne(&survey.Select{
		Message: "选择模型:",
		Options: []string{"qwen3.5-plus", "kimi-k2.5", "glm-5", "MiniMax-M2.5"},
		Default: "qwen3.5-plus",
	}, &model); err != nil {
		return handleSurveyErr(err)
	}
	env = aliyunEnv(apiKey, model)
```

- [ ] **Step 3: Run all tests to verify nothing is broken**

Run: `go test ./cmd/robobee/ -v`
Expected: All tests PASS

- [ ] **Step 4: Commit**

```bash
git add cmd/robobee/config_claude.go
git commit -m "feat(config): add Aliyun (Qianwen) as Claude model provider option"
```
