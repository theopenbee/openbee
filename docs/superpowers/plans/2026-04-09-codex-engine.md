# Codex Engine Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add OpenAI Codex CLI as a second engine in openbee's plugin architecture, selectable via `engine: codex` in config.yaml.

**Architecture:** New `internal/ai/codex` package implements `EngineAdapter` and self-registers as `"codex"` via `init()`. Workspace setup writes `AGENTS.md` + `.openbee.md` (mirroring Claude's dual-file pattern). Process invocation uses `codex exec - --json --yolo` (stdin for new sessions) and `codex exec resume SESSION_ID --json --yolo` for resumes. The `thread_id` from the first `thread.started` JSON event is emitted as `OutputSessionID` so callers can persist it for future resumes.

**Tech Stack:** Go, `os/exec`, `bufio.Scanner`, `encoding/json`, OpenAI Codex CLI binary (`codex`)

---

## File Map

| Action | File | Responsibility |
|--------|------|----------------|
| Modify | `internal/ai/engine.go` | Add `OutputSessionID` constant |
| Modify | `internal/infra/config/config.go` | Add `CodexConfig`, update `BeeConfig` + `EngineConfigRaw()` |
| Create | `internal/ai/codex/agentsmd.go` | Write `AGENTS.md` + `.openbee.md` for bee/worker roles |
| Create | `internal/ai/codex/agentsmd_test.go` | Tests for workspace file content + idempotency |
| Create | `internal/ai/codex/invoker.go` | `codex exec` subprocess, JSON stream parsing, session ID extraction |
| Create | `internal/ai/codex/invoker_test.go` | Tests for command construction + JSON parsing |
| Create | `internal/ai/codex/adapter.go` | `EngineAdapter` implementation + `init()` registration |
| Modify | `internal/app/app.go` | Blank import `_ "…/ai/codex"` |

---

### Task 1: Add `OutputSessionID` to the engine interface

**Files:**
- Modify: `internal/ai/engine.go:31-33`

- [ ] **Step 1: Add the constant**

In `internal/ai/engine.go`, change:

```go
const (
	OutputDone  OutputType = "done"
	OutputError OutputType = "error"
)
```

to:

```go
const (
	OutputDone      OutputType = "done"
	OutputError     OutputType = "error"
	OutputSessionID OutputType = "session_id"
)
```

- [ ] **Step 2: Build to verify no breakage**

```bash
go build ./internal/ai/...
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add internal/ai/engine.go
git commit -m "feat(ai): add OutputSessionID event type"
```

---

### Task 2: Add `CodexConfig` to config

**Files:**
- Modify: `internal/infra/config/config.go:47-93`

- [ ] **Step 1: Write the failing test**

Create `internal/infra/config/codex_config_test.go`:

```go
package config

import (
	"testing"
)

func TestEngineConfigRaw_Codex(t *testing.T) {
	cfg := BeeConfig{
		Engine: "codex",
		Codex:  CodexConfig{Path: "/usr/local/bin/codex"},
	}
	raw := cfg.EngineConfigRaw()
	if raw == nil {
		t.Fatal("expected non-nil raw config for codex engine")
	}
	path, ok := raw["path"].(string)
	if !ok || path != "/usr/local/bin/codex" {
		t.Fatalf("expected path=/usr/local/bin/codex, got %v", raw["path"])
	}
}

func TestEngineConfigRaw_CodexEmptyPath(t *testing.T) {
	cfg := BeeConfig{Engine: "codex"}
	raw := cfg.EngineConfigRaw()
	if raw == nil {
		t.Fatal("expected non-nil raw config for codex engine with empty path")
	}
	path, _ := raw["path"].(string)
	if path != "" {
		t.Fatalf("expected empty path, got %q", path)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

```bash
go test ./internal/infra/config/... -run TestEngineConfigRaw_Codex -v
```

Expected: FAIL — `CodexConfig` undefined.

- [ ] **Step 3: Add `CodexConfig` struct and update `BeeConfig`**

In `internal/infra/config/config.go`, after the `ClaudeConfig` struct (around line 50), add:

```go
type CodexConfig struct {
	Path string `yaml:"path"`
}
```

In `BeeConfig` struct, after the `Claude ClaudeConfig` field, add:

```go
	Codex  CodexConfig  `yaml:"codex"`
```

In `EngineConfigRaw()`, add a case before `default`:

```go
	case "codex":
		return map[string]any{
			"path": b.Codex.Path,
		}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/infra/config/... -run TestEngineConfigRaw_Codex -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/infra/config/config.go internal/infra/config/codex_config_test.go
git commit -m "feat(config): add CodexConfig and EngineConfigRaw support for codex engine"
```

---

### Task 3: Implement workspace file generation (`agentsmd.go`)

**Files:**
- Create: `internal/ai/codex/agentsmd.go`
- Create: `internal/ai/codex/agentsmd_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/ai/codex/agentsmd_test.go`:

```go
package codex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	ai "github.com/theopenbee/openbee/internal/ai"
)

