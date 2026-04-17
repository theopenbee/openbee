# Kimi Engine Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add Kimi as a fourth AI engine (alongside Claude, Codex, Pi) with full backend invocation, result extraction, frontend log rendering, and i18n support.

**Architecture:** New `internal/ai/kimi/` package follows the Pi adapter pattern — `init()` factory registration, an `Invoker` that pipes prompt via stdin and passes `opts.SessionID` directly as `--session=<UUID>`, and role-based stream-JSON log parsing. Frontend gets a `KimiParser` that handles the role-based message format, and `detectEngine` is extended to recognise top-level `role` fields.

**Tech Stack:** Go (backend engine), TypeScript/React (frontend log viewer), Vitest (frontend tests), Go test (backend tests)

---

### Task 1: Add Kimi engine constant and update AllEngines

**Files:**
- Modify: `internal/ai/engine.go:49-54`

- [ ] **Step 1: Add the constant and update AllEngines**

In `internal/ai/engine.go`, change the constants block and `AllEngines` from:

```go
const (
	EngineClaude = "claude"
	EngineCodex  = "codex"
	EnginePi     = "pi"
)

var AllEngines = []string{EngineClaude, EngineCodex, EnginePi}
```

to:

```go
const (
	EngineClaude = "claude"
	EngineCodex  = "codex"
	EnginePi     = "pi"
	EngineKimi   = "kimi"
)

var AllEngines = []string{EngineClaude, EngineCodex, EnginePi, EngineKimi}
```

- [ ] **Step 2: Verify existing validateEngine tests still pass**

```bash
go test ./internal/ai/... -run TestValidateEngine -v
```

Expected: all existing tests PASS; `ValidateEngine("kimi")` would now return `nil` (verified in Task 2).

- [ ] **Step 3: Commit**

```bash
git add internal/ai/engine.go
git commit -m "feat(engine): add EngineKimi constant and register in AllEngines"
```

---

### Task 2: Add KimiConfig to config

**Files:**
- Modify: `internal/infra/config/config.go:59-133`

- [ ] **Step 1: Add KimiConfig struct after PiConfig**

After the `PiConfig` struct (line ~73), add:

```go
type KimiConfig struct {
	Path    string            `yaml:"path"`
	Timeout time.Duration     `yaml:"timeout"`
	Env     map[string]string `yaml:"env"`
}
```

- [ ] **Step 2: Add Kimi field to BeeConfig**

In `BeeConfig` struct, add after `Pi PiConfig`:

```go
Kimi KimiConfig `yaml:"kimi"`
```

- [ ] **Step 3: Update WorkerTimeout() to handle kimi**

Change `WorkerTimeout()` from:

```go
func (b BeeConfig) WorkerTimeout() time.Duration {
	switch b.EffectiveEngine() {
	case "codex":
		return b.Codex.Timeout
	case "pi":
		return b.Pi.Timeout
	default:
		return b.Claude.Timeout
	}
}
```

to:

```go
func (b BeeConfig) WorkerTimeout() time.Duration {
	switch b.EffectiveEngine() {
	case "codex":
		return b.Codex.Timeout
	case "pi":
		return b.Pi.Timeout
	case "kimi":
		return b.Kimi.Timeout
	default:
		return b.Claude.Timeout
	}
}
```

- [ ] **Step 4: Update EngineConfigRawFor() to handle kimi**

Change `EngineConfigRawFor()` from:

```go
func (b BeeConfig) EngineConfigRawFor(name string) map[string]any {
	switch name {
	case "claude":
		return map[string]any{"path": b.Claude.Path}
	case "codex":
		return map[string]any{"path": b.Codex.Path}
	case "pi":
		return map[string]any{"path": b.Pi.Path, "env": b.Pi.Env}
	default:
		return nil
	}
}
```

to:

```go
func (b BeeConfig) EngineConfigRawFor(name string) map[string]any {
	switch name {
	case "claude":
		return map[string]any{"path": b.Claude.Path}
	case "codex":
		return map[string]any{"path": b.Codex.Path}
	case "pi":
		return map[string]any{"path": b.Pi.Path, "env": b.Pi.Env}
	case "kimi":
		return map[string]any{"path": b.Kimi.Path, "env": b.Kimi.Env}
	default:
		return nil
	}
}
```

- [ ] **Step 5: Compile-check**

```bash
go build ./internal/infra/config/...
```

Expected: exits 0, no output.

- [ ] **Step 6: Commit**

```bash
git add internal/infra/config/config.go
git commit -m "feat(config): add KimiConfig and wire into BeeConfig, WorkerTimeout, EngineConfigRawFor"
```

---

### Task 3: Create kimi invoker with tests (TDD)

