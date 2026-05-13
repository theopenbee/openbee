# Collapse `RunOptions.ExtraArgs` to a single string

**Status:** Draft
**Date:** 2026-05-12
**Branch:** `refactor/internal-ai-cleanup`

## Motivation

`internal/ai` currently exposes four engine-args helpers — `EngineArgsMap`,
`ParseEngineArgs`, `ParseEngineArgsJSON`, `MergeEngineArgs` — so that callers
in the `worker` and `bee` domains can decode JSON config, merge layered values,
and pick the slice for the active engine before handing it to
`EngineAdapter.Run` via `RunOptions.ExtraArgs []string`.

The split forces two unrelated packages to repeat the same five-line
parse/merge/lookup dance and bloats the `ai` package's public surface with
machinery that only ever feeds one consumer. Engine backends already own all
other CLI-argv construction; tokenisation belongs on their side of the fence,
not in the caller.

Goal: shrink the `ai` package's engine-args surface to a single resolver, push
tokenisation into the engine backends, and turn `ExtraArgs` into a single raw
CLI line.

## Non-goals

- Reworking how engine_args are stored (sysconfig key layout, worker DB column
  shape, JSON wire format) — these stay byte-for-byte identical.
- Adding new engine-args features (per-role overrides, env-var interpolation,
  etc.).
- Touching engines other than `claude`, `codex`, `kimi`, `pi`.

## Current state

```go
// internal/ai/contracts.go
type RunOptions struct {
    SessionID string
    Resume    bool
    APIKey    string
    ExtraEnv  []string
    ExtraArgs []string // already-tokenised argv tail
}
```

```go
// internal/ai/factory.go (Section 4)
type EngineArgsMap map[string][]string
func ParseEngineArgs(raw map[string]string) (EngineArgsMap, error)
func ParseEngineArgsJSON(value string) EngineArgsMap          // silent on bad JSON
func MergeEngineArgs(base, override EngineArgsMap) EngineArgsMap
func splitCLIArgs(s string) ([]string, error)                 // unexported
```

Both callers run the same pipeline:

```go
// internal/domain/bee/bee_process.go (Run)
opts.ExtraArgs = p.resolveEngineArgs(ctx)

func (p *BeeProcess) resolveEngineArgs(ctx context.Context) []string {
    engineName := p.engineCfg.Get()
    globalMap  := p.loadEngineArgs(ctx, model.SystemConfigKeyEngineArgsGlobal)
    beeMap     := p.loadEngineArgs(ctx, model.SystemConfigKeyEngineArgsBee)
    merged     := ai.MergeEngineArgs(globalMap, beeMap)
    return merged[engineName]
}
```

```go
// internal/domain/worker/manager.go
func (m *Manager) resolveEngineArgs(ctx context.Context, worker model.Worker, engineName string) []string {
    globalMap := m.loadEngineArgs(ctx, model.SystemConfigKeyEngineArgsGlobal)
    workerMap := ai.ParseEngineArgsJSON(worker.EngineArgs)
    merged    := ai.MergeEngineArgs(globalMap, workerMap)
    return merged[engineName]
}

func (m *Manager) ValidateEngineArgs(raw map[string]string) error {
    // ...per-engine name checks...
    if _, err := ai.ParseEngineArgs(raw); err != nil { ... }
}
```

Engine backends append the slice directly:

```go
// claude/backend.go
args = append(args, opts.ExtraArgs...)
// codex/kimi/pi
args := buildArgs(..., opts.ExtraArgs)
```

## Design

### 1. Public `ai` surface

`RunOptions.ExtraArgs` becomes a single string — the raw CLI line for the
engine selected at call time:

```go
// internal/ai/contracts.go
type RunOptions struct {
    SessionID string
    Resume    bool
    APIKey    string
    ExtraEnv  []string
    ExtraArgs string // raw CLI tail; engine tokenises internally
}
```

`internal/ai/factory.go` Section 4 collapses to two functions:

