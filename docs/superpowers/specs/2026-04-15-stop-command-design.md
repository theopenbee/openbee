# `/stop` Command Design

**Date:** 2026-04-15
**Status:** Draft

## Overview

Add a `/stop` slash command that allows users to immediately terminate all running and pending executions/tasks associated with their current session. This addresses scenarios where a message is taking too long to process or was sent in error.

## Problem Statement

When a user sends a message to Bee and the processing takes a long time, there is currently no way to cancel it short of waiting for it to time out or fail. Users need a way to forcibly stop ongoing work mid-session.

## Requirements

- Users can send `/stop` in any platform chat to stop all active work for their session
- The command stops: Bee's own running/pending execution AND all running/pending Worker tasks for the session
- After stopping, the user receives a confirmation message
- If there is nothing running, the user still receives a friendly message indicating no active tasks were found
- The command is intercepted before Bee processes it (Bee never sees `/stop` as a regular message)

## Architecture

### New Component: CommandInterceptor

A new `CommandInterceptor` is introduced in `internal/domain/bee/command_interceptor.go`, sitting between the platform message ingestion and the Feeder's Bee dispatch logic.

```
Platform Message
      ↓
CommandInterceptor.Intercept(msg)
      ├── /stop → execute stop logic → send reply → handled=true
      ├── /other → future commands
      └── non-command → handled=false
      ↓
Feeder normal dispatch (only when handled=false)
```

```go
type CommandInterceptor struct {
    taskStore    store.TaskStore
    execStore    store.ExecutionStore
    sessionStore store.SessionStore
    execStopper  worker.ExecutionStopper
    dispatcher   task.Dispatcher
    sender       platform.Sender
    beeAgentID    string        // bee.BeeAgentID constant
    currentEngine string        // cfg.Bee.EffectiveEngine()
}

func (c *CommandInterceptor) Intercept(ctx context.Context, msg platform.InboundMessage) (handled bool, err error)
```

### Feeder Integration

In `feeder.go`, before the existing Bee dispatch logic:

```go
handled, err := f.commandInterceptor.Intercept(ctx, msg)
if err != nil {
    log.Warn("command interceptor error", "err", err)
}
if handled {
    return
}
// existing Bee dispatch logic...
```

`CommandInterceptor` is injected into Feeder at construction time.

## `/stop` Execution Flow

1. **Detect command:** Check if `strings.TrimSpace(msg.Content) == "/stop"` (case-insensitive)

2. **Find Bee execution session:**
   ```
   SessionStore.GetSessionContextForEngine(ctx, sessionKey, beeAgentID, currentEngine)
   → sessionID
   ```

3. **Stop Bee execution:**
   ```
   ExecutionStore.ListBySessionID(sessionID)
   → filter status=running or pending
   → Manager.StopExecution(executionID) for each
   ```
   Errors from `StopExecution` (e.g., process already exited) are logged and ignored—execution continues.

4. **Cancel Worker tasks:**
   ```
   TaskStore.CancelBySessionKey(ctx, sessionKey)
   → batch UPDATE status=cancelled for running+pending tasks
   ```

5. **Clean dispatcher memory queues:**
   ```
   TaskDispatcher.ClearSession(sessionKey)
   → removes in-memory queued tasks, cancels context funcs
   ```

6. **Reply to user:**
   - If any executions/tasks were stopped: `"✅ 已停止当前会话的所有任务"`
   - If nothing was running: `"当前会话没有正在运行的任务"`

## Edge Cases

| Scenario | Handling |
|----------|---------|
| No session context found | Skip steps 2–3, proceed to step 4–6 |
| No running/pending executions | Skip StopExecution calls, still cancel tasks |
| `StopExecution` fails (process already exited) | Log warning, continue |
| `CancelBySessionKey` returns 0 rows | Still send "no active tasks" reply |
| User sends `/stop` while `/stop` is being processed | Second `/stop` handled correctly (idempotent) |
| `/stop` arrives during Bee message processing | Intercepted before Bee sees it; Bee execution is stopped via step 3 |

## Command Detection

Command detection is strict and minimal:

- Must be exact `/stop` (trimmed, case-insensitive)
- No arguments supported in v1
- Prefix `/` check makes it easy to extend for future commands

## Testing Strategy

Unit tests in `command_interceptor_test.go` with mocked dependencies:

- `/stop` with active execution and tasks → stops all, sends success reply
- `/stop` with no active tasks → sends "no active tasks" reply
- `/stop` where `StopExecution` returns error → logs error, continues, sends reply
- Non-command message → returns `handled=false`, no side effects
- Empty message / whitespace-only → returns `handled=false`

## Files to Change

| File | Change |
|------|--------|
| `internal/domain/bee/command_interceptor.go` | New file: CommandInterceptor implementation |
| `internal/domain/bee/command_interceptor_test.go` | New file: unit tests |
| `internal/domain/bee/feeder.go` | Inject CommandInterceptor, call Intercept before dispatch |
| `internal/app/app.go` (`buildBee` function) | Construct and inject CommandInterceptor into Feeder |
