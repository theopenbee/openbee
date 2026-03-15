# Extract Claude CLI Invoker to Shared Package

## Problem

`bee/bee_process.go` and `worker/claude_runtime.go` duplicate Claude CLI subprocess management: MCP config construction, CLI argument building, pipe setup, goroutine-based output draining, and process lifecycle. The only meaningful difference is output destination (log file vs channel).

## Solution

Extract a unified `claude.Invoker` into a new `internal/claude/` package. Both Bee and Worker delegate process management to it and receive output via `<-chan claude.Output`.

## New Package: `internal/claude/`

### File: `invoker.go`

Types:

```go
type OutputType string

const (
    OutputStdout OutputType = "stdout"
    OutputStderr OutputType = "stderr"
    OutputDone   OutputType = "done"
    OutputError  OutputType = "error"
)

type Output struct {
    Type    OutputType `json:"type"`
    Content string     `json:"content"`
}

type RunOptions struct {
    SessionID string
    Resume    bool
}
```

`Invoker` struct:

```go
type Invoker struct {
    binary string
    mcpURL string
    apiKey string
    cmd    *exec.Cmd
    mu     sync.Mutex
}

func NewInvoker(binary, mcpBaseURL, apiKey string) *Invoker
func (inv *Invoker) Run(ctx context.Context, workDir, prompt string, opts RunOptions) (<-chan Output, error)
func (inv *Invoker) Stop() error
func (inv *Invoker) PID() int
```

`Run` responsibilities:
1. Build MCP config JSON string from `mcpURL` and `apiKey`
2. Build CLI args: `--dangerously-skip-permissions`, `--verbose`, `--output-format stream-json`, `--mcp-config`, session flags, `-p prompt`
3. Create stdout/stderr pipes
4. Start process
5. Spawn goroutines to drain pipes into buffered channel (buffer size 100)
6. Scanner buffer: 1MB (`make([]byte, 1024*1024)`) for both stdout and stderr
7. After both scanners complete, call `cmd.Wait()` and send `OutputDone` or `OutputError`
8. Close channel

## Bee Changes

### `bee_process.go`

- `BeeProcess` stores a `*claude.Invoker` internally (constructed in `NewBeeProcess`)
- `Run` signature changes: `Run(ctx, workDir, prompt, sessionID, resume) (<-chan claude.Output, error)`
- `Run` delegates to `Invoker.Run()` and returns the channel directly
- Remove all subprocess management code (MCP config, args, pipes, scanners)
- `WriteCLAUDEMD` and `DefaultPersona` remain unchanged

### `feeder.go`

- `BeeRunner` interface changes: `Run(...) (<-chan claude.Output, error)`
- `processBeeGroup` consumes the channel:
  - Creates log file (same naming: `{sessionID}_{timestamp}.log`)
  - Iterates channel, writes stdout/stderr lines to log file
  - On `OutputDone`: returns nil (success)
  - On `OutputError`: returns error (triggers rollback)

### `feeder_test.go`

- Mock `BeeRunner` returns a pre-populated channel instead of nil error

## Worker Changes

### `claude_runtime.go`

- `ClaudeRuntime` stores a `*claude.Invoker` internally
- `Execute` delegates to `Invoker.Run()`, returns the channel
- Remove all subprocess management code
- `Stop()` and `PID()` delegate to `Invoker`

### `runtime.go`

- Delete `OutputType`, `Output` type definitions — use `claude.Output` and `claude.OutputType`
- `ExecuteOptions` renamed to use `claude.RunOptions` or kept as a local alias
- `Runtime` interface returns `<-chan claude.Output`

### `manager.go`

- Replace all `Output` references with `claude.Output`
- Replace `OutputStdout`, `OutputDone`, etc. with `claude.OutputStdout`, `claude.OutputDone`
- No logic changes

## What Does NOT Change

- Feeder polling, message grouping, session management
- Manager's `monitorExecution`, WebSocket broadcasting, result extraction
- MCP config format
- Log file directory structure
- CLAUDE.md management

## File Summary

| Action | File |
|--------|------|
| Create | `internal/claude/invoker.go` |
| Modify | `internal/bee/bee_process.go` — use Invoker, change return type |
| Modify | `internal/bee/feeder.go` — update BeeRunner interface, consume channel |
| Modify | `internal/bee/feeder_test.go` — update mock |
| Modify | `internal/worker/claude_runtime.go` — use Invoker |
| Modify | `internal/worker/runtime.go` — remove Output types, use claude package |
| Modify | `internal/worker/manager.go` — update type references |
