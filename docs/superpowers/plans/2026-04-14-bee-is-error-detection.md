# Bee `is_error` Detection Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Detect `is_error: true` in Claude CLI JSON output and propagate it as a failure so users receive a notification even when the process exits with code 0.

**Architecture:** Extend `streamEvent` with the `is_error` field, add a focused `extractResultStatus` helper that reads it from the log, and update the goroutine in `Run()` to emit `OutputError` instead of `OutputDone` when `is_error` is true. The feeder, dispatcher, and notifier are untouched.

**Tech Stack:** Go standard library (`encoding/json`, `os`), existing `ai.ScanJSONLines` utility.

---

### Task 1: Add `IsError` to `streamEvent` and write `extractResultStatus`

**Files:**
- Modify: `internal/ai/claude/invoker.go:25-29` (struct), add function after line 73
- Test: `internal/ai/claude/invoker_test.go`

- [ ] **Step 1: Write the failing tests for `extractResultStatus`**

Add these three test functions to `internal/ai/claude/invoker_test.go`:

```go
func TestExtractResultStatus_IsError(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "run.log")
	content := `{"type":"result","is_error":true,"result":"API Error: 400 {\"error\":\"操作失败\"}"}` + "\n"
	if err := os.WriteFile(logPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	result, isError := extractResultStatus(logPath)
	if !isError {
		t.Error("want isError=true, got false")
	}
	if result != `API Error: 400 {"error":"操作失败"}` {
		t.Errorf("want API Error string, got %q", result)
	}
}

func TestExtractResultStatus_NoError(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "run.log")
	content := `{"type":"result","is_error":false,"result":"all good"}` + "\n"
	if err := os.WriteFile(logPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	result, isError := extractResultStatus(logPath)
	if isError {
		t.Error("want isError=false, got true")
	}
	if result != "all good" {
		t.Errorf("want 'all good', got %q", result)
	}
}

func TestExtractResultStatus_NoResultEvent(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "run.log")
	content := `{"type":"assistant","message":{"content":[{"type":"text","text":"hello"}]}}` + "\n"
	if err := os.WriteFile(logPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	result, isError := extractResultStatus(logPath)
	if isError {
		t.Error("want isError=false when no result event, got true")
	}
	if result != "" {
		t.Errorf("want empty result, got %q", result)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/ai/claude/... -run TestExtractResultStatus -v
```

Expected: `FAIL` — `extractResultStatus` undefined.

- [ ] **Step 3: Add `IsError` to `streamEvent` and implement `extractResultStatus`**

In `internal/ai/claude/invoker.go`, replace the `streamEvent` struct (lines 25-29):

```go
type streamEvent struct {
	Type    string         `json:"type"`
	Message *streamMessage `json:"message,omitempty"`
	Result  string         `json:"result,omitempty"`
	IsError bool           `json:"is_error,omitempty"`
}
```

Add the following function after `ExtractResultFromLog` (after line 73):

```go
// extractResultStatus scans a Claude stream-json log file and returns the
// result string and whether is_error was true in the result event.
func extractResultStatus(logPath string) (result string, isError bool) {
	f, err := os.Open(logPath)
	if err != nil {
		return "", false
	}
	defer f.Close()
	ai.ScanJSONLines(f, func(line string) bool {
		var event streamEvent
		if json.Unmarshal([]byte(line), &event) != nil {
			return true
		}
		if event.Type == "result" {
			result = event.Result
			isError = event.IsError
		}
		return true
	})
	return
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/ai/claude/... -run TestExtractResultStatus -v
```

Expected: all three `TestExtractResultStatus_*` tests `PASS`.

- [ ] **Step 5: Commit**

```bash
git add internal/ai/claude/invoker.go internal/ai/claude/invoker_test.go
git commit -m "feat: add extractResultStatus to detect is_error in Claude log"
```

---

### Task 2: Use `extractResultStatus` in the `Run()` goroutine

**Files:**
- Modify: `internal/ai/claude/invoker.go:111-119` (goroutine)
- Test: `internal/ai/claude/invoker_test.go`

- [ ] **Step 1: Write the failing test**

Add this test to `internal/ai/claude/invoker_test.go`:

```go
func TestInvoker_Run_IsErrorEmitsOutputError(t *testing.T) {
	// Use a shell script that writes an is_error result to stdout then exits 0.
	// 'sh -c' is available on all supported platforms.
	script := `printf '{"type":"result","is_error":true,"result":"API Error: 400 {}"}\n'`
	inv := NewInvoker("sh", "")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	logPath := filepath.Join(t.TempDir(), "run.log")
	_, ch, err := inv.Run(ctx, t.TempDir(), script, ai.RunOptions{}, logPath)
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
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/ai/claude/... -run TestInvoker_Run_IsErrorEmitsOutputError -v
```

Expected: `FAIL` — test expects `OutputError` but current code emits `OutputDone`.

- [ ] **Step 3: Update the goroutine in `Run()`**

In `internal/ai/claude/invoker.go`, replace the goroutine (lines 111-119):

```go
go func() {
	defer close(ch)
	defer logFile.Close()
	if err := cmd.Wait(); err != nil {
		ch <- ai.Output{Type: ai.OutputError, Content: err.Error()}
		return
	}
	result, isError := extractResultStatus(logPath)
	if isError {
		if result == "" {
			result = "bee execution failed with is_error=true"
		}
		ch <- ai.Output{Type: ai.OutputError, Content: result}
		return
	}
	ch <- ai.Output{Type: ai.OutputDone}
}()
```

- [ ] **Step 4: Run all invoker tests**

```bash
go test ./internal/ai/claude/... -v
```

Expected: all tests `PASS`, including the new `TestInvoker_Run_IsErrorEmitsOutputError`.

- [ ] **Step 5: Run the full test suite**

```bash
go test ./...
```

Expected: `ok` for all packages, no failures.

- [ ] **Step 6: Commit**

```bash
git add internal/ai/claude/invoker.go internal/ai/claude/invoker_test.go
git commit -m "feat: emit OutputError when Claude CLI exits 0 with is_error=true"
```