**Files:**
- Create: `internal/ai/kimi/invoker.go`
- Create: `internal/ai/kimi/invoker_test.go`

- [ ] **Step 1: Write failing tests**

Create `internal/ai/kimi/invoker_test.go`:

```go
package kimi

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "kimi-log-*.jsonl")
	if err != nil {
		t.Fatalf("create temp: %v", err)
	}
	if _, err := f.WriteString(content); err != nil {
		t.Fatalf("write temp: %v", err)
	}
	f.Close()
	return f.Name()
}

func TestBuildArgs(t *testing.T) {
	args := buildArgs("550e8400-e29b-41d4-a716-446655440000")
	want := []string{
		"--session=550e8400-e29b-41d4-a716-446655440000",
		"--yolo",
		"--output-format=stream-json",
		"--print",
	}
	if !slices.Equal(args, want) {
		t.Errorf("got %v, want %v", args, want)
	}
}

func TestExtractResultFromLog_StringContent(t *testing.T) {
	log := `{"role":"user","content":"hello"}
{"role":"assistant","content":"world"}
`
	path := writeTemp(t, log)
	got := ExtractResultFromLog(path)
	if got != "world" {
		t.Errorf("got %q, want %q", got, "world")
	}
}

func TestExtractResultFromLog_ArrayContent(t *testing.T) {
	log := `{"role":"assistant","content":[{"type":"text","text":"array answer"}]}
`
	path := writeTemp(t, log)
	got := ExtractResultFromLog(path)
	if got != "array answer" {
		t.Errorf("got %q, want %q", got, "array answer")
	}
}

func TestExtractResultFromLog_ArrayContentFirstTextBlock(t *testing.T) {
	log := `{"role":"assistant","content":[{"type":"tool_use","id":"tc_1"},{"type":"text","text":"after tool"}]}
`
	path := writeTemp(t, log)
	got := ExtractResultFromLog(path)
	if got != "after tool" {
		t.Errorf("got %q, want %q", got, "after tool")
	}
}

func TestExtractResultFromLog_LastAssistantWins(t *testing.T) {
	log := `{"role":"assistant","content":"first"}
{"role":"tool","tool_call_id":"tc_1","content":"result"}
{"role":"assistant","content":"last"}
`
	path := writeTemp(t, log)
	got := ExtractResultFromLog(path)
	if got != "last" {
		t.Errorf("got %q, want %q", got, "last")
	}
}

func TestExtractResultFromLog_Empty(t *testing.T) {
	path := writeTemp(t, "")
	got := ExtractResultFromLog(path)
	if got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestExtractResultFromLog_NoAssistant(t *testing.T) {
	log := `{"role":"user","content":"hi"}
{"role":"tool","tool_call_id":"x","content":"done"}
`
	path := writeTemp(t, log)
	got := ExtractResultFromLog(path)
	if got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestExtractResultFromLog_MissingFile(t *testing.T) {
	got := ExtractResultFromLog(filepath.Join(t.TempDir(), "nonexistent.jsonl"))
	if got != "" {
		t.Errorf("got %q, want empty", got)
	}
}
```

- [ ] **Step 2: Run tests to confirm they fail**

```bash
go test ./internal/ai/kimi/... -v
```

Expected: compile error — package does not exist yet.

- [ ] **Step 3: Write the invoker implementation**

Create `internal/ai/kimi/invoker.go`:

