package ai

import (
	"context"
	"errors"
)

const (
	// SystemRulesFile is the legacy rules file that Claude's Prepare hook cleans up.
	SystemRulesFile = ".openbee.md"
	// ImportLine is the legacy reference line that Claude's Prepare hook removes from CLAUDE.md.
	ImportLine = "@" + SystemRulesFile
)

// Role identifies the openbee agent role.
type Role string

const (
	RoleBee    Role = "bee"
	RoleWorker Role = "worker"
)

// PrepareOptions carries parameters for the engine-specific Prepare hook.
// Add future fields here without changing the Prepare method signature.
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
)

var allEngines = []string{EngineClaude, EngineCodex, EnginePi}

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

// RunResult is the handle returned from EngineAdapter.Run. ExtractResult is
// bound to the engine that handled this Run, so it remains correct even if
// the active engine later changes.
type RunResult struct {
	Process       Process
	Output        <-chan Output
	ExtractResult func(logPath string) string
}

// NewRunResult wraps the (process, output, error) tuple returned by an engine
// invoker into a RunResult, attaching the engine's result extractor on success.
func NewRunResult(proc Process, out <-chan Output, err error, extract func(logPath string) string) (RunResult, error) {
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

// DrainUsageMap converts a model→*TokenUsage aggregation map into a flat slice.
func DrainUsageMap(agg map[string]*TokenUsage) []TokenUsage {
	out := make([]TokenUsage, 0, len(agg))
	for _, u := range agg {
		out = append(out, *u)
	}
	return out
}
