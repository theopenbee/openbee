package ai

import "context"

// Role identifies the openbee agent role for workspace setup.
type Role string

const (
	RoleBee    Role = "bee"
	RoleWorker Role = "worker"
)

// WorkspaceOptions carries per-agent metadata used during workspace initialisation.
type WorkspaceOptions struct {
	Name        string
	Description string
	Memory      string
}

// RunOptions controls session behaviour for an engine invocation.
type RunOptions struct {
	SessionID string
	Resume    bool
	APIKey    string
}

// OutputType classifies a lifecycle event from a running process.
type OutputType string

const (
	OutputDone      OutputType = "done"
	OutputError     OutputType = "error"
	OutputSessionID OutputType = "session_id"
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

// EngineAdapter is the complete plugin contract for an AI engine.
// Implementations must be safe for concurrent use.
type EngineAdapter interface {
	// SetupWorkspace writes engine-specific config files to workDir (system
	// rules, persona, etc.). It must be idempotent.
	SetupWorkspace(workDir string, role Role, opts WorkspaceOptions) error

	// Run executes a task and returns a process handle and an event channel.
	// The channel is closed after the process exits.
	Run(ctx context.Context, workDir, prompt string,
		opts RunOptions, logPath string) (Process, <-chan Output, error)
}
