package ai

import (
	"context"
	"errors"
	"fmt"
)

// Role identifies the openbee agent role.
type Role string

const (
	RoleBee    Role = "bee"
	RoleWorker Role = "worker"
)

// RunOptions controls session behaviour for an engine invocation.
type RunOptions struct {
	SessionID string
	Resume    bool
	Role      Role
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
	// Run executes a task and returns a RunResult carrying the process handle,
	// event channel, and an engine-bound result extractor. The event channel
	// is closed after the process exits.
	Run(ctx context.Context, workDir, prompt string,
		opts RunOptions, logPath string) (RunResult, error)

	CollectTokenUsage(ctx context.Context, sessionIDs []string) (map[string][]TokenUsage, error)
}

type EngineInfo struct {
	ID   string            `json:"id"`
	Name map[string]string `json:"name"`
}

type EngineSource interface {
	Engine(id string) (EngineAdapter, bool)
	Engines() []EngineInfo
}

var ErrNoEngine = errors.New("no engine is installed or enabled")

type EngineUnavailableError struct {
	ID string
}

func (e *EngineUnavailableError) Error() string {
	return fmt.Sprintf("engine %q is not installed or enabled", e.ID)
}

func ErrEngineUnavailable(id string) error {
	return &EngineUnavailableError{ID: id}
}

func IsEngineMissing(err error) bool {
	var unavailable *EngineUnavailableError
	return errors.Is(err, ErrNoEngine) || errors.As(err, &unavailable)
}

func EngineIDs(src EngineSource) []string {
	engines := src.Engines()
	ids := make([]string, 0, len(engines))
	for _, e := range engines {
		ids = append(ids, e.ID)
	}
	return ids
}

// TokenUsage holds per-model token consumption for a single session turn.
type TokenUsage struct {
	Model               string
	InputTokens         int64
	OutputTokens        int64
	CacheCreationTokens int64
	CacheReadTokens     int64
}
