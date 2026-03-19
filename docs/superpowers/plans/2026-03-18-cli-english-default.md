# CLI Default Language: English — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace all Chinese user-facing strings in the 5 CLI source files with English equivalents so `openbee` presents an English interface by default.

**Architecture:** Direct inline string replacement — no new files, no i18n framework, no structural changes. Option strings that appear as `switch` case labels must be updated consistently with their display-string counterparts or the runtime logic breaks.

**Tech Stack:** Go, cobra (CLI framework), AlecAivazis/survey (interactive prompts)

---

## Files Modified

| File | Changes |
|------|---------|
| `cmd/openbee/main.go` | 1 string — root command Short description |
| `cmd/openbee/server.go` | 2 strings — server command description + flag usage |
| `cmd/openbee/upgrade.go` | ~20 strings — command descriptions, stdout messages, errors |
| `cmd/openbee/config.go` | ~30 strings — wizard prompts, section headers, option strings + their switch-case labels |
| `cmd/openbee/config_claude.go` | ~40 strings — provider name constants, prompts, stdout messages, errors; several strings appear twice |

---

### Task 1: Translate `main.go` and `server.go`

**Files:**
- Modify: `cmd/openbee/main.go`
- Modify: `cmd/openbee/server.go`

- [ ] **Step 1: Edit `main.go` — translate root command Short**

  In `cmd/openbee/main.go`, change:
  ```go
  Short:   "OpenBee 核心服务",
  ```
  to:
  ```go
  Short:   "OpenBee core service",
  ```

- [ ] **Step 2: Edit `server.go` — translate command description and flag**

  In `cmd/openbee/server.go`, change:
  ```go
  Short: "启动 OpenBee 服务",
  ```
  to:
  ```go
  Short: "Start the OpenBee server",
  ```

  And change:
  ```go
  serverCmd.Flags().StringVarP(&cfgPath, "config", "c", "config.yaml", "配置文件路径")
  ```
  to:
  ```go
  serverCmd.Flags().StringVarP(&cfgPath, "config", "c", "config.yaml", "path to config file")
  ```

- [ ] **Step 3: Build and test**

  ```bash
  cd /Users/tengyongzhi/work/bot-workspaces/openbee2
  go build ./cmd/openbee/...
  go test ./cmd/openbee/...
  ```
  Expected: build succeeds, all tests pass.

- [ ] **Step 4: Commit**

  ```bash
  git add cmd/openbee/main.go cmd/openbee/server.go
  git commit -m "feat: translate main.go and server.go CLI strings to English"
  ```

---

### Task 2: Translate `upgrade.go`

**Files:**
- Modify: `cmd/openbee/upgrade.go`

- [ ] **Step 1: Translate command metadata**

  ```go
  // Before:
  Short: "升级 openbee 到最新版本",
  Long:  "检测是否有新版本可用，如有则下载并替换当前二进制文件。",
  // After:
  Short: "Upgrade openbee to the latest version",
  Long:  "Check for a new version and replace the current binary if one is available.",
  ```

  ```go
  // Before:
  upgradeCmd.Flags().BoolVar(&upgradeCheckOnly, "check", false, "仅检测是否有新版本，不执行升级")
  // After:
  upgradeCmd.Flags().BoolVar(&upgradeCheckOnly, "check", false, "check for updates only, do not upgrade")
  ```

- [ ] **Step 2: Translate `runUpgrade` stdout messages**

  ```go
  // Before:
  fmt.Printf("当前版本: %s\n", current)
  fmt.Println("正在检测最新版本...")
  // ...
  fmt.Printf("最新版本: %s\n", latest)
  // ...
  fmt.Println("已是最新版本，无需升级。")
  // ...
  fmt.Printf("发现新版本: %s\n", latest)
  // ...
  fmt.Printf("运行 'openbee upgrade' 执行升级。\n")

  // After:
  fmt.Printf("Current version: %s\n", current)
  fmt.Println("Checking for latest version...")
  // ...
  fmt.Printf("Latest version: %s\n", latest)
  // ...
  fmt.Println("Already up to date.")
  // ...
  fmt.Printf("New version available: %s\n", latest)
  // ...
  fmt.Printf("Run 'openbee upgrade' to upgrade.\n")
  ```

  And the error wrap in `runUpgrade`:
  ```go
  // Before:
  return fmt.Errorf("获取最新版本失败: %w", err)
  // After:
  return fmt.Errorf("fetch latest version: %w", err)
  ```

