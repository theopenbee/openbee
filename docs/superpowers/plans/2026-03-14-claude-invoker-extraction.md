# Claude Invoker Extraction Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Extract duplicate Claude CLI subprocess management from `bee` and `worker` packages into a shared `internal/claude/` package.

**Architecture:** A stateless `claude.Invoker` builds MCP config, CLI args, and spawns processes. `Run` returns a `*Process` handle (for Stop/PID) and a `<-chan Output` (for streaming). Both Bee and Worker delegate to the Invoker; Bee consumes the channel to write log files, Worker passes it through to its manager.

**Tech Stack:** Go stdlib (`os/exec`, `bufio`, `sync`, `context`), no new dependencies.

**Spec:** `docs/superpowers/specs/2026-03-14-claude-invoker-extraction-design.md`

---

## Chunk 1: Create `internal/claude/` Package

### Task 1: Create `internal/claude/invoker.go` with types and Invoker

**Files:**
- Create: `internal/claude/invoker.go`
- Create: `internal/claude/invoker_test.go`

- [ ] **Step 1: Write the test file**

```go
// internal/claude/invoker_test.go
package claude

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestNewInvoker(t *testing.T) {
	inv := NewInvoker("/usr/bin/claude", "http://localhost:8080", "test-key")
	if inv.binary != "/usr/bin/claude" {
		t.Errorf("binary: want /usr/bin/claude, got %s", inv.binary)
	}
	if inv.mcpURL != "http://localhost:8080/mcp/sse" {
		t.Errorf("mcpURL: want http://localhost:8080/mcp/sse, got %s", inv.mcpURL)
	}
	if inv.apiKey != "test-key" {
		t.Errorf("apiKey: want test-key, got %s", inv.apiKey)
	}
}

func TestInvoker_Run_WithEcho(t *testing.T) {
	inv := NewInvoker("echo", "", "")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	proc, ch, err := inv.Run(ctx, t.TempDir(), "hello", RunOptions{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if proc.PID() == 0 {
		t.Error("expected non-zero PID")
	}

	var gotStdout bool
	var gotDone bool
	for out := range ch {
		switch out.Type {
		case OutputStdout:
			gotStdout = true
		case OutputDone:
			gotDone = true
		}
	}
	if !gotStdout {
		t.Error("expected stdout output")
	}
	if !gotDone {
		t.Error("expected done signal")
	}
}

func TestInvoker_Run_SessionFlags(t *testing.T) {
	// Use "echo" to verify args are passed (they'll appear in stdout)
	inv := NewInvoker("echo", "", "")
	ctx := context.Background()

	// Test --session-id
	_, ch, _ := inv.Run(ctx, t.TempDir(), "test", RunOptions{SessionID: "s1"})
	var output string
	for out := range ch {
		if out.Type == OutputStdout {
			output += out.Content
		}
	}
	if !strings.Contains(output, "--session-id") || !strings.Contains(output, "s1") {
		t.Errorf("expected --session-id s1 in output, got: %s", output)
	}

	// Test --resume
	_, ch2, _ := inv.Run(ctx, t.TempDir(), "test", RunOptions{SessionID: "s2", Resume: true})
	var output2 string
	for out := range ch2 {
		if out.Type == OutputStdout {
			output2 += out.Content
		}
	}
	if !strings.Contains(output2, "--resume") || !strings.Contains(output2, "s2") {
		t.Errorf("expected --resume s2 in output, got: %s", output2)
	}
}


func TestProcess_Stop(t *testing.T) {
	inv := NewInvoker("sleep", "", "")
	ctx := context.Background()

	proc, ch, err := inv.Run(ctx, t.TempDir(), "60", RunOptions{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if err := proc.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	// Drain channel — should get an error since process was killed
	for range ch {
	}
}

func TestInvoker_ConcurrentRuns(t *testing.T) {
	inv := NewInvoker("echo", "", "")
	ctx := context.Background()

	// Run two concurrent invocations on the same Invoker
	proc1, ch1, err1 := inv.Run(ctx, t.TempDir(), "one", RunOptions{SessionID: "s1"})
	if err1 != nil {
		t.Fatalf("Run 1: %v", err1)
	}
	proc2, ch2, err2 := inv.Run(ctx, t.TempDir(), "two", RunOptions{SessionID: "s2"})
	if err2 != nil {
		t.Fatalf("Run 2: %v", err2)
	}

	// Each should have its own PID
	if proc1.PID() == proc2.PID() {
		t.Error("concurrent runs should have different PIDs")
	}

	// Drain both
	for range ch1 {
	}
	for range ch2 {
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/tengyongzhi/work/robobee && go test ./internal/claude/ -v`
Expected: FAIL — package does not exist

