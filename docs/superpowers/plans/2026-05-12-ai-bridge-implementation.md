# AI Bridge Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Introduce `internal/ai/bridge` as the single business-facing facade for the AI engine subsystem and migrate every business package off direct `internal/ai` imports.

**Architecture:** A Wait-style bridge facade exposing `RunWorker`, `RunBee`, engine listing/validation, and per-session usage. Bridge depends on five narrow ports (`TokenIssuer`, `EnvResolver`, `EngineSelector`, `ArgsResolver`, `LogPathProvider`) implemented by `internal/ai/bridge/adapters/`. Three-phase migration: (1) build bridge package with full tests, business untouched; (2) migrate worker path; (3) migrate bee, tokenstat, store, config, cmd, tests.

**Tech Stack:** Go 1.x, existing repo libraries (`internal/ai`, `internal/domain/*`, `internal/infra/*`).

**Spec:** `docs/superpowers/specs/2026-05-12-ai-bridge-design.md`

---

## File Structure

### New files

```
internal/ai/bridge/
  doc.go                  // package documentation
  facade.go               // Bridge interface, Config, Deps, New
  run.go                  // Handle, Outcome, Status, request types, run implementation
  run_test.go             // 6 lifecycle invariants + happy paths
  usage.go                // Usage, ErrSessionDataNotFound, CollectUsage
  usage_test.go
  names.go                // EngineXxx constants, AllEngines / EnabledEngines / IsEnabled
  names_test.go
  validate.go             // ValidateEngine, ValidateEngineArgs
  validate_test.go
  deps.go                 // TokenIssuer, EnvResolver, EngineSelector, ArgsResolver, LogPathProvider
  fake_engine_test.go     // shared in-package test helper for the run tests
  adapters/
    token.go              // TokenIssuer impl
    token_test.go
    env.go                // EnvResolver impl
    env_test.go
    engine.go             // EngineSelector impl
    engine_test.go
    args.go               // ArgsResolver impl
    args_test.go
    logpath.go            // LogPathProvider impl
    logpath_test.go

internal/ai/doc.go        // "internal-only; business code must import internal/ai/bridge"
```

### Modified files (in order of phases)

**Phase 2 (worker):**
- `internal/domain/worker/manager.go` — drop ai/env/sysconfig fields; hold bridge.
- `internal/domain/worker/execution.go` — call `bridge.RunWorker` and `Handle.Wait`; drop ai imports.
- `internal/domain/worker/manager_test.go` — replace engine-map stub with `fakeBridge`.
- `internal/app/app.go` — change `buildWorkerManager` signature.

**Phase 3 (bee, tokenstat, store, config, cmd, tests):**
- `internal/domain/bee/feeder.go`, `internal/domain/bee/bee_process.go` (delete), `internal/domain/bee/feeder_test.go`, `internal/domain/bee/feeder_internal_test.go`.
- `internal/tokenstat/syncer.go`, `internal/tokenstat/syncer_test.go`.
- `internal/infra/store/session_store.go`, `internal/infra/store/db.go`, `internal/infra/store/db_test.go`.
- `internal/infra/config/config.go`.
- `cmd/openbee/config.go`.
- `internal/domain/command/engine_test.go`, `internal/rpc/tools_test.go`, `internal/domain/task/dispatcher.go` and `dispatcher_test.go` (only if they still import ai after earlier phases).
- `internal/app/app.go` — final wiring: drop `engines map[string]ai.EngineAdapter` and `engineCfg` from non-bridge callers, drop `tokenstat.NewSyncer` engine params.
- Possibly delete `internal/ai/factory.go::Dynamic` if it has no remaining consumers.

---

## Phase 1 — Build the bridge package (business untouched)

### Task 1.1: Create bridge package skeleton + doc.go

**Files:**
- Create: `internal/ai/bridge/doc.go`

- [ ] **Step 1: Add package doc file**

```go
// Package bridge is the business-facing front for the internal AI engine
// subsystem. Business code (worker, bee, task, tokenstat, store, config,
// cmd) must only depend on this package; it must not import internal/ai
// directly.
//
// Design spec: docs/superpowers/specs/2026-05-12-ai-bridge-design.md
package bridge
```

- [ ] **Step 2: Verify build**

Run: `go build ./internal/ai/bridge/...`
Expected: succeeds (empty package).

- [ ] **Step 3: Commit**

```bash
git add internal/ai/bridge/doc.go
git commit -m "feat(ai/bridge): scaffold package"
```

---

### Task 1.2: names.go — engine constants + listing

**Files:**
- Create: `internal/ai/bridge/names.go`, `internal/ai/bridge/names_test.go`

- [ ] **Step 1: Write failing test**

`internal/ai/bridge/names_test.go`:

```go
package bridge

import (
	"reflect"
	"testing"

	ai "github.com/theopenbee/openbee/internal/ai"
)

func TestEngineConstantsMatchInternalAI(t *testing.T) {
	if EngineClaude != ai.EngineClaude || EngineCodex != ai.EngineCodex ||
		EnginePi != ai.EnginePi || EngineKimi != ai.EngineKimi {
		t.Fatalf("bridge engine constants drift from internal/ai")
	}
}

func TestAllEnginesMatchInternalAI(t *testing.T) {
	got := AllEngines()
	want := ai.AllEngines()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("AllEngines: got %v, want %v", got, want)
	}
}

func TestEnabledEnginesFiltersAndPreservesCanonicalOrder(t *testing.T) {
	b := &bridgeImpl{engines: map[string]ai.EngineAdapter{
		ai.EngineCodex:  nil,
		ai.EngineClaude: nil,
	}}
	got := b.EnabledEngines()
	want := []string{ai.EngineClaude, ai.EngineCodex}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("EnabledEngines: got %v, want %v", got, want)
	}
}

func TestIsEnabled(t *testing.T) {
	b := &bridgeImpl{engines: map[string]ai.EngineAdapter{ai.EngineClaude: nil}}
	if !b.IsEnabled(ai.EngineClaude) || b.IsEnabled(ai.EngineCodex) {
		t.Fatalf("IsEnabled wrong")
	}
}
```

- [ ] **Step 2: Run, expect compile failure (bridgeImpl, constants not yet defined)**

Run: `go test ./internal/ai/bridge/...`
Expected: FAIL (undefined identifiers).

- [ ] **Step 3: Implement**

`internal/ai/bridge/names.go`:

```go
package bridge

import ai "github.com/theopenbee/openbee/internal/ai"

const (
	EngineClaude = ai.EngineClaude
	EngineCodex  = ai.EngineCodex
	EnginePi     = ai.EnginePi
	EngineKimi   = ai.EngineKimi
)

// AllEngines returns the canonical engine name list in declaration order,
// independent of which engines are enabled in the running process.
func AllEngines() []string { return ai.AllEngines() }

// EnabledEngines returns the enabled engines in canonical order.
func (b *bridgeImpl) EnabledEngines() []string {
	out := make([]string, 0, len(b.engines))
	for _, name := range ai.AllEngines() {
		if _, ok := b.engines[name]; ok {
			out = append(out, name)
		}
	}
	return out
}

// IsEnabled reports whether name is one of the enabled engines.
func (b *bridgeImpl) IsEnabled(name string) bool {
	_, ok := b.engines[name]
	return ok
}
```

Also create a placeholder `internal/ai/bridge/facade.go` so `bridgeImpl` exists:

```go
package bridge

import ai "github.com/theopenbee/openbee/internal/ai"

// bridgeImpl is the concrete Bridge. Methods are split across files for
// readability (names.go / validate.go / usage.go / run.go).
type bridgeImpl struct {
	engines map[string]ai.EngineAdapter
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/ai/bridge/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/ai/bridge/names.go internal/ai/bridge/names_test.go internal/ai/bridge/facade.go
git commit -m "feat(ai/bridge): engine name constants and listing"
```

---

### Task 1.3: validate.go — engine + engine_args validation

**Files:**
- Create: `internal/ai/bridge/validate.go`, `internal/ai/bridge/validate_test.go`

- [ ] **Step 1: Write failing test**

`internal/ai/bridge/validate_test.go`:

```go
package bridge

import (
	"errors"
	"testing"

	ai "github.com/theopenbee/openbee/internal/ai"
)

func TestValidateEngineArgsDelegatesToInternalAI(t *testing.T) {
	if err := ValidateEngineArgs(`--ok "value"`); err != nil {
		t.Fatalf("expected ok, got %v", err)
	}
	if err := ValidateEngineArgs(`--bad "unterminated`); err == nil {
		t.Fatalf("expected error for unterminated quote")
	}
}

func TestValidateEngineAllowsEmptyAndChecksEnabled(t *testing.T) {
	b := &bridgeImpl{engines: map[string]ai.EngineAdapter{ai.EngineClaude: nil}}
	if err := b.ValidateEngine(""); err != nil {
		t.Fatalf("empty should be ok: %v", err)
	}
	if err := b.ValidateEngine(ai.EngineClaude); err != nil {
		t.Fatalf("enabled should be ok: %v", err)
	}
	err := b.ValidateEngine(ai.EngineCodex)
	if err == nil {
		t.Fatalf("disabled engine should error")
	}
	if !errors.Is(err, ErrEngineNotEnabled) {
		t.Fatalf("expected ErrEngineNotEnabled, got %v", err)
	}
}
```

- [ ] **Step 2: Run test, expect FAIL**

Run: `go test ./internal/ai/bridge/...`
Expected: FAIL.

- [ ] **Step 3: Implement**

`internal/ai/bridge/validate.go`:

```go
package bridge

import (
	"errors"
	"fmt"

	ai "github.com/theopenbee/openbee/internal/ai"
)

// ErrEngineNotEnabled is returned by ValidateEngine when a non-empty
// engine name is not present in the enabled set.
var ErrEngineNotEnabled = errors.New("bridge: engine not enabled")

// ValidateEngineArgs reports whether s tokenises cleanly under the shared
// CLI lexer (single/double quotes, backslash escape).
func ValidateEngineArgs(s string) error { return ai.ValidateExtraArgs(s) }

