# Claude Code 引擎插件化设计

**日期**：2026-04-06  
**状态**：已批准  
**范围**：将 Claude Code 的集成从硬编码改为引擎注册表 + 适配器模式，同时覆盖衍生依赖服务（workspace 初始化、系统规则注入等）的插件化

---

## 背景与问题

当前 openbee 的架构中，`worker.Manager` 和 `bee.BeeProcess` 直接依赖 `internal/ai/claude` 包：

- `worker.Manager` 持有 `*claude.Invoker`，调用 `claude.EnsureSystemRules()` 写入工作目录
- `bee.BeeProcess` 持有 `*claude.Invoker`，调用 `bee.WriteCLAUDEMD()` 写入工作目录
- AI 引擎是硬编码的，无法在不修改 domain 层的情况下替换

此外，Claude Code 带有若干衍生依赖服务：
- **系统规则注入**：`.openbee.md` + `CLAUDE.md`，内容为 Claude 专用提示词
- **Workspace 初始化**：写入角色规则到工作目录（`EnsureSystemRules` / `WriteCLAUDEMD`）
- **API Provider 配置**：Moonshot/DeepSeek/Qwen 等，通过环境变量传入
- **Skills / Plugins 加载**：`skills-lock.json`，openbee-bee / openbee-worker skills
- **MCP 连接**：通过 `OPENBEE_URL` 注入，Claude Code 主动连接回 openbee

这些都是 Claude 特有的，其他引擎会有完全不同的实现方式。

---

## 目标

- 抽象 AI 引擎接口，Claude Code 成为默认的、可替换的实现之一
- 接口覆盖完整生命周期：workspace 初始化 + 任务执行
- 引擎以 Go 内置包的形式存在，通过注册表选取，编译时确定
- domain 层（`worker.Manager`、`bee.BeeProcess`）与具体引擎完全解耦
- 未来新增引擎（如 Gemini CLI、Amp）时，domain 层零改动

---

## 方案：引擎注册表 + EngineAdapter 接口

### 核心接口（`internal/ai/engine.go`）

```go
package ai

import "context"

type Role string

const (
    RoleBee    Role = "bee"
    RoleWorker Role = "worker"
)

type WorkspaceOptions struct {
    Name        string
    Description string
    Memory      string
}

type RunOptions struct {
    SessionID string
    Resume    bool
    APIKey    string
}

type OutputType string

const (
    OutputStdout OutputType = "stdout"
    OutputStderr OutputType = "stderr"
    OutputDone   OutputType = "done"
    OutputError  OutputType = "error"
)

type Output struct {
    Type    OutputType `json:"type"`
    Content string     `json:"content"`
}

type Process interface {
    PID() int
    Stop() error
}

// EngineAdapter 是 AI 引擎插件的完整契约，覆盖 workspace 初始化和任务执行。
type EngineAdapter interface {
    // SetupWorkspace 写入引擎所需的 workspace 配置文件（系统规则、config 等）。
    // 应当是幂等的。
    SetupWorkspace(workDir string, role Role, opts WorkspaceOptions) error

    // Run 执行任务，返回进程句柄和输出事件流。
    // 调用方通过读取 channel 监听事件；channel 在进程退出后关闭。
    Run(ctx context.Context, workDir, prompt string,
        opts RunOptions, logPath string) (Process, <-chan Output, error)
}
```

### 注册表（`internal/ai/registry.go`）

```go
package ai

type EngineConfig struct {
    OpenbeeURL string         // openbee 服务地址，由框架注入（用于 MCP 连接等）
    Raw        map[string]any // 引擎专属配置，从 config.yaml 对应段读取
}

type Factory func(cfg EngineConfig) (EngineAdapter, error)

var registry = map[string]Factory{}

func Register(name string, f Factory) { registry[name] = f }

func New(name string, cfg EngineConfig) (EngineAdapter, error) {
    f, ok := registry[name]
    if !ok {
        return nil, fmt.Errorf("unknown engine: %q", name)
    }
    return f(cfg)
}
```

---

## 包结构变化

```
internal/ai/
  engine.go       ← 新增：EngineAdapter 接口 + 所有共享类型
  registry.go     ← 新增：Register / New
  claude/
    adapter.go    ← 新增：claudeAdapter 实现 EngineAdapter，含 init() 自注册
    invoker.go    ← 保留（内部实现），Process 改为实现 ai.Process 接口
    claudemd.go   ← 保留，被 adapter.go 调用
    claudemd_bee.go   ← 保留
    claudemd_worker.go ← 保留
    download.go   ← 保留
    provider.go   ← 保留（Claude 内部细节）
```

---

## Claude 适配器（`internal/ai/claude/adapter.go`）

