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
