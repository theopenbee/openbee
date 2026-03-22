package logger

import "go.uber.org/zap"

// Logger is a thin wrapper around zap.Logger.
// A Logger created via With() stores fields for deferred resolution so that
// package-level vars like `var log = logger.With(...)` pick up the real
// logger even when evaluated before Init().
type Logger struct {
	zl     *zap.Logger // non-nil for the global logger itself
	fields []zap.Field // stored fields for deferred resolution
}

func newLogger(zl *zap.Logger) *Logger {
	return &Logger{zl: zl}
}

// resolve returns the effective zap.Logger.
// For the global logger (zl is set) it returns zl directly.
// For deferred loggers (created via With) it derives from the current global.
func (l *Logger) resolve() *zap.Logger {
	if l.zl != nil {
		return l.zl
	}
	return globalLogger.zl.With(l.fields...)
}

// With returns a child Logger with the given fields pre-attached.
// The returned Logger is deferred: it resolves against the current
// globalLogger at log time, so package-level vars are safe to use
// before Init() is called.
//
//	var log = logger.With(zap.String("component", "dingtalk"))
func (l *Logger) With(fields ...zap.Field) *Logger {
	if l.zl != nil {
		// Called on a concrete logger (e.g. globalLogger): return deferred.
		return &Logger{fields: fields}
	}
	// Chained With on a deferred logger: merge fields.
	merged := make([]zap.Field, 0, len(l.fields)+len(fields))
	merged = append(merged, l.fields...)
	merged = append(merged, fields...)
	return &Logger{fields: merged}
}

func (l *Logger) Info(msg string, fields ...zap.Field)  { l.resolve().Info(msg, fields...) }
func (l *Logger) Warn(msg string, fields ...zap.Field)  { l.resolve().Warn(msg, fields...) }
func (l *Logger) Error(msg string, fields ...zap.Field) { l.resolve().Error(msg, fields...) }
func (l *Logger) Debug(msg string, fields ...zap.Field) { l.resolve().Debug(msg, fields...) }
