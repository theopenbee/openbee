# Design: Remove platform_context Injection from Bee

**Date:** 2026-04-22  
**Branch:** feat/platform-context-injection  
**Status:** Approved

## Background

Both bee and worker currently inject `platform_context` into their prompt/instruction metadata via `platform.ExtractContext()`. This context contains platform-specific sender and chat metadata (e.g., Feishu open_id, chat_id, DingTalk sender info).

Workers need `platform_context` to reply to messages on the correct platform with the correct identifiers. Bee, however, is platform-agnostic — it routes messages and coordinates workers but does not interact with platforms directly. Injecting `platform_context` into bee's prompt is unnecessary noise that muddies the platform-agnostic nature of bee.

## Decision

Remove `platform_context` from bee's message metadata entirely. Worker retains it unchanged.

**Approach:** Delete both the struct field and the injection logic in bee (not just the injection logic), so the struct accurately reflects that bee has no platform context.

## Changes

### `internal/domain/bee/feeder.go`

**1. Remove `PlatformContext` field from `messageMeta` struct:**

```go
// Before
type messageMeta struct {
    From            string          `json:"from"`
    SessionKey      string          `json:"session_key"`
    MessageID       string          `json:"message_id"`
    PlatformContext json.RawMessage `json:"platform_context,omitempty"`
}

// After
type messageMeta struct {
    From       string `json:"from"`
    SessionKey string `json:"session_key"`
    MessageID  string `json:"message_id"`
}
```

**2. Remove injection logic from `buildPrompt()`:**

```go
// Delete these lines (~352-354)
if ctx := platform.ExtractContext(m.Platform, m.Raw); ctx != "" {
    meta.PlatformContext = json.RawMessage(ctx)
}
```

### `internal/domain/bee/feeder_internal_test.go`

- **Delete** `TestBuildPrompt_WithPlatformContext` — this test asserts that `platform_context` appears in bee's message meta, which is now the wrong behavior.
- **Keep** `TestBuildPrompt_NoPlatformContext` — rename to `TestBuildPrompt_NeverHasPlatformContext` and update its message to reflect the new invariant: bee's message meta never contains `platform_context`, regardless of platform.

## What Does NOT Change

- `platform.ExtractContext()` and the platform extractor registry — unchanged
- `internal/domain/task/dispatcher.go` (worker injection) — unchanged
- Database schema and message ingestion — unchanged
- All other bee tests — unaffected

## Data Flow After Change

```
Bee message metadata (messageMeta):
  { "from": "...", "session_key": "...", "message_id": "..." }
  ← no platform_context, ever

Worker task metadata (taskMeta):
  { "message_id": "...", "task_id": "...", "platform_context": { ... } }
  ← unchanged
```

## Testing

Run after implementation:
```
go test ./internal/domain/bee/...
go test ./internal/domain/task/...
```

All tests must pass. Verify bee prompt output no longer contains `"platform_context"`.
