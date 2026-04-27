# Tokenstat Engine Cohesion Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Move per-engine token-usage extraction logic from `internal/tokenstat` into each engine package (`internal/ai/<engine>/`), and slim `internal/tokenstat/syncer.go` down to a thin orchestrator (scheduling, DB I/O, retry budget, tombstone, legacy fallback).

**Architecture:** Add a mandatory `CollectTokenUsage(ctx, sessionID) ([]TokenUsage, error)` method to the existing `ai.EngineAdapter` interface. Each engine adapter implements it by delegating to a per-engine collector that owns the engine's session-file discovery and JSONL parsing. The syncer accepts an injected `map[string]ai.EngineAdapter` and dispatches by engine name, falling back through a chain only when `bee_executions.engine` is empty (legacy data).

**Tech Stack:** Go, SQLite (`bee_token_stats` schema unchanged), existing zap logger, existing `internal/ai` registry pattern.

**Spec reference:** `docs/superpowers/specs/2026-04-26-tokenstat-engine-cohesion-design.md`

---

## File Structure

**New:**
- `internal/ai/types.go` — `TokenUsage` struct, `ErrSessionDataNotFound` sentinel.
- `internal/ai/sessionfile/sessionfile.go` — shared JSONL helpers (`ScanJSONLFile`, `FindWithLegacyFast`).
- `internal/ai/sessionfile/sessionfile_test.go`
- `internal/ai/claude/token_usage.go` + `_test.go`
- `internal/ai/codex/token_usage.go` + `_test.go`
- `internal/ai/pi/token_usage.go` + `_test.go`
- `internal/ai/kimi/token_usage.go` + `_test.go`

**Modified:**
- `internal/ai/engine.go` — add `CollectTokenUsage` to `EngineAdapter`.
- `internal/ai/{claude,codex,pi,kimi}/adapter.go` — implement `CollectTokenUsage` (delegates to per-package collector).
- `internal/tokenstat/syncer.go` — accept adapters, dispatch via map + fallback chain, drop parser registry.
- `internal/tokenstat/syncer_test.go` — use fake adapter implementing `EngineAdapter`.
- `internal/app/app.go` — pass `engines` map into `NewSyncer`.

**Deleted:**
- `internal/tokenstat/parser.go`
- `internal/tokenstat/session_files.go`
- `internal/tokenstat/claude.go` + `claude_test.go`
- `internal/tokenstat/codex.go` + `codex_test.go`
- `internal/tokenstat/pi.go` + `pi_test.go`
- `internal/tokenstat/kimi.go` + `kimi_test.go`

**Naming notes (must follow exactly):**
- `ai.TokenUsage` deliberately omits `SessionID` and `AgentType` fields. The syncer fills those in when writing to `bee_token_stats` — it knows which session it requested and which engine succeeded.
- Field names match the existing DB columns: `InputTokens`, `OutputTokens`, `CacheCreationTokens`, `CacheReadTokens`. (The spec mentioned `CacheWriteTokens` conceptually but the implementation must keep `CacheCreationTokens` to avoid touching the schema or `model.TokenStats`.)

---

### Task 1: Add `ai.TokenUsage` type and `ErrSessionDataNotFound`, plus stub `CollectTokenUsage` on every adapter

This task introduces the new types and extends the `EngineAdapter` interface, but each adapter gets a stub implementation returning `ErrSessionDataNotFound`. This keeps the codebase compiling while the real implementations are added one engine at a time in later tasks. The old `tokenstat` package is untouched and still works.

**Files:**
- Create: `internal/ai/types.go`
- Modify: `internal/ai/engine.go`
- Modify: `internal/ai/claude/adapter.go`
- Modify: `internal/ai/codex/adapter.go`
- Modify: `internal/ai/pi/adapter.go`
- Modify: `internal/ai/kimi/adapter.go`

- [ ] **Step 1.1: Create `internal/ai/types.go`**

```go
package ai

import "errors"

// TokenUsage represents one model's token consumption within a session,
// emitted by an engine's CollectTokenUsage method. The syncer fills in the
// session ID and engine (agent_type) when writing to bee_token_stats.
//
// Field names mirror bee_token_stats columns to keep the DB layer simple.
type TokenUsage struct {
	Model               string
	InputTokens         int64
	OutputTokens        int64
	CacheCreationTokens int64
	CacheReadTokens     int64
}

// ErrSessionDataNotFound signals that the engine could not yet locate session
// data (file not flushed, mapping missing, etc.). The syncer treats this as
// "retry within budget; tombstone if budget exhausted".
var ErrSessionDataNotFound = errors.New("ai: session data not found")
```

- [ ] **Step 1.2: Extend `EngineAdapter` in `internal/ai/engine.go`**

Replace the existing `EngineAdapter` block (lines 90-103) with:

```go
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

	// CollectTokenUsage extracts token usage for a completed session.
	// Returns ErrSessionDataNotFound if data is not yet on disk; the syncer
	// will retry within its budget and eventually tombstone. Returning
	// ([], nil) means "session located, verifiably no usage" — tombstoned
	// immediately. Other errors are treated as transient and retried.
	CollectTokenUsage(ctx context.Context, sessionID string) ([]TokenUsage, error)
}
```

- [ ] **Step 1.3: Stub `CollectTokenUsage` on `claudeAdapter`**

Append to `internal/ai/claude/adapter.go`:

```go
func (a *claudeAdapter) CollectTokenUsage(_ context.Context, _ string) ([]ai.TokenUsage, error) {
	return nil, ai.ErrSessionDataNotFound
}
```

- [ ] **Step 1.4: Stub `CollectTokenUsage` on `codexAdapter`**

Append to `internal/ai/codex/adapter.go`:

```go
func (a *codexAdapter) CollectTokenUsage(_ context.Context, _ string) ([]ai.TokenUsage, error) {
	return nil, ai.ErrSessionDataNotFound
}
```

- [ ] **Step 1.5: Stub `CollectTokenUsage` on `piAdapter`**

Append to `internal/ai/pi/adapter.go`:

```go
func (a *piAdapter) CollectTokenUsage(_ context.Context, _ string) ([]ai.TokenUsage, error) {
	return nil, ai.ErrSessionDataNotFound
}
```

- [ ] **Step 1.6: Stub `CollectTokenUsage` on `kimiAdapter`**

Append to `internal/ai/kimi/adapter.go`:

```go
func (a *kimiAdapter) CollectTokenUsage(_ context.Context, _ string) ([]ai.TokenUsage, error) {
	return nil, ai.ErrSessionDataNotFound
}
```

- [ ] **Step 1.7: Verify the build**

Run: `go build ./...`
Expected: success, no errors. The old tokenstat parsers still work; new method is stubbed.

- [ ] **Step 1.8: Run existing tests as a regression check**

Run: `go test ./...`
Expected: all tests pass (no behavior change yet).

- [ ] **Step 1.9: Commit**

```bash
git add internal/ai/types.go internal/ai/engine.go \
        internal/ai/claude/adapter.go internal/ai/codex/adapter.go \
        internal/ai/pi/adapter.go internal/ai/kimi/adapter.go
git commit -m "feat(ai): add TokenUsage type and CollectTokenUsage interface method (stubs)"
```