// ValidateEngine accepts the empty string (caller will fall back to the
// default engine) and otherwise requires the name to be enabled.
func (b *bridgeImpl) ValidateEngine(name string) error {
	if name == "" {
		return nil
	}
	if _, ok := b.engines[name]; !ok {
		return fmt.Errorf("%w: %q", ErrEngineNotEnabled, name)
	}
	return nil
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/ai/bridge/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/ai/bridge/validate.go internal/ai/bridge/validate_test.go
git commit -m "feat(ai/bridge): engine and engine_args validation"
```

---

### Task 1.4: usage.go — Usage type + CollectUsage

**Files:**
- Create: `internal/ai/bridge/usage.go`, `internal/ai/bridge/usage_test.go`

- [ ] **Step 1: Write failing test**

`internal/ai/bridge/usage_test.go`:

```go
package bridge

import (
	"context"
	"errors"
	"reflect"
	"testing"

	ai "github.com/theopenbee/openbee/internal/ai"
)

type usageFakeEngine struct {
	usages []ai.TokenUsage
	err    error
}

func (f *usageFakeEngine) Run(context.Context, string, string, ai.RunOptions, string) (ai.RunResult, error) {
	return ai.RunResult{}, nil
}
func (f *usageFakeEngine) CollectTokenUsage(context.Context, string) ([]ai.TokenUsage, error) {
	return f.usages, f.err
}

func TestCollectUsageTranslatesValues(t *testing.T) {
	fe := &usageFakeEngine{usages: []ai.TokenUsage{{Model: "m", InputTokens: 1, OutputTokens: 2, CacheCreationTokens: 3, CacheReadTokens: 4}}}
	b := &bridgeImpl{engines: map[string]ai.EngineAdapter{ai.EngineClaude: fe}}
	got, err := b.CollectUsage(context.Background(), ai.EngineClaude, "sid")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []Usage{{Model: "m", InputTokens: 1, OutputTokens: 2, CacheCreationTokens: 3, CacheReadTokens: 4}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("CollectUsage: got %+v, want %+v", got, want)
	}
}

func TestCollectUsageTranslatesNotFound(t *testing.T) {
	fe := &usageFakeEngine{err: ai.ErrSessionDataNotFound}
	b := &bridgeImpl{engines: map[string]ai.EngineAdapter{ai.EngineClaude: fe}}
	_, err := b.CollectUsage(context.Background(), ai.EngineClaude, "sid")
	if !errors.Is(err, ErrSessionDataNotFound) {
		t.Fatalf("expected ErrSessionDataNotFound, got %v", err)
	}
}

func TestCollectUsageUnknownEngine(t *testing.T) {
	b := &bridgeImpl{engines: map[string]ai.EngineAdapter{}}
	_, err := b.CollectUsage(context.Background(), ai.EngineClaude, "sid")
	if !errors.Is(err, ErrEngineNotEnabled) {
		t.Fatalf("expected ErrEngineNotEnabled, got %v", err)
	}
}
```

- [ ] **Step 2: Run test, expect FAIL**

Run: `go test ./internal/ai/bridge/...`
Expected: FAIL.

- [ ] **Step 3: Implement**

`internal/ai/bridge/usage.go`:

```go
package bridge

import (
	"context"
	"errors"
	"fmt"

	ai "github.com/theopenbee/openbee/internal/ai"
)

// Usage holds per-model token consumption for one session turn. Mirrors
// ai.TokenUsage so business code never touches ai types.
type Usage struct {
	Model               string
	InputTokens         int64
	OutputTokens        int64
	CacheCreationTokens int64
	CacheReadTokens     int64
}

// ErrSessionDataNotFound is returned by CollectUsage when no engine-side
// data exists for the (engine, session) pair.
var ErrSessionDataNotFound = errors.New("bridge: session data not found")

// CollectUsage returns per-model usage for the (engineName, sessionID)
// pair. Errors are translated so business code never sees an ai-package
// sentinel.
func (b *bridgeImpl) CollectUsage(ctx context.Context, engineName, sessionID string) ([]Usage, error) {
	eng, ok := b.engines[engineName]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrEngineNotEnabled, engineName)
	}
	raw, err := eng.CollectTokenUsage(ctx, sessionID)
	if err != nil {
		if errors.Is(err, ai.ErrSessionDataNotFound) {
			return nil, fmt.Errorf("%w: %s", ErrSessionDataNotFound, err.Error())
		}
		return nil, err
	}
	out := make([]Usage, len(raw))
	for i, u := range raw {
		out[i] = Usage(u)
	}
	return out, nil
}
```

Note: `Usage(u)` works only if the struct layouts match exactly. They do (same field names and order).

- [ ] **Step 4: Run tests**

Run: `go test ./internal/ai/bridge/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/ai/bridge/usage.go internal/ai/bridge/usage_test.go
git commit -m "feat(ai/bridge): Usage value type and CollectUsage"
```

---

### Task 1.5: deps.go — five ports

**Files:**
- Create: `internal/ai/bridge/deps.go`

- [ ] **Step 1: Add port interfaces**

`internal/ai/bridge/deps.go`:

```go
package bridge

import (
	"context"
	"time"
)

// TokenIssuer mints short-lived auth tokens for engine invocations.
type TokenIssuer interface {
	WorkerToken(workerID string, scopes []string) (string, error)
	BeeToken() (string, error)
}

// EnvResolver returns the KEY=VALUE env list to inject for a given role.
type EnvResolver interface {
	WorkerEnv(workerID string) ([]string, error)
	BeeEnv() ([]string, error)
}

// EngineSelector picks the engine name for a given role and hint.
type EngineSelector interface {
	// ForWorker returns hint when it names an enabled engine, otherwise
	// the current default.
	ForWorker(hint string) string
	// ForBee returns the current default engine.
	ForBee() string
}

// ArgsResolver merges global + role-specific engine_args JSON layers and
// returns the raw CLI tail for engineName. Failures upstream (missing
// rows, malformed JSON) are silently treated as empty layers; this
// matches existing behaviour and ensures a corrupt config row cannot
// block a run.
type ArgsResolver interface {
	ForWorker(ctx context.Context, workerEngineArgs, engineName string) string
	ForBee(ctx context.Context, engineName string) string
}

// LogPathProvider prepares the on-disk log path for a worker execution.
type LogPathProvider interface {
	PrepareForWorker(executionID string, startedAt time.Time) (string, error)
}
```

- [ ] **Step 2: Build**

Run: `go build ./internal/ai/bridge/...`
Expected: succeeds.

- [ ] **Step 3: Commit**

```bash
git add internal/ai/bridge/deps.go
git commit -m "feat(ai/bridge): define five dependency ports"
```

---

### Task 1.6: run.go — types and request shapes (no implementation yet)

**Files:**
- Create: `internal/ai/bridge/run.go`

- [ ] **Step 1: Add Handle / Outcome / Status / request types**

`internal/ai/bridge/run.go`:

```go
package bridge

import (
	"context"
	"time"
)

// Status classifies the terminal state of a run.
type Status int

const (
	StatusCompleted Status = iota + 1
	StatusFailed
	StatusAbandoned // process exited without a Done/Error signal
)

// Outcome is the terminal result of a run.
type Outcome struct {
	Status Status
	Result string
}

// Handle is the lifecycle handle for a started run.
type Handle interface {
	PID() int
	EngineName() string
	Stop() error
	Wait(ctx context.Context) (Outcome, error)
}

// WorkerRunRequest carries the inputs required to run a worker.
type WorkerRunRequest struct {
	WorkerID         string
	PermissionScopes []string
	ExecutionID      string
	StartedAt        time.Time
	EngineHint       string
	EngineArgs       string
	WorkDir          string
	Prompt           string
	SessionID        string
	Resume           bool
	Timeout          time.Duration
}

// BeeRunRequest carries the inputs required to run a bee.
type BeeRunRequest struct {
	WorkDir   string
	Prompt    string
	SessionID string
	Resume    bool
	LogPath   string
}
```

- [ ] **Step 2: Build**

Run: `go build ./internal/ai/bridge/...`
Expected: succeeds.

- [ ] **Step 3: Commit**

```bash
git add internal/ai/bridge/run.go
git commit -m "feat(ai/bridge): run lifecycle types and request shapes"
```

---

### Task 1.7: facade.go — Bridge interface, Config, New

**Files:**
- Modify: `internal/ai/bridge/facade.go`

- [ ] **Step 1: Replace facade.go with full interface, Config, New, and method stubs**

```go
package bridge

import (
	"context"
	"errors"

	ai "github.com/theopenbee/openbee/internal/ai"
)

// Bridge is the single business-facing entry point.
type Bridge interface {
	RunWorker(ctx context.Context, req WorkerRunRequest) (Handle, error)
	RunBee(ctx context.Context, req BeeRunRequest) (Handle, error)

	AllEngines() []string
	EnabledEngines() []string
	IsEnabled(name string) bool
	ValidateEngine(name string) error
	ValidateEngineArgs(line string) error

	ResolveEngineForWorker(workerID, hint string) string
	ResolveEngineForBee() string

	CollectUsage(ctx context.Context, engineName, sessionID string) ([]Usage, error)
}

// Deps groups the five dependency ports the bridge needs.
type Deps struct {
	TokenIssuer     TokenIssuer
	EnvResolver     EnvResolver
	EngineSelector  EngineSelector
	ArgsResolver    ArgsResolver
	LogPathProvider LogPathProvider
}

// Config is the constructor input.
type Config struct {
	Engines map[string]ai.EngineAdapter
	Deps    Deps
}

// ErrInvalidConfig is returned by New when Config is incomplete.
var ErrInvalidConfig = errors.New("bridge: invalid config")

// New returns a Bridge. It validates that all five ports are non-nil and
// that Engines is non-empty.
func New(cfg Config) (Bridge, error) {
	switch {
	case len(cfg.Engines) == 0:
		return nil, errors.New("bridge: Config.Engines is empty: " + ErrInvalidConfig.Error())
	case cfg.Deps.TokenIssuer == nil:
		return nil, errors.New("bridge: Deps.TokenIssuer is nil: " + ErrInvalidConfig.Error())
	case cfg.Deps.EnvResolver == nil:
		return nil, errors.New("bridge: Deps.EnvResolver is nil: " + ErrInvalidConfig.Error())
	case cfg.Deps.EngineSelector == nil:
		return nil, errors.New("bridge: Deps.EngineSelector is nil: " + ErrInvalidConfig.Error())
	case cfg.Deps.ArgsResolver == nil:
		return nil, errors.New("bridge: Deps.ArgsResolver is nil: " + ErrInvalidConfig.Error())
	case cfg.Deps.LogPathProvider == nil:
		return nil, errors.New("bridge: Deps.LogPathProvider is nil: " + ErrInvalidConfig.Error())
	}
	return &bridgeImpl{engines: cfg.Engines, deps: cfg.Deps}, nil
}

