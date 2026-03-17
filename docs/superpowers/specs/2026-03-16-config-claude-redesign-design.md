# Config 子命令 Claude 配置流程重设计

**日期:** 2026-03-16
**状态:** 待实现

## 背景

当前 `robobee config` 子命令将 Claude 的配置（可执行文件路径、超时）放在可选的"高级配置"步骤中。由于 Claude 是系统的核心依赖，这个位置不合理。需要将 Claude 配置提前到基本配置之后，并增加模型服务商配置功能。

## 设计目标

1. 将 Claude 配置从高级配置中移出，作为独立步骤紧跟基本配置之后
2. 新增 Claude 可执行程序自动检测、手动输入、下载三种方式
3. 新增 Claude 模型服务商配置，支持月之暗面/智谱/稀宇科技/自定义四种服务商
4. 服务商配置直接写入 `~/.claude/settings.json`（和可能的 `~/.claude.json`）

## 整体流程

```
Step 1: 基本配置（端口、Host、Debug、DB路径）— 不变
Step 2: Claude 配置（新）
  2a. Claude 可执行程序配置
  2b. Claude 模型服务商配置
Step 3: MCP 配置 — 不变
Step 4: 平台配置 — 不变
Step 5: 高级配置（移除 Claude 路径和超时，保留 Feeder/Debounce/FFmpeg）
Step 6: 确认写入 — 不变
```

## Step 2 详细设计

### 2a. Claude 可执行程序配置

**流程：**

1. 调用 `exec.LookPath("claude")` 检测 PATH 中是否有 claude
2. **如果找到：** 提示 "已检测到系统安装的 Claude: /path/to/claude，将自动使用"，自动设置 `vals.ClaudePath`
3. **如果没找到：** 提供选择菜单：
   - "手动输入 Claude 路径"
   - "下载 Claude"

**手动输入分支：**
- 用户输入 Claude 可执行文件的完整路径
- 验证文件存在且可执行

**下载分支：**
- 通过 `runtime.GOARCH` 检测当前 CPU 架构（amd64 / arm64）
- 向 placeholder 下载地址发送请求，传递架构参数，获取下载链接
- 下载 Claude 可执行文件
- 保存到 `~/.robobee/bin/claude`
- 设置可执行权限 (`chmod +x`)
- 如果 `~/.robobee/bin/` 目录不存在，先创建
- 下载失败时提示错误并回退到手动输入路径

**Claude 超时配置：**
- 在可执行程序确定后，提示输入 Claude 超时时间（默认 "30m"）
- 超时值写入 config.yaml 的 `bee.claude.timeout`

**输出：** Claude 路径和超时写入 config.yaml 的 `bee.claude.path` 和 `bee.claude.timeout`

### 2b. Claude 模型服务商配置

**流程：**

1. 检测 `~/.claude/settings.json` 是否存在
2. **如果存在：** 提示 "已检测到 Claude 配置文件，是否跳过配置？"
   - 跳过 → 直接进入 Step 3
   - 继续配置 → 进入服务商选择
3. **如果不存在：** 直接进入服务商选择

**服务商选择（Select）：**
- 月之暗面（Kimi）
- 智谱清言（GLM）
- 稀宇科技（MiniMax）
- 自定义服务商

**所有服务商：** 提示用户输入 API Key（`ANTHROPIC_AUTH_TOKEN`），其余字段为常量。

#### 月之暗面（Kimi）

写入 `~/.claude/settings.json`：

```json
{
  "env": {
    "ANTHROPIC_BASE_URL": "https://api.moonshot.cn/anthropic",
    "ANTHROPIC_AUTH_TOKEN": "<用户输入>",
    "ANTHROPIC_MODEL": "kimi-k2.5",
    "ANTHROPIC_SMALL_FAST_MODEL": "kimi-k2.5",
    "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1",
    "API_TIMEOUT_MS": "600000"
  }
}
```

无需写入 `~/.claude.json`。

#### 智谱清言（GLM）

写入 `~/.claude/settings.json`：

```json
{
  "env": {
    "ANTHROPIC_AUTH_TOKEN": "<用户输入>",
    "ANTHROPIC_BASE_URL": "https://open.bigmodel.cn/api/anthropic",
    "API_TIMEOUT_MS": "3000000",
    "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1"
  }
}
```

额外写入 `~/.claude.json`：

```json
{
  "hasCompletedOnboarding": true
}
```

#### 稀宇科技（MiniMax）

写入 `~/.claude/settings.json`：

```json
{
  "env": {
    "ANTHROPIC_BASE_URL": "https://api.minimaxi.com/anthropic",
    "ANTHROPIC_AUTH_TOKEN": "<用户输入>",
    "API_TIMEOUT_MS": "3000000",
    "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1",
    "ANTHROPIC_MODEL": "MiniMax-M2.5",
    "ANTHROPIC_SMALL_FAST_MODEL": "MiniMax-M2.5",
    "ANTHROPIC_DEFAULT_SONNET_MODEL": "MiniMax-M2.5",
    "ANTHROPIC_DEFAULT_OPUS_MODEL": "MiniMax-M2.5",
    "ANTHROPIC_DEFAULT_HAIKU_MODEL": "MiniMax-M2.5"
  }
}
```

额外写入 `~/.claude.json`：

```json
{
  "hasCompletedOnboarding": true
}
```

#### 自定义服务商

提示用户输入：
- `ANTHROPIC_BASE_URL`
- `ANTHROPIC_AUTH_TOKEN`

写入 `~/.claude/settings.json`：

```json
{
  "env": {
    "ANTHROPIC_BASE_URL": "<用户输入>",
    "ANTHROPIC_AUTH_TOKEN": "<用户输入>"
  }
}
```

无需写入 `~/.claude.json`。

### 文件写入细节

- 如果 `~/.claude/` 目录不存在，先创建（`os.MkdirAll`）
- JSON 使用缩进格式化写入，保持可读性
- 文件已存在时直接覆盖（用户已通过交互确认继续配置）

## 代码结构

### configValues 结构体

- 保留 `ClaudePath` 和 `ClaudeTimeout` 字段（写入 config.yaml）
- 服务商配置不加入 configValues，因为直接写入外部文件

### 新增子函数

在 `cmd/robobee/config.go` 中新增：

- `configureClaudeExecutable(vals *configValues) error` — 处理 2a 逻辑
- `configureClaudeProvider() error` — 处理 2b 逻辑

### 高级配置调整

从高级配置中移除：
- Claude 可执行文件路径
- Claude 超时

保留：
- Feeder 超时
- 消息去抖时间
- FFprobe 路径
- FFmpeg 路径

### 新增依赖

- `os/exec`（`exec.LookPath`）
- `runtime`（`runtime.GOARCH`）
- `net/http`（下载 Claude）
- `io`（文件复制）
- `encoding/json`（写入 settings.json）

## 影响范围

| 文件 | 变更 |
|------|------|
| `cmd/robobee/config.go` | 主要变更：新增 Step 2，调整高级配置，新增子函数 |
| `internal/config/config.yaml.tmpl` | 无变更（模板结构不变） |
| `internal/config/config.go` | 无变更 |
