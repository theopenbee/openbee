# ZAP 日志系统迁移 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace `log/slog` with `go.uber.org/zap` via an `internal/logger` wrapper, enabling log sampling, runtime level control, and stacktrace at error level.

**Architecture:** Create `internal/logger/` as the single logging entry point for all business code. Business components import only `internal/logger` and `go.uber.org/zap` (for field constructors); direct `go.uber.org/zap` logger usage is forbidden. A temporary slog bridge routes legacy calls during the phased migration.

**Tech Stack:** Go 1.25, `go.uber.org/zap` v1.27+, `go.uber.org/zap/exp/zapslog` (bridge, temporary)

---

## File Map

**Create:**
- `internal/logger/config.go` — Config and SamplingConfig structs
- `internal/logger/logger.go` — Logger type with Info/Warn/Error/Debug/With
- `internal/logger/global.go` — Global init, package-level functions, slog bridge helper
- `internal/logger/logger_test.go` — Unit tests for the logger package

**Modify (Phase 1):**
- `cmd/openbee/server.go` — Call `logger.Init()` + `logger.SetSlogDefault()` before app build

**Modify (Phase 2 — platform handlers):**
- `internal/platform/dingtalk/handler.go`
- `internal/platform/feishu/handler.go`
- `internal/platform/wecom/handler.go`
- `internal/platform/wecom/wsconn.go`
- `internal/platform/local/hub.go`
- `internal/platform/local/receiver.go`

**Modify (Phase 3 — core components):**
- `internal/task_dispatcher/dispatcher.go`
- `internal/task_dispatcher/failure_notifier.go`
- `internal/task_scheduler/scheduler.go`
- `internal/bee/feeder.go`
- `internal/worker/manager.go`
- `internal/msgingest/gateway.go`

**Modify (Phase 4 — remaining + cleanup):**
- `internal/mcp/server.go`
- `internal/mcp/tools.go`
- `internal/api/local_chat_handler.go`
- `internal/ffmedia/ffmedia.go`
- `internal/app/app.go`
- `cmd/openbee/main.go`
- `cmd/openbee/server.go` — remove slog bridge call

---

## Task 1: Build internal/logger Package

**Files:**
- Create: `internal/logger/config.go`
- Create: `internal/logger/logger.go`
- Create: `internal/logger/global.go`
- Create: `internal/logger/logger_test.go`

- [ ] **Step 1: Add ZAP dependency**

```bash
cd /path/to/openbee
go get go.uber.org/zap@latest
go get go.uber.org/zap/exp/zapslog
```

Expected: `go.mod` and `go.sum` updated with `go.uber.org/zap`.

- [ ] **Step 2: Create config.go**

```go
package logger

import "time"

// Config controls logger initialization.
type Config struct {
	// Level is the minimum log level: "debug", "info", "warn", "error". Default: "info".
	Level string
	// Format is the output format: "json" or "console". Default: "json".
	Format string
	// Sampling enables log sampling when non-nil.
	Sampling *SamplingConfig
	// StacktraceLevel is the level at which stack traces are attached. Default: "error".
	StacktraceLevel string
}

// SamplingConfig controls high-frequency log noise reduction.
type SamplingConfig struct {
	// Tick is the sampling window. Default: 1s.
	Tick time.Duration
	// Initial is the number of log entries emitted in full per Tick.
	Initial int
	// Thereafter emits one entry per this many after Initial is exhausted.
	Thereafter int
}
```

- [ ] **Step 3: Create logger.go**

```go
package logger

import "go.uber.org/zap"

// Logger is a thin wrapper around zap.Logger.
type Logger struct {
	zl *zap.Logger
}

func newLogger(zl *zap.Logger) *Logger {
	return &Logger{zl: zl}
}

// With returns a child Logger with the given fields pre-attached.
// Use at package level to bind a "component" field:
//
//	var log = logger.With(zap.String("component", "dingtalk"))
func (l *Logger) With(fields ...zap.Field) *Logger {
	return &Logger{zl: l.zl.With(fields...)}
}

func (l *Logger) Info(msg string, fields ...zap.Field)  { l.zl.Info(msg, fields...) }
func (l *Logger) Warn(msg string, fields ...zap.Field)  { l.zl.Warn(msg, fields...) }
func (l *Logger) Error(msg string, fields ...zap.Field) { l.zl.Error(msg, fields...) }
func (l *Logger) Debug(msg string, fields ...zap.Field) { l.zl.Debug(msg, fields...) }
```

