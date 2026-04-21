# Reaction Retry Mechanism Design

**Date:** 2026-04-21  
**Status:** Approved

## Background

When a message is received, the bot immediately adds a "processing" reaction emoji to give the user visual feedback (Feishu: "Typing" 🕐, DingTalk: "🤔 Thinking..."). Once the response is ready, the reaction is removed. Due to network fluctuations, these API calls can fail silently. Currently there is no retry logic — failures are logged and ignored (fire-and-forget).

## Requirements

- **Platforms:** Feishu + DingTalk
- **Operations:** Both add and recall (remove) reactions
- **Retry strategy:** Exponential backoff, max 5 retries
- **After exhaustion:** Silent failure — log error, continue message processing
- **Non-blocking:** Retries run inside existing goroutines, never block message replies

## Architecture

### File Changes

```
internal/
  utils/
    retry.go          ← NEW: shared exponential backoff retry utility
  platform/
    feishu/
      handler.go      ← MODIFIED: wrap addReaction / recallReaction with retry
    dingtalk/
      handler.go      ← MODIFIED: wrap addThinkingEmoji / recallThinkingEmoji with retry
```

### Data Flow

```
Message received
  → goroutine: RetryWithBackoff(addReaction, 5, 500ms)
      → attempt 1 → fail → wait 500ms
      → attempt 2 → fail → wait 1s
      → attempt 3 → fail → wait 2s
      → attempt 4 → fail → wait 4s
      → attempt 5 → fail → wait 8s
      → all failed → log.Error, return silently
  → message processing continues unaffected
```

### Retry Parameters

| Parameter | Value |
|-----------|-------|
| Base delay | 500ms |
| Max retries | 5 |
| Backoff sequence | 500ms → 1s → 2s → 4s (between attempts) |
| Total max wait | ~7.5s |
| Strategy | Exponential (delay × 2 each attempt) |

## Implementation

### 1. Shared Retry Utility (`internal/utils/retry.go`)

```go
// RetryWithBackoff retries fn with exponential backoff up to maxRetries times.
// baseDelay is the wait time before the second attempt; it doubles each round.
// Returns the last error if all attempts fail.
func RetryWithBackoff(ctx context.Context, fn func() error, maxRetries int, baseDelay time.Duration) error {
    var err error
    delay := baseDelay
    for i := 0; i < maxRetries; i++ {
        if err = fn(); err == nil {
            return nil
        }
        if i < maxRetries-1 {
            select {
            case <-ctx.Done():
                return ctx.Err()
            case <-time.After(delay):
            }
            delay *= 2
        }
    }
    return err
}
```

**Design decisions:**
- Accepts `context.Context` for cancellation support (e.g., handler shutdown)
- Does not log internally — callers handle logging for proper context
- First attempt is immediate; waiting starts only after the first failure
- Returns last error so callers can log with full context

### 2. Feishu Handler Changes (`internal/platform/feishu/handler.go`)

**addReaction** — wrap `MessageReaction.Create()`:

```go
// Before
resp, err := r.larkClient.Im.MessageReaction.Create(ctx, req)
if err != nil || !resp.Success() {
    log.Error("add reaction error", zap.Error(err))
    close(reactionCh)
    return
}
reactionCh <- resp.Data.ReactionId

// After
err := utils.RetryWithBackoff(ctx, func() error {
    resp, e := r.larkClient.Im.MessageReaction.Create(ctx, req)
    if e != nil {
        return e
    }
    if !resp.Success() {
        return fmt.Errorf("reaction create failed: %v", resp.CodeError)
    }
    reactionCh <- resp.Data.ReactionId
    return nil
}, 5, 500*time.Millisecond)
if err != nil {
    log.Error("add reaction failed after retries", zap.Error(err))
    close(reactionCh)
}
```

**recallReaction** — wrap `MessageReaction.Delete()` similarly; log.Warn on final failure.

### 3. DingTalk Handler Changes (`internal/platform/dingtalk/handler.go`)

Wrap `doEmojiRequest` calls in `addThinkingEmoji` and `recallThinkingEmoji`:

```go
// Before
if err := d.addThinkingEmoji(msgID, convID); err != nil {
    log.Warn("add emoji failed", zap.Error(err))
}

// After
err := utils.RetryWithBackoff(ctx, func() error {
    return d.doEmojiRequest("add", msgID, convID)
}, 5, 500*time.Millisecond)
if err != nil {
    log.Error("add emoji failed after retries", zap.Error(err))
}
```

recall follows the same pattern.

## Error Handling

| Scenario | Behavior |
|----------|----------|
| Network timeout on attempt N (N < 5) | Retry after exponential backoff |
| All 5 attempts fail | log.Error, close channel if needed, return |
| Context cancelled during retry | Exit immediately, return ctx.Err() |
| Recall fails after all retries | log.Warn (reaction may linger briefly, non-critical) |

Reaction failures never block or affect message reply delivery.

## Testing

- Unit test `RetryWithBackoff`: success on first try, success on Nth try, all failures, context cancellation
- Integration: mock Feishu/DingTalk API to return errors for first N calls, verify retry count and final behavior
