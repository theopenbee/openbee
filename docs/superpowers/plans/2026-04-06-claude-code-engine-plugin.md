# Claude Code Engine Plugin Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Abstract the Claude Code AI engine into an `EngineAdapter` interface backed by an engine registry, so future engines (Gemini CLI, Amp, etc.) can be plugged in without touching domain code.

**Architecture:** Define `internal/ai.EngineAdapter` covering the full lifecycle (workspace setup + task execution); Claude Code becomes one registered implementation. `worker.Manager` and `bee.Feeder` depend on the interface, not the concrete `claude.Invoker`.

**Tech Stack:** Go — no new dependencies; `internal/ai` (new), `internal/ai/claude` (refactored), `internal/domain/bee`, `internal/domain/worker`, `internal/app`.

---

## File Map

| Status | Path | Role |
|--------|------|------|
| **Create** | `internal/ai/engine.go` | EngineAdapter interface + shared types (Role, WorkspaceOptions, RunOptions, OutputType, Output, Process) |
| **Create** | `internal/ai/registry.go` | Register / New functions |
| **Create** | `internal/ai/registry_test.go` | Registry unit tests |
| **Create** | `internal/ai/claude/adapter.go` | `claudeAdapter` implementing EngineAdapter; init() self-registration |
| **Create** | `internal/ai/claude/adapter_test.go` | Adapter SetupWorkspace tests |
| **Modify** | `internal/ai/claude/invoker.go` | Return `ai.Process` + `<-chan ai.Output`; use `ai.RunOptions` |
| **Modify** | `internal/infra/config/config.go` | Add `Engine string` to `BeeConfig`; add `EngineConfigRaw()` |
| **Modify** | `internal/domain/bee/feeder.go` | Replace `BeeRunner` with `ai.EngineAdapter`; remove direct claude imports |
| **Modify** | `internal/domain/bee/bee_process.go` | Replace `*claude.Invoker` with `ai.EngineAdapter` |
| **Modify** | `internal/domain/bee/feeder_test.go` | Update mock to implement `ai.EngineAdapter` |
| **Modify** | `internal/domain/worker/manager.go` | Replace `*claude.Invoker` with `ai.EngineAdapter` |
| **Modify** | `internal/domain/worker/manager_test.go` | Pass mock engine to `NewManager` |
| **Modify** | `internal/app/app.go` | Add `buildEngine()`; blank-import claude; wire engine into feeder + manager |

---

## Task 1: Define shared AI types in `internal/ai/engine.go`

**Files:**
- Create: `internal/ai/engine.go`

- [ ] **Step 1: Create the file**

```go
package ai

import "context"

// Role identifies the openbee agent role for workspace setup.
type Role string

const (
	RoleBee    Role = "bee"
	RoleWorker Role = "worker"
)

// WorkspaceOptions carries per-agent metadata used during workspace initialisation.
type WorkspaceOptions struct {
	Name        string
	Description string
	Memory      string
}

// RunOptions controls session behaviour for an engine invocation.
type RunOptions struct {
	SessionID string
	Resume    bool
	APIKey    string
}

// OutputType classifies a lifecycle event from a running process.
type OutputType string

const (
	OutputStdout OutputType = "stdout"
	OutputStderr OutputType = "stderr"
	OutputDone   OutputType = "done"
	OutputError  OutputType = "error"
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

// EngineAdapter is the complete plugin contract for an AI engine.
// Implementations must be safe for concurrent use.
type EngineAdapter interface {
	// SetupWorkspace writes engine-specific config files to workDir (system
	// rules, persona, etc.). It must be idempotent.
	SetupWorkspace(workDir string, role Role, opts WorkspaceOptions) error

	// Run executes a task and returns a process handle and an event channel.
	// The channel is closed after the process exits.
	Run(ctx context.Context, workDir, prompt string,
		opts RunOptions, logPath string) (Process, <-chan Output, error)
}
```

- [ ] **Step 2: Verify it compiles**

```bash
go build github.com/theopenbee/openbee/internal/ai
```

Expected: no output (success).

- [ ] **Step 3: Commit**

```bash
git add internal/ai/engine.go
git commit -m "feat(ai): add EngineAdapter interface and shared types"
```

---

## Task 2: Implement the engine registry

**Files:**
- Create: `internal/ai/registry.go`
- Create: `internal/ai/registry_test.go`

- [ ] **Step 1: Write the failing tests**