```go
package kimi

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	ai "github.com/theopenbee/openbee/internal/ai"
)

// Invoker spawns Kimi CLI processes. It is stateless and safe for concurrent use.
type Invoker struct {
	binary  string
	baseEnv []string
}

// NewInvoker creates an Invoker. openbeeURL is injected as OPENBEE_URL into subprocesses.
// extraEnv entries are merged into the base environment (e.g. MOONSHOT_API_KEY).
func NewInvoker(binary, openbeeURL string, extraEnv map[string]string) *Invoker {
	base := ai.BuildBaseEnv(openbeeURL)
	for k, v := range extraEnv {
		if v != "" {
			base = append(base, k+"="+v)
		}
	}
	base = base[:len(base):len(base)]
	return &Invoker{binary: binary, baseEnv: base}
}

func buildArgs(sessionID string) []string {
	return []string{
		"--session=" + sessionID,
		"--yolo",
		"--output-format=stream-json",
		"--print",
	}
}

type kimiMessage struct {
	Role      string          `json:"role"`
	Content   json.RawMessage `json:"content"`
	ToolCalls []struct{}      `json:"tool_calls,omitempty"`
}

type kimiContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// ExtractResultFromLog scans a Kimi stream-json log and returns the text of the
// last role=assistant message, or "" if none found.
// The content field may be a plain string or an array of content blocks.
func ExtractResultFromLog(logPath string) string {
	f, err := os.Open(logPath)
	if err != nil {
		return ""
	}
	defer f.Close()

	var lastText string
	ai.ScanJSONLines(f, func(line string) bool {
		var msg kimiMessage
		if json.Unmarshal([]byte(line), &msg) != nil || msg.Role != "assistant" {
			return true
		}
		if len(msg.Content) == 0 {
			return true
		}
		// Try string content first.
		var s string
		if json.Unmarshal(msg.Content, &s) == nil && s != "" {
			lastText = s
			return true
		}
		// Try array of content blocks.
		var blocks []kimiContentBlock
		if json.Unmarshal(msg.Content, &blocks) != nil {
			return true
		}
		for _, b := range blocks {
			if b.Type == "text" && b.Text != "" {
				lastText = b.Text
				break
			}
		}
		return true
	})
	return lastText
}

// Run starts a Kimi CLI process, redirecting output to logPath.
// The prompt is passed via stdin; opts.SessionID is passed as --session=<UUID>.
func (inv *Invoker) Run(ctx context.Context, workDir, prompt string,
	opts ai.RunOptions, logPath string) (ai.Process, <-chan ai.Output, error) {

	args := buildArgs(opts.SessionID)

	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return nil, nil, fmt.Errorf("open log file: %w", err)
	}

	cmd := exec.CommandContext(ctx, inv.binary, args...)
	cmd.Dir = workDir
	cmd.Stdin = strings.NewReader(prompt)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Env = ai.BuildRunEnv(inv.baseEnv, opts.ExtraEnv, opts.APIKey)

	if err := cmd.Start(); err != nil {
		logFile.Close()
		return nil, nil, fmt.Errorf("start kimi: %w", err)
	}

	proc := ai.NewCmdProcess(cmd)
	ch := make(chan ai.Output, 1)

	go func() {
		defer close(ch)
		defer logFile.Close()
		if err := cmd.Wait(); err != nil {
			ch <- ai.Output{Type: ai.OutputError, Content: err.Error()}
			return
		}
		ch <- ai.Output{Type: ai.OutputDone}
	}()

	return proc, ch, nil
}
```

- [ ] **Step 4: Run tests to confirm they pass**

```bash
go test ./internal/ai/kimi/... -v
```

Expected: all 7 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/ai/kimi/invoker.go internal/ai/kimi/invoker_test.go
git commit -m "feat(kimi): add invoker with ExtractResultFromLog and Run"
```

---

### Task 4: Create kimi adapter and register with app

**Files:**
- Create: `internal/ai/kimi/adapter.go`
- Create: `internal/ai/kimi/adapter_test.go`
- Modify: `internal/app/app.go:25-27`

- [ ] **Step 1: Write failing adapter test**

Create `internal/ai/kimi/adapter_test.go`:

```go
package kimi_test

import (
	"os"
	"testing"

	ai "github.com/theopenbee/openbee/internal/ai"
	"github.com/theopenbee/openbee/internal/ai/kimi"
)

func TestAdapter_Prepare_NoOp(t *testing.T) {
	dir := t.TempDir()
	a := kimi.NewAdapter("echo", "http://localhost:9999", nil)

	if err := a.Prepare(dir, ai.PrepareOptions{Role: ai.RoleBee}); err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Errorf("Prepare must not create files, found: %v", entries)
	}
}

func TestAdapter_Prepare_BothRoles(t *testing.T) {
	a := kimi.NewAdapter("echo", "http://localhost:9999", nil)
	for _, role := range []ai.Role{ai.RoleBee, ai.RoleWorker} {
		dir := t.TempDir()
		if err := a.Prepare(dir, ai.PrepareOptions{Role: role}); err != nil {
			t.Errorf("Prepare(%s): %v", role, err)
		}
	}
}
```

- [ ] **Step 2: Run test to confirm it fails**

```bash
go test ./internal/ai/kimi/... -run TestAdapter -v
```

Expected: FAIL — `kimi.NewAdapter` not defined.

- [ ] **Step 3: Write the adapter**

Create `internal/ai/kimi/adapter.go`:

```go
package kimi

import (
	"context"

	ai "github.com/theopenbee/openbee/internal/ai"
)

func init() {
	ai.Register(ai.EngineKimi, func(cfg ai.EngineConfig) (ai.EngineAdapter, error) {
		path, _ := cfg.Raw["path"].(string)
		if path == "" {
			path = ai.EngineKimi
		}
		extraEnv, _ := cfg.Raw["env"].(map[string]string)
		return NewAdapter(path, cfg.OpenbeeURL, extraEnv), nil
	})
}

type kimiAdapter struct {
	invoker *Invoker
}

