# CLI 子命令改造设计

> 将 robobee 从单一 web 服务入口改造为 cobra CLI 程序，包含 `server` 和 `config` 两个子命令。

## 背景

当前程序入口为 `cmd/server/main.go`，直接启动 web 服务，配置文件路径通过位置参数传入。随着功能扩展，需要将程序改造为标准 CLI 工具，支持多个子命令。

## 决策记录

| 决策项 | 选择 | 备选项 |
|--------|------|--------|
| CLI 框架 | cobra | urfave/cli, 纯标准库 |
| 交互式库 | AlecAivazis/survey/v2 | bubbletea, fmt.Scan |
| 二进制名称 | robobee | server (原) |
| 目录结构 | 方案 A：cmd/robobee/ + internal/app/ | 方案 B/C |

## 目录结构变更

### 当前结构

```
cmd/server/
  main.go       ← 入口，解析 os.Args 加载配置
  app.go        ← App struct, buildApp, 所有 builder 函数
```

### 目标结构

```
cmd/robobee/
  main.go       ← cobra root command + main()
  server.go     ← "robobee server" 子命令
  config.go     ← "robobee config" 子命令（交互式）
  config.yaml.tmpl  ← 配置文件生成模板（//go:embed）
internal/app/
  app.go        ← App struct, Run(), BuildApp() — 从 cmd/server/app.go 迁移
cmd/server/     ← 删除
```

### 迁移细节

- `cmd/server/app.go` 的全部内容迁移到 `internal/app/app.go`
- 包名从 `main` 改为 `app`
- `buildApp` 改为导出函数 `BuildApp`
- `App.Run()` 保持为导出方法
- 所有 build* 辅助函数（buildStores, buildWorkerManager, buildBee, buildPipeline, buildPlatforms, buildAPIServer）跟随迁移，保持非导出
- `appStores` struct 跟随迁移，保持非导出
- `cmd/server/` 目录在同一个提交中删除，避免出现损坏的中间状态

## CLI 命令设计

### Root 命令

```
robobee — RoboBee 核心服务

Usage:
  robobee [command]

Available Commands:
  server      启动 web 服务
  config      交互式生成配置文件
  help        Help about any command
```

不带子命令时显示帮助信息（cobra 默认行为）。

### server 子命令

```
robobee server [flags]

Flags:
  -c, --config string   配置文件路径 (默认 "config.yaml")
```

**行为：**
1. 解析 `--config` flag
2. 调用 `config.Load(cfgPath)` 加载配置
3. 调用 `app.BuildApp(cfg)` 构建应用
4. 调用 `app.Run()` 启动服务

与当前 `./server` 行为完全一致。

### config 子命令

```
robobee config [flags]

Flags:
  -o, --output string   输出文件路径 (默认 "config.yaml")
```

**行为：** 交互式引导用户生成配置文件。

## 交互式配置流程

使用 `AlecAivazis/survey/v2` 库实现。

### Step 1 — 基础配置

| 提示 | 类型 | 默认值 |
|------|------|--------|
| Server 端口 | Input (数字) | 8080 |
| Server Host | Input | localhost |
| Debug 模式 | Confirm | false |
| 数据库路径 | Input | ./data/robobee.db |

### Step 2 — MCP 配置

| 提示 | 类型 | 默认值 |
|------|------|--------|
| API Key | Password | (必填) |

### Step 3 — 平台配置

| 提示 | 类型 | 默认值 |
|------|------|--------|
| 启用哪些平台？ | MultiSelect [飞书, 钉钉, 企微] | 无 |

根据选择的平台，依次询问凭据：

**飞书：**
- App ID (Input)
- App Secret (Password)

**钉钉：**
- Client ID (Input)
- Client Secret (Password)

**企微：**
- Bot ID (Input)
- Secret (Password)

### Step 4 — 高级配置（可选）

| 提示 | 类型 | 默认值 |
|------|------|--------|
| 是否自定义高级配置？ | Confirm | false |

如果选是，继续询问：

| 提示 | 类型 | 默认值 |
|------|------|--------|
| Claude 可执行文件路径 | Input | claude |
| Claude 超时 | Input | 30m |
| Feeder 超时 | Input | 5m |
| 消息去抖时间 | Input | 3s |
| FFprobe 路径 | Input | ffprobe |
| FFmpeg 路径 | Input | ffmpeg |

### Step 5 — 确认写入

1. 显示将写入的文件路径
2. 如果文件已存在，提示确认覆盖（Confirm）
3. 构建 `config.Config` 结构体，序列化为 YAML
4. 写入文件，输出成功提示

### 默认值处理策略

当用户在交互提示中留空（直接回车）时，使用列表中的默认值写入 YAML 文件。这样生成的配置文件是自文档化的，用户可以直接看到所有字段和当前值。

### 未纳入交互流程的字段

以下字段有合理默认值且极少需要修改，不在交互流程中询问，依赖 `applyDefaults` 在加载时设置：

- `WeComConfig.WebSocketURL` — 默认 `wss://openws.work.weixin.qq.com`
- `FeishuConfig.MaxMediaSize` — 默认 100MB

这些字段在生成的 YAML 中以注释形式标注，方便用户按需手动修改。

### 生成文件格式

生成的 YAML 应包含注释，与 `config.example.yaml` 风格一致。使用 Go `text/template` 嵌入模板（通过 `//go:embed`），以保留注释和格式。模板文件位于 `cmd/robobee/config.yaml.tmpl`。

## Makefile 变更

```makefile
BINARY := robobee                          # 从 server 改为 robobee
# build/release 目标路径从 ./cmd/server/ 改为 ./cmd/robobee/
```

## 新增依赖

| 依赖 | 用途 |
|------|------|
| `github.com/spf13/cobra` | CLI 框架 |
| `github.com/AlecAivazis/survey/v2` | 交互式 prompt |

**注意：** survey/v2 已归档（不再维护），但功能稳定，满足当前需求。如未来需要替换，可考虑 `charmbracelet/huh`。需验证 survey/v2 在 `CGO_ENABLED=0` 交叉编译下正常工作。

## 不变的部分

- `internal/config/config.go` — Config struct 和 Load 函数不变
- `config.example.yaml` — 保留作为参考
- 所有 `internal/` 下的业务逻辑包不变
- Web 前端和 API 层不变

## 错误处理

- `server` 子命令：配置加载失败或 BuildApp 失败时，打印错误并 `os.Exit(1)`（与当前行为一致）
- `config` 子命令：用户中断（Ctrl+C）时优雅退出；写入失败时报错

## 测试策略

- `internal/app/BuildApp` 可被单元测试（从 main 包提取出来后）
- `config` 子命令的交互逻辑可通过注入 survey 的 stdio 进行测试（非必须，初始阶段手动测试即可）
- 确保 `go build ./cmd/robobee/` 编译通过
