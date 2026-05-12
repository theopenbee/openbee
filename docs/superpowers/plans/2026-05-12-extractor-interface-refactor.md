# Extractor Interface Refactor Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace `BaseAdapter.Extract func(string) string` with `BaseAdapter.Extractor Extractor` interface (mirroring `Invoker` / `Collector`), and migrate each engine's `ExtractResultFromLog` free function to a method on a per-engine `Extractor` struct (no remaining free function).

**Architecture:** Additive-then-subtractive migration. Task 1 adds the new `Extractor` interface and `Extractor` field alongside the existing `Extract` func field — `Run` prefers `Extractor` when set, falling back to `Extract` — so engines compile unchanged. Tasks 2–5 migrate each of the four engines (claude, codex, kimi, pi) independently with green builds and committed at each step. Task 6 removes the legacy `Extract` field and dual-path branch.

**Tech Stack:** Go 1.x, standard library only. Existing test framework (`go test`).

---

## File Structure

**Modified:**
- `internal/ai/core/adapter.go` — add `Extractor` interface, add field, dual-path `Run`, then remove old field in Task 6.
- `internal/ai/core/adapter_test.go` — add `fakeExtractor` type, migrate three test cases off the old `Extract:` field.
- `internal/ai/engine/claude/invoker.go` — replace `func ExtractResultFromLog(...)` with `type Extractor struct{}` + `func (Extractor) Extract(...)`.
- `internal/ai/engine/claude/adapter.go` — change `Extract: ExtractResultFromLog` to `Extractor: Extractor{}`.
- `internal/ai/engine/codex/invoker.go` — same pattern as claude.
- `internal/ai/engine/codex/adapter.go` — same pattern as claude.
- `internal/ai/engine/codex/invoker_test.go` — migrate `ExtractResultFromLog(path)` calls to `Extractor{}.Extract(path)`.
- `internal/ai/engine/kimi/invoker.go` — same pattern as claude.
- `internal/ai/engine/kimi/adapter.go` — same pattern as claude.
- `internal/ai/engine/kimi/invoker_test.go` — same pattern as codex test.
- `internal/ai/engine/pi/invoker.go` — same pattern as claude.
- `internal/ai/engine/pi/adapter.go` — same pattern as claude.
- `internal/ai/engine/pi/invoker_test.go` — same pattern as codex test.

claude has no `invoker_test.go` test for `ExtractResultFromLog`, so no test file changes there.

---

## Task 1: Add `Extractor` interface to core (backward compatible)

**Files:**
- Modify: `internal/ai/core/adapter.go`
- Modify: `internal/ai/core/adapter_test.go`

- [ ] **Step 1: Add the failing test for the new interface field**

Append to `internal/ai/core/adapter_test.go`:

```go
type fakeExtractor struct {
	captured *string
	result   string
}

func (f *fakeExtractor) Extract(logPath string) string {
	if f.captured != nil {
		*f.captured = logPath
	}
	return f.result
}

func TestBaseAdapter_RunPrefersExtractorOverExtract(t *testing.T) {
	ch := make(chan ai.Output)
	close(ch)
	var capturedLogPath string
	b := &core.BaseAdapter{
		Invoker:   &fakeInvoker{ch: ch},
		Collector: &fakeCollector{},
		Extractor: &fakeExtractor{captured: &capturedLogPath, result: "from-iface"},
	}
	res, err := b.Run(context.Background(), "/wd", "p", ai.RunOptions{}, "/the/log")
	if err != nil {
		t.Fatal(err)
	}
	if r := res.ExtractResult(); r != "from-iface" {
		t.Errorf("got %q, want %q", r, "from-iface")
	}
	if capturedLogPath != "/the/log" {
		t.Errorf("logPath not bound; got %q", capturedLogPath)
	}
}
```

- [ ] **Step 2: Run the new test to verify it fails**

