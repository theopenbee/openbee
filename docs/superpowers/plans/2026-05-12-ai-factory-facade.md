# `internal/ai` Factory Facade Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the flat, leaky `internal/ai/ai.go` surface with a `Factory` facade. Engines self-register via `ai.RegisterEngine`; `*ai.Factory` becomes the single entry point that the composition root (`app.go`) talks to. Consumer code keeps importing the same value types and interface from `internal/ai` and stays untouched.

**Architecture:** Two files in `internal/ai/`: `contracts.go` (consumer-visible types — `EngineAdapter`, `RunOptions`, `RunResult`, `Output`, `Process`, `TokenUsage`, `ErrSessionDataNotFound`, `Role`) and `factory.go` (engine identifiers, `EngineConfig`, `RegisterEngine`, `Factory` + methods, `dynamicAdapter`, CLI argument helpers). `NewRunResult` moves down into `internal/ai/core` (its only caller). `Registry / DefaultRegistry / DynamicAdapter / ErrUnknownEngine` are removed from the public surface.

**Tech Stack:** Go 1.23, standard library. Tests use `package ai_test` (external) and `testing` from the stdlib.

**Spec:** `docs/superpowers/specs/2026-05-12-ai-factory-facade-design.md`

---

## Pre-flight

- [ ] **Step 0.1: Confirm starting branch**

Run: `git rev-parse --abbrev-ref HEAD`
Expected: `refactor/internal-ai-cleanup`

- [ ] **Step 0.2: Ensure `web/dist/` exists so `internal/app` builds**

The `web/embed.go` file uses `//go:embed all:dist` and fails to compile when `web/dist/` does not exist. The frontend build is not part of this refactor — create a placeholder so Go can compile `internal/app/...` during verification.

Run:
```bash
mkdir -p web/dist && touch web/dist/.placeholder
```

Expected: silent success. Do **not** commit `web/dist/.placeholder` (it is a local dev workaround).

- [ ] **Step 0.3: Baseline tests pass**

Run: `go test ./internal/...`
Expected: every package reports `ok` or `[no test files]`; no `FAIL` lines.

- [ ] **Step 0.4: Note current public surface**

Read `internal/ai/ai.go` once end-to-end so you know what symbols exist. Key public names that disappear by the end of this plan: `Register`, `New(name, cfg)`, `Registry`, `NewRegistry`, `DefaultRegistry`, `ErrUnknownEngine`, `DynamicAdapter`, `NewDynamicAdapter`, `NewRunResult`.

---

## Task 1: Move `NewRunResult` to `internal/ai/core`

**Why first:** `NewRunResult` has exactly one caller (`internal/ai/core/adapter.go`). Moving it down is a self-contained, easily reverted change that shrinks the public `ai` surface before any restructuring happens.

**Files:**
- Create: `internal/ai/core/run_result.go`
- Create: `internal/ai/core/run_result_test.go`
- Modify: `internal/ai/core/adapter.go` (one call site)
- Modify: `internal/ai/ai.go` (remove `NewRunResult` definition)
- Modify: `internal/ai/ai_test.go` (remove the two `TestNewRunResult_*` tests; they relocate)

- [ ] **Step 1.1: Write the failing test in core**

Create `internal/ai/core/run_result_test.go`:

```go
package core

import (
	"errors"
	"testing"

	"github.com/theopenbee/openbee/internal/ai"
)

func TestNewRunResult_MemoizesExtract(t *testing.T) {
	calls := 0
	res, err := NewRunResult(nil, nil, nil, func() string {
		calls++
		return "value"
	})
	if err != nil {
		t.Fatalf("NewRunResult: %v", err)
	}
	for i := range 3 {
		if got := res.ExtractResult(); got != "value" {
			t.Fatalf("call %d: got %q want %q", i, got, "value")
		}
	}
	if calls != 1 {
		t.Errorf("extract should run once; got %d calls", calls)
	}
}

func TestNewRunResult_PropagatesError(t *testing.T) {
	wantErr := errors.New("boom")
	_, err := NewRunResult(nil, nil, wantErr, func() string { return "" })
	if !errors.Is(err, wantErr) {
		t.Errorf("want %v, got %v", wantErr, err)
	}
}

// Ensure ai package import is used so the test file compiles even if
// future edits remove the only ai.* reference.
var _ ai.Process = (ai.Process)(nil)
```

- [ ] **Step 1.2: Run new tests; expect failure**

Run: `go test ./internal/ai/core/ -run TestNewRunResult -v`
Expected: compile error — `undefined: NewRunResult`.