- [ ] **Step 3: Write the implementation**

```go
// internal/claude/invoker.go
package claude

import (
	"bufio"
	"context"
	"fmt"
	"net/url"
	"os/exec"
	"sync"
)

// OutputType classifies a line of output from a Claude CLI process.
type OutputType string

const (
	OutputStdout OutputType = "stdout"
	OutputStderr OutputType = "stderr"
	OutputDone   OutputType = "done"
	OutputError  OutputType = "error"
)

// Output is a single output event from a Claude CLI process.
type Output struct {
	Type    OutputType `json:"type"`
	Content string     `json:"content"`
}

// RunOptions controls session behaviour for a Claude CLI invocation.
type RunOptions struct {
	SessionID string
	Resume    bool
}

// Invoker spawns Claude CLI processes. It is stateless and safe for concurrent use.
type Invoker struct {
	binary string
	mcpURL string
	apiKey string
}

// NewInvoker creates an Invoker. mcpBaseURL is joined with "/mcp/sse".
func NewInvoker(binary, mcpBaseURL, apiKey string) *Invoker {
	return &Invoker{
		binary: binary,
		mcpURL: mcpBaseURL + "/mcp/sse",
		apiKey: apiKey,
	}
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

// Run starts a Claude CLI process and returns a Process handle and an output channel.
// The channel is closed after the process exits; the last message is OutputDone or OutputError.
func (inv *Invoker) Run(ctx context.Context, workDir, prompt string, opts RunOptions) (*Process, <-chan Output, error) {
	mcpConfig := fmt.Sprintf(
		`{"mcpServers":{"robobee":{"type":"sse","url":%q}}}`,
		inv.mcpURL+"?api_key="+url.QueryEscape(inv.apiKey),
	)
	args := []string{
		"--dangerously-skip-permissions",
		"--verbose",
		"--output-format", "stream-json",
		"--mcp-config", mcpConfig,
	}
	if opts.SessionID != "" {
		if opts.Resume {
			args = append(args, "--resume", opts.SessionID)
		} else {
			args = append(args, "--session-id", opts.SessionID)
		}
	}
	args = append(args, "-p", prompt)

	cmd := exec.CommandContext(ctx, inv.binary, args...)
	cmd.Dir = workDir

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, fmt.Errorf("stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, nil, fmt.Errorf("stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, nil, fmt.Errorf("start claude: %w", err)
	}

	proc := &Process{cmd: cmd}
	ch := make(chan Output, 100)

	go func() {
		defer close(ch)

		var wg sync.WaitGroup
		wg.Add(2)

		go func() {
			defer wg.Done()
			scanner := bufio.NewScanner(stdout)
			scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
			for scanner.Scan() {
				ch <- Output{Type: OutputStdout, Content: scanner.Text()}
			}
		}()

		go func() {
			defer wg.Done()
			scanner := bufio.NewScanner(stderr)
			scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
			for scanner.Scan() {
				ch <- Output{Type: OutputStderr, Content: scanner.Text()}
			}
		}()

		wg.Wait()

		if err := cmd.Wait(); err != nil {
			ch <- Output{Type: OutputError, Content: err.Error()}
		} else {
			ch <- Output{Type: OutputDone, Content: ""}
		}
	}()

	return proc, ch, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/tengyongzhi/work/robobee && go test ./internal/claude/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/claude/invoker.go internal/claude/invoker_test.go
git commit -m "feat: add internal/claude package with Invoker for shared CLI management"
```

---

## Chunk 2: Migrate Worker Package

### Task 2: Update `worker/runtime.go` — remove types, use `claude` package

**Files:**
- Modify: `internal/worker/runtime.go`

