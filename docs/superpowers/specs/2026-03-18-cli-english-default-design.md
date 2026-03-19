# CLI Default Language: English

**Date:** 2026-03-18
**Status:** Approved
**Author:** 天天

## Background

The project currently ships CLI user-facing text entirely in Chinese. The team has decided English should be the default language for the CLI to make the tool accessible to a broader audience. The web frontend already defaults to English (see `web/src/i18n.ts`). This spec aligns the CLI with that standard.

## Scope

CLI layer only — 5 files under `cmd/openbee/`. No i18n framework is introduced; strings are replaced in place.

Out of scope:
- IM platform runtime messages (`internal/platform/…`)
- Worker AI persona and system instructions (`internal/bee/`, `internal/claudemd/`)
- Documentation files (`README.zh.md`, `docs/`, `.openbee.md`)
- Web frontend (already English-default)

## Approach

**Direct inline replacement.** Replace each Chinese string with its English equivalent where it appears. No new files, no structural changes.

## File-by-File Changes

### `cmd/openbee/main.go`

| Location | Chinese | English |
|----------|---------|---------|
| `rootCmd.Short` | `"OpenBee 核心服务"` | `"OpenBee core service"` |

### `cmd/openbee/server.go`

| Location | Chinese | English |
|----------|---------|---------|
| `serverCmd.Short` | `"启动 OpenBee 服务"` | `"Start the OpenBee server"` |
| `--config` flag usage | `"配置文件路径"` | `"path to config file"` |

### `cmd/openbee/upgrade.go`

| Location | Chinese | English |
|----------|---------|---------|
| `upgradeCmd.Short` | `"升级 openbee 到最新版本"` | `"Upgrade openbee to the latest version"` |
| `upgradeCmd.Long` | `"检测是否有新版本可用，如有则下载并替换当前二进制文件。"` | `"Check for a new version and replace the current binary if one is available."` |
| `--check` flag usage | `"仅检测是否有新版本，不执行升级"` | `"check for updates only, do not upgrade"` |
| stdout | `"当前版本: %s\n"` | `"Current version: %s\n"` |
| stdout | `"正在检测最新版本..."` | `"Checking for latest version..."` |
| stdout | `"最新版本: %s\n"` | `"Latest version: %s\n"` |
| stdout | `"已是最新版本，无需升级。"` | `"Already up to date."` |
| stdout | `"发现新版本: %s\n"` | `"New version available: %s\n"` |
| stdout | `"运行 'openbee upgrade' 执行升级。\n"` | `"Run 'openbee upgrade' to upgrade.\n"` |
| stdout | `"正在下载 %s...\n"` | `"Downloading %s...\n"` |
| stdout | `"警告: 无法下载 checksums.txt，跳过校验 (%v)\n"` | `"warning: failed to download checksums.txt, skipping verification (%v)\n"` |
| stdout | `"正在校验 SHA256..."` | `"Verifying SHA256..."` |
| stdout | `"SHA256 校验通过。"` | `"SHA256 verified."` |
| stdout | `"升级成功！openbee 已更新到 %s。\n"` | `"Successfully upgraded openbee to %s.\n"` |
| errors | `"获取最新版本失败: %w"` | `"fetch latest version: %w"` |
| errors | `"创建临时目录失败: %w"` | `"create temp dir: %w"` |
| errors | `"下载失败: %w"` | `"download: %w"` |
| errors | `"校验失败: %w"` | `"checksum verification: %w"` |
| errors | `"解压失败: %w"` | `"extract: %w"` |
| errors | `"无法确定当前可执行文件路径: %w"` | `"determine executable path: %w"` |
| errors | `"解析符号链接失败: %w"` | `"resolve symlink: %w"` |
| errors | `"无法在 %s 创建临时文件 (可能需要 sudo): %w"` | `"create temp file in %s (may need sudo): %w"` |
| errors | `"复制新二进制失败: %w"` | `"copy new binary: %w"` |
| errors | `"设置权限失败: %w"` | `"set permissions: %w"` |
| errors | `"替换二进制失败 (可能需要 sudo): %w"` | `"replace binary (may need sudo): %w"` |
| errors | `"GitHub API 返回 %d"` | `"GitHub API returned %d"` |
| errors | `"解析响应失败: %w"` | `"parse response: %w"` |
| errors | `"获取到的版本号为空"` | `"empty version tag"` |
| errors | `"checksums.txt 中未找到 %s 的校验值"` | `"no checksum for %s in checksums.txt"` |
| errors | `"SHA256 不匹配\n  期望: %s\n  实际: %s"` | `"SHA256 mismatch\n  expected: %s\n  got:      %s"` |
| errors | `"压缩包中未找到 openbee 二进制文件"` | `"openbee binary not found in archive"` |

