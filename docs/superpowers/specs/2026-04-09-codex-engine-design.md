# Codex Engine 适配器设计

**日期**：2026-04-09
**状态**：已批准
**范围**：在现有引擎插件架构下新增 OpenAI Codex CLI 作为第二个引擎实现

---

## 背景

openbee 已完成引擎插件化（`EngineAdapter` 接口 + 注册表），Claude Code 为默认实现。本设计在此基础上新增 `codex` 引擎，对接 OpenAI Codex CLI（`codex` 二进制），domain 层零改动。

---

## 目标

- 实现 `codex` 引擎适配器，注册为 `"codex"`
- 工作区初始化使用 `AGENTS.md` + `.openbee.md`（镜像 Claude 的双文件方案）
- 非交互式 JSON 流模式调用，行为与 Claude 引擎一致
- 解析 Codex 输出流中的 session ID 并通过 Output channel 上报
- 只支持 OpenAI 官方（`OPENAI_API_KEY` 由用户环境提供）

---

## 包结构

```
internal/ai/codex/
  adapter.go    - EngineAdapter 实现 + init() 自注册为 "codex"
  invoker.go    - codex exec 子进程管理、JSON 流解析、session ID 提取
  agentsmd.go   - AGENTS.md / .openbee.md 内容生成（bee / worker 两种角色）
```

相比 Claude 适配器更精简：无 provider 选择逻辑、无二进制下载逻辑。

---

## Workspace 初始化

`SetupWorkspace` 写入两个文件，均幂等（`O_EXCL`，已存在则跳过）。

### Worker 角色

**`AGENTS.md`**
```
@.openbee.md
```

**`.openbee.md`**
```
# {name}

{description}

## Memory
{memory}

## Rules
{openbee worker 系统规则}
```

### Bee 角色

**`AGENTS.md`**
```
You are B, an AI assistant.

@.openbee.md
```

**`.openbee.md`**
```
{openbee bee 系统规则}
```

---

## 进程调用

### 新 Session

```
codex exec - --json --yolo
stdin = prompt
```

启动后开 goroutine 实时读 JSON 流：
1. 解析到 session ID 字段时，立即发送 `Output{Type: OutputSessionID, Content: "<id>"}`
2. 进程退出后发 `Output{Type: OutputDone}` 或 `Output{Type: OutputError, Content: err}`

### Resume Session

```
codex exec resume SESSION_ID --json --yolo
```

若 prompt 非空，追加为位置参数（Codex follow-up prompt）。

### 环境变量

与 Claude 引擎一致：
- `OPENBEE_URL`：openbee MCP base URL（openbee ctl 子命令依赖）
- `OPENBEE_API_KEY`：短期 auth token（openbee ctl 子命令依赖）
- `OPENAI_API_KEY`：由用户系统环境透传，适配器不注入

### Session ID 限制

Codex 不支持预指定 session ID（Claude 有 `--session-id`）。新 session 的 ID 由 Codex 自动生成，通过 `OutputSessionID` 事件上报给上层，上层负责持久化以供后续 resume 使用。

---

## 接口变动

**`internal/ai/engine.go`** 新增一个 OutputType：

```go
const (
    OutputDone      OutputType = "done"
    OutputError     OutputType = "error"
    OutputSessionID OutputType = "session_id" // 新增：Content 字段携带 session ID
)
```

`Output` 结构体不变，`Content` 复用。

---

## 配置变更

**`internal/infra/config/config.go`**

```go
type CodexConfig struct {
    Path string `yaml:"path"`
}

// BeeConfig 新增字段：
Codex CodexConfig `yaml:"codex"`

// EngineConfigRaw() 新增 case：
case "codex":
    return map[string]any{"path": b.Codex.Path}
```

**`config.yaml` 示例**

```yaml
bee:
  engine: codex
  codex:
    path: codex   # 空值默认 "codex"
```

**`internal/app/app.go`** 新增 blank import：

```go
import _ "github.com/theopenbee/openbee/internal/ai/codex"
```

---

## 实现注意事项

- Codex JSON 流中 session ID 的具体字段名需在实现阶段读取实际输出确认
- `AGENTS.md` 中 `@.openbee.md` 引用语法需验证 Codex 是否与 Claude 一致
- Codex JSON 输出格式（事件类型、结构）与 Claude stream-json 不同，`ExtractResultFromLog` 需单独实现
