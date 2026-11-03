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