```go
// internal/ai/registry_test.go
package ai_test

import (
	"context"
	"errors"
	"testing"

	ai "github.com/theopenbee/openbee/internal/ai"
)

// stubAdapter is a no-op EngineAdapter for registry tests.
type stubAdapter struct{}

func (s *stubAdapter) SetupWorkspace(_ string, _ ai.Role, _ ai.WorkspaceOptions) error {
	return nil
}
func (s *stubAdapter) Run(_ context.Context, _, _ string, _ ai.RunOptions, _ string) (ai.Process, <-chan ai.Output, error) {
	return nil, nil, nil
}

func TestRegistry_NewReturnsRegisteredEngine(t *testing.T) {
	r := ai.NewRegistry()
	r.Register("stub", func(_ ai.EngineConfig) (ai.EngineAdapter, error) {
		return &stubAdapter{}, nil
	})
	eng, err := r.New("stub", ai.EngineConfig{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if eng == nil {
		t.Error("expected non-nil adapter")
	}
}

func TestRegistry_NewUnknownEngineReturnsError(t *testing.T) {
	r := ai.NewRegistry()
	_, err := r.New("unknown", ai.EngineConfig{})
	if err == nil {
		t.Fatal("expected error for unknown engine")
	}
	if !errors.Is(err, ai.ErrUnknownEngine) {
		t.Errorf("expected ErrUnknownEngine, got: %v", err)
	}
}

func TestRegistry_NewCallsFactory(t *testing.T) {
	r := ai.NewRegistry()
	called := false
	r.Register("called", func(cfg ai.EngineConfig) (ai.EngineAdapter, error) {
		called = true
		return &stubAdapter{}, nil
	})
	r.New("called", ai.EngineConfig{})
	if !called {
		t.Error("factory was not called")
	}
}
```

- [ ] **Step 2: Run tests — confirm they fail**

```bash
go test github.com/theopenbee/openbee/internal/ai -run TestRegistry -v
```

Expected: compile error (Registry not defined yet).

- [ ] **Step 3: Implement the registry**

```go
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
```

- [ ] **Step 4: Run tests — confirm they pass**

```bash
go test github.com/theopenbee/openbee/internal/ai -run TestRegistry -v
```

Expected: all three tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/ai/registry.go internal/ai/registry_test.go
git commit -m "feat(ai): add engine registry with Register/New and ErrUnknownEngine"
```

---

## Task 3: Update `claude.Invoker` to use `ai` types

**Files:**
- Modify: `internal/ai/claude/invoker.go`

- [ ] **Step 1: Update `invoker.go`**

Replace the local type definitions and update `Run` to return `ai.Process` and `<-chan ai.Output`. The `Process` struct already has `PID()` and `Stop()` so it satisfies the interface with no struct changes.

Key changes:
1. Remove `OutputType`, `Output`, `RunOptions` type declarations.
2. Add import for `ai "github.com/theopenbee/openbee/internal/ai"`.
3. Change `Run` signature to return `(ai.Process, <-chan ai.Output, error)`.
4. Change channel element type from `Output` to `ai.Output`.
5. Change `RunOptions` parameter to `ai.RunOptions`.

```go
// internal/ai/claude/invoker.go
package claude

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	ai "github.com/theopenbee/openbee/internal/ai"
)

// Invoker spawns Claude CLI processes. It is stateless and safe for concurrent use.
type Invoker struct {
	binary  string
	baseEnv []string
}

// NewInvoker creates an Invoker. openbeeURL is injected as OPENBEE_URL into subprocesses.
func NewInvoker(binary, openbeeURL string) *Invoker {
	sysEnv := os.Environ()
	env := make([]string, 0, len(sysEnv)+3)
	if exePath, err := os.Executable(); err == nil {
		patchedPath := "PATH=" + filepath.Dir(exePath) + string(os.PathListSeparator) + os.Getenv("PATH")
		for _, e := range sysEnv {
			if !strings.HasPrefix(e, "PATH=") {
				env = append(env, e)
			}
		}
		env = append(env, patchedPath)
	} else {
		env = append(env, sysEnv...)
	}
	env = append(env, "OPENBEE_URL="+openbeeURL)
	return &Invoker{binary: binary, baseEnv: env}
}

// Process represents a running Claude CLI invocation.
type Process struct {
	cmd *exec.Cmd
	mu  sync.Mutex
}

// PID returns the process ID, or 0 if the process has not started.
func (p *Process) PID() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cmd != nil && p.cmd.Process != nil {
		return p.cmd.Process.Pid
	}
	return 0
}

// Stop kills the process.
func (p *Process) Stop() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cmd != nil && p.cmd.Process != nil {
		return p.cmd.Process.Kill()
	}
	return nil
}

