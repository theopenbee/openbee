package ai

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	cliargs "github.com/theopenbee/openbee/internal/ai/cliargs"
)

// =========================================================
// Section 1: Engine identifiers
// =========================================================

const (
	EngineClaude = "claude"
	EngineCodex  = "codex"
	EnginePi     = "pi"
	EngineKimi   = "kimi"
)

var allEngineNames = []string{EngineClaude, EngineCodex, EnginePi, EngineKimi}

// AllEngines returns the canonical engine name list in declaration
// order. It is independent of which engine packages have been
// imported, so callers that build engine-name maps (e.g., config UIs,
// tokenstat fallback chains) get a stable list without needing blank
// imports of every engine.
//
// Use Factory.Names() when you need the names actually registered in
// a particular Factory instance.
func AllEngines() []string {
	cp := make([]string, len(allEngineNames))
	copy(cp, allEngineNames)
	return cp
}

// =========================================================
// Section 2: Engine registration (called from engine init())
// =========================================================

// EngineConfig holds the configuration passed to a constructor when
// building an engine instance.
type EngineConfig struct {
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

// EngineConstructor builds an EngineAdapter from an EngineConfig.
type EngineConstructor func(cfg EngineConfig) (EngineAdapter, error)

type engineRegistration struct {
	name string
	ctor EngineConstructor
}

var (
	registrationsMu sync.Mutex
	registrations   []engineRegistration
)

// RegisterEngine records an engine constructor under name. Called from
// each engine's init(). Panics on duplicate registration (programmer
// error caught at startup).
func RegisterEngine(name string, ctor EngineConstructor) {
	registrationsMu.Lock()
	defer registrationsMu.Unlock()
	for _, r := range registrations {
		if r.name == name {
			panic(fmt.Sprintf("ai: engine %q already registered", name))
		}
	}
	registrations = append(registrations, engineRegistration{name: name, ctor: ctor})
}

// =========================================================
// Section 3: Factory facade
// =========================================================

// Factory is the unified entry point for engine construction, lookup,
// and dynamic routing. The composition root (internal/app/app.go)
// builds one Factory, Builds the enabled engines, and hands derived
// values (Enabled map / Dynamic adapter) to consumer code.
type Factory struct {
	names []string
	ctors map[string]EngineConstructor
	built map[string]EngineAdapter
}

// NewFactory snapshots the package-level registrations into a new
// Factory instance. Engines registered after this call do not appear.
func NewFactory() *Factory {
	registrationsMu.Lock()
	defer registrationsMu.Unlock()
	f := &Factory{
		names: make([]string, 0, len(registrations)),
		ctors: make(map[string]EngineConstructor, len(registrations)),
		built: make(map[string]EngineAdapter),
	}
	for _, r := range registrations {
		f.names = append(f.names, r.name)
		f.ctors[r.name] = r.ctor
	}
	return f
}

// Build constructs every registered engine for which isEnabled returns
// true. Iteration follows registration order; on the first constructor
// error, Build returns the error wrapped as "init engine %q: %w". The
// caller should discard the Factory on error.
func (f *Factory) Build(isEnabled func(name string) bool, rawCfg func(name string) map[string]any) error {
	for _, name := range f.names {
		if !isEnabled(name) {
			continue
		}
		adapter, err := f.ctors[name](EngineConfig{Raw: rawCfg(name)})
		if err != nil {
			return fmt.Errorf("init engine %q: %w", name, err)
		}
		f.built[name] = adapter
	}
	return nil
}

// Get returns the previously built engine and whether it exists.
func (f *Factory) Get(name string) (EngineAdapter, bool) {
	a, ok := f.built[name]
	return a, ok
}

// Enabled returns a fresh map of name -> built engine. Callers may
// mutate the returned map freely; the Factory's internal state is
// unaffected.
func (f *Factory) Enabled() map[string]EngineAdapter {
	out := make(map[string]EngineAdapter, len(f.built))
	for k, v := range f.built {
		out[k] = v
	}
	return out
}

// Names returns all registered engine names in registration order.
func (f *Factory) Names() []string {
	out := make([]string, len(f.names))
	copy(out, f.names)
	return out
}

// =========================================================
// Section 4: Engine args resolver
// =========================================================

// ResolveExtraArgs merges any number of engine_args JSON layers and
// returns the raw CLI line for engineName. Each layer is JSON shaped as
// {"<engine>": "<cli line>", ...}. Empty layers ("" and "{}") are
// skipped. A malformed JSON layer is silently skipped, matching the
// behaviour of the old ParseEngineArgsJSON: a corrupt sysconfig row
// must not block running engines.
//
// Layers are concatenated in the order given (base, override, ...) with
// a single space separator. The same base+override semantics as the
// previous MergeEngineArgs but on the un-tokenised string — equivalent
// because the lexer treats whitespace as the token separator.
//
// Returns "" when no layer contributes a value for engineName.
func ResolveExtraArgs(engineName string, layers ...string) string {
	var parts []string
	for _, layer := range layers {
		if layer == "" || layer == "{}" {
			continue
		}
		var raw map[string]string
		if json.Unmarshal([]byte(layer), &raw) != nil {
			continue
		}
		if v, ok := raw[engineName]; ok && v != "" {
			parts = append(parts, v)
		}
	}
	return strings.Join(parts, " ")
}

// ValidateExtraArgs returns nil if s tokenises cleanly under the shared
// CLI lexer (single/double quotes, backslash escape). Used at config
// ingestion to surface typos before they hit a running engine.
func ValidateExtraArgs(s string) error {
	_, err := cliargs.SplitArgs(s)
	return err
}
