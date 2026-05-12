# Unified `BaseAdapter` Interface Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the three `core` engine-side interfaces (`Invoker` + `Collector` + `Extractor`) and the `BaseAdapter struct` with a single `core.BaseAdapter` interface exposing `Run` / `Collect` / `Extract`. Each engine collapses its three small structs into one `Backend` type in `backend.go`.

**Architecture:** Stage the change so the build and tests stay green between tasks. (1) Rename the existing `core.BaseAdapter` struct to a transitional `core.Composite` and update its four engine callers. (2) Introduce the new `core.BaseAdapter` interface plus `core.NewEngineAdapter` wrapper alongside the transitional struct. (3) Migrate engines one at a time to `Backend` + `core.NewEngineAdapter`. For Claude, fold `cleanupLegacyRules` into `Backend.Run` and delete the `claudeAdapter` wrapper. (4) Once every engine is migrated, delete the transitional `Composite` struct and the orphan `Invoker` / `Collector` / `Extractor` interfaces.

**Tech Stack:** Go, `internal/ai/core`, `internal/ai/engine/{claude,codex,pi,kimi}`. Spec: `docs/superpowers/specs/2026-05-12-base-adapter-unified-interface-design.md`.

**Working directory:** repository root.

---

## File Map

**Modified throughout the migration:**
- `internal/ai/core/adapter.go` — rewritten across Tasks 1, 2, 7.
- `internal/ai/core/adapter_test.go` — updated in Tasks 1, 2, 7.
- `internal/ai/engine/codex/{invoker.go, token_usage.go, adapter.go}` → new `backend.go` (Task 3).
- `internal/ai/engine/pi/{invoker.go, token_usage.go, adapter.go}` → new `backend.go` (Task 4).
- `internal/ai/engine/kimi/{invoker.go, token_usage.go, adapter.go}` → new `backend.go` (Task 5).
- `internal/ai/engine/claude/{invoker.go, token_usage.go, adapter.go}` → new `backend.go`; `claudeAdapter` removed (Task 6).
- Engine `invoker_test.go` / `token_usage_test.go` — constructor renames only.

---

## Task 1: Rename `core.BaseAdapter` struct → `core.Composite` (transitional)

**Why:** Frees the name `BaseAdapter` so it can be reused as an interface in Task 2 without a same-package name collision. Keeps the build green.

**Files:**
- Modify: `internal/ai/core/adapter.go`
- Modify: `internal/ai/core/adapter_test.go`
- Modify: `internal/ai/engine/claude/adapter.go`
- Modify: `internal/ai/engine/codex/adapter.go`
- Modify: `internal/ai/engine/pi/adapter.go`
- Modify: `internal/ai/engine/kimi/adapter.go`

- [ ] **Step 1.1: Rename type and methods in `internal/ai/core/adapter.go`**

Replace the struct + method block (lines 23–47 of the current file) so the struct is now named `Composite`:

```go
// Composite is the transitional struct previously named BaseAdapter. It is
// being replaced by the BaseAdapter interface and will be removed once every
// engine migrates to core.NewEngineAdapter. New code MUST NOT reference it.
type Composite struct {
    Invoker   Invoker
    Collector Collector
    Extractor Extractor
}

// Run launches the invoker and binds the extractor to logPath in the returned RunResult.
func (b *Composite) Run(ctx context.Context, workDir, prompt string,
    opts ai.RunOptions, logPath string) (ai.RunResult, error) {
    proc, out, err := b.Invoker.Run(ctx, workDir, prompt, opts, logPath)
    return ai.NewRunResult(proc, out, err, func() string {
        return b.Extractor.Extract(logPath)
    })
}

// CollectTokenUsage delegates to the embedded collector.
func (b *Composite) CollectTokenUsage(ctx context.Context, sessionID string) ([]ai.TokenUsage, error) {
    return b.Collector.Collect(ctx, sessionID)
}
```

Leave `Invoker`, `Collector`, `Extractor` interfaces unchanged. Their package-level doc comments stay.

- [ ] **Step 1.2: Update `internal/ai/core/adapter_test.go`**

Replace the three `&core.BaseAdapter{...}` literals (lines 36, 55, 68) with `&core.Composite{...}`. Field names and rest of the file are unchanged.

Run: `go test ./internal/ai/core/...`
Expected: PASS.

- [ ] **Step 1.3: Update `internal/ai/engine/codex/adapter.go`**

```go
return &core.Composite{
    Invoker:   NewInvoker(binaryPath, store, extraEnv),
    Collector: NewCollector(),
    Extractor: Extractor{},
}, nil
```