// Run starts a Claude CLI process, redirecting output to logPath.
func (inv *Invoker) Run(ctx context.Context, workDir, prompt string, opts ai.RunOptions, logPath string) (ai.Process, <-chan ai.Output, error) {
	args := []string{
		"--dangerously-skip-permissions",
		"--verbose",
		"--output-format", "stream-json",
	}
	if opts.SessionID != "" {
		if opts.Resume {
			args = append(args, "--resume", opts.SessionID)
		} else {
			args = append(args, "--session-id", opts.SessionID)
		}
	}
	args = append(args, "--print")

	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return nil, nil, fmt.Errorf("open log file: %w", err)
	}

	cmd := exec.CommandContext(ctx, inv.binary, args...)
	cmd.Dir = workDir
	cmd.Stdin = strings.NewReader(prompt)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Env = append(inv.baseEnv, "OPENBEE_API_KEY="+opts.APIKey)

	if err := cmd.Start(); err != nil {
		logFile.Close()
		return nil, nil, fmt.Errorf("start claude: %w", err)
	}

	proc := &Process{cmd: cmd}
	ch := make(chan ai.Output, 1)

	go func() {
		defer close(ch)
		defer logFile.Close()
		if err := cmd.Wait(); err != nil {
			ch <- ai.Output{Type: ai.OutputError, Content: err.Error()}
		} else {
			ch <- ai.Output{Type: ai.OutputDone, Content: ""}
		}
	}()

	return proc, ch, nil
}
```

- [ ] **Step 2: Update `invoker_test.go`**

The test file is `package claude` (internal) and uses bare `RunOptions` and `OutputDone`. Add the `ai` import and replace all occurrences:

```go
// Add to imports:
ai "github.com/theopenbee/openbee/internal/ai"

// Replace all RunOptions{...} with ai.RunOptions{...}
// Replace out.Type == OutputDone with out.Type == ai.OutputDone

// Specific changes:
// Line: proc, ch, err := inv.Run(ctx, t.TempDir(), "hello", RunOptions{}, logPath)
//   → inv.Run(ctx, t.TempDir(), "hello", ai.RunOptions{}, logPath)

// Line: if out.Type == OutputDone {
//   → if out.Type == ai.OutputDone {

// Line: _, ch, _ := inv.Run(ctx, t.TempDir(), "test", RunOptions{SessionID: "s1"}, logPath1)
//   → inv.Run(ctx, t.TempDir(), "test", ai.RunOptions{SessionID: "s1"}, logPath1)

// Line: _, ch2, _ := inv.Run(ctx, t.TempDir(), "test", RunOptions{SessionID: "s2", Resume: true}, logPath2)
//   → inv.Run(ctx, t.TempDir(), "test", ai.RunOptions{SessionID: "s2", Resume: true}, logPath2)

// Line: proc, ch, err := inv.Run(ctx, t.TempDir(), "60", RunOptions{}, logPath)
//   → inv.Run(ctx, t.TempDir(), "60", ai.RunOptions{}, logPath)

// Lines with RunOptions{SessionID: "s1"} and RunOptions{SessionID: "s2"} in TestInvoker_ConcurrentRuns:
//   → ai.RunOptions{SessionID: "s1"} and ai.RunOptions{SessionID: "s2"}
```

- [ ] **Step 3: Verify build**

```bash
go build github.com/theopenbee/openbee/internal/ai/claude
```

Expected: no errors (other packages that import claude will fail until updated — that's expected).

- [ ] **Step 4: Run claude package tests**

```bash
go test github.com/theopenbee/openbee/internal/ai/claude/... -v
```

Expected: all tests pass.

- [ ] **Step 5: Commit**

```bash
git add internal/ai/claude/invoker.go internal/ai/claude/invoker_test.go
git commit -m "refactor(claude): use ai.RunOptions, ai.Output, ai.Process in Invoker"
```

---

## Task 4: Create Claude engine adapter

**Files:**
- Create: `internal/ai/claude/adapter.go`
- Create: `internal/ai/claude/adapter_test.go`

- [ ] **Step 1: Write failing adapter tests**

```go
// internal/ai/claude/adapter_test.go
package claude_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	ai "github.com/theopenbee/openbee/internal/ai"
	"github.com/theopenbee/openbee/internal/ai/claude"
)

func newTestAdapter(t *testing.T) ai.EngineAdapter {
	t.Helper()
	return claude.NewAdapter("echo", "http://localhost:9999")
}

