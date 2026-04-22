# `/stop` Command Design

**Date:** 2026-04-21  
**Status:** Draft

## Problem

The bee scheduler (Feeder layer) occasionally stalls on long-running executions. During this time, new messages from the same session accumulate in the `platform_messages` table with status `received`. There is no way for the user to interrupt the stuck bee or discard the backlog without also clearing the conversation history (`/clear`).

## Goal

Add a `/stop` slash command that:
1. Immediately terminates the running bee process for the current session
2. Cancels all pending (`received`) messages accumulated in the session
3. Preserves session context (conversation history is not cleared)
4. Sends a result summary to the user after execution

## Non-Goals

- Stopping worker task dispatcher queue (task layer is out of scope)
- Clearing session context (use `/clear` for that)
- Requiring user confirmation (executes immediately)

## Design

### Execution Order

To avoid a race condition with `ClaimBatch` (which skips sessions with a `feeding` message), the stop handler executes in this order:

```
1. FailReceived(sessionKey)   — mark 'received' messages as 'failed' while M1 is still 'feeding'
2. StopSession(sessionKey)    — cancel the running bee's context
3. sendReply                  — send /stop result to user
```

**Why this order is safe:** `ClaimBatch` enforces at-most-one-feeding-per-session. While the bee is running (M1 is `feeding`), pending messages (M2, M3) cannot be claimed. `FailReceived` runs first while M1 is still `feeding`, so M2 and M3 are safely marked failed before any new bee could pick them up.

**Residual edge case:** If the bee completes naturally in the window between the user sending `/stop` and the handler executing, M1 is no longer `feeding`, and a subsequent tick could claim M2 before `FailReceived` runs. This is acceptable — it means the bee finished on its own, and the user's `/stop` arrives too late to be meaningful. The impact is that M2 gets processed normally.

### Data Flow

```
User sends "/stop"
  → Gateway.Dispatch: IsCommand → cmdCh
  → runCommandConsumer → StopCommandHandler.HandleCommand(ctx, content, replyTo)
      1. msgStore.FailReceived(ctx, sessionKey)
             → UPDATE platform_messages SET status='failed'
               WHERE session_key=? AND status='received'
             → returns []string of failed message IDs
      2. for each ID: failureNotifier.NotifyTaskFailure(ctx, id, FailureInfo{...})
      3. feeder.StopSession(sessionKey)
             → f.runningMu.Lock()
             → cancel, ok := f.running[sessionKey]
             → f.runningMu.Unlock()
             → if ok: cancel()    (cancels beeCtx → runner.Run() returns context.Canceled)
      4. feeder's processBeeGroup detects cancellation
             → failMessages(ctx, msgs, reason)  (handles the 'feeding' message M1)
      5. sendReply → /stop result message
```

### Component Changes

#### 1. `internal/domain/bee/feeder.go`

Add cancel registry to `Feeder`:

```go
type Feeder struct {
    // ... existing fields ...
    runningMu sync.Mutex
    running   map[string]context.CancelFunc
}
```

In `NewFeeder`, initialize `running: make(map[string]context.CancelFunc)`.

In `processBeeGroup`, wrap the `beeCtx` creation with registration:

```go
beeCtx, cancel := context.WithTimeout(ctx, f.cfg.Engine.Timeout.Bee)
f.runningMu.Lock()
f.running[sessionKey] = cancel
f.runningMu.Unlock()
defer func() {
    cancel()
    f.runningMu.Lock()
    delete(f.running, sessionKey)
    f.runningMu.Unlock()
}()
```

New exported method:

```go
// StopSession cancels the running bee for sessionKey.
// Returns true if a bee was running and was cancelled.
func (f *Feeder) StopSession(sessionKey string) bool {
    f.runningMu.Lock()
    cancel, ok := f.running[sessionKey]
    f.runningMu.Unlock()
    if ok {
        cancel()
    }
    return ok
}
```

#### 2. `internal/infra/store/message_store.go`

```go
// FailReceived marks all 'received' messages for a session as 'failed'.
// Returns the IDs of affected messages.
func (s *MessageStore) FailReceived(ctx context.Context, sessionKey string) ([]string, error)
```

Implementation: single UPDATE to mark `received → failed` for the session, then SELECT to return affected IDs.

#### 3. `internal/domain/command/stop.go` (new file)

Narrow interfaces:

```go
type BeeStopper interface {
    StopSession(sessionKey string) bool
}

type StopMessageStore interface {
    FailReceived(ctx context.Context, sessionKey string) ([]string, error)
}

type StopFailureNotifier interface {
    NotifyTaskFailure(ctx context.Context, messageID string, info model.FailureInfo) error
}
```

Handler:

```go
type StopCommandHandler struct {
    feeder   BeeStopper
    msgs     StopMessageStore
    notifier StopFailureNotifier
    senders  map[string]platform.PlatformSenderAdapter
}
```

`IsCommand`: returns `content == CmdStop`

`HandleCommand`:
1. `FailReceived` → get failed IDs
2. Notify each ID via `notifier.NotifyTaskFailure`
3. `StopSession` → get `beeWasStopped bool`
4. Build result string based on `beeWasStopped` and `len(ids)`, call `sendReply`

#### 4. `internal/domain/command/engine.go`

```go
CmdStop = "/stop"
```

#### 5. i18n — `internal/infra/i18n/messages.go`

Add to `RuntimeMessages`:

```go
StopCommand StopCommandMessages `yaml:"stop_command"`
```

New struct:

```go
type StopCommandMessages struct {
    Stopped              string `yaml:"stopped"`               // no pending messages
    StoppedWithMessages  string `yaml:"stopped_with_messages"` // contains %d
    CancelledMessages    string `yaml:"cancelled_messages"`    // bee not running; contains %d
    NothingToStop        string `yaml:"nothing_to_stop"`
}
```

#### 6. `internal/infra/i18n/locales/zh.yaml`

```yaml
stop_command:
  stopped: "✅ 已停止 bee 执行"
  stopped_with_messages: "✅ 已停止 bee 执行，取消了 %d 条待处理消息"
  cancelled_messages: "✅ 已取消 %d 条待处理消息"
  nothing_to_stop: "当前会话没有需要停止的内容"
```

#### 7. `internal/infra/i18n/locales/en.yaml`

```yaml
stop_command:
  stopped: "✅ Stopped bee execution"
  stopped_with_messages: "✅ Stopped bee execution and cancelled %d pending message(s)"
  cancelled_messages: "✅ Cancelled %d pending message(s)"
  nothing_to_stop: "Nothing to stop in this session"
```

#### 8. `internal/app/app.go`

```go
stopCmdHandler := command.NewStopCommandHandler(feeder, s.msgStore, failureNotifier, sendersByPlatform)
cmdChain := msgingest.ChainHandlers(engineCmdHandler, clearCmdHandler, stopCmdHandler)
```

### Result Message Matrix

| bee running | pending messages | reply |
|-------------|-----------------|-------|
| yes | yes (N) | `StoppedWithMessages` (%d=N) |
| yes | no | `Stopped` |
| no | yes (N) | `CancelledMessages` (%d=N) |
| no | no | `NothingToStop` |

### Error Handling

- `FailReceived` failure: log error, skip failure notifications for pending messages, still attempt `StopSession`, reply with whatever result is available
- `StopSession` when no bee running: returns false, reflected in result message
- `NotifyTaskFailure` failure: log and continue (non-fatal, same as existing feeder behavior)
