# Engine Tests Reorganization Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rename and merge `internal/ai/engine/{claude,codex,pi,kimi}` test files so that each source file maps 1:1 to its test file, unify on internal test packages, and drop two redundant test cases.

**Architecture:** Pure test-file reorganization. One engine package per task: merge `invoker_test.go` + `token_usage_test.go` into a single internal-package `backend_test.go`, rename any build-tagged or bench variants, delete obsolete `adapter_test.go` files. Production code only changes in `codex/adapter.go` (compile-time interface assertion).

**Tech Stack:** Go test files. `go test ./internal/ai/engine/...` for verification.

---

## File Structure

After this reorg, each engine package has the following test layout:

```
internal/ai/engine/claude/
  backend_test.go         (merged: invoker_test + token_usage_test + cleanupLegacyRules tests)
  backend_unix_test.go    (renamed from invoker_unix_test.go; keeps //go:build !windows)
  download_test.go        (unchanged)
  provider_test.go        (unchanged)
  # adapter_test.go deleted

internal/ai/engine/codex/
  backend_test.go         (merged: invoker_test + token_usage_test)
  session_store_test.go   (unchanged)
  # adapter_test.go deleted; compile-time assertion moved into adapter.go

internal/ai/engine/pi/
  backend_test.go         (merged: invoker_test + token_usage_test)
  backend_bench_test.go   (renamed from invoker_bench_test.go)

internal/ai/engine/kimi/
  backend_test.go         (merged: invoker_test + token_usage_test)
```

All `_test.go` files in the merged set use `package <engine>` (internal), not `package <engine>_test`.

---

## Task 1: Kimi — merge invoker_test + token_usage_test into backend_test

**Files:**
- Delete: `internal/ai/engine/kimi/invoker_test.go`
- Delete: `internal/ai/engine/kimi/token_usage_test.go`
- Create: `internal/ai/engine/kimi/backend_test.go`

- [ ] **Step 1: Confirm baseline (read current files)**

Read the two source-of-truth test files to copy their assertions verbatim:
- `internal/ai/engine/kimi/invoker_test.go` — uses `package kimi` (internal), defines helper `writeTemp`, tests `TestBuildArgs`, `TestExtractResultFromLog_*`.
- `internal/ai/engine/kimi/token_usage_test.go` — uses `package kimi_test` (external), defines helper `makeKimiSessionFile`, tests `TestKimiCollector_Collect_*`.

- [ ] **Step 2: Create the merged `backend_test.go`**

Create `internal/ai/engine/kimi/backend_test.go`:

```go
package kimi

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	ai "github.com/theopenbee/openbee/internal/ai"
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

func makeKimiSessionFile(t *testing.T, sessionsDir, sessionID, content string) {
	t.Helper()
	dir := filepath.Join(sessionsDir, "bucket-01", sessionID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "wire.jsonl"), []byte(content), 0644); err != nil {
		t.Fatalf("write wire.jsonl: %v", err)
	}
}

func TestBuildArgs(t *testing.T) {
	args := buildArgs("550e8400-e29b-41d4-a716-446655440000", nil)
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
	got := (&Backend{}).Extract(path)
	if got != "world" {
		t.Errorf("got %q, want %q", got, "world")
	}
}

func TestExtractResultFromLog_ArrayContent(t *testing.T) {
	log := `{"role":"assistant","content":[{"type":"text","text":"array answer"}]}
`
	path := writeTemp(t, log)
	got := (&Backend{}).Extract(path)
	if got != "array answer" {
		t.Errorf("got %q, want %q", got, "array answer")
	}
}

func TestExtractResultFromLog_ArrayContentFirstTextBlock(t *testing.T) {
	log := `{"role":"assistant","content":[{"type":"tool_use","id":"tc_1"},{"type":"text","text":"after tool"}]}
`
	path := writeTemp(t, log)
	got := (&Backend{}).Extract(path)
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
	got := (&Backend{}).Extract(path)
	if got != "last" {
		t.Errorf("got %q, want %q", got, "last")
	}
}

func TestExtractResultFromLog_Empty(t *testing.T) {
	path := writeTemp(t, "")
	got := (&Backend{}).Extract(path)
	if got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestExtractResultFromLog_NoAssistant(t *testing.T) {
	log := `{"role":"user","content":"hi"}
{"role":"tool","tool_call_id":"x","content":"done"}
`
	path := writeTemp(t, log)
	got := (&Backend{}).Extract(path)
	if got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestExtractResultFromLog_MissingFile(t *testing.T) {
	got := (&Backend{}).Extract(filepath.Join(t.TempDir(), "nonexistent.jsonl"))
	if got != "" {
		t.Errorf("got %q, want %q", got, "")
	}
}

func TestKimiCollector_Collect_TakesLastStatusUpdate(t *testing.T) {
	home := t.TempDir()
	sessionsDir := filepath.Join(home, ".kimi", "sessions")
	makeKimiSessionFile(t, sessionsDir, "sess-abc", `{"message":{"type":"Other","payload":{}}}
{"message":{"type":"StatusUpdate","payload":{"token_usage":{"input_other":100,"output":50,"input_cache_read":200,"input_cache_creation":10}}}}
{"message":{"type":"StatusUpdate","payload":{"token_usage":{"input_other":446,"output":70,"input_cache_read":16384,"input_cache_creation":0}}}}
`)

	collector := NewBackendAt("", nil, sessionsDir)
	usages, err := collector.Collect(context.Background(), "sess-abc")
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if len(usages) != 1 {
		t.Fatalf("expected 1 usage, got %d", len(usages))
	}
	u := usages[0]
	if u.InputTokens != 446 {
		t.Errorf("InputTokens: want 446, got %d", u.InputTokens)
	}
	if u.OutputTokens != 70 {
		t.Errorf("OutputTokens: want 70, got %d", u.OutputTokens)
	}
	if u.CacheReadTokens != 16384 {
		t.Errorf("CacheReadTokens: want 16384, got %d", u.CacheReadTokens)
	}
	if u.CacheCreationTokens != 0 {
		t.Errorf("CacheCreationTokens: want 0, got %d", u.CacheCreationTokens)
	}
	if u.Model != "kimi" {
		t.Errorf("Model: want kimi, got %s", u.Model)
	}
}

func TestKimiCollector_Collect_FileNotFound(t *testing.T) {
	home := t.TempDir()
	sessionsDir := filepath.Join(home, ".kimi", "sessions")

	collector := NewBackendAt("", nil, sessionsDir)
	_, err := collector.Collect(context.Background(), "nonexistent-session")
	if !errors.Is(err, ai.ErrSessionDataNotFound) {
		t.Fatalf("expected ErrSessionDataNotFound, got %v", err)
	}
}

func TestKimiCollector_Collect_NoStatusUpdate(t *testing.T) {
	home := t.TempDir()
	sessionsDir := filepath.Join(home, ".kimi", "sessions")
	makeKimiSessionFile(t, sessionsDir, "sess-empty", `{"message":{"type":"Other","payload":{}}}
{"message":{"type":"Progress","payload":{}}}
`)

	collector := NewBackendAt("", nil, sessionsDir)
	_, err := collector.Collect(context.Background(), "sess-empty")
	if !errors.Is(err, ai.ErrSessionDataNotFound) {
		t.Fatalf("expected ErrSessionDataNotFound, got %v", err)
	}
}

func TestKimiCollector_Collect_ZeroTokens(t *testing.T) {
	home := t.TempDir()
	sessionsDir := filepath.Join(home, ".kimi", "sessions")
	makeKimiSessionFile(t, sessionsDir, "sess-zero", `{"message":{"type":"StatusUpdate","payload":{"token_usage":{"input_other":0,"output":0,"input_cache_read":0,"input_cache_creation":0}}}}