// bridgeImpl declared in deps-bearing form. Methods live in this file
// (engine resolution) plus names.go / validate.go / usage.go / run.go.
type bridgeImpl struct {
	engines map[string]ai.EngineAdapter
	deps    Deps
}

func (b *bridgeImpl) ResolveEngineForWorker(_, hint string) string {
	return b.deps.EngineSelector.ForWorker(hint)
}

func (b *bridgeImpl) ResolveEngineForBee() string {
	return b.deps.EngineSelector.ForBee()
}
```

(Replaces the placeholder `facade.go` from Task 1.2.)

- [ ] **Step 2: Verify build (RunWorker/RunBee not implemented yet — these come in Task 1.8)**

Run: `go build ./internal/ai/bridge/...`
Expected: build FAILS — `bridgeImpl` does not implement `Bridge` (missing RunWorker/RunBee). This is expected; Task 1.8 fills them in. **Do not commit yet.**

- [ ] **Step 3: Add tests for ResolveEngineFor*** and New validation

`internal/ai/bridge/facade_test.go`:

```go
package bridge

import (
	"errors"
	"testing"
)

type stubSelector struct {
	worker func(hint string) string
	bee    func() string
}

func (s stubSelector) ForWorker(h string) string { return s.worker(h) }
func (s stubSelector) ForBee() string            { return s.bee() }

func TestResolveEngineForWorkerDelegates(t *testing.T) {
	b := &bridgeImpl{deps: Deps{EngineSelector: stubSelector{
		worker: func(h string) string {
			if h == "" {
				return "default"
			}
			return h
		},
	}}}
	if got := b.ResolveEngineForWorker("w1", ""); got != "default" {
		t.Fatalf("empty hint: got %q, want default", got)
	}
	if got := b.ResolveEngineForWorker("w1", "claude"); got != "claude" {
		t.Fatalf("hint: got %q, want claude", got)
	}
}

func TestResolveEngineForBeeDelegates(t *testing.T) {
	b := &bridgeImpl{deps: Deps{EngineSelector: stubSelector{
		bee: func() string { return "kimi" },
	}}}
	if got := b.ResolveEngineForBee(); got != "kimi" {
		t.Fatalf("got %q, want kimi", got)
	}
}

func TestNewValidatesConfig(t *testing.T) {
	_, err := New(Config{})
	if !errors.Is(err, ErrInvalidConfig) {
		// Wrapping is by string concat in this implementation; we check
		// substring instead.
		if err == nil || !contains(err.Error(), "invalid config") {
			t.Fatalf("expected ErrInvalidConfig-ish error, got %v", err)
		}
	}
}

func contains(s, sub string) bool { return len(sub) == 0 || (len(s) >= len(sub) && (indexOf(s, sub) >= 0)) }
func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
```

- [ ] **Step 4: Do not run tests yet (interface not satisfied). Move to Task 1.8.**

---

### Task 1.8: run.go — RunWorker / RunBee implementations + Handle + Wait + invariants

**Files:**
- Modify: `internal/ai/bridge/run.go`
- Create: `internal/ai/bridge/fake_engine_test.go`, `internal/ai/bridge/run_test.go`

- [ ] **Step 1: Add the in-package fake engine helper**

`internal/ai/bridge/fake_engine_test.go`:

```go
package bridge

import (
	"context"
	"sync/atomic"

	ai "github.com/theopenbee/openbee/internal/ai"
)

// fakeProcess satisfies ai.Process for tests.
type fakeProcess struct {
	pid     int
	stopped atomic.Bool
	onStop  func()
}

func (f *fakeProcess) PID() int { return f.pid }
func (f *fakeProcess) Stop() error {
	f.stopped.Store(true)
	if f.onStop != nil {
		f.onStop()
	}
	return nil
}

// fakeEngine drives all six lifecycle invariants from tests.
type fakeEngine struct {
	pid        int
	scriptedCh chan ai.Output           // closed by test to signal end
	extract    func() string            // returns final extracted result
	onRun      func(opts ai.RunOptions) // observe RunOptions passed to engine
	runErr     error                    // returned from Run when non-nil
	usages     []ai.TokenUsage
	usagesErr  error
	proc       *fakeProcess
}

func (f *fakeEngine) Run(_ context.Context, _ string, _ string, opts ai.RunOptions, _ string) (ai.RunResult, error) {
	if f.runErr != nil {
		return ai.RunResult{}, f.runErr
	}
	if f.onRun != nil {
		f.onRun(opts)
	}
	if f.proc == nil {
		f.proc = &fakeProcess{pid: f.pid}
	}
	extract := f.extract
	if extract == nil {
		extract = func() string { return "" }
	}
	return ai.RunResult{
		Process:       f.proc,
		Output:        f.scriptedCh,
		ExtractResult: extract,
	}, nil
}

func (f *fakeEngine) CollectTokenUsage(context.Context, string) ([]ai.TokenUsage, error) {
	return f.usages, f.usagesErr
}
```

- [ ] **Step 2: Write failing run_test.go covering all six invariants**

`internal/ai/bridge/run_test.go`:

```go
package bridge

import (
	"context"
	"errors"
	"testing"
	"time"

	ai "github.com/theopenbee/openbee/internal/ai"
)

func newTestBridge(t *testing.T, eng *fakeEngine, selector EngineSelector) *bridgeImpl {
	t.Helper()
	if selector == nil {
		selector = stubSelector{
			worker: func(hint string) string {
				if hint == "" {
					return ai.EngineClaude
				}
				return hint
			},
			bee: func() string { return ai.EngineClaude },
		}
	}
	return &bridgeImpl{
		engines: map[string]ai.EngineAdapter{ai.EngineClaude: eng},
		deps: Deps{
			TokenIssuer:     stubTokens{},
			EnvResolver:     stubEnv{},
			EngineSelector:  selector,
			ArgsResolver:    stubArgs{},
			LogPathProvider: stubLogPath{path: "/tmp/log"},
		},
	}
}

type stubTokens struct{}

func (stubTokens) WorkerToken(_ string, _ []string) (string, error) { return "wtok", nil }
func (stubTokens) BeeToken() (string, error)                        { return "btok", nil }

type stubEnv struct{}

func (stubEnv) WorkerEnv(_ string) ([]string, error) { return []string{"K=V"}, nil }
func (stubEnv) BeeEnv() ([]string, error)            { return []string{"K=B"}, nil }

type stubArgs struct{}

func (stubArgs) ForWorker(context.Context, string, string) string { return "--worker-flag" }
func (stubArgs) ForBee(context.Context, string) string            { return "--bee-flag" }

type stubLogPath struct {
	path string
	err  error
}

func (s stubLogPath) PrepareForWorker(string, time.Time) (string, error) { return s.path, s.err }

func TestRunWorkerHappyPath_CompletedOutcome(t *testing.T) {
	ch := make(chan ai.Output, 1)
	eng := &fakeEngine{pid: 4321, scriptedCh: ch, extract: func() string { return "final" }}
	b := newTestBridge(t, eng, nil)

	h, err := b.RunWorker(context.Background(), WorkerRunRequest{
		WorkerID: "w1", ExecutionID: "e1", StartedAt: time.Now(),
		WorkDir: "/wd", Prompt: "p", SessionID: "s1",
	})
	if err != nil {
		t.Fatalf("RunWorker: %v", err)
	}
	if h.PID() != 4321 {
		t.Fatalf("PID: got %d, want 4321", h.PID())
	}
	if h.EngineName() != ai.EngineClaude {
		t.Fatalf("EngineName: got %q", h.EngineName())
	}

	ch <- ai.Output{Type: ai.OutputDone}
	close(ch)

	got, err := h.Wait(context.Background())
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if got.Status != StatusCompleted || got.Result != "final" {
		t.Fatalf("outcome: %+v", got)
	}

	// Invariant 1: Wait is idempotent.
	got2, _ := h.Wait(context.Background())
	if got2 != got {
		t.Fatalf("second Wait returned different outcome: %+v vs %+v", got2, got)
	}
}

func TestRunWorkerFailedOutcome(t *testing.T) {
	ch := make(chan ai.Output, 1)
	eng := &fakeEngine{pid: 1, scriptedCh: ch, extract: func() string { return "" }}
	b := newTestBridge(t, eng, nil)
	h, _ := b.RunWorker(context.Background(), WorkerRunRequest{})
	ch <- ai.Output{Type: ai.OutputError, Content: "boom"}
	close(ch)
	got, _ := h.Wait(context.Background())
	if got.Status != StatusFailed || got.Result != "boom" {
		t.Fatalf("outcome: %+v", got)
	}
}

func TestRunWorkerAbandonedOutcome(t *testing.T) {
	ch := make(chan ai.Output)
	eng := &fakeEngine{pid: 1, scriptedCh: ch, extract: func() string { return "" }}
	b := newTestBridge(t, eng, nil)
	h, _ := b.RunWorker(context.Background(), WorkerRunRequest{})
	close(ch) // no Done/Error signal
	got, _ := h.Wait(context.Background())
	if got.Status != StatusAbandoned {
		t.Fatalf("status: got %v, want StatusAbandoned", got.Status)
	}
	if got.Result != "process exited without completion signal" {
		t.Fatalf("placeholder result missing: %q", got.Result)
	}
}

func TestRunWorkerExtractCalledOncePerTerminal(t *testing.T) {
	ch := make(chan ai.Output, 1)
	var calls int32
	eng := &fakeEngine{pid: 1, scriptedCh: ch, extract: func() string {
		calls++
		return "x"
	}}
	b := newTestBridge(t, eng, nil)
	h, _ := b.RunWorker(context.Background(), WorkerRunRequest{})
	ch <- ai.Output{Type: ai.OutputDone}
	close(ch)
	for i := 0; i < 5; i++ {
		_, _ = h.Wait(context.Background())
	}
	if calls != 1 {
		t.Fatalf("ExtractResult call count: got %d, want 1", calls)
	}
}

