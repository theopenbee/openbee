# Execution Log Simplification Design

**Date:** 2026-03-25
**Status:** Approved

## Background

The current execution logging mechanism uses three layers:
1. `ActiveLogRegistry` — in-memory buffer for live log streaming during execution
2. `rawLog strings.Builder` — local accumulator in `monitorExecution`/`drainBeeOutput`, flushed to disk on completion
3. Log file (`{logsDir}/YYYY-MM-DD/{execution_id}.log`) — final persistent storage

The goal is to simplify this by redirecting the Claude CLI process stdout/stderr directly to a log file at launch time (OS-level redirect), eliminating the in-memory buffer and `ActiveLogRegistry`.

## Approach

**Path 1: OS-level redirect (modify `claude.Invoker`)**

- Determine log file path before launching the process
- Pass `logPath` to `Invoker.Run()`, which opens the file and sets `cmd.Stdout = logFile`, `cmd.Stderr = logFile`
- Channel carries only lifecycle signals (`OutputDone` / `OutputError`)
- Live log API reads directly from the (incrementally written) file
- After completion, parse log file to extract `result` field (post-processing)

## Execution Flow

```
旧：启动进程 → channel 收集 stdout → 内存缓冲 → 执行完成写文件 → DB 记录 log_path
新：确定 log_path → DB 记录 log_path → 打开文件 → 启动进程（stdout/stderr → 文件）→ 等待完成 → 解析文件提取 result
```

Live log API:
```
旧：先查 ActiveLogRegistry（运行中）→ 若无则读文件
新：直接读文件（文件从启动就在增量写入）
```

## Interface Changes

### `claude.Invoker.Run()`

```go
// Before
func (inv *Invoker) Run(ctx context.Context, workDir, prompt string, opts RunOptions) (*Process, <-chan Output, error)

// After
func (inv *Invoker) Run(ctx context.Context, workDir, prompt string, opts RunOptions, logPath string) (*Process, <-chan Output, error)
```

Internal changes:
- Open `logPath` with `os.OpenFile(logPath, O_CREATE|O_WRONLY|O_TRUNC, 0o644)`
- Set `cmd.Stdout = logFile`, `cmd.Stderr = logFile`
- Remove stdout/stderr pipe setup and two scanning goroutines
- Channel only sends `OutputDone` / `OutputError`

### `bee.BeeRunner` interface

```go
// Before
type BeeRunner interface {
    Run(ctx context.Context, workDir, prompt, sessionID string, resume bool) (*claude.Process, <-chan claude.Output, error)
}

// After
type BeeRunner interface {
    Run(ctx context.Context, workDir, prompt string, opts claude.RunOptions, logPath string) (*claude.Process, <-chan claude.Output, error)
}
```

### `store.ExecutionStore`

- Add `SetLogPath(id string, logPath string) error` — called at launch time to record the path
- Remove `WriteLog()` — file is written by the OS, no Go-layer write needed
- `ReadLog()` unchanged — reads the file at the stored path

## Core Logic Changes

### `worker.Manager.launchRuntime()`

```go
// Determine log path before launch and store in DB
logPath := filepath.Join(s.logsDir, time.Now().Format("2006-01-02"), exec.ID+".log")
os.MkdirAll(filepath.Dir(logPath), 0o755)
m.executionStore.SetLogPath(exec.ID, logPath)

proc, outputCh, err := m.invoker.Run(execCtx, worker.WorkDir, prompt, opts, logPath)
```

### `worker.Manager.monitorExecution()`

Simplified — only handles Done/Error, no stdout accumulation:

```go
for out := range outputCh {
    switch out.Type {
    case claude.OutputDone:
        result := extractResultFromLog(logPath)
        m.executionStore.UpdateResult(exec.ID, result, model.ExecStatusCompleted)
        m.workerStore.UpdateStatus(worker.ID, model.WorkerStatusIdle)
    case claude.OutputError:
        m.executionStore.UpdateResult(exec.ID, out.Content, model.ExecStatusFailed)
        m.workerStore.UpdateStatus(worker.ID, model.WorkerStatusError)
    }
}
```

