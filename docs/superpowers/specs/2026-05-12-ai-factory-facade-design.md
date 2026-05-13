# internal/ai Factory Facade Refactor

## Background

`internal/ai/ai.go` is a 349-line file containing five loosely related
sections: engine name constants, core contracts, the engine registry,
the dynamic-routing adapter, and CLI argument parsing. Because every
symbol is exported at package level, any caller in the codebase can
reach into low-level primitives (`Registry`, `DefaultRegistry`,
`NewRunResult`, `DynamicAdapter`, `EngineConfig`) regardless of whether
those are appropriate for that layer. The package therefore has no
clear front door: the API surface is a flat namespace of unrelated
helpers.

Concretely, the current callers fall into three groups but the public
surface does not reflect this:

- **Composition root** (`internal/app/app.go`): builds the engine
  map, looks up the default engine, constructs the dynamic adapter,
  and wires everything into worker / bee / tokenstat.
- **Business consumers** (`tokenstat`, `worker`, `bee`, `task`, `rpc`):
  only need the `EngineAdapter` interface plus value types
  (`RunOptions`, `RunResult`, `Output`, `TokenUsage`,
  `ErrSessionDataNotFound`).
- **Engine implementations** (`internal/ai/engine/{claude,codex,pi,kimi}`):
  self-register via `init()` and rely on `internal/ai/core` for the
  base adapter scaffolding.

Right now group 2 can also see `Registry`, `New(name, cfg)`,
`NewRunResult`, `DynamicAdapter`, and the args parser. The refactor's
goal is to give group 1 a single, cohesive entry point (a `Factory`)
and to hide the construction-time machinery from group 2.

## Goals

- Introduce `ai.Factory` as the sole front door for engine
  construction, lookup, dynamic routing, and enabled-engine iteration.
- Hide registry plumbing (`Registry`, `DefaultRegistry`,
  `DynamicAdapter`) from consumers; replace `ai.Register` with
  `ai.RegisterEngine` and the per-call `ai.New(name, cfg)` with
  `Factory.Build` + `Factory.Get`.
- Split the package into two files reflecting the public/private
  contract layering: `factory.go` for the factory facade and engine
  domain helpers, `contracts.go` for consumer-visible value types and
  the `EngineAdapter` interface.
- Drop `NewRunResult` from the public `ai` surface; the only caller
  lives in `internal/ai/core`, so the helper moves there.
- Leave consumer code (`tokenstat`, `worker`, `bee`, `task`, `rpc`,
  `cmd/openbee/config.go`) unchanged — they continue to depend only on
  the consumer contracts.

## Non-Goals

- No changes to `internal/ai/core/*` beyond accepting the relocated
  `NewRunResult` function.
- No changes to engine backend implementations
  (`internal/ai/engine/*/backend.go`).
- No changes to `RunOptions`, `RunResult`, `Output`, or any other
  consumer-visible field or behaviour.
- No changes to `internal/domain/enginecfg`.
- No reshuffling of the args parsing API at the call sites
  (`worker/manager.go`, `bee/bee_process.go`) — only the file location
  inside `internal/ai/` changes.

## File Structure

```
internal/ai/
├── factory.go          (new — Factory, engine constants, RegisterEngine,
│                        EngineConfig, args helpers)
├── contracts.go        (new — consumer-visible types and interfaces)
├── factory_test.go     (new — Factory / Registry / Args / Dynamic tests)
├── core/               (unchanged, now hosts NewRunResult)
└── engine/             (unchanged — init() switches to RegisterEngine)
```

`ai.go` and `ai_test.go` are deleted. The existing `ai_test.go`
contents are split: `TestNewRunResult_*` moves to
`internal/ai/core/run_result_test.go` (alongside the relocated
`NewRunResult`); everything else moves to `factory_test.go` and is
rewritten against the new Factory API.

## Consumer Contracts (`contracts.go`)

Stable, externally visible types and interfaces. No behavioural change
from the current implementation:

```go
package ai

type Role string
const (
    RoleBee    Role = "bee"
    RoleWorker Role = "worker"
)

type RunOptions struct {
    SessionID string
    Resume    bool
    APIKey    string
    ExtraEnv  []string
    ExtraArgs []string
}

type OutputType string
const (
    OutputDone  OutputType = "done"
    OutputError OutputType = "error"
)

type Output struct {
    Type    OutputType `json:"type"`
    Content string     `json:"content"`
}

type Process interface {
    PID() int
    Stop() error
}

type RunResult struct {
    Process       Process
    Output        <-chan Output
    ExtractResult func() string
}

type EngineAdapter interface {
    Run(ctx context.Context, workDir, prompt string,
        opts RunOptions, logPath string) (RunResult, error)
    CollectTokenUsage(ctx context.Context, sessionID string) ([]TokenUsage, error)
}

type TokenUsage struct {
    Model               string
    InputTokens         int64
    OutputTokens        int64
    CacheCreationTokens int64
    CacheReadTokens     int64
}

var ErrSessionDataNotFound = errors.New("ai: session data not found")
```