func TestClaudeAdapter_SetupWorkspace_Worker(t *testing.T) {
	dir := t.TempDir()
	// pre-create CLAUDE.md so EnsureSystemRules can find it
	os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte("@.openbee.md\n"), 0644)

	adapter := newTestAdapter(t)
	err := adapter.SetupWorkspace(dir, ai.RoleWorker, ai.WorkspaceOptions{
		Name:        "test-worker",
		Description: "runs tests",
		Memory:      "",
	})
	if err != nil {
		t.Fatalf("SetupWorkspace: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, claude.SystemRulesFile))
	if err != nil {
		t.Fatalf("read system rules: %v", err)
	}
	if !strings.Contains(string(data), "test-worker") {
		t.Error("worker name not found in system rules")
	}
}

func TestClaudeAdapter_SetupWorkspace_Bee_CreatesCLAUDEMD(t *testing.T) {
	dir := t.TempDir()

	adapter := newTestAdapter(t)
	err := adapter.SetupWorkspace(dir, ai.RoleBee, ai.WorkspaceOptions{})
	if err != nil {
		t.Fatalf("SetupWorkspace bee: %v", err)
	}

	claudeMD := filepath.Join(dir, "CLAUDE.md")
	if _, err := os.Stat(claudeMD); os.IsNotExist(err) {
		t.Error("CLAUDE.md was not created for bee workspace")
	}

	data, err := os.ReadFile(filepath.Join(dir, claude.SystemRulesFile))
	if err != nil {
		t.Fatalf("read system rules: %v", err)
	}
	if !strings.Contains(string(data), "coordinator") {
		t.Error("bee role rules missing 'coordinator'")
	}
}

func TestClaudeAdapter_SetupWorkspace_Bee_DoesNotOverwriteCLAUDEMD(t *testing.T) {
	dir := t.TempDir()
	existing := "# my custom persona\n"
	os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte(existing), 0644)

	adapter := newTestAdapter(t)
	adapter.SetupWorkspace(dir, ai.RoleBee, ai.WorkspaceOptions{})

	data, _ := os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
	if string(data) != existing {
		t.Error("SetupWorkspace overwrote existing CLAUDE.md")
	}
}
```

- [ ] **Step 2: Run tests — confirm compile error**

```bash
go test github.com/theopenbee/openbee/internal/ai/claude -run TestClaudeAdapter -v
```

Expected: compile error (`NewAdapter` not defined).

- [ ] **Step 3: Create adapter.go**

`DefaultPersona` and the `WriteCLAUDEMD` logic are absorbed here; the `bee` package will no longer own them.

```go
// internal/ai/claude/adapter.go
package claude

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	ai "github.com/theopenbee/openbee/internal/ai"
)

// DefaultPersona is the default CLAUDE.md content for the bee workspace.
const DefaultPersona = `You are B, an AI assistant.`

func init() {
	ai.Register("claude", func(cfg ai.EngineConfig) (ai.EngineAdapter, error) {
		path, _ := cfg.Raw["path"].(string)
		if path == "" {
			path = "claude"
		}
		return NewAdapter(path, cfg.OpenbeeURL), nil
	})
}

type claudeAdapter struct {
	invoker *Invoker
}

// NewAdapter creates a claudeAdapter. Exported for testing.
func NewAdapter(binaryPath, openbeeURL string) ai.EngineAdapter {
	return &claudeAdapter{invoker: NewInvoker(binaryPath, openbeeURL)}
}

// SetupWorkspace implements ai.EngineAdapter.
func (a *claudeAdapter) SetupWorkspace(workDir string, role ai.Role, opts ai.WorkspaceOptions) error {
	switch role {
	case ai.RoleWorker:
		claudeMD := filepath.Join(workDir, "CLAUDE.md")
		if _, err := os.Stat(claudeMD); os.IsNotExist(err) {
			if err := os.MkdirAll(workDir, 0o755); err != nil {
				return fmt.Errorf("mkdir workdir: %w", err)
			}
			if err := os.WriteFile(claudeMD, []byte(ImportLine+"\n"), 0o644); err != nil {
				return fmt.Errorf("create CLAUDE.md: %w", err)
			}
		}
		return EnsureSystemRules(workDir, RoleWorker,
			WithName(opts.Name),
			WithDescription(opts.Description),
			WithMemory(opts.Memory),
		)
	case ai.RoleBee:
		if err := writeCLAUDEMD(workDir, DefaultPersona); err != nil {
			return err
		}
		return EnsureSystemRules(workDir, RoleBee)
	default:
		return fmt.Errorf("unknown role: %q", role)
	}
}

// Run implements ai.EngineAdapter.
func (a *claudeAdapter) Run(ctx context.Context, workDir, prompt string,
	opts ai.RunOptions, logPath string) (ai.Process, <-chan ai.Output, error) {
	return a.invoker.Run(ctx, workDir, prompt, opts, logPath)
}