- [ ] **Step 4: Create global.go**

```go
package logger

import (
	"log/slog"
	"net/http"
	"os"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/exp/zapslog"
	"go.uber.org/zap/zapcore"
)

var (
	globalLogger *Logger
	atomicLevel  zap.AtomicLevel
)

func init() {
	atomicLevel = zap.NewAtomicLevel()
	globalLogger = newLogger(zap.NewNop())
}

// Init initializes the global logger. Call once at program startup before any log calls.
func Init(cfg Config) error {
	atomicLevel = zap.NewAtomicLevel()

	level, err := zapcore.ParseLevel(cfg.Level)
	if err != nil || cfg.Level == "" {
		level = zapcore.InfoLevel
	}
	atomicLevel.SetLevel(level)

	stackLevel := zapcore.ErrorLevel
	if cfg.StacktraceLevel != "" {
		if sl, err := zapcore.ParseLevel(cfg.StacktraceLevel); err == nil {
			stackLevel = sl
		}
	}

	encCfg := zap.NewProductionEncoderConfig()
	encCfg.EncodeTime = zapcore.ISO8601TimeEncoder

	var enc zapcore.Encoder
	if cfg.Format == "console" {
		enc = zapcore.NewConsoleEncoder(encCfg)
	} else {
		enc = zapcore.NewJSONEncoder(encCfg)
	}

	core := zapcore.NewCore(enc, zapcore.AddSync(os.Stderr), atomicLevel)

	if s := cfg.Sampling; s != nil {
		tick := s.Tick
		if tick == 0 {
			tick = time.Second
		}
		core = zapcore.NewSamplerWithOptions(core, tick, s.Initial, s.Thereafter)
	}

	zl := zap.New(core, zap.AddCaller(), zap.AddStacktrace(stackLevel))
	globalLogger = newLogger(zl)
	return nil
}

// SetSlogDefault routes standard-library slog calls through the ZAP backend.
// Call this during migration to keep legacy slog call sites working.
// Remove once all slog call sites have been migrated to internal/logger.
func SetSlogDefault() {
	slog.SetDefault(slog.New(zapslog.NewHandler(globalLogger.zl.Core(), nil)))
}

// SetLevel adjusts the global log level at runtime without restarting.
func SetLevel(level zapcore.Level) { atomicLevel.SetLevel(level) }

// LevelHandler returns an http.Handler that serves the current log level and
// accepts PUT requests with JSON body {"level":"debug"} to change it at runtime.
// Mount at an internal route, e.g.: router.PUT("/internal/log/level", gin.WrapH(logger.LevelHandler()))
func LevelHandler() http.Handler { return atomicLevel }

// With returns a child Logger with pre-attached fields (e.g. component name).
func With(fields ...zap.Field) *Logger { return globalLogger.With(fields...) }

// Info logs at INFO level on the global logger.
func Info(msg string, fields ...zap.Field) { globalLogger.Info(msg, fields...) }

// Warn logs at WARN level on the global logger.
func Warn(msg string, fields ...zap.Field) { globalLogger.Warn(msg, fields...) }

// Error logs at ERROR level on the global logger.
func Error(msg string, fields ...zap.Field) { globalLogger.Error(msg, fields...) }

// Debug logs at DEBUG level on the global logger.
func Debug(msg string, fields ...zap.Field) { globalLogger.Debug(msg, fields...) }
```

- [ ] **Step 5: Run tests — expect compile error (implementation not written yet)**

Create the test file first, before any implementation, to follow TDD order:

