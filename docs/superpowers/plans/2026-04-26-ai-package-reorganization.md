# ai Package Reorganization Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rename and merge files in `internal/ai/` so that every filename immediately answers "what's in here?" with zero logic changes.

**Architecture:** Five atomic changes — two renames (`engine.go`→`contracts.go`, `rules.go`→`prompt.go`), two merges-then-deletes (`types.go`→`contracts.go`, `scan.go`→`process.go`), and one inline (`claudemd.go`→`claude/adapter.go`). Go's file-agnostic package system means no import paths change anywhere.

**Tech Stack:** Go standard library; `go build` and `go test` as verification tools.

---

## File Map

| Action | Source | Destination |
|--------|--------|-------------|
| Create (replaces two files) | `engine.go` + `types.go` | `internal/ai/contracts.go` |
| Delete | `internal/ai/engine.go` | — |
| Delete | `internal/ai/types.go` | — |
| Modify (absorbs scan.go) | `internal/ai/scan.go` | `internal/ai/process.go` |
| Delete | `internal/ai/scan.go` | — |
| Rename | `internal/ai/rules.go` | `internal/ai/prompt.go` |
| Rename | `internal/ai/rules_test.go` | `internal/ai/prompt_test.go` |
| Modify (absorbs claudemd.go) | `internal/ai/claude/claudemd.go` | `internal/ai/claude/adapter.go` |
| Delete | `internal/ai/claude/claudemd.go` | — |

---

## Task 1: Create `contracts.go`, delete `engine.go` and `types.go`

**Files:**
- Create: `internal/ai/contracts.go`
- Delete: `internal/ai/engine.go`
- Delete: `internal/ai/types.go`

- [ ] **Step 1: Confirm baseline tests pass**

```bash
go test ./internal/ai/...
```
Expected: all tests pass (PASS lines, no FAIL).

- [ ] **Step 2: Create `internal/ai/contracts.go`**

Write this file verbatim (it is the union of `engine.go` + `types.go`, import block merged):

```go
package ai

import (
	"context"
	"errors"
)

const (
	// SystemRulesFile is the legacy rules file that Claude's Prepare hook cleans up.
	SystemRulesFile = ".openbee.md"
	// ImportLine is the legacy reference line that Claude's Prepare hook removes from CLAUDE.md.
	ImportLine = "@" + SystemRulesFile
)

// Role identifies the openbee agent role.
type Role string

const (
	RoleBee    Role = "bee"
	RoleWorker Role = "worker"
)

// PrepareOptions carries parameters for the engine-specific Prepare hook.
// Add future fields here without changing the Prepare method signature.
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

// Engine name constants used for registration and configuration.
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

// RunResult is the handle returned from EngineAdapter.Run. ExtractResult is
// bound to the engine that handled this Run, so it remains correct even if
// the active engine later changes.
type RunResult struct {
	Process       Process
	Output        <-chan Output
	ExtractResult func(logPath string) string
}

// NewRunResult wraps the (process, output, error) tuple returned by an engine
// invoker into a RunResult, attaching the engine's result extractor on success.
func NewRunResult(proc Process, out <-chan Output, err error, extract func(logPath string) string) (RunResult, error) {
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

// DrainUsageMap converts a model→*TokenUsage aggregation map into a flat slice.
func DrainUsageMap(agg map[string]*TokenUsage) []TokenUsage {
	out := make([]TokenUsage, 0, len(agg))
	for _, u := range agg {
		out = append(out, *u)
	}
	return out
}
```

- [ ] **Step 3: Delete `engine.go` and `types.go`**

```bash
git rm internal/ai/engine.go internal/ai/types.go
```

- [ ] **Step 4: Verify build and tests pass**

```bash
go build ./internal/ai/...
go test ./internal/ai/...
```
Expected: `go build` exits 0; all tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/ai/contracts.go
git commit -m "refactor(ai): merge engine.go+types.go into contracts.go"
```

---

## Task 2: Merge `scan.go` into `process.go`, delete `scan.go`

**Files:**
- Modify: `internal/ai/process.go`
- Delete: `internal/ai/scan.go`

- [ ] **Step 1: Replace `internal/ai/process.go` with the merged version**

Write this file verbatim (original content + `ScanJSONLines` appended, imports expanded with `bufio` and `io`):

```go
package ai

