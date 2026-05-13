# Collapse `RunOptions.ExtraArgs` to a single string — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Shrink the `ai` package's engine-args surface to one resolver + one validator, push CLI tokenisation into engine backends, and turn `RunOptions.ExtraArgs` from `[]string` into a single raw CLI line.

**Architecture:** Add `core.SplitArgs` and `ai.{ResolveExtraArgs, ValidateExtraArgs}` first (additive — tree stays green). Then atomically flip the `RunOptions.ExtraArgs` type and update all four engine backends plus both callers (bee, worker). Finally delete the legacy helpers (`EngineArgsMap`, `ParseEngineArgs`, `ParseEngineArgsJSON`, `MergeEngineArgs`) and their tests.

**Tech Stack:** Go 1.22+, standard library only (`encoding/json`, `strings`, `unicode`). Test runner: `go test`.

**Spec reference:** `docs/superpowers/specs/2026-05-12-extra-args-string-design.md`.

---

## Task 1: Add `core.SplitArgs` as the single tokeniser

**Files:**
- Create: `internal/ai/core/cli_args.go`
- Create: `internal/ai/core/cli_args_test.go`

The body is lifted from the existing private `splitCLIArgs` in `internal/ai/factory.go` (lines 240-314) and renamed. Engines will switch to call this in Task 3; the legacy `splitCLIArgs` in `factory.go` stays in place until Task 4 removes it. Co-existence is fine — they share no symbol name.

- [ ] **Step 1: Write the failing test file**

Create `internal/ai/core/cli_args_test.go`:

```go
package core_test

import (
	"slices"
	"strings"
	"testing"

	core "github.com/theopenbee/openbee/internal/ai/core"
)

func TestSplitArgs_PreservesOrderAndQuotedValues(t *testing.T) {
	got, err := core.SplitArgs(`--model claude-sonnet-4-5 --append-system-prompt "be terse" --verbose`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"--model", "claude-sonnet-4-5", "--append-system-prompt", "be terse", "--verbose"}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestSplitArgs_PreservesDuplicateFlags(t *testing.T) {
	got, err := core.SplitArgs(`--include src --include test`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"--include", "src", "--include", "test"}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestSplitArgs_PreservesEmptyQuotedValue(t *testing.T) {
	got, err := core.SplitArgs(`--append-system-prompt "" --verbose`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"--append-system-prompt", "", "--verbose"}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestSplitArgs_HandlesSingleQuotes(t *testing.T) {
	got, err := core.SplitArgs(`--msg 'hello world'`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"--msg", "hello world"}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestSplitArgs_HandlesBackslashEscape(t *testing.T) {
	got, err := core.SplitArgs(`a\ b c`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"a b", "c"}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestSplitArgs_EmptyStringReturnsNil(t *testing.T) {
	got, err := core.SplitArgs("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got %v, want empty", got)
	}
}

func TestSplitArgs_UnterminatedDoubleQuote(t *testing.T) {
	_, err := core.SplitArgs(`--model "unterminated`)
	if err == nil {
		t.Fatal("expected parse error, got nil")
	}
	if !strings.Contains(err.Error(), "unterminated quoted string") {
		t.Errorf("error = %q, want contains 'unterminated quoted string'", err.Error())
	}
}

func TestSplitArgs_UnterminatedSingleQuote(t *testing.T) {
	_, err := core.SplitArgs(`--msg 'unterminated`)
	if err == nil {
		t.Fatal("expected parse error, got nil")
	}
}