`NewRunResult` is removed from this file; see the Relocations section
below.

## Factory Facade (`factory.go`)

### Engine identifiers

```go
const (
    EngineClaude = "claude"
    EngineCodex  = "codex"
    EnginePi     = "pi"
    EngineKimi   = "kimi"
)

// AllEngines returns the canonical engine names in registration order.
// Implemented as a snapshot over the internal registration list so that
// behaviour matches the previous package-level helper.
func AllEngines() []string
```

### Engine self-registration

```go
type EngineConfig struct {
    Raw map[string]any
}

func (c EngineConfig) PathOrDefault(def string) string
func (c EngineConfig) ExtraEnv() map[string]string

type EngineConstructor func(cfg EngineConfig) (EngineAdapter, error)

// RegisterEngine records an engine constructor under name. Called from
// each engine's init(). Panics on duplicate registration (programmer
// error caught at startup). Replaces the previous ai.Register.
func RegisterEngine(name string, ctor EngineConstructor)
```

The registration list is package-private state — a slice of
`(name, ctor)` pairs preserving insertion order. `AllEngines` and
`Factory` both read from it.

### Factory type

```go
type Factory struct {
    ctors map[string]EngineConstructor // populated from registrations
    built map[string]EngineAdapter     // populated by Build
    names []string                     // insertion order of registrations
}

// NewFactory snapshots the package-level registrations into a new
// Factory instance.
func NewFactory() *Factory

// Build constructs every registered engine for which isEnabled returns
// true, using rawCfg to fetch its raw configuration map. Iteration
// proceeds in registration order; on the first constructor error,
// Build aborts and returns the error wrapped as
// "init engine %q: %w". Previously-built engines remain in the
// Factory's internal map; the caller is responsible for discarding
// the Factory on failure.
func (f *Factory) Build(
    isEnabled func(name string) bool,
    rawCfg    func(name string) map[string]any,
) error

// Get returns the previously built engine and whether it exists.
func (f *Factory) Get(name string) (EngineAdapter, bool)

// Enabled returns a fresh map of name -> built engine. Callers mutate
// the returned map freely; the Factory's internal state is unaffected.
func (f *Factory) Enabled() map[string]EngineAdapter

// Names returns all registered engine names in registration order
// (whether or not Build constructed them).
func (f *Factory) Names() []string

// Dynamic returns an EngineAdapter that routes each call through
// cfg.Get() at invocation time. The returned adapter is the previously
// public DynamicAdapter, now an unexported type behind the interface.
func (f *Factory) Dynamic(cfg *enginecfg.Store) EngineAdapter
```

The unexported `dynamicAdapter` type inside `factory.go` carries
exactly the current `DynamicAdapter` behaviour:

- `Run` looks up the current engine in `f.built` via `cfg.Get()` and
  delegates; its `ExtractResult` closes over the engine picked at
  `Run` time, so a later `cfg.Set` does not affect in-flight results.
- `CollectTokenUsage` returns `ErrSessionDataNotFound` (unchanged).

### Engine CLI argument helpers

Package-level functions, in `factory.go`. Signatures preserved
verbatim from the current implementation so callers in
`worker/manager.go`, `bee/bee_process.go`, and `factory_test.go` need
no changes beyond the package import path (which is the same).

```go
type EngineArgsMap map[string][]string
func ParseEngineArgs(raw map[string]string) (EngineArgsMap, error)
func ParseEngineArgsJSON(value string) EngineArgsMap
func MergeEngineArgs(base, override EngineArgsMap) EngineArgsMap
```

`splitCLIArgs` stays unexported. No behaviour change.

## Symbols That Disappear from the Public Surface

| Symbol | Disposition |
|--------|-------------|
| `Registry`, `NewRegistry` | Replaced by `Factory`'s private `ctors` map |
| `DefaultRegistry` | Replaced by package-private registration list |
| `Register(name, fn)` | Renamed `RegisterEngine(name, ctor)` |
| `New(name, cfg)` | Folded into `Factory.Build` |
| `ErrUnknownEngine` | Removed; `Factory.Get` returns `(_, false)` instead |
| `DynamicAdapter`, `NewDynamicAdapter` | Unexported `dynamicAdapter`; produced via `Factory.Dynamic` |
| `NewRunResult` | Moved to `internal/ai/core` (sole caller) |

## Caller Migration

