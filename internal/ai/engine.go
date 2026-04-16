package ai

import "context"

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

// AllEngines is the canonical list of supported engine names.
var AllEngines = []string{EngineClaude, EngineCodex, EnginePi}

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
	// Prepare is an engine-specific initialisation hook called before each Run.
	// It must be idempotent. Claude uses it to clean up legacy config files;
	// other engines return nil.
	Prepare(workDir string, opts PrepareOptions) error

	// Run executes a task and returns a process handle and an event channel.
	// The channel is closed after the process exits.
	Run(ctx context.Context, workDir, prompt string,
		opts RunOptions, logPath string) (Process, <-chan Output, error)

	// ExtractResult parses the engine-specific log file and returns the final
	// result string, or "" if none found.
	ExtractResult(logPath string) string
}
