# Design: Unify `core` Engine Contracts Into a Single `BaseAdapter` Interface

Date: 2026-05-12
Branch: `refactor/internal-ai-cleanup`

## Background

`internal/ai/core/adapter.go` currently exposes three small interfaces that every engine must implement:

```go
type Invoker interface {
    Run(ctx context.Context, workDir, prompt string, opts ai.RunOptions, logPath string) (ai.Process, <-chan ai.Output, error)
}

type Collector interface {
    Collect(ctx context.Context, sessionID string) ([]ai.TokenUsage, error)
}

type Extractor interface {
    Extract(logPath string) string
}
```

These three interfaces are wired into a `BaseAdapter struct` that engines embed:

```go
type BaseAdapter struct {
    Invoker   Invoker
    Collector Collector
    Extractor Extractor
}
```

Each engine package (`claude`, `codex`, `pi`, `kimi`) provides three independent types — `Invoker struct{...}`, `Collector struct{...}`, `Extractor struct{}` — and `NewAdapter` plugs them into a `BaseAdapter`.

This shape is fragmented:

- Three contracts that always travel together but are named/typed/tested separately.
- Every engine repeats the same wiring (three constructors, three fields).
- The `Extractor` struct in every engine is `struct{}` — a clear sign the interface boundary is too granular.

## Goal

Collapse the three interfaces into a single interface, exposed under the existing name `BaseAdapter`. Each engine provides one type (`Backend`) that implements all three methods.

## Non-Goals

- No change to `ai.EngineAdapter` (the engine-facing contract used outside `core`).
- No change to `ai.RunResult`, `ai.NewRunResult`, or `RunOptions`.
- No change to engine behavior or output. This is a pure structural refactor.
- No removal of `claude`'s legacy-rules cleanup; only its placement moves.

## Design

### 1. `internal/ai/core/adapter.go` — interface + wrapper

Delete `Invoker`, `Collector`, `Extractor`, and the `BaseAdapter struct`. Replace with:

```go
// BaseAdapter is the unified engine-side contract. Engine packages provide a
// single type satisfying this interface (typically named Backend).
type BaseAdapter interface {
    // Run launches the engine subprocess; the returned channel is closed after
    // the process exits.
    Run(ctx context.Context, workDir, prompt string,
        opts ai.RunOptions, logPath string) (ai.Process, <-chan ai.Output, error)

    // Collect reads per-turn token usage for the given session. Returns
    // ai.ErrSessionDataNotFound when no data is available.
    Collect(ctx context.Context, sessionID string) ([]ai.TokenUsage, error)

    // Extract reads the per-engine result text from the log file produced by Run.
    Extract(logPath string) string
}

// NewEngineAdapter wraps a BaseAdapter to satisfy ai.EngineAdapter:
//   - Run is wired into ai.NewRunResult with Extract bound to logPath.
//   - CollectTokenUsage delegates to Collect.
func NewEngineAdapter(base BaseAdapter) ai.EngineAdapter {
    return &engineAdapter{base: base}
}

type engineAdapter struct{ base BaseAdapter }

func (a *engineAdapter) Run(ctx context.Context, workDir, prompt string,
    opts ai.RunOptions, logPath string) (ai.RunResult, error) {
    proc, out, err := a.base.Run(ctx, workDir, prompt, opts, logPath)
    return ai.NewRunResult(proc, out, err, func() string {
        return a.base.Extract(logPath)
    })
}

func (a *engineAdapter) CollectTokenUsage(ctx context.Context, sessionID string) ([]ai.TokenUsage, error) {
    return a.base.Collect(ctx, sessionID)
}
```

Shared behavior (binding `Extract` to `logPath`, delegating `Collect`) lives on the unexported `engineAdapter` and is reached only through `NewEngineAdapter`.

### 2. Each engine — single `Backend` type in `backend.go`

For every engine in `internal/ai/engine/{claude,codex,pi,kimi}`:

- Merge the three structs (`Invoker`, `Collector`, `Extractor`) into one struct named `Backend` defined in a new file `backend.go` inside the engine package.
- Methods on `Backend`: `Run`, `Collect`, `Extract` — bodies are the existing implementations moved verbatim from `invoker.go` (Run, Extract) and `token_usage.go` (Collect).
- `NewBackend(...)` replaces the three former constructors (`NewInvoker`, `NewCollector`, plus the implicit `Extractor{}`).
- Delete `invoker.go`'s `Invoker` and `Extractor` types and `token_usage.go`'s `Collector` type after migration. Helper functions (e.g. `extractSentMsg`, `heredocRe`, parsing helpers) that are still used move into `backend.go` alongside `Backend`.

Engine fields collapse to whatever is actually shared. Example (codex):

```go
type Backend struct {
    binary  string
    baseEnv []string
    store   *SessionStore
}

func NewBackend(binary string, store *SessionStore, extraEnv map[string]string) *Backend {
    return &Backend{
        binary:  binary,
        baseEnv: core.NewBaseEnv(extraEnv),
        store:   store,
    }
}

func (b *Backend) Run(ctx context.Context, workDir, prompt string,
    opts ai.RunOptions, logPath string) (ai.Process, <-chan ai.Output, error) { /* moved from Invoker.Run */ }

func (b *Backend) Collect(ctx context.Context, sessionID string) ([]ai.TokenUsage, error) { /* moved from Collector.Collect */ }

func (b *Backend) Extract(logPath string) string { /* moved from Extractor.Extract */ }
```