// writeCLAUDEMD creates workDir/CLAUDE.md with persona only if it does not exist.
func writeCLAUDEMD(workDir, persona string) error {
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return fmt.Errorf("mkdir bee workdir: %w", err)
	}
	path := filepath.Join(workDir, "CLAUDE.md")
	if _, err := os.Stat(path); err == nil {
		return nil // already exists
	}
	return os.WriteFile(path, []byte(persona), 0o644)
}
```

- [ ] **Step 4: Run adapter tests — confirm they pass**

```bash
go test github.com/theopenbee/openbee/internal/ai/claude -run TestClaudeAdapter -v
```

Expected: all three PASS.

- [ ] **Step 5: Run full claude package tests**

```bash
go test github.com/theopenbee/openbee/internal/ai/claude/... -v
```

Expected: all tests pass.

- [ ] **Step 6: Commit**

```bash
git add internal/ai/claude/adapter.go internal/ai/claude/adapter_test.go
git commit -m "feat(claude): add claudeAdapter implementing ai.EngineAdapter with self-registration"
```

---

## Task 5: Add `Engine` field to `BeeConfig`

**Files:**
- Modify: `internal/infra/config/config.go`

- [ ] **Step 1: Add `Engine` field and `EngineConfigRaw` method**

In `config.go`, locate `BeeConfig` and add:

```go
type BeeConfig struct {
    Engine  string        `yaml:"engine"`   // AI engine name; defaults to "claude"
    Claude  ClaudeConfig  `yaml:"claude"`
    // ... existing fields unchanged
}

// EngineConfigRaw returns the raw config map for the selected engine.
// Used by the engine registry factory.
func (b BeeConfig) EngineConfigRaw() map[string]any {
    name := b.Engine
    if name == "" {
        name = "claude"
    }
    switch name {
    case "claude":
        return map[string]any{
            "path":    b.Claude.Path,
            "timeout": b.Claude.Timeout,
        }
    default:
        return nil
    }
}
```

- [ ] **Step 2: Build config package**

```bash
go build github.com/theopenbee/openbee/internal/infra/config
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add internal/infra/config/config.go
git commit -m "feat(config): add Engine field and EngineConfigRaw to BeeConfig"
```

---

## Task 6: Update `bee.Feeder` and `bee.BeeProcess`

**Files:**
- Modify: `internal/domain/bee/feeder.go`
- Modify: `internal/domain/bee/bee_process.go`
- Modify: `internal/domain/bee/feeder_test.go`

- [ ] **Step 1: Rewrite `feeder.go` — replace `BeeRunner` with `ai.EngineAdapter`**

Change the `BeeRunner` interface declaration and all its usages:

```go
// Replace the BeeRunner interface:
// OLD:
// type BeeRunner interface {
//     Run(ctx context.Context, workDir, prompt string, opts claude.RunOptions, logPath string) (*claude.Process, <-chan claude.Output, error)
// }