func TestSetupWorkspace_Bee(t *testing.T) {
	dir := t.TempDir()
	if err := setupWorkspace(dir, ai.RoleBee, ai.WorkspaceOptions{}); err != nil {
		t.Fatalf("setupWorkspace: %v", err)
	}

	agentsmd := filepath.Join(dir, "AGENTS.md")
	data, err := os.ReadFile(agentsmd)
	if err != nil {
		t.Fatalf("read AGENTS.md: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "You are B") {
		t.Errorf("AGENTS.md missing bee persona, got: %q", content)
	}
	if !strings.Contains(content, "@.openbee.md") {
		t.Errorf("AGENTS.md missing @.openbee.md import, got: %q", content)
	}

	rules := filepath.Join(dir, ".openbee.md")
	rulesData, err := os.ReadFile(rules)
	if err != nil {
		t.Fatalf("read .openbee.md: %v", err)
	}
	if !strings.Contains(string(rulesData), "openbee-bee") {
		t.Errorf(".openbee.md missing bee rules, got: %q", string(rulesData))
	}
}

func TestSetupWorkspace_Worker(t *testing.T) {
	dir := t.TempDir()
	opts := ai.WorkspaceOptions{
		Name:        "my-worker",
		Description: "does things",
		Memory:      "remember X",
	}
	if err := setupWorkspace(dir, ai.RoleWorker, opts); err != nil {
		t.Fatalf("setupWorkspace: %v", err)
	}

	agentsmd := filepath.Join(dir, "AGENTS.md")
	data, err := os.ReadFile(agentsmd)
	if err != nil {
		t.Fatalf("read AGENTS.md: %v", err)
	}
	if !strings.Contains(string(data), "@.openbee.md") {
		t.Errorf("AGENTS.md missing @.openbee.md import, got: %q", string(data))
	}

	rules := filepath.Join(dir, ".openbee.md")
	rulesData, err := os.ReadFile(rules)
	if err != nil {
		t.Fatalf("read .openbee.md: %v", err)
	}
	content := string(rulesData)
	if !strings.Contains(content, "my-worker") {
		t.Errorf(".openbee.md missing name, got: %q", content)
	}
	if !strings.Contains(content, "does things") {
		t.Errorf(".openbee.md missing description, got: %q", content)
	}
	if !strings.Contains(content, "remember X") {
		t.Errorf(".openbee.md missing memory, got: %q", content)
	}
	if !strings.Contains(content, "openbee-worker") {
		t.Errorf(".openbee.md missing worker rules, got: %q", content)
	}
}

func TestSetupWorkspace_Idempotent(t *testing.T) {
	dir := t.TempDir()
	// First call writes files.
	if err := setupWorkspace(dir, ai.RoleBee, ai.WorkspaceOptions{}); err != nil {
		t.Fatalf("first setupWorkspace: %v", err)
	}
	// Manually modify AGENTS.md to verify second call does NOT overwrite.
	agentsmd := filepath.Join(dir, "AGENTS.md")
	if err := os.WriteFile(agentsmd, []byte("custom content"), 0o644); err != nil {
		t.Fatalf("write custom content: %v", err)
	}
	if err := setupWorkspace(dir, ai.RoleBee, ai.WorkspaceOptions{}); err != nil {
		t.Fatalf("second setupWorkspace: %v", err)
	}
	data, _ := os.ReadFile(agentsmd)
	if string(data) != "custom content" {
		t.Errorf("setupWorkspace overwrote existing AGENTS.md")
	}
}
```

- [ ] **Step 2: Run to verify they fail**

```bash
go test ./internal/ai/codex/... -v
```

Expected: FAIL — package does not exist.

- [ ] **Step 3: Implement `agentsmd.go`**

Create `internal/ai/codex/agentsmd.go`:

```go
package codex

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	ai "github.com/theopenbee/openbee/internal/ai"
)