`)

	collector := NewBackendAt("", nil, sessionsDir)
	usages, err := collector.Collect(context.Background(), "sess-zero")
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if len(usages) != 1 {
		t.Fatalf("expected 1 usage, got %d", len(usages))
	}
	u := usages[0]
	if u.InputTokens != 0 || u.OutputTokens != 0 {
		t.Errorf("expected all zero tokens, got %+v", u)
	}
}
```

- [ ] **Step 3: Delete the old files**

```bash
rm internal/ai/engine/kimi/invoker_test.go internal/ai/engine/kimi/token_usage_test.go
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/ai/engine/kimi/...`
Expected: PASS (all tests previously in invoker_test.go and token_usage_test.go).

- [ ] **Step 5: Commit**

```bash
git add internal/ai/engine/kimi/
git commit -m "refactor(ai/kimi): merge engine tests into backend_test.go

Collapse invoker_test.go + token_usage_test.go into a single internal-
package backend_test.go aligned with the post-refactor backend.go source
file."
```

---

## Task 2: Pi — merge invoker_test + token_usage_test, rename bench

**Files:**
- Delete: `internal/ai/engine/pi/invoker_test.go`
- Delete: `internal/ai/engine/pi/token_usage_test.go`
- Rename: `internal/ai/engine/pi/invoker_bench_test.go` → `internal/ai/engine/pi/backend_bench_test.go`
- Create: `internal/ai/engine/pi/backend_test.go`

- [ ] **Step 1: Confirm baseline (read current files)**

- `internal/ai/engine/pi/invoker_test.go` — `package pi` (internal), helper `writeTemp`, tests for `buildArgs`, `Backend.Extract`, `sessionFilePath`, `Run`, `isStreamingDelta`, `checkAgentError`, `extractPiError`.
- `internal/ai/engine/pi/token_usage_test.go` — `package pi_test` (external), helper `writePiTempFile`, tests for `Collect`.
- `internal/ai/engine/pi/invoker_bench_test.go` — `package pi` (internal), benchmarks `stripThinkingSignature`. Content does not need to change, only the filename.

- [ ] **Step 2: Rename the bench file**

```bash
git mv internal/ai/engine/pi/invoker_bench_test.go internal/ai/engine/pi/backend_bench_test.go
```

- [ ] **Step 3: Create the merged `backend_test.go`**

Create `internal/ai/engine/pi/backend_test.go`:

```go
package pi

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	ai "github.com/theopenbee/openbee/internal/ai"
)

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "pi-log-*.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(content); err != nil {
		t.Fatal(err)
	}
	return f.Name()
}

func writePiTempFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
}

func TestBuildArgs_NewSession(t *testing.T) {
	args := buildArgs("", "/tmp/session.jsonl", nil)
	want := []string{"--mode", "json", "--session", "/tmp/session.jsonl", "-p", ""}
	if !slices.Equal(args, want) {
		t.Errorf("got %v, want %v", args, want)
	}
}

func TestBuildArgs_WithPrompt(t *testing.T) {
	args := buildArgs("hello world", "/tmp/session.jsonl", nil)
	want := []string{"--mode", "json", "--session", "/tmp/session.jsonl", "-p", "hello world"}
	if !slices.Equal(args, want) {
		t.Errorf("got %v, want %v", args, want)
	}
}

func TestExtractResultFromLog_BasicResult(t *testing.T) {
	log := `{"type":"turn.started"}
{"type":"agent_end","messages":[{"role":"assistant","content":[{"type":"text","text":"final answer"}]}]}
`
	path := writeTemp(t, log)
	result := (&Backend{}).Extract(path)
	if result != "final answer" {
		t.Errorf("got %q, want %q", result, "final answer")
	}
}

func TestExtractResultFromLog_LastAgentEndWins(t *testing.T) {
	log := `{"type":"agent_end","messages":[{"role":"assistant","content":[{"type":"text","text":"first"}]}]}
{"type":"agent_end","messages":[{"role":"assistant","content":[{"type":"text","text":"last"}]}]}
`
	path := writeTemp(t, log)
	result := (&Backend{}).Extract(path)
	if result != "last" {
		t.Errorf("got %q, want %q", result, "last")
	}
}

func TestExtractResultFromLog_SkipsNonTextContent(t *testing.T) {
	log := `{"type":"agent_end","messages":[{"role":"assistant","content":[{"type":"tool_use","id":"x"},{"type":"text","text":"answer"}]}]}