// NEW — delete BeeRunner entirely; use ai.EngineAdapter directly.
```

Update `Feeder.runner` field type:
```go
type Feeder struct {
    // ...
    runner  ai.EngineAdapter   // was: BeeRunner
    // ...
}
```

Update `NewFeeder` signature:
```go
func NewFeeder(ms *store.MessageStore, ts *store.TaskStore, ss *store.SessionStore,
    es *store.ExecutionStore, runner ai.EngineAdapter, workDir string,
    cfg config.BeeConfig, opts ...Option) *Feeder {
```

Replace the `WriteCLAUDEMD` + `EnsureSystemRules` block (feeder.go ~line 122-128):
```go
// OLD:
// if err := WriteCLAUDEMD(f.workDir, DefaultPersona); err != nil { ... }
// if err := claude.EnsureSystemRules(f.workDir, claude.RoleBee); err != nil { ... }

// NEW:
if err := f.runner.SetupWorkspace(f.workDir, ai.RoleBee, ai.WorkspaceOptions{}); err != nil {
    log.Error("setup bee workspace", zap.Error(err))
    f.rollback(ctx, msgs, err.Error())
    return
}
```

Update the `f.runner.Run(...)` call (~line 186):
```go
// OLD:
// proc, outputCh, err := f.runner.Run(beeCtx, f.workDir, prompt, claude.RunOptions{SessionID: sessionID, Resume: resume}, logPath)

// NEW:
proc, outputCh, err := f.runner.Run(beeCtx, f.workDir, prompt, ai.RunOptions{SessionID: sessionID, Resume: resume}, logPath)
```

Update `waitBeeOutput` signature and switch cases:
```go
func (f *Feeder) waitBeeOutput(ch <-chan ai.Output) error {
    for out := range ch {
        switch out.Type {
        case ai.OutputDone:
            return nil
        case ai.OutputError:
            return fmt.Errorf("%s", out.Content)
        }
    }
    return nil
}
```

Remove `"github.com/theopenbee/openbee/internal/ai/claude"` import; add `ai "github.com/theopenbee/openbee/internal/ai"`.

- [ ] **Step 2: Update `bee_process.go`**

Replace `*claude.Invoker` with `ai.EngineAdapter`. Remove `WriteCLAUDEMD` and `DefaultPersona` (moved to adapter). Keep `tokenSecret` / `tokenTTL` fields for token generation.

```go
// internal/domain/bee/bee_process.go
package bee

import (
	"context"
	"fmt"
	"time"

	ai "github.com/theopenbee/openbee/internal/ai"
	"github.com/theopenbee/openbee/internal/infra/auth"
	"github.com/theopenbee/openbee/internal/infra/config"
)

// BeeProcess wraps an EngineAdapter and injects a short-lived auth token into each Run call.
type BeeProcess struct {
	engine      ai.EngineAdapter
	tokenSecret string
	tokenTTL    time.Duration
}

// NewBeeProcess creates a BeeProcess.
func NewBeeProcess(cfg config.BeeConfig, engine ai.EngineAdapter) *BeeProcess {
	return &BeeProcess{
		engine:      engine,
		tokenSecret: cfg.MCP.TokenSecret,
		tokenTTL:    cfg.MCP.TokenTTL,
	}
}

// SetupWorkspace delegates to the underlying engine adapter.
func (p *BeeProcess) SetupWorkspace(workDir string, role ai.Role, opts ai.WorkspaceOptions) error {
	return p.engine.SetupWorkspace(workDir, role, opts)
}

// Run injects a bee auth token then delegates to the engine.
func (p *BeeProcess) Run(ctx context.Context, workDir, prompt string, opts ai.RunOptions, logPath string) (ai.Process, <-chan ai.Output, error) {
	token, err := auth.GenerateBeeToken(p.tokenSecret, p.tokenTTL)
	if err != nil {
		return nil, nil, fmt.Errorf("generate bee token: %w", err)
	}
	opts.APIKey = token
	return p.engine.Run(ctx, workDir, prompt, opts, logPath)
}
```

- [ ] **Step 3: Update `feeder_test.go` mock**

Replace `mockBeeRunner` and `callbackBeeRunner` with implementations of `ai.EngineAdapter`. A minimal `mockProcess` is needed since `ai.Process` is now an interface.

```go
// In feeder_test.go, replace the mock types:

import (
    ai "github.com/theopenbee/openbee/internal/ai"
    // remove: "github.com/theopenbee/openbee/internal/ai/claude"
)

// mockProcess satisfies ai.Process.
type mockProcess struct{}
func (m *mockProcess) PID() int      { return 0 }
func (m *mockProcess) Stop() error   { return nil }

// mockBeeRunner records Run calls and implements ai.EngineAdapter.
type mockBeeRunner struct {
    mu          sync.Mutex
    calls       []beeCall
    err         error
    outputLines []ai.Output
}

func (m *mockBeeRunner) SetupWorkspace(_ string, _ ai.Role, _ ai.WorkspaceOptions) error {
    return nil
}

func (m *mockBeeRunner) Run(_ context.Context, _, prompt string, opts ai.RunOptions, logPath string) (ai.Process, <-chan ai.Output, error) {
    m.mu.Lock()
    m.calls = append(m.calls, beeCall{prompt: prompt, opts: opts})
    m.mu.Unlock()

    if m.err != nil {
        return nil, nil, m.err
    }

    var lines []ai.Output
    if len(m.outputLines) > 0 {
        lines = m.outputLines
    } else if m.err != nil {
        lines = []ai.Output{{Type: ai.OutputError, Content: m.err.Error()}}
    } else {
        lines = []ai.Output{{Type: ai.OutputDone}}
    }

    ch := make(chan ai.Output, len(lines))
    for _, l := range lines {
        ch <- l
    }
    close(ch)
    return &mockProcess{}, ch, nil
}

// callbackBeeRunner implements ai.EngineAdapter.
type callbackBeeRunner struct {
    fn   func()
    done chan struct{}
}

func (r *callbackBeeRunner) SetupWorkspace(_ string, _ ai.Role, _ ai.WorkspaceOptions) error {
    return nil
}

func (r *callbackBeeRunner) Run(_ context.Context, _, _ string, _ ai.RunOptions, _ string) (ai.Process, <-chan ai.Output, error) {
    ch := make(chan ai.Output, 1)
    go func() {
        r.fn()
        close(r.done)
        ch <- ai.Output{Type: ai.OutputDone}
        close(ch)  // NOTE: close after send
    }()
    return &mockProcess{}, ch, nil
}
```

Also update `newFeeder` helper:
```go
func newFeeder(ms *store.MessageStore, ts *store.TaskStore, ss *store.SessionStore,
    es *store.ExecutionStore, runner ai.EngineAdapter) *bee.Feeder {
    // ...
}
```

And the `beeCall` struct's `opts` field type:
```go
type beeCall struct {
    prompt string
    opts   ai.RunOptions
}
```

- [ ] **Step 4: Build bee package**

```bash
go build github.com/theopenbee/openbee/internal/domain/bee/...
```

Expected: no errors.

- [ ] **Step 5: Run bee tests**

```bash
go test github.com/theopenbee/openbee/internal/domain/bee/... -v
```

Expected: all tests pass. (Note: `TestWriteCLAUDEMD_*` tests are removed in this step — their coverage is now in `adapter_test.go`.)

- [ ] **Step 6: Commit**

```bash
git add internal/domain/bee/feeder.go internal/domain/bee/bee_process.go internal/domain/bee/feeder_test.go
git commit -m "refactor(bee): replace claude.Invoker with ai.EngineAdapter; remove WriteCLAUDEMD from bee package"
```

---

## Task 7: Update `worker.Manager`

**Files:**
- Modify: `internal/domain/worker/manager.go`
- Modify: `internal/domain/worker/manager_test.go`

- [ ] **Step 1: Update `manager.go`**

Replace all `claude.*` references:

```go
// Replace import:
// "github.com/theopenbee/openbee/internal/ai/claude"
// with:
// ai "github.com/theopenbee/openbee/internal/ai"

// Replace struct field:
// invoker        *claude.Invoker        →  engine  ai.EngineAdapter
// activeProcesses map[string]*claude.Process  →  activeProcesses map[string]ai.Process

// Replace NewManager body:
func NewManager(
    workerBaseDir string,
    bc config.BeeConfig,
    ws *store.WorkerStore,
    es *store.ExecutionStore,
    engine ai.EngineAdapter,
) *Manager {
    return &Manager{
        workerBaseDir:   workerBaseDir,
        beeCfg:          bc,
        workerStore:     ws,
        executionStore:  es,
        engine:          engine,
        activeProcesses: make(map[string]ai.Process),
    }
}
```

In `CreateWorker`, replace the CLAUDE.md + EnsureSystemRules block:
```go
// OLD:
// claudeMD := filepath.Join(workDir, "CLAUDE.md")
// if _, err := os.Stat(claudeMD); os.IsNotExist(err) {
//     initialContent := claude.ImportLine + "\n"
//     os.WriteFile(claudeMD, []byte(initialContent), 0644)
// }
// claude.EnsureSystemRules(workDir, claude.RoleWorker, ...)

// NEW:
if err := m.engine.SetupWorkspace(workDir, ai.RoleWorker, ai.WorkspaceOptions{
    Name:        name,
    Description: description,
    Memory:      memory,
}); err != nil {
    log.Error("setup worker workspace", zap.String("op", "create"), zap.Error(err))
}
```

In `ExecuteWorker`, replace the second `EnsureSystemRules` call:
```go
// OLD: claude.EnsureSystemRules(worker.WorkDir, claude.RoleWorker, ...)
// NEW:
if err := m.engine.SetupWorkspace(worker.WorkDir, ai.RoleWorker, ai.WorkspaceOptions{
    Name:        worker.Name,
    Description: worker.Description,
    Memory:      worker.Memory,
}); err != nil {
    log.Error("setup worker workspace", zap.String("op", "execute"), zap.Error(err))
}
```

In `launchRuntime`, replace `m.invoker.Run(...)`:
```go
// OLD: proc, outputCh, err := m.invoker.Run(execCtx, worker.WorkDir, prompt, claude.RunOptions{...}, logPath)
// NEW:
proc, outputCh, err := m.engine.Run(execCtx, worker.WorkDir, prompt, ai.RunOptions{
    SessionID: exec.SessionID,
    Resume:    resume,
    APIKey:    token,
}, logPath)
```

In `monitorExecution`, update the channel type and switch cases:
```go
func (m *Manager) monitorExecution(exec model.WorkerExecution, worker model.Worker,
    outputCh <-chan ai.Output, cancel context.CancelFunc, logPath string) {
    defer cancel()
    for out := range outputCh {
        switch out.Type {
        case ai.OutputDone:
            // ...
        case ai.OutputError:
            // ...
        }
    }
    // ...
}
```

- [ ] **Step 2: Update `manager_test.go`**

`NewManager` now requires an engine parameter. Add a `mockEngine` stub:

```go
// In manager_test.go, add before tests:
type mockEngine struct{}

func (e *mockEngine) SetupWorkspace(_ string, _ ai.Role, _ ai.WorkspaceOptions) error {
    return nil
}

func (e *mockEngine) Run(_ context.Context, _, _ string, _ ai.RunOptions, _ string) (ai.Process, <-chan ai.Output, error) {
    ch := make(chan ai.Output, 1)
    ch <- ai.Output{Type: ai.OutputDone}
    close(ch)
    return &mockProcess{}, ch, nil
}

type mockProcess struct{}
func (p *mockProcess) PID() int    { return 0 }
func (p *mockProcess) Stop() error { return nil }
```

Update the test to pass `&mockEngine{}`:
```go
mgr := NewManager(dir, cfg, ws, es, &mockEngine{})
```

- [ ] **Step 3: Build and test**

```bash
go build github.com/theopenbee/openbee/internal/domain/worker/...
go test github.com/theopenbee/openbee/internal/domain/worker/... -v
```

Expected: all tests pass.

- [ ] **Step 4: Commit**

```bash
git add internal/domain/worker/manager.go internal/domain/worker/manager_test.go
git commit -m "refactor(worker): replace claude.Invoker with ai.EngineAdapter in Manager"
```

---

## Task 8: Wire engine in `app.go`

**Files:**
- Modify: `internal/app/app.go`

- [ ] **Step 1: Add blank import and `buildEngine` function**

Add to imports:
```go
import (
    // existing imports...
    ai "github.com/theopenbee/openbee/internal/ai"
    _ "github.com/theopenbee/openbee/internal/ai/claude" // triggers init() registration
)
```

Add helper function (place near `buildWorkerManager`):
```go
func buildEngine(cfg config.BeeConfig) (ai.EngineAdapter, error) {
    name := cfg.Engine
    if name == "" {
        name = "claude"
    }
    return ai.New(name, ai.EngineConfig{
        OpenbeeURL: cfg.MCPBaseURL,
        Raw:        cfg.EngineConfigRaw(),
    })
}
```

- [ ] **Step 2: Update `buildWorkerManager` signature and body**

```go
func buildWorkerManager(bc config.BeeConfig, s appStores, engine ai.EngineAdapter) *worker.Manager {
    return worker.NewManager(config.DefaultWorkerBaseDir(), bc, s.workerStore, s.execStore, engine)
}
```

- [ ] **Step 3: Update `buildBee` signature and body**

```go
func buildBee(cfg config.BeeConfig, s appStores, dispatchCh chan task.DispatchTask,
    failureNotifier bee.FailureNotifier, engine ai.EngineAdapter) (*bee.Feeder, *task.Scheduler) {
    beeProcess := bee.NewBeeProcess(cfg, engine)
    feeder := bee.NewFeeder(s.msgStore, s.taskStore, s.sessionStore, s.execStore,
        beeProcess, config.DefaultBeeWorkDir(), cfg,
        bee.WithFailureNotifier(failureNotifier))
    sched := task.NewScheduler(s.taskStore, dispatchCh, bee.PollInterval)
    return feeder, sched
}
```

- [ ] **Step 4: Update the call sites in the app wiring function**

Find where `buildWorkerManager` and `buildBee` are called and pass the engine:

```go
engine, err := buildEngine(cfg.Bee)
if err != nil {
    return nil, fmt.Errorf("init engine: %w", err)
}
mgr := buildWorkerManager(cfg.Bee, s, engine)
feeder, sched := buildBee(cfg.Bee, s, dispatchCh, failureNotifier, engine)
```

- [ ] **Step 5: Full build**

```bash
go build github.com/theopenbee/openbee/...
```

Expected: no errors.

- [ ] **Step 6: Commit**

```bash
git add internal/app/app.go
git commit -m "feat(app): wire engine registry into bee and worker via buildEngine()"
```

---

## Task 9: Final verification

- [ ] **Step 1: Run all tests**

```bash
go test ./... -count=1
```

Expected: all packages pass, zero failures.

- [ ] **Step 2: Verify no remaining direct claude imports in domain layer**

```bash
grep -r '"github.com/theopenbee/openbee/internal/ai/claude"' \
    internal/domain/ internal/app/
```

Expected: no matches (domain and app layers should only import `internal/ai`, not `internal/ai/claude`). Only `internal/app/app.go`'s blank import `_ "…/ai/claude"` is allowed.

- [ ] **Step 3: Final commit**

```bash
git add -A
git commit -m "chore: engine plugin refactor complete — all tests green"
```