// NewAdapter creates a kimiAdapter. It is exported for use in tests.
func NewAdapter(binaryPath, openbeeURL string, extraEnv map[string]string) ai.EngineAdapter {
	return &kimiAdapter{invoker: NewInvoker(binaryPath, openbeeURL, extraEnv)}
}

func (a *kimiAdapter) Prepare(_ string, _ ai.PrepareOptions) error {
	return nil
}

func (a *kimiAdapter) Run(ctx context.Context, workDir, prompt string,
	opts ai.RunOptions, logPath string) (ai.Process, <-chan ai.Output, error) {
	return a.invoker.Run(ctx, workDir, prompt, opts, logPath)
}

func (a *kimiAdapter) ExtractResult(logPath string) string {
	return ExtractResultFromLog(logPath)
}

var _ ai.EngineAdapter = (*kimiAdapter)(nil)
```

- [ ] **Step 4: Run tests to confirm they pass**

```bash
go test ./internal/ai/kimi/... -v
```

Expected: all tests PASS.

- [ ] **Step 5: Add blank import to app.go**

In `internal/app/app.go`, add `_ "github.com/theopenbee/openbee/internal/ai/kimi"` alongside the existing engine imports:

```go
_ "github.com/theopenbee/openbee/internal/ai/claude"
_ "github.com/theopenbee/openbee/internal/ai/codex"
_ "github.com/theopenbee/openbee/internal/ai/kimi"
_ "github.com/theopenbee/openbee/internal/ai/pi"
```

- [ ] **Step 6: Verify full build**

```bash
go build ./...
```

Expected: exits 0, no output.

- [ ] **Step 7: Run all engine tests**

```bash
go test ./internal/ai/... -v
```

Expected: all tests PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/ai/kimi/adapter.go internal/ai/kimi/adapter_test.go internal/app/app.go
git commit -m "feat(kimi): add adapter, register factory, wire into app bootstrap"
```

---

### Task 5: Update config template and interactive config wizard

**Files:**
- Modify: `internal/infra/config/config.yaml.tmpl`
- Modify: `cmd/openbee/config.go` (configValues struct, loadExistingConfig, runConfig defaults + engine select)
- Create: `cmd/openbee/config_kimi.go`
- Modify: `internal/infra/i18n/messages.go`
- Modify: `internal/infra/i18n/locales/en.yaml`
- Modify: `internal/infra/i18n/locales/zh.yaml`

- [ ] **Step 1: Add kimi section to config.yaml.tmpl**

In `internal/infra/config/config.yaml.tmpl`, add a kimi block after the `pi` block (after line ~34):

```yaml
  kimi:
    path: {{.KimiPath}}
    timeout: {{.KimiTimeout}}
    {{- if .KimiEnv}}
    env:
    {{- range $k, $v := .KimiEnv}}
      {{$k}}: "{{$v}}"
    {{- end}}
    {{- end}}
```

- [ ] **Step 2: Add Kimi fields to configValues struct**

In `cmd/openbee/config.go`, add three fields to `configValues` after `PiEnv`:

```go
KimiPath    string            // existing: PiEnv map[string]string
KimiTimeout string
KimiEnv     map[string]string
```

The full addition (place after `PiEnv map[string]string`):

```go
KimiPath    string            `// no yaml tag — this is a template data struct`
KimiTimeout string
KimiEnv     map[string]string
```

In Go the struct fields have no tags; add them just after `PiEnv`:

```go
PiEnv    map[string]string

KimiPath    string
KimiTimeout string
KimiEnv     map[string]string
```

- [ ] **Step 3: Add kimi to loadExistingConfig**

In `loadExistingConfig`, after the Pi fields, add:

```go
KimiPath:    cfg.Bee.Kimi.Path,
KimiTimeout: cfg.Bee.Kimi.Timeout.String(),
KimiEnv:     cfg.Bee.Kimi.Env,
```

- [ ] **Step 4: Add kimi defaults to runConfig**

In `runConfig`, after `PiTimeout: "30m"`, add:

```go
KimiPath:    "kimi",
KimiTimeout: "30m",
```

- [ ] **Step 5: Add Kimi to engine select survey options**

In `runConfig`, find the `survey.Select` for engine selection. Change:

```go
Options: []string{
    i18n.M.Prompt.OptionEngineClaude,
    i18n.M.Prompt.OptionEngineCodex,
    i18n.M.Prompt.OptionEnginePi,
},
```

to:

```go
Options: []string{
    i18n.M.Prompt.OptionEngineClaude,
    i18n.M.Prompt.OptionEngineCodex,
    i18n.M.Prompt.OptionEnginePi,
    i18n.M.Prompt.OptionEngineKimi,
},
```

Also update the `defaultEngineOpt` switch and the `selectedEngine` switch to handle kimi:

In the `defaultEngineOpt` switch, add:
```go
case "kimi":
    defaultEngineOpt = i18n.M.Prompt.OptionEngineKimi