- [ ] **Step 1.4: Update `internal/ai/engine/pi/adapter.go`**

Replace `&core.BaseAdapter{...}` with `&core.Composite{...}`. Field literal contents unchanged.

- [ ] **Step 1.5: Update `internal/ai/engine/kimi/adapter.go`**

Replace `&core.BaseAdapter{...}` with `&core.Composite{...}`. Field literal contents unchanged.

- [ ] **Step 1.6: Update `internal/ai/engine/claude/adapter.go`**

Three edits in this file:

```go
// claudeAdapter embeds core.Composite and wraps Run to clean up the legacy
// openbee rules file and matching import line in CLAUDE.md before each run.
type claudeAdapter struct {
    *core.Composite
}

// NewAdapter constructs a Claude engine adapter.
func NewAdapter(binaryPath string, extraEnv map[string]string) ai.EngineAdapter {
    return &claudeAdapter{
        Composite: &core.Composite{
            Invoker:   NewInvoker(binaryPath, extraEnv),
            Collector: NewCollector(),
            Extractor: Extractor{},
        },
    }
}

// Run cleans up legacy openbee rules before delegating to the embedded Composite.
func (a *claudeAdapter) Run(ctx context.Context, workDir, prompt string,
    opts ai.RunOptions, logPath string) (ai.RunResult, error) {
    if err := cleanupLegacyRules(workDir); err != nil {
        return ai.RunResult{}, err
    }
    return a.Composite.Run(ctx, workDir, prompt, opts, logPath)
}
```

The field name in `claudeAdapter` must change from `BaseAdapter` to `Composite` because Go embedded-field names follow the type name. The call site `a.BaseAdapter.Run(...)` becomes `a.Composite.Run(...)`.

- [ ] **Step 1.7: Build + test the whole module**

Run:
```bash
go build ./...
go test ./internal/ai/...
```
Expected: both green.

- [ ] **Step 1.8: Commit**

```bash
git add internal/ai/core/adapter.go internal/ai/core/adapter_test.go \
        internal/ai/engine/claude/adapter.go internal/ai/engine/codex/adapter.go \
        internal/ai/engine/pi/adapter.go internal/ai/engine/kimi/adapter.go
git commit -m "refactor(ai/core): rename BaseAdapter struct to Composite (transitional)"
```

---

## Task 2: Add new `core.BaseAdapter` interface and `core.NewEngineAdapter`

**Why:** Lands the new contract and wrapper alongside the transitional `Composite`. After this task the module exposes both the legacy struct (still used by all four engines) and the new interface (used by no one yet).

**Files:**
- Modify: `internal/ai/core/adapter.go`
- Modify: `internal/ai/core/adapter_test.go`

- [ ] **Step 2.1: Append the new interface + wrapper to `internal/ai/core/adapter.go`**

Add at the bottom of the file:

```go
// BaseAdapter is the unified engine-side contract. Engine packages provide a
// single type (typically named Backend) satisfying this interface.
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

// NewEngineAdapter wraps a BaseAdapter to satisfy ai.EngineAdapter. The
// wrapper binds Extract to logPath in the returned ai.RunResult and delegates
// CollectTokenUsage to Collect.
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

Existing imports already cover `context` and `ai`; no import additions needed.

- [ ] **Step 2.2: Write failing tests for the new wrapper in `internal/ai/core/adapter_test.go`**

Append at the bottom of the existing file (after the last test):

```go
type fakeBackend struct {
    ch              <-chan ai.Output
    proc            ai.Process
    runErr          error
    capturedLogPath *string
    extractResult   string
    collectUsages   []ai.TokenUsage
    collectErr      error
}

func (f *fakeBackend) Run(ctx context.Context, workDir, prompt string,
    opts ai.RunOptions, logPath string) (ai.Process, <-chan ai.Output, error) {
    return f.proc, f.ch, f.runErr
}

func (f *fakeBackend) Collect(ctx context.Context, sessionID string) ([]ai.TokenUsage, error) {
    return f.collectUsages, f.collectErr
}

func (f *fakeBackend) Extract(logPath string) string {
    if f.capturedLogPath != nil {
        *f.capturedLogPath = logPath
    }
    return f.extractResult
}

func TestNewEngineAdapter_RunBindsExtract(t *testing.T) {
    ch := make(chan ai.Output)
    close(ch)
    var capturedLogPath string
    a := core.NewEngineAdapter(&fakeBackend{
        ch:              ch,
        capturedLogPath: &capturedLogPath,
        extractResult:   "x",
    })
    res, err := a.Run(context.Background(), "/wd", "p", ai.RunOptions{}, "/the/log")
    if err != nil {
        t.Fatal(err)
    }
    if r := res.ExtractResult(); r != "x" {
        t.Errorf("got %q", r)
    }
    if capturedLogPath != "/the/log" {
        t.Errorf("logPath not bound; got %q", capturedLogPath)
    }
}

