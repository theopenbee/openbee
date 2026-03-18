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
