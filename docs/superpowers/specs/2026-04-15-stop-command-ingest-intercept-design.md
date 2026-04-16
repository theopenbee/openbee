# Design: /stop Command Ingest-Level Interception

**Date**: 2026-04-15  
**Branch**: feature/stop-command  
**Status**: Approved

## Problem

The `/stop` command is queued and processed in FIFO order like any other message, which defeats its purpose. Two failure scenarios:

- **Scenario A**: User sends messages then `/stop` — `/stop` waits behind earlier messages before being processed.
- **Scenario B**: Bee is actively running (session has a `feeding` message) — `ClaimBatch()` skips the entire session, so `/stop` is never claimed.

Both scenarios share the same root cause: `/stop` should not enter the normal message queue at all.

## Solution: Ingest-Level Interception

Intercept `/stop` in `msgingest.Gateway.Dispatch()` **before** the message enters the debounce/DB queue. Execute stop logic immediately, then discard the message from the normal flow.

The Feeder-level `CommandInterceptor` is **removed** — all `/stop` handling is now done at the ingest layer.

## Architecture

### Current Flow
```
Platform → Gateway.Dispatch() → debounce → DB (received) → Feeder.tick() → ClaimBatch() → [waits FIFO] → CommandInterceptor.Intercept()
```

### New Flow
```
Platform → Gateway.Dispatch()
              ↓ /stop detected
           CommandInterceptor.InterceptInbound()
              → save msg to DB as bee_processed (chat history)
              → go handleStop() [async, non-blocking]
              → return (message not queued)

              ↓ normal message
           debounce → DB (received) → Feeder... [unchanged]
```

## Code Changes

### 1. `internal/domain/msgingest/gateway.go`

Add `InboundInterceptor` interface:

```go
// InboundInterceptor intercepts an inbound message before it enters the debounce queue.
// Returns true if the message was handled and should not be queued.
type InboundInterceptor interface {
    InterceptInbound(ctx context.Context, msg platform.InboundMessage) bool
}
```

Add `interceptor` field to `Gateway` struct and a setter:

```go
func (g *Gateway) SetInboundInterceptor(h InboundInterceptor) {
    g.interceptor = h
}
```

Add check at the top of `Dispatch()`:

```go
func (g *Gateway) Dispatch(msg platform.InboundMessage) {
    if g.interceptor != nil && g.interceptor.InterceptInbound(context.Background(), msg) {
        return
    }
    // ... existing debounce logic unchanged
}
```

### 2. `internal/domain/bee/command_interceptor.go`

Add `InterceptInbound()` method implementing `msgingest.InboundInterceptor`:

```go
func (c *CommandInterceptor) InterceptInbound(ctx context.Context, msg platform.InboundMessage) bool {
    if !strings.EqualFold(strings.TrimSpace(msg.Content), cmdStop) {
        return false
    }
    msgID := uuid.New().String()
    // Persist to DB so /stop appears in chat history
    c.msgStore.Create(ctx, msgID, msg.SessionKey, msg.Platform, msg.Content,
        msg.Raw, msg.PlatformMessageID, msg.MessageTime)
    c.msgStore.MarkBeeProcessed(ctx, []string{msgID})
    // Run stop logic asynchronously — do not block Dispatch
    go c.handleStop(context.Background(), msg.SessionKey, store.ClaimedMessage{
        ID:         msgID,
        SessionKey: msg.SessionKey,
        Platform:   msg.Platform,
        Content:    msg.Content,
    })
    return true
}
```

`CommandInterceptor` satisfies `msgingest.InboundInterceptor` via duck typing — no import cycle.

### 3. `internal/app/app.go`

After building `ci`, wire it into both Gateway instances. Remove the Feeder-level wiring:

```go
ci := bee.NewCommandInterceptor(...)
// feeder.SetCommandInterceptor(ci) — removed, Feeder no longer handles commands
ingest.SetInboundInterceptor(ci)      // main gateway (all external platforms)
localIngest.SetInboundInterceptor(ci) // local platform gateway
```

Build order is unchanged — `ci` is already constructed after both gateways.

Also remove: `feeder.SetCommandInterceptor()` method, `commandInterceptor` field on `Feeder`, and the `Intercept()` call inside `processBeeGroup()`.

## Files Modified

| File | Change |
|------|--------|
| `internal/domain/msgingest/gateway.go` | Add `InboundInterceptor` interface, `interceptor` field, `SetInboundInterceptor()`, intercept check in `Dispatch()` |
| `internal/domain/bee/command_interceptor.go` | Add `InterceptInbound()` method |
| `internal/domain/bee/feeder.go` | Remove `commandInterceptor` field, `SetCommandInterceptor()`, and the `Intercept()` call in `processBeeGroup()` |
| `internal/app/app.go` | Remove `feeder.SetCommandInterceptor(ci)`; add `ingest.SetInboundInterceptor(ci)` and `localIngest.SetInboundInterceptor(ci)` |

## Design Decisions

- **Async stop execution**: `handleStop` runs in a goroutine so `Dispatch()` is not blocked. Stop is inherently fire-and-forget.
- **Persist to DB**: `/stop` is saved with status `bee_processed` immediately, so it appears in chat history.
- **Feeder-level interception removed**: `commandInterceptor` field, `SetCommandInterceptor()`, and the intercept call in `processBeeGroup()` are deleted from `Feeder`. All command handling lives at the ingest layer.
- **Duck typing**: `CommandInterceptor` satisfies `msgingest.InboundInterceptor` without importing `msgingest`, avoiding import cycles.
- **Two Gateway instances**: Both `ingest` and `localIngest` must be wired — they serve different receivers (external platforms vs local chat UI).