- [ ] **Step 3: Translate `doUpgrade` stdout messages and errors**

  ```go
  // stdout
  fmt.Printf("正在下载 %s...\n", archiveName)
  →
  fmt.Printf("Downloading %s...\n", archiveName)

  fmt.Printf("警告: 无法下载 checksums.txt，跳过校验 (%v)\n", err)
  →
  fmt.Printf("warning: failed to download checksums.txt, skipping verification (%v)\n", err)

  fmt.Println("正在校验 SHA256...")
  →
  fmt.Println("Verifying SHA256...")

  fmt.Println("SHA256 校验通过。")
  →
  fmt.Println("SHA256 verified.")

  fmt.Printf("升级成功！openbee 已更新到 %s。\n", newVersion)
  →
  fmt.Printf("Successfully upgraded openbee to %s.\n", newVersion)
  ```

  ```go
  // errors
  fmt.Errorf("创建临时目录失败: %w", err)   → fmt.Errorf("create temp dir: %w", err)
  fmt.Errorf("下载失败: %w", err)           → fmt.Errorf("download: %w", err)
  fmt.Errorf("校验失败: %w", err)           → fmt.Errorf("checksum verification: %w", err)
  fmt.Errorf("解压失败: %w", err)           → fmt.Errorf("extract: %w", err)
  fmt.Errorf("无法确定当前可执行文件路径: %w", err) → fmt.Errorf("determine executable path: %w", err)
  fmt.Errorf("解析符号链接失败: %w", err)   → fmt.Errorf("resolve symlink: %w", err)
  fmt.Errorf("无法在 %s 创建临时文件 (可能需要 sudo): %w", dir, err) → fmt.Errorf("create temp file in %s (may need sudo): %w", dir, err)
  fmt.Errorf("复制新二进制失败: %w", err)   → fmt.Errorf("copy new binary: %w", err)
  fmt.Errorf("设置权限失败: %w", err)       → fmt.Errorf("set permissions: %w", err)
  fmt.Errorf("替换二进制失败 (可能需要 sudo): %w", err) → fmt.Errorf("replace binary (may need sudo): %w", err)
  ```

- [ ] **Step 4: Translate `fetchLatestVersion` and `verifyChecksum` errors**

  ```go
  // fetchLatestVersion
  fmt.Errorf("GitHub API 返回 %d", resp.StatusCode) → fmt.Errorf("GitHub API returned %d", resp.StatusCode)
  fmt.Errorf("解析响应失败: %w", err)               → fmt.Errorf("parse response: %w", err)
  fmt.Errorf("获取到的版本号为空")                   → fmt.Errorf("empty version tag")

  // verifyChecksum
  fmt.Errorf("checksums.txt 中未找到 %s 的校验值", archiveName) → fmt.Errorf("no checksum for %s in checksums.txt", archiveName)
  fmt.Errorf("SHA256 不匹配\n  期望: %s\n  实际: %s", expected, actual) → fmt.Errorf("SHA256 mismatch\n  expected: %s\n  got:      %s", expected, actual)

  // extractBinary
  fmt.Errorf("压缩包中未找到 openbee 二进制文件") → fmt.Errorf("openbee binary not found in archive")
  ```

- [ ] **Step 5: Build and test**

  ```bash
  go build ./cmd/openbee/...
  go test ./cmd/openbee/...
  ```
  Expected: build succeeds, all tests pass.

- [ ] **Step 6: Commit**

  ```bash
  git add cmd/openbee/upgrade.go
  git commit -m "feat: translate upgrade.go CLI strings to English"
  ```

---

### Task 3: Translate `config.go`

**Files:**
- Modify: `cmd/openbee/config.go`

> **Note:** The platform strings `"飞书"`, `"钉钉"`, `"企微"` and MCP method strings `"随机生成"`, `"手动输入"` each appear both as option display values AND as `switch` case labels and default-value assignments. All occurrences must be updated together.

- [ ] **Step 1: Translate command metadata**

  ```go
  // Before:
  Short: "交互式生成配置文件",
  // After:
  Short: "Interactively generate a config file",

  // Before:
  configCmd.Flags().StringVarP(&configOutputPath, "output", "o", "config.yaml", "输出配置文件路径")
  // After:
  configCmd.Flags().StringVarP(&configOutputPath, "output", "o", "config.yaml", "output config file path")
  ```