func TestNewEngineAdapter_RunPropagatesError(t *testing.T) {
    wantErr := errors.New("boom")
    a := core.NewEngineAdapter(&fakeBackend{runErr: wantErr})
    _, err := a.Run(context.Background(), "/wd", "", ai.RunOptions{}, "/log")
    if !errors.Is(err, wantErr) {
        t.Errorf("want wantErr, got %v", err)
    }
}

func TestNewEngineAdapter_CollectDelegates(t *testing.T) {
    want := []ai.TokenUsage{{Model: "m", InputTokens: 7}}
    a := core.NewEngineAdapter(&fakeBackend{collectUsages: want})
    got, err := a.CollectTokenUsage(context.Background(), "sid")
    if err != nil {
        t.Fatal(err)
    }
    if len(got) != 1 || got[0].Model != "m" || got[0].InputTokens != 7 {
        t.Errorf("delegation broken; got %+v", got)
    }
}
```

- [ ] **Step 2.3: Run the new tests**

```bash
go test ./internal/ai/core/ -run TestNewEngineAdapter -v
```
Expected: 3 tests PASS.

- [ ] **Step 2.4: Run the full core + engine test suite**

```bash
go test ./internal/ai/...
```
Expected: PASS. (Old tests use `core.Composite`, new tests use `core.NewEngineAdapter`. Both should be green.)

- [ ] **Step 2.5: Commit**

```bash
git add internal/ai/core/adapter.go internal/ai/core/adapter_test.go
git commit -m "feat(ai/core): add unified BaseAdapter interface and NewEngineAdapter wrapper"
```

---

## Task 3: Migrate codex to `Backend` + `core.NewEngineAdapter`

**Why:** Codex has the cleanest shape (no special pre-Run logic) and exercises the migration template the remaining engines will follow.

**Files:**
- Create: `internal/ai/engine/codex/backend.go`
- Delete: `internal/ai/engine/codex/invoker.go`
- Delete: `internal/ai/engine/codex/token_usage.go`
- Modify: `internal/ai/engine/codex/adapter.go`
- Modify: `internal/ai/engine/codex/invoker_test.go`
- Modify: `internal/ai/engine/codex/token_usage_test.go`

- [ ] **Step 3.1: Create `internal/ai/engine/codex/backend.go`**

Move the full content of `invoker.go` (package decl, imports, log var, constants, `switchableWriter`, types, helpers, `Extractor` type) and the full content of `token_usage.go` (Collector type, defaults, constants, types, `Collect`, `findCodexSessionFile`, `parseCodexFile`, `addCodexUsage`, `codexResolveModel`) into one file, then merge into a single `Backend` type.

The result of the merge:

```go
package codex

import (
    "context"
    "encoding/json"
    "errors"
    "fmt"
    "io"
    "io/fs"
    "os"
    "os/exec"
    "path/filepath"
    "strings"
    "sync"

    ai "github.com/theopenbee/openbee/internal/ai"
    core "github.com/theopenbee/openbee/internal/ai/core"
    "github.com/theopenbee/openbee/internal/infra/config"
    "github.com/theopenbee/openbee/internal/infra/logger"
    "github.com/theopenbee/openbee/internal/utils/sessionfile"
    "go.uber.org/zap"
)

var log = logger.With(zap.String("component", "codex"))

const (
    codexEventThreadStarted = "thread.started"
    codexEventItemCompleted = "item.completed"
    codexItemAgentMessage   = "agent_message"
)

// (paste switchableWriter type + Write + Detach methods verbatim from invoker.go:29-50)

// (paste codexEvent, codexItem types verbatim from invoker.go:65-79)

// Backend is the codex engine implementation of core.BaseAdapter. It spawns the
// Codex CLI process, extracts the final agent_message from the JSON log, and
// reads token-usage data from the session JSONL written by the CLI.
type Backend struct {
    binary     string
    baseEnv    []string
    store      *SessionStore
    mappingDir string
    codexBase  string
}

// NewBackend builds a codex Backend. extraEnv entries are merged into the
// base environment at lowest priority. OPENBEE_URL is inherited from the
// server process environment.
func NewBackend(binary string, store *SessionStore, extraEnv map[string]string) *Backend {
    return &Backend{
        binary:     binary,
        baseEnv:    core.NewBaseEnv(extraEnv),
        store:      store,
        mappingDir: config.DefaultCodexSessionsDir(),
        codexBase:  config.EngineSessionsDir("CODEX_HOME", defaultCodexBase),
    }
}