Run: `go test ./internal/ai/core/ -run TestBaseAdapter_RunPrefersExtractorOverExtract -v`

Expected: build error — `unknown field Extractor in struct literal of type core.BaseAdapter` (or similar).

- [ ] **Step 3: Add interface and field to core/adapter.go**

Replace `internal/ai/core/adapter.go` with:

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

// Extractor reads the per-engine result text from a log file produced by Invoker.Run.
type Extractor interface {
	Extract(logPath string) string
}

// BaseAdapter implements the EngineAdapter parts that are identical across
// engines: Run wires the invoker output into a RunResult with a bound result
// extractor; CollectTokenUsage delegates to the collector. Engines embed
// BaseAdapter and optionally override Prepare.
type BaseAdapter struct {
	Invoker   Invoker
	Collector Collector
	// Extractor is the per-engine result extractor; preferred over Extract when set.
	Extractor Extractor
	// Extract is the legacy func-typed extractor; will be removed once all engines migrate.
	Extract func(logPath string) string
}

// Run launches the invoker and binds the extractor to logPath in the returned RunResult.
func (b *BaseAdapter) Run(ctx context.Context, workDir, prompt string,
	opts ai.RunOptions, logPath string) (ai.RunResult, error) {
	proc, out, err := b.Invoker.Run(ctx, workDir, prompt, opts, logPath)
	return ai.NewRunResult(proc, out, err, func() string {
		if b.Extractor != nil {
			return b.Extractor.Extract(logPath)
		}
		return b.Extract(logPath)
	})
}

// CollectTokenUsage delegates to the embedded collector.
func (b *BaseAdapter) CollectTokenUsage(ctx context.Context, sessionID string) ([]ai.TokenUsage, error) {
	return b.Collector.Collect(ctx, sessionID)
}

// Prepare is a no-op default that engines may override (e.g. claude).
func (b *BaseAdapter) Prepare(string, ai.PrepareOptions) error { return nil }
```

- [ ] **Step 4: Run all core tests to verify**

Run: `go test ./internal/ai/core/ -v`

Expected: all pass, including the new `TestBaseAdapter_RunPrefersExtractorOverExtract` and the existing `TestBaseAdapter_RunBindsExtract` (which still uses the old `Extract:` field).

- [ ] **Step 5: Verify the whole tree still builds**

Run: `go build ./...`

Expected: success — engines still compile because `Extract` field is preserved.

- [ ] **Step 6: Commit**

```bash
git add internal/ai/core/adapter.go internal/ai/core/adapter_test.go
git commit -m "refactor(ai/core): add Extractor interface alongside Extract field"
```

---

## Task 2: Migrate claude engine to Extractor interface

**Files:**
- Modify: `internal/ai/engine/claude/invoker.go:70-78`
- Modify: `internal/ai/engine/claude/adapter.go:36-43`

claude has no test file calling `ExtractResultFromLog` directly; no test changes needed here.

- [ ] **Step 1: Replace the free function with a method**

In `internal/ai/engine/claude/invoker.go`, replace:

```go
// ExtractResultFromLog scans a Claude stream-json log file and returns the best
// result string: prefers {"type":"result"} over the last assistant text.
func ExtractResultFromLog(logPath string) string {
	result, _, lastAssistantText := scanResultLog(logPath)
	if result != "" {
		return result
	}
	return lastAssistantText
}
```

with:

```go
// Extractor reads the result text from a Claude stream-json log file: prefers
// {"type":"result"} over the last assistant text.
type Extractor struct{}