const (
	systemRulesFile = ".openbee.md"
	importLine      = "@" + systemRulesFile
)

func setupWorkspace(workDir string, role ai.Role, opts ai.WorkspaceOptions) error {
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return fmt.Errorf("mkdir workdir: %w", err)
	}
	switch role {
	case ai.RoleBee:
		if err := writeAgentsMD(workDir, "You are B, an AI assistant.\n"+importLine+"\n"); err != nil {
			return err
		}
		return writeSystemRules(workDir, beeRules())
	case ai.RoleWorker:
		if err := writeAgentsMD(workDir, importLine+"\n"); err != nil {
			return err
		}
		return writeSystemRules(workDir, workerRules(opts.Name, opts.Description, opts.Memory))
	default:
		return fmt.Errorf("unknown role: %q", role)
	}
}

// writeAgentsMD creates workDir/AGENTS.md only if it does not already exist.
func writeAgentsMD(workDir, content string) error {
	path := filepath.Join(workDir, "AGENTS.md")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if errors.Is(err, fs.ErrExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("create AGENTS.md: %w", err)
	}
	defer f.Close()
	_, err = f.WriteString(content)
	return err
}

// writeSystemRules always overwrites .openbee.md with the latest rules.
func writeSystemRules(workDir, content string) error {
	path := filepath.Join(workDir, systemRulesFile)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", systemRulesFile, err)
	}
	return nil
}

func beeRules() string {
	return "You are the coordinator and dispatcher of an AI team. Before processing each user message, you must invoke the Skill tool to load the openbee-bee skill and strictly follow all rules defined in that skill.\n"
}

func workerRules(name, description, memory string) string {
	rules := "You are a Worker in an AI team, responsible for executing tasks assigned to you. You must invoke the Skill tool to load the openbee-worker skill and strictly follow all rules defined in that skill.\n"
	if name != "" {
		rules += fmt.Sprintf("Name: %s\n", name)
	}
	if description != "" {
		rules += fmt.Sprintf("Description: %s\n", description)
	}
	if memory != "" {
		rules += fmt.Sprintf("\n## Memory Constraints\n%s\n", memory)
	}
	return rules
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/ai/codex/... -run TestSetupWorkspace -v
```

Expected: PASS for all three TestSetupWorkspace tests.

- [ ] **Step 5: Commit**

```bash
git add internal/ai/codex/agentsmd.go internal/ai/codex/agentsmd_test.go
git commit -m "feat(codex): implement AGENTS.md + .openbee.md workspace generation"
```

---

### Task 4: Implement subprocess management (`invoker.go`)

**Files:**
- Create: `internal/ai/codex/invoker.go`
- Create: `internal/ai/codex/invoker_test.go`

**Background — Codex JSON event format** (confirmed by running `codex exec - --json --yolo`):

```
{"type":"thread.started","thread_id":"019d7293-0a51-71e0-b634-02183839d7b2"}
{"type":"turn.started"}
{"type":"item.started","item":{"id":"item_0","type":"command_execution",...}}
{"type":"item.completed","item":{"id":"item_1","type":"agent_message","text":"..."}}
{"type":"turn.completed","usage":{...}}
```

Session ID = `thread_id` from the `thread.started` event.
Final result = `text` from the last `item.completed` event where `item.type == "agent_message"`.

- [ ] **Step 1: Write the failing tests**

Create `internal/ai/codex/invoker_test.go`:

```go
package codex

import (
	"strings"
	"testing"
)

func TestBuildArgs_NewSession(t *testing.T) {
	args := buildArgs("", false, "")
	want := []string{"exec", "-", "--json", "--yolo"}
	if !equalSlices(args, want) {
		t.Errorf("got %v, want %v", args, want)
	}
}

func TestBuildArgs_ResumeWithID(t *testing.T) {
	args := buildArgs("sess-123", true, "")
	want := []string{"exec", "resume", "sess-123", "--json", "--yolo"}
	if !equalSlices(args, want) {
		t.Errorf("got %v, want %v", args, want)
	}
}

func TestBuildArgs_ResumeWithIDAndPrompt(t *testing.T) {
	args := buildArgs("sess-123", true, "do something")
	want := []string{"exec", "resume", "sess-123", "--json", "--yolo", "do something"}
	if !equalSlices(args, want) {
		t.Errorf("got %v, want %v", args, want)
	}
}

func TestExtractSessionID(t *testing.T) {
	jsonStream := `{"type":"thread.started","thread_id":"019d7293-0a51-71e0-b634-02183839d7b2"}
{"type":"turn.started"}
{"type":"turn.completed","usage":{}}
`
	id := extractSessionID(strings.NewReader(jsonStream))
	if id != "019d7293-0a51-71e0-b634-02183839d7b2" {
		t.Errorf("got %q, want 019d7293-0a51-71e0-b634-02183839d7b2", id)
	}
}

func TestExtractResultFromLog(t *testing.T) {
	jsonStream := `{"type":"thread.started","thread_id":"abc"}
{"type":"item.completed","item":{"type":"agent_message","text":"hello world"}}
{"type":"turn.completed","usage":{}}
`
	tmpFile := t.TempDir() + "/test.log"
	if err := os.WriteFile(tmpFile, []byte(jsonStream), 0o644); err != nil {
		t.Fatal(err)
	}
	result := ExtractResultFromLog(tmpFile)
	if result != "hello world" {
		t.Errorf("got %q, want %q", result, "hello world")
	}
}

func equalSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
```

Add missing import to the test file (needs `os`):

```go
import (
	"os"
	"strings"
	"testing"
)
```

- [ ] **Step 2: Run to verify they fail**

```bash
go test ./internal/ai/codex/... -run "TestBuildArgs|TestExtract" -v
```

Expected: FAIL — `buildArgs`, `extractSessionID`, `ExtractResultFromLog` undefined.

- [ ] **Step 3: Implement `invoker.go`**

Create `internal/ai/codex/invoker.go`:

```go
package codex

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	ai "github.com/theopenbee/openbee/internal/ai"
)

