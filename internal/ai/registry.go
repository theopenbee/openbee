// internal/ai/registry.go
package ai

import "fmt"

// ErrUnknownEngine is returned when New is called with an unregistered engine name.
var ErrUnknownEngine = fmt.Errorf("unknown engine")

// EngineConfig holds the configuration passed to a Factory when constructing an engine.
type EngineConfig struct {
	// OpenbeeURL is the openbee server base URL injected for MCP connectivity.
	OpenbeeURL string
	// Raw holds engine-specific configuration (parsed from config.yaml).
	Raw map[string]any
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