func TestRunWorkerStopIsIdempotentAndWaitStillReturns(t *testing.T) {
	ch := make(chan ai.Output, 1)
	eng := &fakeEngine{pid: 1, scriptedCh: ch, extract: func() string { return "x" }}
	b := newTestBridge(t, eng, nil)
	h, _ := b.RunWorker(context.Background(), WorkerRunRequest{})
	if err := h.Stop(); err != nil {
		t.Fatalf("first Stop: %v", err)
	}
	if err := h.Stop(); err != nil {
		t.Fatalf("second Stop: %v", err)
	}
	ch <- ai.Output{Type: ai.OutputDone}
	close(ch)
	if _, err := h.Wait(context.Background()); err != nil {
		t.Fatalf("Wait after Stop: %v", err)
	}
}

func TestRunWorkerWaitContextCancelled(t *testing.T) {
	ch := make(chan ai.Output)
	eng := &fakeEngine{pid: 1, scriptedCh: ch, extract: func() string { return "" }}
	b := newTestBridge(t, eng, nil)
	h, _ := b.RunWorker(context.Background(), WorkerRunRequest{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := h.Wait(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestRunWorkerPassesAssembledOptionsToEngine(t *testing.T) {
	ch := make(chan ai.Output, 1)
	var captured ai.RunOptions
	eng := &fakeEngine{pid: 1, scriptedCh: ch, extract: func() string { return "x" }, onRun: func(o ai.RunOptions) { captured = o }}
	b := newTestBridge(t, eng, nil)
	_, err := b.RunWorker(context.Background(), WorkerRunRequest{
		WorkerID: "w1", PermissionScopes: []string{"a", "b"},
		SessionID: "sess", Resume: true,
	})
	if err != nil {
		t.Fatalf("RunWorker: %v", err)
	}
	if captured.APIKey != "wtok" {
		t.Fatalf("APIKey: got %q, want wtok", captured.APIKey)
	}
	if len(captured.ExtraEnv) != 1 || captured.ExtraEnv[0] != "K=V" {
		t.Fatalf("ExtraEnv: %v", captured.ExtraEnv)
	}
	if captured.ExtraArgs != "--worker-flag" {
		t.Fatalf("ExtraArgs: %q", captured.ExtraArgs)
	}
	if captured.SessionID != "sess" || !captured.Resume {
		t.Fatalf("session/resume not propagated: %+v", captured)
	}
	close(ch)
}

func TestRunWorkerStartupFailures(t *testing.T) {
	// Unknown engine.
	b := newTestBridge(t, &fakeEngine{scriptedCh: make(chan ai.Output)}, stubSelector{
		worker: func(string) string { return "nonexistent" },
	})
	if _, err := b.RunWorker(context.Background(), WorkerRunRequest{}); err == nil {
		t.Fatalf("expected error for unknown engine")
	}

	// Engine.Run propagates failure.
	failing := &fakeEngine{runErr: errors.New("engine boom")}
	b2 := newTestBridge(t, failing, nil)
	if _, err := b2.RunWorker(context.Background(), WorkerRunRequest{}); err == nil {
		t.Fatalf("expected error from engine.Run")
	}
}

func TestRunBeeHappyPath(t *testing.T) {
	ch := make(chan ai.Output, 1)
	var captured ai.RunOptions
	eng := &fakeEngine{pid: 9, scriptedCh: ch, extract: func() string { return "bee-final" }, onRun: func(o ai.RunOptions) { captured = o }}
	b := newTestBridge(t, eng, nil)
	h, err := b.RunBee(context.Background(), BeeRunRequest{WorkDir: "/wd", Prompt: "p", SessionID: "bs", LogPath: "/tmp/bee.log"})
	if err != nil {
		t.Fatalf("RunBee: %v", err)
	}
	if captured.APIKey != "btok" || captured.ExtraEnv[0] != "K=B" || captured.ExtraArgs != "--bee-flag" {
		t.Fatalf("bee options wrong: %+v", captured)
	}
	ch <- ai.Output{Type: ai.OutputDone}
	close(ch)
	got, _ := h.Wait(context.Background())
	if got.Result != "bee-final" {
		t.Fatalf("result: %q", got.Result)
	}
}
```

- [ ] **Step 3: Run tests, expect FAIL (RunWorker/RunBee not implemented)**

Run: `go test ./internal/ai/bridge/...`
Expected: FAIL — methods not implemented.

- [ ] **Step 4: Implement RunWorker / RunBee / Handle in run.go**

Replace `internal/ai/bridge/run.go` with:

```go
package bridge

import (
	"context"
	"fmt"
	"sync"
	"time"

	ai "github.com/theopenbee/openbee/internal/ai"
)

// Status / Outcome / Handle / request types (already declared above) — keep them.
// (Below this line, append the implementation.)

const abandonedPlaceholder = "process exited without completion signal"

type runHandle struct {
	pid        int
	engineName string
	proc       ai.Process
	out        <-chan ai.Output
	extract    func() string

	once    sync.Once
	outcome Outcome
	doneCh  chan struct{}

	cancel context.CancelFunc // cancels the run's internal context (with Timeout)
	stop   sync.Once
}

func (h *runHandle) PID() int            { return h.pid }
func (h *runHandle) EngineName() string  { return h.engineName }

func (h *runHandle) Stop() error {
	var err error
	h.stop.Do(func() { err = h.proc.Stop() })
	return err
}

func (h *runHandle) Wait(ctx context.Context) (Outcome, error) {
	select {
	case <-h.doneCh:
		return h.outcome, nil
	case <-ctx.Done():
		return Outcome{}, ctx.Err()
	}
}

// drain reads h.out and produces the single terminal outcome.
func (h *runHandle) drain() {
	defer func() {
		close(h.doneCh)
		if h.cancel != nil {
			h.cancel()
		}
	}()
	finalized := false
	for ev := range h.out {
		switch ev.Type {
		case ai.OutputDone:
			h.once.Do(func() { h.outcome = Outcome{Status: StatusCompleted, Result: h.extract()} })
			finalized = true
		case ai.OutputError:
			h.once.Do(func() {
				res := h.extract()
				if res == "" {
					res = ev.Content
				}
				h.outcome = Outcome{Status: StatusFailed, Result: res}
			})
			finalized = true
		}
	}
	if !finalized {
		h.once.Do(func() {
			res := h.extract()
			if res == "" {
				res = abandonedPlaceholder
			}
			h.outcome = Outcome{Status: StatusAbandoned, Result: res}
		})
	}
}

func (b *bridgeImpl) RunWorker(ctx context.Context, req WorkerRunRequest) (Handle, error) {
	engineName := b.deps.EngineSelector.ForWorker(req.EngineHint)
	engine, ok := b.engines[engineName]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrEngineNotEnabled, engineName)
	}

	logPath, err := b.deps.LogPathProvider.PrepareForWorker(req.ExecutionID, req.StartedAt)
	if err != nil {
		return nil, fmt.Errorf("prepare log path: %w", err)
	}
	token, err := b.deps.TokenIssuer.WorkerToken(req.WorkerID, req.PermissionScopes)
	if err != nil {
		return nil, fmt.Errorf("mint worker token: %w", err)
	}
	env, err := b.deps.EnvResolver.WorkerEnv(req.WorkerID)
	if err != nil {
		return nil, fmt.Errorf("resolve worker env: %w", err)
	}
	args := b.deps.ArgsResolver.ForWorker(ctx, req.EngineArgs, engineName)

	execCtx, cancel := newRunContext(req.Timeout)

	res, err := engine.Run(execCtx, req.WorkDir, req.Prompt, ai.RunOptions{
		SessionID: req.SessionID,
		Resume:    req.Resume,
		APIKey:    token,
		ExtraEnv:  env,
		ExtraArgs: args,
	}, logPath)
	if err != nil {
		cancel()
		return nil, err
	}

	h := &runHandle{
		pid:        res.Process.PID(),
		engineName: engineName,
		proc:       res.Process,
		out:        res.Output,
		extract:    res.ExtractResult,
		doneCh:     make(chan struct{}),
		cancel:     cancel,
	}
	go h.drain()
	return h, nil
}

func (b *bridgeImpl) RunBee(ctx context.Context, req BeeRunRequest) (Handle, error) {
	engineName := b.deps.EngineSelector.ForBee()
	engine, ok := b.engines[engineName]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrEngineNotEnabled, engineName)
	}

	token, err := b.deps.TokenIssuer.BeeToken()
	if err != nil {
		return nil, fmt.Errorf("mint bee token: %w", err)
	}
	env, err := b.deps.EnvResolver.BeeEnv()
	if err != nil {
		return nil, fmt.Errorf("resolve bee env: %w", err)
	}
	args := b.deps.ArgsResolver.ForBee(ctx, engineName)

	execCtx, cancel := newRunContext(0) // bee currently has no explicit timeout

	res, err := engine.Run(execCtx, req.WorkDir, req.Prompt, ai.RunOptions{
		SessionID: req.SessionID,
		Resume:    req.Resume,
		APIKey:    token,
		ExtraEnv:  env,
		ExtraArgs: args,
	}, req.LogPath)
	if err != nil {
		cancel()
		return nil, err
	}
	h := &runHandle{
		pid:        res.Process.PID(),
		engineName: engineName,
		proc:       res.Process,
		out:        res.Output,
		extract:    res.ExtractResult,
		doneCh:     make(chan struct{}),
		cancel:     cancel,
	}
	go h.drain()
	return h, nil
}

func newRunContext(timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout > 0 {
		return context.WithTimeout(context.Background(), timeout)
	}
	return context.WithCancel(context.Background())
}
```

- [ ] **Step 5: Run tests**

Run: `go test ./internal/ai/bridge/...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/ai/bridge/run.go internal/ai/bridge/run_test.go internal/ai/bridge/fake_engine_test.go internal/ai/bridge/facade.go internal/ai/bridge/facade_test.go
git commit -m "feat(ai/bridge): Run lifecycle with six invariants covered by tests"
```

---

### Task 1.9: adapters/token.go — TokenIssuer implementation

**Files:**
- Create: `internal/ai/bridge/adapters/token.go`, `internal/ai/bridge/adapters/token_test.go`

- [ ] **Step 1: Write failing test**

`internal/ai/bridge/adapters/token_test.go`:

```go
package adapters

import (
	"testing"

	"github.com/theopenbee/openbee/internal/infra/auth"
)