`
	path := writeTemp(t, log)
	result := (&Backend{}).Extract(path)
	if result != "answer" {
		t.Errorf("got %q, want %q", result, "answer")
	}
}

func TestExtractResultFromLog_Empty(t *testing.T) {
	path := writeTemp(t, "")
	result := (&Backend{}).Extract(path)
	if result != "" {
		t.Errorf("got %q, want %q", result, "")
	}
}

func TestExtractResultFromLog_NoAgentEnd(t *testing.T) {
	path := writeTemp(t, `{"type":"turn.started"}`+"\n")
	result := (&Backend{}).Extract(path)
	if result != "" {
		t.Errorf("got %q, want %q", result, "")
	}
}

func TestResolveSessionPath_UsesUUID(t *testing.T) {
	t.Setenv("OPENBEE_URL", "http://localhost:8080")
	sessionID := "4d0ce91b-0856-44e2-b0d7-7765d824bba3"
	b, err := NewBackend("true", nil)
	if err != nil {
		t.Fatalf("NewBackend: %v", err)
	}
	got := b.sessionFilePath(sessionID)
	home, _ := os.UserHomeDir()
	want := filepath.Join(home, ".openbee", ".pi", "sessions", sessionID+".jsonl")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestInvoker_Run_ExitsCleanly(t *testing.T) {
	t.Setenv("OPENBEE_URL", "http://localhost:8080")
	b, err := NewBackend("true", nil)
	if err != nil {
		t.Fatalf("NewBackend: %v", err)
	}
	logPath := filepath.Join(t.TempDir(), "pi.log")

	_, ch, err := b.Run(context.Background(), t.TempDir(), "hello",
		ai.RunOptions{SessionID: "4d0ce91b-0856-44e2-b0d7-7765d824bba3"}, logPath)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	for range ch {
	}
}

func TestIsStreamingDelta(t *testing.T) {
	tests := []struct {
		name string
		line string
		want bool
	}{
		{"message_update filtered", `{"type":"message_update","message":{}}`, true},
		{"tool_execution_update filtered", `{"type":"tool_execution_update","toolCallId":"x","partialResult":{"content":[]}}`, true},
		{"tool_execution_start not filtered", `{"type":"tool_execution_start","toolCallId":"x","toolName":"bash","args":{}}`, false},
		{"tool_execution_end not filtered", `{"type":"tool_execution_end","toolCallId":"x","result":{"content":[]}}`, false},
		{"message_end not filtered", `{"type":"message_end","message":{"role":"assistant","content":[]}}`, false},
		{"plain JSON not filtered", `{"type":"agent_start"}`, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := isStreamingDelta([]byte(tc.line)); got != tc.want {
				t.Errorf("isStreamingDelta(%q) = %v, want %v", tc.line, got, tc.want)
			}
		})
	}
}

func TestCheckAgentError_AuthError(t *testing.T) {
	log := `{"type":"session","version":3,"id":"abc","timestamp":"2026-04-15T03:36:31.419Z","cwd":"/tmp"}
{"type":"agent_end","messages":[{"role":"user","content":[{"type":"text","text":"hello"}]},{"role":"assistant","content":[],"stopReason":"error","errorMessage":"401 {\"error\":{\"type\":\"authentication_error\",\"message\":\"The API Key appears to be invalid or may have expired.\"}}"}]}
`
	path := writeTemp(t, log)
	got := checkAgentError(path)
	want := `401 {"error":{"type":"authentication_error","message":"The API Key appears to be invalid or may have expired."}}`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestCheckAgentError_NoError(t *testing.T) {
	log := `{"type":"agent_end","messages":[{"role":"assistant","content":[{"type":"text","text":"hello"}],"stopReason":"stop"}]}
`
	path := writeTemp(t, log)
	got := checkAgentError(path)
	if got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestCheckAgentError_EmptyLog(t *testing.T) {
	path := writeTemp(t, "")
	got := checkAgentError(path)
	if got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestExtractPiError_NonJSONLine(t *testing.T) {
	stderr := `{"type":"session","version":3}
No API key found for unknown.
Use /login or set an API key environment variable.`
	got := extractPiError(stderr, "exit status 1")
	want := "No API key found for unknown."
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestExtractPiError_FallbackOnAllJSON(t *testing.T) {
	stderr := `{"type":"session","version":3}`
	got := extractPiError(stderr, "exit status 1")
	if got != "exit status 1" {
		t.Errorf("got %q, want %q", got, "exit status 1")
	}
}

func TestExtractPiError_EmptyStderr(t *testing.T) {
	got := extractPiError("", "exit status 1")
	if got != "exit status 1" {
		t.Errorf("got %q, want %q", got, "exit status 1")
	}
}

func TestPiCollector_Collect_AggregatesByModel(t *testing.T) {
	sessionsDir := t.TempDir()
	sessionID := "pi-sess-abc123"

	writePiTempFile(t, sessionsDir, "20250101_"+sessionID+".jsonl", `{"type":"message","message":{"role":"assistant","model":"claude-3-5-sonnet","usage":{"input":100,"output":50,"cacheWrite":10,"cacheRead":5}}}
{"type":"message","message":{"role":"user","content":"hello"}}
{"type":"message","message":{"role":"assistant","model":"claude-3-5-sonnet","usage":{"input":200,"output":80,"cacheWrite":0,"cacheRead":15}}}
{"type":"message","message":{"role":"assistant","model":"claude-3-opus","usage":{"input":300,"output":100,"cacheWrite":5,"cacheRead":0}}}
{"type":"other","message":{"role":"assistant","model":"claude-3-5-sonnet","usage":{"input":999,"output":999}}}
`)
	t.Setenv("PI_AGENT_DIR", sessionsDir)
	collector := NewBackendAt("", nil, sessionsDir)

	usages, err := collector.Collect(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	byModel := map[string]ai.TokenUsage{}
	for _, u := range usages {
		byModel[u.Model] = u
	}

	sonnet := byModel["claude-3-5-sonnet"]
	if sonnet.InputTokens != 300 {
		t.Errorf("sonnet InputTokens: want 300, got %d", sonnet.InputTokens)
	}
	if sonnet.OutputTokens != 130 {
		t.Errorf("sonnet OutputTokens: want 130, got %d", sonnet.OutputTokens)
	}
	if sonnet.CacheCreationTokens != 10 {
		t.Errorf("sonnet CacheCreationTokens: want 10, got %d", sonnet.CacheCreationTokens)
	}
	if sonnet.CacheReadTokens != 20 {
		t.Errorf("sonnet CacheReadTokens: want 20, got %d", sonnet.CacheReadTokens)
	}

	opus := byModel["claude-3-opus"]
	if opus.InputTokens != 300 {
		t.Errorf("opus InputTokens: want 300, got %d", opus.InputTokens)
	}
}

func TestPiCollector_Collect_SkipsNonAssistantAndWrongType(t *testing.T) {
	sessionsDir := t.TempDir()
	sessionID := "skip-test"

	writePiTempFile(t, sessionsDir, sessionID+".jsonl", `{"type":"message","message":{"role":"user","model":"claude-3-5-sonnet","usage":{"input":999,"output":999}}}
{"type":"message","message":{"role":"assistant","model":"claude-3-5-sonnet","usage":{"input":100,"output":50}}}
{"type":"other","message":{"role":"assistant","model":"claude-3-5-sonnet","usage":{"input":999,"output":999}}}
`)
	t.Setenv("PI_AGENT_DIR", sessionsDir)

	usages, err := NewBackendAt("", nil, sessionsDir).Collect(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if len(usages) != 1 {
		t.Fatalf("expected 1 usage (only assistant+message), got %d", len(usages))
	}
	if usages[0].InputTokens != 100 {
		t.Errorf("InputTokens: want 100, got %d", usages[0].InputTokens)
	}
}

func TestPiCollector_Collect_FileNotFound(t *testing.T) {
	dir := t.TempDir()
	_, err := NewBackendAt("", nil, dir).Collect(context.Background(), "nonexistent-session")
	if !errors.Is(err, ai.ErrSessionDataNotFound) {
		t.Fatalf("expected ErrSessionDataNotFound, got %v", err)
	}
}

func TestPiCollector_Collect_DirectoryNotFound(t *testing.T) {
	_, err := NewBackendAt("", nil, t.TempDir()+"/missing").Collect(context.Background(), "nonexistent-session")
	if !errors.Is(err, ai.ErrSessionDataNotFound) {
		t.Fatalf("expected ErrSessionDataNotFound, got %v", err)
	}
}

func TestPiCollector_Collect_UsesOpenbeeDefaultSessionsDir(t *testing.T) {
	home := t.TempDir()
	sessionID := "default-dir-session"
	t.Setenv("HOME", home)
	t.Setenv("PI_AGENT_DIR", "")

	writePiTempFile(t, home, ".openbee/.pi/sessions/"+sessionID+".jsonl", `{"type":"message","message":{"role":"assistant","model":"claude-3-5-sonnet","usage":{"input":100,"output":50}}}`)

	sessionsDir := home + "/.openbee/.pi/sessions"
	usages, err := NewBackendAt("", nil, sessionsDir).Collect(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if len(usages) != 1 {
		t.Fatalf("expected 1 usage, got %d", len(usages))
	}
	if usages[0].InputTokens != 100 || usages[0].OutputTokens != 50 {
		t.Fatalf("unexpected usage: %+v", usages[0])
	}
}
```

