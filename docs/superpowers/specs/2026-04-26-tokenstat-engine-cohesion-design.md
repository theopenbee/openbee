# tokenstat 与 ai engine 内聚化重构设计

- 日期：2026-04-26
- 作者：小乔
- 状态：待评审

## 背景

当前 `tokenstat` 包同时承担两件事：
1. 调度 / 存储：`Syncer` 每 10 分钟轮询、查询待同步 session、写 `bee_token_stats` 表。
2. 引擎特定的 token 解析：`claude.go` / `codex.go` / `pi.go` / `kimi.go`，每个文件实现 `Parser` 接口、读取该 engine 自有 session 制品（JSONL 文件等）。

这导致每个 engine 的"运行时"和"token 解析"被拆到了两个包：
- `internal/ai/<engine>/adapter.go` —— engine 注册与对话生命周期
- `internal/tokenstat/<engine>.go` —— engine 的 token 解析

未来新增 engine 需要同时在两处改代码，且解析逻辑往往依赖 engine 的内部约定（session 文件路径、字段格式），放在外部包不利于内聚。

## 目标

把 4 个 engine 的 token parser 实现物理搬到 `internal/ai/<engine>/` 下，作为 `ai.Engine` 接口的一个方法实现。`tokenstat` 包退化为"调度 + 存储"两件事。

## 非目标

- 不改变数据流模型（仍是 pull / 文件轮询，不引入 push 模式）。
- 不修改 `bee_token_stats` 表结构 / store 接口 / API handler。
- 不更名 `tokenstat` 包（store 表名、API 字段都已用此名）。
- 不重构未涉及代码（YAGNI）。

## 设计

### 架构总览

- 数据流不变：syncer 每 10 分钟轮询 → 找出待同步 session → 按 engine 派发解析 → 写 store；tombstone 机制保留。
- 依赖方向：`tokenstat → ai`（单向）。`internal/ai` 不感知 store / syncer 的存在。
- 接口形态：A1 强制接口 —— 所有 engine 必须实现 `ParseTokenUsage`。

### 共享类型与接口（在 `internal/ai`）

新增文件 `internal/ai/token_usage.go`：

```go
package ai

// TokenUsage 表示某个 session 在某个模型下的累计 token 消耗。
type TokenUsage struct {
    Model        string
    InputTokens  int64
    OutputTokens int64
    CacheCreate  int64
    CacheRead    int64
}

// TombstoneModel 标记"已扫过但无可解析数据"，避免无限重扫。
const TombstoneModel = "unknown"
```

> 实际字段名沿用 `tokenstat.Usage` 的现有命名以保持 store 写入逻辑零改动。

扩展 `internal/ai/engine.go` 中的 `Engine` 接口：

```go
type Engine interface {
    // ...现有方法...

    // ParseTokenUsage 从 engine 自有的 session 制品中解析 token 使用量。
    // 无可解析数据时返回空切片 + nil；syncer 据此写入 tombstone。
    ParseTokenUsage(sessionID string) ([]TokenUsage, error)
}
```

### 文件搬迁映射

| 现状 | 搬迁后 |
|---|---|
| `internal/tokenstat/claude.go` | `internal/ai/claude/token_usage.go` |
| `internal/tokenstat/codex.go` | `internal/ai/codex/token_usage.go` |
| `internal/tokenstat/pi.go` | `internal/ai/pi/token_usage.go` |
| `internal/tokenstat/kimi.go` | `internal/ai/kimi/token_usage.go` |
| `internal/tokenstat/parser.go` | 删除（接口归并入 `ai.Engine`） |
| `internal/tokenstat/syncer.go` | 保留，移除 parser 注册逻辑 |
| `internal/tokenstat/store/*` | 保留 |

每个 engine 包下，原 parser 的 `Parse(sessionID)` 函数变成该 engine adapter 的 `ParseTokenUsage(sessionID)` 方法（receiver 为已有 adapter struct）。文件级辅助函数（读 JSONL、聚合等）随包搬过去，作为包内私有 helper。`*_test.go` 同步搬迁。

### Syncer 改造

文件：`internal/tokenstat/syncer.go`

- 构造函数 `NewSyncer` 不再接收 / 内置 parser 列表，改为持有 `map[string]ai.Engine`（engine name → 实例），由 `internal/app/app.go` 在装配阶段注入。
- 派发逻辑（preferred engine → fallback）保留语义，调用从 `parser.Parse(sessionID)` 改为 `engine.ParseTokenUsage(sessionID)`。
- tombstone 写入逻辑保留不动；`TombstoneModel` 引用从 `ai` 包来。

### 不变的部分

- 10 分钟轮询节奏。
- `bee_token_stats` 表结构、`token_stats_store` 接口。
- API handler（`internal/api/session_handler.go`）。
- engine 注册机制（`ai.Register()` / `ai.AllEngines()`）。

## 影响面

- 修改：`internal/ai/engine.go`、`internal/ai/<engine>/adapter.go`（每个 engine 各加一个方法）、`internal/tokenstat/syncer.go`、`internal/app/app.go`（装配时注入 engine map）。
- 新增：`internal/ai/token_usage.go`、`internal/ai/<engine>/token_usage.go`（4 个）及对应 `_test.go`。
- 删除：`internal/tokenstat/{claude,codex,pi,kimi,parser}.go` 及对应测试。

## 测试策略

- 现有 `internal/tokenstat/<engine>_test.go` 测试用例随实现搬到 `internal/ai/<engine>/`，确保 fixture 与断言不变（行为零回归）。
- syncer 的派发与 tombstone 测试保留在 `tokenstat` 包，使用一个测试用 fake `ai.Engine` 替代真实 engine。

## 风险与回滚

- 风险点：`Engine` 接口扩展属于破坏性改动，所有 engine 实现需同步更新；好处是编译期保证不漏。
- 回滚：单 PR 可整体 revert。store schema 与 API 不变，无数据迁移。