import (
	"bufio"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// CmdProcess implements Process for an os/exec.Cmd.
type CmdProcess struct {
	cmd *exec.Cmd
	mu  sync.Mutex
}

// NewCmdProcess wraps an exec.Cmd as a Process.
func NewCmdProcess(cmd *exec.Cmd) *CmdProcess {
	return &CmdProcess{cmd: cmd}
}

func (p *CmdProcess) PID() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cmd != nil && p.cmd.Process != nil {
		return p.cmd.Process.Pid
	}
	return 0
}

func (p *CmdProcess) Stop() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cmd != nil && p.cmd.Process != nil {
		return p.cmd.Process.Kill()
	}
	return nil
}

// BuildRunEnv assembles the final env slice for a subprocess run.
// Entries are ordered baseEnv → extraEnv → apiKey; for duplicate keys the
// last value wins (standard subprocess env resolution on Linux/macOS), so
// extraEnv overrides baseEnv and apiKey overrides both.
func BuildRunEnv(baseEnv, extraEnv []string, apiKey string) []string {
	env := make([]string, 0, len(baseEnv)+len(extraEnv)+1)
	env = append(env, baseEnv...)
	env = append(env, extraEnv...)
	env = append(env, "OPENBEE_API_KEY="+apiKey)
	return env
}

// AppendExtraEnv appends non-empty entries from extraEnv to base and returns
// the result re-clipped to its length, preventing concurrent Run() appends from
// sharing the backing array with other goroutines.
func AppendExtraEnv(base []string, extraEnv map[string]string) []string {
	for k, v := range extraEnv {
		if v != "" {
			base = append(base, k+"="+v)
		}
	}
	return base[:len(base):len(base)]
}

// BuildBaseEnv constructs the base environment for engine subprocesses.
// It prepends the current executable's directory to PATH and appends OPENBEE_URL.
func BuildBaseEnv(openbeeURL string) []string {
	sysEnv := os.Environ()
	env := make([]string, 0, len(sysEnv)+2)
	if exePath, err := os.Executable(); err == nil {
		oldPath := os.Getenv("PATH")
		for _, e := range sysEnv {
			if !strings.HasPrefix(e, "PATH=") {
				env = append(env, e)
			}
		}
		env = append(env, "PATH="+filepath.Dir(exePath)+string(os.PathListSeparator)+oldPath)
	} else {
		env = append(env, sysEnv...)
	}
	env = append(env, "OPENBEE_URL="+openbeeURL)
	// Clip to length so concurrent append calls in Run() cannot share the backing array.
	return env[:len(env):len(env)]
}

// ScanJSONLines reads r line by line and calls fn for each line that starts
// with '{'. fn returns true to keep scanning or false to stop early.
func ScanJSONLines(r io.Reader, fn func(line string) bool) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(nil, 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "{") && !fn(line) {
			return
		}
	}
}
```

- [ ] **Step 2: Delete `scan.go`**

```bash
git rm internal/ai/scan.go
```

- [ ] **Step 3: Verify build and tests pass**

```bash
go build ./internal/ai/...
go test ./internal/ai/...
```
Expected: `go build` exits 0; all tests PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/ai/process.go
git commit -m "refactor(ai): merge scan.go into process.go"
```

---

## Task 3: Rename `rules.go` → `prompt.go` (and test file)

**Files:**
- Rename: `internal/ai/rules.go` → `internal/ai/prompt.go`
- Rename: `internal/ai/rules_test.go` → `internal/ai/prompt_test.go`

- [ ] **Step 1: Rename both files with git mv**

```bash
git mv internal/ai/rules.go internal/ai/prompt.go
git mv internal/ai/rules_test.go internal/ai/prompt_test.go
```

- [ ] **Step 2: Verify build and tests pass**

```bash
go build ./internal/ai/...
go test ./internal/ai/...
```
Expected: `go build` exits 0; all tests PASS (including the renamed test file).