```

In the `selectedEngine` switch, add:
```go
case i18n.M.Prompt.OptionEngineKimi:
    vals.Engine = "kimi"
    if err := configureKimiExecutable(&vals); err != nil {
        return err
    }
```

- [ ] **Step 6: Create cmd/openbee/config_kimi.go**

```go
package main

import "github.com/theopenbee/openbee/internal/infra/i18n"

func configureKimiExecutable(vals *configValues) error {
	return configureEngineExecutable(
		"kimi",
		i18n.M.Output.Config.KimiFound,
		i18n.M.Output.Config.KimiManualEntry,
		i18n.M.Prompt.KimiPath,
		i18n.M.Prompt.KimiTimeout,
		&vals.KimiPath,
		&vals.KimiTimeout,
	)
}
```

- [ ] **Step 7: Add Kimi fields to i18n messages.go**

In `internal/infra/i18n/messages.go`, add `OptionEngineKimi` to the prompt section after `OptionEnginePi`:

```go
OptionEngineKimi    string `yaml:"option_engine_kimi"`
```

Add `KimiPath` and `KimiTimeout` to the prompt section after `PiTimeout`:

```go
KimiPath    string `yaml:"kimi_path"`
KimiTimeout string `yaml:"kimi_timeout"`
```

Add `KimiFound` and `KimiManualEntry` to the output/config section after `PiManualEntry`:

```go
KimiFound       string `yaml:"kimi_found"`        // contains %s
KimiManualEntry string `yaml:"kimi_manual_entry"`
```

- [ ] **Step 8: Add kimi strings to internal/infra/i18n/locales/en.yaml**

Add after `option_engine_pi: "Pi"`:
```yaml
  option_engine_kimi: "Kimi"
```

Add after `pi_timeout: "Pi timeout:"`:
```yaml
  kimi_path: "Kimi executable path:"
  kimi_timeout: "Kimi timeout:"
```

Add after `pi_manual_entry` in the output/config section:
```yaml
    kimi_found: "Found Kimi in PATH: %s, using it automatically."
    kimi_manual_entry: "Kimi not found in PATH. Please enter the path manually."
```

- [ ] **Step 9: Add kimi strings to internal/infra/i18n/locales/zh.yaml**

Add after `option_engine_pi: "Pi"`:
```yaml
  option_engine_kimi: "Kimi"
```

Add after `pi_timeout: "Pi 超时时间："`:
```yaml
  kimi_path: "Kimi 可执行文件路径："
  kimi_timeout: "Kimi 超时时间："
```

Add after `pi_manual_entry` in the output/config section:
```yaml
    kimi_found: "在 PATH 中找到 Kimi：%s，将自动使用。"
    kimi_manual_entry: "未在 PATH 中找到 Kimi，请手动输入路径。"
```

- [ ] **Step 10: Build to confirm no compile errors**

```bash
go build ./...
```

Expected: exits 0, no output.

- [ ] **Step 11: Commit**

```bash
git add internal/infra/config/config.yaml.tmpl \
        cmd/openbee/config.go \
        cmd/openbee/config_kimi.go \
        internal/infra/i18n/messages.go \
        internal/infra/i18n/locales/en.yaml \
        internal/infra/i18n/locales/zh.yaml
git commit -m "feat(config): add kimi to config template, wizard, and i18n"
```

---

### Task 6: Add kimi to frontend types and i18n

**Files:**
- Modify: `web/src/lib/types.ts:136`
- Modify: `web/src/locales/en.json:114-118`
- Modify: `web/src/locales/zh.json:114-118`

- [ ] **Step 1: Add "kimi" to ENGINES**

In `web/src/lib/types.ts`, change:

```typescript
export const ENGINES = ["claude", "codex", "pi"] as const
```

to:

```typescript
export const ENGINES = ["claude", "codex", "pi", "kimi"] as const
```

- [ ] **Step 2: Add i18n key to en.json**

In `web/src/locales/en.json`, change the engines section:

```json
"engines": {
  "claude": "Claude Code",
  "codex": "Codex",
  "pi": "Pi"
}
```

to:

```json
"engines": {
  "claude": "Claude Code",
  "codex": "Codex",
  "pi": "Pi",
  "kimi": "Kimi"
}
```

- [ ] **Step 3: Add i18n key to zh.json**

In `web/src/locales/zh.json`, change the engines section:

```json
"engines": {
  "claude": "Claude Code",
  "codex": "Codex",
  "pi": "Pi"
}
```

to:

```json
"engines": {
  "claude": "Claude Code",
  "codex": "Codex",
  "pi": "Pi",
  "kimi": "Kimi"
}
```

- [ ] **Step 4: TypeScript check**

```bash
cd web && npx tsc --noEmit
```

Expected: exits 0, no type errors.

- [ ] **Step 5: Commit**

```bash
git add web/src/lib/types.ts web/src/locales/en.json web/src/locales/zh.json
git commit -m "feat(frontend): add kimi to ENGINES type and i18n labels"
```

---

### Task 7: Update engine detection for Kimi logs

**Files:**
- Modify: `web/src/components/log-viewer/detect-engine.ts`
- Modify: `web/src/components/log-viewer/__tests__/detect-engine.test.ts`

- [ ] **Step 1: Write failing tests for kimi detection**

In `web/src/components/log-viewer/__tests__/detect-engine.test.ts`, add these test cases inside the existing `describe("detectEngine", ...)` block:

```typescript
it("returns 'kimi' when first line has role but no type", () => {
  const line = JSON.stringify({ role: "user", content: "hello" })
  expect(detectEngine([line])).toBe("kimi")
})

