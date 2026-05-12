# Claude Adapter Prepare 高内聚改造

日期：2026-05-12
作者：貂蝉
状态：待实施

## 背景

`claudeAdapter.Prepare`（`internal/ai/engine/claude/adapter.go:46`）当前作为 `ai.EngineAdapter` 接口（`internal/ai/ai.go:113`）的公开方法被外部直接调用：

- `internal/domain/worker/worker.go:187`
- `internal/domain/worker/execution.go:35`
- `internal/domain/bee/feeder.go:121`
- `internal/domain/bee/bee_process.go:45`（再包一层）

只有 claude 引擎在 `Prepare` 内做事（清理遗留的 `.openbee.md` 文件和 `CLAUDE.md` 中 `@.openbee.md` 引用行），其余引擎都是 no-op。

## 问题

1. **耦合：调用方需要知道"先 Prepare 再 Run"的顺序**，可能漏调或重复调；接口层把生命周期细节暴露出去了。
2. **冗余接口方法**：其他三个引擎被迫实现空 `Prepare`。
3. **责任泄漏**：清理遗留文件本质是 claude 引擎自己的事，不应该需要外部协作。

## 目标

让 claude 的"清理遗留配置"逻辑完全收敛在 claude 包内，外部只通过 `Run` 与引擎交互。

## 设计

### 接口变更

从 `ai.EngineAdapter` 接口删除 `Prepare`：

```go
type EngineAdapter interface {
    Run(ctx context.Context, workDir, prompt string,
        opts RunOptions, logPath string) (RunResult, error)
    CollectTokenUsage(ctx context.Context, sessionID string) ([]TokenUsage, error)
}
```

`PrepareOptions` 类型不再被接口使用，整体删除。`ai.Role` 类型保留（在 `core/session.go`、`task/dispatcher.go`、`bee/feeder.go` 等位置仍被使用）。

### core.BaseAdapter

删除 `core/adapter.go:43` 的 `Prepare` 默认实现。其他引擎（codex/kimi/pi）不需要任何改动。

### claudeAdapter

把 `Prepare` 方法降级为包内私有函数 `cleanupLegacyRules(workDir string) error`（保留 `removeImportLine` 不变），并在 claude 自己的 `Run` 里调用：

```go
func (a *claudeAdapter) Run(ctx context.Context, workDir, prompt string,
    opts ai.RunOptions, logPath string) (ai.RunResult, error) {
    if err := cleanupLegacyRules(workDir); err != nil {
        return ai.RunResult{}, err
    }
    return a.BaseAdapter.Run(ctx, workDir, prompt, opts, logPath)
}
```

清理函数本身是幂等且 I/O 开销极小（两次 stat，最多一次 read+write），不需要 `sync.Once` 缓存——简单可靠优先。

`SystemRulesFile` 和 `ImportLine` 常量从 `Exported` 降级为 `unexported`（`systemRulesFile`、`importLine`），因为外部不再需要引用。

### DynamicAdapter

`internal/ai/ai.go:226` 的 `DynamicAdapter.Prepare` 整体删除。`DynamicAdapter` 的 `Run` 已经只路由到选中的引擎，逻辑天然正确：当且仅当用户切换到 claude 时才会触发清理。这是合理的——遗留文件本身就是 claude 专属的。

### 调用方清理

删除以下四处对 `Prepare` 的显式调用：

- `internal/domain/worker/worker.go:187`
- `internal/domain/worker/execution.go:35`
- `internal/domain/bee/feeder.go:121`
- `internal/domain/bee/bee_process.go:45-47`（整个 `BeeProcess.Prepare` 包装方法删除）

需要确认这些位置在调用 `Prepare` 之后没有依赖其副作用——只有 claude 真的有副作用，副作用现在跟着 `Run` 走，逻辑等价。

### 测试调整

现有 claude 测试在 `claude_test` 外部包通过 `Prepare` 验证清理。改造方案：

- 将 `internal/ai/engine/claude/adapter_test.go` 从 `claude_test` 改为 `claude` 同包测试，直接调用 `cleanupLegacyRules(dir)` 验证清理行为。
- 不再为"通过 Run 走清理路径"再写一个集成测试——`Run` 启动真实子进程，单测代价不成比例；保留单测覆盖 `cleanupLegacyRules` 已足够。

其它需要更新的测试 stub（删除 `Prepare` 方法及 `prepared` 字段，相关测试用例同步删除）：

- `internal/ai/ai_test.go`（`stubEngine.Prepare`、`stubAdapter.Prepare`；删除 `TestDynamicAdapter_PrepareCallsAll`）
- `internal/ai/core/adapter_test.go` 删除 `TestBaseAdapter_PrepareIsNoop`
- `internal/ai/engine/codex/adapter_test.go` 删除 `TestAdapter_Prepare_NoOp`、`TestAdapter_Prepare_BothRoles`
- `internal/ai/engine/kimi/adapter_test.go` 删除对应 `Prepare` 测试
- `internal/ai/engine/pi/adapter_test.go` 删除对应 `Prepare` 测试
- `internal/domain/worker/manager_test.go`（`mockEngine.Prepare`、`silentMockEngine.Prepare`）
- `internal/domain/bee/feeder_test.go`（`mockBeeRunner.Prepare`、`callbackBeeRunner.Prepare`）
- `internal/rpc/tools_test.go`（`stubEngineAdapter.Prepare`）
- `internal/tokenstat/syncer_test.go`（`fakeAdapter.Prepare`）

## 影响范围

接口减一个方法 + 调用方四处删除 + 测试 stub 同步删除。改动是机械式的，主要风险点是测试同步。

## 验证

- `go build ./...`
- `go test ./...`
- 重点关注 `internal/ai/...`、`internal/domain/worker/...`、`internal/domain/bee/...`、`internal/rpc/...`、`internal/tokenstat/...` 包的测试。