```go
package claude

import ai "github.com/theopenbee/openbee/internal/ai"

func init() {
    ai.Register("claude", func(cfg ai.EngineConfig) (ai.EngineAdapter, error) {
        claudeCfg := parseClaudeConfig(cfg.Raw)
        return &claudeAdapter{
            invoker: NewInvoker(claudeCfg.Path, cfg.OpenbeeURL),
        }, nil
    })
}

type claudeAdapter struct {
    invoker *Invoker
}

func (a *claudeAdapter) SetupWorkspace(workDir string, role ai.Role, opts ai.WorkspaceOptions) error {
    switch role {
    case ai.RoleWorker:
        return EnsureSystemRules(workDir, RoleWorker,
            WithName(opts.Name),
            WithDescription(opts.Description),
            WithMemory(opts.Memory),
        )
    case ai.RoleBee:
        if err := WriteCLAUDEMD(workDir, DefaultPersona); err != nil {
            return err
        }
        return EnsureSystemRules(workDir, RoleBee)
    }
    return nil
}

func (a *claudeAdapter) Run(ctx context.Context, workDir, prompt string,
    opts ai.RunOptions, logPath string) (ai.Process, <-chan ai.Output, error) {
    return a.invoker.Run(ctx, workDir, prompt, RunOptions(opts), logPath)
}
```

---

## 配置变更（`config.yaml` + `BeeConfig`）

`config.yaml` 新增 `engine` 字段，默认值 `"claude"`，现有配置不受影响：

```yaml
bee:
  engine: claude      # 新增，默认 "claude"
  claude:
    path: claude
    timeout: 30m
```

`BeeConfig` 结构体：

```go
type BeeConfig struct {
    Engine  string        `yaml:"engine"`   // 新增，缺省为 "claude"
    Claude  ClaudeConfig  `yaml:"claude"`
    // ...其余字段不变
}

// EngineConfigRaw 返回当前引擎对应的原始配置 map（供 Registry 使用）。
func (b BeeConfig) EngineConfigRaw() map[string]any {
    switch b.Engine {
    case "claude", "":
        return map[string]any{
            "path":    b.Claude.Path,
            "timeout": b.Claude.Timeout,
        }
    }
    return nil
}
```

---

## Domain 层变更

### `worker.Manager`

```go
// 变更前
type Manager struct {
    invoker *claude.Invoker
    ...
}

// 变更后
type Manager struct {
    engine ai.EngineAdapter
    ...
}

func NewManager(workerBaseDir string, bc config.BeeConfig,
    ws *store.WorkerStore, es *store.ExecutionStore,
    engine ai.EngineAdapter) *Manager { ... }
```

- `CreateWorker` 中 `claude.EnsureSystemRules(...)` 替换为：
  ```go
  engine.SetupWorkspace(workDir, ai.RoleWorker, ai.WorkspaceOptions{
      Name: name, Description: description, Memory: memory,
  })
  ```
- 执行任务时 `invoker.Run(...)` 替换为 `engine.Run(...)`

### `bee.BeeProcess`

```go
// 变更前
type BeeProcess struct {
    invoker *claude.Invoker
    ...
}

// 变更后
type BeeProcess struct {
    engine ai.EngineAdapter
    ...
}

func NewBeeProcess(cfg config.BeeConfig, engine ai.EngineAdapter) *BeeProcess { ... }
```

- Workspace 初始化（`WriteCLAUDEMD` + `EnsureSystemRules`）移入 `engine.SetupWorkspace(workDir, ai.RoleBee, ...)`
- `bee` 包不再 import `internal/ai/claude`

---

## app.go 接线

```go
import (
    ai "github.com/theopenbee/openbee/internal/ai"
    _ "github.com/theopenbee/openbee/internal/ai/claude" // 触发 init() 自注册
)

func buildEngine(cfg config.BeeConfig) (ai.EngineAdapter, error) {
    engineName := cfg.Engine
    if engineName == "" {
        engineName = "claude"
    }
    return ai.New(engineName, ai.EngineConfig{
        OpenbeeURL: cfg.MCPBaseURL,
        Raw:        cfg.EngineConfigRaw(),
    })
}
```

`buildWorkerManager` 和 `buildBee` 各自接收 `ai.EngineAdapter` 参数。

---

## 完整数据流

```
config.yaml (engine: claude)
  -> app.go 调用 ai.New("claude", engineCfg)
  -> registry 查找 claude factory，返回 claudeAdapter
  -> engine 注入 worker.Manager 和 bee.BeeProcess
  -> 创建 Worker 时：engine.SetupWorkspace(workDir, RoleWorker, opts)
       -> claudeAdapter: EnsureSystemRules() 写 .openbee.md + CLAUDE.md
  -> 执行任务时：engine.Run(ctx, workDir, prompt, opts, logPath)
       -> claudeAdapter: invoker.Run() 启动 claude 子进程
```

---

## 扩展路径：新增引擎

未来接入 Gemini CLI 只需：

1. 新建 `internal/ai/gemini/adapter.go`，实现 `ai.EngineAdapter`
2. 在 `init()` 中调用 `ai.Register("gemini", ...)`
3. `app.go` 加 `import _ "…/ai/gemini"`
4. `config.yaml` 改 `engine: gemini`

**domain 层零改动。**

---

## 测试策略

- `internal/ai` 包：单元测试 Registry（Register/New、未知引擎错误）
- `internal/ai/claude/adapter_test.go`：mock `Invoker`，验证 `SetupWorkspace` 对不同 role 的文件写入行为
- `worker.Manager` / `bee.BeeProcess`：注入 `MockEngineAdapter`，彻底解除对真实 Claude 二进制的依赖
- 集成测试：现有 e2e 测试无需改动，仍通过真实 Claude 二进制运行