```go
// ResolveExtraArgs merges any number of engine_args JSON layers and returns
// the raw CLI line for engineName. Each layer is JSON shaped as
// {"<engine>": "<cli line>", ...}. Empty layers ("", "{}") are skipped.
// A malformed JSON layer is skipped silently (matches the pre-refactor
// behaviour of ParseEngineArgsJSON; bad configs do not block runs).
// Merge order: layers are concatenated in the order given, with a single
// space separator, so later layers append CLI flags after earlier ones —
// the same base+override semantics as the previous MergeEngineArgs.
// Returns "" when no layer contributes a value for engineName.
func ResolveExtraArgs(engineName string, layers ...string) string

// ValidateExtraArgs returns nil if s tokenises cleanly under the standard
// CLI lexer (single/double quotes, backslash escapes). Used at config
// ingestion to surface user typos before they hit a running engine.
func ValidateExtraArgs(s string) error
```

**Removed:** `EngineArgsMap`, `ParseEngineArgs`, `ParseEngineArgsJSON`,
`MergeEngineArgs`.

**Errors:** `ResolveExtraArgs` is intentionally lossy on bad JSON (matches
existing behaviour, avoids fail-closed at runtime when stored config is
corrupt). Strict validation is the job of `ValidateExtraArgs` at the
ingestion path.

### 2. Tokeniser lives in `internal/ai/core`

The current private `splitCLIArgs` in `factory.go` moves to
`internal/ai/core` and is exported as:

```go
// internal/ai/core/cli_args.go
func SplitArgs(s string) ([]string, error)
```

Why `core` and not `ai`: every engine backend (`claude`, `codex`, `kimi`,
`pi`) already imports `internal/ai/core` for `SubprocessSpec` and
`BuildRunEnv`. Putting `SplitArgs` there means engines tokenise without
adding a new dep and without `ai` re-exporting a low-level helper. Domain
packages (`worker`, `bee`) do not import `core` and never need to.

`ai.ValidateExtraArgs` delegates internally to `core.SplitArgs`, so the
parser implementation lives in exactly one place.

### 3. Engine backends tokenise locally

All four engines apply the same mechanical change. Tokenisation errors
surface as `Run` errors:

```go
// internal/ai/engine/claude/backend.go
extra, err := core.SplitArgs(opts.ExtraArgs)
if err != nil {
    return nil, nil, fmt.Errorf("parse extra args: %w", err)
}
// ...existing arg assembly...
args = append(args, extra...)
```

For `codex`, `kimi`, `pi` the shape is `buildArgs(..., extra)`; the tokenise
step happens just before the `buildArgs` call, with the same error return.

### 4. Callers collapse

**`internal/domain/bee/bee_process.go`** drops `resolveEngineArgs` and
`loadEngineArgs`. The `Run` method reads both sysconfig layers inline and
hands them to `ai.ResolveExtraArgs`:

```go
globalJSON := p.readSysConfig(ctx, model.SystemConfigKeyEngineArgsGlobal)
beeJSON    := p.readSysConfig(ctx, model.SystemConfigKeyEngineArgsBee)
opts.ExtraArgs = ai.ResolveExtraArgs(p.engineCfg.Get(), globalJSON, beeJSON)
```

`readSysConfig` is a tiny private helper that returns `cfg.Value` or `""`
on miss/error — replaces today's `loadEngineArgs` (which was already a
near-identical wrapper).

**`internal/domain/worker/manager.go`** mirrors the shape:

```go
func (m *Manager) resolveEngineArgs(ctx context.Context, worker model.Worker, engineName string) string {
    globalJSON := m.readSysConfig(ctx, model.SystemConfigKeyEngineArgsGlobal)
    return ai.ResolveExtraArgs(engineName, globalJSON, worker.EngineArgs)
}

func (m *Manager) ValidateEngineArgs(raw map[string]string) error {
    for engine, line := range raw {
        if engine == "" {
            return fmt.Errorf("engine_args contains an empty engine name: %w", ErrValidation)
        }
        if err := m.ValidateEngine(engine); err != nil {
            return fmt.Errorf("engine_args[%q]: %w", engine, err)
        }
        if err := ai.ValidateExtraArgs(line); err != nil {
            return fmt.Errorf("engine_args[%q]: %w", engine, err)
        }
    }
    return nil
}
```