// NewBackendAt is a test seam allowing arbitrary mapping/codex roots.
func NewBackendAt(binary string, store *SessionStore, extraEnv map[string]string,
    mappingDir, codexBase string) *Backend {
    return &Backend{
        binary:     binary,
        baseEnv:    core.NewBaseEnv(extraEnv),
        store:      store,
        mappingDir: mappingDir,
        codexBase:  codexBase,
    }
}

func defaultCodexBase() string {
    home, _ := os.UserHomeDir()
    return filepath.Join(home, ".codex")
}

// (paste buildArgs verbatim from invoker.go:81-92)

// (paste extractSessionID verbatim from invoker.go:96-110)

// Extract returns the text of the last agent_message item from the Codex JSON log.
func (b *Backend) Extract(logPath string) string {
    // (paste body of (Extractor).Extract from invoker.go:117-137)
}

// Run starts a Codex CLI process, redirecting output to logPath.
func (b *Backend) Run(ctx context.Context, workDir, prompt string,
    opts ai.RunOptions, logPath string) (ai.Process, <-chan ai.Output, error) {
    // (paste body of (*Invoker).Run from invoker.go:141-203, replacing every `inv` with `b`)
}

func (b *Backend) resolveThread(openbeeUUID string, resume bool) (threadID string, resolvedResume bool) {
    // (paste body of (*Invoker).resolveThread from invoker.go:205-215, replacing `inv` with `b`)
}

// (paste codexLineTurnContext, codexLineEventMsg, codexPayloadTokens const block from token_usage.go:45-49)

// (paste codexJSONLLine, codexTokenInfo, codexTokenUsage types + methods verbatim from token_usage.go:51-98)

// Collect reads the openbee-UUID → codex-thread-ID mapping written by
// SessionStore, then aggregates token usage from the codex session JSONL.
func (b *Backend) Collect(_ context.Context, sessionID string) ([]ai.TokenUsage, error) {
    // (paste body of (*Collector).Collect from token_usage.go:100-120, replacing `c.mappingDir` with `b.mappingDir` and `c.codexBase` with `b.codexBase`)
}

// (paste findCodexSessionFile verbatim from token_usage.go:122-130)
// (paste parseCodexFile verbatim from token_usage.go:132-181)
// (paste addCodexUsage verbatim from token_usage.go:183-187)
// (paste codexResolveModel verbatim from token_usage.go:189-200)
```

Key notes for the move:
- `Invoker` struct + `NewInvoker` are deleted. Their receiver `inv` becomes `b` on `Backend`.
- `Extractor struct{}` + `(Extractor).Extract` is deleted. Becomes `(*Backend).Extract`.
- `Collector` struct + `NewCollector` + `NewCollectorAt` are deleted. Their two fields (`mappingDir`, `codexBase`) move into `Backend`.
- `defaultCodexBase` stays as a package-level helper.

- [ ] **Step 3.2: Delete `internal/ai/engine/codex/invoker.go` and `internal/ai/engine/codex/token_usage.go`**

```bash
git rm internal/ai/engine/codex/invoker.go internal/ai/engine/codex/token_usage.go
```

- [ ] **Step 3.3: Rewrite `internal/ai/engine/codex/adapter.go`**

```go
package codex

import (
    "fmt"

    ai "github.com/theopenbee/openbee/internal/ai"
    core "github.com/theopenbee/openbee/internal/ai/core"
)

func init() {
    ai.Register(ai.EngineCodex, func(cfg ai.EngineConfig) (ai.EngineAdapter, error) {
        return NewAdapter(cfg.PathOrDefault(ai.EngineCodex), cfg.ExtraEnv())
    })
}