### `cmd/openbee/config.go`

| Location | Chinese | English |
|----------|---------|---------|
| `configCmd.Short` | `"交互式生成配置文件"` | `"Interactively generate a config file"` |
| `-o` flag usage | `"输出配置文件路径"` | `"output config file path"` |
| stdout | `"已检测到现有配置文件: %s，将使用其中的值作为默认值。\n"` | `"Found existing config at %s, using its values as defaults.\n"` |
| section header | `"\n=== 基本配置 ==="` | `"\n=== Basic Configuration ==="` |
| prompt | `"Server 端口:"` | `"Server port:"` |
| validator | `"端口必须是整数"` | `"port must be an integer"` |
| prompt | `"Debug 模式?"` | `"Debug mode?"` |
| prompt | `"数据库路径:"` | `"Database path:"` |
| section header | `"\n=== Claude 配置 ==="` | `"\n=== Claude Configuration ==="` |
| section header | `"\n=== MCP 配置 ==="` | `"\n=== MCP Configuration ==="` |
| option | `"随机生成"` | `"Generate randomly"` |
| option | `"手动输入"` | `"Enter manually"` |
| prompt | `"MCP API Key 设置方式:"` | `"MCP API Key setup:"` |
| stdout | `"已生成 MCP API Key: %s\n"` | `"Generated MCP API Key: %s\n"` |
| section header | `"\n=== 平台配置 ==="` | `"\n=== Platform Configuration ==="` |
| option | `"飞书"` | `"Feishu"` |
| option | `"钉钉"` | `"DingTalk"` |
| option | `"企微"` | `"WeCom"` |
| prompt | `"启用哪些平台？"` | `"Which platforms to enable?"` |
| prompt | `"飞书 App ID:"` | `"Feishu App ID:"` |
| prompt | `"飞书 App Secret:"` | `"Feishu App Secret:"` |
| prompt | `"钉钉 Client ID:"` | `"DingTalk Client ID:"` |
| prompt | `"钉钉 Client Secret:"` | `"DingTalk Client Secret:"` |
| prompt | `"企微 Bot ID:"` | `"WeCom Bot ID:"` |
| prompt | `"企微 Secret:"` | `"WeCom Secret:"` |
| section header | `"\n=== 高级配置 ==="` | `"\n=== Advanced Configuration ==="` |
| prompt | `"是否自定义高级配置？"` | `"Customize advanced settings?"` |
| prompt | `"消息去抖时间:"` | `"Message debounce:"` |
| section header | `"\n=== 写入配置 ===\n"` | `"\n=== Write Configuration ===\n"` |
| stdout | `"输出文件: %s\n"` | `"Output file: %s\n"` |
| prompt | `"确认写入配置文件？"` | `"Confirm write config file?"` |
| stdout | `"已取消写入。"` | `"Write cancelled."` |
| stdout | `"配置文件已生成: %s\n"` | `"Config file written to: %s\n"` |
| stdout | `"\n已取消。"` | `"\nCancelled."` |
| errors | `"生成随机 key 失败: %w"` | `"generate random key: %w"` |
| errors | `"解析模板失败: %w"` | `"parse template: %w"` |
| errors | `"渲染模板失败: %w"` | `"render template: %w"` |
| errors | `"写入文件失败: %w"` | `"write file: %w"` |

### `cmd/openbee/config_claude.go`

**Provider display name constants:**

