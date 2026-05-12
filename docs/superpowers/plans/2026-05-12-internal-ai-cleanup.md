# `internal/ai/` Cleanup Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Collapse `internal/ai/` from 19 top-level Go files into `ai.go` + `core/` + `engine/{claude,codex,kimi,pi}/` with no behavior change.

**Architecture:** Three-layer split — `ai.go` holds the public package surface (contracts, registry, dynamic routing, helpers); `core/` is a sibling package with the shared subprocess/usage infrastructure that engines embed; `engine/*` are the concrete engines (moved from `internal/ai/{claude,codex,kimi,pi}`). The `ai` package never imports `core`; `core` and `engine/*` both import `ai`; `engine/*` also imports `core`. No public type/method on the `ai` package surface changes name or signature, so all external `ai.XXX` callers compile unchanged.

**Tech Stack:** Go 1.22+, `go build` / `go test`, `git mv` for file moves.

**Spec:** `docs/superpowers/specs/2026-05-12-internal-ai-cleanup-design.md`

---

## Pre-flight

- [ ] **Step 0.1: Confirm starting state**

Run:
```bash
git status
git rev-parse --abbrev-ref HEAD
```
Expected: clean working tree, branch `refactor/internal-ai-cleanup`.

- [ ] **Step 0.2: Baseline build + test**

Run:
```bash
go build ./...
go test ./internal/ai/...
```
Expected: both succeed. If anything is already broken on `main`, stop and surface it.

---

## Task 1: Move engine subdirs into `internal/ai/engine/`

**Goal:** Physical move of the 4 engine packages from `internal/ai/{claude,codex,kimi,pi}` into `internal/ai/engine/{claude,codex,kimi,pi}`. Update all import paths. No symbol renames in this task.

**Files:**
- Move: `internal/ai/{claude,codex,kimi,pi}/` → `internal/ai/engine/{claude,codex,kimi,pi}/` (entire directories)
- Modify (external caller imports): `cmd/openbee/claude.go`, `cmd/openbee/config_claude.go`, `cmd/openbee/config.go`, `internal/app/app.go`
- Modify (in-engine test self-imports): `internal/ai/engine/claude/adapter_test.go`, `internal/ai/engine/claude/token_usage_test.go`, `internal/ai/engine/codex/adapter_test.go`, `internal/ai/engine/codex/token_usage_test.go`, `internal/ai/engine/kimi/adapter_test.go`, `internal/ai/engine/kimi/token_usage_test.go`, `internal/ai/engine/pi/adapter_test.go`, `internal/ai/engine/pi/token_usage_test.go`

- [ ] **Step 1.1: Create the `engine/` parent**

Run:
```bash
mkdir -p internal/ai/engine
```
Expected: directory exists.

- [ ] **Step 1.2: `git mv` each engine subdir**

Run (one at a time so each rename is clearly tracked):
```bash
git mv internal/ai/claude internal/ai/engine/claude
git mv internal/ai/codex  internal/ai/engine/codex
git mv internal/ai/kimi   internal/ai/engine/kimi
git mv internal/ai/pi     internal/ai/engine/pi
```
Expected: `git status` shows 4 rename operations covering ~40 files.

- [ ] **Step 1.3: Update external caller imports in `cmd/openbee/`**

Apply this exact replacement across the 3 files in `cmd/openbee/`:

`cmd/openbee/claude.go` line 10:
```go
// before
	claude "github.com/theopenbee/openbee/internal/ai/claude"
// after
	claude "github.com/theopenbee/openbee/internal/ai/engine/claude"
```

`cmd/openbee/config_claude.go` line 8:
```go
// before
	claude "github.com/theopenbee/openbee/internal/ai/claude"
// after
	claude "github.com/theopenbee/openbee/internal/ai/engine/claude"
```

`cmd/openbee/config.go` line 18:
```go
// before
	"github.com/theopenbee/openbee/internal/ai/claude"
// after
	"github.com/theopenbee/openbee/internal/ai/engine/claude"
```

- [ ] **Step 1.4: Update `internal/app/app.go` blank imports**