// Invoker spawns Codex CLI processes. It is stateless and safe for concurrent use.
type Invoker struct {
	binary  string
	baseEnv []string
}

// NewInvoker creates an Invoker. openbeeURL is injected as OPENBEE_URL into subprocesses.
func NewInvoker(binary, openbeeURL string) *Invoker {
	sysEnv := os.Environ()
	env := make([]string, 0, len(sysEnv)+2)
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

// Process represents a running Codex CLI invocation.
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

// codexEvent is the top-level structure of a Codex JSON stream event.
type codexEvent struct {
	Type     string      `json:"type"`
	ThreadID string      `json:"thread_id,omitempty"`
	Item     *codexItem  `json:"item,omitempty"`
}

type codexItem struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// buildArgs constructs the codex CLI arguments.
func buildArgs(sessionID string, resume bool, prompt string) []string {
	if resume && sessionID != "" {
		args := []string{"exec", "resume", sessionID, "--json", "--yolo"}
		if prompt != "" {
			args = append(args, prompt)
		}
		return args
	}
	return []string{"exec", "-", "--json", "--yolo"}
}

// extractSessionID reads a Codex JSON stream and returns the thread_id from
// the first "thread.started" event, or "" if not found.
func extractSessionID(r io.Reader) string {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "{") {
			continue
		}
		var event codexEvent
		if json.Unmarshal([]byte(line), &event) != nil {
			continue
		}
		if event.Type == "thread.started" && event.ThreadID != "" {
			return event.ThreadID
		}
	}
	return ""
}

// ExtractResultFromLog scans a Codex JSON log file and returns the text of the
// last agent_message item, or "" if none found.
func ExtractResultFromLog(logPath string) string {
	f, err := os.Open(logPath)
	if err != nil {
		return ""
	}
	defer f.Close()

	var lastText string
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "{") {
			continue
		}
		var event codexEvent
		if json.Unmarshal([]byte(line), &event) != nil {
			continue
		}
		if event.Type == "item.completed" && event.Item != nil &&
			event.Item.Type == "agent_message" && event.Item.Text != "" {
			lastText = event.Item.Text
		}
	}
	return lastText
}