- [ ] **Step 3: Commit**

```bash
git commit -m "refactor(ai): rename rules.go → prompt.go"
```

---

## Task 4: Inline `claudemd.go` into `claude/adapter.go`, delete `claudemd.go`

**Files:**
- Modify: `internal/ai/claude/adapter.go`
- Delete: `internal/ai/claude/claudemd.go`

- [ ] **Step 1: Replace `internal/ai/claude/adapter.go` with the inlined version**

Write this file verbatim (`removeImportLine` added at the bottom, `"bytes"` added to imports):

```go
package claude

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	ai "github.com/theopenbee/openbee/internal/ai"
)

func init() {
	ai.Register(ai.EngineClaude, func(cfg ai.EngineConfig) (ai.EngineAdapter, error) {
		return NewAdapter(cfg.PathOrDefault(ai.EngineClaude), cfg.OpenbeeURL, cfg.ExtraEnv()), nil
	})
}

type claudeAdapter struct {
	invoker   *Invoker
	collector *Collector
}

func NewAdapter(binaryPath, openbeeURL string, extraEnv map[string]string) ai.EngineAdapter {
	return &claudeAdapter{
		invoker:   NewInvoker(binaryPath, openbeeURL, extraEnv),
		collector: NewCollector(),
	}
}

func (a *claudeAdapter) Prepare(workDir string, _ ai.PrepareOptions) error {
	rulesPath := filepath.Join(workDir, ai.SystemRulesFile)
	if err := os.Remove(rulesPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("remove %s: %w", ai.SystemRulesFile, err)
	}
	return removeImportLine(workDir)
}

func (a *claudeAdapter) Run(ctx context.Context, workDir, prompt string,
	opts ai.RunOptions, logPath string) (ai.RunResult, error) {
	proc, out, err := a.invoker.Run(ctx, workDir, prompt, opts, logPath)
	return ai.NewRunResult(proc, out, err, ExtractResultFromLog)
}

func (a *claudeAdapter) CollectTokenUsage(ctx context.Context, sessionID string) ([]ai.TokenUsage, error) {
	return a.collector.Collect(ctx, sessionID)
}

func removeImportLine(workDir string) error {
	claudePath := filepath.Join(workDir, "CLAUDE.md")
	data, err := os.ReadFile(claudePath)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read CLAUDE.md: %w", err)
	}

	target := []byte(ai.ImportLine)
	lines := bytes.Split(data, []byte("\n"))
	out := lines[:0]
	for _, line := range lines {
		if !bytes.Equal(bytes.TrimRight(line, "\r"), target) {
			out = append(out, line)
		}
	}
	cleaned := bytes.Join(out, []byte("\n"))
	if bytes.Equal(cleaned, data) {
		return nil
	}
	return os.WriteFile(claudePath, cleaned, 0o644)
}
```

- [ ] **Step 2: Delete `claudemd.go`**

```bash
git rm internal/ai/claude/claudemd.go
```

- [ ] **Step 3: Verify build and tests pass**

```bash
go build ./internal/ai/...
go test ./internal/ai/...
```
Expected: `go build` exits 0; all tests PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/ai/claude/adapter.go
git commit -m "refactor(claude): inline claudemd.go into adapter.go"
```

---

## Task 5: Final verification

- [ ] **Step 1: Full build and test of the entire repo**

```bash
go build ./...
go test ./...
```
Expected: `go build` exits 0; all tests PASS; no compilation errors anywhere.

- [ ] **Step 2: Confirm file structure matches spec**

```bash
ls internal/ai/*.go internal/ai/claude/*.go
```
Expected output (non-test files):
```
internal/ai/contracts.go
internal/ai/dynamic.go
internal/ai/engine_args.go
internal/ai/process.go
internal/ai/prompt.go
internal/ai/registry.go
internal/ai/claude/adapter.go
internal/ai/claude/download.go
internal/ai/claude/invoker.go
internal/ai/claude/provider.go
internal/ai/claude/token_usage.go
```
`engine.go`, `types.go`, `scan.go`, `rules.go`, and `claude/claudemd.go` must NOT appear.