it("returns 'kimi' when assistant role line appears first", () => {
  const line = JSON.stringify({ role: "assistant", content: "hi" })
  expect(detectEngine([line])).toBe("kimi")
})

it("does not mistake Claude assistant event for kimi", () => {
  // Claude lines have a top-level "type" field
  const line = JSON.stringify({ type: "assistant", message: { content: [] } })
  expect(detectEngine([line])).toBe("claude")
})

it("returns 'kimi' when kimi line appears after an unknown line", () => {
  const unknown = "not json"
  const kimiLine = JSON.stringify({ role: "user", content: "hello" })
  expect(detectEngine([unknown, kimiLine])).toBe("kimi")
})
```

- [ ] **Step 2: Run tests to confirm they fail**

```bash
cd web && npx vitest run src/components/log-viewer/__tests__/detect-engine.test.ts
```

Expected: the 4 new kimi tests FAIL.

- [ ] **Step 3: Update detect-engine.ts**

Replace the content of `web/src/components/log-viewer/detect-engine.ts` with:

```typescript
import { parseJsonEvent } from "./types"

function hasTopLevelRole(line: string): boolean {
  try {
    const obj = JSON.parse(line)
    return obj !== null && typeof obj === "object" && typeof obj.role === "string" && typeof obj.type === "undefined"
  } catch {
    return false
  }
}

export function detectEngine(lines: string[]): "claude" | "codex" | "pi" | "kimi" {
  for (const line of lines) {
    const event = parseJsonEvent<{ type: string }>(line)
    if (event?.type === "thread.started") return "codex"
    if (event?.type === "agent_start") return "pi"
    if (hasTopLevelRole(line)) return "kimi"
  }
  return "claude"
}
```

- [ ] **Step 4: Run all detect-engine tests**

```bash
cd web && npx vitest run src/components/log-viewer/__tests__/detect-engine.test.ts
```

Expected: all tests PASS (including the 4 new ones).

- [ ] **Step 5: Commit**

```bash
git add web/src/components/log-viewer/detect-engine.ts \
        web/src/components/log-viewer/__tests__/detect-engine.test.ts
git commit -m "feat(log-viewer): detect Kimi engine from top-level role field"
```

---

### Task 8: Implement KimiParser with tests (TDD)

**Files:**
- Create: `web/src/components/log-viewer/kimi-parser.ts`
- Create: `web/src/components/log-viewer/__tests__/kimi-parser.test.ts`

- [ ] **Step 1: Write failing tests**

Create `web/src/components/log-viewer/__tests__/kimi-parser.test.ts`:

```typescript
import { describe, expect, it } from "vitest"
import { KimiParser } from "../kimi-parser"
import type { ParsedEntry } from "../types"

function run(lines: string[], logType = "stdout"): ParsedEntry[] {
  const parser = new KimiParser()
  const entries: ParsedEntry[] = []
  const itemMap = new Map<string, number>()
  for (const line of lines) parser.parseLine(line, logType, entries, itemMap)
  return entries
}

