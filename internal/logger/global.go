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