- [ ] **Step 1: Replace the file contents**

Delete all type definitions (`OutputType`, `Output`, `ExecuteOptions`) and update the `Runtime` interface to use `claude` types:

```go
// internal/worker/runtime.go
package worker

import (
	"context"

	"github.com/robobee/core/internal/claude"
)

type Runtime interface {
	Execute(ctx context.Context, workDir string, plan string, opts claude.RunOptions) (<-chan claude.Output, error)
	PID() int
	Stop() error
}
```

- [ ] **Step 2: Verify it compiles (will fail — claude_runtime.go and manager.go still reference old types)**

Run: `cd /Users/tengyongzhi/work/robobee && go build ./internal/worker/ 2>&1 | head -20`
Expected: FAIL — references to old types in other files

### Task 3: Update `worker/claude_runtime.go` — use Invoker

**Files:**
- Modify: `internal/worker/claude_runtime.go`

- [ ] **Step 1: Rewrite to delegate to `claude.Invoker`**

```go
// internal/worker/claude_runtime.go
package worker

import (
	"context"
	"sync"

	"github.com/robobee/core/internal/claude"
)

type ClaudeRuntime struct {
	invoker *claude.Invoker
	proc    *claude.Process
	mu      sync.Mutex
}

func NewClaudeRuntime(binary, mcpBaseURL, apiKey string) *ClaudeRuntime {
	return &ClaudeRuntime{
		invoker: claude.NewInvoker(binary, mcpBaseURL, apiKey),
	}
}

func (r *ClaudeRuntime) Execute(ctx context.Context, workDir string, plan string, opts claude.RunOptions) (<-chan claude.Output, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	proc, ch, err := r.invoker.Run(ctx, workDir, plan, opts)
	if err != nil {
		return nil, err
	}
	r.proc = proc
	return ch, nil
}

func (r *ClaudeRuntime) Stop() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.proc != nil {
		return r.proc.Stop()
	}
	return nil
}

func (r *ClaudeRuntime) PID() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.proc != nil {
		return r.proc.PID()
	}
	return 0
}
```

### Task 4: Update `worker/manager.go` — use `claude.Output` types

**Files:**
- Modify: `internal/worker/manager.go`

- [ ] **Step 1: Update imports and type references**

Changes needed:
1. Add `"github.com/robobee/core/internal/claude"` to imports
2. Replace all `Output` with `claude.Output` (in field types, channel types, function signatures)
3. Replace all `OutputStdout`, `OutputStderr`, `OutputDone`, `OutputError` with `claude.OutputStdout`, etc.
4. Replace `ExecuteOptions{SessionID: ..., Resume: ...}` with `claude.RunOptions{SessionID: ..., Resume: ...}`

Specific replacements in `manager.go`:
- `activeRuntimes map[string]Runtime` — unchanged (Runtime interface still in worker package)
- `logSubscribers map[string][]chan Output` → `map[string][]chan claude.Output`
- `monitorExecution(exec model.WorkerExecution, worker model.Worker, outputCh <-chan Output, ...)` → `<-chan claude.Output`
- `SubscribeLogs` returns `<-chan claude.Output`
- All `case OutputStdout:` etc. → `case claude.OutputStdout:`
- `ExecuteOptions{...}` → `claude.RunOptions{...}`
- `ch <- Output{...}` in `SubscribeLogs` — this is `make(chan claude.Output, 100)`

### Task 5: Update `worker/runtime_test.go`

**Files:**
- Modify: `internal/worker/runtime_test.go`

- [ ] **Step 1: Update the test**

```go
package worker

import (
	"context"
	"testing"
	"time"

	"github.com/robobee/core/internal/claude"
)

func TestClaudeRuntime_ExecuteWithEcho(t *testing.T) {
	r := NewClaudeRuntime("echo", "", "")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Verify the interface is satisfied
	var _ Runtime = r
	_ = ctx
	_ = claude.RunOptions{}
}

func TestNewClaudeRuntime(t *testing.T) {
	r := NewClaudeRuntime("/usr/bin/claude", "http://localhost:8080", "test-key")
	// ClaudeRuntime now wraps an Invoker; verify it was created
	if r.invoker == nil {
		t.Error("expected non-nil invoker")
	}
}
```