Lines 21–24:
```go
// before
	_ "github.com/theopenbee/openbee/internal/ai/claude"
	_ "github.com/theopenbee/openbee/internal/ai/codex"
	_ "github.com/theopenbee/openbee/internal/ai/kimi"
	_ "github.com/theopenbee/openbee/internal/ai/pi"
// after
	_ "github.com/theopenbee/openbee/internal/ai/engine/claude"
	_ "github.com/theopenbee/openbee/internal/ai/engine/codex"
	_ "github.com/theopenbee/openbee/internal/ai/engine/kimi"
	_ "github.com/theopenbee/openbee/internal/ai/engine/pi"
```

- [ ] **Step 1.5: Update in-engine test files that import their own package by full path**

Eight files use external test packages (`package xxx_test`) that import their own package by full path. Rewrite each line:

| File | Replace |
|------|---------|
| `internal/ai/engine/claude/adapter_test.go` line 10 | `"github.com/theopenbee/openbee/internal/ai/claude"` → `"github.com/theopenbee/openbee/internal/ai/engine/claude"` |
| `internal/ai/engine/claude/token_usage_test.go` line 11 | same pattern |
| `internal/ai/engine/codex/adapter_test.go` line 9 | `.../internal/ai/codex` → `.../internal/ai/engine/codex` |
| `internal/ai/engine/codex/token_usage_test.go` line 11 | same |
| `internal/ai/engine/kimi/adapter_test.go` line 8 | `.../internal/ai/kimi` → `.../internal/ai/engine/kimi` |
| `internal/ai/engine/kimi/token_usage_test.go` line 11 | same |
| `internal/ai/engine/pi/adapter_test.go` line 8 | `.../internal/ai/pi` → `.../internal/ai/engine/pi` |
| `internal/ai/engine/pi/token_usage_test.go` line 11 | same |

Sanity check: use the Grep tool with pattern `"github.com/theopenbee/openbee/internal/ai/(claude|codex|kimi|pi)"`. Expected: 0 matches outside `docs/`.

- [ ] **Step 1.6: Build**

Run:
```bash
go build ./...
```
Expected: succeeds.

- [ ] **Step 1.7: Test**

Run:
```bash
go test ./...
```
Expected: succeeds.

- [ ] **Step 1.8: Commit**

```bash
git add -A
git commit -m "$(cat <<'EOF'
refactor(ai): move engine subdirs under internal/ai/engine/

Pure path move with import-path updates across cmd/openbee/ and
internal/app/. Engine package names unchanged. No symbol renames.
EOF
)"
```

---

## Task 2: Extract shared infrastructure into `internal/ai/core/`

**Goal:** Move `base_adapter.go`, `process*.go`, `spawn.go`, `usage.go` (plus tests) into a new `core` package. Rename `EngineInvoker → Invoker` and `EngineCollector → Collector` (the `Engine` prefix is redundant inside the core package). Update all `engine/*` adapters to import `core` and use `core.XXX` for the moved symbols. The `ai` package shrinks but every type it still exports keeps its current name.

**Files:**
- Move + edit: `internal/ai/base_adapter.go` → `internal/ai/core/adapter.go`
- Move + edit: `internal/ai/process.go` → `internal/ai/core/process.go`
- Move: `internal/ai/process_unix.go` → `internal/ai/core/process_unix.go`
- Move: `internal/ai/process_windows.go` → `internal/ai/core/process_windows.go`
- Move + edit: `internal/ai/spawn.go` → `internal/ai/core/spawn.go`
- Move + edit: `internal/ai/usage.go` → `internal/ai/core/usage.go`
- Move + edit: `internal/ai/base_adapter_test.go` → `internal/ai/core/adapter_test.go`
- Move: `internal/ai/process_test.go` → `internal/ai/core/process_test.go`
- Move + edit: `internal/ai/spawn_test.go` → `internal/ai/core/spawn_test.go`
- Move + edit: `internal/ai/usage_test.go` → `internal/ai/core/usage_test.go`
- Modify (engine consumers): `internal/ai/engine/claude/adapter.go`, `internal/ai/engine/claude/invoker.go`, `internal/ai/engine/claude/token_usage.go`, `internal/ai/engine/codex/adapter.go`, `internal/ai/engine/codex/invoker.go`, `internal/ai/engine/codex/token_usage.go`, `internal/ai/engine/kimi/adapter.go`, `internal/ai/engine/kimi/invoker.go`, `internal/ai/engine/kimi/token_usage.go`, `internal/ai/engine/pi/adapter.go`, `internal/ai/engine/pi/invoker.go`, `internal/ai/engine/pi/token_usage.go`