- [ ] **Step 2: Translate existing-config detection message and Basic Configuration section**

  ```go
  // Before:
  fmt.Printf("已检测到现有配置文件: %s，将使用其中的值作为默认值。\n", configOutputPath)
  // After:
  fmt.Printf("Found existing config at %s, using its values as defaults.\n", configOutputPath)

  // Before:
  fmt.Println("\n=== 基本配置 ===")
  // After:
  fmt.Println("\n=== Basic Configuration ===")
  ```

  Translate prompts in the basic config block:
  ```go
  Message: "Server 端口:"   → Message: "Server port:"
  return fmt.Errorf("端口必须是整数") → return fmt.Errorf("port must be an integer")
  Message: "Debug 模式?"    → Message: "Debug mode?"
  Message: "数据库路径:"    → Message: "Database path:"
  ```

- [ ] **Step 3: Translate Claude and MCP Configuration section headers**

  ```go
  fmt.Println("\n=== Claude 配置 ===") → fmt.Println("\n=== Claude Configuration ===")
  fmt.Println("\n=== MCP 配置 ===")    → fmt.Println("\n=== MCP Configuration ===")
  ```

- [ ] **Step 4: Translate MCP key setup — options, switch cases, and default assignment**

  The string `"随机生成"` appears in:
  1. The `Options` slice of the `Select` prompt
  2. The `Default` field of the same prompt (via `mcpKeyChoice`)
  3. The `case "随机生成":` switch label

  The string `"手动输入"` appears in:
  1. The `Options` slice
  2. The default assignment `mcpKeyChoice = "手动输入"`
  3. The `case "手动输入":` switch label

  Update all occurrences consistently:

  ```go
  // Before:
  mcpKeyChoice := "随机生成"
  if vals.MCPAPIKey != "" {
      mcpKeyChoice = "手动输入"
  }
  // ...
  Options: []string{"随机生成", "手动输入"},
  // ...
  switch mcpMethod {
  case "随机生成":
      // ...
  case "手动输入":

  // After:
  mcpKeyChoice := "Generate randomly"
  if vals.MCPAPIKey != "" {
      mcpKeyChoice = "Enter manually"
  }
  // ...
  Options: []string{"Generate randomly", "Enter manually"},
  // ...
  switch mcpMethod {
  case "Generate randomly":
      // ...
  case "Enter manually":
  ```

  Also translate:
  ```go
  Message: "MCP API Key 设置方式:" → Message: "MCP API Key setup:"
  fmt.Printf("已生成 MCP API Key: %s\n", vals.MCPAPIKey) → fmt.Printf("Generated MCP API Key: %s\n", vals.MCPAPIKey)
  fmt.Errorf("生成随机 key 失败: %w", err) → fmt.Errorf("generate random key: %w", err)
  ```

- [ ] **Step 5: Translate Platform Configuration section — options, switch cases, and prompts**

  The platform strings `"飞书"`, `"钉钉"`, `"企微"` appear in:
  1. `defaultPlatforms = append(defaultPlatforms, "飞书")` (and DingTalk, WeCom)
  2. `Options: []string{"飞书", "钉钉", "企微"}`
  3. `case "飞书":`, `case "钉钉":`, `case "企微":` switch labels

  Update all occurrences consistently:

  ```go
  // Section header
  fmt.Println("\n=== 平台配置 ===") → fmt.Println("\n=== Platform Configuration ===")

  // defaultPlatforms appends
  defaultPlatforms = append(defaultPlatforms, "飞书")   → append(defaultPlatforms, "Feishu")
  defaultPlatforms = append(defaultPlatforms, "钉钉")   → append(defaultPlatforms, "DingTalk")
  defaultPlatforms = append(defaultPlatforms, "企微")   → append(defaultPlatforms, "WeCom")

  // MultiSelect options and prompt
  Message: "启用哪些平台？"            → Message: "Which platforms to enable?"
  Options: []string{"飞书", "钉钉", "企微"} → Options: []string{"Feishu", "DingTalk", "WeCom"}

  // switch cases
  case "飞书": → case "Feishu":
  case "钉钉": → case "DingTalk":
  case "企微": → case "WeCom":

  // per-platform prompts
  Message: "飞书 App ID:"      → Message: "Feishu App ID:"
  Message: "飞书 App Secret:"  → Message: "Feishu App Secret:"
  Message: "钉钉 Client ID:"   → Message: "DingTalk Client ID:"
  Message: "钉钉 Client Secret:" → Message: "DingTalk Client Secret:"
  Message: "企微 Bot ID:"      → Message: "WeCom Bot ID:"
  Message: "企微 Secret:"      → Message: "WeCom Secret:"
  ```

