# Config Claude: 新增火山引擎与腾讯云服务商

**日期:** 2026-03-16
**状态:** 已批准

## 概述

在 `robobee config` 的 Claude 服务商选择中新增两个"模型市场"类型服务商：火山引擎（豆包）和腾讯云。两者均采用与阿里云（千问）相同的实现模式。

## 设计决策

| 决策 | 选择 |
|------|------|
| 环境变量集合 | 最小集合（4 个变量），与阿里云一致 |
| 列表位置 | 阿里云之后、自定义之前 |
| 显示名称 | `火山引擎（豆包）`、`腾讯云` |
| 默认模型 | 火山引擎: `doubao-seed-2.0-code`，腾讯云: `tc-code-latest（auto）` |
| Timeout | 不设置，使用 Claude Code 默认值 |
| needClaudeJSON | `true`（两者都需要写 `~/.claude.json`） |

## 服务商配置

### 火山引擎（豆包）

- **Base URL:** `https://ark.cn-beijing.volces.com/api/coding`
- **交互:** 输入 API Key + 选择模型
- **可选模型:**
  - `doubao-seed-2.0-code`（默认）
  - `doubao-seed-2.0-pro`
  - `doubao-seed-2.0-lite`
  - `doubao-seed-code`
  - `minimax-m2.5`
  - `glm-4.7`
  - `deepseek-v3.2`
  - `kimi-k2.5`

### 腾讯云

- **Base URL:** `https://api.lkeap.cloud.tencent.com/coding/anthropic`
- **交互:** 输入 API Key + 选择模型
- **可选模型:**
  - `tc-code-latest（auto）`（默认）
  - `hunyuan-2.0-instruct`
  - `hunyuan-2.0-thinking`
  - `minimax-m2.5`
  - `kimi-k2.5`
  - `glm-5`
  - `hunyuan-t1`
  - `hunyuan-turbos`

## env 函数签名

```go
func volcengineEnv(apiKey, model string) map[string]string
func tencentEnv(apiKey, model string) map[string]string
```

两者均返回 4 个键：
- `ANTHROPIC_AUTH_TOKEN`
- `ANTHROPIC_BASE_URL`
- `ANTHROPIC_MODEL`
- `CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC`

## 生成的配置文件

### ~/.claude/settings.json

```json
{
  "env": {
    "ANTHROPIC_AUTH_TOKEN": "<用户输入的 API Key>",
    "ANTHROPIC_BASE_URL": "<服务商 Base URL>",
    "ANTHROPIC_MODEL": "<用户选择的模型>",
    "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1"
  }
}
```

### ~/.claude.json

```json
{
  "hasCompletedOnboarding": true
}
```

## 改动范围

| 文件 | 改动 |
|------|------|
| `cmd/robobee/config_claude.go` | 新增 `volcengineEnv()`、`tencentEnv()` 函数；Options 列表追加 2 项；switch 新增 2 个 case |
| `cmd/robobee/config_claude_test.go` | 新增 `TestProviderEnvMap_Volcengine`、`TestProviderEnvMap_Tencent` 测试 |

## 服务商选择列表（更新后）

```
1. 月之暗面（Kimi）
2. 深度求索（DeepSeek）
3. 智谱清言（GLM）
4. 稀宇科技（MiniMax）
5. 阿里云（千问）
6. 火山引擎（豆包）    ← 新增
7. 腾讯云              ← 新增
8. 自定义服务商
```
