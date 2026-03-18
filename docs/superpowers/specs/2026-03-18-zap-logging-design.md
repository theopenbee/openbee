# ZAP 日志系统迁移设计规格

**日期**：2026-03-18
**状态**：待审批
**驱动因素**：高级功能（采样/动态级别/stacktrace）+ 团队规范统一

---

## 1. 背景与目标

### 现状

项目当前使用 Go 标准库 `log/slog`，零外部日志依赖，18 个文件中约 158 处日志调用。所有调用均采用结构化 key-value 格式，并通过 `"component"` 字段标识来源组件。日志输出到 stderr，无显式初始化，无动态级别控制，无日志轮转。

### 迁移目标

1. **高级功能**：引入 ZAP 的采样（Sampling）、原子动态级别（AtomicLevel）、Error 级 Stacktrace 等 slog 不具备的能力
2. **规范统一**：通过 `internal/logger` 封装层定义团队标准日志 API，各组件不直接依赖 `go.uber.org/zap`
3. **强类型字段**：使用 `zap.String`、`zap.Int`、`zap.Error` 等编译期类型安全字段，避免 slog 松散 key-value 的键错位问题

### 非目标

- 不引入日志文件输出或日志轮转（保持输出到 stderr）
- 不实现多后端（wrapper 封装 ZAP，不设计可插拔的日志后端接口）
- 不做跨服务日志追踪（无 trace ID 注入）

---

## 2. 架构设计

### 包结构

```
internal/logger/
├── logger.go    # Logger 类型定义、构造函数、Info/Warn/Error/Debug/With 方法
├── global.go    # 全局默认 logger、Init()、包级别便捷函数、SetLevel()
└── config.go    # Config 和 SamplingConfig 结构体定义
```

### 依赖关系

```
各业务组件 (dingtalk, feeder, dispatcher...)
    ↓ import
internal/logger          ← 唯一暴露给业务层的日志 API
    ↓ import
go.uber.org/zap          ← 仅在 internal/logger 内部使用
go.uber.org/zap/zapcore
```

业务代码中**唯一允许**出现的日志导入路径：
```go
import "github.com/theopenbee/openbee/internal/logger"
```

---

## 3. API 设计

### 3.1 Config

```go
// Config 控制 logger 的初始化行为
type Config struct {
    // Level 设置最低输出级别，有效值："debug", "info", "warn", "error"
    // 默认："info"
    Level string

    // Format 控制输出格式
    // "json"    → 结构化 JSON，适合日志平台对接
    // "console" → 人类可读格式，适合本地开发
    // 默认："json"
    Format string

    // Sampling 启用日志采样，nil 表示不采样
    Sampling *SamplingConfig

    // StacktraceLevel 开始附加 stacktrace 的级别
    // 默认："error"
    StacktraceLevel string
}

// SamplingConfig 控制高频场景下的日志降噪
type SamplingConfig struct {
    // Tick 是采样的时间窗口，默认 1s
    Tick time.Duration
    // Initial 是每个 Tick 内完整记录的条数上限
    Initial int
    // Thereafter 是超出 Initial 后每隔多少条记录一条
    Thereafter int
}
```

### 3.2 Logger 类型

```go
// Logger 是对 zap.Logger 的轻量封装
type Logger struct {
    zl *zap.Logger
}

// With 返回附加了预置字段的子 logger，不修改原 logger
// 主要用于各组件在包级别预置 "component" 字段
func (l *Logger) With(fields ...zap.Field) *Logger

func (l *Logger) Info(msg string, fields ...zap.Field)
func (l *Logger) Warn(msg string, fields ...zap.Field)
func (l *Logger) Error(msg string, fields ...zap.Field)
func (l *Logger) Debug(msg string, fields ...zap.Field)
```

### 3.3 全局函数（global.go）

```go
// Init 使用给定配置初始化全局 logger，应在 main() 最早处调用一次
func Init(cfg Config) error

// With 在全局 logger 上附加字段，返回子 logger
// 各组件包级别调用：var log = logger.With(zap.String("component", "dingtalk"))
func With(fields ...zap.Field) *Logger

// SetLevel 运行时动态调整全局日志级别，无需重启
func SetLevel(level zapcore.Level)

// LevelHandler 返回可挂载到 HTTP 路由的动态级别接口
// PUT /log/level {"level":"debug"} 即可切换
func LevelHandler() http.Handler

// 包级别便捷函数，等价于在全局 logger 上调用同名方法
func Info(msg string, fields ...zap.Field)
func Warn(msg string, fields ...zap.Field)
func Error(msg string, fields ...zap.Field)
func Debug(msg string, fields ...zap.Field)
```

### 3.4 各组件的使用模式

```go
// 各组件文件顶部，包级别声明一个带 component 字段的子 logger
var log = logger.With(zap.String("component", "dingtalk"))

// 函数内直接使用
log.Info("message received", zap.String("msgtype", msgtype))
log.Error("reconnect failed", zap.Error(err))
log.Warn("heartbeat timeout",
    zap.String("workerID", id),
    zap.Duration("elapsed", elapsed),
)
```

---

## 4. 初始化集成

在 `cmd/openbee/main.go` 或 `internal/app/app.go` 的启动流程中，最早处调用：