| Caller | Before | After |
|--------|--------|-------|
| `internal/ai/engine/{claude,codex,pi,kimi}/adapter.go` | `ai.Register(ai.EngineClaude, fn)` | `ai.RegisterEngine(ai.EngineClaude, fn)` |
| `internal/app/app.go` `buildAllEngines` | Loops `ai.AllEngines()`, calls `ai.New(name, ...)`, returns `map[string]ai.EngineAdapter` | Returns `*ai.Factory`; body becomes `f := ai.NewFactory(); err := f.Build(cfg.Bee.Engines.IsEnabled, cfg.Bee.EngineConfigRawFor); return f, err` |
| `app.go` default-engine check | `engines[defaultEngine] == nil` | `_, ok := factory.Get(defaultEngine); !ok` |
| `app.go` worker manager | `buildWorkerManager(..., engines, ...)` | `buildWorkerManager(..., factory.Enabled(), ...)` |
| `app.go` bee wiring | `dynamic := ai.NewDynamicAdapter(engines, engineCfg)` | `dynamic := factory.Dynamic(engineCfg)` |
| `app.go` token syncer | `tokenstat.NewSyncer(db, store, engines, ai.AllEngines())` | `tokenstat.NewSyncer(db, store, factory.Enabled(), factory.Names())` |
| `internal/domain/worker/manager.go` | `ai.ParseEngineArgs`, `ai.ParseEngineArgsJSON`, `ai.MergeEngineArgs`, `ai.EngineArgsMap` | Unchanged |
| `internal/domain/bee/bee_process.go` | Same args helpers | Unchanged |
| `internal/tokenstat/*`, `internal/rpc/*`, `internal/domain/{task,worker,bee}/*` consumer code | `ai.EngineAdapter`, `ai.RunOptions`, `ai.RunResult`, `ai.Output`, `ai.TokenUsage`, `ai.ErrSessionDataNotFound` | Unchanged |
| `cmd/openbee/config.go` | `ai.EngineClaude` etc., `ai.AllEngines()` | Unchanged |
| `internal/ai/core/*` | `ai.NewRunResult` | `core.NewRunResult` (relocated, intra-package call) |

## Relocations

- `NewRunResult` (and its associated `sync.Once` memoization) moves
  from `internal/ai/ai.go` to a new `internal/ai/core/run_result.go`,
  becoming `core.NewRunResult` with the same signature. The two
  `TestNewRunResult_*` cases move to `internal/ai/core/run_result_test.go`.
- Callers inside `internal/ai/core` switch from `ai.NewRunResult` to
  the package-local symbol. The only such call sites are in the
  existing `core` package files; no external caller is affected.

## Error Handling

- `Factory.Build` returns the first construction error, wrapped as
  `fmt.Errorf("init engine %q: %w", name, err)` — same wording as the
  current `app.go` block.
- `Factory.Get(name)` returns `(nil, false)` for unknown names. The
  old `ErrUnknownEngine` sentinel disappears; callers (only
  `app.go`) check the boolean.
- `Factory.Dynamic` returns an adapter whose `Run` produces
  `fmt.Errorf("engine %q not available", name)` when the configured
  engine is not in `f.built` — unchanged from the existing
  `DynamicAdapter.Run`.
- `RegisterEngine` panics on duplicate name (matches current
  `Registry.Register`, since duplicate registration is a programmer
  error caught at startup).

## Testing Strategy

The new `internal/ai/factory_test.go` covers:

- **Build**: skips engines for which `isEnabled` returns false; builds
  the rest; returns the first error wrapped as expected.
- **Get / Enabled / Names**: `Get` returns built engines and `(_, false)`
  for unknown names; `Enabled` returns a defensive copy (mutating it
  does not affect a subsequent call); `Names` preserves registration
  order regardless of which engines `Build` actually constructed.
- **Dynamic**: routes to the engine selected by `enginecfg.Store.Get()`
  at `Run` time; `RunResult.ExtractResult` is bound to the engine
  picked at `Run`, not the one current at extraction time; returns an
  error from `Run` when the configured engine is not in the Factory.
- **Args helpers**: keep the existing `TestParseEngineArgs_*` and
  `TestMergeEngineArgs_*` cases verbatim; they continue to exercise
  package-level functions.

`internal/ai/core/run_result_test.go` keeps the two `NewRunResult`
cases (memoization and error propagation).

Existing tests in `internal/ai/core/*_test.go` and
`internal/ai/engine/*/backend_test.go` are unaffected.

## Open Risks

- Engine `init()` ordering: switching from `ai.Register` to
  `ai.RegisterEngine` does not change Go's package-init ordering, but
  the new name must be in place in `internal/ai` before any engine
  package compiles. The implementation plan should land the new API
  and engine `init()` updates in a single commit to avoid a broken
  intermediate state.
- `Factory.NewFactory` snapshotting registrations means engine packages
  must be imported (for their side effects) before `NewFactory` is
  called. `internal/app/app.go` already does this via blank imports;
  no other entry point needs updating.