- [ ] **Step 6: Translate Advanced Configuration section — header, prompts**

  ```go
  fmt.Println("\n=== 高级配置 ===") → fmt.Println("\n=== Advanced Configuration ===")
  Message: "是否自定义高级配置？"  → Message: "Customize advanced settings?"
  Message: "Feeder 超时:"          → Message: "Feeder timeout:"
  Message: "消息去抖时间:"         → Message: "Message debounce:"
  Message: "FFprobe 路径:"         → Message: "FFprobe path:"
  Message: "FFmpeg 路径:"          → Message: "FFmpeg path:"
  ```

  > These 4 prompts (`"Feeder 超时:"`, `"消息去抖时间:"`, `"FFprobe 路径:"`, `"FFmpeg 路径:"`) appear inside the `if customAdvanced {` block in `runConfig`.

- [ ] **Step 7: Translate Write Configuration section and error strings**

  ```go
  fmt.Printf("\n=== 写入配置 ===\n") → fmt.Printf("\n=== Write Configuration ===\n")
  fmt.Printf("输出文件: %s\n", configOutputPath) → fmt.Printf("Output file: %s\n", configOutputPath)
  Message: "确认写入配置文件？"    → Message: "Confirm write config file?"
  fmt.Println("已取消写入。")      → fmt.Println("Write cancelled.")
  fmt.Printf("配置文件已生成: %s\n", configOutputPath) → fmt.Printf("Config file written to: %s\n", configOutputPath)
  ```

  Errors:
  ```go
  fmt.Errorf("解析模板失败: %w", err) → fmt.Errorf("parse template: %w", err)
  fmt.Errorf("渲染模板失败: %w", err) → fmt.Errorf("render template: %w", err)
  fmt.Errorf("写入文件失败: %w", err) → fmt.Errorf("write file: %w", err)
  ```

- [ ] **Step 8: Translate `handleSurveyErr` cancel message**

  ```go
  fmt.Println("\n已取消。") → fmt.Println("\nCancelled.")
  ```

- [ ] **Step 9: Build and test**

  ```bash
  go build ./cmd/openbee/...
  go test ./cmd/openbee/...
  ```
  Expected: build succeeds, all tests pass.

- [ ] **Step 10: Commit**

  ```bash
  git add cmd/openbee/config.go
  git commit -m "feat: translate config.go CLI strings to English"
  ```

---

### Task 4: Translate `config_claude.go`

**Files:**
- Modify: `cmd/openbee/config_claude.go`

> **Note:** The strings `"创建目录失败: %w"`, `"序列化 JSON 失败: %w"`, and `"获取用户目录失败: %w"` each appear **twice** in this file. Also, the method strings `"手动输入路径"` and `"下载 Claude"` each appear in both the `Options` slice and as a `case` label — update all occurrences.

- [ ] **Step 1: Translate provider display name constants**

  In the `const` block at the top of the file:
  ```go
  // Before:
  providerMoonshot   = "月之暗面（Kimi）"
  providerDeepSeek   = "深度求索（DeepSeek）"
  providerGLM        = "智谱清言（GLM）"
  providerMiniMax    = "稀宇科技（MiniMax）"
  providerAliyun     = "阿里云（千问）"
  providerVolcengine = "火山引擎（豆包）"
  providerTencent    = "腾讯云"
  providerCustom     = "自定义服务商"

  // After:
  providerMoonshot   = "Moonshot (Kimi)"
  providerDeepSeek   = "DeepSeek"
  providerGLM        = "Zhipu (GLM)"
  providerMiniMax    = "MiniMax"
  providerAliyun     = "Alibaba Cloud (Qwen)"
  providerVolcengine = "Volcengine (Doubao)"
  providerTencent    = "Tencent Cloud"
  providerCustom     = "Custom provider"
  ```

  Because `switch provider { case providerMoonshot: ... }` matches on the constant value, updating the constant values automatically updates all case labels — no further change needed for the provider switch.