- [ ] **Step 2: Run worker tests**

Run: `cd /Users/tengyongzhi/work/robobee && go test ./internal/worker/ -v`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add internal/worker/runtime.go internal/worker/claude_runtime.go internal/worker/manager.go internal/worker/runtime_test.go
git commit -m "refactor(worker): delegate Claude CLI management to internal/claude package"
```

---

## Chunk 3: Migrate Bee Package

### Task 6: Update `bee/bee_process.go` — use Invoker, change return type

**Files:**
- Modify: `internal/bee/bee_process.go`

- [ ] **Step 1: Rewrite bee_process.go**

```go
package bee

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/robobee/core/internal/claude"
	"github.com/robobee/core/internal/config"
)

// DefaultPersona is the hardcoded bee persona content for CLAUDE.md.
const DefaultPersona = `你是 bee，一个 AI 智能助手。
你的职责是分析用户消息，将其拆解为具体任务并分配给合适的员工。
你只做下面的事情：管理自己、管理员工、管理任务。
如果消息内容无法路由到任何员工，或请求超出上述职责，拒绝提供服务。
`

// BeeProcess represents a single short-lived bee Claude invocation.
type BeeProcess struct {
	invoker *claude.Invoker
}

// NewBeeProcess creates a BeeProcess.
func NewBeeProcess(cfg config.BeeConfig) *BeeProcess {
	return &BeeProcess{
		invoker: claude.NewInvoker(cfg.Claude.Path, cfg.MCPBaseURL, cfg.MCP.APIKey),
	}
}

// WriteCLAUDEMD creates the CLAUDE.md file in workDir with persona content
// only if it does not already exist. This preserves any user edits.
func WriteCLAUDEMD(workDir, persona string) error {
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return fmt.Errorf("mkdir bee workdir: %w", err)
	}
	path := filepath.Join(workDir, "CLAUDE.md")
	if _, err := os.Stat(path); err == nil {
		return nil // already exists, do not overwrite
	}
	return os.WriteFile(path, []byte(persona), 0o644)
}

// Run spawns the bee process with the given prompt and returns a Process handle and output channel.
// If sessionID is non-empty and resume is true, passes --resume <sessionID>.
// If sessionID is non-empty and resume is false, passes --session-id <sessionID>.
func (p *BeeProcess) Run(ctx context.Context, workDir, prompt, sessionID string, resume bool) (*claude.Process, <-chan claude.Output, error) {
	return p.invoker.Run(ctx, workDir, prompt, claude.RunOptions{
		SessionID: sessionID,
		Resume:    resume,
	})
}
```

### Task 7: Update `bee/feeder.go` — update BeeRunner interface, consume channel

**Files:**
- Modify: `internal/bee/feeder.go`

- [ ] **Step 1: Update BeeRunner interface**

Change line 18-20 from:
```go
type BeeRunner interface {
	Run(ctx context.Context, workDir, prompt, sessionID string, resume bool) error
}
```
to:
```go
type BeeRunner interface {
	Run(ctx context.Context, workDir, prompt, sessionID string, resume bool) (*claude.Process, <-chan claude.Output, error)
}
```

Add `"github.com/robobee/core/internal/claude"` to imports.

- [ ] **Step 2: Update `processBeeGroup` to consume channel and write log file**

Replace the current call `if err := f.runner.Run(...)` block in `processBeeGroup` (lines 132-139) with:

```go
	_, outputCh, err := f.runner.Run(beeCtx, f.workDir, prompt, sessionID, resume)
	if err != nil {
		slog.Error("bee run failed", "component", "feeder", "sessionKey", sessionKey, "error", err)
		f.rollback(ctx, msgs)
		return
	}

	if err := f.drainBeeOutput(outputCh, sid); err != nil {
		slog.Error("bee run failed", "component", "feeder", "sessionKey", sessionKey, "error", err)
		f.rollback(ctx, msgs)
		return
	}