- [ ] **Step 1.3: Create `internal/ai/core/run_result.go`**

```go
package core

import (
	"sync"

	"github.com/theopenbee/openbee/internal/ai"
)

// NewRunResult builds an ai.RunResult, propagating err unchanged. The
// provided extract function is wrapped with sync.Once so ExtractResult
// only runs the underlying scan the first time and returns the cached
// result thereafter.
func NewRunResult(proc ai.Process, out <-chan ai.Output, err error, extract func() string) (ai.RunResult, error) {
	if err != nil {
		return ai.RunResult{}, err
	}
	var (
		once   sync.Once
		result string
	)
	memo := func() string {
		once.Do(func() { result = extract() })
		return result
	}
	return ai.RunResult{Process: proc, Output: out, ExtractResult: memo}, nil
}
```

- [ ] **Step 1.4: Update `internal/ai/core/adapter.go` to call the local symbol**

Open `internal/ai/core/adapter.go`. The body of `(*engineAdapter).Run` currently calls `ai.NewRunResult(...)`. Replace the call:

Before (around line 36-39):
```go
	proc, out, err := a.base.Run(ctx, workDir, prompt, opts, logPath)
	return ai.NewRunResult(proc, out, err, func() string {
		return a.base.Extract(logPath)
	})
```

After:
```go
	proc, out, err := a.base.Run(ctx, workDir, prompt, opts, logPath)
	return NewRunResult(proc, out, err, func() string {
		return a.base.Extract(logPath)
	})
```

- [ ] **Step 1.5: Remove `NewRunResult` from `internal/ai/ai.go`**

Delete lines 87-100 (the `NewRunResult` function). Also remove the `"sync"` import at the top of the file — it is no longer referenced after this deletion (verify by reading the imports block: if `sync` is unused, `go build` will complain).

- [ ] **Step 1.6: Remove the relocated tests from `internal/ai/ai_test.go`**

Delete `TestNewRunResult_MemoizesExtract` and `TestNewRunResult_PropagatesError` (lines 28-53 in the current file).

- [ ] **Step 1.7: Verify build and tests**

Run: `go test ./internal/ai/...`
Expected: all packages `ok`. The two `NewRunResult` tests now run inside `internal/ai/core/` instead of `internal/ai/`.

Run: `go vet ./internal/ai/...`
Expected: no output.

- [ ] **Step 1.8: Commit**

```bash
git add internal/ai/core/run_result.go internal/ai/core/run_result_test.go internal/ai/core/adapter.go internal/ai/ai.go internal/ai/ai_test.go
git commit -m "$(cat <<'EOF'
refactor(ai): move NewRunResult into core package

NewRunResult had a single caller in internal/ai/core/adapter.go. Moving
it down removes a leaked helper from the public ai surface ahead of
the Factory facade refactor.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: Add `Factory` facade alongside existing API

**Why this shape:** The Factory is added without removing the old `Register / New / DynamicAdapter` symbols. Both APIs coexist during this task so engine `init()`s and `app.go` keep compiling. The next two tasks migrate the callers, and Task 5 removes the old API. This keeps every commit green.

**Files:**
- Create: `internal/ai/factory.go`
- Create: `internal/ai/factory_test.go`

- [ ] **Step 2.1: Write failing tests for the Factory**

Create `internal/ai/factory_test.go`:

```go
package ai_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	ai "github.com/theopenbee/openbee/internal/ai"
	"github.com/theopenbee/openbee/internal/domain/enginecfg"
)

// Test stubs registered at init time. Each test uses a unique engine
// name so we never trip the RegisterEngine duplicate-name panic.
func init() {
	ai.RegisterEngine("factory-test-a", func(_ ai.EngineConfig) (ai.EngineAdapter, error) {
		return &factoryStubEngine{name: "factory-test-a"}, nil
	})
	ai.RegisterEngine("factory-test-b", func(_ ai.EngineConfig) (ai.EngineAdapter, error) {
		return &factoryStubEngine{name: "factory-test-b"}, nil
	})
	ai.RegisterEngine("factory-test-fail", func(_ ai.EngineConfig) (ai.EngineAdapter, error) {
		return nil, errors.New("boom")
	})
}

type factoryStubEngine struct{ name string }

func (s *factoryStubEngine) Run(_ context.Context, _, _ string, _ ai.RunOptions, _ string) (ai.RunResult, error) {
	name := s.name
	return ai.RunResult{ExtractResult: func() string { return name + "-result" }}, errors.New(name + " run called")
}
func (s *factoryStubEngine) CollectTokenUsage(_ context.Context, _ string) ([]ai.TokenUsage, error) {
	return nil, ai.ErrSessionDataNotFound
}