- [ ] **Step 2: Translate `mergeClaudeSettings` and `mergeClaudeJSON` errors (appear twice)**

  Both functions contain `"创建目录失败: %w"` and `"序列化 JSON 失败: %w"`. Update all occurrences:

  ```go
  // mergeClaudeSettings (line ~122) and downloadClaude (line ~327):
  fmt.Errorf("创建目录失败: %w", err) → fmt.Errorf("create directory: %w", err)

  // mergeClaudeSettings (line ~146) and mergeClaudeJSON (line ~165):
  fmt.Errorf("序列化 JSON 失败: %w", err) → fmt.Errorf("marshal JSON: %w", err)
  ```

  Also translate the warning in both functions:
  ```go
  fmt.Printf("警告: %s JSON 格式错误，将覆盖: %v\n", path, err) → fmt.Printf("warning: %s has invalid JSON, overwriting: %v\n", path, err)
  ```

- [ ] **Step 3: Translate `configureClaudeExecutable` — prompts and method options/switch cases**

  The strings `"手动输入路径"` and `"下载 Claude"` appear in both `Options` and `case` labels:

  ```go
  // stdout
  fmt.Printf("已检测到系统安装的 Claude: %s，将自动使用。\n", claudePath) → fmt.Printf("Found Claude in PATH: %s, using it automatically.\n", claudePath)

  // Select prompt + Options + switch cases
  Message: "未检测到 Claude，请选择获取方式:" → Message: "Claude not found, how would you like to get it?"
  Options: []string{"手动输入路径", "下载 Claude"} → Options: []string{"Enter path manually", "Download Claude"}
  case "手动输入路径": → case "Enter path manually":
  case "下载 Claude": → case "Download Claude":

  // fallback stdout
  fmt.Printf("下载失败: %v\n", err)    → fmt.Printf("Download failed: %v\n", err)
  fmt.Println("请手动输入 Claude 路径。") → fmt.Println("Please enter the Claude path manually.")

  // Claude timeout prompt
  Message: "Claude 超时:" → Message: "Claude timeout:"
  ```

- [ ] **Step 4: Translate `promptClaudeManualPath` validator errors**

  ```go
  Message: "Claude 可执行文件路径:" → Message: "Claude executable path:"
  return fmt.Errorf("文件不存在: %s", path)         → return fmt.Errorf("file not found: %s", path)
  return fmt.Errorf("路径是目录而非文件: %s", path)  → return fmt.Errorf("path is a directory, not a file: %s", path)
  return fmt.Errorf("文件不可执行: %s", path)        → return fmt.Errorf("file is not executable: %s", path)
  ```

- [ ] **Step 5: Translate `downloadClaude` — stdout, errors, and unsupported-platform message**

  ```go
  // Unsupported platform (multi-line string):
  // Before:
  return fmt.Errorf(
      "当前系统 (%s/%s) 不支持自动下载 Claude Code。\n"+
          "支持的平台: darwin-arm64, darwin-x64, linux-arm64, linux-x64, linux-arm64-musl, linux-x64-musl\n"+
          "请手动安装。",
      runtime.GOOS, runtime.GOARCH,
  )
  // After:
  return fmt.Errorf(
      "current platform (%s/%s) does not support automatic Claude Code download.\n"+
          "Supported platforms: darwin-arm64, darwin-x64, linux-arm64, linux-x64, linux-arm64-musl, linux-x64-musl\n"+
          "Please install manually.",
      runtime.GOOS, runtime.GOARCH,
  )

  // "获取用户目录失败" — first occurrence (in downloadClaude):
  fmt.Errorf("获取用户目录失败: %w", err) → fmt.Errorf("get home directory: %w", err)

  // "创建目录失败" — second occurrence (in downloadClaude, already done in step 2):
  // (already handled in step 2)

  // stdout
  fmt.Printf("正在下载 Claude (%s/%s)...\n", platform.os, platform.arch) → fmt.Printf("Downloading Claude (%s/%s)...\n", platform.os, platform.arch)
  fmt.Printf("Claude 已下载到: %s\n", destPath) → fmt.Printf("Claude downloaded to: %s\n", destPath)

  // errors
  fmt.Errorf("请求下载地址失败: %w", err)  → fmt.Errorf("request download URL: %w", err)
  fmt.Errorf("下载失败，状态码: %d", resp.StatusCode) → fmt.Errorf("download failed with status: %d", resp.StatusCode)
  fmt.Errorf("创建文件失败: %w", err)      → fmt.Errorf("create file: %w", err)
  fmt.Errorf("写入文件失败: %w", err)      → fmt.Errorf("write file: %w", err)
  fmt.Errorf("关闭文件失败: %w", err)      → fmt.Errorf("close file: %w", err)
  fmt.Errorf("设置可执行权限失败: %w", err) → fmt.Errorf("set executable permission: %w", err)
  fmt.Errorf("移动文件失败: %w", err)      → fmt.Errorf("move file: %w", err)
  ```