describe("KimiParser", () => {
  // --- role=user ---

  it("skips role=user lines", () => {
    const line = JSON.stringify({ role: "user", content: "hello" })
    expect(run([line])).toHaveLength(0)
  })

  // --- role=assistant string content ---

  it("creates text entry for assistant with string content", () => {
    const line = JSON.stringify({ role: "assistant", content: "world" })
    const entries = run([line])
    expect(entries).toHaveLength(1)
    expect(entries[0]).toEqual({ kind: "text", text: "world" })
  })

  it("merges consecutive assistant text into one entry", () => {
    const l1 = JSON.stringify({ role: "assistant", content: "first" })
    const l2 = JSON.stringify({ role: "assistant", content: "second" })
    const entries = run([l1, l2])
    expect(entries).toHaveLength(1)
    expect(entries[0]).toEqual({ kind: "text", text: "first\n\nsecond" })
  })

  // --- role=assistant array content ---

  it("creates text entry for assistant with array content (text block)", () => {
    const line = JSON.stringify({
      role: "assistant",
      content: [{ type: "text", text: "array answer" }],
    })
    const entries = run([line])
    expect(entries).toHaveLength(1)
    expect(entries[0]).toEqual({ kind: "text", text: "array answer" })
  })

  it("skips non-text blocks in array content", () => {
    const line = JSON.stringify({
      role: "assistant",
      content: [{ type: "tool_use", id: "tc_1" }, { type: "text", text: "after tool" }],
    })
    const entries = run([line])
    expect(entries).toHaveLength(1)
    expect(entries[0]).toEqual({ kind: "text", text: "after tool" })
  })

  // --- tool_calls ---

  it("creates in-progress tool entry for each tool_call", () => {
    const line = JSON.stringify({
      role: "assistant",
      content: "calling",
      tool_calls: [
        { type: "function", id: "tc_1", function: { name: "Shell", arguments: '{"command":"ls"}' } },
      ],
    })
    const entries = run([line])
    // text entry + 1 tool entry
    expect(entries).toHaveLength(2)
    expect(entries[0]).toEqual({ kind: "text", text: "calling" })
    expect(entries[1]).toMatchObject({ kind: "tool", id: "tc_1", name: "Shell", result: undefined })
    expect((entries[1] as { kind: "tool"; input: unknown }).input).toEqual({ command: "ls" })
  })

  it("parses tool_call input as JSON when possible", () => {
    const line = JSON.stringify({
      role: "assistant",
      content: "",
      tool_calls: [
        { type: "function", id: "tc_2", function: { name: "Read", arguments: '{"path":"/tmp/x"}' } },
      ],
    })
    const entries = run([line])
    const tool = entries.find((e) => e.kind === "tool") as Extract<ParsedEntry, { kind: "tool" }>
    expect(tool.input).toEqual({ path: "/tmp/x" })
  })

  it("uses raw string input when tool_call arguments are not valid JSON", () => {
    const line = JSON.stringify({
      role: "assistant",
      content: "",
      tool_calls: [
        { type: "function", id: "tc_3", function: { name: "Shell", arguments: "not-json" } },
      ],
    })
    const entries = run([line])
    const tool = entries.find((e) => e.kind === "tool") as Extract<ParsedEntry, { kind: "tool" }>
    expect(tool.input).toBe("not-json")
  })

  // --- role=tool (tool results) ---

  it("updates tool entry with result when role=tool arrives", () => {
    const assistantLine = JSON.stringify({
      role: "assistant",
      content: "doing it",
      tool_calls: [
        { type: "function", id: "tc_1", function: { name: "Shell", arguments: '{"command":"ls"}' } },
      ],
    })
    const toolLine = JSON.stringify({ role: "tool", tool_call_id: "tc_1", content: "file1\nfile2" })
    const entries = run([assistantLine, toolLine])
    const tool = entries.find((e) => e.kind === "tool") as Extract<ParsedEntry, { kind: "tool" }>
    expect(tool.result).toBe("file1\nfile2")
  })

  it("ignores role=tool when tool_call_id not in itemMap", () => {
    const toolLine = JSON.stringify({ role: "tool", tool_call_id: "unknown", content: "result" })
    expect(run([toolLine])).toHaveLength(0)
  })

  // --- non-JSON / unknown ---

  it("emits raw entry for non-JSON lines", () => {
    const entries = run(["not json at all"])
    expect(entries).toHaveLength(1)
    expect(entries[0]).toMatchObject({ kind: "raw", content: "not json at all" })
  })

  it("emits raw entry for stderr lines", () => {
    const line = JSON.stringify({ role: "assistant", content: "x" })
    const entries = run([line], "stderr")
    expect(entries).toHaveLength(1)
    expect(entries[0]).toMatchObject({ kind: "raw", logType: "stderr" })
  })
})
```

- [ ] **Step 2: Run tests to confirm they fail**

```bash
cd web && npx vitest run src/components/log-viewer/__tests__/kimi-parser.test.ts
```

Expected: FAIL — `KimiParser` not found.

- [ ] **Step 3: Write the KimiParser implementation**

Create `web/src/components/log-viewer/kimi-parser.ts`:

```typescript
import type { ParsedEntry, StreamParser } from "./types"
import { appendRawEntry, appendTextEntry } from "./types"