- [ ] **Step 2.1: Create the `core/` directory**

```bash
mkdir -p internal/ai/core
```

- [ ] **Step 2.2: Move `base_adapter.go` and rewrite as `core/adapter.go`**

```bash
git mv internal/ai/base_adapter.go internal/ai/core/adapter.go
```

Replace the entire file contents with:

```go
package core

import (
	"context"

	"github.com/theopenbee/openbee/internal/ai"
)

// Invoker is the minimal subprocess launcher contract every engine implements.
type Invoker interface {
	Run(ctx context.Context, workDir, prompt string, opts ai.RunOptions, logPath string) (ai.Process, <-chan ai.Output, error)
}

// Collector is the minimal token-usage reader contract.
type Collector interface {
	Collect(ctx context.Context, sessionID string) ([]ai.TokenUsage, error)
}

// BaseAdapter implements the EngineAdapter parts that are identical across
// engines: Run wires the invoker output into a RunResult with a bound result
// extractor; CollectTokenUsage delegates to the collector. Engines embed
// BaseAdapter and optionally override Prepare.
type BaseAdapter struct {
	Invoker   Invoker
	Collector Collector
	// Extract is the per-engine result extractor bound to logPath in Run.
	Extract func(logPath string) string
}

// Run launches the invoker and binds Extract to logPath in the returned RunResult.
func (b *BaseAdapter) Run(ctx context.Context, workDir, prompt string,
	opts ai.RunOptions, logPath string) (ai.RunResult, error) {
	proc, out, err := b.Invoker.Run(ctx, workDir, prompt, opts, logPath)
	return ai.NewRunResult(proc, out, err, func() string { return b.Extract(logPath) })
}

// CollectTokenUsage delegates to the embedded collector.
func (b *BaseAdapter) CollectTokenUsage(ctx context.Context, sessionID string) ([]ai.TokenUsage, error) {
	return b.Collector.Collect(ctx, sessionID)
}

// Prepare is a no-op default that engines may override (e.g. claude).
func (b *BaseAdapter) Prepare(string, ai.PrepareOptions) error { return nil }
```

- [ ] **Step 2.3: Move process source files (no edits to unix/windows)**

```bash
git mv internal/ai/process.go         internal/ai/core/process.go
git mv internal/ai/process_unix.go    internal/ai/core/process_unix.go
git mv internal/ai/process_windows.go internal/ai/core/process_windows.go
```

In `internal/ai/core/process.go`, change the package line:
```go
// before
package ai
// after
package core
```

(The platform files `process_unix.go` and `process_windows.go` only contain private helpers; change their package lines from `package ai` to `package core` too.)

- [ ] **Step 2.4: Move `spawn.go` and rewrite as `core/spawn.go`**

```bash
git mv internal/ai/spawn.go internal/ai/core/spawn.go
```

In the moved file, change:

```go
// before
package ai
// after
package core

import (
	// ... existing imports ...
	"github.com/theopenbee/openbee/internal/ai"
)
```

Then within the file, replace every bare type/function reference that is now in the `ai` package with the `ai.` prefix. From the current file the relevant references are:
- `Output` → `ai.Output`
- `OutputDone` → `ai.OutputDone`
- `OutputError` → `ai.OutputError`
- `Process` (return type) → `ai.Process`
- `NewCmdProcess(...)` stays unqualified (CmdProcess is in this same `core` package).

Inspect the file after editing and confirm no symbol from the original `ai` package is referenced without an `ai.` prefix.

- [ ] **Step 2.5: Move `usage.go` and rewrite as `core/usage.go`**

```bash
git mv internal/ai/usage.go internal/ai/core/usage.go
```

In the moved file, change `package ai` → `package core`, add `ai` import, and qualify type references:

