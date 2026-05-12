# `internal/ai/` 目录重整 — 设计文档

- 日期: 2026-05-12
- 分支: `refactor/internal-ai-cleanup`
- 状态: Draft → 待评审

## 1. 背景与目标

`internal/ai/` 顶层目前散落 19 个 `.go` 文件（含 test 共 ~1410 行），混合了三类不同职责的代码：

1. 对外 API（业务上层调用的）：`contracts.go`、`registry.go`、`dynamic.go`、`prompt.go`、`engine_args.go`。
2. 引擎实现共享的内部 helper：`base_adapter.go`、`process.go`(+unix/windows)、`spawn.go`、`usage.go`。
3. 引擎实现（已是子目录）：`claude/`、`codex/`、`kimi/`、`pi/`。

混居导致顶层目录"乱"：阅读者无法一眼分辨"哪些是 ai 包对外暴露的契约"vs"哪些只是 engine 实现私下共用的工具"。

**目标**：把 `internal/ai/` 顶层收敛为 **1 个对外文件 + 2 个子目录** 的清晰结构，让"对外契约"和"内部实现"在物理上完全分离。

## 2. 最终目录结构

```
internal/ai/
├── ai.go                         # 唯一对外文件（package ai）
├── *_test.go                     # 对外契约的测试，保持现状（5 个 _test.go）
├── core/                         # package core — 引擎实现共享的内部基础设施
│   ├── adapter.go                # BaseAdapter / Invoker / Collector
│   ├── adapter_test.go
│   ├── process.go                # CmdProcess
│   ├── process_unix.go
│   ├── process_windows.go
│   ├── process_test.go
│   ├── spawn.go                  # SpawnSubprocess + SubprocessSpec
│   ├── spawn_test.go
│   ├── usage.go                  # AggregateUsage
│   └── usage_test.go
└── engine/
    ├── claude/                   # 原 internal/ai/claude/ 整体平移
    ├── codex/
    ├── kimi/
    └── pi/
```

顶层从 19 个 `.go` → **1 个 `.go` 产品代码（+ 5 个 `_test.go`）+ 2 个目录**。

### 2.1 关于 `core/` 命名的 trade-off

目录使用 `core/` 而非 `internal/`。Go 编译器不会强制限制 `internal/ai/core/` 的可见性，模块内任何包都能 import 它。本设计依赖**约定**（"core 是 ai 的内部细节"）来约束使用范围。若未来需要编译期强约束，可改为 `internal/ai/internal/core/`。

## 3. `ai.go` 内容布局

合并原 5 个顶层文件，按"由外而内"顺序排列，每段以注释分隔。预估 ~405 行。

| Section | 来源 | 内容 |
|---------|------|------|
| 1. Engine identifiers | `contracts.go` const 部分 | `EngineClaude/EngineCodex/EnginePi/EngineKimi`, `AllEngines()` |
| 2. Core contracts | `contracts.go` 主体 | `Role`, `PrepareOptions`, `RunOptions`, `Output`, `OutputType`, `Process`, `RunResult`, `TokenUsage`, `EngineAdapter`, `ErrSessionDataNotFound`, `NewRunResult` |
| 3. Registry | `registry.go` | `EngineConfig`, `Factory`, `Registry`, `DefaultRegistry`, `Register`, `New`, `ErrUnknownEngine` |
| 4. Dynamic routing | `dynamic.go` | `DynamicAdapter`, `NewDynamicAdapter` + 3 方法 |
| 5. Helper utilities | `prompt.go` + `engine_args.go` | `WorkerPersona`, `EngineArgsMap`, `ParseEngineArgs`, `splitCLIArgs`(私有) |

### 3.1 测试文件策略

5 个原顶层 `_test.go` 各自保留为独立文件，**不合并**：
- 现有 `contracts.go` 没有独立 test，`NewRunResult` 等若未被覆盖在迁移中也不补测。
- `registry_test.go`
- `dynamic_test.go`
- `prompt_test.go`
- `engine_args_test.go`

理由：单文件 500+ 行测试不利于检索，"一个文件"是产品代码的目标，测试维持现状更实用。

## 4. `core/` 包内容

```go
package core // path: internal/ai/core

// adapter.go
type Invoker interface {           // 原 ai.EngineInvoker
    Run(ctx context.Context, workDir, prompt string, opts ai.RunOptions, logPath string) (ai.Process, <-chan ai.Output, error)
}
type Collector interface {         // 原 ai.EngineCollector
    Collect(ctx context.Context, sessionID string) ([]ai.TokenUsage, error)
}
type BaseAdapter struct { Invoker; Collector }
func (BaseAdapter) Run(...) (ai.RunResult, error)
func (BaseAdapter) CollectTokenUsage(...) ([]ai.TokenUsage, error)

// process.go (+ process_unix.go / process_windows.go)
type CmdProcess struct { ... }
func NewCmdProcess(*exec.Cmd) *CmdProcess
func (CmdProcess) PID() int
func (CmdProcess) Stop() error

// spawn.go
type SubprocessSpec struct { Binary, Args, WorkDir, LogPath, Env, ... }
func SpawnSubprocess(ctx context.Context, spec SubprocessSpec) (*exec.Cmd, error)

// usage.go
func AggregateUsage[T any](path string, fold func(line T, agg map[string]*ai.TokenUsage)) ([]ai.TokenUsage, error)
```