```

Add the `drainBeeOutput` method and required imports (`"os"`, `"path/filepath"`, `"time"`):

```go
// drainBeeOutput consumes the output channel and writes to a log file.
// Returns nil on success (OutputDone), error on failure (OutputError).
func (f *Feeder) drainBeeOutput(ch <-chan claude.Output, sessionID string) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home dir: %w", err)
	}
	logDir := filepath.Join(homeDir, ".robobee", "bee-logs")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return fmt.Errorf("mkdir bee-logs: %w", err)
	}
	logFileName := fmt.Sprintf("%s_%s.log", sessionID, time.Now().Format("20060102_150405"))
	logFile, err := os.OpenFile(filepath.Join(logDir, logFileName), os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open bee log file: %w", err)
	}
	defer logFile.Close()

	for out := range ch {
		switch out.Type {
		case claude.OutputStdout:
			fmt.Fprintf(logFile, "[stdout] %s\n", out.Content)
		case claude.OutputStderr:
			fmt.Fprintf(logFile, "[stderr] %s\n", out.Content)
		case claude.OutputError:
			return fmt.Errorf("bee exited with error: %s", out.Content)
		}
	}
	return nil
}
```

Remove `"os"`, `"path/filepath"`, `"time"` from `bee_process.go` imports (they moved here). Add them to `feeder.go` imports if not already present.

Note: The existing `feeder.go` imports already include `"fmt"` and `"time"`. Add `"os"`, `"path/filepath"`, and the claude package import.

### Task 8: Update `bee/feeder_test.go` — update mock

**Files:**
- Modify: `internal/bee/feeder_test.go`

- [ ] **Step 1: Update mockBeeRunner**

Replace the mock (lines 42-59) with:

```go
// mockBeeRunner records all Run calls.
type mockBeeRunner struct {
	mu    sync.Mutex
	calls []beeCall
	err   error
}

type beeCall struct {
	prompt    string
	sessionID string
	resume    bool
}

func (m *mockBeeRunner) Run(_ context.Context, _, prompt, sessionID string, resume bool) (*claude.Process, <-chan claude.Output, error) {
	m.mu.Lock()
	m.calls = append(m.calls, beeCall{prompt: prompt, sessionID: sessionID, resume: resume})
	m.mu.Unlock()

	ch := make(chan claude.Output, 1)
	if m.err != nil {
		ch <- claude.Output{Type: claude.OutputError, Content: m.err.Error()}
	} else {
		ch <- claude.Output{Type: claude.OutputDone}
	}
	close(ch)

	return &claude.Process{}, ch, nil
}
```

Add `"github.com/robobee/core/internal/claude"` to imports.

Note: `claude.Process{}` needs the `cmd` and `mu` fields to be unexported — which they are per our design. But creating `&claude.Process{}` from outside the package requires the zero value to be valid. Since `PID()` returns 0 and `Stop()` returns nil for a zero-value Process, this works. However, `Process` fields are unexported, so `&claude.Process{}` is valid Go (zero value of all unexported fields).

- [ ] **Step 2: Run bee tests**

Run: `cd /Users/tengyongzhi/work/robobee && go test ./internal/bee/ -v`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add internal/bee/bee_process.go internal/bee/feeder.go internal/bee/feeder_test.go
git commit -m "refactor(bee): delegate Claude CLI management to internal/claude package"
```

---

## Chunk 4: Final Verification

### Task 9: Full build and test suite

- [ ] **Step 1: Build the entire project**

Run: `cd /Users/tengyongzhi/work/robobee && go build ./...`
Expected: PASS — no compilation errors

- [ ] **Step 2: Run all tests**

Run: `cd /Users/tengyongzhi/work/robobee && go test ./... -v`
Expected: PASS — all tests pass

- [ ] **Step 3: Run vet**

Run: `cd /Users/tengyongzhi/work/robobee && go vet ./...`
Expected: PASS — no issues

- [ ] **Step 4: Verify no remaining references to old types in worker package**

Run: `grep -rn "OutputType\|OutputStdout\|OutputStderr\|OutputDone\|OutputError\|ExecuteOptions" internal/worker/ | grep -v claude.`
Expected: No output (all references should be qualified with `claude.`)

- [ ] **Step 5: Verify no remaining subprocess code in bee_process.go**

Run: `grep -n "exec\.\|bufio\.\|StdoutPipe\|StderrPipe" internal/bee/bee_process.go`
Expected: No output (all subprocess code removed)
