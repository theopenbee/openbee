package ai

import (
	"context"
	"errors"
)

// Role identifies the openbee agent role.
type Role string

const (
	RoleBee    Role = "bee"
	RoleWorker Role = "worker"
)

// PrepareOptions carries parameters for the engine-specific Prepare hook.
type PrepareOptions struct {
	Role Role
}

// RunOptions controls session behaviour for an engine invocation.
type RunOptions struct {
	SessionID string
	Resume    bool
	APIKey    string
	ExtraEnv  []string // additional KEY=VALUE env vars to inject
	ExtraArgs []string // additional CLI args to pass to the engine
}

// OutputType classifies a lifecycle event from a running process.
type OutputType string

const (
	OutputDone  OutputType = "done"
	OutputError OutputType = "error"
)

// Engine name constants used for registration and configuration.
const (
	EngineClaude = "claude"
	EngineCodex  = "codex"
	EnginePi     = "pi"
	EngineKimi   = "kimi"
)

var allEngines = []string{EngineClaude, EngineCodex, EnginePi, EngineKimi}

// AllEngines returns a snapshot of the canonical engine name list in registration order.
func AllEngines() []string {
	cp := make([]string, len(allEngines))
	copy(cp, allEngines)
	return cp
}

// Output is a single lifecycle event.
type Output struct {
	Type    OutputType `json:"type"`
	Content string     `json:"content"`
}

// Process is the handle for a running engine process.
type Process interface {
	PID() int
	Stop() error
}

// RunResult is the handle returned from EngineAdapter.Run.
type RunResult struct {
	Process       Process
	Output        <-chan Output
	ExtractResult func() string
}

// NewRunResult builds a RunResult, propagating err unchanged.
func NewRunResult(proc Process, out <-chan Output, err error, extract func() string) (RunResult, error) {
	if err != nil {
		return RunResult{}, err
	}
	return RunResult{Process: proc, Output: out, ExtractResult: extract}, nil
}

// EngineAdapter is the complete plugin contract for an AI engine.
// Implementations must be safe for concurrent use.
type EngineAdapter interface {
	// Prepare is an engine-specific initialisation hook called before each Run.
	// It must be idempotent. Claude uses it to clean up legacy config files;
	// other engines return nil.
	Prepare(workDir string, opts PrepareOptions) error

	// Run executes a task and returns a RunResult carrying the process handle,
	// event channel, and an engine-bound result extractor. The event channel
	// is closed after the process exits.
	Run(ctx context.Context, workDir, prompt string,
		opts RunOptions, logPath string) (RunResult, error)

	// CollectTokenUsage reads per-turn token usage for the given session from
	// engine-specific storage. Returns ErrSessionDataNotFound when no data is
	// available for the session.
	CollectTokenUsage(ctx context.Context, sessionID string) ([]TokenUsage, error)
}

// TokenUsage holds per-model token consumption for a single session turn.
type TokenUsage struct {
	Model               string
	InputTokens         int64
	OutputTokens        int64
	CacheCreationTokens int64
	CacheReadTokens     int64
}

// ErrSessionDataNotFound is returned by CollectTokenUsage when no session data exists.
var ErrSessionDataNotFound = errors.New("ai: session data not found")
