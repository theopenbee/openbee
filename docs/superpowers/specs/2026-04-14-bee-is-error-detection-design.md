# Bee `is_error` Detection Design

**Date:** 2026-04-14  
**Status:** Approved

## Problem

Claude CLI exits with code 0 even when execution fails at the API level. In these cases the JSON result stream contains `"is_error": true`, but the openbee system never reads that field. Because the feeder only treats a non-zero exit code as failure, API errors such as HTTP 400 pass silently as successes — no failure notification is sent to the user.

Example log entry that currently goes undetected:

```json
{
  "type": "result",
  "subtype": "success",
  "is_error": true,
  "result": "API Error: 400 {\"type\":\"error\",\"error\":{\"message\":\"操作失败\",\"code\":\"500\"}}",
  "stop_reason": "stop_sequence"
}
```

## Goal

Detect all `is_error: true` results from Claude CLI, regardless of exit code, and propagate them as failures so that `failMessages()` is called and the user receives a notification.

## Non-Goals

- Parsing or reformatting the error message content — the raw `result` string is passed through as-is.
- Changing behaviour for non-Claude runners (GLM, etc.).
- Modifying the feeder, task dispatcher, or failure notifier.

## Design

### Affected file

`internal/ai/claude/invoker.go` — only this file changes.

### 1. Extend `streamEvent`

Add `IsError bool` so the existing JSON unmarshalling picks up the field:

```go
type streamEvent struct {
    Type    string         `json:"type"`
    Message *streamMessage `json:"message,omitempty"`
    Result  string         `json:"result,omitempty"`
    IsError bool           `json:"is_error,omitempty"`
}
```

### 2. Add `extractResultStatus`

A focused helper that scans the log file for a `type=result` event and returns the result string plus the `is_error` flag. Does not replace `ExtractResultFromLog` (which is used on the existing error path).

```go
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

### 3. Update the goroutine in `Run()`

After a clean process exit (code 0), check `is_error` before emitting `OutputDone`:

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

## Data Flow

| Scenario | Signal emitted | feeder outcome |
|---|---|---|
| exit non-0 | `OutputError(err)` | `failMessages()` called — unchanged |
| exit 0, `is_error=false` | `OutputDone` | success — unchanged |
| exit 0, `is_error=true` | `OutputError(result)` | `failMessages()` called — **new** |
| exit 0, no result event | `OutputDone` | success — unchanged |

## Race-Condition Analysis

`extractResultStatus` is called only after `cmd.Wait()` returns. `cmd.Wait()` wraps `waitpid()`, which is a happens-before boundary: all user-space buffers in the child are flushed as part of normal exit, and the kernel page cache is coherent across processes on the same host. There is no window where the log file is partially written when we read it after exit 0.

## Testing

- Unit test: feed a mock log containing `{"type":"result","is_error":true,"result":"API Error: 400 ..."}` and assert `OutputError` is emitted.
- Unit test: feed a mock log with `is_error=false` and assert `OutputDone` is emitted.
- Unit test: feed a log with no result event and assert `OutputDone` is emitted.
- Existing invoker tests must continue to pass unchanged.
