package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode"

	"github.com/theopenbee/openbee/internal/domain/enginecfg"
)

// =========================================================
// Section 1: Engine identifiers (from contracts.go)
// =========================================================

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

// =========================================================
// Section 2: Core contracts (from contracts.go)
// =========================================================

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

// =========================================================
// Section 3: Registry (from registry.go)
// =========================================================

// ErrUnknownEngine is returned when New is called with an unregistered engine name.
var ErrUnknownEngine = fmt.Errorf("unknown engine")

// EngineConfig holds the configuration passed to a Factory when constructing an engine.
type EngineConfig struct {
	// Raw holds engine-specific configuration (parsed from config.yaml).
	Raw map[string]any
}

// PathOrDefault returns Raw["path"] when it's a non-empty string, else def.
func (c EngineConfig) PathOrDefault(def string) string {
	if path, _ := c.Raw["path"].(string); path != "" {
		return path
	}
	return def
}

// ExtraEnv returns Raw["env"] as a map[string]string, or nil if absent / mistyped.
func (c EngineConfig) ExtraEnv() map[string]string {
	env, _ := c.Raw["env"].(map[string]string)
	return env
}

// Factory creates an EngineAdapter from the supplied config.
type Factory func(cfg EngineConfig) (EngineAdapter, error)

// Registry maps engine names to their factories.
type Registry struct {
	factories map[string]Factory
}

// NewRegistry creates an empty Registry.
func NewRegistry() *Registry {
	return &Registry{factories: make(map[string]Factory)}
}

