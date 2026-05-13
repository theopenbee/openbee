package ai

import (
	"context"
	"errors"
	"fmt"

	"github.com/theopenbee/openbee/internal/domain/enginecfg"
)

// =========================================================
// Section 2: Core contracts (from contracts.go)
// =========================================================

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

// RunResult is the handle returned from EngineAdapter.Run.
type RunResult struct {
	Process       Process
	Output        <-chan Output
	ExtractResult func() string
}

// EngineAdapter is the complete plugin contract for an AI engine.
// Implementations must be safe for concurrent use.
type EngineAdapter interface {
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

// =========================================================
// Section 3: Registry (from registry.go)
// =========================================================

// ErrUnknownEngine is returned when New is called with an unregistered engine name.
var ErrUnknownEngine = fmt.Errorf("unknown engine")

// legacyFactory creates an EngineAdapter from the supplied config.
type legacyFactory func(cfg EngineConfig) (EngineAdapter, error)

// Registry maps engine names to their factories.
type Registry struct {
	factories map[string]legacyFactory
}

// NewRegistry creates an empty Registry.
func NewRegistry() *Registry {
	return &Registry{factories: make(map[string]legacyFactory)}
}

// Register adds a factory under name. Panics if name is already registered.
func (r *Registry) Register(name string, f legacyFactory) {
	if _, exists := r.factories[name]; exists {
		panic(fmt.Sprintf("ai: engine %q already registered", name))
	}
	r.factories[name] = f
}

// New constructs the engine registered under name.
func (r *Registry) New(name string, cfg EngineConfig) (EngineAdapter, error) {
	f, ok := r.factories[name]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnknownEngine, name)
	}
	return f(cfg)
}

// DefaultRegistry is the process-wide registry populated by engine init() functions.
var DefaultRegistry = NewRegistry()

// Register adds a factory to the DefaultRegistry.
func Register(name string, f legacyFactory) { DefaultRegistry.Register(name, f) }

// New constructs an engine from the DefaultRegistry.
func New(name string, cfg EngineConfig) (EngineAdapter, error) {
	return DefaultRegistry.New(name, cfg)
}

// =========================================================
// Section 4 (transitional): legacy DynamicAdapter
// =========================================================
//
// Kept as a compatibility shim until app.go migrates to Factory.Dynamic
// (Task 4 of the Factory-facade refactor). Removed in Task 5 alongside
// the rest of the legacy registry.

// DynamicAdapter wraps a map of engines and routes each Run call to
// whichever engine cfg.Get() returns at call time.
type DynamicAdapter struct {
	engines map[string]EngineAdapter
	cfg     *enginecfg.Store
}

// NewDynamicAdapter constructs a DynamicAdapter routing through cfg.
func NewDynamicAdapter(engines map[string]EngineAdapter, cfg *enginecfg.Store) *DynamicAdapter {
	return &DynamicAdapter{engines: engines, cfg: cfg}
}

func (d *DynamicAdapter) Run(ctx context.Context, workDir, prompt string, opts RunOptions, logPath string) (RunResult, error) {
	name := d.cfg.Get()
	e, ok := d.engines[name]
	if !ok {
		return RunResult{}, fmt.Errorf("engine %q not available", name)
	}
	return e.Run(ctx, workDir, prompt, opts, logPath)
}

func (d *DynamicAdapter) CollectTokenUsage(_ context.Context, _ string) ([]TokenUsage, error) {
	return nil, ErrSessionDataNotFound
}