### 3. Each engine — `NewAdapter` simplification

`NewAdapter` becomes a thin shell:

```go
// codex
func NewAdapter(binaryPath string, extraEnv map[string]string) (ai.EngineAdapter, error) {
    store, err := NewSessionStore()
    if err != nil {
        return nil, fmt.Errorf("init codex session store: %w", err)
    }
    return core.NewEngineAdapter(NewBackend(binaryPath, store, extraEnv)), nil
}
```

```go
// kimi / pi (no extra state)
func NewAdapter(binaryPath string, extraEnv map[string]string) ai.EngineAdapter {
    return core.NewEngineAdapter(NewBackend(binaryPath, extraEnv))
}
```

### 4. Claude — fold cleanup into `Backend.Run`, delete `claudeAdapter`

Currently `claude/adapter.go` defines:

```go
type claudeAdapter struct{ *core.BaseAdapter }

func (a *claudeAdapter) Run(...) (ai.RunResult, error) {
    if err := cleanupLegacyRules(workDir); err != nil { return ai.RunResult{}, err }
    return a.BaseAdapter.Run(...)
}
```

After the refactor there is no `BaseAdapter struct` to embed and override. The legacy-rules cleanup moves into `claude.Backend.Run`:

```go
func (b *Backend) Run(ctx context.Context, workDir, prompt string,
    opts ai.RunOptions, logPath string) (ai.Process, <-chan ai.Output, error) {
    if err := cleanupLegacyRules(workDir); err != nil {
        return nil, nil, err
    }
    // existing Invoker.Run body
}
```

`cleanupLegacyRules` and `removeImportLine` stay where they are (or move into `backend.go` alongside `Backend.Run`). `claudeAdapter` is deleted entirely, and `claude.NewAdapter` becomes the same shape as the other engines:

```go
func NewAdapter(binaryPath string, extraEnv map[string]string) ai.EngineAdapter {
    return core.NewEngineAdapter(NewBackend(binaryPath, extraEnv))
}
```

Run-signature change: `claude.Backend.Run` must now return `(nil, nil, err)` from cleanup failure rather than the old `(ai.RunResult{}, err)` shape, because it implements the core-level signature now, not the ai-level one.

### 5. Tests

- `internal/ai/core/adapter_test.go`: replace `fakeInvoker` + `fakeCollector` + `fakeExtractor` (wired into a `BaseAdapter struct`) with a single `fakeBackend` that implements all three methods. Wrap it with `core.NewEngineAdapter` and assert the same three things: logPath binding into `ExtractResult`, error propagation from `Run`, and delegation of `CollectTokenUsage` to `Collect`.
- Engine `invoker_test.go` / `token_usage_test.go`: method signatures and behavior are unchanged. Tests construct `Backend` via `NewBackend(...)` instead of `NewInvoker(...)` / `NewCollector(...)`; assertions on `Run` / `Collect` / `Extract` outputs stay identical.
- `claude` cleanup tests: switch from invoking the override on `claudeAdapter.Run` to invoking `claude.Backend.Run`; assertion targets are unchanged (file removed, import line stripped).

## Files Touched

- `internal/ai/core/adapter.go` — rewritten.
- `internal/ai/core/adapter_test.go` — updated test fakes.
- `internal/ai/engine/claude/{invoker.go, token_usage.go, adapter.go}` — invoker.go + token_usage.go content moves to a new `backend.go`; `adapter.go` keeps only `init()` + `NewAdapter` (claudeAdapter deleted).
- `internal/ai/engine/codex/{invoker.go, token_usage.go, adapter.go}` — same migration.
- `internal/ai/engine/pi/{invoker.go, token_usage.go, adapter.go}` — same migration.
- `internal/ai/engine/kimi/{invoker.go, token_usage.go, adapter.go}` — same migration.
- Engine test files (`invoker_test.go`, `token_usage_test.go`) — constructor renames + minor type references; assertions unchanged.

## What This Buys

- `core` exposes one unified contract instead of three sibling micro-interfaces.
- Each engine has a single canonical type (`Backend`) instead of three (`Invoker` + `Collector` + `Extractor`, the latter usually an empty struct).
- `NewAdapter` in every engine reduces to one line wrapping `core.NewEngineAdapter(NewBackend(...))`.
- Claude no longer needs an extra adapter wrapper to inject behavior into `Run` — cleanup becomes part of "running claude", which is more honest.

## Risks / Trade-offs

- Loses the option to mix-and-match an `Invoker` from one source with an `Extractor` from another. In practice this has never happened in this codebase, and the merge can be reversed if needed.
- Engine `backend.go` will be larger (Run + Collect + Extract co-located). Currently the three concerns sit in two files (`invoker.go` holds Run+Extract; `token_usage.go` holds Collect). Those two collapse into one `backend.go`; `adapter.go` (init + `NewAdapter`) stays. The boss has approved this consolidation.
- `claude.Backend.Run` now mixes "side-effect cleanup" with "spawn subprocess". This is intentional — it removes the wrapper layer whose only purpose was to inject that cleanup.