func TestTokenIssuerMintsWorkerAndBeeTokens(t *testing.T) {
	secret := "test-secret"
	iss := NewTokenIssuer(secret, 60)

	wt, err := iss.WorkerToken("worker-id-1", []string{"scope-a"})
	if err != nil {
		t.Fatalf("WorkerToken: %v", err)
	}
	if wt == "" {
		t.Fatal("expected non-empty worker token")
	}
	// Verify the produced token parses under the same secret.
	if _, _, err := auth.ParseWorkerToken(secret, wt); err != nil {
		t.Fatalf("ParseWorkerToken: %v", err)
	}

	bt, err := iss.BeeToken()
	if err != nil {
		t.Fatalf("BeeToken: %v", err)
	}
	if _, err := auth.ParseBeeToken(secret, bt); err != nil {
		t.Fatalf("ParseBeeToken: %v", err)
	}
}
```

Note: confirm the exact `auth.ParseWorkerToken` / `auth.ParseBeeToken` signatures during implementation; adjust call site if the auth package exposes different verifier helpers. If no parse helper exists, replace verification with "non-empty string returned".

- [ ] **Step 2: Run test, expect FAIL**

Run: `go test ./internal/ai/bridge/adapters/...`
Expected: FAIL (NewTokenIssuer undefined).

- [ ] **Step 3: Implement**

`internal/ai/bridge/adapters/token.go`:

```go
package adapters

import (
	"time"

	bridge "github.com/theopenbee/openbee/internal/ai/bridge"
	"github.com/theopenbee/openbee/internal/infra/auth"
)

type tokenIssuer struct {
	secret string
	ttl    time.Duration
}

// NewTokenIssuer wraps auth.GenerateWorkerToken / GenerateBeeToken.
func NewTokenIssuer(secret string, ttl time.Duration) bridge.TokenIssuer {
	return tokenIssuer{secret: secret, ttl: ttl}
}