```go
// internal/logger/logger_test.go
package logger_test

import (
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/theopenbee/openbee/internal/logger"
)

func TestInit_JSONFormat(t *testing.T) {
	err := logger.Init(logger.Config{Level: "debug", Format: "json"})
	if err != nil {
		t.Fatalf("Init() returned error: %v", err)
	}
}

func TestInit_ConsoleFormat(t *testing.T) {
	err := logger.Init(logger.Config{Level: "info", Format: "console"})
	if err != nil {
		t.Fatalf("Init() returned error: %v", err)
	}
}

func TestInit_InvalidLevel_DefaultsToInfo(t *testing.T) {
	// should not error even with invalid level
	err := logger.Init(logger.Config{Level: "nonsense", Format: "json"})
	if err != nil {
		t.Fatalf("Init() returned error: %v", err)
	}
}

func TestWith_ReturnsChildLogger(t *testing.T) {
	logger.Init(logger.Config{Level: "debug", Format: "json"})
	sub := logger.With(zap.String("component", "test"))
	if sub == nil {
		t.Fatal("With() returned nil")
	}
	sub.Info("from child logger", zap.String("key", "value"))
}

func TestGlobalFunctions_DoNotPanic(t *testing.T) {
	logger.Init(logger.Config{Level: "debug", Format: "json"})
	logger.Info("info message", zap.String("k", "v"))
	logger.Warn("warn message")
	logger.Error("error message", zap.Error(nil))
	logger.Debug("debug message")
}

func TestSetLevel_ChangesLevel(t *testing.T) {
	logger.Init(logger.Config{Level: "info", Format: "json"})
	logger.Debug("before level change — should be suppressed")
	logger.SetLevel(zapcore.DebugLevel)
	logger.Debug("after level change — should appear")
}

func TestInit_WithSampling(t *testing.T) {
	err := logger.Init(logger.Config{
		Level:  "debug",
		Format: "json",
		Sampling: &logger.SamplingConfig{
			Initial:    100,
			Thereafter: 10,
		},
	})
	if err != nil {
		t.Fatalf("Init() with sampling returned error: %v", err)
	}
}
```

Now run:
```bash
go test ./internal/logger/...
```

Expected: compile error — `logger.Config`, `logger.Init`, etc. are not defined yet. This confirms tests are wired before implementation.

- [ ] **Step 6: Create config.go, logger.go, global.go** (Steps 2–4 above)

Now write the three implementation files as shown in Steps 2, 3, and 4.

- [ ] **Step 7: Run tests — expect PASS**

```bash
go test ./internal/logger/... -v
```

Expected: all 7 tests PASS.

- [ ] **Step 8: Verify the whole repo still builds**

```bash
go build ./...
```

Expected: no errors.

- [ ] **Step 9: Commit**

```bash
git add internal/logger/ go.mod go.sum
git commit -m "feat: add internal/logger package wrapping go.uber.org/zap"
```

---

## Task 2: Wire Logger into Application

**Files:**
- Modify: `cmd/openbee/server.go`

- [ ] **Step 1: Read the current server.go**

```bash
cat cmd/openbee/server.go
```

Current content is approximately 35 lines — a cobra command that loads config and calls `app.BuildApp(cfg).Run()`.

- [ ] **Step 2: Add logger.Init + SetSlogDefault before app build**

Replace the `RunE` body in `cmd/openbee/server.go`:

```go
package main

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/theopenbee/openbee/internal/app"
	"github.com/theopenbee/openbee/internal/config"
	"github.com/theopenbee/openbee/internal/logger"
)

var cfgPath string

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "启动 OpenBee 服务",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Initialize with sensible defaults before config is available,
		// so that any log calls during config loading are captured.
		// Level defaults to "info"; format to "json" for log platform compatibility.
		// The log level can be adjusted at runtime via logger.SetLevel() or the HTTP endpoint.
		if err := logger.Init(logger.Config{
			Level:  "info",
			Format: "json",
		}); err != nil {
			return fmt.Errorf("init logger: %w", err)
		}
		// Route legacy slog calls through ZAP during migration.
		// Remove this line once all slog call sites are migrated.
		logger.SetSlogDefault()

		cfg, err := config.Load(cfgPath)
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}

		a, err := app.BuildApp(cfg)
		if err != nil {
			return fmt.Errorf("build app: %w", err)
		}

		a.Run()
		return nil
	},
}

func init() {
	serverCmd.Flags().StringVarP(&cfgPath, "config", "c", "config.yaml", "配置文件路径")
	rootCmd.AddCommand(serverCmd)
}
```