// NewAdapter constructs a Codex engine adapter.
func NewAdapter(binaryPath string, extraEnv map[string]string) (ai.EngineAdapter, error) {
    store, err := NewSessionStore()
    if err != nil {
        return nil, fmt.Errorf("init codex session store: %w", err)
    }
    return core.NewEngineAdapter(NewBackend(binaryPath, store, extraEnv)), nil
}
```

- [ ] **Step 3.4: Update `internal/ai/engine/codex/invoker_test.go`**

Open the file and locate every construction of `Invoker` and `Extractor`:

```bash
grep -n "NewInvoker\|Invoker{\|Extractor{}" internal/ai/engine/codex/invoker_test.go
```

For each match:
- Replace `codex.NewInvoker(...)` (or `NewInvoker(...)` inside the same package) with `codex.NewBackend(...)` (or `NewBackend(...)`).
- Replace `&codex.Invoker{...}` literals with `&codex.Backend{...}` (field names are identical so values transfer 1:1; if test code touches `binary`, `baseEnv`, `store`, those are still valid Backend fields).
- Replace `Extractor{}.Extract(p)` with `(&Backend{}).Extract(p)` — Extract has no Backend-state dependency so a zero-value receiver is fine. If a test was constructing an `Invoker` and separately calling `Extractor{}.Extract`, prefer reusing the same `Backend` instance.
- Type assertions `var _ ai.EngineAdapter = ...` that pointed at `&Invoker{}` should be removed (Backend no longer claims to be an EngineAdapter — it's a BaseAdapter; the EngineAdapter wrapper is built by `NewAdapter`).

- [ ] **Step 3.5: Update `internal/ai/engine/codex/token_usage_test.go`**

For each `NewCollector(...)` or `NewCollectorAt(...)` call:
- `NewCollector()` → `NewBackend("", nil, nil)` — but Collect only reads `mappingDir` / `codexBase`, which need to be set. Prefer using `NewBackendAt("", nil, nil, mappingDir, codexBase)` everywhere.
- `NewCollectorAt(mappingDir, codexBase)` → `NewBackendAt("", nil, nil, mappingDir, codexBase)`.

Assertions on the value returned by `Collect` stay unchanged.

If the test asserts a concrete type via `var _ ai.EngineAdapter` or similar, drop that — Backend is not an `ai.EngineAdapter`, only a `core.BaseAdapter`. If you want to keep a static-typing assertion, use:

```go
var _ core.BaseAdapter = (*Backend)(nil)
```

- [ ] **Step 3.6: Run codex tests**

```bash
go test ./internal/ai/engine/codex/... -v
```
Expected: PASS for every previously-passing test.

- [ ] **Step 3.7: Run full ai test suite**

```bash
go build ./...
go test ./internal/ai/...
```
Expected: PASS.

- [ ] **Step 3.8: Commit**

```bash
git add internal/ai/engine/codex/
git commit -m "refactor(ai/codex): collapse Invoker+Collector+Extractor into Backend"
```

---

## Task 4: Migrate pi to `Backend` + `core.NewEngineAdapter`

**Why:** Same shape as codex, no session store wrinkle.

**Files:**
- Create: `internal/ai/engine/pi/backend.go`
- Delete: `internal/ai/engine/pi/invoker.go`
- Delete: `internal/ai/engine/pi/token_usage.go`
- Modify: `internal/ai/engine/pi/adapter.go`
- Modify: `internal/ai/engine/pi/invoker_test.go`
- Modify: `internal/ai/engine/pi/invoker_bench_test.go`
- Modify: `internal/ai/engine/pi/token_usage_test.go`

- [ ] **Step 4.1: Read the current pi sources**

```bash
cat internal/ai/engine/pi/invoker.go
cat internal/ai/engine/pi/token_usage.go
cat internal/ai/engine/pi/adapter.go
```
Inventory: identify the `Invoker` struct's fields, the `Collector` struct's fields (if any), the `Extractor` definition, and all helpers/types.

- [ ] **Step 4.2: Create `internal/ai/engine/pi/backend.go`**

Follow the codex template from Task 3.1:

```go
package pi

// (imports — union of invoker.go and token_usage.go imports)

// (paste invoker.go's package-level types, constants, and helper functions verbatim — everything outside the Invoker/Extractor type bodies)

// Backend is the pi engine implementation of core.BaseAdapter.
type Backend struct {
    // Union of the fields previously held by Invoker, Collector (if any non-empty).
    // For pi: typically binary + baseEnv (no SessionStore equivalent).
    binary  string
    baseEnv []string
    // ... plus any Collector-side fields if Collector is not stateless.
}

// NewBackend constructs a pi Backend.
func NewBackend(binary string, extraEnv map[string]string) *Backend {
    return &Backend{binary: binary, baseEnv: core.NewBaseEnv(extraEnv)}
}

// Run — paste the body of (*Invoker).Run; replace `inv` with `b`.
func (b *Backend) Run(ctx context.Context, workDir, prompt string,
    opts ai.RunOptions, logPath string) (ai.Process, <-chan ai.Output, error) {
    // ...
}

// Collect — paste the body of (*Collector).Collect; replace receiver name.
func (b *Backend) Collect(ctx context.Context, sessionID string) ([]ai.TokenUsage, error) {
    // ...
}

