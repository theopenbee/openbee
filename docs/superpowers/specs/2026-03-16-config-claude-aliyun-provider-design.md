# Config Claude: Add Aliyun (Qianwen) Provider

**Date:** 2026-03-16
**Status:** Draft

## Summary

Add Alibaba Cloud (Qianwen/千问) as a new model provider option in the `robobee config` claude configuration step (Step 2b). Unlike existing providers that use a fixed model, Aliyun requires the user to select from multiple available models.

## Background

The `configureClaudeProvider()` function in `cmd/robobee/config_claude.go` currently supports 5 providers: Moonshot, DeepSeek, GLM, MiniMax, and Custom. Each provider defines an env map builder function and a corresponding switch-case in the provider selection flow.

## Design

### Approach: B — Collect model in switch-case, pass to env function

The model selection prompt is placed inside the switch-case block of `configureClaudeProvider()`, before calling the env builder. This keeps the interactive logic centralized and follows the existing pattern.

**Why not Approach A (env function handles model internally)?** — The env builder functions are pure data mappers with no I/O. Putting the interactive `survey.Select` inside `aliyunEnv` would break that convention. The two-parameter signature `(apiKey, model string)` is a deliberate deviation from the existing single-parameter pattern, necessary because Aliyun is a model marketplace supporting multiple vendors' models through one endpoint.

**Why not Approach C (closure/default)?** — Over-engineered for a simple requirement.

### New `aliyunEnv` function

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

Only 4 environment variables are set:
- `ANTHROPIC_AUTH_TOKEN` — user-provided API key
- `ANTHROPIC_BASE_URL` — Aliyun's Anthropic-compatible endpoint
- `ANTHROPIC_MODEL` — user-selected model
- `CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC` — standard flag

Not set (per user decision):
- `ANTHROPIC_SMALL_FAST_MODEL` / `ANTHROPIC_DEFAULT_*_MODEL` — Aliyun as a marketplace endpoint routes all model requests internally; these overrides are unnecessary.
- `API_TIMEOUT_MS` — user chose not to set; Claude Code will use its default timeout behavior.
- `~/.claude.json` onboarding — not needed (same as Moonshot/DeepSeek).

**Note on model identifiers and base URL:** The model names (`qwen3.5-plus`, `kimi-k2.5`, `glm-5`, `MiniMax-M2.5`) and base URL (`https://coding.dashscope.aliyuncs.com/apps/anthropic`) are provided directly by the user from Aliyun's documentation. The `coding.dashscope` subdomain is Aliyun's dedicated Claude Code compatibility endpoint.

### Provider selection flow

In `configureClaudeProvider()`:

1. Add `"阿里云（千问）"` to the provider options list (position: after MiniMax, before Custom)
2. New switch-case:
   - Prompt for API Key (required, label: `"阿里云 API Key:"`)
   - Prompt for model selection via `survey.Select` with options:
     - `qwen3.5-plus` (default)
     - `kimi-k2.5`
     - `glm-5`
     - `MiniMax-M2.5`
   - Call `aliyunEnv(apiKey, model)`
   - No `~/.claude.json` needed (default `needClaudeJSON` is already `false`)

### Test

Add `TestProviderEnvMap_Aliyun` in `config_claude_test.go`:
- Verify env map contains correct base URL, model, auth token, and disable-traffic flag
- Verify env map has exactly 4 entries
- Test with both default model and a non-default model

## Files to modify

| File | Change |
|------|--------|
| `cmd/robobee/config_claude.go` | Add `aliyunEnv()` function; add `"阿里云（千问）"` case in `configureClaudeProvider()` |
| `cmd/robobee/config_claude_test.go` | Add `TestProviderEnvMap_Aliyun` |