func enabledForTest(names ...string) func(string) bool {
	set := make(map[string]struct{}, len(names))
	for _, n := range names {
		set[n] = struct{}{}
	}
	return func(name string) bool { _, ok := set[name]; return ok }
}

func rawCfgForTest(map[string]any) func(string) map[string]any {
	return func(string) map[string]any { return nil }
}

func TestFactory_BuildOnlyConstructsEnabled(t *testing.T) {
	f := ai.NewFactory()
	err := f.Build(enabledForTest("factory-test-a"), func(string) map[string]any { return nil })
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if _, ok := f.Get("factory-test-a"); !ok {
		t.Error("expected factory-test-a to be built")
	}
	if _, ok := f.Get("factory-test-b"); ok {
		t.Error("expected factory-test-b to be skipped")
	}
}

func TestFactory_BuildPropagatesError(t *testing.T) {
	f := ai.NewFactory()
	err := f.Build(enabledForTest("factory-test-fail"), func(string) map[string]any { return nil })
	if err == nil {
		t.Fatal("expected Build to return error")
	}
	want := `init engine "factory-test-fail": boom`
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}

func TestFactory_GetUnknownReturnsFalse(t *testing.T) {
	f := ai.NewFactory()
	if _, ok := f.Get("does-not-exist"); ok {
		t.Error("expected Get to return false for unknown engine")
	}
}

func TestFactory_EnabledReturnsCopy(t *testing.T) {
	f := ai.NewFactory()
	if err := f.Build(enabledForTest("factory-test-a"), func(string) map[string]any { return nil }); err != nil {
		t.Fatalf("Build: %v", err)
	}
	m := f.Enabled()
	delete(m, "factory-test-a")
	if _, ok := f.Get("factory-test-a"); !ok {
		t.Error("mutating Enabled() result must not affect Factory state")
	}
}

func TestFactory_NamesIncludesAllRegistrations(t *testing.T) {
	f := ai.NewFactory()
	names := f.Names()
	if !slices.Contains(names, "factory-test-a") || !slices.Contains(names, "factory-test-b") {
		t.Errorf("Names() = %v, expected to include factory-test-a and factory-test-b", names)
	}
	// Names returns all registrations regardless of Build.
	if err := f.Build(enabledForTest("factory-test-a"), func(string) map[string]any { return nil }); err != nil {
		t.Fatalf("Build: %v", err)
	}
	if got := f.Names(); !slices.Contains(got, "factory-test-b") {
		t.Errorf("Names() after Build = %v, expected factory-test-b still present", got)
	}
}

func TestFactory_DynamicRoutesToCurrentEngine(t *testing.T) {
	f := ai.NewFactory()
	if err := f.Build(enabledForTest("factory-test-a", "factory-test-b"), func(string) map[string]any { return nil }); err != nil {
		t.Fatalf("Build: %v", err)
	}
	cfg := enginecfg.NewStore("factory-test-a")
	dyn := f.Dynamic(cfg)

	_, err := dyn.Run(context.Background(), "/w", "p", ai.RunOptions{}, "/log")
	if err == nil || err.Error() != "factory-test-a run called" {
		t.Errorf("expected 'factory-test-a run called', got %v", err)
	}

	cfg.Set("factory-test-b")
	_, err = dyn.Run(context.Background(), "/w", "p", ai.RunOptions{}, "/log")
	if err == nil || err.Error() != "factory-test-b run called" {
		t.Errorf("expected 'factory-test-b run called', got %v", err)
	}
}

func TestFactory_DynamicBindsExtractToRunTimeEngine(t *testing.T) {
	f := ai.NewFactory()
	if err := f.Build(enabledForTest("factory-test-a", "factory-test-b"), func(string) map[string]any { return nil }); err != nil {
		t.Fatalf("Build: %v", err)
	}
	cfg := enginecfg.NewStore("factory-test-a")
	dyn := f.Dynamic(cfg)

	res, _ := dyn.Run(context.Background(), "/w", "p", ai.RunOptions{}, "/log")
	cfg.Set("factory-test-b") // simulate /engine switch mid-execution
	if got := res.ExtractResult(); got != "factory-test-a-result" {
		t.Errorf("expected run-time engine extractor; got %s", got)
	}
}