// Extract — paste the body of (Extractor).Extract.
func (b *Backend) Extract(logPath string) string {
    // ...
}

// (paste remaining package-level helper functions from token_usage.go: parse helpers, model resolution, JSONL types, etc.)
```

If `Collector` has its own state (e.g. a `mappingDir` field for finding session files), add those fields to `Backend` and update `NewBackend` (or add a `NewBackendAt` test seam similar to codex).

- [ ] **Step 4.3: Delete the old files**

```bash
git rm internal/ai/engine/pi/invoker.go internal/ai/engine/pi/token_usage.go
```

- [ ] **Step 4.4: Rewrite `internal/ai/engine/pi/adapter.go`**

```go
package pi

import (
    ai "github.com/theopenbee/openbee/internal/ai"
    core "github.com/theopenbee/openbee/internal/ai/core"
)

func init() {
    ai.Register(ai.EnginePi, func(cfg ai.EngineConfig) (ai.EngineAdapter, error) {
        return NewAdapter(cfg.PathOrDefault(ai.EnginePi), cfg.ExtraEnv()), nil
    })
}

// NewAdapter constructs a pi engine adapter.
func NewAdapter(binaryPath string, extraEnv map[string]string) ai.EngineAdapter {
    return core.NewEngineAdapter(NewBackend(binaryPath, extraEnv))
}
```

Note: the existing pi `NewAdapter` signature may differ slightly. Match the file's current return-type signature; only the body changes.

- [ ] **Step 4.5: Update pi tests**

Same procedure as Task 3.4 / 3.5:
- `NewInvoker(...)` → `NewBackend(...)`
- `NewCollector(...)` → `NewBackend(...)` (or `NewBackendAt(...)` if a test seam is needed for the Collector's state)
- `Extractor{}.Extract(p)` → `(&Backend{}).Extract(p)`

Apply across `invoker_test.go`, `invoker_bench_test.go`, and `token_usage_test.go`.

- [ ] **Step 4.6: Test**

```bash
go test ./internal/ai/engine/pi/... -v
go build ./...
go test ./internal/ai/...
```
Expected: PASS.

- [ ] **Step 4.7: Commit**

```bash
git add internal/ai/engine/pi/
git commit -m "refactor(ai/pi): collapse Invoker+Collector+Extractor into Backend"
```

---

## Task 5: Migrate kimi to `Backend` + `core.NewEngineAdapter`

**Why:** Same shape as pi.

**Files:**
- Create: `internal/ai/engine/kimi/backend.go`
- Delete: `internal/ai/engine/kimi/invoker.go`
- Delete: `internal/ai/engine/kimi/token_usage.go`
- Modify: `internal/ai/engine/kimi/adapter.go`
- Modify: `internal/ai/engine/kimi/invoker_test.go`
- Modify: `internal/ai/engine/kimi/token_usage_test.go`

- [ ] **Step 5.1: Read the current kimi sources**

```bash
cat internal/ai/engine/kimi/invoker.go
cat internal/ai/engine/kimi/token_usage.go
cat internal/ai/engine/kimi/adapter.go
```
Inventory: the Invoker fields, Collector state (if any), Extractor's `Extract` body, plus helpers (`extractSentMsg`, `heredocRe`, kimi-specific JSON types).

- [ ] **Step 5.2: Create `internal/ai/engine/kimi/backend.go`**

Apply the same template as Task 4.2. Important kimi specifics to preserve:
- The `extractSentMsg` helper and `heredocRe` regex remain package-level.
- The `kimiMessage`, `kimiContentBlock`, `kimiToolCall` types remain package-level.
- The Extract method's behaviour (returning heredoc body when the assistant ends with `(Empty response:`) must be preserved verbatim — only the receiver changes.

- [ ] **Step 5.3: Delete the old files**

```bash
git rm internal/ai/engine/kimi/invoker.go internal/ai/engine/kimi/token_usage.go
```

- [ ] **Step 5.4: Rewrite `internal/ai/engine/kimi/adapter.go`**

```go
package kimi

import (
    ai "github.com/theopenbee/openbee/internal/ai"
    core "github.com/theopenbee/openbee/internal/ai/core"
)

func init() {
    ai.Register(ai.EngineKimi, func(cfg ai.EngineConfig) (ai.EngineAdapter, error) {
        return NewAdapter(cfg.PathOrDefault(ai.EngineKimi), cfg.ExtraEnv()), nil
    })
}

// NewAdapter constructs a kimi engine adapter.
func NewAdapter(binaryPath string, extraEnv map[string]string) ai.EngineAdapter {
    return core.NewEngineAdapter(NewBackend(binaryPath, extraEnv))
}
```

Match the existing kimi `NewAdapter` signature; only the body changes.

- [ ] **Step 5.5: Update kimi tests**

Same constructor-rename pattern as Task 4.5.

- [ ] **Step 5.6: Test**

```bash
go test ./internal/ai/engine/kimi/... -v
go build ./...
go test ./internal/ai/...
```
Expected: PASS.

- [ ] **Step 5.7: Commit**

```bash
git add internal/ai/engine/kimi/
git commit -m "refactor(ai/kimi): collapse Invoker+Collector+Extractor into Backend"
```

---

## Task 6: Migrate claude — fold `cleanupLegacyRules` into `Backend.Run`, delete `claudeAdapter`

**Why:** Claude is the only engine that wraps Run with side-effects. After the refactor there is no `Composite` to embed, so the cleanest path is to make cleanup part of `claude.Backend.Run` and delete the wrapper struct.

**Files:**
- Create: `internal/ai/engine/claude/backend.go`
- Delete: `internal/ai/engine/claude/invoker.go`
- Delete: `internal/ai/engine/claude/token_usage.go`
- Modify: `internal/ai/engine/claude/adapter.go` (claudeAdapter and its Run method removed; only `init()` and `NewAdapter` remain)
- Modify: `internal/ai/engine/claude/invoker_test.go`
- Modify: `internal/ai/engine/claude/invoker_unix_test.go`
- Modify: `internal/ai/engine/claude/token_usage_test.go`
- Modify: `internal/ai/engine/claude/adapter_test.go`

- [ ] **Step 6.1: Read the current claude sources**

```bash
cat internal/ai/engine/claude/invoker.go
cat internal/ai/engine/claude/token_usage.go
cat internal/ai/engine/claude/adapter.go
```

- [ ] **Step 6.2: Create `internal/ai/engine/claude/backend.go`**

Follow the template from Task 3. Additionally, move `cleanupLegacyRules`, `removeImportLine`, and the `systemRulesFile` / `importLine` constants from `adapter.go` into `backend.go` (they are claude-Run helpers).

`Backend.Run` becomes:

```go
// Run cleans up legacy openbee rules before launching the Claude CLI process.
func (b *Backend) Run(ctx context.Context, workDir, prompt string,
    opts ai.RunOptions, logPath string) (ai.Process, <-chan ai.Output, error) {
    if err := cleanupLegacyRules(workDir); err != nil {
        return nil, nil, err
    }
    // (paste body of original (*Invoker).Run verbatim; replace `inv` with `b`)
}
```

Note: the original `Invoker.Run` returned `(ai.Process, <-chan ai.Output, error)`. The wrapped `claudeAdapter.Run` returned `(ai.RunResult, error)`. The new `Backend.Run` keeps the core-level signature — so cleanup-failure path returns `(nil, nil, err)`, not `(ai.RunResult{}, err)`.

- [ ] **Step 6.3: Delete the old per-concern files**

```bash
git rm internal/ai/engine/claude/invoker.go internal/ai/engine/claude/token_usage.go
```

- [ ] **Step 6.4: Rewrite `internal/ai/engine/claude/adapter.go`**

```go
package claude

import (
    ai "github.com/theopenbee/openbee/internal/ai"
    core "github.com/theopenbee/openbee/internal/ai/core"
)

func init() {
    ai.Register(ai.EngineClaude, func(cfg ai.EngineConfig) (ai.EngineAdapter, error) {
        return NewAdapter(cfg.PathOrDefault(ai.EngineClaude), cfg.ExtraEnv()), nil
    })
}

// NewAdapter constructs a Claude engine adapter.
func NewAdapter(binaryPath string, extraEnv map[string]string) ai.EngineAdapter {
    return core.NewEngineAdapter(NewBackend(binaryPath, extraEnv))
}
```

The `claudeAdapter` type, its `Run` method, `cleanupLegacyRules`, `removeImportLine`, and the related constants are removed from this file. (Constants and cleanup helpers live in `backend.go` now.)

- [ ] **Step 6.5: Update claude tests**

`invoker_test.go` / `invoker_unix_test.go` / `token_usage_test.go`:
- `NewInvoker(...)` → `NewBackend(...)`.
- `Extractor{}.Extract(p)` → `(&Backend{}).Extract(p)`.
- `NewCollector(...)` → `NewBackend(...)` (or `NewBackendAt(...)` if Collector has state and tests need a seam).

`adapter_test.go`:
- The existing `TestCleanupLegacyRules_*` tests call `cleanupLegacyRules(dir)` directly. Since `cleanupLegacyRules` moved from `adapter.go` to `backend.go` but stayed package-private with the same name and signature, these tests don't need to change.
- If any test referenced `claudeAdapter` directly, delete or rewrite to use the public surface (`NewAdapter(...)` returns an `ai.EngineAdapter`; the underlying cleanup is exercised via `(&Backend{...}).Run(...)` or the existing `cleanupLegacyRules(dir)` direct calls).

- [ ] **Step 6.6: Test**

```bash
go test ./internal/ai/engine/claude/... -v
go build ./...
go test ./internal/ai/...
```
Expected: PASS.

- [ ] **Step 6.7: Commit**

```bash
git add internal/ai/engine/claude/
git commit -m "refactor(ai/claude): collapse Invoker+Collector+Extractor into Backend, fold cleanup into Run"
```

---

## Task 7: Remove transitional `Composite` struct and orphan interfaces

**Why:** At this point no code references `core.Composite`, `core.Invoker`, `core.Collector`, or `core.Extractor`. Delete them so the public surface of `core` is exactly: `BaseAdapter` interface + `NewEngineAdapter` + the existing helpers (`NewBaseEnv`, `BuildRunEnv`, `ScanJSONLines`, `AggregateUsage`, etc., which live in other files).

**Files:**
- Modify: `internal/ai/core/adapter.go`
- Modify: `internal/ai/core/adapter_test.go`

- [ ] **Step 7.1: Verify nothing references the transitional types**

```bash
grep -rn "core\.Composite\|core\.Invoker\b\|core\.Collector\b\|core\.Extractor\b" internal/
```
Expected: zero hits (the `core.Invoker`/`core.Collector`/`core.Extractor` doc-comment references inside engine files were removed in Tasks 3–6 along with the methods they annotated).

If anything is still found, stop and resolve the reference before proceeding.

- [ ] **Step 7.2: Trim `internal/ai/core/adapter.go`**

Remove the following from the file:
- The `Invoker` interface (lines 9–12 of the original file).
- The `Collector` interface (lines 14–17 of the original file).
- The `Extractor` interface (lines 19–22 of the original file).
- The `Composite` struct and its two methods (added in Task 1).

The file should now contain only the `BaseAdapter` interface, the `NewEngineAdapter` function, the unexported `engineAdapter` struct, and its `Run` / `CollectTokenUsage` methods.

- [ ] **Step 7.3: Trim `internal/ai/core/adapter_test.go`**

Remove the now-orphan legacy tests (`TestBaseAdapter_RunBindsExtract`, `TestBaseAdapter_RunPropagatesError`, `TestBaseAdapter_CollectDelegates`) and the helper fakes that only they used (`fakeInvoker`, `fakeCollector`, `fakeExtractor`). Keep the three `TestNewEngineAdapter_*` tests and the `fakeBackend` helper added in Task 2.

- [ ] **Step 7.4: Build + run all tests**

```bash
go build ./...
go test ./...
```
Expected: PASS across the whole module.

- [ ] **Step 7.5: Commit**

```bash
git add internal/ai/core/adapter.go internal/ai/core/adapter_test.go
git commit -m "refactor(ai/core): drop transitional Composite struct and per-concern interfaces"
```

---

## Final Verification

- [ ] **Step F.1: Full build + test sweep**

```bash
go build ./...
go vet ./...
go test ./...
```
Expected: green across the module.

- [ ] **Step F.2: Inspect the `core` public surface**

```bash
grep -nE "^(type|func)" internal/ai/core/adapter.go
```
Expected output should be exactly:
- `type BaseAdapter interface { ... }`
- `func NewEngineAdapter(base BaseAdapter) ai.EngineAdapter`
- `type engineAdapter struct{ base BaseAdapter }`
- `func (a *engineAdapter) Run(...) (ai.RunResult, error)`
- `func (a *engineAdapter) CollectTokenUsage(...) ([]ai.TokenUsage, error)`

No `Invoker`, `Collector`, `Extractor`, or `Composite` should remain.

- [ ] **Step F.3: Inspect each engine's surface**

```bash
for e in claude codex pi kimi; do
  echo "== $e =="
  grep -nE "^(type|func) " internal/ai/engine/$e/backend.go | head -30
  grep -nE "^(type|func) " internal/ai/engine/$e/adapter.go
done
```
Expected per engine:
- `backend.go`: `type Backend struct`, `func NewBackend(...) *Backend`, plus `Run` / `Collect` / `Extract` methods on `*Backend` and any engine-local helpers.
- `adapter.go`: only `init()` and `NewAdapter(...)`. No `claudeAdapter` or other wrapper structs.