- [ ] **Step 4: Delete the old test files**

```bash
rm internal/ai/engine/pi/invoker_test.go internal/ai/engine/pi/token_usage_test.go
```

- [ ] **Step 5: Run tests and benchmarks**

```bash
go test ./internal/ai/engine/pi/...
go test -bench=. -benchtime=1x ./internal/ai/engine/pi/...
```
Expected: All tests PASS; all three `BenchmarkStripThinkingSignature_*` benchmarks execute.

- [ ] **Step 6: Commit**

```bash
git add internal/ai/engine/pi/
git commit -m "refactor(ai/pi): merge engine tests into backend_test.go

Collapse invoker_test.go + token_usage_test.go into a single internal-
package backend_test.go and rename invoker_bench_test.go to
backend_bench_test.go to match the post-refactor backend.go source file."
```

---

## Task 3: Codex — merge invoker_test + token_usage_test, move interface assertion, drop adapter_test

**Files:**
- Modify: `internal/ai/engine/codex/adapter.go` (add compile-time interface assertion)
- Delete: `internal/ai/engine/codex/adapter_test.go`
- Delete: `internal/ai/engine/codex/invoker_test.go`
- Delete: `internal/ai/engine/codex/token_usage_test.go`
- Create: `internal/ai/engine/codex/backend_test.go`

- [ ] **Step 1: Confirm baseline**