---

### Task 2: Create `internal/ai/sessionfile/` shared helper package

The current parsers share two helpers in `internal/tokenstat/session_files.go`: `scanJSONLFile` and `findWithLegacyFast`. Move them into a new package so per-engine collectors can import them without depending on tokenstat. Keep the `getOrCreate` and `mapValues` helpers private to each engine collector (they're trivially small and engine-specific data lives in them).

**Files:**
- Create: `internal/ai/sessionfile/sessionfile.go`
- Create: `internal/ai/sessionfile/sessionfile_test.go`

- [ ] **Step 2.1: Write failing test `internal/ai/sessionfile/sessionfile_test.go`**

```go
package sessionfile_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	ai "github.com/theopenbee/openbee/internal/ai"
	"github.com/theopenbee/openbee/internal/ai/sessionfile"
)

func TestScanJSONLFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.jsonl")
	content := "line one\nline two\nline three\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	var lines []string
	if err := sessionfile.ScanJSONLFile(path, func(b []byte) {
		lines = append(lines, string(b))
	}); err != nil {
		t.Fatalf("scan: %v", err)
	}
	want := []string{"line one", "line two", "line three"}
	if len(lines) != len(want) {
		t.Fatalf("got %d lines, want %d", len(lines), len(want))
	}
	for i := range want {
		if lines[i] != want[i] {
			t.Errorf("line %d = %q, want %q", i, lines[i], want[i])
		}
	}
}

func TestFindWithLegacyFast_LegacyHit(t *testing.T) {
	dir := t.TempDir()
	legacy := filepath.Join(dir, "abc.jsonl")
	if err := os.WriteFile(legacy, []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := sessionfile.FindWithLegacyFast(dir, "abc.jsonl", func(_ string, d os.DirEntry) bool {
		return d.Name() == "abc.jsonl"
	})
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if got != legacy {
		t.Errorf("got %q, want %q", got, legacy)
	}
}

func TestFindWithLegacyFast_NestedHit(t *testing.T) {
	dir := t.TempDir()
	nested := filepath.Join(dir, "sub", "sess-42.jsonl")
	if err := os.MkdirAll(filepath.Dir(nested), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(nested, []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := sessionfile.FindWithLegacyFast(dir, "sess-42.jsonl", func(_ string, d os.DirEntry) bool {
		return d.Name() == "sess-42.jsonl"
	})
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if got != nested {
		t.Errorf("got %q, want %q", got, nested)
	}
}

func TestFindWithLegacyFast_NotFound(t *testing.T) {
	dir := t.TempDir()
	_, err := sessionfile.FindWithLegacyFast(dir, "missing.jsonl", func(_ string, _ os.DirEntry) bool { return false })
	if !errors.Is(err, ai.ErrSessionDataNotFound) {
		t.Errorf("got err %v, want wraps ErrSessionDataNotFound", err)
	}
}
```

- [ ] **Step 2.2: Run test, verify it fails**

Run: `go test ./internal/ai/sessionfile/...`
Expected: FAIL — package `sessionfile` not found.

- [ ] **Step 2.3: Create `internal/ai/sessionfile/sessionfile.go`**

```go
// Package sessionfile contains shared file discovery and JSONL scanning
// helpers used by per-engine token-usage collectors.
package sessionfile

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	ai "github.com/theopenbee/openbee/internal/ai"
)

// 16 MiB: session files can embed base64-encoded content or long conversation turns.
const scannerBufSize = 16 * 1024 * 1024

var errStopWalk = errors.New("stop walk")

// ScanJSONLFile streams `path` line by line, invoking `fn` with a copy of each
// line's bytes. Empty lines are still passed through.
func ScanJSONLFile(path string, fn func([]byte)) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(nil, scannerBufSize)
	for scanner.Scan() {
		fn(append([]byte(nil), scanner.Bytes()...))
	}
	return scanner.Err()
}

// FindWithLegacyFast first checks for a flat-layout file at dir/legacyName
// (the old session layout). If absent, it walks dir recursively and returns
// the first file for which match returns true.
//
// Returns ai.ErrSessionDataNotFound (wrapped) when nothing matches or when
// the directory does not exist.
func FindWithLegacyFast(dir, legacyName string, match func(string, fs.DirEntry) bool) (string, error) {
	legacyPath := filepath.Join(dir, legacyName)
	if _, err := os.Stat(legacyPath); err == nil {
		return legacyPath, nil
	}
	return findSessionFile(dir, match)
}

func findSessionFile(root string, match func(path string, d fs.DirEntry) bool) (string, error) {
	var found string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if match(path, d) {
			found = path
			return errStopWalk
		}
		return nil
	})
	if err != nil && !errors.Is(err, errStopWalk) {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("%w: %s", ai.ErrSessionDataNotFound, root)
		}
		return "", fmt.Errorf("walk session root %s: %w", root, err)
	}
	if found == "" {
		return "", fmt.Errorf("%w: %s", ai.ErrSessionDataNotFound, root)
	}
	return found, nil
}
```

- [ ] **Step 2.4: Run test to verify it passes**

Run: `go test ./internal/ai/sessionfile/...`
Expected: PASS.

- [ ] **Step 2.5: Verify full build**

Run: `go build ./...`
Expected: success.

- [ ] **Step 2.6: Commit**

```bash
git add internal/ai/sessionfile/
git commit -m "feat(ai): add sessionfile package with shared JSONL helpers"
```

---

### Task 3: Implement Claude collector

Move the Claude parser logic into `internal/ai/claude/token_usage.go` and replace the stub on `claudeAdapter`. Migrate the existing `internal/tokenstat/claude_test.go` to live alongside the new collector. Old `tokenstat/claude.go` stays in place this task — it will be deleted in Task 9 along with the rest.

**Files:**
- Create: `internal/ai/claude/token_usage.go`
- Create: `internal/ai/claude/token_usage_test.go`
- Modify: `internal/ai/claude/adapter.go` (replace stub from Task 1)

- [ ] **Step 3.1: Migrate test to `internal/ai/claude/token_usage_test.go`**

Read `internal/tokenstat/claude_test.go` and copy it to `internal/ai/claude/token_usage_test.go` with these adjustments:
- Change package to `package claude_test`
- Replace import of `tokenstat` with `claude` (`github.com/theopenbee/openbee/internal/ai/claude`)
- Replace import of `ai` with the `ai` package for `TokenUsage` / `ErrSessionDataNotFound`
- Call `claude.NewCollector()` (or whatever helper is exposed in Step 3.2) instead of `tokenstat.NewClaudeParser()`
- Compare against `ai.TokenUsage` (no SessionID/AgentType fields — the test setup helper assigns model/token fields only)
- Expected errors: `errors.Is(err, ai.ErrSessionDataNotFound)`

The exact source of `internal/tokenstat/claude_test.go` (full read it before adapting) provides the test fixtures (JSONL contents, expected token counts). Do not invent new fixtures — preserve test value.

Pseudocode of what the adapted test looks like:

```go
package claude_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	ai "github.com/theopenbee/openbee/internal/ai"
	"github.com/theopenbee/openbee/internal/ai/claude"
)

func TestClaudeCollector_Parse(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", dir)
	// ...setup .claude/projects/<sub>/<sessionID>.jsonl with fixture content
	collector := claude.NewCollector()
	usages, err := collector.Collect(context.Background(), "<sessionID>")
	// ...assertions on usages
}

// Adapt remaining cases (fast model, synthetic filter, not-found) the same way.
```

- [ ] **Step 3.2: Run test, verify it fails**

Run: `go test ./internal/ai/claude/... -run TestClaude`
Expected: FAIL — `claude.NewCollector` undefined.

- [ ] **Step 3.3: Create `internal/ai/claude/token_usage.go`**

Translate the logic from `internal/tokenstat/claude.go` into the new package. Key edits:
- Change package to `claude`.
- Replace `tokenstat.SessionTokenUsage` with `ai.TokenUsage`. Drop `SessionID` and `AgentType` fields (engine name is implicit).
- Replace `scanJSONLFile` with `sessionfile.ScanJSONLFile`.
- Replace `findWithLegacyFast` with `sessionfile.FindWithLegacyFast`.
- Replace `tokenstat.ErrSessionDataNotFound` with `ai.ErrSessionDataNotFound`.
- The aggregator helpers (`getOrCreate`, `mapValues`) are inlined as private helpers in this file, keyed by model only.

Concrete code:

```go
package claude

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	ai "github.com/theopenbee/openbee/internal/ai"
	"github.com/theopenbee/openbee/internal/ai/sessionfile"
	"github.com/theopenbee/openbee/internal/infra/utils"
)

const syntheticModel = "<synthetic>"

// Collector extracts token usage from Claude JSONL session files.
type Collector struct {
	baseDirs []string
}

// NewCollector builds a Collector using CLAUDE_CONFIG_DIR (colon-separated)
// or the standard ~/.claude and ~/.config/claude locations.
func NewCollector() *Collector {
	return &Collector{baseDirs: claudeBaseDirs()}
}

func claudeBaseDirs() []string {
	if env := os.Getenv("CLAUDE_CONFIG_DIR"); env != "" {
		return utils.SplitAndTrim(env)
	}
	home, _ := os.UserHomeDir()
	return []string{
		filepath.Join(home, ".claude"),
		filepath.Join(home, ".config", "claude"),
	}
}

type claudeJSONLLine struct {
	Message struct {
		Model string `json:"model"`
		Speed string `json:"speed"`
		Usage *struct {
			InputTokens              int64 `json:"input_tokens"`
			OutputTokens             int64 `json:"output_tokens"`
			CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
			CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
		} `json:"usage"`
	} `json:"message"`
}

// Collect implements the per-engine collection contract.
func (c *Collector) Collect(_ context.Context, sessionID string) ([]ai.TokenUsage, error) {
	name := sessionID + ".jsonl"
	for _, base := range c.baseDirs {
		path, err := sessionfile.FindWithLegacyFast(
			filepath.Join(base, "projects"),
			name,
			func(_ string, d os.DirEntry) bool { return d.Name() == name },
		)
		if err == nil {
			return parseClaudeFile(path)
		}
		if !errors.Is(err, ai.ErrSessionDataNotFound) {
			return nil, err
		}
	}
	return nil, fmt.Errorf("%w: claude session file not found for %s", ai.ErrSessionDataNotFound, sessionID)
}

func parseClaudeFile(path string) ([]ai.TokenUsage, error) {
	agg := map[string]*ai.TokenUsage{}
	err := sessionfile.ScanJSONLFile(path, func(data []byte) {
		var line claudeJSONLLine
		if err := json.Unmarshal(data, &line); err != nil {
			return
		}
		m := line.Message.Model
		if m == "" || m == syntheticModel || line.Message.Usage == nil {
			return
		}
		if line.Message.Speed == "fast" {
			m += "-fast"
		}
		u, ok := agg[m]
		if !ok {
			u = &ai.TokenUsage{Model: m}
			agg[m] = u
		}
		u.InputTokens += line.Message.Usage.InputTokens
		u.OutputTokens += line.Message.Usage.OutputTokens
		u.CacheCreationTokens += line.Message.Usage.CacheCreationInputTokens
		u.CacheReadTokens += line.Message.Usage.CacheReadInputTokens
	})
	if err != nil {
		return nil, fmt.Errorf("scan claude session file: %w", err)
	}
	out := make([]ai.TokenUsage, 0, len(agg))
	for _, u := range agg {
		out = append(out, *u)
	}
	return out, nil
}
```

- [ ] **Step 3.4: Replace the `CollectTokenUsage` stub on `claudeAdapter`**

Edit `internal/ai/claude/adapter.go`. Add a `collector` field to `claudeAdapter`, initialize it in `NewAdapter`, and replace the stub.

- Add field to struct:

```go
type claudeAdapter struct {
	invoker   *Invoker
	collector *Collector
}
```

- Update `NewAdapter`:

```go
func NewAdapter(binaryPath, openbeeURL string, extraEnv map[string]string) ai.EngineAdapter {
	return &claudeAdapter{
		invoker:   NewInvoker(binaryPath, openbeeURL, extraEnv),
		collector: NewCollector(),
	}
}
```

- Replace the stub method (added in Task 1.3) with:

```go
func (a *claudeAdapter) CollectTokenUsage(ctx context.Context, sessionID string) ([]ai.TokenUsage, error) {
	return a.collector.Collect(ctx, sessionID)
}
```

- [ ] **Step 3.5: Run tests, verify they pass**

Run: `go test ./internal/ai/claude/...`
Expected: PASS.

- [ ] **Step 3.6: Verify full build and full test suite still pass**

Run: `go build ./... && go test ./...`
Expected: PASS. (The old `tokenstat/claude.go` is still in place and still used by the old syncer — both code paths coexist.)

- [ ] **Step 3.7: Commit**

```bash
git add internal/ai/claude/
git commit -m "feat(ai/claude): implement CollectTokenUsage"
```

---

### Task 4: Implement Codex collector

Mirror Task 3 for codex. The codex collector uses `SessionStore` (already in package `codex`) directly, eliminating the cross-package access tokenstat had via `config.DefaultCodexSessionsDir()`.

**Files:**
- Create: `internal/ai/codex/token_usage.go`
- Create: `internal/ai/codex/token_usage_test.go`
- Modify: `internal/ai/codex/adapter.go`

- [ ] **Step 4.1: Migrate test to `internal/ai/codex/token_usage_test.go`**

Read `internal/tokenstat/codex_test.go`. Adapt the same way Task 3.1 adapted the claude test:
- Package `codex_test`.
- Constructor becomes `codex.NewCollectorAt(mappingDir, codexBase)` (a test seam that Step 4.3 introduces, paralleling the existing test seam shape).
- Compare against `ai.TokenUsage`.
- Sentinel: `ai.ErrSessionDataNotFound`.

Preserve all four existing test cases verbatim (JSONL fixtures, mapping file contents, expected aggregated counts, legacy fallback case, missing-mapping case).

- [ ] **Step 4.2: Run test, verify it fails**

Run: `go test ./internal/ai/codex/... -run TestCodex`
Expected: FAIL — `codex.NewCollectorAt` undefined.

- [ ] **Step 4.3: Create `internal/ai/codex/token_usage.go`**

Translate `internal/tokenstat/codex.go` with the same field/import substitutions as Task 3.3. The `mappingDir` is the same directory the in-package `SessionStore` uses (`config.DefaultCodexSessionsDir()`), so the collector reads the openbee-UUID → codex-thread-ID mapping files written by `SessionStore.Set`.

```go
package codex

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	ai "github.com/theopenbee/openbee/internal/ai"
	"github.com/theopenbee/openbee/internal/ai/sessionfile"
	"github.com/theopenbee/openbee/internal/infra/config"
)

// Collector extracts token usage from codex JSONL session files. It reads the
// openbee-UUID → codex-thread-ID mapping written by SessionStore (same dir),
// then locates the codex session JSONL under codexBase/sessions/.
type Collector struct {
	mappingDir string
	codexBase  string
}

// NewCollector builds a Collector using the default mapping directory and
// CODEX_HOME (or ~/.codex if unset).
func NewCollector() *Collector {
	codexBase := os.Getenv("CODEX_HOME")
	if codexBase == "" {
		home, _ := os.UserHomeDir()
		codexBase = filepath.Join(home, ".codex")
	}
	return NewCollectorAt(config.DefaultCodexSessionsDir(), codexBase)
}

// NewCollectorAt is a test seam allowing arbitrary mapping/codex roots.
func NewCollectorAt(mappingDir, codexBase string) *Collector {
	return &Collector{mappingDir: mappingDir, codexBase: codexBase}
}

type codexJSONLLine struct {
	Type    string `json:"type"`
	Payload struct {
		Type  string          `json:"type"`
		Model string          `json:"model"`
		Info  *codexTokenInfo `json:"info"`
	} `json:"payload"`
	Info *codexTokenInfo `json:"info"`
}

type codexTokenInfo struct {
	Model     string `json:"model"`
	ModelName string `json:"model_name"`
	Metadata  struct {
		Model string `json:"model"`
	} `json:"metadata"`
	LastTokenUsage  *codexTokenUsage `json:"last_token_usage"`
	TotalTokenUsage *codexTokenUsage `json:"total_token_usage"`
}

type codexTokenUsage struct {
	InputTokens       int64 `json:"input_tokens"`
	OutputTokens      int64 `json:"output_tokens"`
	CachedInputTokens int64 `json:"cached_input_tokens"`
}

func (t *codexTokenUsage) advance(usage codexTokenUsage) {
	t.InputTokens += usage.InputTokens
	t.OutputTokens += usage.OutputTokens
	t.CachedInputTokens += usage.CachedInputTokens
}

func (t *codexTokenUsage) deltaAndSet(total codexTokenUsage) codexTokenUsage {
	delta := codexTokenUsage{
		InputTokens:       total.InputTokens - t.InputTokens,
		OutputTokens:      total.OutputTokens - t.OutputTokens,
		CachedInputTokens: total.CachedInputTokens - t.CachedInputTokens,
	}
	*t = total
	return delta
}

func (l codexJSONLLine) tokenInfo() *codexTokenInfo {
	if l.Payload.Info != nil {
		return l.Payload.Info
	}
	return l.Info
}

// Collect implements the per-engine collection contract.
func (c *Collector) Collect(_ context.Context, sessionID string) ([]ai.TokenUsage, error) {
	data, err := os.ReadFile(filepath.Join(c.mappingDir, sessionID))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: read codex mapping for session %s", ai.ErrSessionDataNotFound, sessionID)
		}
		return nil, fmt.Errorf("read codex mapping for session %s: %w", sessionID, err)
	}
	codexSessionID := strings.TrimSpace(string(data))
	if codexSessionID == "" {
		return nil, fmt.Errorf("empty codex session id in mapping for %s", sessionID)
	}
	path, err := findCodexSessionFile(c.codexBase, codexSessionID)
	if err != nil {
		if errors.Is(err, ai.ErrSessionDataNotFound) {
			return nil, fmt.Errorf("%w: codex session file not found for %s", ai.ErrSessionDataNotFound, codexSessionID)
		}
		return nil, err
	}
	return parseCodexFile(path)
}

func findCodexSessionFile(codexBase, sessionID string) (string, error) {
	return sessionfile.FindWithLegacyFast(
		filepath.Join(codexBase, "sessions"),
		sessionID+".jsonl",
		func(_ string, d os.DirEntry) bool {
			return strings.HasSuffix(d.Name(), ".jsonl") && strings.Contains(d.Name(), sessionID)
		},
	)
}

func parseCodexFile(path string) ([]ai.TokenUsage, error) {
	agg := map[string]*ai.TokenUsage{}
	prevByModel := map[string]*codexTokenUsage{}
	currentModel := ""
	err := sessionfile.ScanJSONLFile(path, func(data []byte) {
		var line codexJSONLLine
		if err := json.Unmarshal(data, &line); err != nil {
			return
		}
		switch line.Type {
		case "turn_context":
			if line.Payload.Model != "" {
				currentModel = line.Payload.Model
			}
		case "event_msg":
			if line.Payload.Type != "" && line.Payload.Type != "token_count" {
				return
			}
			info := line.tokenInfo()
			if info == nil {
				return
			}
			m := codexResolveModel(info, currentModel)
			if m == "" {
				return
			}
			u, ok := agg[m]
			if !ok {
				u = &ai.TokenUsage{Model: m}
				agg[m] = u
			}
			if prevByModel[m] == nil {
				prevByModel[m] = &codexTokenUsage{}
			}
			prev := prevByModel[m]
			if info.LastTokenUsage != nil {
				addCodexUsage(u, *info.LastTokenUsage)
				if info.TotalTokenUsage != nil {
					// Codex emits both fields together when a turn is replayed/resumed;
					// the cumulative total is authoritative, so reset prev instead of
					// double-counting by advancing it.
					*prev = *info.TotalTokenUsage
				} else {
					prev.advance(*info.LastTokenUsage)
				}
			} else if info.TotalTokenUsage != nil {
				addCodexUsage(u, prev.deltaAndSet(*info.TotalTokenUsage))
			}
		}
	})
	if err != nil {
		return nil, fmt.Errorf("scan codex session file: %w", err)
	}
	out := make([]ai.TokenUsage, 0, len(agg))
	for _, u := range agg {
		out = append(out, *u)
	}
	return out, nil
}

func addCodexUsage(dst *ai.TokenUsage, usage codexTokenUsage) {
	dst.InputTokens += usage.InputTokens
	dst.OutputTokens += usage.OutputTokens
	dst.CacheReadTokens += usage.CachedInputTokens
}

func codexResolveModel(info *codexTokenInfo, currentModel string) string {
	if info.Model != "" {
		return info.Model
	}
	if info.ModelName != "" {
		return info.ModelName
	}
	if info.Metadata.Model != "" {
		return info.Metadata.Model
	}
	return currentModel
}
```

- [ ] **Step 4.4: Replace the stub on `codexAdapter`**

Edit `internal/ai/codex/adapter.go`:

- Add field:

```go
type codexAdapter struct {
	invoker   *Invoker
	collector *Collector
}
```

- Update `NewAdapter`:

```go
func NewAdapter(binaryPath, openbeeURL string, extraEnv map[string]string) (ai.EngineAdapter, error) {
	store, err := NewSessionStore()
	if err != nil {
		return nil, fmt.Errorf("init codex session store: %w", err)
	}
	return &codexAdapter{
		invoker:   NewInvoker(binaryPath, openbeeURL, store, extraEnv),
		collector: NewCollector(),
	}, nil
}
```

- Replace the stub:

```go
func (a *codexAdapter) CollectTokenUsage(ctx context.Context, sessionID string) ([]ai.TokenUsage, error) {
	return a.collector.Collect(ctx, sessionID)
}
```

- [ ] **Step 4.5: Run tests, verify they pass**

Run: `go test ./internal/ai/codex/...`
Expected: PASS.

- [ ] **Step 4.6: Full build + test**

Run: `go build ./... && go test ./...`
Expected: PASS.

- [ ] **Step 4.7: Commit**

```bash
git add internal/ai/codex/
git commit -m "feat(ai/codex): implement CollectTokenUsage"
```

---

### Task 5: Implement Pi collector

Mirror Tasks 3-4 for pi.

**Files:**
- Create: `internal/ai/pi/token_usage.go`
- Create: `internal/ai/pi/token_usage_test.go`
- Modify: `internal/ai/pi/adapter.go`

- [ ] **Step 5.1: Migrate test to `internal/ai/pi/token_usage_test.go`**

Read `internal/tokenstat/pi_test.go`. Adapt with the same recipe:
- Package `pi_test`, import `pi` and `ai`.
- Constructor: `pi.NewCollector()` (uses `PI_AGENT_DIR` env or `config.DefaultPiSessionsDir()`).
- `t.Setenv("PI_AGENT_DIR", dir)` to redirect lookups in tests.
- Compare against `ai.TokenUsage`; expected error wraps `ai.ErrSessionDataNotFound`.

- [ ] **Step 5.2: Run test, verify it fails**

Run: `go test ./internal/ai/pi/... -run TestPi`
Expected: FAIL — `pi.NewCollector` undefined.

- [ ] **Step 5.3: Create `internal/ai/pi/token_usage.go`**

```go
package pi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	ai "github.com/theopenbee/openbee/internal/ai"
	"github.com/theopenbee/openbee/internal/ai/sessionfile"
	"github.com/theopenbee/openbee/internal/infra/config"
)

// Collector extracts token usage from pi JSONL session files.
type Collector struct {
	sessionsDir string
}

// NewCollector builds a Collector using PI_AGENT_DIR or the config default.
func NewCollector() *Collector {
	dir := os.Getenv("PI_AGENT_DIR")
	if dir == "" {
		dir = config.DefaultPiSessionsDir()
	}
	return &Collector{sessionsDir: dir}
}

type piJSONLLine struct {
	Type    string `json:"type"`
	Message struct {
		Role  string `json:"role"`
		Model string `json:"model"`
		Usage *struct {
			Input      int64 `json:"input"`
			Output     int64 `json:"output"`
			CacheWrite int64 `json:"cacheWrite"`
			CacheRead  int64 `json:"cacheRead"`
		} `json:"usage"`
	} `json:"message"`
}

// Collect implements the per-engine collection contract.
func (c *Collector) Collect(_ context.Context, sessionID string) ([]ai.TokenUsage, error) {
	path, err := sessionfile.FindWithLegacyFast(c.sessionsDir, sessionID+".jsonl", func(_ string, d os.DirEntry) bool {
		return strings.HasSuffix(d.Name(), "_"+sessionID+".jsonl")
	})
	if err != nil {
		if errors.Is(err, ai.ErrSessionDataNotFound) {
			return nil, fmt.Errorf("%w: pi session file not found for %s", ai.ErrSessionDataNotFound, sessionID)
		}
		return nil, fmt.Errorf("pi session file lookup for %s: %w", sessionID, err)
	}
	return parsePiFile(path)
}

func parsePiFile(path string) ([]ai.TokenUsage, error) {
	agg := map[string]*ai.TokenUsage{}
	err := sessionfile.ScanJSONLFile(path, func(data []byte) {
		var line piJSONLLine
		if err := json.Unmarshal(data, &line); err != nil {
			return
		}
		if line.Type != "message" || line.Message.Role != "assistant" || line.Message.Usage == nil {
			return
		}
		m := line.Message.Model
		u, ok := agg[m]
		if !ok {
			u = &ai.TokenUsage{Model: m}
			agg[m] = u
		}
		u.InputTokens += line.Message.Usage.Input
		u.OutputTokens += line.Message.Usage.Output
		u.CacheCreationTokens += line.Message.Usage.CacheWrite
		u.CacheReadTokens += line.Message.Usage.CacheRead
	})
	if err != nil {
		return nil, fmt.Errorf("scan pi session file: %w", err)
	}
	out := make([]ai.TokenUsage, 0, len(agg))
	for _, u := range agg {
		out = append(out, *u)
	}
	return out, nil
}
```

- [ ] **Step 5.4: Replace the stub on `piAdapter`**

Edit `internal/ai/pi/adapter.go`:

```go
type piAdapter struct {
	invoker   *Invoker
	collector *Collector
}

func NewAdapter(binaryPath, openbeeURL string, extraEnv map[string]string) (ai.EngineAdapter, error) {
	inv, err := NewInvoker(binaryPath, openbeeURL, extraEnv)
	if err != nil {
		return nil, err
	}
	return &piAdapter{invoker: inv, collector: NewCollector()}, nil
}

func (a *piAdapter) CollectTokenUsage(ctx context.Context, sessionID string) ([]ai.TokenUsage, error) {
	return a.collector.Collect(ctx, sessionID)
}
```

- [ ] **Step 5.5: Run tests, verify they pass**

Run: `go test ./internal/ai/pi/...`
Expected: PASS.

- [ ] **Step 5.6: Full build + test**

Run: `go build ./... && go test ./...`
Expected: PASS.

- [ ] **Step 5.7: Commit**

```bash
git add internal/ai/pi/
git commit -m "feat(ai/pi): implement CollectTokenUsage"
```

---

### Task 6: Implement Kimi collector

Mirror the previous tasks for kimi.

**Files:**
- Create: `internal/ai/kimi/token_usage.go`
- Create: `internal/ai/kimi/token_usage_test.go`
- Modify: `internal/ai/kimi/adapter.go`

- [ ] **Step 6.1: Migrate test to `internal/ai/kimi/token_usage_test.go`**

Read `internal/tokenstat/kimi_test.go`. Same adaptation recipe:
- Package `kimi_test`, import `kimi` and `ai`.
- Constructor: `kimi.NewCollectorAt(dir)` (test seam) or `kimi.NewCollector()` for the default.
- Compare against `ai.TokenUsage` and `ai.ErrSessionDataNotFound`.

Kimi tests touch four cases (parse, not-found, no-StatusUpdate, all-zero); preserve all four.

- [ ] **Step 6.2: Run test, verify it fails**

Run: `go test ./internal/ai/kimi/... -run TestKimi`
Expected: FAIL — `kimi.NewCollector*` undefined.

- [ ] **Step 6.3: Create `internal/ai/kimi/token_usage.go`**

```go
package kimi

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	ai "github.com/theopenbee/openbee/internal/ai"
	"github.com/theopenbee/openbee/internal/ai/sessionfile"
	"github.com/theopenbee/openbee/internal/infra/config"
)

const kimiModel = "kimi"

// Collector extracts token usage from kimi wire.jsonl files.
type Collector struct {
	sessionsDir string
}

// NewCollector builds a Collector at the default sessions root.
func NewCollector() *Collector {
	return NewCollectorAt(config.DefaultKimiSessionsDir())
}

// NewCollectorAt is a test seam allowing arbitrary roots.
func NewCollectorAt(dir string) *Collector {
	return &Collector{sessionsDir: dir}
}

type kimiTokenUsage struct {
	InputOther         int64 `json:"input_other"`
	Output             int64 `json:"output"`
	InputCacheRead     int64 `json:"input_cache_read"`
	InputCacheCreation int64 `json:"input_cache_creation"`
}

type kimiJSONLLine struct {
	Message struct {
		Type    string `json:"type"`
		Payload struct {
			TokenUsage *kimiTokenUsage `json:"token_usage"`
		} `json:"payload"`
	} `json:"message"`
}

// Collect implements the per-engine collection contract.
func (c *Collector) Collect(_ context.Context, sessionID string) ([]ai.TokenUsage, error) {
	matches, err := filepath.Glob(filepath.Join(c.sessionsDir, "*", sessionID, "wire.jsonl"))
	if err != nil {
		return nil, fmt.Errorf("glob kimi session: %w", err)
	}
	if len(matches) == 0 {
		return nil, fmt.Errorf("%w: kimi session file not found for %s", ai.ErrSessionDataNotFound, sessionID)
	}
	return parseKimiFile(matches[0])
}

func parseKimiFile(path string) ([]ai.TokenUsage, error) {
	var last *kimiTokenUsage
	err := sessionfile.ScanJSONLFile(path, func(data []byte) {
		var line kimiJSONLLine
		if err := json.Unmarshal(data, &line); err != nil {
			return
		}
		if line.Message.Type != "StatusUpdate" || line.Message.Payload.TokenUsage == nil {
			return
		}
		last = line.Message.Payload.TokenUsage
	})
	if err != nil {
		return nil, fmt.Errorf("scan kimi session file: %w", err)
	}
	if last == nil {
		return nil, fmt.Errorf("%w: no StatusUpdate found in %s", ai.ErrSessionDataNotFound, path)
	}
	return []ai.TokenUsage{{
		Model:               kimiModel,
		InputTokens:         last.InputOther,
		OutputTokens:        last.Output,
		CacheReadTokens:     last.InputCacheRead,
		CacheCreationTokens: last.InputCacheCreation,
	}}, nil
}
```

- [ ] **Step 6.4: Replace the stub on `kimiAdapter`**

Edit `internal/ai/kimi/adapter.go`:

```go
type kimiAdapter struct {
	invoker   *Invoker
	collector *Collector
}

func NewAdapter(binaryPath, openbeeURL string, extraEnv map[string]string) ai.EngineAdapter {
	return &kimiAdapter{invoker: NewInvoker(binaryPath, openbeeURL, extraEnv), collector: NewCollector()}
}

func (a *kimiAdapter) CollectTokenUsage(ctx context.Context, sessionID string) ([]ai.TokenUsage, error) {
	return a.collector.Collect(ctx, sessionID)
}
```

- [ ] **Step 6.5: Run tests, verify they pass**

Run: `go test ./internal/ai/kimi/...`
Expected: PASS.

- [ ] **Step 6.6: Full build + test**

Run: `go build ./... && go test ./...`
Expected: PASS.

- [ ] **Step 6.7: Commit**

```bash
git add internal/ai/kimi/
git commit -m "feat(ai/kimi): implement CollectTokenUsage"
```

---

### Task 7: Refactor `tokenstat/syncer.go` to dispatch via `EngineAdapter`

Replace the parser registry with an adapter map and a fallback chain (used only for legacy sessions where `bee_executions.engine` is empty). Behavior is preserved 1:1 with today's syncer for known-engine sessions; the fallback chain is preserved 1:1 for empty-engine sessions.

**Files:**
- Modify: `internal/tokenstat/syncer.go`

The previously-extracted helpers `findWithLegacyFast`, `scanJSONLFile`, `getOrCreate`, `mapValues`, and `ErrSessionDataNotFound` are still defined in `internal/tokenstat/session_files.go` at this point — leave that file alone for this task; Task 9 deletes it. The new syncer references only `ai.ErrSessionDataNotFound`, so no cross-reference.

- [ ] **Step 7.1: Replace the contents of `internal/tokenstat/syncer.go`**

Full new file:

```go
package tokenstat

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	ai "github.com/theopenbee/openbee/internal/ai"
	"github.com/theopenbee/openbee/internal/infra/logger"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/infra/store"
	"go.uber.org/zap"
)

const syncInterval = 10 * time.Minute

// Syncer periodically reads completed sessions from bee_executions and asks
// the matching engine adapter to produce per-model token usage, then upserts
// into bee_token_stats. Engines whose bee_executions.engine field is empty
// (legacy data) are dispatched through a fixed fallback chain.
type Syncer struct {
	db         *sql.DB
	tokenStore *store.TokenStatsStore

	// adapters maps engine name → adapter. Always non-empty for non-legacy rows.
	adapters map[string]ai.EngineAdapter

	// fallbackOrder is the deterministic engine name order used when
	// dispatching a session whose engine field is empty. Each name must
	// appear in adapters; absent names are silently skipped.
	fallbackOrder []string

	// engines, collectSQL, engineArgs are precomputed for collectSessions.
	engines    []string
	collectSQL string
	engineArgs []any
}

// NewSyncer builds a Syncer that dispatches to the supplied adapters.
// fallbackOrder controls the legacy fallback chain — pass ai.AllEngines()
// to preserve the historical chain order.
func NewSyncer(db *sql.DB, tokenStore *store.TokenStatsStore, adapters map[string]ai.EngineAdapter, fallbackOrder []string) *Syncer {
	engines := ai.AllEngines()
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(engines)), ",")
	collectSQL := fmt.Sprintf(`
		SELECT e.session_id, COALESCE(MAX(NULLIF(e.engine, '')), '')
		FROM bee_executions e
		LEFT JOIN bee_token_stats ts ON ts.session_id = e.session_id
		WHERE (e.engine = '' OR e.engine IN (%s))
		GROUP BY e.session_id
		HAVING MAX(e.completed_at) > COALESCE(MAX(ts.synced_at), 0)
		LIMIT 500`, placeholders)
	engineArgs := make([]any, len(engines))
	for i, e := range engines {
		engineArgs[i] = e
	}
	return &Syncer{
		db:            db,
		tokenStore:    tokenStore,
		adapters:      adapters,
		fallbackOrder: fallbackOrder,
		engines:       engines,
		collectSQL:    collectSQL,
		engineArgs:    engineArgs,
	}
}

func (s *Syncer) Run(ctx context.Context) {
	logger.Info("tokenstat: sync loop started", zap.Duration("interval", syncInterval))
	s.SyncOnce(ctx)
	ticker := time.NewTicker(syncInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			logger.Info("tokenstat: sync loop stopped")
			return
		case <-ticker.C:
			s.SyncOnce(ctx)
		}
	}
}

func (s *Syncer) SyncOnce(ctx context.Context) {
	sessions, err := s.collectSessions(ctx)
	if err != nil {
		logger.Error("tokenstat: collect sessions", zap.Error(err))
		return
	}
	if len(sessions) == 0 {
		logger.Debug("tokenstat: no sessions pending sync")
		return
	}
	logger.Info("tokenstat: syncing sessions", zap.Int("count", len(sessions)))
	var synced, failed int
	for _, item := range sessions {
		if err := s.syncSession(ctx, item.sessionID, item.engine); err != nil {
			failed++
			logger.Warn("tokenstat: sync session failed",
				zap.String("session_id", item.sessionID),
				zap.String("engine", item.engine),
				zap.Error(err))
		} else {
			synced++
		}
	}
	logger.Info("tokenstat: sync round complete",
		zap.Int("synced", synced),
		zap.Int("failed", failed))
}

type sessionItem struct {
	sessionID string
	engine    string
}

func (s *Syncer) collectSessions(ctx context.Context) ([]sessionItem, error) {
	rows, err := s.db.QueryContext(ctx, s.collectSQL, s.engineArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []sessionItem
	for rows.Next() {
		var item sessionItem
		if err := rows.Scan(&item.sessionID, &item.engine); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// syncSession dispatches one session to the appropriate adapter.
//   - If item.engine is non-empty and registered: call that adapter once.
//   - If item.engine is empty OR not registered (legacy data, or an engine
//     that's no longer compiled in): walk fallbackOrder, advancing only
//     when the adapter reports ErrSessionDataNotFound.
//
// Treating unknown-engine the same as empty-engine matches the historical
// syncer's parserOrder behavior, which fell back to all parsers in either
// case.
//
// On any non-NotFound outcome (success, empty result, hard error), the
// session is considered handled for this round.
func (s *Syncer) syncSession(ctx context.Context, sessionID, engine string) error {
	if engine != "" {
		if adapter, ok := s.adapters[engine]; ok {
			return s.tryAdapter(ctx, sessionID, engine, adapter)
		}
	}

	// Empty or unknown engine → fallback chain.
	var sawNotFound bool
	for _, name := range s.fallbackOrder {
		adapter, ok := s.adapters[name]
		if !ok {
			continue
		}
		err := s.tryAdapter(ctx, sessionID, name, adapter)
		if err == nil || !errors.Is(err, ai.ErrSessionDataNotFound) {
			return err
		}
		sawNotFound = true
	}
	if sawNotFound {
		return s.tombstone(sessionID, "no adapter found data (legacy fallback)")
	}
	return s.tombstone(sessionID, "no adapters available")
}

// tryAdapter calls the adapter's CollectTokenUsage and persists the result.
//
//   - usages non-empty → upsert and return nil
//   - usages empty + err == nil → tombstone and return nil
//   - err == ErrSessionDataNotFound → propagate so the caller can fall through
//   - other err → propagate (the caller logs and counts as failed)
func (s *Syncer) tryAdapter(ctx context.Context, sessionID, engine string, adapter ai.EngineAdapter) error {
	usages, err := adapter.CollectTokenUsage(ctx, sessionID)
	if err != nil {
		if errors.Is(err, ai.ErrSessionDataNotFound) {
			logger.Debug("tokenstat: session data not found",
				zap.String("session_id", sessionID),
				zap.String("engine", engine))
			return err
		}
		return fmt.Errorf("%s collector: %w", engine, err)
	}
	if len(usages) == 0 {
		logger.Debug("tokenstat: session located but empty, writing tombstone",
			zap.String("session_id", sessionID),
			zap.String("engine", engine))
		return s.tombstone(sessionID, "empty usages")
	}
	if err := s.storeUsages(sessionID, engine, usages); err != nil {
		return fmt.Errorf("store usages: %w", err)
	}
	logger.Info("tokenstat: session synced",
		zap.String("session_id", sessionID),
		zap.String("engine", engine),
		zap.Int("models", len(usages)))
	return nil
}

func (s *Syncer) tombstone(sessionID, reason string) error {
	logger.Debug("tokenstat: tombstoning session",
		zap.String("session_id", sessionID),
		zap.String("reason", reason))
	return s.upsertRows(sessionID, "", []ai.TokenUsage{{Model: store.TombstoneModel}})
}

func (s *Syncer) storeUsages(sessionID, engine string, usages []ai.TokenUsage) error {
	return s.upsertRows(sessionID, engine, usages)
}

func (s *Syncer) upsertRows(sessionID, agentType string, usages []ai.TokenUsage) error {
	if len(usages) == 0 {
		return nil
	}
	now := time.Now().UnixMilli()
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck
	for _, u := range usages {
		if err := s.tokenStore.UpsertTx(tx, model.TokenStats{
			SessionID:           sessionID,
			AgentType:           agentType,
			Model:               u.Model,
			InputTokens:         u.InputTokens,
			OutputTokens:        u.OutputTokens,
			CacheCreationTokens: u.CacheCreationTokens,
			CacheReadTokens:     u.CacheReadTokens,
			TotalTokens:         u.InputTokens + u.OutputTokens + u.CacheCreationTokens + u.CacheReadTokens,
			SyncedAt:            now,
		}); err != nil {
			return fmt.Errorf("upsert: %w", err)
		}
	}
	return tx.Commit()
}
```

- [ ] **Step 7.2: Build will fail until callers are updated**

Run: `go build ./...`
Expected: FAIL — `internal/app/app.go:178` calls the old 2-arg `NewSyncer`. This is fixed in Task 8. Do not commit yet.

---

### Task 8: Wire the new syncer in `internal/app/app.go`

The `engines` map (`map[string]ai.EngineAdapter`) is already constructed at `app.go:107`; pass it (and the canonical fallback order) into `tokenstat.NewSyncer`.

**Files:**
- Modify: `internal/app/app.go`

- [ ] **Step 8.1: Update the syncer construction call at `internal/app/app.go:178`**

Replace this single line:

```go
tokenSyncer := tokenstat.NewSyncer(db, s.tokenStatsStore)
```

with:

```go
tokenSyncer := tokenstat.NewSyncer(db, s.tokenStatsStore, engines, ai.AllEngines())
```

The `engines` variable already exists in scope (declared at `app.go:107`). The `ai.AllEngines()` slice preserves today's fallback order (claude, codex, pi, kimi).

- [ ] **Step 8.2: Build the binary**

Run: `go build ./...`
Expected: PASS.

- [ ] **Step 8.3: Run the existing test suite (excluding tokenstat tests, which are about to be rewritten)**

Run: `go test $(go list ./... | grep -v internal/tokenstat)`
Expected: PASS.

The `internal/tokenstat` tests are expected to fail at this point because the old `NewSyncer` signature changed. Task 7's new code is exercised here, but `syncer_test.go` is rewritten in Task 9.

- [ ] **Step 8.4: Commit Tasks 7 + 8 together**

```bash
git add internal/tokenstat/syncer.go internal/app/app.go
git commit -m "refactor(tokenstat): dispatch via EngineAdapter, inject adapters from app"
```

---

### Task 9: Delete legacy parsers and rewrite syncer tests

Now that all four engines own their collectors and the syncer dispatches through the new interface, the old parser files and the `Parser` interface are dead code. Remove them and rewrite `syncer_test.go` to drive the new dispatch logic via a fake adapter.

**Files:**
- Delete: `internal/tokenstat/parser.go`
- Delete: `internal/tokenstat/session_files.go`
- Delete: `internal/tokenstat/claude.go`
- Delete: `internal/tokenstat/claude_test.go`
- Delete: `internal/tokenstat/codex.go`
- Delete: `internal/tokenstat/codex_test.go`
- Delete: `internal/tokenstat/pi.go`
- Delete: `internal/tokenstat/pi_test.go`
- Delete: `internal/tokenstat/kimi.go`
- Delete: `internal/tokenstat/kimi_test.go`
- Modify: `internal/tokenstat/syncer_test.go`

- [ ] **Step 9.1: Remove the dead parser files**

```bash
rm internal/tokenstat/parser.go \
   internal/tokenstat/session_files.go \
   internal/tokenstat/claude.go internal/tokenstat/claude_test.go \
   internal/tokenstat/codex.go internal/tokenstat/codex_test.go \
   internal/tokenstat/pi.go internal/tokenstat/pi_test.go \
   internal/tokenstat/kimi.go internal/tokenstat/kimi_test.go
```

- [ ] **Step 9.2: Confirm only the new files (and a now-broken syncer_test) remain**

Run: `ls internal/tokenstat/`
Expected output:
```
syncer.go
syncer_test.go
```

- [ ] **Step 9.3: Rewrite `internal/tokenstat/syncer_test.go`**

The existing test file (lines 1-355+) contains 9 cases that exercise the syncer end-to-end against real engine parsers. The rewrite drives the new dispatcher via a fake `ai.EngineAdapter`. Read the existing file first to preserve test names and intent — each existing scenario maps to a fake-adapter scenario:

- `TestSyncer_Direct_KnownEngine` — known engine succeeds, row written.
- `TestSyncer_Direct_KnownEngine_NotFound_Tombstones` — known engine returns `ErrSessionDataNotFound`, syncer tombstones.
- `TestSyncer_Direct_KnownEngine_Empty_Tombstones` — known engine returns `([], nil)`, syncer tombstones.
- `TestSyncer_Direct_KnownEngine_HardError_NoTombstone` — non-NotFound error, no row written, returned as failed.
- `TestSyncer_Legacy_FallbackHits` — engine empty, third adapter in chain succeeds.
- `TestSyncer_Legacy_AllNotFound_Tombstones` — engine empty, all adapters NotFound → tombstone.
- `TestSyncer_UnknownEngine_FallsBack` — engine present but not in adapter map → walks the fallback chain (matches historical behavior); add a sub-case where every fallback also returns NotFound and assert tombstone.
- `TestSyncer_DoesNotResyncCompleted` — session with synced_at > completed_at is skipped.

The fake adapter:

```go
type fakeAdapter struct {
	collect func(ctx context.Context, sessionID string) ([]ai.TokenUsage, error)
}

func (f *fakeAdapter) Prepare(string, ai.PrepareOptions) error { return nil }
func (f *fakeAdapter) Run(context.Context, string, string, ai.RunOptions, string) (ai.RunResult, error) {
	return ai.RunResult{}, nil
}
func (f *fakeAdapter) CollectTokenUsage(ctx context.Context, sessionID string) ([]ai.TokenUsage, error) {
	return f.collect(ctx, sessionID)
}
```

A small fixture helper to seed `bee_executions` and read back `bee_token_stats` (read the existing `syncer_test.go` for the exact SQL it uses — it sets up the schema and inserts rows; preserve those helpers verbatim, only swap construction of the syncer).

Construction in tests becomes:

```go
adapters := map[string]ai.EngineAdapter{
	ai.EngineClaude: &fakeAdapter{collect: func(_ context.Context, _ string) ([]ai.TokenUsage, error) {
		return []ai.TokenUsage{{Model: "sonnet-4", InputTokens: 100, OutputTokens: 50}}, nil
	}},
	// ...
}
syncer := tokenstat.NewSyncer(db, tokenStore, adapters, []string{ai.EngineClaude, ai.EngineCodex, ai.EnginePi, ai.EngineKimi})
syncer.SyncOnce(context.Background())

// then assert against bee_token_stats rows
```

For each test case:
1. Seed `bee_executions` with the relevant `(session_id, engine, completed_at)` row(s).
2. Build adapters whose `collect` closures return the fixture for that scenario.
3. Call `syncer.SyncOnce(ctx)`.
4. Query `bee_token_stats` and assert: row count, model values, agent_type column, tombstone presence.

Write each test case before its predecessor passes — this is TDD; do not write the entire file then implement. (The implementation is done; the failing-then-passing cycle here is on the test code itself versus the new syncer behavior.)

- [ ] **Step 9.4: Run the new syncer tests**

Run: `go test ./internal/tokenstat/...`
Expected: PASS — all rewritten cases.

- [ ] **Step 9.5: Run the full suite as a regression check**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 9.6: Verify the tokenstat package surface area**

Run: `go doc ./internal/tokenstat`
Expected: only `Syncer`, `NewSyncer`, `Run`, `SyncOnce` exposed. No `Parser`, no `SessionTokenUsage`, no `ErrSessionDataNotFound` (moved to `ai`).

- [ ] **Step 9.7: Commit**

```bash
git add internal/tokenstat/
git commit -m "refactor(tokenstat): drop per-engine parsers, drive syncer with EngineAdapter map"
```

---

### Task 10: Final verification

Sanity check the whole change end-to-end before considering this done.

- [ ] **Step 10.1: Lint / vet**

Run: `go vet ./...`
Expected: no warnings.

- [ ] **Step 10.2: Format check**

Run: `gofmt -l internal/ai/ internal/tokenstat/ internal/app/`
Expected: empty output.

- [ ] **Step 10.3: Full build + full test (final)**

Run: `go build ./... && go test ./...`
Expected: PASS.

- [ ] **Step 10.4: Inspect git log**

Run: `git log --oneline -8`
Expected: roughly seven new commits, in this order:
1. `feat(ai): add TokenUsage type and CollectTokenUsage interface method (stubs)`
2. `feat(ai): add sessionfile package with shared JSONL helpers`
3. `feat(ai/claude): implement CollectTokenUsage`
4. `feat(ai/codex): implement CollectTokenUsage`
5. `feat(ai/pi): implement CollectTokenUsage`
6. `feat(ai/kimi): implement CollectTokenUsage`
7. `refactor(tokenstat): dispatch via EngineAdapter, inject adapters from app`
8. `refactor(tokenstat): drop per-engine parsers, drive syncer with EngineAdapter map`

- [ ] **Step 10.5: Manual smoke test (optional but recommended)**

If a local dev environment with each engine binary installed is available:
- Run a Claude session, wait for the syncer round, query `SELECT * FROM bee_token_stats WHERE session_id = '<id>'` and confirm a row exists.
- Repeat for codex, pi, kimi.

If only one engine is available locally, smoke that one and rely on unit tests for the rest.