// Run starts a Codex CLI process, redirecting output to logPath.
// For new sessions (Resume=false), prompt is passed via stdin.
// For resume sessions, prompt is passed as a follow-up argument (if non-empty).
func (inv *Invoker) Run(ctx context.Context, workDir, prompt string, opts ai.RunOptions, logPath string) (ai.Process, <-chan ai.Output, error) {
	args := buildArgs(opts.SessionID, opts.Resume, prompt)

	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return nil, nil, fmt.Errorf("open log file: %w", err)
	}

	// Tee stdout to both the log file and an in-memory pipe for session ID extraction.
	pr, pw := io.Pipe()
	writer := io.MultiWriter(logFile, pw)

	cmd := exec.CommandContext(ctx, inv.binary, args...)
	cmd.Dir = workDir
	cmd.Stdout = writer
	cmd.Stderr = logFile
	cmd.Env = append(inv.baseEnv, "OPENBEE_API_KEY="+opts.APIKey)

	// For new sessions, pass prompt via stdin.
	if !opts.Resume {
		cmd.Stdin = strings.NewReader(prompt)
	}

	if err := cmd.Start(); err != nil {
		logFile.Close()
		pr.Close()
		pw.Close()
		return nil, nil, fmt.Errorf("start codex: %w", err)
	}

	proc := &Process{cmd: cmd}
	ch := make(chan ai.Output, 2)

	go func() {
		defer close(ch)
		defer logFile.Close()

		// Read the pipe to extract session ID, then drain remaining output.
		sessionID := extractSessionID(pr)
		if sessionID != "" {
			ch <- ai.Output{Type: ai.OutputSessionID, Content: sessionID}
		}
		// Drain the rest so the writer doesn't block.
		io.Copy(io.Discard, pr)

		if err := cmd.Wait(); err != nil {
			ch <- ai.Output{Type: ai.OutputError, Content: err.Error()}
		} else {
			ch <- ai.Output{Type: ai.OutputDone}
		}
		pw.Close()
	}()

	return proc, ch, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/ai/codex/... -run "TestBuildArgs|TestExtract" -v
```

Expected: PASS for all four tests.

- [ ] **Step 5: Commit**

```bash
git add internal/ai/codex/invoker.go internal/ai/codex/invoker_test.go
git commit -m "feat(codex): implement invoker with JSON stream parsing and session ID extraction"
```

---

### Task 5: Implement `adapter.go` (EngineAdapter + registration)

**Files:**
- Create: `internal/ai/codex/adapter.go`
- Create: `internal/ai/codex/adapter_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/ai/codex/adapter_test.go`:

```go
package codex

import (
	"os"
	"path/filepath"
	"testing"

	ai "github.com/theopenbee/openbee/internal/ai"
)