```go
if err := logger.Init(logger.Config{
    Level:  cfg.Log.Level,   // 从应用配置读取，默认 "info"
    Format: cfg.Log.Format,  // 从应用配置读取，默认 "json"
}); err != nil {
    panic(err)
}
```

若需暴露动态级别 HTTP 接口，在路由注册阶段：

```go
router.PUT("/internal/log/level", gin.WrapH(logger.LevelHandler()))
```

---

## 5. ZAP 核心功能启用策略

| 功能 | 默认状态 | 配置方式 |
|------|---------|---------|
| JSON 格式输出 | 开启（Format="json"） | Config.Format |
| Caller 信息（file:line） | 开启 | 固定开启，不暴露配置 |
| Error 级 Stacktrace | 开启 | Config.StacktraceLevel |
| 采样 | 关闭 | Config.Sampling 非 nil 时启用 |
| 动态级别调整 | 始终可用 | logger.SetLevel() / HTTP 接口 |

---

## 6. 迁移策略

### 原则

- **分阶段、可回滚**：每个阶段独立可合并，不影响线上运行
- **先搭桥、再迁移**：阶段 1 完成后系统即可正常运行，后续阶段是逐步优化
- **优先高频**：先迁移日志调用最多的组件，尽快完成主要迁移量

### 阶段 1：基础设施（不影响现有代码）

**目标**：添加 `internal/logger` 包，在 `main.go` 初始化，用 ZAP 的 slog bridge 让现有 slog 调用临时走 ZAP 输出。

**改动范围**：
- 新建 `internal/logger/` 包（3 个文件）
- 修改 `go.mod` 添加 `go.uber.org/zap`
- 修改 `cmd/openbee/main.go` 添加 logger.Init() 调用（可同时设置 slog bridge）

**验证**：启动服务，确认日志以 JSON 格式正常输出

### 阶段 2：迁移平台 handler（约 60% 日志量）

**目标文件**（日志最密集）：
- `internal/platform/dingtalk/handler.go`
- `internal/platform/feishu/handler.go`
- `internal/platform/wecom/handler.go`
- `internal/platform/wecom/wsconn.go`
- `internal/platform/local/hub.go`
- `internal/platform/local/receiver.go`

**迁移模式**（每个文件）：
1. 移除 `"log/slog"` import，添加 `"github.com/theopenbee/openbee/internal/logger"` 和 `"go.uber.org/zap"`
2. 在包级别声明 `var log = logger.With(zap.String("component", "<name>"))`
3. 将 `slog.Info("msg", "k1", v1, "k2", v2)` 改为 `log.Info("msg", zap.String("k1", v1), zap.String("k2", v2))`（注意字段类型选择）
4. `slog.Error("msg", "error", err)` 改为 `log.Error("msg", zap.Error(err))`

### 阶段 3：迁移核心组件

**目标文件**：
- `internal/task_dispatcher/dispatcher.go`
- `internal/task_dispatcher/failure_notifier.go`
- `internal/task_scheduler/scheduler.go`
- `internal/bee/feeder.go`
- `internal/worker/manager.go`
- `internal/msgingest/gateway.go`

迁移方式同阶段 2。

### 阶段 4：扫尾与清理

**目标文件**：
- `internal/mcp/server.go`
- `internal/mcp/tools.go`
- `internal/api/local_chat_handler.go`
- `internal/ffmedia/ffmedia.go`
- `internal/app/app.go`
- `cmd/openbee/main.go`（移除 slog bridge）

完成后：
- 确认全库无 `"log/slog"` import（可通过 `grep -r "log/slog"` 验证）
- 移除 slog bridge 相关代码

---

## 7. 字段类型选择指南

| Go 类型 | ZAP 字段 |
|---------|---------|
| `string` | `zap.String("key", val)` |
| `int` / `int64` | `zap.Int("key", val)` / `zap.Int64("key", val)` |
| `error` | `zap.Error(err)` |
| `time.Duration` | `zap.Duration("key", d)` |
| `time.Time` | `zap.Time("key", t)` |
| `bool` | `zap.Bool("key", val)` |
| 任意结构体 | `zap.Any("key", val)`（慎用，有反射开销） |
| `[]string` 等切片 | `zap.Strings("key", val)` |

---

## 8. 测试策略

- `internal/logger` 包本身：单元测试覆盖 Init、With、SetLevel 行为；使用 `zaptest.NewLogger` 在测试中捕获日志输出
- 各业务组件：迁移后已有测试继续运行，无需专门新增测试
- 集成验证：在 CI 中添加 `grep -r '"log/slog"' internal/` 断言，确保迁移完成后不再有 slog 直接调用

---

## 9. 依赖变更

新增直接依赖：

```
go.uber.org/zap v1.27.x
go.uber.org/multierr v1.11.x  (zap 传递依赖)
go.uber.org/atomic v1.11.x    (zap 传递依赖)
```

无间接影响，不修改现有依赖。

---

## 10. 风险与缓解

| 风险 | 可能性 | 缓解措施 |
|------|-------|---------|
| 字段键错位（key 对应错误类型） | 中 | 强类型字段在编译期报错，无法漏掉 |
| 遗漏迁移某文件 | 低 | 阶段 4 用 grep 全库验证 |
| slog bridge 行为差异 | 低 | bridge 仅在阶段 1 临时使用，阶段 4 移除 |
| ZAP 初始化失败 | 极低 | Init 返回 error，main 中 panic 快速失败 |
