# CLI 子命令改造 — 实施计划

> 基于设计文档 `docs/superpowers/specs/2026-03-16-cli-subcommands-design.md`

## 概览

将 robobee 从 `cmd/server/main.go` 单一入口改造为 Cobra CLI 程序，支持 `server` 和 `config` 两个子命令。改造范围涉及：目录结构重组、依赖引入、CLI 命令实现、配置模板创建、Makefile 更新。

---

## Step 1：引入依赖

**目标：** 添加 cobra 和 survey/v2 依赖。

**操作：**
```bash
go get github.com/spf13/cobra
go get github.com/AlecAivazis/survey/v2
```

**验证：** `go.mod` 中出现两个新依赖，`go mod tidy` 无报错。

---

## Step 2：创建 `internal/app/` 包 — 迁移 App 逻辑

**目标：** 将 `cmd/server/app.go` 的全部内容迁移到 `internal/app/app.go`。

**操作：**
1. 创建 `internal/app/app.go`
2. 包名改为 `app`
3. `buildApp` → 导出为 `BuildApp`
4. `App` struct 及 `Run()` 方法保持导出
5. 所有 `build*` 辅助函数（`buildStores`, `buildWorkerManager`, `buildBee`, `buildPipeline`, `buildPlatforms`, `buildAPIServer`）和 `appStores` struct 跟随迁移，保持非导出
6. 更新所有 import 路径（`github.com/robobee/core/internal/app`）

**不改变任何逻辑**，纯搬运 + 包名/导出调整。

**验证：** `go build ./internal/app/` 编译通过。

---

## Step 3：创建 `cmd/robobee/` — Root 命令 + Server 子命令

**目标：** 创建 Cobra CLI 入口，实现 root 命令和 `server` 子命令。

### 3a. `cmd/robobee/main.go` — Root 命令

```go
// root command: robobee
// 不带子命令时显示帮助（cobra 默认行为）
var rootCmd = &cobra.Command{
    Use:   "robobee",
    Short: "RoboBee 核心服务",
}

func main() {
    if err := rootCmd.Execute(); err != nil {
        os.Exit(1)
    }
}
```

### 3b. `cmd/robobee/server.go` — Server 子命令

```go
// robobee server -c config.yaml
var serverCmd = &cobra.Command{
    Use:   "server",
    Short: "启动 web 服务",
    RunE: func(cmd *cobra.Command, args []string) error {
        // 1. 解析 --config flag
        // 2. config.Load(cfgPath)
        // 3. app.BuildApp(cfg)
        // 4. app.Run()
    },
}
// flag: -c, --config string (默认 "config.yaml")
```

行为与当前 `cmd/server/main.go` 完全一致。

**验证：** `go build ./cmd/robobee/` 编译通过；`./robobee server -c config.yaml` 可正常启动服务。

---

## Step 4：创建 `cmd/robobee/config.go` — Config 子命令

**目标：** 实现交互式配置生成子命令。

### 4a. 创建配置模板 `cmd/robobee/config.yaml.tmpl`

基于 `config.example.yaml` 的风格，使用 Go `text/template` 语法，通过 `//go:embed` 嵌入。模板中包含注释，生成的 YAML 自文档化。

对于未纳入交互流程的字段（`WebSocketURL`, `MaxMediaSize`），以注释形式写入模板。

### 4b. 实现交互流程

使用 `survey/v2` 按设计文档的 5 个步骤实现：

1. **基础配置** — Server 端口、Host、Debug、数据库路径
2. **MCP 配置** — API Key（Password 类型）
3. **平台配置** — MultiSelect 选择平台，按选择询问凭据
4. **高级配置（可选）** — Claude 路径/超时、Feeder 超时、消息去抖、FFprobe/FFmpeg 路径
5. **确认写入** — 显示路径、确认覆盖、渲染模板、写入文件

**Flag：** `-o, --output string` (默认 `"config.yaml"`)

**错误处理：**
- 用户 Ctrl+C 中断时优雅退出
- 文件写入失败时报错

**默认值策略：** 用户留空时使用设计文档中定义的默认值写入 YAML。

**验证：** `go build ./cmd/robobee/` 编译通过；`./robobee config` 可正常运行交互流程，生成有效的 `config.yaml`。

---

## Step 5：删除 `cmd/server/` 并更新 Makefile

**目标：** 清理旧入口，更新构建配置。

**操作：**
1. 删除 `cmd/server/` 目录（`main.go` + `app.go`）
2. 更新 `Makefile`：
   - `BINARY := server` → `BINARY := robobee`
   - `./cmd/server/` → `./cmd/robobee/`（build 和 release 目标）

**注意：** 此步骤与 Step 2-4 在同一个提交中完成，避免出现损坏的中间状态。如果分步提交，在删除 `cmd/server/` 之前确保 `cmd/robobee/` 已完全就绪。

**验证：** `make build` 成功产出 `dist/robobee`；`make release` 为所有平台构建成功。

---

## Step 6：最终验证

1. `go build ./cmd/robobee/` — 编译通过
2. `go vet ./...` — 无警告
3. `go test ./...` — 所有测试通过
4. `./dist/robobee` — 显示帮助信息
5. `./dist/robobee server -c config.yaml` — 正常启动
6. `./dist/robobee config` — 交互式生成配置文件，内容正确
7. `make build && make release` — 构建成功

---

## 不变的部分（确认清单）

- [x] `internal/config/config.go` — Config struct 和 Load 函数不变
- [x] `config.example.yaml` — 保留
- [x] 所有 `internal/` 下的业务逻辑包不变
- [x] Web 前端和 API 层不变

## 风险与注意事项

1. **survey/v2 已归档** — 功能稳定，暂不影响。未来可考虑替换为 `charmbracelet/huh`
2. **CGO_ENABLED=0 兼容性** — 需验证 survey/v2 在交叉编译下正常工作
3. **import 路径** — 迁移 `app.go` 时需确保所有 internal 包的 import 路径正确
