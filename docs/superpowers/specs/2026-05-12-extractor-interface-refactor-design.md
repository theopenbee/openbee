# Extractor Interface Refactor — Design

## 背景

`internal/ai/core/adapter.go` 中的 `BaseAdapter` 当前定义如下：

```go
type BaseAdapter struct {
    Invoker   Invoker
    Collector Collector
    // Extract is the per-engine result extractor bound to logPath in Run.
    Extract func(logPath string) string
}
```

`Invoker` 和 `Collector` 都是 interface 类型，由各 engine（claude / codex / kimi / pi）用具体 struct 实现；唯独 `Extract` 是裸 `func` 字段，由各 engine 传入自己的 `ExtractResultFromLog` 函数赋值。

四个 engine 当前的赋值现场：

```go
// internal/ai/engine/claude/adapter.go
Extract:   ExtractResultFromLog,
// internal/ai/engine/codex/adapter.go
Extract:   ExtractResultFromLog,
// internal/ai/engine/kimi/adapter.go
Extract:   ExtractResultFromLog,
// internal/ai/engine/pi/adapter.go
Extract:   ExtractResultFromLog,
```

每个 engine 包内都定义了一个独立的 `ExtractResultFromLog(logPath string) string` 函数，并有对应的单元测试直接调用该函数。

## 问题

1. **三个字段风格不一致**。`Invoker` / `Collector` 是 interface，`Extract` 是裸 `func`，看起来像是"还没收尾"的设计。
2. **裸 func 字段不利于扩展**。若未来 extractor 需要持有状态（缓存、配置、依赖注入），`func` 类型无法承载，必须先做一轮重构。
3. **类型语义弱**。`func(string) string` 这个签名没有领域意义，看不出它是 result extractor；类型化后 IDE/调用方一眼能看出契约。

## 目标

- 把 `Extract` 改成与 `Invoker` / `Collector` 对称的 interface。
- 各 engine 提供一个具体 struct 实现该 interface，函数体直接以 method 形式承载（不保留游离的 `ExtractResultFromLog` 函数）。
- 现有单元测试同步迁移到通过 method 调用，不留下两条等价的代码路径。
- 不引入新行为、不改变现有提取逻辑、不破坏外部契约。

## 非目标

- 不引入 Extractor 的有状态实现（保持 zero-value struct，纯函数式行为）。
- 不动 `Invoker` / `Collector` / `Prepare` 等其它 BaseAdapter 字段。
- 不改 `RunResult` 的 API，`Run` 内部仍通过闭包绑定 `logPath`。

## 设计

### 1. core 层：定义 Extractor 接口

`internal/ai/core/adapter.go`：

```go
// Extractor reads the per-engine result text from a log file.
type Extractor interface {
    Extract(logPath string) string
}

type BaseAdapter struct {
    Invoker   Invoker
    Collector Collector
    Extractor Extractor
}

func (b *BaseAdapter) Run(ctx context.Context, workDir, prompt string,
    opts ai.RunOptions, logPath string) (ai.RunResult, error) {
    proc, out, err := b.Invoker.Run(ctx, workDir, prompt, opts, logPath)
    return ai.NewRunResult(proc, out, err, func() string {
        return b.Extractor.Extract(logPath)
    })
}
```

字段命名：选 `Extractor`（与 `Invoker` / `Collector` 同形），不是 `Extract`，避免和接口方法名冲突且更对称。

### 2. 各 engine：把函数搬到 method 上

每个 engine 包内做相同模式的迁移。以 claude 为例：

**Before**（`internal/ai/engine/claude/invoker.go`）：

```go
func ExtractResultFromLog(logPath string) string {
    // ... 既有逻辑 ...
}
```

**After**：

```go
type Extractor struct{}

func (Extractor) Extract(logPath string) string {
    // ... 既有逻辑（原样搬运，不改实现）...
}
```

`adapter.go` 内：

```go
// Before
Extract:   ExtractResultFromLog,
// After
Extractor: Extractor{},
```

四个 engine（claude / codex / kimi / pi）都做同样的改造。

### 3. 单元测试迁移

四个 engine 各自的 `invoker_test.go` 中所有 `ExtractResultFromLog(path)` 调用点，改为 `Extractor{}.Extract(path)`。函数名和断言不动，只换调用形式。

`internal/ai/core/adapter_test.go` 内的 fake：

**Before**：

```go
b := &core.BaseAdapter{
    Invoker:   &fakeInvoker{ch: ch},
    Collector: &fakeCollector{},
    Extract:   func(logPath string) string { ... },
}
```

**After**：新增一个 `fakeExtractor` 类型，与 `fakeInvoker` / `fakeCollector` 对称：

```go
type fakeExtractor struct {
    captured *string
    result   string
}

func (f *fakeExtractor) Extract(logPath string) string {
    if f.captured != nil {
        *f.captured = logPath
    }
    return f.result
}
```

测试用例改为传入 `&fakeExtractor{...}`。

## 影响面

涉及文件（共 13 个）：

- `internal/ai/core/adapter.go` — 接口定义 + `BaseAdapter` 字段
- `internal/ai/core/adapter_test.go` — fakeExtractor + 测试用例迁移
- `internal/ai/engine/claude/invoker.go` — `ExtractResultFromLog` → `Extractor.Extract`
- `internal/ai/engine/claude/adapter.go` — 字段赋值
- `internal/ai/engine/codex/invoker.go` — 同上
- `internal/ai/engine/codex/adapter.go` — 字段赋值
- `internal/ai/engine/codex/invoker_test.go` — 调用点迁移
- `internal/ai/engine/kimi/invoker.go` — 同上
- `internal/ai/engine/kimi/adapter.go` — 字段赋值
- `internal/ai/engine/kimi/invoker_test.go` — 调用点迁移
- `internal/ai/engine/pi/invoker.go` — 同上
- `internal/ai/engine/pi/adapter.go` — 字段赋值
- `internal/ai/engine/pi/invoker_test.go` — 调用点迁移

注：claude 当前没有 `invoker_test.go` 直接测 `ExtractResultFromLog`，无需动测试文件。

## 风险与对策

- **风险**：method 改名后外部包若有引用 `ExtractResultFromLog` 会编译失败。
  **对策**：grep 已确认全仓所有调用点都在 engine 自己的 `adapter.go` / `invoker_test.go` 内，无外部引用。
- **风险**：测试逻辑迁移时漏改某处调用点。
  **对策**：迁移完成后 `go build ./... && go test ./...`，编译会报缺失符号，测试会跑出回归。

## 验收标准

- `go build ./...` 通过。
- `go test ./internal/ai/...` 全绿。
- 全仓 `grep ExtractResultFromLog` 应当无结果（函数已不再存在，调用点全部迁移）。
- `BaseAdapter` 的三个字段类型风格一致（均为 interface）。