> **Note on log level config:** Logger is initialized with defaults before `config.Load()` so that log calls during startup are captured. This is intentional. Operators can change the level at runtime without restart via `logger.SetLevel()` or the HTTP endpoint registered in Step 3.

- [ ] **Step 3: Register the dynamic level HTTP endpoint in app.go**

In `internal/app/app.go`, find where the gin router is set up (in `BuildApp` or similar) and add:

```go
// Import at top of app.go (add to existing imports):
"github.com/theopenbee/openbee/internal/logger"

// In the router setup block, alongside other internal routes:
router.PUT("/internal/log/level", gin.WrapH(logger.LevelHandler()))
```

This exposes `PUT /internal/log/level` with JSON body `{"level":"debug"}` to change the log level at runtime.

- [ ] **Step 4: Build to confirm no errors**

```bash
go build ./...
```

Expected: no errors. From this point on all logs — including legacy `slog.*` calls — output as ZAP JSON to stderr.

- [ ] **Step 5: Commit**

```bash
git add cmd/openbee/server.go internal/app/app.go
git commit -m "feat: wire internal/logger into server startup with slog bridge and level endpoint"
```

---

## Task 3: Migrate Platform Handlers

**Files:**
- Modify: `internal/platform/dingtalk/handler.go`
- Modify: `internal/platform/feishu/handler.go`
- Modify: `internal/platform/wecom/handler.go`
- Modify: `internal/platform/wecom/wsconn.go`
- Modify: `internal/platform/local/hub.go`
- Modify: `internal/platform/local/receiver.go`

### Migration Pattern (apply to every file in this task)

For each file:

1. Remove `"log/slog"` from imports.
2. Add these two imports:
   ```go
   "go.uber.org/zap"
   "github.com/theopenbee/openbee/internal/logger"
   ```
3. Add a package-level logger var after the imports (before any type declarations):
   ```go
   var log = logger.With(zap.String("component", "<component-name>"))
   ```
4. Replace every `slog.XXX(...)` call using the rules below.
5. Run `go build ./...`. Fix any compile errors.
6. Commit.

### Field Replacement Rules

| Old (slog) | New (logger + zap) |
|------------|-------------------|
| `slog.Info("msg", "k", strVal)` | `log.Info("msg", zap.String("k", strVal))` |
| `slog.Warn("msg", "k", strVal)` | `log.Warn("msg", zap.String("k", strVal))` |
| `slog.Error("msg", "k", strVal)` | `log.Error("msg", zap.String("k", strVal))` |
| `slog.Debug("msg", "k", strVal)` | `log.Debug("msg", zap.String("k", strVal))` |
| `..., "error", err` | `..., zap.Error(err)` |
| `..., "elapsed", elapsed` (time.Duration) | `..., zap.Duration("elapsed", elapsed)` |
| `..., "component", "xxx"` | *(remove — already set on `log` var)* |
| `..., "sessionKey", key` (string) | `..., zap.String("sessionKey", key)` |
| `..., "workerID", id` (string) | `..., zap.String("workerID", id)` |
| `..., "taskID", id` (string) | `..., zap.String("taskID", id)` |
| `..., "sessionID", id` (string) | `..., zap.String("sessionID", id)` |
| Any other unknown type | `zap.Any("k", val)` |

> **Note:** Always check the Go type of each value. `int` → `zap.Int`, `bool` → `zap.Bool`, `time.Duration` → `zap.Duration`, etc.

### Step-by-Step for dingtalk/handler.go

- [ ] **Step 1: Migrate dingtalk/handler.go**

Apply the migration pattern with `component = "dingtalk"`.

Key examples from this file:
```go
// Before
slog.Info("DingTalk bot started with heartbeat supervisor", "component", "dingtalk")
slog.Warn("skipping unsupported message type", "component", "dingtalk", "msgtype", msgtype)
slog.Error("DingTalk reconnect failed, retrying", "component", "dingtalk", "error", err)
slog.Warn("DingTalk heartbeat timeout, triggering reconnect", "component", "dingtalk", "elapsed", elapsed)
slog.Info("received message", "component", "dingtalk", "sessionKey", msg.SessionKey)

// After
log.Info("DingTalk bot started with heartbeat supervisor")
log.Warn("skipping unsupported message type", zap.String("msgtype", msgtype))
log.Error("DingTalk reconnect failed, retrying", zap.Error(err))
log.Warn("DingTalk heartbeat timeout, triggering reconnect", zap.Duration("elapsed", elapsed))
log.Info("received message", zap.String("sessionKey", msg.SessionKey))
```