// Register adds a factory under name. Panics if name is already registered.
func (r *Registry) Register(name string, f Factory) {
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
func Register(name string, f Factory) { DefaultRegistry.Register(name, f) }

// New constructs an engine from the DefaultRegistry.
func New(name string, cfg EngineConfig) (EngineAdapter, error) {
	return DefaultRegistry.New(name, cfg)
}

// =========================================================
// Section 4: Dynamic routing (from dynamic.go)
// =========================================================

// DynamicAdapter wraps multiple EngineAdapters and routes each Run call to
// whichever engine cfg.Get() returns at call time. The RunResult's
// ExtractResult closes over the engine that was actually picked, so callers
// processing results asynchronously are immune to later /engine switches.
type DynamicAdapter struct {
	engines map[string]EngineAdapter
	cfg     *enginecfg.Store
}

// NewDynamicAdapter constructs a DynamicAdapter routing through cfg.
func NewDynamicAdapter(engines map[string]EngineAdapter, cfg *enginecfg.Store) *DynamicAdapter {
	return &DynamicAdapter{engines: engines, cfg: cfg}
}

// Prepare initialises every engine adapter for the given workDir.
func (d *DynamicAdapter) Prepare(workDir string, opts PrepareOptions) error {
	for name, e := range d.engines {
		if err := e.Prepare(workDir, opts); err != nil {
			return fmt.Errorf("prepare engine %q: %w", name, err)
		}
	}
	return nil
}

func (d *DynamicAdapter) Run(ctx context.Context, workDir, prompt string, opts RunOptions, logPath string) (RunResult, error) {
	name := d.cfg.Get()
	e, ok := d.engines[name]
	if !ok {
		return RunResult{}, fmt.Errorf("engine %q not available", name)
	}
	return e.Run(ctx, workDir, prompt, opts, logPath)
}

func (d *DynamicAdapter) CollectTokenUsage(ctx context.Context, sessionID string) ([]TokenUsage, error) {
	return nil, ErrSessionDataNotFound
}

// =========================================================
// Section 5: Helper utilities (from prompt.go + engine_args.go)
// =========================================================

// WorkerPersona returns the persona-only content injected into new worker session prompts.
func WorkerPersona(name, description, constraints string) string {
	s := "## Role\nYou are a Worker in an AI team.\n"
	if name != "" || description != "" {
		s += "\n## Identity\n"
	}
	if name != "" {
		s += fmt.Sprintf("Name: %s\n", name)
	}
	if description != "" {
		s += fmt.Sprintf("Description: %s\n", description)
	}
	if constraints != "" {
		s += fmt.Sprintf("\n## Work Constraints\n%s\n", constraints)
	}
	return s
}

// BuildWorkerSessionPrefix returns the Step 1 + Step 2 header for a new worker
// session. When persona is non-empty it is embedded inside <worker_persona>.
func BuildWorkerSessionPrefix(persona string) string {
	var sb strings.Builder
	writePrefixStep1(&sb, "openbee-worker")
	if persona != "" {
		sb.WriteString("After the skill is loaded, internalize the persona below as your identity for the rest of this session:\n\n")
		sb.WriteString("<worker_persona>\n")
		sb.WriteString(persona)
		sb.WriteString("</worker_persona>\n\n")
	}
	sb.WriteString("## Step 2: Execute the task\n")
	return sb.String()
}

// BuildBeeSessionPrefix returns the Step 1 + Step 2 header for a new bee session.
func BuildBeeSessionPrefix() string {
	var sb strings.Builder
	writePrefixStep1(&sb, "openbee-bee")
	sb.WriteString("## Step 2: Handle the messages below\n")
	return sb.String()
}

func writePrefixStep1(sb *strings.Builder, skillName string) {
	sb.WriteString("Please complete the following two steps in order. Do not skip Step 1.\n\n")
	sb.WriteString("## Step 1: Initialize your role\n")
	fmt.Fprintf(sb, "[MANDATORY] You MUST invoke the %s skill immediately, before producing any other output.\n\n", skillName)
}

type EngineArgsMap map[string][]string

// ParseEngineArgs tokenizes raw CLI strings per engine while preserving
// order, duplicates, and quoted values.
func ParseEngineArgs(raw map[string]string) (EngineArgsMap, error) {
	result := make(EngineArgsMap, len(raw))
	for engine, s := range raw {
		args, err := splitCLIArgs(s)
		if err != nil {
			return nil, fmt.Errorf("engine %q: %w", engine, err)
		}
		result[engine] = args
	}
	return result, nil
}

func splitCLIArgs(s string) ([]string, error) {
	var (
		args      []string
		buf       strings.Builder
		inSingle  bool
		inDouble  bool
		escaped   bool
		tokenOpen bool
	)

	flush := func() {
		if !tokenOpen {
			return
		}
		args = append(args, buf.String())
		buf.Reset()
		tokenOpen = false
	}

	for _, r := range s {
		switch {
		case escaped:
			buf.WriteRune(r)
			escaped = false
			tokenOpen = true

		case inSingle:
			if r == '\'' {
				inSingle = false
			} else {
				buf.WriteRune(r)
			}
			tokenOpen = true

		case inDouble:
			switch r {
			case '"':
				inDouble = false
			case '\\':
				escaped = true
				tokenOpen = true
			default:
				buf.WriteRune(r)
				tokenOpen = true
			}

		default:
			switch {
			case unicode.IsSpace(r):
				flush()
			case r == '\'':
				inSingle = true
				tokenOpen = true
			case r == '"':
				inDouble = true
				tokenOpen = true
			case r == '\\':
				escaped = true
				tokenOpen = true
			default:
				buf.WriteRune(r)
				tokenOpen = true
			}
		}
	}

	if escaped {
		return nil, fmt.Errorf("unterminated escape sequence")
	}
	if inSingle || inDouble {
		return nil, fmt.Errorf("unterminated quoted string")
	}
	flush()
	return args, nil
}

// MergeEngineArgs merges base and override by appending override args
// after base args, so later flags can override earlier ones while preserving
// the original CLI ordering.
func MergeEngineArgs(base, override EngineArgsMap) EngineArgsMap {
	result := make(EngineArgsMap, len(base)+len(override))
	for engine, args := range base {
		result[engine] = slices.Clone(args)
	}
	for engine, overrideArgs := range override {
		result[engine] = append(result[engine], overrideArgs...)
	}
	return result
}

// ParseEngineArgsJSON returns nil for empty/unset values.
func ParseEngineArgsJSON(value string) EngineArgsMap {
	if value == "" || value == "{}" {
		return nil
	}
	var raw map[string]string
	if json.Unmarshal([]byte(value), &raw) != nil {
		return nil
	}
	parsed, _ := ParseEngineArgs(raw)
	return parsed
}