Remove: `logRegistry` field, `writeLine` parameter.

### `bee.Feeder.processBeeGroup()`

Create execution record **before** launching process (aligned with manager.go):

```go
// Create record first to get exec.ID for log path
exec, err := f.execStore.CreateBeeExecution(sessionID, prompt)
logPath := filepath.Join(f.logsDir, time.Now().Format("2006-01-02"), exec.ID+".log")
os.MkdirAll(filepath.Dir(logPath), 0o755)
f.execStore.SetLogPath(exec.ID, logPath)

proc, outputCh, err := f.runner.Run(beeCtx, f.workDir, prompt, opts, logPath)
```

### `bee.Feeder.drainBeeOutput()` → renamed `waitBeeOutput()`

Simplified — no accumulation, just lifecycle:

```go
func (f *Feeder) waitBeeOutput(ch <-chan claude.Output) error {
    for out := range ch {
        if out.Type == claude.OutputDone { return nil }
        if out.Type == claude.OutputError { return fmt.Errorf("bee exited with error: %s", out.Content) }
    }
    return fmt.Errorf("bee output channel closed without completion signal")
}
```

Remove: `logRegistry` field, `WithLogRegistry` option.

### `api.Server.getExecutionLogs()`

Simplified — single-stage file read:

```go
func (s *Server) getExecutionLogs(c *gin.Context) {
    content, err := s.ExecutionStore.ReadLog(c.Param("id"))
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    if content != "" {
        c.Header("Cache-Control", "public, max-age=3600")
    }
    c.String(http.StatusOK, content)
}
```

Remove: `LogRegistry` field from `api.Server`.

## Result Extraction

After `OutputDone`, parse the log file for the `result` field:

```go
func extractResultFromLog(logPath string) string {
    data, _ := os.ReadFile(logPath)
    var lastAssistantText, streamResult string
    for _, line := range strings.Split(string(data), "\n") {
        line = strings.TrimSpace(line)
        if !strings.HasPrefix(line, "{") { continue }
        var event claudeStreamEvent
        if json.Unmarshal([]byte(line), &event) != nil { continue }
        switch event.Type {
        case "assistant":
            if event.Message != nil && len(event.Message.Content) > 0 {
                if event.Message.Content[0].Type == "text" {
                    lastAssistantText = event.Message.Content[0].Text
                }
            }
        case "result":
            if event.Result != "" { streamResult = event.Result }
        }
    }
    if streamResult != "" { return streamResult }
    return lastAssistantText
}
```

Same logic as current `monitorExecution`, just reading from file instead of channel.

## Edge Cases

| Scenario | Handling |
|----------|---------|
| File creation fails before launch | Return error, mark execution failed |
| Process killed mid-execution | File retains partial content; `OutputError` triggers; result = error message |
| Log file not yet written (read during startup race) | `ReadLog()` returns empty string (existing behavior) |
| Execution record created but process fails to start (feeder) | Record marked failed; log_path set but file may be empty |

## Files Changed

| File | Change |
|------|--------|
| `internal/claude/invoker.go` | Add `logPath` param; redirect stdout/stderr to file; remove scanning goroutines |
| `internal/worker/log_registry.go` | **Delete entire file** |
| `internal/worker/manager.go` | Remove `logRegistry`; simplify `monitorExecution`; add `extractResultFromLog` |
| `internal/bee/feeder.go` | Remove `logRegistry`; reorder execution record creation; simplify `drainBeeOutput` → `waitBeeOutput` |
| `internal/store/execution_store.go` | Remove `WriteLog()`; add `SetLogPath()` |
| `internal/api/execution_handler.go` | Remove `LogRegistry`; simplify `getExecutionLogs` to single-stage read |