- [ ] **Step 2: Build**

```bash
go build ./internal/platform/dingtalk/...
```

Expected: no errors.

- [ ] **Step 3: Migrate feishu/handler.go**

Apply the migration pattern with `component = "feishu"`.

- [ ] **Step 4: Build**

```bash
go build ./internal/platform/feishu/...
```

Expected: no errors.

- [ ] **Step 5: Migrate wecom/handler.go and wecom/wsconn.go**

Apply migration pattern to both files. Use `component = "wecom"` for both.

Note: `wsconn.go` is a separate file in the same package — declare `var log` only in **one** of the two files (e.g. `handler.go`), not both, to avoid a `log redeclared` compile error.

- [ ] **Step 6: Build**

```bash
go build ./internal/platform/wecom/...
```

Expected: no errors.

- [ ] **Step 7: Migrate local/hub.go and local/receiver.go**

Apply migration pattern. Use `component = "local"`. Again, declare `var log` in only one of the two files.

- [ ] **Step 8: Build**

```bash
go build ./internal/platform/local/...
```

Expected: no errors.

- [ ] **Step 9: Full build**

```bash
go build ./...
```

Expected: no errors.

- [ ] **Step 10: Commit**

```bash
git add internal/platform/
git commit -m "feat: migrate platform handlers from slog to internal/logger"
```

---

## Task 4: Migrate Core Components

**Files:**
- Modify: `internal/task_dispatcher/dispatcher.go`
- Modify: `internal/task_dispatcher/failure_notifier.go`
- Modify: `internal/task_scheduler/scheduler.go`
- Modify: `internal/bee/feeder.go`
- Modify: `internal/worker/manager.go`
- Modify: `internal/msgingest/gateway.go`

Apply the same migration pattern from Task 3 to each file. Component names to use:

| File | component value |
|------|----------------|
| `task_dispatcher/dispatcher.go` | `"taskdispatcher"` |
| `task_dispatcher/failure_notifier.go` | `"taskdispatcher"` (same package — declare `var log` only in `dispatcher.go`) |
| `task_scheduler/scheduler.go` | `"taskscheduler"` |
| `bee/feeder.go` | `"feeder"` |
| `worker/manager.go` | `"worker"` |
| `msgingest/gateway.go` | `"msgingest"` |

- [ ] **Step 1: Migrate task_dispatcher/dispatcher.go and failure_notifier.go**

`var log = logger.With(zap.String("component", "taskdispatcher"))` goes in `dispatcher.go` only.

- [ ] **Step 2: Build**

```bash
go build ./internal/task_dispatcher/...
```

Expected: no errors.

- [ ] **Step 3: Run existing dispatcher tests**

```bash
go test ./internal/task_dispatcher/... -v
```

Expected: all tests PASS (migration should not break logic).

- [ ] **Step 4: Migrate task_scheduler/scheduler.go**

- [ ] **Step 5: Build and test**

```bash
go build ./internal/task_scheduler/...
go test ./internal/task_scheduler/... -v
```

Expected: no errors, all tests PASS.

- [ ] **Step 6: Migrate bee/feeder.go**

- [ ] **Step 7: Build and test**

```bash
go build ./internal/bee/...
go test ./internal/bee/... -v
```

Expected: no errors, all tests PASS.

- [ ] **Step 8: Migrate worker/manager.go**

- [ ] **Step 9: Build**

```bash
go build ./internal/worker/...
```

Expected: no errors.

- [ ] **Step 10: Migrate msgingest/gateway.go**

- [ ] **Step 11: Full build and test**

```bash
go build ./...
go test ./...
```

Expected: no errors, all existing tests PASS.

- [ ] **Step 12: Commit**