### 4.1 命名重命名

| 原名 (package ai) | 新名 (package core) | 理由 |
|------|------|------|
| `EngineInvoker` | `Invoker` | 在 core 包里 "Engine" 前缀冗余 |
| `EngineCollector` | `Collector` | 同上 |
| `BaseAdapter` | `BaseAdapter` | 不变 |
| `CmdProcess` | `CmdProcess` | 不变 |
| `SpawnSubprocess` | `SpawnSubprocess` | 不变 |
| `SubprocessSpec` | `SubprocessSpec` | 不变 |
| `AggregateUsage` | `AggregateUsage` | 不变 |

## 5. 依赖图

```
       ┌────────────────────┐
       │  ai (package ai)   │ ← ai.go：所有对外类型与方法
       └──────────▲─────────┘
                  │ import
       ┌──────────┴─────────┐
       │  core (package core) │ ← 引用 ai 类型，提供 BaseAdapter / CmdProcess / SpawnSubprocess / AggregateUsage
       └──────────▲─────────┘
                  │ import
   ┌──────────────┼──────────────┬──────────────┐
   ▼              ▼              ▼              ▼
 claude         codex           kimi             pi    ← import ai + core；init() 调用 ai.Register()
```

**关键不变量**：
- `ai` 不 import `core`（无环）。
- `engine/*` 同时 import `ai` 和 `core`。
- 上层 caller 通过 `import _ "internal/ai/engine/claude"` 触发副作用注册。

## 6. 外部 caller 迁移

外部 import 路径变化只有一项：

```
"github.com/.../internal/ai/{claude,codex,kimi,pi}"
  →  "github.com/.../internal/ai/engine/{claude,codex,kimi,pi}"
```

`internal/ai` 本身的 import 路径不变。`ai.New() / ai.EngineAdapter / ai.EngineConfig / ai.RunOptions` 等全部对外类型名零变更。

预估涉及外部修改 ~10 个文件（`cmd/openbee/`、`internal/app/`、`internal/tokenstat/`、`internal/domain/...` 等 import 了 engine 子包的地方）。纯路径批量改名。

## 7. YAGNI / 不做的事

- 不动 `engine/{claude,codex,kimi,pi}/` 内部文件结构（`adapter.go`/`invoker.go`/`token_usage.go` 等保留）。
- 不动 `engine_args` / `prompt` 的功能行为，仅搬位置。
- 不动 `contracts.go` 里任何已有类型的字段。
- 不新增测试用例（重构不引入新行为）。
- 不引入新的抽象层、不重写 init 注册机制。

## 8. 测试与验证

**核心原则**：所有现有 `*_test.go` 必须继续 PASS，不允许借机改测试期望。

测试文件迁移：
- `internal/ai/*_test.go`（5 个顶层）→ 留在顶层，package `ai`，零修改。
- `internal/ai/{base_adapter,process,spawn,usage}_test.go` → 平移至 `core/`，package 改为 `core`；原文件在 package ai 中裸引用的类型（如 `RunOptions`、`Process`），改为 `ai.RunOptions`、`ai.Process` 等带包名引用。
- `internal/ai/{claude,codex,kimi,pi}/*_test.go` → 跟着 engine 整体平移；改用 `core.BaseAdapter` 等。

验证命令：

```
go build ./...
go test ./internal/ai/...
go test ./...                # 全仓回归
```

外加本地烟测：跑一个最简单的 worker 任务，确认 engine init 注册没断。

## 9. 实施步骤（高层）

1. 建 `internal/ai/core/`、`internal/ai/engine/` 空目录。
2. `git mv` 4 个 engine 目录到 `engine/` 下，**先不改代码**，确认 import 路径修改后 `go build` 通过。
3. 把 `base_adapter`/`process`/`spawn`/`usage` 4 组文件移到 `core/`，重命名 `EngineInvoker → Invoker`、`EngineCollector → Collector`；engine/* 同步替换 `ai.XXX → core.XXX`。
4. 合并 `contracts/registry/dynamic/prompt/engine_args` 5 个文件为 `ai.go`，删除旧文件，加 section 注释。
5. 更新 ~10 个外部 caller 的 engine 子包 import 路径。
6. 全量验证：`go build ./... && go test ./...`。
7. 本地烟测一个 worker 任务。

每一步独立 commit，便于 review 和回滚。

## 10. 风险与回滚

- **风险**：engine 通过 `init()` 自注册 — 外部 caller 必须显式 `import _ "...engine/xxx"`；若漏改 import，注册不会触发，`ai.New("claude")` 会返回 `ErrUnknownEngine`。烟测要确保覆盖。
- **回滚**：每步独立 commit，出问题可单步 revert。