`execution.go` updates the field assignment:

```go
ExtraArgs: m.resolveEngineArgs(ctx, worker, engineName), // now a string
```

## Affected files

| File | Change |
|---|---|
| `internal/ai/contracts.go` | `ExtraArgs []string` → `string`. |
| `internal/ai/factory.go` | Replace Section 4 with `ResolveExtraArgs` + `ValidateExtraArgs`; delete `EngineArgsMap`, `ParseEngineArgs`, `ParseEngineArgsJSON`, `MergeEngineArgs`, `splitCLIArgs`. |
| `internal/ai/core/cli_args.go` | **New.** Holds `SplitArgs` (moved from `factory.go`). |
| `internal/ai/engine/claude/backend.go` | Tokenise `opts.ExtraArgs` via `core.SplitArgs`. |
| `internal/ai/engine/codex/backend.go` | Same. |
| `internal/ai/engine/kimi/backend.go` | Same. |
| `internal/ai/engine/pi/backend.go` | Same. |
| `internal/domain/bee/bee_process.go` | Inline resolution; delete `resolveEngineArgs`, `loadEngineArgs`. |
| `internal/domain/worker/manager.go` | Inline resolution; rewrite `ValidateEngineArgs` against `ai.ValidateExtraArgs`. |
| `internal/domain/worker/execution.go` | `ExtraArgs` assignment now takes a string. |
| `internal/ai/factory_test.go` | Replace `TestParseEngineArgs_*` / `TestMergeEngineArgs_*` with `TestResolveExtraArgs_*` and `TestValidateExtraArgs_*`. |
| `internal/ai/core/cli_args_test.go` | **New.** Table-driven tests for `SplitArgs` (port the quoting / dup-flag / unterminated-quote / empty-quoted-value cases). |

## Test plan

`internal/ai/core/cli_args_test.go` — table-driven cases ported verbatim
from the existing `factory_test.go` quoting / dup-flag / unterminated-quote
/ empty-quoted-value cases.

`internal/ai/factory_test.go`:

- `TestResolveExtraArgs_SingleLayer` — one layer, one engine, returns its
  CLI line untouched.
- `TestResolveExtraArgs_MergesLayersInOrder` — global + override; result
  is `"<global> <override>"` (single space).
- `TestResolveExtraArgs_MissingEngineReturnsEmpty` — engine absent from
  all layers.
- `TestResolveExtraArgs_SkipsEmptyLayers` — `""` and `"{}"` are no-ops.
- `TestResolveExtraArgs_SkipsMalformedJSON` — malformed layer is ignored,
  later layer still applied (locks the lenient choice into a regression
  test).
- `TestResolveExtraArgs_PreservesQuoting` — round-trips a value containing
  spaces and quoted substrings (verifies we are not re-tokenising before
  hand-off).
- `TestValidateExtraArgs_UnterminatedQuote` — surfaces the lexer error.
- `TestValidateExtraArgs_OK` — happy path passes.

Manager-level: keep the existing `ValidateEngineArgs` tests; update their
expectations only if a fixture exercises one of the removed helpers
directly.

## Behaviour notes / risk

- **Wire format unchanged.** Stored JSON for global/scoped engine_args is
  read the same way; only the internal data flow differs.
- **Lenient bad-JSON behaviour preserved.** A corrupted sysconfig row
  cannot brick a Bee or Worker run — the layer is dropped, others still
  apply. `ValidateExtraArgs` is the place to catch typos.
- **Merge semantics preserved.** Concatenation with a single space is
  equivalent to the previous slice-append after tokenisation, because the
  CLI lexer treats whitespace as the token separator. The only edge case
  is a layer ending with an unterminated quote — but that is exactly the
  case `ValidateExtraArgs` rejects at ingestion, so it should not appear
  at runtime.
- **No public API beyond the `ai` package boundary changes for the wire
  format**; the breaking change is purely the `ai` package's Go API. Both
  in-tree callers are updated in the same commit chain.