func TestAdapter_SetupWorkspace_Bee(t *testing.T) {
	dir := t.TempDir()
	a := NewAdapter("codex", "http://localhost:8080")
	if err := a.SetupWorkspace(dir, ai.RoleBee, ai.WorkspaceOptions{}); err != nil {
		t.Fatalf("SetupWorkspace: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "AGENTS.md")); err != nil {
		t.Errorf("AGENTS.md not created: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".openbee.md")); err != nil {
		t.Errorf(".openbee.md not created: %v", err)
	}
}

func TestAdapter_SetupWorkspace_Worker(t *testing.T) {
	dir := t.TempDir()
	a := NewAdapter("codex", "http://localhost:8080")
	opts := ai.WorkspaceOptions{Name: "w1", Description: "desc", Memory: "mem"}
	if err := a.SetupWorkspace(dir, ai.RoleWorker, opts); err != nil {
		t.Fatalf("SetupWorkspace: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "AGENTS.md")); err != nil {
		t.Errorf("AGENTS.md not created: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".openbee.md")); err != nil {
		t.Errorf(".openbee.md not created: %v", err)
	}
}

func TestAdapter_SetupWorkspace_UnknownRole(t *testing.T) {
	dir := t.TempDir()
	a := NewAdapter("codex", "http://localhost:8080")
	err := a.SetupWorkspace(dir, ai.Role("unknown"), ai.WorkspaceOptions{})
	if err == nil {
		t.Error("expected error for unknown role, got nil")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

```bash
go test ./internal/ai/codex/... -run TestAdapter -v
```

Expected: FAIL — `NewAdapter` undefined.

- [ ] **Step 3: Implement `adapter.go`**

Create `internal/ai/codex/adapter.go`:

```go
package codex

import (
	"context"
	"fmt"

	ai "github.com/theopenbee/openbee/internal/ai"
)

func init() {
	ai.Register("codex", func(cfg ai.EngineConfig) (ai.EngineAdapter, error) {
		path, _ := cfg.Raw["path"].(string)
		if path == "" {
			path = "codex"
		}
		return NewAdapter(path, cfg.OpenbeeURL), nil
	})
}

type codexAdapter struct {
	invoker *Invoker
}

// NewAdapter creates a codexAdapter.
func NewAdapter(binaryPath, openbeeURL string) ai.EngineAdapter {
	return &codexAdapter{invoker: NewInvoker(binaryPath, openbeeURL)}
}

func (a *codexAdapter) SetupWorkspace(workDir string, role ai.Role, opts ai.WorkspaceOptions) error {
	return setupWorkspace(workDir, role, opts)
}

func (a *codexAdapter) Run(ctx context.Context, workDir, prompt string,
	opts ai.RunOptions, logPath string) (ai.Process, <-chan ai.Output, error) {
	return a.invoker.Run(ctx, workDir, prompt, opts, logPath)
}

// Ensure codexAdapter satisfies the interface at compile time.
var _ ai.EngineAdapter = (*codexAdapter)(nil)

// compile-time check that Process implements ai.Process
var _ ai.Process = (*Process)(nil)

// unknownRoleError is returned by setupWorkspace for unrecognised roles.
func unknownRoleError(role ai.Role) error {
	return fmt.Errorf("unknown role: %q", role)
}
```

- [ ] **Step 4: Run all codex tests**

```bash
go test ./internal/ai/codex/... -v
```

Expected: all tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/ai/codex/adapter.go internal/ai/codex/adapter_test.go
git commit -m "feat(codex): implement EngineAdapter and init() registration"
```

---

### Task 6: Wire Codex into the application

**Files:**
- Modify: `internal/app/app.go:25`

- [ ] **Step 1: Add blank import**

In `internal/app/app.go`, after the existing `_ "github.com/theopenbee/openbee/internal/ai/claude"` import line, add:

```go
_ "github.com/theopenbee/openbee/internal/ai/codex"
```

- [ ] **Step 2: Build the full application**

```bash
go build ./...
```

Expected: no errors.

- [ ] **Step 3: Verify codex engine is selectable**

```bash
go run . --help 2>&1 | head -5
```

The app should build without errors. To smoke-test the engine registration, you can temporarily add this to a test file and remove it after:

```go
func TestCodexEngineRegistered(t *testing.T) {
	cfg := ai.EngineConfig{Raw: map[string]any{"path": "echo"}, OpenbeeURL: "http://x"}
	eng, err := ai.New("codex", cfg)
	if err != nil {
		t.Fatalf("codex engine not registered: %v", err)
	}
	if eng == nil {
		t.Fatal("expected non-nil engine")
	}
}
```

Run: `go test ./internal/ai/codex/... -run TestCodexEngineRegistered -v`

But note: this test must import the `ai` package (not `codex` package) — put it in `internal/ai/codex/adapter_test.go` using `package codex` (it already imports `ai`). Actually since `init()` in `adapter.go` registers it, and the test is in the same package, the registration happens automatically. Remove this test after verifying.

- [ ] **Step 4: Commit**

```bash
git add internal/app/app.go
git commit -m "feat(app): register codex engine adapter"
```

---

## Self-Review

**Spec coverage check:**

| Spec requirement | Task |
|---|---|
| New package `internal/ai/codex/` with adapter, invoker, agentsmd | Tasks 3–5 |
| `OutputSessionID` constant | Task 1 |
| `AGENTS.md` + `.openbee.md` for bee/worker | Task 3 |
| `codex exec - --json --yolo` for new sessions | Task 4 |
| `codex exec resume SESSION_ID --json --yolo` for resume | Task 4 |
| `OPENBEE_URL` + `OPENBEE_API_KEY` env injection | Task 4 |
| session ID extracted from `thread.started` event | Task 4 |
| `OutputSessionID` emitted via Output channel | Task 4 |
| `CodexConfig` + `EngineConfigRaw` | Task 2 |
| Blank import in `app.go` | Task 6 |

All spec requirements covered. No placeholders. Types and method names consistent across tasks.