- [ ] **Step 6: Translate `configureClaudeProvider` — prompts and errors**

  ```go
  // settings.json skip prompt
  Message: "已检测到 Claude 配置文件 (~/.claude/settings.json)，是否跳过模型服务商配置？" → Message: "Found ~/.claude/settings.json, skip model provider setup?"

  // provider selection prompt
  Message: "选择模型服务商:" → Message: "Select model provider:"

  // per-provider API key prompts — these are inline string literals passed to promptAPIKey()
  // in the switch cases for providerGLM, providerAliyun, providerVolcengine, providerTencent
  promptAPIKey("智谱 API Key:")    → promptAPIKey("Zhipu API Key:")
  promptAPIKey("阿里云 API Key:")  → promptAPIKey("Alibaba Cloud API Key:")
  promptAPIKey("火山引擎 API Key:") → promptAPIKey("Volcengine API Key:")
  promptAPIKey("腾讯云 API Key:")  → promptAPIKey("Tencent Cloud API Key:")

  // model selection prompt (appears 3 times, one per provider that offers model choice)
  Message: "选择模型:" → Message: "Select model:"

  // stdout after writing files
  fmt.Println("已写入 ~/.claude/settings.json") → fmt.Println("Written ~/.claude/settings.json")
  fmt.Println("已写入 ~/.claude.json")           → fmt.Println("Written ~/.claude.json")

  // "获取用户目录失败" — second occurrence (in configureClaudeProvider):
  fmt.Errorf("获取用户目录失败: %w", err) → fmt.Errorf("get home directory: %w", err)

  // errors
  fmt.Errorf("写入 settings.json 失败: %w", err) → fmt.Errorf("write settings.json: %w", err)
  fmt.Errorf("写入 .claude.json 失败: %w", err)  → fmt.Errorf("write .claude.json: %w", err)
  ```

- [ ] **Step 7: Build and test**

  ```bash
  go build ./cmd/openbee/...
  go test ./cmd/openbee/...
  ```
  Expected: build succeeds, all tests pass.

- [ ] **Step 8: Commit**

  ```bash
  git add cmd/openbee/config_claude.go
  git commit -m "feat: translate config_claude.go CLI strings to English"
  ```

---

### Task 5: Final verification

- [ ] **Step 1: Full build and test pass**

  ```bash
  go build ./cmd/openbee/...
  go test ./cmd/openbee/...
  ```
  Expected: clean build, all tests pass.

- [ ] **Step 2: Verify no Chinese characters remain in CLI source files**

  Run from the repo root (`/Users/tengyongzhi/work/bot-workspaces/openbee2`):

  ```bash
  cd /Users/tengyongzhi/work/bot-workspaces/openbee2 && python3 -c "
  import re, sys
  pattern = re.compile(r'[\u4e00-\u9fff]')
  files = [
      'cmd/openbee/main.go',
      'cmd/openbee/server.go',
      'cmd/openbee/upgrade.go',
      'cmd/openbee/config.go',
      'cmd/openbee/config_claude.go',
  ]
  found = []
  for f in files:
      for i, line in enumerate(open(f), 1):
          if pattern.search(line):
              found.append(f'{f}:{i}: {line.rstrip()}')
  if found:
      print('FAIL — Chinese characters remaining:')
      for x in found: print(x)
      sys.exit(1)
  else:
      print('PASS — no Chinese characters found')
  "
  ```
  Expected: `PASS — no Chinese characters found`

- [ ] **Step 3: Smoke-test help output is in English**

  ```bash
  go run ./cmd/openbee/... --help
  go run ./cmd/openbee/... server --help
  go run ./cmd/openbee/... upgrade --help
  go run ./cmd/openbee/... config --help
  ```
  Verify all output is in English.

- [ ] **Step 4: Final commit if any fixups needed**

  ```bash
  git add -p
  git commit -m "feat: translate remaining CLI strings to English"
  ```