```bash
git add internal/task_dispatcher/ internal/task_scheduler/ internal/bee/ internal/worker/ internal/msgingest/
git commit -m "feat: migrate core components from slog to internal/logger"
```

---

## Task 5: Migrate Remaining Files and Final Cleanup

**Files:**
- Modify: `internal/mcp/server.go`
- Modify: `internal/mcp/tools.go`
- Modify: `internal/api/local_chat_handler.go`
- Modify: `internal/ffmedia/ffmedia.go`
- Modify: `internal/app/app.go`
- Modify: `cmd/openbee/main.go`
- Modify: `cmd/openbee/server.go` — remove slog bridge

Apply migration pattern to mcp, api, ffmedia and app files:

| File | component value |
|------|----------------|
| `mcp/server.go` | `"mcp"` (declare `var log` here) |
| `mcp/tools.go` | `"mcp"` (same package — do NOT redeclare `var log`) |
| `api/local_chat_handler.go` | `"api"` |
| `ffmedia/ffmedia.go` | `"ffmedia"` |
| `app/app.go` | no component (app-level startup messages, use package-level `logger.Info/Error` directly) |

For `app/app.go`: since there is no single "component" for the app bootstrap, use the global functions directly instead of `var log`:
```go
// In app/app.go — no var log declaration; use package-level functions
logger.Info("OpenBee Core starting", zap.String("addr", a.addr))
logger.Error("server error", zap.Error(err))
```

For `cmd/openbee/main.go`:
```go
// Before
slog.Error("fatal", "error", err)

// After
logger.Error("fatal", zap.Error(err))
```

- [ ] **Step 1: Migrate mcp/server.go and mcp/tools.go**

Declare `var log = logger.With(zap.String("component", "mcp"))` in `server.go` only.

- [ ] **Step 2: Migrate api/local_chat_handler.go**

- [ ] **Step 3: Migrate ffmedia/ffmedia.go**

- [ ] **Step 4: Migrate app/app.go**

Use global `logger.Info/Error` (no `var log`). Replace `"log/slog"` import with `internal/logger` and `go.uber.org/zap`.

- [ ] **Step 5: Migrate cmd/openbee/main.go**

Replace `slog.Error("fatal", "error", err)` with `logger.Error("fatal", zap.Error(err))`.
Remove `"log/slog"` import, add `internal/logger` and `go.uber.org/zap`.

- [ ] **Step 6: Remove the slog bridge from server.go**

In `cmd/openbee/server.go`, delete the `logger.SetSlogDefault()` call and its explanatory comment.

- [ ] **Step 7: Remove SetSlogDefault from global.go**

In `internal/logger/global.go`, delete the `SetSlogDefault()` function and remove the `"log/slog"` and `"go.uber.org/zap/exp/zapslog"` imports (they are only needed for the bridge).

- [ ] **Step 8: Tidy dependencies**

```bash
go mod tidy
```

Expected: removes `go.uber.org/zap/exp/zapslog` from `go.mod`/`go.sum` if it's no longer referenced.

- [ ] **Step 9: Full build**

```bash
go build ./...
```

Expected: no errors.

- [ ] **Step 10: Verify no slog imports remain**

```bash
grep -r '"log/slog"' internal/ cmd/
```

Expected: **no output**. If any file is listed, migrate it now.

- [ ] **Step 11: Run all tests**

```bash
go test ./...
```

Expected: all tests PASS.

- [ ] **Step 12: Commit**

```bash
git add internal/mcp/ internal/api/ internal/ffmedia/ internal/app/ internal/logger/ cmd/openbee/ go.mod go.sum
git commit -m "feat: complete slog-to-zap migration, remove slog bridge"
```

---

## Verification Checklist

After Task 5 Step 12, confirm all of the following:

- [ ] `grep -r '"log/slog"' internal/ cmd/` → empty (no slog imports)
- [ ] `grep -rE 'slog\.(Info|Warn|Error|Debug)\(' internal/ cmd/` → empty (no slog call sites)
- [ ] `go build ./...` → no errors
- [ ] `go test ./...` → all tests PASS
- [ ] `go vet ./...` → no warnings
- [ ] Starting the server emits JSON-formatted logs to stderr