```go
package core

import (
	"encoding/json"

	"github.com/theopenbee/openbee/internal/ai"
	"github.com/theopenbee/openbee/internal/utils/sessionfile"
)

// AggregateUsage scans a JSONL file at path, unmarshals each line as T, and
// lets fold accumulate per-model TokenUsage into agg. Lines that fail to
// unmarshal are silently skipped (matches existing per-engine behavior).
// The returned slice ordering is unspecified.
func AggregateUsage[T any](path string, fold func(line T, agg map[string]*ai.TokenUsage)) ([]ai.TokenUsage, error) {
	agg := map[string]*ai.TokenUsage{}
	err := sessionfile.ScanJSONLFile(path, func(data []byte) {
		var line T
		if json.Unmarshal(data, &line) != nil {
			return
		}
		fold(line, agg)
	})
	if err != nil {
		return nil, err
	}
	out := make([]ai.TokenUsage, 0, len(agg))
	for _, u := range agg {
		out = append(out, *u)
	}
	return out, nil
}
```

(Adjust to match the exact body of the original `usage.go` — keep behavior identical; only the package and the `ai.` qualifications change.)

- [ ] **Step 2.6: Move tests to `core/` and rewrite imports/qualifications**

```bash
git mv internal/ai/base_adapter_test.go internal/ai/core/adapter_test.go
git mv internal/ai/process_test.go      internal/ai/core/process_test.go
git mv internal/ai/spawn_test.go        internal/ai/core/spawn_test.go
git mv internal/ai/usage_test.go        internal/ai/core/usage_test.go
```