- `internal/ai/engine/codex/adapter.go` — `NewAdapter` returns `ai.EngineAdapter`; the type returned is `*core.EngineAdapter` wrapped, so the compile-time assertion must target the concrete type returned by `NewBackend`, not `Adapter` (which doesn't exist as a named type). Read the file once more before editing.
- `internal/ai/engine/codex/invoker_test.go` — `package codex` (internal), tests `buildArgs`, `extractSessionID`, `Backend.Extract`.
- `internal/ai/engine/codex/token_usage_test.go` — `package codex_test` (external), helper `writeCodexTempFile`, tests `Collect`.
- `internal/ai/engine/codex/adapter_test.go` — `package codex_test` (external), 18 lines: constructs adapter with extra env and does a `var _ ai.EngineAdapter = a` type assertion. We are dropping this test entirely; the type assertion will move into the source file.

Run: `cat internal/ai/engine/codex/adapter.go`
Expected: see the existing `NewAdapter` implementation; no existing `var _` assertion at package level.

- [ ] **Step 2: Add compile-time assertion in `adapter.go`**

The existing `NewAdapter` returns `ai.EngineAdapter` already, so this is guaranteed at the function-signature level. To preserve the original test's intent (catching accidental signature changes that lose the interface), add a package-level assertion that the value `NewAdapter` constructs internally — `core.NewEngineAdapter(NewBackend(...))` — satisfies `ai.EngineAdapter`. The cleanest expression is to assert the return type of a no-op call, but Go doesn't allow that at package scope. Instead, since `core.NewEngineAdapter` already returns `ai.EngineAdapter`, the existing function signature is the assertion. **No change needed to `adapter.go` after all — the runtime test was redundant.**

Skip this step; just verify by reading the file that `NewAdapter` already has return type `ai.EngineAdapter`.

Run: `grep "func NewAdapter" internal/ai/engine/codex/adapter.go`
Expected output: `func NewAdapter(binaryPath string, extraEnv map[string]string) (ai.EngineAdapter, error) {`

(The return-type signature is itself a compile-time guarantee. The old runtime test added no value over the signature; it can simply be deleted.)

- [ ] **Step 3: Create the merged `backend_test.go`**

Create `internal/ai/engine/codex/backend_test.go`:

```go
package codex

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/theopenbee/openbee/internal/ai"
)

func writeCodexTempFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("MkdirAll %s: %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile %s/%s: %v", dir, name, err)
	}
}

func TestBuildArgs_NewSession(t *testing.T) {
	args := buildArgs("", false, "", nil)
	want := []string{"exec", "-", "--json", "--dangerously-bypass-approvals-and-sandbox"}
	if !slices.Equal(args, want) {
		t.Errorf("got %v, want %v", args, want)
	}
}

func TestBuildArgs_ResumeWithID(t *testing.T) {
	args := buildArgs("sess-123", true, "", nil)
	want := []string{"exec", "resume", "sess-123", "--json", "--dangerously-bypass-approvals-and-sandbox"}
	if !slices.Equal(args, want) {
		t.Errorf("got %v, want %v", args, want)
	}
}

func TestBuildArgs_ResumeWithIDAndPrompt(t *testing.T) {
	args := buildArgs("sess-123", true, "do something", nil)
	want := []string{"exec", "resume", "sess-123", "--json", "--dangerously-bypass-approvals-and-sandbox", "do something"}
	if !slices.Equal(args, want) {
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

func TestBuildArgs_ResumeUsesThreadID(t *testing.T) {
	args := buildArgs("thread-xyz-from-store", true, "follow up", nil)
	want := []string{"exec", "resume", "thread-xyz-from-store", "--json", "--dangerously-bypass-approvals-and-sandbox", "follow up"}
	if !slices.Equal(args, want) {
		t.Errorf("got %v, want %v", args, want)
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
	result := (&Backend{}).Extract(tmpFile)
	if result != "hello world" {
		t.Errorf("got %q, want %q", result, "hello world")
	}
}

func TestCodexCollector_Collect_WithLastTokenUsage(t *testing.T) {
	base := t.TempDir()
	mappingDir := filepath.Join(base, "mapping")
	codexBase := filepath.Join(base, "codex")
	os.MkdirAll(mappingDir, 0755)
	os.MkdirAll(filepath.Join(codexBase, "sessions", "2026", "04", "23"), 0755)

	os.WriteFile(filepath.Join(mappingDir, "openbee-sess-1"), []byte("codex-real-sess-1\n"), 0644)
	writeCodexTempFile(t, filepath.Join(codexBase, "sessions", "2026", "04", "23"), "rollout-2026-04-23T01-02-03-codex-real-sess-1.jsonl", `{"type":"turn_context","payload":{"model":"gpt-4o"}}
{"type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":100,"output_tokens":50,"cached_input_tokens":20},"total_token_usage":{"input_tokens":100,"output_tokens":50,"cached_input_tokens":20}}}}
{"type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":200,"output_tokens":80,"cached_input_tokens":10},"total_token_usage":{"input_tokens":300,"output_tokens":130,"cached_input_tokens":30}}}}
{"type":"turn_context","payload":{"model":"o1-mini"}}
{"type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":300,"output_tokens":100,"cached_input_tokens":0},"total_token_usage":{"input_tokens":600,"output_tokens":230,"cached_input_tokens":30}}}}
`)

	collector := NewBackendAt("", nil, nil, mappingDir, codexBase)
	usages, err := collector.Collect(context.Background(), "openbee-sess-1")
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}

	byModel := map[string]ai.TokenUsage{}
	for _, u := range usages {
		byModel[u.Model] = u
	}

	gpt4o := byModel["gpt-4o"]
	if gpt4o.InputTokens != 300 {
		t.Errorf("gpt-4o InputTokens: want 300, got %d", gpt4o.InputTokens)
	}
	if gpt4o.OutputTokens != 130 {
		t.Errorf("gpt-4o OutputTokens: want 130, got %d", gpt4o.OutputTokens)
	}
	if gpt4o.CacheReadTokens != 30 {
		t.Errorf("gpt-4o CacheReadTokens: want 30, got %d", gpt4o.CacheReadTokens)
	}
	if gpt4o.CacheCreationTokens != 0 {
		t.Errorf("gpt-4o CacheCreationTokens: want 0, got %d", gpt4o.CacheCreationTokens)
	}

	o1mini := byModel["o1-mini"]
	if o1mini.InputTokens != 300 {
		t.Errorf("o1-mini InputTokens: want 300, got %d", o1mini.InputTokens)
	}
}

func TestCodexCollector_Collect_DeltaFromTotalTokenUsage(t *testing.T) {
	base := t.TempDir()
	mappingDir := filepath.Join(base, "mapping")
	codexBase := filepath.Join(base, "codex")
	os.MkdirAll(mappingDir, 0755)
	os.MkdirAll(filepath.Join(codexBase, "sessions"), 0755)

	os.WriteFile(filepath.Join(mappingDir, "openbee-sess-2"), []byte("codex-real-sess-2"), 0644)
	writeCodexTempFile(t, filepath.Join(codexBase, "sessions"), "codex-real-sess-2.jsonl", `{"type":"turn_context","payload":{"model":"gpt-4o"}}
{"type":"event_msg","info":{"total_token_usage":{"input_tokens":100,"output_tokens":50,"cached_input_tokens":10}}}
{"type":"event_msg","info":{"total_token_usage":{"input_tokens":250,"output_tokens":120,"cached_input_tokens":25}}}
`)

	collector := NewBackendAt("", nil, nil, mappingDir, codexBase)
	usages, err := collector.Collect(context.Background(), "openbee-sess-2")
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if len(usages) != 1 {
		t.Fatalf("expected 1 usage, got %d", len(usages))
	}
	if usages[0].InputTokens != 250 {
		t.Errorf("InputTokens: want 250, got %d", usages[0].InputTokens)
	}
	if usages[0].OutputTokens != 120 {
		t.Errorf("OutputTokens: want 120, got %d", usages[0].OutputTokens)
	}
}

func TestCodexCollector_Collect_LegacyTopLevelInfo(t *testing.T) {
	base := t.TempDir()
	mappingDir := filepath.Join(base, "mapping")
	codexBase := filepath.Join(base, "codex")
	os.MkdirAll(mappingDir, 0755)
	os.MkdirAll(filepath.Join(codexBase, "sessions"), 0755)

	os.WriteFile(filepath.Join(mappingDir, "openbee-sess-legacy"), []byte("codex-real-sess-legacy"), 0644)
	writeCodexTempFile(t, filepath.Join(codexBase, "sessions"), "codex-real-sess-legacy.jsonl", `{"type":"turn_context","payload":{"model":"gpt-4o"}}
{"type":"event_msg","info":{"last_token_usage":{"input_tokens":100,"output_tokens":50,"cached_input_tokens":10}}}
`)

	collector := NewBackendAt("", nil, nil, mappingDir, codexBase)
	usages, err := collector.Collect(context.Background(), "openbee-sess-legacy")
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if len(usages) != 1 {
		t.Fatalf("expected 1 usage, got %d", len(usages))
	}
	if usages[0].InputTokens != 100 || usages[0].OutputTokens != 50 || usages[0].CacheReadTokens != 10 {
		t.Fatalf("unexpected usage: %+v", usages[0])
	}
}

func TestCodexCollector_Collect_MappingFileNotFound(t *testing.T) {
	mappingDir := t.TempDir()
	codexBase := t.TempDir()
	collector := NewBackendAt("", nil, nil, mappingDir, codexBase)
	_, err := collector.Collect(context.Background(), "nonexistent-session")
	if err == nil {
		t.Error("expected error for missing mapping file, got nil")
	}
	if !errors.Is(err, ai.ErrSessionDataNotFound) {
		t.Errorf("expected ErrSessionDataNotFound, got: %v", err)
	}
}
```