| Constant | Chinese | English |
|----------|---------|---------|
| `providerMoonshot` | `"月之暗面（Kimi）"` | `"Moonshot (Kimi)"` |
| `providerDeepSeek` | `"深度求索（DeepSeek）"` | `"DeepSeek"` |
| `providerGLM` | `"智谱清言（GLM）"` | `"Zhipu (GLM)"` |
| `providerMiniMax` | `"稀宇科技（MiniMax）"` | `"MiniMax"` |
| `providerAliyun` | `"阿里云（千问）"` | `"Alibaba Cloud (Qwen)"` |
| `providerVolcengine` | `"火山引擎（豆包）"` | `"Volcengine (Doubao)"` |
| `providerTencent` | `"腾讯云"` | `"Tencent Cloud"` |
| `providerCustom` | `"自定义服务商"` | `"Custom provider"` |

**Prompts and messages:**

| Location | Chinese | English |
|----------|---------|---------|
| prompt | `"已检测到系统安装的 Claude: %s，将自动使用。\n"` | `"Found Claude in PATH: %s, using it automatically.\n"` |
| prompt | `"未检测到 Claude，请选择获取方式:"` | `"Claude not found, how would you like to get it?"` |
| option | `"手动输入路径"` | `"Enter path manually"` |
| option | `"下载 Claude"` | `"Download Claude"` |
| stdout | `"下载失败: %v\n"` | `"Download failed: %v\n"` |
| stdout | `"请手动输入 Claude 路径。"` | `"Please enter the Claude path manually."` |
| prompt | `"已检测到 Claude 配置文件 (~/.claude/settings.json)，是否跳过模型服务商配置？"` | `"Found ~/.claude/settings.json, skip model provider setup?"` |
| prompt | `"选择模型服务商:"` | `"Select model provider:"` |
| prompt | `"选择模型:"` | `"Select model:"` |
| prompt | `"智谱 API Key:"` | `"Zhipu API Key:"` |
| prompt | `"阿里云 API Key:"` | `"Alibaba Cloud API Key:"` |
| prompt | `"火山引擎 API Key:"` | `"Volcengine API Key:"` |
| prompt | `"腾讯云 API Key:"` | `"Tencent Cloud API Key:"` |
| stdout | `"正在下载 Claude (%s/%s)...\n"` | `"Downloading Claude (%s/%s)...\n"` |
| stdout | `"Claude 已下载到: %s\n"` | `"Claude downloaded to: %s\n"` |
| stdout | `"已写入 ~/.claude/settings.json"` | `"Written ~/.claude/settings.json"` |
| stdout | `"已写入 ~/.claude.json"` | `"Written ~/.claude.json"` |
| stdout | `"警告: %s JSON 格式错误，将覆盖: %v\n"` | `"warning: %s has invalid JSON, overwriting: %v\n"` |
| errors | `"创建目录失败: %w"` | `"create directory: %w"` |
| errors | `"序列化 JSON 失败: %w"` | `"marshal JSON: %w"` |
| errors | `"获取用户目录失败: %w"` | `"get home directory: %w"` |
| errors | `"写入 settings.json 失败: %w"` | `"write settings.json: %w"` |
| errors | `"写入 .claude.json 失败: %w"` | `"write .claude.json: %w"` |
| errors | `"文件不存在: %s"` | `"file not found: %s"` |
| errors | `"路径是目录而非文件: %s"` | `"path is a directory, not a file: %s"` |
| errors | `"文件不可执行: %s"` | `"file is not executable: %s"` |
| errors | `"当前系统 (%s/%s) 不支持自动下载..."` | `"current platform (%s/%s) does not support automatic Claude Code download..."` |
| errors | `"请求下载地址失败: %w"` | `"request download URL: %w"` |
| errors | `"下载失败，状态码: %d"` | `"download failed with status: %d"` |
| errors | `"创建文件失败: %w"` | `"create file: %w"` |
| errors | `"写入文件失败: %w"` | `"write file: %w"` |
| errors | `"关闭文件失败: %w"` | `"close file: %w"` |
| errors | `"设置可执行权限失败: %w"` | `"set executable permission: %w"` |
| errors | `"移动文件失败: %w"` | `"move file: %w"` |

## Testing

After each file is changed:
```
go build ./cmd/openbee/...
go test ./cmd/openbee/...
```

All existing tests must pass. No new tests are required — the changes are purely cosmetic string replacements.