interface KimiContentBlock {
  type: string
  text?: string
}

interface KimiToolCall {
  type: string
  id: string
  function: {
    name: string
    arguments: string
  }
}

interface KimiMessage {
  role: string
  content?: string | KimiContentBlock[]
  tool_call_id?: string
  tool_calls?: KimiToolCall[]
}

function parseToolInput(args: string): unknown {
  try {
    return JSON.parse(args)
  } catch {
    return args
  }
}

export class KimiParser implements StreamParser {
  parseLine(
    line: string,
    logType: string,
    entries: ParsedEntry[],
    itemMap: Map<string, number>
  ): void {
    if (logType !== "stdout") {
      appendRawEntry(line, logType, entries)
      return
    }

    let msg: KimiMessage
    try {
      const parsed = JSON.parse(line)
      if (!parsed || typeof parsed.role !== "string") {
        appendRawEntry(line, logType, entries)
        return
      }
      msg = parsed as KimiMessage
    } catch {
      appendRawEntry(line, logType, entries)
      return
    }

    switch (msg.role) {
      case "user":
        return

      case "assistant": {
        // Text content
        if (typeof msg.content === "string" && msg.content !== "") {
          appendTextEntry(msg.content, entries)
        } else if (Array.isArray(msg.content)) {
          for (const block of msg.content) {
            if (block.type === "text" && block.text?.trim()) {
              appendTextEntry(block.text, entries)
            }
          }
        }
        // Tool calls
        if (Array.isArray(msg.tool_calls)) {
          for (const tc of msg.tool_calls) {
            if (!tc.id || !tc.function?.name) continue
            const input = parseToolInput(tc.function.arguments ?? "")
            itemMap.set(tc.id, entries.length)
            entries.push({ kind: "tool", id: tc.id, name: tc.function.name, input, result: undefined })
          }
        }
        return
      }

      case "tool": {
        const { tool_call_id, content } = msg
        if (!tool_call_id) return
        const idx = itemMap.get(tool_call_id)
        if (idx === undefined) return
        const existing = entries[idx]
        if (existing?.kind !== "tool") return
        entries[idx] = { ...existing, result: typeof content === "string" ? content : "" }
        itemMap.delete(tool_call_id)
        return
      }

      default:
        appendRawEntry(line, logType, entries)
    }
  }
}
```

- [ ] **Step 4: Run tests to confirm they all pass**

```bash
cd web && npx vitest run src/components/log-viewer/__tests__/kimi-parser.test.ts
```

Expected: all tests PASS.

- [ ] **Step 5: Commit**

```bash
git add web/src/components/log-viewer/kimi-parser.ts \
        web/src/components/log-viewer/__tests__/kimi-parser.test.ts
git commit -m "feat(log-viewer): add KimiParser for role-based stream-JSON format"
```

---

### Task 9: Wire KimiParser into log-viewer.tsx

**Files:**
- Modify: `web/src/components/log-viewer.tsx:12-14,408-413`

- [ ] **Step 1: Add KimiParser import**

In `web/src/components/log-viewer.tsx`, add the import alongside the existing parsers:

```typescript
import { KimiParser } from "./log-viewer/kimi-parser"
```

(Place after the `PiParser` import on line 14.)

- [ ] **Step 2: Wire KimiParser in ensureParser**

In the `ensureParser` function (around line 408), change:

```typescript
parserRef.current =
  engine === "codex"
    ? new CodexParser()
    : engine === "pi"
      ? new PiParser()
      : new ClaudeParser()
```

to:

```typescript
parserRef.current =
  engine === "codex"
    ? new CodexParser()
    : engine === "pi"
      ? new PiParser()
      : engine === "kimi"
        ? new KimiParser()
        : new ClaudeParser()
```

- [ ] **Step 3: TypeScript check**

```bash
cd web && npx tsc --noEmit
```

Expected: exits 0, no type errors.

- [ ] **Step 4: Run full frontend test suite**

```bash
cd web && npx vitest run
```

Expected: all tests PASS.

- [ ] **Step 5: Commit**

```bash
git add web/src/components/log-viewer.tsx
git commit -m "feat(log-viewer): wire KimiParser into log viewer engine selection"
```

---

### Task 10: Final verification

- [ ] **Step 1: Run all Go tests**

```bash
go test ./...
```

Expected: all tests PASS.

- [ ] **Step 2: Run all frontend tests**

```bash
cd web && npx vitest run
```

Expected: all tests PASS.

- [ ] **Step 3: Build backend**

```bash
go build ./...
```

Expected: exits 0.

- [ ] **Step 4: Build frontend**

```bash
cd web && npm run build
```

Expected: exits 0, no errors.