- [ ] **Step 4: Delete the obsolete test files**

```bash
rm internal/ai/engine/codex/invoker_test.go \
   internal/ai/engine/codex/token_usage_test.go \
   internal/ai/engine/codex/adapter_test.go
```

- [ ] **Step 5: Run tests**

Run: `go test ./internal/ai/engine/codex/...`
Expected: All tests PASS (including `session_store_test.go`).

- [ ] **Step 6: Commit**

```bash
git add internal/ai/engine/codex/
git commit -m "refactor(ai/codex): merge engine tests into backend_test.go

Collapse invoker_test.go + token_usage_test.go into a single internal-
package backend_test.go and drop adapter_test.go — its runtime type
assertion was redundant with the NewAdapter signature."
```

---

## Task 4: Claude — merge invoker_test + token_usage_test + cleanupLegacyRules, rename unix, delete adapter_test

**Files:**
- Delete: `internal/ai/engine/claude/adapter_test.go`
- Delete: `internal/ai/engine/claude/invoker_test.go`
- Delete: `internal/ai/engine/claude/token_usage_test.go`
- Rename: `internal/ai/engine/claude/invoker_unix_test.go` → `internal/ai/engine/claude/backend_unix_test.go`
- Create: `internal/ai/engine/claude/backend_test.go`
- Keep: `internal/ai/engine/claude/download_test.go`, `internal/ai/engine/claude/provider_test.go` unchanged

- [ ] **Step 1: Confirm baseline**

- `invoker_test.go` — `package claude` internal; has `TestMain` for the `GO_TEST_EMIT_IS_ERROR=1` subprocess mode (critical — `TestBackend_Run_IsErrorEmitsOutputError` depends on it).
- `invoker_unix_test.go` — `package claude`, `//go:build !windows`. Only the filename changes.
- `adapter_test.go` — `package claude`, tests `cleanupLegacyRules` (which lives in `backend.go`). The case `TestCleanupLegacyRules_Stub` is redundant with `TestCleanupLegacyRules_NoopWhenFilesAbsent`; drop it during the move.
- `token_usage_test.go` — `package claude_test` external, helper `writeClaudeTempFile`, tests `Collect`. Switching to internal package and inlining is straightforward.

- [ ] **Step 2: Rename the unix file (filename only)**

```bash
git mv internal/ai/engine/claude/invoker_unix_test.go internal/ai/engine/claude/backend_unix_test.go
```

Verify the build tag and package line are still intact:

Run: `head -3 internal/ai/engine/claude/backend_unix_test.go`
Expected: `//go:build !windows` followed by blank line and `package claude`.

- [ ] **Step 3: Create the merged `backend_test.go`**

Create `internal/ai/engine/claude/backend_test.go`:

```go
package claude

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	ai "github.com/theopenbee/openbee/internal/ai"
)

func TestMain(m *testing.M) {
	// Subprocess helper: when GO_TEST_EMIT_IS_ERROR=1, write an is_error result
	// event to stdout and exit immediately without running any tests.
	if os.Getenv("GO_TEST_EMIT_IS_ERROR") == "1" {
		os.Stdout.WriteString(`{"type":"result","is_error":true,"result":"API Error: 400 {}"}` + "\n")
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func writeClaudeTempFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
}

func TestNewBackend(t *testing.T) {
	t.Setenv("OPENBEE_URL", "http://localhost:8080")
	b := NewBackend("/usr/bin/claude", nil)
	if b.binary != "/usr/bin/claude" {
		t.Errorf("binary: want /usr/bin/claude, got %s", b.binary)
	}
	wantURL := "OPENBEE_URL=http://localhost:8080"
	if !slices.Contains(b.baseEnv, wantURL) {
		t.Errorf("baseEnv missing %s", wantURL)
	}
}

func TestBackend_Run_WritesOutputToFile(t *testing.T) {
	inv := NewBackend("echo", nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	logPath := filepath.Join(t.TempDir(), "test.log")
	proc, ch, err := inv.Run(ctx, t.TempDir(), "hello", ai.RunOptions{}, logPath)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if proc.PID() == 0 {
		t.Error("expected non-zero PID")
	}

	var gotDone bool
	for out := range ch {
		if out.Type == ai.OutputDone {
			gotDone = true
		}
	}
	if !gotDone {
		t.Error("expected done signal")
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	if len(data) == 0 {
		t.Error("expected non-empty log file after echo")
	}
}

func TestBackend_Run_SessionFlags(t *testing.T) {
	inv := NewBackend("echo", nil)
	ctx := context.Background()

	logPath1 := filepath.Join(t.TempDir(), "s1.log")
	_, ch, _ := inv.Run(ctx, t.TempDir(), "test", ai.RunOptions{SessionID: "s1"}, logPath1)
	for range ch {
	}
	data, _ := os.ReadFile(logPath1)
	output := string(data)
	if !strings.Contains(output, "--session-id") || !strings.Contains(output, "s1") {
		t.Errorf("expected --session-id s1 in log file, got: %s", output)
	}

	logPath2 := filepath.Join(t.TempDir(), "s2.log")
	_, ch2, _ := inv.Run(ctx, t.TempDir(), "test", ai.RunOptions{SessionID: "s2", Resume: true}, logPath2)
	for range ch2 {
	}
	data2, _ := os.ReadFile(logPath2)
	output2 := string(data2)
	if !strings.Contains(output2, "--resume") || !strings.Contains(output2, "s2") {
		t.Errorf("expected --resume s2 in log file, got: %s", output2)
	}
}

func TestProcess_Stop(t *testing.T) {
	inv := NewBackend("sleep", nil)
	ctx := context.Background()

	logPath := filepath.Join(t.TempDir(), "stop.log")
	proc, ch, err := inv.Run(ctx, t.TempDir(), "60", ai.RunOptions{}, logPath)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if err := proc.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	for range ch {
	}
}

func TestBackend_ConcurrentRuns(t *testing.T) {
	inv := NewBackend("echo", nil)
	ctx := context.Background()

	logPath1 := filepath.Join(t.TempDir(), "one.log")
	logPath2 := filepath.Join(t.TempDir(), "two.log")

	proc1, ch1, err1 := inv.Run(ctx, t.TempDir(), "one", ai.RunOptions{SessionID: "s1"}, logPath1)
	if err1 != nil {
		t.Fatalf("Run 1: %v", err1)
	}
	proc2, ch2, err2 := inv.Run(ctx, t.TempDir(), "two", ai.RunOptions{SessionID: "s2"}, logPath2)
	if err2 != nil {
		t.Fatalf("Run 2: %v", err2)
	}

	if proc1.PID() == proc2.PID() {
		t.Error("concurrent runs should have different PIDs")
	}

	for range ch1 {
	}
	for range ch2 {
	}
}

func TestBackend_Run_IsErrorEmitsOutputError(t *testing.T) {
	// Run the test binary itself as the subprocess. TestMain detects
	// GO_TEST_EMIT_IS_ERROR=1, writes is_error JSON to stdout, and exits 0.
	// BuildBaseEnv inherits os.Environ(), so t.Setenv propagates to the child.
	t.Setenv("GO_TEST_EMIT_IS_ERROR", "1")

	inv := NewBackend(os.Args[0], nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	dir := t.TempDir()
	logPath := filepath.Join(dir, "run.log")
	_, ch, err := inv.Run(ctx, dir, "", ai.RunOptions{}, logPath)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	var gotError bool
	var errorContent string
	for out := range ch {
		if out.Type == ai.OutputError {
			gotError = true
			errorContent = out.Content
		}
	}
	if !gotError {
		t.Error("want OutputError, got none")
	}
	if !strings.Contains(errorContent, "API Error: 400") {
		t.Errorf("want error content to contain 'API Error: 400', got %q", errorContent)
	}
}

func TestScanResultLog_IsError(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "run.log")
	content := `{"type":"result","is_error":true,"result":"API Error: 400 {\"error\":\"操作失败\"}"}` + "\n"
	if err := os.WriteFile(logPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	result, isError, _ := scanResultLog(logPath)
	if !isError {
		t.Error("want isError=true, got false")
	}
	if result != `API Error: 400 {"error":"操作失败"}` {
		t.Errorf("want API Error string, got %q", result)
	}
}

func TestScanResultLog_NoError(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "run.log")
	content := `{"type":"result","is_error":false,"result":"all good"}` + "\n"
	if err := os.WriteFile(logPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	result, isError, _ := scanResultLog(logPath)
	if isError {
		t.Error("want isError=false, got true")
	}
	if result != "all good" {
		t.Errorf("want 'all good', got %q", result)
	}
}

func TestScanResultLog_NoResultEvent(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "run.log")
	content := `{"type":"assistant","message":{"content":[{"type":"text","text":"hello"}]}}` + "\n"
	if err := os.WriteFile(logPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	result, isError, _ := scanResultLog(logPath)
	if isError {
		t.Error("want isError=false when no result event, got true")
	}
	if result != "" {
		t.Errorf("want empty result, got %q", result)
	}
}

func TestCleanupLegacyRules_DeletesOpenbeeFile(t *testing.T) {
	dir := t.TempDir()
	openbeeFile := filepath.Join(dir, systemRulesFile)
	if err := os.WriteFile(openbeeFile, []byte("old rules"), 0o644); err != nil {
		t.Fatalf("write .openbee.md: %v", err)
	}

	if err := cleanupLegacyRules(dir); err != nil {
		t.Fatalf("cleanupLegacyRules: %v", err)
	}

	if _, err := os.Stat(openbeeFile); !os.IsNotExist(err) {
		t.Error(".openbee.md should have been deleted")
	}
}

func TestCleanupLegacyRules_RemovesImportLine(t *testing.T) {
	dir := t.TempDir()
	claudeFile := filepath.Join(dir, "CLAUDE.md")
	content := "# My Bot\n" + importLine + "\nOther content\n"
	if err := os.WriteFile(claudeFile, []byte(content), 0o644); err != nil {
		t.Fatalf("write CLAUDE.md: %v", err)
	}

	if err := cleanupLegacyRules(dir); err != nil {
		t.Fatalf("cleanupLegacyRules: %v", err)
	}

	data, _ := os.ReadFile(claudeFile)
	got := string(data)
	if strings.Contains(got, importLine) {
		t.Errorf("CLAUDE.md should not contain import line, got:\n%s", got)
	}
	if !strings.Contains(got, "# My Bot") {
		t.Error("CLAUDE.md should preserve other content")
	}
	if !strings.Contains(got, "Other content") {
		t.Error("CLAUDE.md should preserve other content")
	}
}

func TestCleanupLegacyRules_PreservesOtherCLAUDEMDContent(t *testing.T) {
	dir := t.TempDir()
	claudeFile := filepath.Join(dir, "CLAUDE.md")
	original := "# Custom instructions\nDo something special.\n"
	if err := os.WriteFile(claudeFile, []byte(original), 0o644); err != nil {
		t.Fatalf("write CLAUDE.md: %v", err)
	}

	if err := cleanupLegacyRules(dir); err != nil {
		t.Fatalf("cleanupLegacyRules: %v", err)
	}

	data, _ := os.ReadFile(claudeFile)
	if string(data) != original {
		t.Errorf("CLAUDE.md should be unchanged when import line is absent.\nGot: %q\nWant: %q", string(data), original)
	}
}

func TestCleanupLegacyRules_NoopWhenFilesAbsent(t *testing.T) {
	dir := t.TempDir()
	if err := cleanupLegacyRules(dir); err != nil {
		t.Fatalf("cleanupLegacyRules should not error when no files exist: %v", err)
	}
}

func TestCleanupLegacyRules_BothLegacyFilesPresent(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, systemRulesFile), []byte("rules"), 0o644)
	os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte(importLine+"\n"), 0o644)

	if err := cleanupLegacyRules(dir); err != nil {
		t.Fatalf("cleanupLegacyRules: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, systemRulesFile)); !os.IsNotExist(err) {
		t.Errorf(".openbee.md should be deleted")
	}
}

func TestClaudeCollector_Collect_AggregatesByModel(t *testing.T) {
	base := t.TempDir()
	writeClaudeTempFile(t, base, "projects/project-a/test-session.jsonl", `{"message":{"model":"claude-3-5-sonnet","usage":{"input_tokens":100,"output_tokens":50,"cache_creation_input_tokens":20,"cache_read_input_tokens":10}}}
{"message":{"model":"claude-3-5-sonnet","usage":{"input_tokens":200,"output_tokens":100,"cache_creation_input_tokens":0,"cache_read_input_tokens":5}}}
{"message":{"model":"claude-3-opus","usage":{"input_tokens":300,"output_tokens":150}}}
{"timestamp":"2025-01-01T00:00:00Z"}
`)
	t.Setenv("CLAUDE_CONFIG_DIR", base)
	collector := NewBackend("", nil)

	usages, err := collector.Collect(context.Background(), "test-session")
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}

	byModel := map[string]ai.TokenUsage{}
	for _, u := range usages {
		byModel[u.Model] = u
	}

	sonnet := byModel["claude-3-5-sonnet"]
	if sonnet.InputTokens != 300 {
		t.Errorf("sonnet InputTokens: want 300, got %d", sonnet.InputTokens)
	}
	if sonnet.OutputTokens != 150 {
		t.Errorf("sonnet OutputTokens: want 150, got %d", sonnet.OutputTokens)
	}
	if sonnet.CacheCreationTokens != 20 {
		t.Errorf("sonnet CacheCreationTokens: want 20, got %d", sonnet.CacheCreationTokens)
	}
	if sonnet.CacheReadTokens != 15 {
		t.Errorf("sonnet CacheReadTokens: want 15, got %d", sonnet.CacheReadTokens)
	}

	opus := byModel["claude-3-opus"]
	if opus.InputTokens != 300 {
		t.Errorf("opus InputTokens: want 300, got %d", opus.InputTokens)
	}
}

func TestClaudeCollector_Collect_FastSpeedSuffix(t *testing.T) {
	base := t.TempDir()
	writeClaudeTempFile(t, base, "projects/project-a/fast-session.jsonl",
		`{"message":{"model":"claude-3-5-sonnet","speed":"fast","usage":{"input_tokens":100,"output_tokens":50}}}`+"\n")
	t.Setenv("CLAUDE_CONFIG_DIR", base)

	usages, err := NewBackend("", nil).Collect(context.Background(), "fast-session")
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if len(usages) != 1 {
		t.Fatalf("expected 1 usage, got %d", len(usages))
	}
	if usages[0].Model != "claude-3-5-sonnet-fast" {
		t.Errorf("Model: want claude-3-5-sonnet-fast, got %s", usages[0].Model)
	}
}

func TestClaudeCollector_Collect_SkipsSyntheticModel(t *testing.T) {
	base := t.TempDir()
	writeClaudeTempFile(t, base, "projects/project-a/syn-session.jsonl",
		`{"message":{"model":"<synthetic>","usage":{"input_tokens":0,"output_tokens":0,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}}`+"\n"+
			`{"message":{"model":"claude-3-5-sonnet","usage":{"input_tokens":100,"output_tokens":50}}}`+"\n")
	t.Setenv("CLAUDE_CONFIG_DIR", base)

	usages, err := NewBackend("", nil).Collect(context.Background(), "syn-session")
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if len(usages) != 1 {
		t.Fatalf("expected 1 usage, got %d", len(usages))
	}
	if usages[0].Model != "claude-3-5-sonnet" {
		t.Errorf("Model: want claude-3-5-sonnet, got %s", usages[0].Model)
	}
}

func TestClaudeCollector_Collect_FileNotFound(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())
	_, err := NewBackend("", nil).Collect(context.Background(), "nonexistent-session")
	if err == nil {
		t.Error("expected error for missing file, got nil")
	}
	if !errors.Is(err, ai.ErrSessionDataNotFound) {
		t.Errorf("expected ErrSessionDataNotFound, got: %v", err)
	}
}
```

Note: `TestCleanupLegacyRules_Stub` from the original `adapter_test.go` is intentionally omitted — it was redundant with `TestCleanupLegacyRules_NoopWhenFilesAbsent`.

- [ ] **Step 4: Delete the obsolete test files**

```bash
rm internal/ai/engine/claude/invoker_test.go \
   internal/ai/engine/claude/token_usage_test.go \
   internal/ai/engine/claude/adapter_test.go
```

- [ ] **Step 5: Run tests**

```bash
go test ./internal/ai/engine/claude/...
```
Expected: All tests PASS. The cross-file `download_test.go` and `provider_test.go` should be unaffected.

- [ ] **Step 6: Commit**

```bash
git add internal/ai/engine/claude/
git commit -m "refactor(ai/claude): merge engine tests into backend_test.go

Collapse invoker_test.go + token_usage_test.go + cleanupLegacyRules cases
from adapter_test.go into a single internal-package backend_test.go,
rename invoker_unix_test.go to backend_unix_test.go, and drop the
redundant TestCleanupLegacyRules_Stub case."
```

---

## Task 5: Final verification across all four packages

**Files:** None modified.

- [ ] **Step 1: Run the full engine test suite**

Run: `go test ./internal/ai/engine/...`
Expected: All packages PASS.

- [ ] **Step 2: Confirm benchmarks still execute**

Run: `go test -bench=. -benchtime=1x -run='^$' ./internal/ai/engine/pi/...`
Expected: All three `BenchmarkStripThinkingSignature_*` benchmarks run.

- [ ] **Step 3: Verify build**

Run: `go build ./...`
Expected: Clean build with no errors.

- [ ] **Step 4: Verify final file layout**

```bash
ls internal/ai/engine/claude/*_test.go
ls internal/ai/engine/codex/*_test.go
ls internal/ai/engine/pi/*_test.go
ls internal/ai/engine/kimi/*_test.go
```

Expected output:
```
internal/ai/engine/claude/backend_test.go
internal/ai/engine/claude/backend_unix_test.go
internal/ai/engine/claude/download_test.go
internal/ai/engine/claude/provider_test.go
internal/ai/engine/codex/backend_test.go
internal/ai/engine/codex/session_store_test.go
internal/ai/engine/pi/backend_bench_test.go
internal/ai/engine/pi/backend_test.go
internal/ai/engine/kimi/backend_test.go
```

If the layout matches, the reorg is complete. No additional commit needed — each package was committed individually.