func TestSplitArgs_UnterminatedEscape(t *testing.T) {
	_, err := core.SplitArgs(`--flag value\`)
	if err == nil {
		t.Fatal("expected parse error, got nil")
	}
	if !strings.Contains(err.Error(), "unterminated escape sequence") {
		t.Errorf("error = %q, want contains 'unterminated escape sequence'", err.Error())
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/ai/core/ -run TestSplitArgs -v`
Expected: FAIL — `undefined: core.SplitArgs`

- [ ] **Step 3: Implement `core.SplitArgs`**

Create `internal/ai/core/cli_args.go`:

```go
package core

import (
	"fmt"
	"strings"
	"unicode"
)

// SplitArgs tokenises a shell-style CLI line into argv, preserving order,
// duplicates, and quoted (single or double) values. Backslash escapes a
// single following rune. Returns an error on unterminated quotes or
// trailing escapes.
//
// This is the canonical tokeniser shared by ai.ValidateExtraArgs and the
// engine backends; do not duplicate this logic elsewhere.
func SplitArgs(s string) ([]string, error) {
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
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/ai/core/ -run TestSplitArgs -v`
Expected: PASS — all 9 subtests green.

- [ ] **Step 5: Run the full package suite (no regressions)**

Run: `go test ./internal/ai/core/...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/ai/core/cli_args.go internal/ai/core/cli_args_test.go
git commit -m "$(cat <<'EOF'
feat(ai/core): add SplitArgs as canonical CLI tokeniser

SplitArgs lifts the existing splitCLIArgs logic out of internal/ai and
into internal/ai/core so engine backends can tokenise opts.ExtraArgs
internally without going through the ai package. The legacy private
splitCLIArgs in factory.go stays in place until callers migrate.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: Add `ai.ResolveExtraArgs` and `ai.ValidateExtraArgs`

**Files:**
- Modify: `internal/ai/factory.go` (append new functions; do not delete the old helpers yet)
- Modify: `internal/ai/factory_test.go` (append new tests; keep the old `TestParseEngineArgs_*` / `TestMergeEngineArgs_*` for now)

Both new helpers are additive. The old `EngineArgsMap`, `ParseEngineArgs`, `ParseEngineArgsJSON`, `MergeEngineArgs`, and private `splitCLIArgs` keep working — they get deleted in Task 4 once nothing references them.

- [ ] **Step 1: Write failing tests for `ResolveExtraArgs` and `ValidateExtraArgs`**

Append to `internal/ai/factory_test.go`:

```go
func TestResolveExtraArgs_SingleLayer(t *testing.T) {
	layer := `{"claude": "--model sonnet --verbose"}`
	got := ai.ResolveExtraArgs("claude", layer)
	if want := "--model sonnet --verbose"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestResolveExtraArgs_MergesLayersInOrder(t *testing.T) {
	base := `{"claude": "--model sonnet --verbose"}`
	override := `{"claude": "--model opus"}`
	got := ai.ResolveExtraArgs("claude", base, override)
	if want := "--model sonnet --verbose --model opus"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestResolveExtraArgs_MissingEngineReturnsEmpty(t *testing.T) {
	layer := `{"codex": "--model o3"}`
	if got := ai.ResolveExtraArgs("claude", layer); got != "" {
		t.Errorf("got %q, want empty string", got)
	}
}

func TestResolveExtraArgs_SkipsEmptyLayers(t *testing.T) {
	got := ai.ResolveExtraArgs("claude", "", "{}", `{"claude":"--model opus"}`)
	if want := "--model opus"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestResolveExtraArgs_SkipsMalformedJSON(t *testing.T) {
	// Malformed first layer is silently dropped; later layer still applies.
	got := ai.ResolveExtraArgs("claude", `{not json`, `{"claude":"--verbose"}`)
	if want := "--verbose"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestResolveExtraArgs_PreservesQuotingInValue(t *testing.T) {
	// Quoted substrings round-trip verbatim into the returned line.
	layer := `{"claude": "--append-system-prompt \"be terse\" --verbose"}`
	got := ai.ResolveExtraArgs("claude", layer)
	if want := `--append-system-prompt "be terse" --verbose`; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestResolveExtraArgs_SkipsEmptyEngineValue(t *testing.T) {
	// An engine whose value is "" contributes nothing to the merged line.
	got := ai.ResolveExtraArgs("claude", `{"claude":""}`, `{"claude":"--verbose"}`)
	if want := "--verbose"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestValidateExtraArgs_OK(t *testing.T) {
	if err := ai.ValidateExtraArgs(`--model sonnet --verbose --msg "hi there"`); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateExtraArgs_Empty(t *testing.T) {
	if err := ai.ValidateExtraArgs(""); err != nil {
		t.Errorf("empty string should validate, got %v", err)
	}
}

func TestValidateExtraArgs_UnterminatedQuote(t *testing.T) {
	err := ai.ValidateExtraArgs(`--model "unterminated`)
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
}
```

- [ ] **Step 2: Run the new tests to verify they fail**

Run: `go test ./internal/ai/ -run 'TestResolveExtraArgs|TestValidateExtraArgs' -v`
Expected: FAIL — `undefined: ai.ResolveExtraArgs` and `undefined: ai.ValidateExtraArgs`.

- [ ] **Step 3: Implement `ResolveExtraArgs` and `ValidateExtraArgs`**

Append to `internal/ai/factory.go` (after the existing Section 4 helpers — those stay until Task 4):

```go
// =========================================================
// Section 5: Engine args resolver (new public surface)
// =========================================================

// ResolveExtraArgs merges any number of engine_args JSON layers and
// returns the raw CLI line for engineName. Each layer is JSON shaped as
// {"<engine>": "<cli line>", ...}. Empty layers ("" and "{}") are
// skipped. A malformed JSON layer is silently skipped, matching the
// behaviour of the old ParseEngineArgsJSON: a corrupt sysconfig row
// must not block running engines.
//
// Layers are concatenated in the order given (base, override, ...) with
// a single space separator. The same base+override semantics as the
// previous MergeEngineArgs but on the un-tokenised string — equivalent
// because the lexer treats whitespace as the token separator.
//
// Returns "" when no layer contributes a value for engineName.
func ResolveExtraArgs(engineName string, layers ...string) string {
	var parts []string
	for _, layer := range layers {
		if layer == "" || layer == "{}" {
			continue
		}
		var raw map[string]string
		if json.Unmarshal([]byte(layer), &raw) != nil {
			continue
		}
		if v, ok := raw[engineName]; ok && v != "" {
			parts = append(parts, v)
		}
	}
	return strings.Join(parts, " ")
}

// ValidateExtraArgs returns nil if s tokenises cleanly under the shared
// CLI lexer (single/double quotes, backslash escape). Used at config
// ingestion to surface typos before they hit a running engine.
func ValidateExtraArgs(s string) error {
	_, err := core.SplitArgs(s)
	return err
}
```

Then update the `import` block at the top of `internal/ai/factory.go` to add the core package. Current imports (lines 3-13):

```go
import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"sync"
	"unicode"

	"github.com/theopenbee/openbee/internal/domain/enginecfg"
)
```

Change to:

```go
import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"sync"
	"unicode"

	core "github.com/theopenbee/openbee/internal/ai/core"
	"github.com/theopenbee/openbee/internal/domain/enginecfg"
)
```

(`unicode` and `slices` are still needed by the legacy helpers until Task 4 removes them; leave both in place.)

- [ ] **Step 4: Run the new tests to verify they pass**

Run: `go test ./internal/ai/ -run 'TestResolveExtraArgs|TestValidateExtraArgs' -v`
Expected: PASS — all 10 subtests green.

- [ ] **Step 5: Run the full ai package suite (no regressions)**

Run: `go test ./internal/ai/...`
Expected: PASS — new tests plus existing Factory + Parse/Merge tests all green.

- [ ] **Step 6: Commit**

```bash
git add internal/ai/factory.go internal/ai/factory_test.go
git commit -m "$(cat <<'EOF'
feat(ai): add ResolveExtraArgs and ValidateExtraArgs

Introduces the additive public surface that will replace the four
legacy helpers (EngineArgsMap, ParseEngineArgs, ParseEngineArgsJSON,
MergeEngineArgs) in a subsequent commit. ResolveExtraArgs merges N
JSON layers and returns the raw CLI line for the named engine;
malformed layers are dropped silently to preserve the existing
fail-open semantics on corrupt sysconfig rows. ValidateExtraArgs
delegates to core.SplitArgs for ingestion-time syntax checks.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: Flip `RunOptions.ExtraArgs` to `string` and migrate all consumers

**Files:**
- Modify: `internal/ai/contracts.go` (field type change)
- Modify: `internal/ai/engine/claude/backend.go` (tokenise via `core.SplitArgs`)
- Modify: `internal/ai/engine/codex/backend.go` (tokenise via `core.SplitArgs`)
- Modify: `internal/ai/engine/kimi/backend.go` (tokenise via `core.SplitArgs`)
- Modify: `internal/ai/engine/pi/backend.go` (tokenise via `core.SplitArgs`)
- Modify: `internal/domain/bee/bee_process.go` (collapse `resolveEngineArgs` + `loadEngineArgs`)
- Modify: `internal/domain/worker/manager.go` (rewrite `resolveEngineArgs` + `ValidateEngineArgs`, replace `loadEngineArgs` with a one-line raw reader)
- Modify: `internal/domain/worker/execution.go` (pass the string through)

This task is one atomic compile-unit: the field type change breaks every consumer simultaneously. Edit in the order below, run `go build ./...` only at the end of the task, then run tests, then commit once.

The legacy `EngineArgsMap`, `ParseEngineArgs`, `ParseEngineArgsJSON`, `MergeEngineArgs` remain in `factory.go` for now — they have no callers after this task, but their tests still reference them. Task 4 removes them.

- [ ] **Step 1: Change `RunOptions.ExtraArgs` field type**

Edit `internal/ai/contracts.go` line 22:

```go
// before
ExtraArgs []string // additional CLI args to pass to the engine

// after
ExtraArgs string // raw CLI tail for the active engine; tokenised by the engine backend
```

- [ ] **Step 2: Update `claude` backend to tokenise internally**

Edit `internal/ai/engine/claude/backend.go`. Locate the existing block (currently around line 136):

```go
args = append(args, opts.ExtraArgs...)
```

Replace with:

```go
extra, err := core.SplitArgs(opts.ExtraArgs)
if err != nil {
	return nil, nil, fmt.Errorf("parse extra args: %w", err)
}
args = append(args, extra...)
```

The function this lives in returns `(ai.Process, <-chan ai.Output, error)`, so `nil, nil, err` is the right signature for the early return. The function already imports `core` and `fmt`; no import changes needed (verify the import block contains both before saving).

- [ ] **Step 3: Update `codex` backend to tokenise internally**

Edit `internal/ai/engine/codex/backend.go`. Locate the `Run` function (currently around line 170-172):

```go
threadID, resume := b.resolveThread(opts.SessionID, opts.Resume)
args := buildArgs(threadID, resume, prompt, opts.ExtraArgs)
```

Replace with:

```go
threadID, resume := b.resolveThread(opts.SessionID, opts.Resume)
extra, err := core.SplitArgs(opts.ExtraArgs)
if err != nil {
	return nil, nil, fmt.Errorf("parse extra args: %w", err)
}
args := buildArgs(threadID, resume, prompt, extra)
```

`buildArgs` still takes `[]string` as its fourth parameter — no signature change needed.

- [ ] **Step 4: Update `kimi` backend to tokenise internally**

Edit `internal/ai/engine/kimi/backend.go`. Locate the `Run` function (currently around lines 168-179):

```go
func (b *Backend) Run(ctx context.Context, workDir, prompt string,
	opts ai.RunOptions, logPath string) (ai.Process, <-chan ai.Output, error) {

	spec := core.SubprocessSpec{
		Binary:  b.binary,
		Args:    buildArgs(opts.SessionID, opts.ExtraArgs),
		WorkDir: workDir,
		LogPath: logPath,
		Env:     core.BuildRunEnv(b.baseEnv, opts.ExtraEnv, opts.APIKey),
		Stdin:   prompt,
	}
	return core.SpawnSubprocess(ctx, spec)
}
```

Replace with:

```go
func (b *Backend) Run(ctx context.Context, workDir, prompt string,
	opts ai.RunOptions, logPath string) (ai.Process, <-chan ai.Output, error) {

	extra, err := core.SplitArgs(opts.ExtraArgs)
	if err != nil {
		return nil, nil, fmt.Errorf("parse extra args: %w", err)
	}
	spec := core.SubprocessSpec{
		Binary:  b.binary,
		Args:    buildArgs(opts.SessionID, extra),
		WorkDir: workDir,
		LogPath: logPath,
		Env:     core.BuildRunEnv(b.baseEnv, opts.ExtraEnv, opts.APIKey),
		Stdin:   prompt,
	}
	return core.SpawnSubprocess(ctx, spec)
}
```

If `fmt` is not currently imported in this file, add it to the import block.

- [ ] **Step 5: Update `pi` backend to tokenise internally**

Edit `internal/ai/engine/pi/backend.go`. Locate the `Run` function (currently around lines 188-193):

```go
func (b *Backend) Run(ctx context.Context, workDir, prompt string,
	opts ai.RunOptions, logPath string) (ai.Process, <-chan ai.Output, error) {

	sessionPath := b.sessionFilePath(opts.SessionID)

	args := buildArgs(prompt, sessionPath, opts.ExtraArgs)
```

Replace the body up to the `args :=` line with:

```go
func (b *Backend) Run(ctx context.Context, workDir, prompt string,
	opts ai.RunOptions, logPath string) (ai.Process, <-chan ai.Output, error) {

	sessionPath := b.sessionFilePath(opts.SessionID)
	extra, err := core.SplitArgs(opts.ExtraArgs)
	if err != nil {
		return nil, nil, fmt.Errorf("parse extra args: %w", err)
	}
	args := buildArgs(prompt, sessionPath, extra)
```

If `fmt` is not currently imported, add it.

Note: the file already declares `err` later (at the log-file `os.OpenFile` step). Renaming `extra, err :=` to `extra, splitErr :=` is *not* required because Go scopes the second `err` declaration in `cmd.StdoutPipe()` separately via `:=`; however if the existing function uses `err :=` at the top level you may need to switch to `=` later. Verify after editing: `go build ./internal/ai/engine/pi/...` should report any shadowing issues.

- [ ] **Step 6: Collapse `BeeProcess` resolver**

Edit `internal/domain/bee/bee_process.go`. The current file (lines 46-83) has `Run`, `resolveEngineArgs`, and `loadEngineArgs`. Replace the three methods so the file ends up as:

```go
// Run injects a bee auth token then delegates to the engine.
func (p *BeeProcess) Run(ctx context.Context, workDir, prompt string, opts ai.RunOptions, logPath string) (ai.RunResult, error) {
	token, err := auth.GenerateBeeToken(p.tokenSecret, p.tokenTTL)
	if err != nil {
		return ai.RunResult{}, fmt.Errorf("generate bee token: %w", err)
	}

	extraEnv, err := p.envService.ResolveBeeEnv(defaultBeeID)
	if err != nil {
		return ai.RunResult{}, fmt.Errorf("resolve bee env: %w", err)
	}

	globalJSON := p.readSysConfig(ctx, model.SystemConfigKeyEngineArgsGlobal)
	beeJSON := p.readSysConfig(ctx, model.SystemConfigKeyEngineArgsBee)

	opts.ExtraEnv = extraEnv
	opts.APIKey = token
	opts.ExtraArgs = ai.ResolveExtraArgs(p.engineCfg.Get(), globalJSON, beeJSON)
	return p.engine.Run(ctx, workDir, prompt, opts, logPath)
}

func (b *BeeProcess) CollectTokenUsage(ctx context.Context, sessionID string) ([]ai.TokenUsage, error) {
	return b.engine.CollectTokenUsage(ctx, sessionID)
}

// readSysConfig returns the raw config value, or "" on miss / read error.
// Errors are deliberately swallowed: a missing or corrupt engine_args row
// must not block the bee from running.
func (p *BeeProcess) readSysConfig(ctx context.Context, key string) string {
	if p.sysConfigStore == nil {
		return ""
	}
	cfg, found, err := p.sysConfigStore.Get(ctx, key)
	if err != nil || !found {
		return ""
	}
	return cfg.Value
}
```

This deletes both `resolveEngineArgs` and `loadEngineArgs`. The new `readSysConfig` replaces `loadEngineArgs` and returns the raw `string` instead of an `ai.EngineArgsMap`.

- [ ] **Step 7: Collapse `worker.Manager` resolver and rewrite `ValidateEngineArgs`**

Edit `internal/domain/worker/manager.go`. Replace the block at lines 106-140 (the three methods `loadEngineArgs`, `resolveEngineArgs`, `ValidateEngineArgs`) with:

```go
// readSysConfigValue returns the raw config value, or "" on miss / read
// error. Errors are deliberately swallowed: a missing or corrupt
// engine_args row must not block worker runs.
func (m *Manager) readSysConfigValue(ctx context.Context, key string) string {
	if m.sysConfigStore == nil {
		return ""
	}
	cfg, found, err := m.sysConfigStore.Get(ctx, key)
	if err != nil || !found {
		return ""
	}
	return cfg.Value
}

func (m *Manager) resolveEngineArgs(ctx context.Context, worker model.Worker, engineName string) string {
	globalJSON := m.readSysConfigValue(ctx, model.SystemConfigKeyEngineArgsGlobal)
	return ai.ResolveExtraArgs(engineName, globalJSON, worker.EngineArgs)
}

func (m *Manager) ValidateEngineArgs(raw map[string]string) error {
	if len(raw) == 0 {
		return nil
	}
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

- [ ] **Step 8: Update the `execution.go` call site**

Edit `internal/domain/worker/execution.go` line 71-79. The current code:

```go
extraArgs := m.resolveEngineArgs(ctx, worker, engineName)

runRes, err := engine.Run(execCtx, worker.WorkDir, prompt, ai.RunOptions{
	SessionID: exec.SessionID,
	Resume:    resume,
	APIKey:    token,
	ExtraEnv:  extraEnv,
	ExtraArgs: extraArgs,
}, logPath)
```

stays as-is. `extraArgs` is now a `string` because `resolveEngineArgs` returns `string`; the struct field is also `string`. No edits required in this step — confirm the file compiles by running `go build ./internal/domain/worker/...` after Step 9.

- [ ] **Step 9: Build the whole module to catch any missed reference**

Run: `go build ./...`
Expected: clean build. If any callsite still passes `[]string` to `ExtraArgs` (e.g., a test fixture), the compile error points right at it — fix in place and re-run.

- [ ] **Step 10: Run the full test suite**

Run: `go test ./...`
Expected: PASS. The legacy `TestParseEngineArgs_*` and `TestMergeEngineArgs_*` cases still pass because the old helpers are still in the package (Task 4 removes them).

- [ ] **Step 11: Commit**

```bash
git add internal/ai/contracts.go \
        internal/ai/engine/claude/backend.go \
        internal/ai/engine/codex/backend.go \
        internal/ai/engine/kimi/backend.go \
        internal/ai/engine/pi/backend.go \
        internal/domain/bee/bee_process.go \
        internal/domain/worker/manager.go
git commit -m "$(cat <<'EOF'
refactor(ai): switch RunOptions.ExtraArgs to string

Engine backends now own CLI tokenisation via core.SplitArgs. Bee and
worker callers collapse their parse/merge/lookup pipelines into a
single ai.ResolveExtraArgs call. Manager.ValidateEngineArgs validates
each CLI line via ai.ValidateExtraArgs.

Legacy helpers (EngineArgsMap, ParseEngineArgs, ParseEngineArgsJSON,
MergeEngineArgs, splitCLIArgs) are no longer referenced; they are
removed in the follow-up commit so this change is reviewable in
isolation.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: Delete the legacy helpers and their tests

**Files:**
- Modify: `internal/ai/factory.go` (delete Section 4)
- Modify: `internal/ai/factory_test.go` (delete `TestParseEngineArgs_*` and `TestMergeEngineArgs_*`)

- [ ] **Step 1: Delete the legacy helpers**

In `internal/ai/factory.go`, delete the entire `Section 4: Engine CLI argument helpers` block and the trailing `splitCLIArgs` function. The exact span in the current file is **lines 192-314 inclusive** — from the header comment:

```go
// =========================================================
// Section 4: Engine CLI argument helpers
// =========================================================
```

through the closing brace of `splitCLIArgs` (the last `}` before EOF, before Section 5). After this delete the file should end with Section 5 (`ResolveExtraArgs` and `ValidateExtraArgs`).

Also re-check the import block at the top of `internal/ai/factory.go`:

- `encoding/json` — still needed by `ResolveExtraArgs`. Keep.
- `slices` — was used by `MergeEngineArgs`. **Delete** if no other reference.
- `strings` — still needed by `ResolveExtraArgs` (`strings.Join`). Keep.
- `unicode` — was used by `splitCLIArgs`. **Delete** if no other reference.

After editing imports, the import block should read:

```go
import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	core "github.com/theopenbee/openbee/internal/ai/core"
	"github.com/theopenbee/openbee/internal/domain/enginecfg"
)
```

- [ ] **Step 2: Delete the legacy tests**

In `internal/ai/factory_test.go`, delete the five test functions:

- `TestParseEngineArgs_PreservesOrderAndQuotedValues`
- `TestParseEngineArgs_PreservesDuplicateFlags`
- `TestParseEngineArgs_PreservesEmptyQuotedValue`
- `TestParseEngineArgs_UnterminatedQuote`
- `TestMergeEngineArgs_AppendsOverrideArgs`

(Equivalent coverage now lives in `TestSplitArgs_*` in `internal/ai/core/cli_args_test.go` plus `TestResolveExtraArgs_*` / `TestValidateExtraArgs_*` in `internal/ai/factory_test.go`.)

Also re-check the import block at the top of `internal/ai/factory_test.go`:

- `slices` — was used by the deleted tests. **Delete** if no other test still uses it. (`TestFactory_NamesIncludesAllRegistrations` uses `slices.Contains`, so keep it.)

- [ ] **Step 3: Build and run the full test suite**

Run: `go build ./... && go test ./...`
Expected: PASS. No references to the deleted symbols anywhere in the module.

- [ ] **Step 4: Verify no straggler references**

Run: `grep -rn 'EngineArgsMap\|ParseEngineArgs\|ParseEngineArgsJSON\|MergeEngineArgs\|splitCLIArgs' internal/`
Expected: no output. (Use the Grep tool, not Bash `grep`, when running this from the agent.)

- [ ] **Step 5: Commit**

```bash
git add internal/ai/factory.go internal/ai/factory_test.go
git commit -m "$(cat <<'EOF'
refactor(ai): remove legacy engine-args helpers

EngineArgsMap, ParseEngineArgs, ParseEngineArgsJSON, MergeEngineArgs,
and the private splitCLIArgs are no longer referenced in the tree.
Equivalent coverage is provided by core.SplitArgs + ai.ResolveExtraArgs
+ ai.ValidateExtraArgs.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

## Final verification

- [ ] **Step 1: Full module build**

Run: `go build ./...`
Expected: clean build.

- [ ] **Step 2: Full module test**

Run: `go test ./...`
Expected: PASS, no skipped suites.

- [ ] **Step 3: Vet**

Run: `go vet ./...`
Expected: no findings.

- [ ] **Step 4: Public-surface diff check**

Run (Grep tool):

Pattern: `^func [A-Z]|^type [A-Z]|^var [A-Z]|^const [A-Z]`
Path: `internal/ai/factory.go`, `internal/ai/contracts.go`

Confirm against the spec's "Public `ai` surface" section:
- ✅ Present: `AllEngines`, `RegisterEngine`, `NewFactory`, `Factory` (methods: `Build`, `Get`, `Enabled`, `Names`, `Dynamic`), `EngineConfig`, `EngineConstructor`, `ResolveExtraArgs`, `ValidateExtraArgs`, plus the existing `Role`, `RunOptions`, `Output*`, `RunResult`, `Process`, `EngineAdapter`, `TokenUsage`, `ErrSessionDataNotFound`.
- ❌ Absent: `EngineArgsMap`, `ParseEngineArgs`, `ParseEngineArgsJSON`, `MergeEngineArgs`.

- [ ] **Step 5: Worktree handoff**

If the work was done in a worktree, switch to the `finishing-a-development-branch` skill to merge, open a PR, or otherwise integrate.