For each moved test file:
1. Change the package line: `package ai_test` → `package core_test` (or `package ai` → `package core` if it was an internal test).
2. The existing import alias `ai "github.com/theopenbee/openbee/internal/ai"` stays — `ai.RunOptions`, `ai.Output`, `ai.Process`, `ai.TokenUsage`, `ai.PrepareOptions`, etc. remain valid because those types stay in the `ai` package.
3. Add `core "github.com/theopenbee/openbee/internal/ai/core"` import.
4. Replace references to the moved symbols:
   - `ai.BaseAdapter` → `core.BaseAdapter`
   - `ai.SubprocessSpec` → `core.SubprocessSpec`
   - `ai.SpawnSubprocess` → `core.SpawnSubprocess`
   - `ai.CmdProcess` / `ai.NewCmdProcess` → `core.CmdProcess` / `core.NewCmdProcess`
   - `ai.AggregateUsage` → `core.AggregateUsage`
   - For `adapter_test.go`: `ai.RunOptions` stays (it's in `ai`), but `ai.BaseAdapter`, the `Invoker`/`Collector` interface references, become `core.BaseAdapter` / `core.Invoker` / `core.Collector`.

- [ ] **Step 2.7: Update `engine/claude/adapter.go`**

Imports: keep the `ai` alias, add `core`:
```go
import (
	// ...
	ai   "github.com/theopenbee/openbee/internal/ai"
	core "github.com/theopenbee/openbee/internal/ai/core"
)
```

Replace within the file body:
- `*ai.BaseAdapter` → `*core.BaseAdapter` (in the `claudeAdapter` struct field and the `NewAdapter` literal).

(`ai.Register`, `ai.EngineClaude`, `ai.EngineAdapter`, `ai.EngineConfig`, `ai.RunOptions`, `ai.RunResult`, `ai.PrepareOptions` stay unqualified-to-`ai`.)

- [ ] **Step 2.8: Update `engine/claude/invoker.go`**

Replace within the file body:
- `ai.SubprocessSpec` → `core.SubprocessSpec`
- `ai.SpawnSubprocess` → `core.SpawnSubprocess`

Add `core "github.com/theopenbee/openbee/internal/ai/core"` to the import block.

- [ ] **Step 2.9: Update `engine/claude/token_usage.go`**

Replace:
- `ai.AggregateUsage` → `core.AggregateUsage`

Add `core` import. (`ai.TokenUsage` stays unqualified-to-`ai`.)

- [ ] **Step 2.10: Update `engine/codex/adapter.go`**

Same pattern as Step 2.7:
- `*ai.BaseAdapter` → `*core.BaseAdapter` (in the `NewAdapter` return literal).
- Add `core` import.

- [ ] **Step 2.11: Update `engine/codex/invoker.go`**

Replace:
- `ai.NewCmdProcess` → `core.NewCmdProcess`

Add `core` import.

- [ ] **Step 2.12: Update `engine/codex/token_usage.go`**

Replace:
- `ai.AggregateUsage` → `core.AggregateUsage`

Add `core` import.

- [ ] **Step 2.13: Update `engine/kimi/adapter.go`**

Replace:
- `*ai.BaseAdapter` / `&ai.BaseAdapter{` → use `core.BaseAdapter`.

Add `core` import.

- [ ] **Step 2.14: Update `engine/kimi/invoker.go`**

Replace:
- `ai.SubprocessSpec` → `core.SubprocessSpec`
- `ai.SpawnSubprocess` → `core.SpawnSubprocess`

Add `core` import.

- [ ] **Step 2.15: Update `engine/kimi/token_usage.go`**

Replace:
- `ai.AggregateUsage` → `core.AggregateUsage`

Add `core` import.

- [ ] **Step 2.16: Update `engine/pi/adapter.go`**

Replace:
- `&ai.BaseAdapter{` → use `core.BaseAdapter`.

Add `core` import.

- [ ] **Step 2.17: Update `engine/pi/invoker.go`**

Replace:
- `ai.NewCmdProcess` → `core.NewCmdProcess`

Add `core` import.

- [ ] **Step 2.18: Update `engine/pi/token_usage.go`**

Replace:
- `ai.AggregateUsage` → `core.AggregateUsage`

Add `core` import.

- [ ] **Step 2.19: Grep guard — no `ai.BaseAdapter` / `ai.SpawnSubprocess` / etc. should remain**

Use the Grep tool with pattern `\bai\.(BaseAdapter|EngineInvoker|EngineCollector|CmdProcess|NewCmdProcess|SpawnSubprocess|SubprocessSpec|AggregateUsage)\b`. Expected: 0 matches outside the spec/plan documents.

- [ ] **Step 2.20: Build**

```bash
go build ./...
```
Expected: succeeds. If you see `undefined: core.XXX` errors, check that the `core` import was added to that file. If you see `undefined: ai.BaseAdapter` errors, you missed a rename — go back to Step 2.19 and rerun.

- [ ] **Step 2.21: Test**

```bash
go test ./...
```
Expected: succeeds.

- [ ] **Step 2.22: Commit**

```bash
git add -A
git commit -m "$(cat <<'EOF'
refactor(ai): extract shared infra into internal/ai/core/

Moves BaseAdapter, CmdProcess, SpawnSubprocess, AggregateUsage and
related helpers/tests into a new core package. Renames Engine{Invoker,
Collector} -> {Invoker, Collector} inside core. All engine/* adapters
updated to use core.XXX. The ai package surface is unchanged.
EOF
)"
```

---

## Task 3: Merge top-level files into `internal/ai/ai.go`

**Goal:** Consolidate `contracts.go`, `registry.go`, `dynamic.go`, `prompt.go`, `engine_args.go` into a single `ai.go` with section-comment dividers. Existing tests stay where they are.

**Files:**
- Create: `internal/ai/ai.go`
- Delete: `internal/ai/contracts.go`, `internal/ai/registry.go`, `internal/ai/dynamic.go`, `internal/ai/prompt.go`, `internal/ai/engine_args.go`
- Keep as-is: all 5 corresponding `*_test.go` files (`contracts` has no test; `registry_test.go`, `dynamic_test.go`, `prompt_test.go`, `engine_args_test.go` remain).

- [ ] **Step 3.1: Write the new `ai.go`**

Create `internal/ai/ai.go` with this exact content. All function/method bodies are inlined verbatim from the 5 source files — no placeholders.

```go
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
```

After writing the file, do a quick diff sanity check:

```bash
# These should be identical (or empty) — confirms no body was lost.
diff <(awk '/^func |^type |^var |^const /' internal/ai/ai.go | sort) \
     <(awk '/^func |^type |^var |^const /' internal/ai/contracts.go internal/ai/registry.go internal/ai/dynamic.go internal/ai/prompt.go internal/ai/engine_args.go | sort)
```
Expected: identical top-level decls in both sides. Any one-sided line is a missed copy — go back and add it.

- [ ] **Step 3.2: Delete the 5 source files**

```bash
git rm internal/ai/contracts.go
git rm internal/ai/registry.go
git rm internal/ai/dynamic.go
git rm internal/ai/prompt.go
git rm internal/ai/engine_args.go
```

- [ ] **Step 3.3: Build**

```bash
go build ./...
```
Expected: succeeds. If you see `undefined: ...` errors, you missed copying a helper or const from one of the source files; open the corresponding section in `ai.go` and add it.

- [ ] **Step 3.4: Test**

```bash
go test ./...
```
Expected: succeeds, including all 4 retained `*_test.go` files at the top of `internal/ai/`.

- [ ] **Step 3.5: Verify final directory shape**

Use the Glob tool:
- Pattern `internal/ai/*.go` — expected: exactly `ai.go` plus the retained `*_test.go` files (`registry_test.go`, `dynamic_test.go`, `prompt_test.go`, `engine_args_test.go`).
- Pattern `internal/ai/core/*.go` — expected: `adapter.go`, `adapter_test.go`, `process.go`, `process_unix.go`, `process_windows.go`, `process_test.go`, `spawn.go`, `spawn_test.go`, `usage.go`, `usage_test.go`.
- Pattern `internal/ai/engine/*` — expected: directories `claude/`, `codex/`, `kimi/`, `pi/`.

- [ ] **Step 3.6: Commit**

```bash
git add -A
git commit -m "$(cat <<'EOF'
refactor(ai): merge top-level files into ai.go

Combines contracts.go, registry.go, dynamic.go, prompt.go,
engine_args.go into a single ai.go with section-comment dividers.
No public symbol changes; the 4 test files remain untouched.
EOF
)"
```

---

## Task 4: Final verification + smoke test

**Goal:** End-to-end confidence the engine init-registration chain still fires.

- [ ] **Step 4.1: Full build + test once more**

```bash
go build ./...
go test ./...
```
Expected: succeeds.

- [ ] **Step 4.2: Static grep guard — no stale paths/symbols**

Using the Grep tool, run each of these and expect **0** matches outside `docs/`:

| Pattern | Notes |
|---------|-------|
| `"github.com/theopenbee/openbee/internal/ai/(claude\|codex\|kimi\|pi)"` (not under `engine/`) | All callers must use `engine/<x>`. |
| `\bai\.(BaseAdapter\|EngineInvoker\|EngineCollector\|CmdProcess\|NewCmdProcess\|SpawnSubprocess\|SubprocessSpec\|AggregateUsage)\b` | All renamed symbols must now be `core.XXX`. |

- [ ] **Step 4.3: Smoke test — one worker task end-to-end**

Run a minimal worker invocation locally. Suggested approach:
```bash
# Example: run openbee CLI in dry-run / dispatch a trivial worker task.
# Use whatever the project's standard local-run command is.
./scripts/dev.sh  # adjust to whatever invokes a worker locally
```
Look for: a successful engine.Register hit at startup (no `ErrUnknownEngine` for `claude`/`codex`/`kimi`/`pi`) and a clean task completion.

If the project does not have a single-command smoke harness, manually:
1. Start the openbee server.
2. Submit a hello-world task to a `claude` worker (or whichever engine you have credentials for).
3. Verify the task completes without `ai: engine "claude" already registered` or `ErrUnknownEngine`.

- [ ] **Step 4.4: No commit needed**

This task is verification only. If everything passes, the refactor is complete on `refactor/internal-ai-cleanup`.

---

## Out of scope (do NOT do)

- Renaming any public type in the `ai` package (e.g., `EngineAdapter`, `RunOptions`, `TokenUsage` stay).
- Touching engine internals (`adapter.go`/`invoker.go`/`token_usage.go` bodies) beyond the mechanical `ai.X → core.X` rename.
- Adding new test cases. The plan is behavior-preserving.
- Adding a `internal/` layer under `internal/ai/` to enforce compile-time visibility on `core/`. (Spec section 2.1 already accepted the convention-only trade-off.)
- Touching `internal/domain/...` consumers of `ai.ParseEngineArgs`, `ai.EngineArgsMap`, `ai.WorkerPersona` — those symbols stay in the `ai` package and don't change.