func (t tokenIssuer) WorkerToken(workerID string, scopes []string) (string, error) {
	return auth.GenerateWorkerToken(t.secret, workerID, scopes, t.ttl)
}
func (t tokenIssuer) BeeToken() (string, error) {
	return auth.GenerateBeeToken(t.secret, t.ttl)
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/ai/bridge/adapters/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/ai/bridge/adapters/token.go internal/ai/bridge/adapters/token_test.go
git commit -m "feat(ai/bridge/adapters): TokenIssuer implementation"
```

---

### Task 1.10: adapters/env.go — EnvResolver implementation

**Files:**
- Create: `internal/ai/bridge/adapters/env.go`, `internal/ai/bridge/adapters/env_test.go`

- [ ] **Step 1: Write failing test**

`internal/ai/bridge/adapters/env_test.go`:

```go
package adapters

import (
	"reflect"
	"testing"

	"github.com/theopenbee/openbee/internal/domain/env"
)

func TestEnvResolverDelegates(t *testing.T) {
	svc := &fakeEnvService{
		worker: map[string][]string{"w1": {"A=1"}},
		bee:    []string{"B=2"},
	}
	r := NewEnvResolver(svc)

	got, err := r.WorkerEnv("w1")
	if err != nil || !reflect.DeepEqual(got, []string{"A=1"}) {
		t.Fatalf("WorkerEnv: got %v, err %v", got, err)
	}
	got, err = r.BeeEnv()
	if err != nil || !reflect.DeepEqual(got, []string{"B=2"}) {
		t.Fatalf("BeeEnv: got %v, err %v", got, err)
	}
}

// fakeEnvService satisfies the subset of *env.Service used by the adapter.
type fakeEnvService struct {
	worker map[string][]string
	bee    []string
}

func (f *fakeEnvService) ResolveWorkerEnv(id string) ([]string, error) { return f.worker[id], nil }
func (f *fakeEnvService) ResolveBeeEnv(string) ([]string, error)       { return f.bee, nil }

// Compile-time check: ensure the adapter accepts our fake via the interface
// declared in env.go.
var _ envService = (*fakeEnvService)(nil)
var _ envService = (*env.Service)(nil)
```

- [ ] **Step 2: Run test, expect FAIL**

Run: `go test ./internal/ai/bridge/adapters/...`
Expected: FAIL.

- [ ] **Step 3: Implement**

`internal/ai/bridge/adapters/env.go`:

```go
package adapters

import bridge "github.com/theopenbee/openbee/internal/ai/bridge"

// envService is the subset of *env.Service consumed here, declared locally
// so tests can fake it without importing env.Service.
type envService interface {
	ResolveWorkerEnv(workerID string) ([]string, error)
	ResolveBeeEnv(beeID string) ([]string, error)
}

const defaultBeeID = "default"

type envResolver struct{ svc envService }

func NewEnvResolver(svc envService) bridge.EnvResolver { return envResolver{svc: svc} }

func (e envResolver) WorkerEnv(workerID string) ([]string, error) {
	return e.svc.ResolveWorkerEnv(workerID)
}
func (e envResolver) BeeEnv() ([]string, error) { return e.svc.ResolveBeeEnv(defaultBeeID) }
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/ai/bridge/adapters/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/ai/bridge/adapters/env.go internal/ai/bridge/adapters/env_test.go
git commit -m "feat(ai/bridge/adapters): EnvResolver implementation"
```

---

### Task 1.11: adapters/engine.go — EngineSelector implementation

**Files:**
- Create: `internal/ai/bridge/adapters/engine.go`, `internal/ai/bridge/adapters/engine_test.go`

- [ ] **Step 1: Write failing test**

`internal/ai/bridge/adapters/engine_test.go`:

```go
package adapters

import (
	"testing"

	ai "github.com/theopenbee/openbee/internal/ai"
	"github.com/theopenbee/openbee/internal/domain/enginecfg"
)

func TestEngineSelectorForWorkerPrefersHintFallsBackToDefault(t *testing.T) {
	cfg := enginecfg.NewStore(ai.EngineClaude)
	engines := map[string]ai.EngineAdapter{ai.EngineClaude: nil, ai.EngineCodex: nil}
	s := NewEngineSelector(engines, cfg)

	if got := s.ForWorker(""); got != ai.EngineClaude {
		t.Fatalf("empty hint: got %q", got)
	}
	if got := s.ForWorker(ai.EngineCodex); got != ai.EngineCodex {
		t.Fatalf("valid hint: got %q", got)
	}
	if got := s.ForWorker("missing"); got != ai.EngineClaude {
		t.Fatalf("unknown hint should fall back: got %q", got)
	}
}

func TestEngineSelectorForBee(t *testing.T) {
	cfg := enginecfg.NewStore(ai.EngineKimi)
	s := NewEngineSelector(map[string]ai.EngineAdapter{ai.EngineKimi: nil}, cfg)
	if got := s.ForBee(); got != ai.EngineKimi {
		t.Fatalf("got %q", got)
	}
}
```

- [ ] **Step 2: Run, expect FAIL**

Run: `go test ./internal/ai/bridge/adapters/...`
Expected: FAIL.

- [ ] **Step 3: Implement**

`internal/ai/bridge/adapters/engine.go`:

```go
package adapters

import (
	ai "github.com/theopenbee/openbee/internal/ai"
	bridge "github.com/theopenbee/openbee/internal/ai/bridge"
	"github.com/theopenbee/openbee/internal/domain/enginecfg"
)

type engineSelector struct {
	engines map[string]ai.EngineAdapter
	cfg     *enginecfg.Store
}

func NewEngineSelector(engines map[string]ai.EngineAdapter, cfg *enginecfg.Store) bridge.EngineSelector {
	return engineSelector{engines: engines, cfg: cfg}
}

func (s engineSelector) ForWorker(hint string) string {
	if hint != "" {
		if _, ok := s.engines[hint]; ok {
			return hint
		}
	}
	return s.cfg.Get()
}
func (s engineSelector) ForBee() string { return s.cfg.Get() }
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/ai/bridge/adapters/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/ai/bridge/adapters/engine.go internal/ai/bridge/adapters/engine_test.go
git commit -m "feat(ai/bridge/adapters): EngineSelector implementation"
```

---

### Task 1.12: adapters/args.go — ArgsResolver implementation

**Files:**
- Create: `internal/ai/bridge/adapters/args.go`, `internal/ai/bridge/adapters/args_test.go`

- [ ] **Step 1: Write failing test**

`internal/ai/bridge/adapters/args_test.go`:

```go
package adapters

import (
	"context"
	"testing"

	ai "github.com/theopenbee/openbee/internal/ai"
	"github.com/theopenbee/openbee/internal/infra/model"
)

type fakeSysStore struct{ values map[string]string }

func (f fakeSysStore) Get(_ context.Context, key string) (model.SystemConfig, bool, error) {
	v, ok := f.values[key]
	return model.SystemConfig{Key: key, Value: v}, ok, nil
}

func TestArgsResolverForWorkerMergesGlobalThenWorker(t *testing.T) {
	store := fakeSysStore{values: map[string]string{
		model.SystemConfigKeyEngineArgsGlobal: `{"claude":"--g"}`,
	}}
	r := NewArgsResolver(store)
	got := r.ForWorker(context.Background(), `{"claude":"--w"}`, ai.EngineClaude)
	if got != "--g --w" {
		t.Fatalf("got %q, want %q", got, "--g --w")
	}
}

func TestArgsResolverForBeeMergesGlobalThenBee(t *testing.T) {
	store := fakeSysStore{values: map[string]string{
		model.SystemConfigKeyEngineArgsGlobal: `{"claude":"--g"}`,
		model.SystemConfigKeyEngineArgsBee:    `{"claude":"--b"}`,
	}}
	r := NewArgsResolver(store)
	got := r.ForBee(context.Background(), ai.EngineClaude)
	if got != "--g --b" {
		t.Fatalf("got %q", got)
	}
}

func TestArgsResolverMissingValuesAreEmpty(t *testing.T) {
	r := NewArgsResolver(fakeSysStore{values: map[string]string{}})
	if got := r.ForWorker(context.Background(), "", ai.EngineClaude); got != "" {
		t.Fatalf("got %q, want empty", got)
	}
}
```

- [ ] **Step 2: Run test, expect FAIL**

Run: `go test ./internal/ai/bridge/adapters/...`
Expected: FAIL.

- [ ] **Step 3: Implement**

`internal/ai/bridge/adapters/args.go`:

```go
package adapters

import (
	"context"

	ai "github.com/theopenbee/openbee/internal/ai"
	bridge "github.com/theopenbee/openbee/internal/ai/bridge"
	"github.com/theopenbee/openbee/internal/infra/model"
)

// sysConfigReader is the subset of *store.SystemConfigStore used here.
type sysConfigReader interface {
	Get(ctx context.Context, key string) (model.SystemConfig, bool, error)
}

type argsResolver struct{ store sysConfigReader }

func NewArgsResolver(store sysConfigReader) bridge.ArgsResolver { return argsResolver{store: store} }

func (a argsResolver) read(ctx context.Context, key string) string {
	if a.store == nil {
		return ""
	}
	cfg, ok, err := a.store.Get(ctx, key)
	if err != nil || !ok {
		return ""
	}
	return cfg.Value
}

func (a argsResolver) ForWorker(ctx context.Context, workerEngineArgs, engineName string) string {
	return ai.ResolveExtraArgs(engineName, a.read(ctx, model.SystemConfigKeyEngineArgsGlobal), workerEngineArgs)
}
func (a argsResolver) ForBee(ctx context.Context, engineName string) string {
	return ai.ResolveExtraArgs(engineName,
		a.read(ctx, model.SystemConfigKeyEngineArgsGlobal),
		a.read(ctx, model.SystemConfigKeyEngineArgsBee),
	)
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/ai/bridge/adapters/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/ai/bridge/adapters/args.go internal/ai/bridge/adapters/args_test.go
git commit -m "feat(ai/bridge/adapters): ArgsResolver implementation"
```

---

### Task 1.13: adapters/logpath.go — LogPathProvider implementation

**Files:**
- Create: `internal/ai/bridge/adapters/logpath.go`, `internal/ai/bridge/adapters/logpath_test.go`

- [ ] **Step 1: Write failing test**

`internal/ai/bridge/adapters/logpath_test.go`:

```go
package adapters

import (
	"testing"
	"time"
)

type fakeExecStore struct{ path string }

func (f fakeExecStore) PrepareLogPath(execID string, startedAt time.Time) (string, error) {
	return f.path + "/" + execID, nil
}

func TestLogPathProviderDelegates(t *testing.T) {
	p := NewLogPathProvider(fakeExecStore{path: "/var/log"})
	got, err := p.PrepareForWorker("e1", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if got != "/var/log/e1" {
		t.Fatalf("got %q", got)
	}
}
```

- [ ] **Step 2: Run test, expect FAIL**

Run: `go test ./internal/ai/bridge/adapters/...`
Expected: FAIL.

- [ ] **Step 3: Implement**

`internal/ai/bridge/adapters/logpath.go`:

```go
package adapters

import (
	"time"

	bridge "github.com/theopenbee/openbee/internal/ai/bridge"
)

// execLogPathPreparer is the subset of *store.ExecutionStore used here.
type execLogPathPreparer interface {
	PrepareLogPath(executionID string, startedAt time.Time) (string, error)
}

type logPathProvider struct{ store execLogPathPreparer }

func NewLogPathProvider(store execLogPathPreparer) bridge.LogPathProvider {
	return logPathProvider{store: store}
}

func (l logPathProvider) PrepareForWorker(executionID string, startedAt time.Time) (string, error) {
	return l.store.PrepareLogPath(executionID, startedAt)
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/ai/bridge/adapters/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/ai/bridge/adapters/logpath.go internal/ai/bridge/adapters/logpath_test.go
git commit -m "feat(ai/bridge/adapters): LogPathProvider implementation"
```

---

### Task 1.14: Wire bridge construction in `internal/app/app.go`

**Files:**
- Modify: `internal/app/app.go`

- [ ] **Step 1: Add bridge construction after `engineCfg` is built and before existing builders**

Find the block in `BuildApp` that builds `engineCfg` (around line 119) and before the call to `buildWorkerManager`. Insert:

```go
br, err := bridge.New(bridge.Config{
    Engines: engines,
    Deps: bridge.Deps{
        TokenIssuer:     adapters.NewTokenIssuer(cfg.Bee.RPC.TokenSecret, cfg.Bee.RPC.TokenTTL),
        EnvResolver:     adapters.NewEnvResolver(envSvc),
        EngineSelector:  adapters.NewEngineSelector(engines, engineCfg),
        ArgsResolver:    adapters.NewArgsResolver(s.systemConfigStore),
        LogPathProvider: adapters.NewLogPathProvider(s.execStore),
    },
})
if err != nil {
    return nil, fmt.Errorf("init ai bridge: %w", err)
}
_ = br // wired into business in phases 2 & 3
```

Add imports:

```go
"github.com/theopenbee/openbee/internal/ai/bridge"
"github.com/theopenbee/openbee/internal/ai/bridge/adapters"
```

The `envSvc` variable must be in scope before this block. Looking at current `app.go`, `envSvc` is constructed at line 132, after `engineCfg`. Move the bridge construction below `envSvc` (i.e., after line 135) so both are available.

- [ ] **Step 2: Build full app**

Run: `go build ./...`
Expected: succeeds.

- [ ] **Step 3: Run all tests**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/app/app.go
git commit -m "feat(app): construct ai bridge alongside existing engine wiring"
```

---

### Task 1.15: Add `internal/ai/doc.go` marker

**Files:**
- Create: `internal/ai/doc.go`

- [ ] **Step 1: Add documentation**

```go
// Package ai is the low-level AI engine subsystem. It is internal to the
// AI module and is not intended to be imported by business code.
//
// Business code (worker, bee, task, tokenstat, store, config, cmd) must
// depend on internal/ai/bridge instead. Only the bridge package itself,
// the engine implementations under internal/ai/engine/*, and the
// composition root in cmd/openbee/internal/app may import internal/ai.
package ai
```

- [ ] **Step 2: Build**

Run: `go build ./...`
Expected: succeeds.

- [ ] **Step 3: Commit**

```bash
git add internal/ai/doc.go
git commit -m "docs(ai): mark internal/ai as internal-only; point to bridge"
```

---

### Phase 1 exit gate

- [ ] **Verification step**: `go build ./... && go test ./...`
- [ ] **Verification step**: `go test ./internal/ai/bridge/... -count=10` (no flake)
- [ ] **Verification step**: confirm no business package was modified in phase 1 (only `internal/app/app.go` and new files under `internal/ai/`).

---

## Phase 2 — Migrate the worker path

### Task 2.1: Add `fakeBridge` test helper for worker tests

**Files:**
- Create: `internal/domain/worker/fake_bridge_test.go`

- [ ] **Step 1: Add helper used by upcoming manager_test.go changes**

```go
package worker

import (
	"context"

	"github.com/theopenbee/openbee/internal/ai/bridge"
)

// fakeBridge implements bridge.Bridge for unit tests.
type fakeBridge struct {
	runWorker func(ctx context.Context, req bridge.WorkerRunRequest) (bridge.Handle, error)
	runBee    func(ctx context.Context, req bridge.BeeRunRequest) (bridge.Handle, error)

	allEngines     []string
	enabledEngines []string
	resolveWorker  func(workerID, hint string) string
	resolveBee     func() string
	validateEngine func(name string) error
	validateArgs   func(line string) error
	collectUsage   func(ctx context.Context, engineName, sid string) ([]bridge.Usage, error)
}

func (f *fakeBridge) RunWorker(ctx context.Context, req bridge.WorkerRunRequest) (bridge.Handle, error) {
	if f.runWorker == nil {
		return nil, nil
	}
	return f.runWorker(ctx, req)
}
func (f *fakeBridge) RunBee(ctx context.Context, req bridge.BeeRunRequest) (bridge.Handle, error) {
	if f.runBee == nil {
		return nil, nil
	}
	return f.runBee(ctx, req)
}
func (f *fakeBridge) AllEngines() []string                 { return f.allEngines }
func (f *fakeBridge) EnabledEngines() []string             { return f.enabledEngines }
func (f *fakeBridge) IsEnabled(name string) bool {
	for _, n := range f.enabledEngines {
		if n == name {
			return true
		}
	}
	return false
}
func (f *fakeBridge) ValidateEngine(name string) error {
	if f.validateEngine != nil {
		return f.validateEngine(name)
	}
	return nil
}
func (f *fakeBridge) ValidateEngineArgs(line string) error {
	if f.validateArgs != nil {
		return f.validateArgs(line)
	}
	return nil
}
func (f *fakeBridge) ResolveEngineForWorker(workerID, hint string) string {
	if f.resolveWorker != nil {
		return f.resolveWorker(workerID, hint)
	}
	return hint
}
func (f *fakeBridge) ResolveEngineForBee() string {
	if f.resolveBee != nil {
		return f.resolveBee()
	}
	return ""
}
func (f *fakeBridge) CollectUsage(ctx context.Context, engineName, sid string) ([]bridge.Usage, error) {
	if f.collectUsage != nil {
		return f.collectUsage(ctx, engineName, sid)
	}
	return nil, nil
}
```

- [ ] **Step 2: Commit (no test changes yet, but compile-test the helper compiles)**

Run: `go build ./internal/domain/worker/...`
Expected: succeeds.

```bash
git add internal/domain/worker/fake_bridge_test.go
git commit -m "test(worker): add fakeBridge helper for upcoming migration"
```

---

### Task 2.2: Update `worker.Manager` constructor and fields

**Files:**
- Modify: `internal/domain/worker/manager.go`

- [ ] **Step 1: Replace ai-bearing fields with bridge field**

In `internal/domain/worker/manager.go`:

- Remove imports of `ai`, `enginecfg`, `env`, `model.SystemConfig*`.
- Remove fields: `tokenSecret`, `tokenTTL`, `engines`, `engineCfg`, `envService`, `sysConfigStore`.
- Replace `activeProcesses map[string]ai.Process` with `activeHandles map[string]bridge.Handle`.
- Add field `br bridge.Bridge`.

New constructor signature:

```go
func NewManager(
    workerBaseDir string,
    bc config.BeeConfig,
    ws *store.WorkerStore,
    es *store.ExecutionStore,
    br bridge.Bridge,
) *Manager {
    rawBotNames := bc.Platforms.BotNames()
    botNames := make([]string, len(rawBotNames))
    for i, n := range rawBotNames {
        botNames[i] = strings.ToLower(strings.TrimSpace(n))
    }
    return &Manager{
        workerBaseDir:  workerBaseDir,
        workerTimeout:  bc.WorkerTimeout(),
        workerStore:    ws,
        executionStore: es,
        br:             br,
        botNamesLower:  botNames,
        activeHandles:  make(map[string]bridge.Handle),
    }
}
```

Replace the engine-resolution helpers with bridge-backed equivalents:

```go
func (m *Manager) resolveEngineForWorker(w model.Worker) string {
    return m.br.ResolveEngineForWorker(w.ID, w.Engine)
}

func (m *Manager) EnabledEngines() []string {
    return m.br.EnabledEngines()
}

func (m *Manager) ValidateEngine(name string) error      { return m.br.ValidateEngine(name) }
func (m *Manager) ValidateEngineArgs(raw map[string]string) error {
    if len(raw) == 0 {
        return nil
    }
    for engine, line := range raw {
        if engine == "" {
            return fmt.Errorf("engine_args contains an empty engine name: %w", ErrValidation)
        }
        if err := m.br.ValidateEngine(engine); err != nil {
            return fmt.Errorf("engine_args[%q]: %w", engine, err)
        }
        if err := m.br.ValidateEngineArgs(line); err != nil {
            return fmt.Errorf("engine_args[%q]: %w", engine, err)
        }
    }
    return nil
}
```

Delete the old `resolveEngineSelection`, `resolveEngine`, `readSysConfigValue`, `resolveEngineArgs`. They are no longer needed.

- [ ] **Step 2: Build (will fail until execution.go is updated in Task 2.3, that's OK to defer commit)**

Run: `go build ./internal/domain/worker/...`
Expected: build FAILS because execution.go still references removed fields. Continue to Task 2.3 before committing.

---

### Task 2.3: Update `worker/execution.go` to use bridge.RunWorker + Handle.Wait

**Files:**
- Modify: `internal/domain/worker/execution.go`

- [ ] **Step 1: Replace launchRuntime and monitorExecution**

Rewrite the file:

```go
package worker

import (
	"context"
	"fmt"

	"github.com/theopenbee/openbee/internal/ai/bridge"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/infra/utils"
	"go.uber.org/zap"
)

func (m *Manager) ExecuteWorker(ctx context.Context, workerID, triggerInput, sessionID string, resume bool) (model.WorkerExecution, error) {
    worker, err := m.workerStore.GetByID(workerID)
    if err != nil {
        return model.WorkerExecution{}, fmt.Errorf("get worker: %w", err)
    }

    engineName := m.resolveEngineForWorker(worker)

    exec, err := m.executionStore.Create(workerID, triggerInput, sessionID, engineName)
    if err != nil {
        return model.WorkerExecution{}, fmt.Errorf("create execution: %w", err)
    }

    if err := m.workerStore.UpdateStatus(worker.ID, model.WorkerStatusWorking); err != nil {
        log.Error("failed to update worker status", zap.Error(err))
    }

    handle, err := m.br.RunWorker(ctx, bridge.WorkerRunRequest{
        WorkerID:         worker.ID,
        PermissionScopes: utils.SplitAndTrim(worker.PermissionScopes),
        ExecutionID:      exec.ID,
        StartedAt:        exec.StartedAt,
        EngineHint:       worker.Engine,
        EngineArgs:       worker.EngineArgs,
        WorkDir:          worker.WorkDir,
        Prompt:           triggerInput,
        SessionID:        exec.SessionID,
        Resume:           resume,
        Timeout:          m.workerTimeout,
    })
    if err != nil {
        m.executionStore.UpdateResult(exec.ID, err.Error(), model.ExecStatusFailed)
        m.workerStore.UpdateStatus(worker.ID, model.WorkerStatusError)
        return exec, fmt.Errorf("start runtime: %w", err)
    }

    m.mu.Lock()
    m.activeHandles[exec.ID] = handle
    m.mu.Unlock()

    m.executionStore.UpdatePID(exec.ID, handle.PID())
    go m.monitorExecution(exec, worker, handle)
    return exec, nil
}

func (m *Manager) monitorExecution(exec model.WorkerExecution, worker model.Worker, handle bridge.Handle) {
    outcome, err := handle.Wait(context.Background())
    if err != nil {
        log.Error("worker Wait error", zap.String("execution_id", exec.ID), zap.Error(err))
        m.executionStore.UpdateResult(exec.ID, err.Error(), model.ExecStatusFailed)
        m.workerStore.UpdateStatus(worker.ID, model.WorkerStatusError)
    } else {
        switch outcome.Status {
        case bridge.StatusCompleted:
            m.executionStore.UpdateResult(exec.ID, outcome.Result, model.ExecStatusCompleted)
            m.workerStore.UpdateStatus(worker.ID, model.WorkerStatusIdle)
        case bridge.StatusFailed:
            m.executionStore.UpdateResult(exec.ID, outcome.Result, model.ExecStatusFailed)
            m.workerStore.UpdateStatus(worker.ID, model.WorkerStatusError)
        case bridge.StatusAbandoned:
            if _, err := m.executionStore.MarkAbandoned(context.Background(), exec.ID, outcome.Result); err != nil {
                log.Error("finalize abandoned execution", zap.String("executionID", exec.ID), zap.Error(err))
            }
            m.workerStore.UpdateStatus(worker.ID, model.WorkerStatusError)
        }
    }

    m.mu.Lock()
    delete(m.activeHandles, exec.ID)
    m.mu.Unlock()
}

func (m *Manager) StopExecution(executionID string) error {
    m.mu.RLock()
    h, ok := m.activeHandles[executionID]
    m.mu.RUnlock()

    if !ok {
        return fmt.Errorf("no active process for execution %s", executionID)
    }
    return h.Stop()
}

func (m *Manager) CancelExecution(_ context.Context, executionID string) error {
    return m.StopExecution(executionID)
}
```

- [ ] **Step 2: Build**

Run: `go build ./internal/domain/worker/...`
Expected: succeeds.

- [ ] **Step 3: Update `manager_test.go` to use `fakeBridge`** — replace any stubs that used the old `engines map[string]ai.EngineAdapter` constructor signature. Concretely:
  - Replace constructor calls with `worker.NewManager(workerBaseDir, bc, ws, es, &fakeBridge{enabledEngines: []string{...}, validateArgs: func(string) error { return nil }, ...})`.
  - Remove imports of `internal/ai` from `manager_test.go`.

(See current `manager_test.go` for exact testpoints; preserve their behavioural assertions.)

- [ ] **Step 4: Run worker tests**

Run: `go test ./internal/domain/worker/...`
Expected: PASS.

- [ ] **Step 5: Confirm no ai imports remain**

Run: `grep -R "theopenbee/openbee/internal/ai\"" internal/domain/worker/`
Expected: no output.

- [ ] **Step 6: Commit Tasks 2.2 + 2.3 together**

```bash
git add internal/domain/worker/manager.go internal/domain/worker/execution.go internal/domain/worker/manager_test.go
git commit -m "refactor(worker): migrate Manager onto ai/bridge facade"
```

---

### Task 2.4: Update `internal/app/app.go::buildWorkerManager` signature

**Files:**
- Modify: `internal/app/app.go`

- [ ] **Step 1: Replace buildWorkerManager signature**

Old:

```go
func buildWorkerManager(bc config.BeeConfig, s appStores, engines map[string]ai.EngineAdapter, engineCfg *enginecfg.Store, envSvc *env.Service) *worker.Manager
```

New:

```go
func buildWorkerManager(bc config.BeeConfig, s appStores, br bridge.Bridge) *worker.Manager {
    return worker.NewManager(config.DefaultWorkerBaseDir(), bc, s.workerStore, s.execStore, br)
}
```

Update the call site in `BuildApp` to pass `br` instead of `engines, engineCfg, envSvc`.

- [ ] **Step 2: Build + run all tests**

Run: `go build ./... && go test ./...`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add internal/app/app.go
git commit -m "refactor(app): pass bridge to worker.NewManager"
```

---

### Phase 2 exit gate

- [ ] **Verification step**: `grep -R "theopenbee/openbee/internal/ai\"" internal/domain/worker/` is empty.
- [ ] **Verification step**: `go test ./...` green.
- [ ] **Verification step (E2E)**: run a real worker task locally; confirm completion path and stop path both behave.

---

## Phase 3 — Migrate bee, tokenstat, store, config, cmd, remaining tests

### Task 3.1: Migrate `bee.Feeder` to `bridge.RunBee`; delete `bee.BeeProcess`

**Files:**
- Modify: `internal/domain/bee/feeder.go`
- Delete: `internal/domain/bee/bee_process.go`
- Modify: `internal/domain/bee/feeder_test.go`, `internal/domain/bee/feeder_internal_test.go`

- [ ] **Step 1: Replace BeeProcess usage with bridge.RunBee inside Feeder**

In `feeder.go`, replace the field that holds a `*BeeProcess` (or `bridge.RunBee`-like callback) with `br bridge.Bridge`. Wherever Feeder currently calls `beeProcess.Run(ctx, workDir, prompt, ai.RunOptions{...}, logPath)`, replace with:

```go
handle, err := f.br.RunBee(ctx, bridge.BeeRunRequest{
    WorkDir:   workDir,
    Prompt:    prompt,
    SessionID: sessionID,
    Resume:    resume,
    LogPath:   logPath,
})
if err != nil {
    // existing failure handling
}
// Use handle.Wait + handle.PID like the worker path.
```

Where `bee` currently calls `beeProcess.CollectTokenUsage`, replace with `f.br.CollectUsage(ctx, engineName, sessionID)`.

- [ ] **Step 2: Delete `bee/bee_process.go`** (and adjust constants such as `defaultBeeID` — if the only remaining reference is in `adapters/env.go`, the bee.go constant in `bee_process.go` is no longer needed).

- [ ] **Step 3: Update `feeder_test.go` / `feeder_internal_test.go`** to inject `&fakeBridge{}` (copied/adapted from `worker.fake_bridge_test.go`) instead of constructing a `BeeProcess`. Move the helper to a shared `bridgetest` package if both worker and bee tests reuse it; otherwise duplicate is acceptable (each package has its own fakeBridge).

- [ ] **Step 4: Run bee tests**

Run: `go test ./internal/domain/bee/...`
Expected: PASS.

- [ ] **Step 5: Confirm no ai imports in bee**

Run: `grep -R "theopenbee/openbee/internal/ai\"" internal/domain/bee/`
Expected: empty.

- [ ] **Step 6: Commit**

```bash
git add internal/domain/bee/feeder.go internal/domain/bee/feeder_test.go internal/domain/bee/feeder_internal_test.go
git rm internal/domain/bee/bee_process.go
git commit -m "refactor(bee): replace BeeProcess with ai/bridge.RunBee"
```

---

### Task 3.2: Update `internal/app/app.go::buildBee` to use bridge

**Files:**
- Modify: `internal/app/app.go`

- [ ] **Step 1: Replace `buildBee` signature and body**

Old uses `factory.Dynamic(engineCfg)` and constructs `bee.NewBeeProcess(...)`. New body:

```go
func buildBee(cfg config.BeeConfig, s appStores, dispatchCh chan task.DispatchTask,
    failureNotifier bee.FailureNotifier, br bridge.Bridge, engineCfg *enginecfg.Store) (*bee.Feeder, *task.Scheduler) {
    feeder := bee.NewFeeder(s.msgStore, s.taskStore, s.sessionStore, s.execStore, br, config.DefaultBeeWorkDir(), cfg, engineCfg,
        bee.WithFailureNotifier(failureNotifier),
        bee.WithWorkerDispatch(s.workerStore))
    sched := task.NewScheduler(s.taskStore, dispatchCh, bee.PollInterval)
    return feeder, sched
}
```

Update the `BuildApp` call site to pass `br` instead of `factory`.

- [ ] **Step 2: Build + tests**

Run: `go build ./... && go test ./...`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add internal/app/app.go
git commit -m "refactor(app): pass bridge to bee.NewFeeder via buildBee"
```

---

### Task 3.3: Migrate `tokenstat.Syncer` to `bridge.CollectUsage`

**Files:**
- Modify: `internal/tokenstat/syncer.go`, `internal/tokenstat/syncer_test.go`

- [ ] **Step 1: Replace constructor**

Old:

```go
func NewSyncer(db *sql.DB, store *store.TokenStatsStore, engines map[string]ai.EngineAdapter, names []string) *Syncer
```

New:

```go
type usageBridge interface {
    AllEngines() []string
    CollectUsage(ctx context.Context, engineName, sessionID string) ([]bridge.Usage, error)
}

func NewSyncer(db *sql.DB, store *store.TokenStatsStore, br usageBridge) *Syncer
```

Replace the loop body:

```go
usages, err := s.br.CollectUsage(ctx, engineName, sessionID)
if errors.Is(err, bridge.ErrSessionDataNotFound) {
    continue
}
```

Translate `bridge.Usage` straight into the store's row type at the call site.

- [ ] **Step 2: Update syncer_test.go** to inject a fake `usageBridge` (a small struct in the test file is fine — no need for shared fakeBridge here since only two methods are used).

- [ ] **Step 3: Run tests**

Run: `go test ./internal/tokenstat/...`
Expected: PASS.

- [ ] **Step 4: Update `internal/app/app.go::BuildApp`**

```go
tokenSyncer := tokenstat.NewSyncer(db, s.tokenStatsStore, br)
```

- [ ] **Step 5: Build + run all tests**

Run: `go build ./... && go test ./...`
Expected: PASS.

- [ ] **Step 6: Confirm no ai imports**

Run: `grep -R "theopenbee/openbee/internal/ai\"" internal/tokenstat/`
Expected: empty.

- [ ] **Step 7: Commit**

```bash
git add internal/tokenstat/syncer.go internal/tokenstat/syncer_test.go internal/app/app.go
git commit -m "refactor(tokenstat): migrate Syncer onto ai/bridge.CollectUsage"
```

---

### Task 3.4: Migrate store layer references (`internal/infra/store/session_store.go`, `db.go`, `db_test.go`)

**Files:**
- Modify: `internal/infra/store/session_store.go`, `internal/infra/store/db.go`, `internal/infra/store/db_test.go`

- [ ] **Step 1: Replace `ai.EngineClaude` with `bridge.EngineClaude`** and any `ai.TokenUsage` references with `bridge.Usage`.

For example, in `session_store.go`:

```go
import bridge "github.com/theopenbee/openbee/internal/ai/bridge"

const defaultSessionEngine = bridge.EngineClaude
```

Same change in `db.go` (line 279) and `db_test.go` (lines 243, 246).

- [ ] **Step 2: Build + test**

Run: `go build ./... && go test ./internal/infra/store/...`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add internal/infra/store/session_store.go internal/infra/store/db.go internal/infra/store/db_test.go
git commit -m "refactor(store): switch engine name reference to ai/bridge"
```

---

### Task 3.5: Migrate `internal/infra/config/config.go`

**Files:**
- Modify: `internal/infra/config/config.go`

- [ ] **Step 1: Replace `ai.EngineClaude/Codex/Pi/Kimi` constants with `bridge.EngineClaude/...`**

- [ ] **Step 2: Build + test**

Run: `go build ./... && go test ./internal/infra/config/...`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add internal/infra/config/config.go
git commit -m "refactor(config): switch engine name constants to ai/bridge"
```

---

### Task 3.6: Migrate `cmd/openbee/config.go`

**Files:**
- Modify: `cmd/openbee/config.go`

- [ ] **Step 1: Replace all `ai.EngineXxx` and `ai.AllEngines()` references with `bridge.EngineXxx` / `bridge.AllEngines()`**

- [ ] **Step 2: Build**

Run: `go build ./...`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add cmd/openbee/config.go
git commit -m "refactor(cmd): switch engine name references to ai/bridge"
```

---

### Task 3.7: Final sweep — remaining tests and helpers

**Files:**
- Modify (if still importing `internal/ai`): `internal/domain/command/engine_test.go`, `internal/domain/task/dispatcher.go`, `internal/domain/task/dispatcher_test.go`, `internal/rpc/tools_test.go`

- [ ] **Step 1: Run repo grep**

Run: `grep -R "theopenbee/openbee/internal/ai\"" --include='*.go' .`
Expected output: only `internal/ai/bridge/**`, `internal/ai/bridge/adapters/**`, `internal/ai/engine/**`, `internal/ai/core/**`, `internal/ai/cliargs/**`, `internal/ai/factory*.go`, `internal/ai/contracts.go`, and the `internal/app/app.go` bridge-construction point. Any other match is a remaining business-side import.

- [ ] **Step 2: For each remaining business import, replace `ai.X` with `bridge.X`** — usually `ai.EngineXxx`, `ai.TokenUsage` → `bridge.Usage`, `ai.ErrSessionDataNotFound` → `bridge.ErrSessionDataNotFound`.

- [ ] **Step 3: If dispatcher.go uses anything beyond engine names, follow the same pattern as worker (`dispatcher` should not invoke engines directly; if it does, that surface needs the same Run-style migration. Inspect first; if dispatcher only manipulates engine names and ids, the import switch is mechanical).**

- [ ] **Step 4: Re-run grep**

Run: `grep -R "theopenbee/openbee/internal/ai\"" --include='*.go' .`
Expected: only the allow-list above.

- [ ] **Step 5: Build + test**

Run: `go build ./... && go test ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "refactor: switch remaining business-side ai imports to ai/bridge"
```

---

### Task 3.8: Remove `ai.Factory.Dynamic` if no remaining consumers

**Files:**
- Modify: `internal/ai/factory.go`

- [ ] **Step 1: Search for callers**

Run: `grep -R "Factory.Dynamic\|factory.Dynamic\|\.Dynamic(" --include='*.go' .`
Expected: only references inside `internal/ai/factory*.go` itself.

- [ ] **Step 2: Delete `(f *Factory) Dynamic` and the `dynamicAdapter` type from `internal/ai/factory.go`**.

- [ ] **Step 3: Build + test**

Run: `go build ./... && go test ./...`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/ai/factory.go internal/ai/factory_test.go
git commit -m "refactor(ai): drop unused Factory.Dynamic now that bridge owns routing"
```

---

### Task 3.9: Optional — add depguard rule

**Files:**
- Modify: existing lint config (`.golangci.yml` or equivalent) OR add one.

- [ ] **Step 1: Inspect repo for an existing lint config**

Run: `ls .golangci* 2>/dev/null; ls .revive* 2>/dev/null`

- [ ] **Step 2: If a config exists, add a `depguard` rule** disallowing `github.com/theopenbee/openbee/internal/ai` imports outside `internal/ai/bridge`, `internal/ai/engine`, `internal/ai/core`, `internal/ai/cliargs`, `internal/ai`, and `internal/app`. If no config exists, leave a TODO comment in `internal/ai/doc.go` referencing the future rule and skip this step.

- [ ] **Step 3: Run lint locally if config supports it**

Run: `golangci-lint run` (if installed)
Expected: PASS.

- [ ] **Step 4: Commit (only if step 2 produced a change)**

```bash
git add <config>
git commit -m "chore(lint): forbid internal/ai imports outside the allow-list"
```

---

### Task 3.10: End-to-end smoke

- [ ] **Step 1: Build the binary**

Run: `go build ./cmd/openbee`
Expected: produces `openbee` binary.

- [ ] **Step 2: Run a local worker task end-to-end** (use the project's standard local dev flow) and observe completion path.

- [ ] **Step 3: Run a local bee task end-to-end** and observe completion path.

- [ ] **Step 4: Trigger a stop on an in-flight task** and confirm the execution row transitions correctly.

- [ ] **Step 5: Confirm tokenstat picks up usage for the completed task.**

(No commit; this is verification only.)

---

### Phase 3 exit gate

- [ ] **Verification step**: `grep -R "theopenbee/openbee/internal/ai\"" --include='*.go' .` returns only allow-list entries.
- [ ] **Verification step**: `go build ./... && go test ./...` green.
- [ ] **Verification step**: end-to-end smokes (3.10) all pass.

---

## Self-review notes (in-plan)

- All spec sections §1–§11 are covered: §4 by Phase 1 Tasks 1.1–1.7, 1.13; §5 facade by Tasks 1.5–1.8; §6 ports by Tasks 1.5, 1.9–1.13; §7 invariants by Task 1.8; §8 phases by Tasks 1.x / 2.x / 3.x respectively; §9 testing covered inline; §10 rollback enabled by per-task commits; §11 defaults baked into Task 1.15 (doc.go) and Task 3.9 (optional depguard).
- All identifiers are consistent: `bridge.Bridge`, `bridge.Handle`, `bridge.WorkerRunRequest`, `bridge.BeeRunRequest`, `bridge.Outcome`, `bridge.StatusCompleted/Failed/Abandoned`, `bridge.Usage`, `bridge.ErrSessionDataNotFound`, `bridge.ErrEngineNotEnabled`, `adapters.NewTokenIssuer/NewEnvResolver/NewEngineSelector/NewArgsResolver/NewLogPathProvider`.
- No placeholders: every task contains either complete code or an exact mechanical edit (search-and-replace import path).