func TestFactory_DynamicUnknownEngineErrors(t *testing.T) {
	f := ai.NewFactory()
	if err := f.Build(enabledForTest("factory-test-a"), func(string) map[string]any { return nil }); err != nil {
		t.Fatalf("Build: %v", err)
	}
	cfg := enginecfg.NewStore("not-built")
	dyn := f.Dynamic(cfg)
	_, err := dyn.Run(context.Background(), "/w", "p", ai.RunOptions{}, "/log")
	if err == nil {
		t.Fatal("expected error for unknown engine")
	}
}
```

- [ ] **Step 2.2: Run new tests; expect failure**

Run: `go test ./internal/ai/ -run TestFactory -v`
Expected: compile error — `undefined: ai.NewFactory`, `undefined: ai.RegisterEngine`, etc.

- [ ] **Step 2.3: Create `internal/ai/factory.go`**

```go
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
```

Note: this file currently **duplicates** the engine identifiers, `EngineConfig`, args helpers, and (functionally) the dynamic adapter that still live in `ai.go`. That is intentional — Task 5 deletes the old copies. Go's compile will fail with duplicate-declaration errors until then. **Do not run `go build` yet** — proceed directly to Step 2.4 which removes the duplicates.

- [ ] **Step 2.4: Remove the now-duplicated declarations from `internal/ai/ai.go`**

Open `internal/ai/ai.go`. Delete the following ranges (line numbers refer to the file's state after Task 1, where `NewRunResult` and `sync` import are already gone):

1. **Section 1 (Engine identifiers)** — delete the `const` block for `EngineClaude/EngineCodex/EnginePi/EngineKimi`, the `allEngines` var, and the `AllEngines` function. Replaced by `factory.go`.
2. **Section 4 (Dynamic routing)** — delete the `DynamicAdapter` type, `NewDynamicAdapter`, and both methods. Replaced by `dynamicAdapter` in `factory.go`.
3. **Section 5 (Engine argument helpers)** — delete `EngineArgsMap`, `ParseEngineArgs`, `splitCLIArgs`, `MergeEngineArgs`, `ParseEngineArgsJSON`. Replaced by `factory.go`.
4. **Section 3 (Registry) — keep for now.** `Register / New / Registry / NewRegistry / DefaultRegistry / ErrUnknownEngine / EngineConfig / Factory (type alias for constructor)` still exist; engines and `app.go` will keep using them until Tasks 3-4. BUT there is a name collision: `factory.go` defines `EngineConfig` and `Factory`, and `ai.go` also defines them.

  Resolve the collisions:
  - **`EngineConfig`** (struct): delete the duplicate from `ai.go`. `factory.go` keeps it. Tests in `ai_test.go` will continue to import it via `ai.EngineConfig` — same package-level symbol, same fields.
  - **`Factory`** (type alias `type Factory func(cfg EngineConfig) (EngineAdapter, error)`): rename in `ai.go` to **`legacyFactory`**, unexported, and update the one reference inside the `Registry` struct (`factories map[string]Factory` → `factories map[string]legacyFactory`) plus the `Register` method signature (`f Factory` → `f legacyFactory`). Update `ai.Register(name, fn)` callers? No — Go uses structural typing for function types only when the argument is a function literal, so any literal passed at a call site still binds; named-type usages would need an update but there are none (engine `init()`s pass anonymous func literals, and tests in `ai_test.go` likewise pass literals).

  After the rename, `ai.go` no longer exports `Factory` as the legacy constructor alias, so `factory.go`'s `Factory` *struct* compiles uniquely.

  Specifically, in `ai.go` change:

  ```go
  type Factory func(cfg EngineConfig) (EngineAdapter, error)
  ```

  to:

  ```go
  type legacyFactory func(cfg EngineConfig) (EngineAdapter, error)
  ```

  and update inside `ai.go`:
  ```go
  factories map[string]Factory       // -> factories map[string]legacyFactory
  func (r *Registry) Register(name string, f Factory) // -> func (r *Registry) Register(name string, f legacyFactory)
  func Register(name string, f Factory)               // -> func Register(name string, f legacyFactory)
  ```

  Then update `internal/ai/ai_test.go` lines 182, 208 — the test cases pass `func(_ ai.EngineConfig) (ai.EngineAdapter, error) { ... }` literals, which assign-fit `legacyFactory` automatically. **No change needed in `ai_test.go`** for the rename itself. Only the `ai.RegisterEngine` based factory tests in `factory_test.go` (Step 2.1) use the new API.

  At engine init() sites (`internal/ai/engine/*/adapter.go`), `ai.Register(name, func(...) ...)` also passes literals — no change needed until Task 3.

5. **Imports**: after the deletions in `ai.go`, several imports become unused. Inspect the imports block and remove anything no longer referenced. The remaining `ai.go` should need only `errors`, `context`, `fmt`. (`encoding/json`, `slices`, `strings`, `unicode`, `sync`, `enginecfg` move with their code into `factory.go`.)

Note: `context` is used by the `EngineAdapter` interface signature (still in `ai.go` for now; moves to `contracts.go` in Task 5).

6. **Delete the `TestDynamicAdapter_*` tests from `internal/ai/ai_test.go`.** The production `DynamicAdapter` type and `NewDynamicAdapter` constructor are gone, so these three tests would fail to compile:
   - `TestDynamicAdapter_RunRoutesToCurrentEngine`
   - `TestDynamicAdapter_RunBindsExtractResultToEngine`
   - `TestDynamicAdapter_RunUnknownEngine`

   Equivalent coverage now lives in `factory_test.go` as `TestFactory_DynamicRoutesToCurrentEngine`, `TestFactory_DynamicBindsExtractToRunTimeEngine`, and `TestFactory_DynamicUnknownEngineErrors`. The `TestRegistry_*` tests and `TestParseEngineArgs_*` / `TestMergeEngineArgs_*` tests stay in `ai_test.go` for now — they keep passing because `Registry / Register / EngineConfig / ParseEngineArgs / MergeEngineArgs / EngineArgsMap` are still package-level symbols (Registry survives until Task 5; the args helpers are now in `factory.go`).

- [ ] **Step 2.5: Verify build and tests**

Run: `go test ./internal/ai/...`
Expected: all packages `ok`. Both old `TestRegistry_*` tests (still in `ai_test.go`) and new `TestFactory_*` tests pass.

Run: `go vet ./internal/ai/...`
Expected: no output.

- [ ] **Step 2.6: Commit**

```bash
git add internal/ai/factory.go internal/ai/factory_test.go internal/ai/ai.go
git commit -m "$(cat <<'EOF'
refactor(ai): add Factory facade alongside legacy registry

Adds ai.Factory, ai.RegisterEngine, ai.NewFactory, and an unexported
dynamicAdapter behind Factory.Dynamic. The legacy Register / New /
Registry / DefaultRegistry / DynamicAdapter symbols still exist so
engine init() functions and app.go keep compiling; subsequent commits
migrate them.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: Migrate engine `init()` functions to `RegisterEngine`

**Files:**
- Modify: `internal/ai/engine/claude/adapter.go`
- Modify: `internal/ai/engine/codex/adapter.go`
- Modify: `internal/ai/engine/kimi/adapter.go`
- Modify: `internal/ai/engine/pi/adapter.go`

- [ ] **Step 3.1: Update `claude/adapter.go`**

Open `internal/ai/engine/claude/adapter.go`. Replace `ai.Register` with `ai.RegisterEngine`:

Before:
```go
func init() {
	ai.Register(ai.EngineClaude, func(cfg ai.EngineConfig) (ai.EngineAdapter, error) {
		return NewAdapter(cfg.PathOrDefault(ai.EngineClaude), cfg.ExtraEnv()), nil
	})
}
```

After:
```go
func init() {
	ai.RegisterEngine(ai.EngineClaude, func(cfg ai.EngineConfig) (ai.EngineAdapter, error) {
		return NewAdapter(cfg.PathOrDefault(ai.EngineClaude), cfg.ExtraEnv()), nil
	})
}
```

- [ ] **Step 3.2: Update `codex/adapter.go`** — same edit, `ai.Register` → `ai.RegisterEngine`, on line 11.

- [ ] **Step 3.3: Update `kimi/adapter.go`** — same edit, line 9.

- [ ] **Step 3.4: Update `pi/adapter.go`** — same edit, line 9.

- [ ] **Step 3.5: Verify**

Run: `go test ./internal/ai/...`
Expected: all packages `ok`. The old `ai.Register` is still present in `ai.go` and the new `ai.RegisterEngine` is present in `factory.go`; engines now use the latter.

- [ ] **Step 3.6: Commit**

```bash
git add internal/ai/engine/claude/adapter.go internal/ai/engine/codex/adapter.go internal/ai/engine/kimi/adapter.go internal/ai/engine/pi/adapter.go
git commit -m "$(cat <<'EOF'
refactor(ai): migrate engine init() to ai.RegisterEngine

Drops the last non-test callers of the legacy ai.Register API. The
next commit migrates app.go off ai.New, after which the legacy
registry can be deleted.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: Migrate `app.go` to use the Factory

**Files:**
- Modify: `internal/app/app.go`

- [ ] **Step 4.1: Rewrite `buildAllEngines` to return a Factory**

Open `internal/app/app.go`. Locate `buildAllEngines` (around lines 271-288). Replace the existing function with:

```go
// buildEngineFactory constructs the ai.Factory and builds every
// enabled engine. The returned Factory becomes the single source of
// truth for engine lookup and dynamic routing.
func buildEngineFactory(cfg config.BeeConfig) (*ai.Factory, error) {
	os.Setenv("OPENBEE_URL", cfg.RPCBaseURL) //nolint:errcheck

	f := ai.NewFactory()
	if err := f.Build(cfg.Engines.IsEnabled, cfg.EngineConfigRawFor); err != nil {
		return nil, err
	}
	return f, nil
}
```

The error wrapping (`"init engine %q: %w"`) is performed inside `Factory.Build`, so the wrapper here is just `f.Build(...)`'s return value.

- [ ] **Step 4.2: Update `BuildApp` to use the new function**

In `BuildApp` (lines ~108-115), replace:

```go
	engines, err := buildAllEngines(cfg.Bee)
	if err != nil {
		return nil, fmt.Errorf("init engines: %w", err)
	}
	defaultEngine := cfg.Bee.EffectiveEngine()
	if engines[defaultEngine] == nil {
		return nil, fmt.Errorf("default engine %q is not enabled; enable it under bee.engines in config", defaultEngine)
	}
```

with:

```go
	factory, err := buildEngineFactory(cfg.Bee)
	if err != nil {
		return nil, fmt.Errorf("init engines: %w", err)
	}
	defaultEngine := cfg.Bee.EffectiveEngine()
	if _, ok := factory.Get(defaultEngine); !ok {
		return nil, fmt.Errorf("default engine %q is not enabled; enable it under bee.engines in config", defaultEngine)
	}
	engines := factory.Enabled()
```

The local `engines` map keeps the same shape (`map[string]ai.EngineAdapter`) so downstream helpers (`buildWorkerManager`, `buildBee`, `tokenstat.NewSyncer`) do not change.

- [ ] **Step 4.3: Replace `ai.NewDynamicAdapter` with `factory.Dynamic`**

In `buildBee` (around line 296), replace:

```go
	dynamic := ai.NewDynamicAdapter(engines, engineCfg)
```

with:

```go
	dynamic := factory.Dynamic(engineCfg)
```

You will need to pass `factory` into `buildBee`. Update the signature:

Before:
```go
func buildBee(cfg config.BeeConfig, s appStores, dispatchCh chan task.DispatchTask,
	failureNotifier bee.FailureNotifier, engines map[string]ai.EngineAdapter, engineCfg *enginecfg.Store, envSvc *env.Service) (*bee.Feeder, *task.Scheduler) {
	dynamic := ai.NewDynamicAdapter(engines, engineCfg)
```

After:
```go
func buildBee(cfg config.BeeConfig, s appStores, dispatchCh chan task.DispatchTask,
	failureNotifier bee.FailureNotifier, factory *ai.Factory, engineCfg *enginecfg.Store, envSvc *env.Service) (*bee.Feeder, *task.Scheduler) {
	dynamic := factory.Dynamic(engineCfg)
```

Update the call site in `BuildApp` accordingly:

Before:
```go
	feeder, sched := buildBee(cfg.Bee, s, dispatchCh, failureNotifier, engines, engineCfg, envSvc)
```

After:
```go
	feeder, sched := buildBee(cfg.Bee, s, dispatchCh, failureNotifier, factory, engineCfg, envSvc)
```

`buildWorkerManager` continues to accept `map[string]ai.EngineAdapter` — leave it as is.

- [ ] **Step 4.4: Replace `ai.AllEngines()` with `factory.Names()` at the syncer call**

In `BuildApp` (line ~194), replace:

```go
	tokenSyncer := tokenstat.NewSyncer(db, s.tokenStatsStore, engines, ai.AllEngines())
```

with:

```go
	tokenSyncer := tokenstat.NewSyncer(db, s.tokenStatsStore, engines, factory.Names())
```

Other `ai.AllEngines()` callers (`cmd/openbee/config.go`) keep using the package-level function — that is still correct.

- [ ] **Step 4.5: Verify build and tests**

Run: `go build ./internal/app/...`
Expected: success.

Run: `go test ./internal/app/... ./internal/ai/... ./internal/domain/... ./internal/tokenstat/... ./internal/rpc/...`
Expected: all `ok`.

- [ ] **Step 4.6: Commit**

```bash
git add internal/app/app.go
git commit -m "$(cat <<'EOF'
refactor(app): wire engine setup through ai.Factory

BuildApp now constructs an ai.Factory once and derives the engine map,
dynamic adapter, and engine-name list from it. Removes the last
caller of ai.NewDynamicAdapter and ai.AllEngines that needed to know
about the legacy registry.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: Delete the legacy API and split contracts into `contracts.go`

**Why now:** Tasks 3 and 4 removed every non-test caller of `Register / New / Registry / DefaultRegistry / DynamicAdapter / NewDynamicAdapter / ErrUnknownEngine`. The legacy tests in `ai_test.go` (`TestRegistry_*`, `TestDynamicAdapter_*`) duplicate coverage we have in `factory_test.go`, so they can go too.

**Files:**
- Delete: `internal/ai/ai.go`
- Delete: `internal/ai/ai_test.go`
- Create: `internal/ai/contracts.go`

- [ ] **Step 5.1: Create `internal/ai/contracts.go`**

```go
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
	// Run executes a task and returns a RunResult carrying the process
	// handle, event channel, and an engine-bound result extractor. The
	// event channel is closed after the process exits.
	Run(ctx context.Context, workDir, prompt string,
		opts RunOptions, logPath string) (RunResult, error)

	// CollectTokenUsage reads per-turn token usage for the given
	// session from engine-specific storage. Returns
	// ErrSessionDataNotFound when no data is available for the
	// session.
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

// ErrSessionDataNotFound is returned by CollectTokenUsage when no
// session data exists.
var ErrSessionDataNotFound = errors.New("ai: session data not found")
```

- [ ] **Step 5.2: Move the args tests into `factory_test.go`**

Before removing `ai_test.go`, port the `TestParseEngineArgs_*` and `TestMergeEngineArgs_*` cases. The `TestRegistry_*` cases die with the legacy registry — equivalent coverage already exists in `factory_test.go`'s `TestFactory_BuildOnlyConstructsEnabled` and `TestFactory_GetUnknownReturnsFalse`.

Append the following blocks to `internal/ai/factory_test.go`:

```go
func TestParseEngineArgs_PreservesOrderAndQuotedValues(t *testing.T) {
	raw := map[string]string{
		"claude": `--model claude-sonnet-4-5 --append-system-prompt "be terse" --verbose`,
	}
	got, err := ai.ParseEngineArgs(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"--model", "claude-sonnet-4-5", "--append-system-prompt", "be terse", "--verbose"}
	if !slices.Equal(got["claude"], want) {
		t.Fatalf("got %v, want %v", got["claude"], want)
	}
}

func TestParseEngineArgs_PreservesDuplicateFlags(t *testing.T) {
	raw := map[string]string{
		"codex": `--include src --include test`,
	}
	got, err := ai.ParseEngineArgs(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"--include", "src", "--include", "test"}
	if !slices.Equal(got["codex"], want) {
		t.Fatalf("got %v, want %v", got["codex"], want)
	}
}

func TestParseEngineArgs_PreservesEmptyQuotedValue(t *testing.T) {
	raw := map[string]string{
		"claude": `--append-system-prompt "" --verbose`,
	}
	got, err := ai.ParseEngineArgs(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"--append-system-prompt", "", "--verbose"}
	if !slices.Equal(got["claude"], want) {
		t.Fatalf("got %v, want %v", got["claude"], want)
	}
}

func TestParseEngineArgs_UnterminatedQuote(t *testing.T) {
	_, err := ai.ParseEngineArgs(map[string]string{
		"claude": `--model "unterminated`,
	})
	if err == nil {
		t.Fatal("expected parse error, got nil")
	}
}

func TestMergeEngineArgs_AppendsOverrideArgs(t *testing.T) {
	base := ai.EngineArgsMap{
		"claude": {"--model", "sonnet", "--verbose"},
	}
	override := ai.EngineArgsMap{
		"claude": {"--model", "opus"},
		"codex":  {"--model", "o3"},
	}
	got := ai.MergeEngineArgs(base, override)

	if want := []string{"--model", "sonnet", "--verbose", "--model", "opus"}; !slices.Equal(got["claude"], want) {
		t.Fatalf("claude args = %v, want %v", got["claude"], want)
	}
	if want := []string{"--model", "o3"}; !slices.Equal(got["codex"], want) {
		t.Fatalf("codex args = %v, want %v", got["codex"], want)
	}
}
```

- [ ] **Step 5.2.1: Delete `internal/ai/ai.go` and `internal/ai/ai_test.go`**

Run:
```bash
rm internal/ai/ai.go internal/ai/ai_test.go
```

What goes away: `legacyFactory`, `Registry`, `NewRegistry`, `DefaultRegistry`, `Register`, `New`, `ErrUnknownEngine`, and the `TestRegistry_*` cases. The consumer-visible types (`Role`, `RunOptions`, `Output*`, `Process`, `RunResult`, `EngineAdapter`, `TokenUsage`, `ErrSessionDataNotFound`) already live in `contracts.go` from Step 5.1.

- [ ] **Step 5.3: Verify build and tests**

Run: `go build ./internal/...`
Expected: success.

Run: `go test ./internal/...`
Expected: all packages `ok` or `[no test files]`. No `FAIL` lines.

Run: `go vet ./internal/ai/...`
Expected: no output.

- [ ] **Step 5.4: Confirm the public surface matches the spec**

Run: `go doc github.com/theopenbee/openbee/internal/ai`

The output lists every exported symbol in the package. Verify that:

**Must appear:**
`AllEngines`, `EngineClaude`, `EngineCodex`, `EngineKimi`, `EnginePi`, `EngineAdapter`, `EngineArgsMap`, `EngineConfig`, `EngineConstructor`, `ErrSessionDataNotFound`, `Factory`, `MergeEngineArgs`, `NewFactory`, `Output`, `OutputDone`, `OutputError`, `OutputType`, `ParseEngineArgs`, `ParseEngineArgsJSON`, `Process`, `RegisterEngine`, `Role`, `RoleBee`, `RoleWorker`, `RunOptions`, `RunResult`, `TokenUsage`.

**Must NOT appear:**
`Register` (without `Engine` suffix), `New` (the old `func New(name, cfg)`), `Registry`, `NewRegistry`, `DefaultRegistry`, `DynamicAdapter`, `NewDynamicAdapter`, `ErrUnknownEngine`, `NewRunResult`, `legacyFactory`.

- [ ] **Step 5.5: Commit**

```bash
git add internal/ai/contracts.go internal/ai/factory_test.go
git rm internal/ai/ai.go internal/ai/ai_test.go
git commit -m "$(cat <<'EOF'
refactor(ai): remove legacy registry and split contracts.go

Deletes Register/New/Registry/DefaultRegistry/DynamicAdapter and
ErrUnknownEngine now that every caller has migrated to ai.Factory.
Consumer-visible types (EngineAdapter, RunOptions, RunResult, Output,
Process, TokenUsage, Role, ErrSessionDataNotFound) move into a
dedicated contracts.go file so the package's public surface reflects
the consumer vs. composition-root layering.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

## Task 6: Final verification

- [ ] **Step 6.1: Full test sweep**

Run: `go test ./internal/...`
Expected: every line is `ok` or `[no test files]`. Zero `FAIL`.

- [ ] **Step 6.2: Vet sweep**

Run: `go vet ./internal/...`
Expected: no output.

- [ ] **Step 6.3: Build everything that doesn't depend on web/dist**

Run:
```bash
go build ./internal/... ./cmd/...
```
Expected: success.

- [ ] **Step 6.4: Confirm no stale references to old names**

Run:
```bash
grep -rnE 'ai\.(NewDynamicAdapter|DefaultRegistry|NewRegistry|ErrUnknownEngine|NewRunResult|DynamicAdapter)' --include='*.go' internal/ cmd/
grep -rnE 'ai\.Register\(' --include='*.go' internal/ cmd/   # matches old ai.Register(, NOT ai.RegisterEngine(
```

Expected: no matches from either grep.

- [ ] **Step 6.5: Push branch**

Run:
```bash
git push -u origin refactor/internal-ai-cleanup
```

- [ ] **Step 6.6: Cleanup**

Remove the local-only dev workaround:
```bash
rm -f web/dist/.placeholder
rmdir web/dist 2>/dev/null || true
```

---

## Notes for the executing engineer

- The `EngineConfig` struct in the new `factory.go` is structurally identical to the old one. If a test or piece of business code constructs `ai.EngineConfig{Raw: m}` literally, it still compiles.
- `factory.RegisterEngine` panics on duplicate names. The tests in `factory_test.go` register their stubs with unique names (`factory-test-a/b/fail`) precisely to avoid colliding with each other or with the four production engines.
- `Factory.NewFactory` snapshots `registrations` once. If a later test registers an engine after `NewFactory()` runs, that engine is **not** visible to that Factory. Use a fresh `ai.NewFactory()` inside each test that needs the latest set.
- The dev-only `web/dist/.placeholder` exists only to satisfy `web/embed.go`'s `//go:embed all:dist` directive during local Go builds. It is **not** part of this refactor and should not be committed.