// Extract implements core.Extractor.
func (Extractor) Extract(logPath string) string {
	result, _, lastAssistantText := scanResultLog(logPath)
	if result != "" {
		return result
	}
	return lastAssistantText
}
```

- [ ] **Step 2: Update adapter.go to use the new field**

In `internal/ai/engine/claude/adapter.go`, change the `NewAdapter` body's struct literal from:

```go
return &claudeAdapter{
	BaseAdapter: &core.BaseAdapter{
		Invoker:   NewInvoker(binaryPath, extraEnv),
		Collector: NewCollector(),
		Extract:   ExtractResultFromLog,
	},
}
```

to:

```go
return &claudeAdapter{
	BaseAdapter: &core.BaseAdapter{
		Invoker:   NewInvoker(binaryPath, extraEnv),
		Collector: NewCollector(),
		Extractor: Extractor{},
	},
}
```

- [ ] **Step 3: Build and test claude package**

Run: `go build ./internal/ai/engine/claude/... && go test ./internal/ai/engine/claude/... -v`

Expected: build success, all tests pass.

- [ ] **Step 4: Verify whole tree still builds**

Run: `go build ./...`

Expected: success.

- [ ] **Step 5: Commit**

```bash
git add internal/ai/engine/claude/invoker.go internal/ai/engine/claude/adapter.go
git commit -m "refactor(ai/claude): migrate ExtractResultFromLog to Extractor.Extract"
```

---

## Task 3: Migrate codex engine to Extractor interface

**Files:**
- Modify: `internal/ai/engine/codex/invoker.go:112-134`
- Modify: `internal/ai/engine/codex/adapter.go:22-26`
- Modify: `internal/ai/engine/codex/invoker_test.go:53-62`

- [ ] **Step 1: Replace the free function with a method**

In `internal/ai/engine/codex/invoker.go`, replace:

```go
// ExtractResultFromLog scans a Codex JSON log file and returns the text of the
// last agent_message item, or "" if none found.
func ExtractResultFromLog(logPath string) string {
	f, err := os.Open(logPath)
	if err != nil {
		return ""
	}
	defer f.Close()

	var lastText string
	core.ScanJSONLines(f, func(line string) bool {
		var event codexEvent
		if json.Unmarshal([]byte(line), &event) != nil {
			return true
		}
		if event.Type == codexEventItemCompleted && event.Item != nil &&
			event.Item.Type == codexItemAgentMessage && event.Item.Text != "" {
			lastText = event.Item.Text
		}
		return true
	})
	return lastText
}
```

with:

```go
// Extractor reads the result text from a Codex JSON log file: returns the text
// of the last agent_message item, or "" if none found.
type Extractor struct{}

