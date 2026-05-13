package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"sync"
	"unicode"

	"github.com/theopenbee/openbee/internal/domain/enginecfg"
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

// AllEngines returns the canonical engine name list in registration order.
func AllEngines() []string {
	registrationsMu.Lock()
	defer registrationsMu.Unlock()
	names := make([]string, 0, len(registrations))
	for _, r := range registrations {
		names = append(names, r.name)
	}
	return names
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

// Dynamic returns an EngineAdapter that routes each call through
// cfg.Get() at invocation time. The RunResult.ExtractResult closes
// over the engine picked at Run time, so a later cfg.Set does not
// affect in-flight results.
func (f *Factory) Dynamic(cfg *enginecfg.Store) EngineAdapter {
	return &dynamicAdapter{factory: f, cfg: cfg}
}

type dynamicAdapter struct {
	factory *Factory
	cfg     *enginecfg.Store
}

func (d *dynamicAdapter) Run(ctx context.Context, workDir, prompt string, opts RunOptions, logPath string) (RunResult, error) {
	name := d.cfg.Get()
	e, ok := d.factory.built[name]
	if !ok {
		return RunResult{}, fmt.Errorf("engine %q not available", name)
	}
	return e.Run(ctx, workDir, prompt, opts, logPath)
}

func (d *dynamicAdapter) CollectTokenUsage(_ context.Context, _ string) ([]TokenUsage, error) {
	return nil, ErrSessionDataNotFound
}

// =========================================================
// Section 4: Engine CLI argument helpers
// =========================================================

// EngineArgsMap maps an engine name to its parsed argv slice.
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

// MergeEngineArgs merges base and override by appending override args
// after base args, so later flags can override earlier ones while
// preserving the original CLI ordering.
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