// Extract implements core.Extractor.
func (Extractor) Extract(logPath string) string {
	f, err := os.Open(logPath)
	if err != nil {
		return ""
	}
	defer f.Close()

	var lastText string
	core.ScanJSONLines(f, func(line string) bool {
		var event codexEvent
		if json.Unmarshal([]byte(line), &event) != nil {
			return true
		}
		if event.Type == codexEventItemCompleted && event.Item != nil &&
			event.Item.Type == codexItemAgentMessage && event.Item.Text != "" {
			lastText = event.Item.Text
		}
		return true
	})
	return lastText
}
```

- [ ] **Step 2: Update adapter.go to use the new field**

In `internal/ai/engine/codex/adapter.go`, change the `NewAdapter` struct literal from:

```go
return &core.BaseAdapter{
	Invoker:   NewInvoker(binaryPath, store, extraEnv),
	Collector: NewCollector(),
	Extract:   ExtractResultFromLog,
}, nil
```

to:

```go
return &core.BaseAdapter{
	Invoker:   NewInvoker(binaryPath, store, extraEnv),
	Collector: NewCollector(),
	Extractor: Extractor{},
}, nil
```

- [ ] **Step 3: Update test file calls**

In `internal/ai/engine/codex/invoker_test.go`, find the line:

```go
result := ExtractResultFromLog(tmpFile)
```

and replace with:

```go
result := Extractor{}.Extract(tmpFile)
```

The test function name (`TestExtractResultFromLog`) stays as-is — it's a label, not a reference.

- [ ] **Step 4: Build and test codex package**

Run: `go build ./internal/ai/engine/codex/... && go test ./internal/ai/engine/codex/... -v`

Expected: build success, all tests pass (including the renamed-call `TestExtractResultFromLog`).

- [ ] **Step 5: Verify whole tree still builds**

Run: `go build ./...`

Expected: success.

- [ ] **Step 6: Commit**

```bash
git add internal/ai/engine/codex/invoker.go internal/ai/engine/codex/adapter.go internal/ai/engine/codex/invoker_test.go
git commit -m "refactor(ai/codex): migrate ExtractResultFromLog to Extractor.Extract"
```

---

## Task 4: Migrate kimi engine to Extractor interface

**Files:**
- Modify: `internal/ai/engine/kimi/invoker.go:78-142`
- Modify: `internal/ai/engine/kimi/adapter.go:15-21`
- Modify: `internal/ai/engine/kimi/invoker_test.go:36-99`

- [ ] **Step 1: Replace the free function with a method**

In `internal/ai/engine/kimi/invoker.go`, change the function definition line from:

```go
func ExtractResultFromLog(logPath string) string {
```

to a struct + method definition above it:

```go
// Extractor reads the result text from a Kimi stream-json log.
//
// The content field may be a plain string or an array of content blocks.
// Text blocks starting with "(Empty response:" are skipped — when Kimi ends
// with such a placeholder, the actual response was already sent to the user
// via `openbee ctl message send --stdin`. In that case the heredoc body from
// the last matching Shell tool call is returned instead.
type Extractor struct{}

// Extract implements core.Extractor.
func (Extractor) Extract(logPath string) string {
```

(Move the existing block-comment that was above `ExtractResultFromLog` into the new struct comment as shown; the function body itself is unchanged.)

Verify by reading the surrounding code — the body from the existing function (lines 86–142) stays the same; only the signature line and surrounding comment change.

- [ ] **Step 2: Update adapter.go to use the new field**

In `internal/ai/engine/kimi/adapter.go`, change the struct literal from:

```go
return &core.BaseAdapter{
	Invoker:   NewInvoker(binaryPath, extraEnv),
	Collector: NewCollector(),
	Extract:   ExtractResultFromLog,
}
```

to:

```go
return &core.BaseAdapter{
	Invoker:   NewInvoker(binaryPath, extraEnv),
	Collector: NewCollector(),
	Extractor: Extractor{},
}
```

- [ ] **Step 3: Update all test call sites in invoker_test.go**

In `internal/ai/engine/kimi/invoker_test.go`, replace every occurrence of:

```go
ExtractResultFromLog(
```

with:

```go
Extractor{}.Extract(
```

There are 7 call sites (per test functions: `_StringContent`, `_ArrayContent`, `_ArrayContentFirstTextBlock`, `_LastAssistantWins`, `_Empty`, `_NoAssistant`, `_MissingFile`). Use `grep -n ExtractResultFromLog internal/ai/engine/kimi/invoker_test.go` first to confirm each one, then edit. Test function names stay as labels.

- [ ] **Step 4: Build and test kimi package**

Run: `go build ./internal/ai/engine/kimi/... && go test ./internal/ai/engine/kimi/... -v`

Expected: build success, all kimi tests pass.

- [ ] **Step 5: Verify whole tree still builds**

Run: `go build ./...`

Expected: success.

- [ ] **Step 6: Commit**

```bash
git add internal/ai/engine/kimi/invoker.go internal/ai/engine/kimi/adapter.go internal/ai/engine/kimi/invoker_test.go
git commit -m "refactor(ai/kimi): migrate ExtractResultFromLog to Extractor.Extract"
```

---

## Task 5: Migrate pi engine to Extractor interface

**Files:**
- Modify: `internal/ai/engine/pi/invoker.go:89-102`
- Modify: `internal/ai/engine/pi/adapter.go:20-24`
- Modify: `internal/ai/engine/pi/invoker_test.go:29-71`

- [ ] **Step 1: Replace the free function with a method**

In `internal/ai/engine/pi/invoker.go`, replace:

```go
// ExtractResultFromLog scans logPath for the last agent_end event and returns
// the text of the last assistant message's first text content item, or "".
func ExtractResultFromLog(logPath string) string {
	msg := scanLastAssistantMessage(logPath)
	if msg == nil {
		return ""
	}
	for _, c := range msg.Content {
		if c.Type == contentTypeText && c.Text != "" {
			return c.Text
		}
	}
	return ""
}
```

with:

```go
// Extractor reads the result text from a pi log: returns the text of the last
// assistant message's first text content item from the last agent_end event,
// or "".
type Extractor struct{}

// Extract implements core.Extractor.
func (Extractor) Extract(logPath string) string {
	msg := scanLastAssistantMessage(logPath)
	if msg == nil {
		return ""
	}
	for _, c := range msg.Content {
		if c.Type == contentTypeText && c.Text != "" {
			return c.Text
		}
	}
	return ""
}
```

- [ ] **Step 2: Update adapter.go to use the new field**

In `internal/ai/engine/pi/adapter.go`, change the struct literal from:

```go
return &core.BaseAdapter{
	Invoker:   inv,
	Collector: NewCollector(),
	Extract:   ExtractResultFromLog,
}, nil
```

to:

```go
return &core.BaseAdapter{
	Invoker:   inv,
	Collector: NewCollector(),
	Extractor: Extractor{},
}, nil
```

- [ ] **Step 3: Update all test call sites in invoker_test.go**

In `internal/ai/engine/pi/invoker_test.go`, replace every occurrence of:

```go
ExtractResultFromLog(
```

with:

```go
Extractor{}.Extract(
```

There are 5 call sites (per test functions: `_BasicResult`, `_LastAgentEndWins`, `_SkipsNonTextContent`, `_Empty`, `_NoAgentEnd`). Use `grep -n ExtractResultFromLog internal/ai/engine/pi/invoker_test.go` first to confirm each one, then edit. Test function names stay as labels.

- [ ] **Step 4: Build and test pi package**

Run: `go build ./internal/ai/engine/pi/... && go test ./internal/ai/engine/pi/... -v`

Expected: build success, all pi tests pass.

- [ ] **Step 5: Verify whole tree still builds**

Run: `go build ./...`

Expected: success.

- [ ] **Step 6: Commit**

```bash
git add internal/ai/engine/pi/invoker.go internal/ai/engine/pi/adapter.go internal/ai/engine/pi/invoker_test.go
git commit -m "refactor(ai/pi): migrate ExtractResultFromLog to Extractor.Extract"
```

---

## Task 6: Remove legacy `Extract` field from core

**Files:**
- Modify: `internal/ai/core/adapter.go`
- Modify: `internal/ai/core/adapter_test.go`

- [ ] **Step 1: Verify no engine still references the old field**

Run: `grep -rn "Extract:" internal/ai/engine/`

Expected: no output (every engine should now set `Extractor:` instead).

Run: `grep -rn "ExtractResultFromLog" internal/`

Expected: no output (every free function has been replaced by an `Extractor` struct method, and tests now call `Extractor{}.Extract`).

If either grep returns hits, stop and migrate them before proceeding.

- [ ] **Step 2: Remove the Extract field and dual-path branch from adapter.go**

In `internal/ai/core/adapter.go`, change the `BaseAdapter` struct from:

```go
type BaseAdapter struct {
	Invoker   Invoker
	Collector Collector
	// Extractor is the per-engine result extractor; preferred over Extract when set.
	Extractor Extractor
	// Extract is the legacy func-typed extractor; will be removed once all engines migrate.
	Extract func(logPath string) string
}
```

to:

```go
type BaseAdapter struct {
	Invoker   Invoker
	Collector Collector
	// Extractor is the per-engine result extractor bound to logPath in Run.
	Extractor Extractor
}
```

And change `Run`'s closure body from:

```go
return ai.NewRunResult(proc, out, err, func() string {
	if b.Extractor != nil {
		return b.Extractor.Extract(logPath)
	}
	return b.Extract(logPath)
})
```

to:

```go
return ai.NewRunResult(proc, out, err, func() string {
	return b.Extractor.Extract(logPath)
})
```

- [ ] **Step 3: Migrate the legacy adapter tests in adapter_test.go**

In `internal/ai/core/adapter_test.go`, three existing tests still use `Extract: func(string) string {...}` literals: `TestBaseAdapter_RunBindsExtract`, `TestBaseAdapter_RunPropagatesError`, `TestBaseAdapter_CollectDelegates`. Migrate them to use `fakeExtractor` (added in Task 1).

Also: the new `TestBaseAdapter_RunPrefersExtractorOverExtract` from Task 1 is now obsolete (it tested the dual-path) — delete it.

After edits, the relevant test bodies should look like:

```go
func TestBaseAdapter_RunBindsExtract(t *testing.T) {
	ch := make(chan ai.Output)
	close(ch)
	var capturedLogPath string
	b := &core.BaseAdapter{
		Invoker:   &fakeInvoker{ch: ch},
		Collector: &fakeCollector{},
		Extractor: &fakeExtractor{captured: &capturedLogPath, result: "x"},
	}
	res, err := b.Run(context.Background(), "/wd", "p", ai.RunOptions{}, "/the/log")
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

func TestBaseAdapter_RunPropagatesError(t *testing.T) {
	wantErr := errors.New("boom")
	b := &core.BaseAdapter{
		Invoker:   &fakeInvoker{err: wantErr},
		Collector: &fakeCollector{},
		Extractor: &fakeExtractor{},
	}
	_, err := b.Run(context.Background(), "/wd", "", ai.RunOptions{}, "/log")
	if !errors.Is(err, wantErr) {
		t.Errorf("want wantErr, got %v", err)
	}
}

func TestBaseAdapter_CollectDelegates(t *testing.T) {
	want := []ai.TokenUsage{{Model: "m", InputTokens: 7}}
	b := &core.BaseAdapter{
		Invoker:   &fakeInvoker{},
		Collector: &fakeCollector{usages: want},
		Extractor: &fakeExtractor{},
	}
	got, err := b.CollectTokenUsage(context.Background(), "sid")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Model != "m" || got[0].InputTokens != 7 {
		t.Errorf("delegation broken; got %+v", got)
	}
}
```

`TestBaseAdapter_PrepareIsNoop` is unchanged. `fakeExtractor` (added in Task 1) is unchanged.

- [ ] **Step 4: Run all core tests**

Run: `go test ./internal/ai/core/ -v`

Expected: all four tests pass (`RunBindsExtract`, `RunPropagatesError`, `PrepareIsNoop`, `CollectDelegates`); the obsolete `RunPrefersExtractorOverExtract` is gone.

- [ ] **Step 5: Run the full test suite**

Run: `go test ./...`

Expected: all packages green.

- [ ] **Step 6: Final grep verification**

Run: `grep -rn "ExtractResultFromLog" internal/ cmd/`

Expected: no output.

Run: `grep -rn "Extract func" internal/ai/`

Expected: no output (the legacy field signature is gone).

- [ ] **Step 7: Commit**

```bash
git add internal/ai/core/adapter.go internal/ai/core/adapter_test.go
git commit -m "refactor(ai/core): remove legacy Extract func field"
```

---

## Acceptance Verification

After Task 6:

- `go build ./...` exits 0.
- `go test ./...` passes.
- `grep -rn "ExtractResultFromLog" internal/ cmd/` returns nothing.
- `BaseAdapter` has three same-style fields: `Invoker`, `Collector`, `Extractor`, all interface-typed.
- Six commits on the branch implementing the refactor.
