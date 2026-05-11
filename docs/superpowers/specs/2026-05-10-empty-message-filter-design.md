# Empty Message Filter Design

- Date: 2026-05-10
- Owner: 小乔
- Status: Approved (pending plan)

## Problem

When a user sends a bare bot @mention with no further content (e.g. `@貂蝉`, where `貂蝉` is the worker/bot display name), the message ingestion pipeline strips the mention via `Gateway.stripBotMention` and the resulting content becomes an empty string. The empty content is then:

1. Written to the `bee_platform_messages` table via `CreateBatch`.
2. Pushed into the per-session debounce window in `Gateway.Dispatch`.
3. Emitted as an `IngestedMessage` and ultimately handed to an LLM-backed worker.

The downstream effect is wasted tokens, confusing worker output ("I have nothing to do" hallucinations), and noise in the message history.

## Goals

- Detect "empty after mention strip" messages at ingest time.
- Reply to the user with a localized hint that emphasizes the message looked empty and ask them to elaborate.
- Do not write the empty message to the database.
- Do not let the empty message enter the debounce window.
- Do not invoke the LLM.

## Non-Goals

- No per-session rate limiting. Every empty message receives a reply. Platform-level deduplication (existing `seen` set keyed by `PlatformMessageID`) is the only suppression mechanism; it prevents duplicate replies caused by retransmission, not by user behavior.
- No DB audit row for dropped empty messages.
- No new metrics. Existing structured logging is sufficient.
- No changes inside platform-specific receivers (`feishu`, `dingtalk`, `wecom`, etc.). All logic stays in the platform-agnostic `msgingest` package.

## Definition of "Empty"

A message is considered empty when `strings.TrimSpace(stripped) == ""`, where `stripped` is the result of `Gateway.stripBotMention(msg.Content, msg.Platform)`.

This rule transparently distinguishes "empty text" from "media-only message" because all three IM platforms (Feishu, DingTalk, WeCom) materialize image/file/audio/video attachments into a placeholder string (e.g. `[name="x.png" path="..."]`) inside `InboundMessage.Content` via `media.Service.BuildPlaceholder`. A media-only message therefore has a non-empty `Content` after stripping and is not flagged as empty.

Reference call sites that prove this invariant:
- `internal/platform/feishu/handler.go` — `case "image", "file", "audio", "video", "media", "sticker"` → `resolveMediaContent` → placeholder.
- `internal/platform/dingtalk/handler.go` — `case "picture" | "file" | "audio" | "video"` → `BuildPlaceholder`.
- `internal/platform/wecom/handler.go` — `case msgTypeImage | msgTypeFile` → `download` returns placeholder.

## Architecture

### Insertion Point in `Gateway.Dispatch`

```
Receiver
  → Gateway.Dispatch
    → stripBotMention                       (existing)
    → [mu.Lock]
    → platform_msg_id dedup                 (existing)
    → **EMPTY CHECK** → emptyHandler.HandleEmpty → return
    → commandHandler.IsCommand?             (existing)
    → debounce accumulate                   (existing)
```

The empty check runs **after** dedup and **before** the command check.

- After dedup so that platform retransmissions of the same empty message do not produce duplicate replies.
- Before the command check because empty content is never a slash command, and emitting an empty string into accumulation would create a stray `mergedSeparator`-prefixed merged content if a real message arrived inside the same debounce window.

### New Interface

File: `internal/domain/msgingest/empty_handler.go`

```go
type EmptyMessageHandler interface {
    HandleEmpty(ctx context.Context, msg platform.InboundMessage)
}

type DefaultEmptyMessageHandler struct {
    senders map[string]platform.PlatformSenderAdapter
}

func NewDefaultEmptyMessageHandler(senders map[string]platform.PlatformSenderAdapter) *DefaultEmptyMessageHandler

func (h *DefaultEmptyMessageHandler) HandleEmpty(ctx context.Context, msg platform.InboundMessage)
```

`DefaultEmptyMessageHandler.HandleEmpty` looks up the sender for `msg.Platform` and emits an `OutboundMessage` whose `Content` is the localized hint, `ReplyTo` is the original inbound message, and `SourceType` is `"system"`. If no sender is registered for the platform, the call is a no-op with a `log.Warn`. If `sender.Send` returns an error, it is logged at warn level; no retry.

### Gateway Wiring

File: `internal/domain/msgingest/gateway.go`

Add field and Option:

```go
type Gateway struct {
    // ...existing fields...
    emptyHandler EmptyMessageHandler
}

func WithEmptyMessageHandler(h EmptyMessageHandler) Option {
    return func(g *Gateway) { g.emptyHandler = h }
}
```

Modify `Dispatch` to insert the empty check after dedup:

```go
if strings.TrimSpace(stripped) == "" {
    g.mu.Unlock()
    log.Info("empty message after strip, replying with hint",
        zap.String("sessionKey", msg.SessionKey),
        zap.String("platform", msg.Platform),
        zap.String("platformMsgID", msg.PlatformMessageID))
    if g.emptyHandler != nil {
        g.emptyHandler.HandleEmpty(context.Background(), msg)
    }
    return
}
```

The `emptyHandler != nil` guard preserves backward compatibility for existing unit tests that construct a `Gateway` without the new option. Production wiring must inject the handler.

### Production Wiring

At the Gateway construction site (the same place that already wires `MessageStore`, `CommandHandler`, and `WithPlatformBotNames`), append:

```go
msgingest.WithEmptyMessageHandler(
    msgingest.NewDefaultEmptyMessageHandler(platformSenders),
)
```

`platformSenders` is the same `map[string]platform.PlatformSenderAdapter` already built and passed to `EngineCommandHandler` / `StopCommandHandler` / `ClearCommandHandler`.

### Concurrency

- The empty check runs while `g.mu` is held but releases the lock before invoking `HandleEmpty`, mirroring the existing command-path pattern. No IO under lock.
- `HandleEmpty` is invoked synchronously by `Dispatch`. Unlike commands (which are serialized through `cmdCh`), empty replies are stateless and have no ordering requirement, so the extra channel hop is not justified.

## i18n

Add to `internal/infra/i18n/locales/zh.yaml` under the `runtime` namespace:

```yaml
  empty_message:
    hint: "📭 老板，您发的消息看起来是空的（可能只 @ 了我没带内容）。请补一下要我做什么？"
```

Add to `internal/infra/i18n/locales/en.yaml`:

```yaml
  empty_message:
    hint: "📭 Looks like your message is empty (perhaps just an @mention with no content). Could you tell me what you'd like me to do?"
```

Add the matching Go struct field to the i18n package so `i18n.M.Runtime.EmptyMessage.Hint` resolves.

## Testing

### `internal/domain/msgingest/empty_handler_test.go` (new)

| Test | Scenario | Assertion |
|------|----------|-----------|
| `TestDefaultEmptyMessageHandler_SendsHint` | Sender map contains the inbound platform | `Send` invoked once with `Content == i18n.M.Runtime.EmptyMessage.Hint`, `ReplyTo == inbound msg`, `SessionKey` matches, `SourceType == "system"` |
| `TestDefaultEmptyMessageHandler_NoSenderForPlatform` | Sender map missing the inbound platform | No panic, no `Send` call |
| `TestDefaultEmptyMessageHandler_SenderError` | `Send` returns an error | No panic, no retry |

### `internal/domain/msgingest/gateway_test.go` (extend)

| Test | Scenario | Assertion |
|------|----------|-----------|
| `TestDispatch_EmptyAfterStrip_InvokesEmptyHandler` | Inbound `"@机器人"` with bot name `"机器人"` | Mock `EmptyMessageHandler.HandleEmpty` called once; `MessageStore.CreateBatch` never called; no value emitted on `g.Out()` |
| `TestDispatch_EmptyAfterStrip_DedupStillApplies` | Same `PlatformMessageID` delivered twice with empty content | `HandleEmpty` called exactly once |
| `TestDispatch_EmptyAfterStrip_NoHandler_NoOp` | `emptyHandler == nil`, empty content | No panic, function returns; `CreateBatch` never called |
| `TestDispatch_NonEmpty_DoesNotInvokeEmptyHandler` | Inbound `"@机器人 hello"` | Mock `HandleEmpty` not called; debounce path engages |

`strip_test.go` already covers the "entire content is mention" case that produces an empty string; no changes needed there.

## Out of Scope (YAGNI)

- Per-session cooldown / rate limit (every empty message gets a reply by design).
- Persisting empty messages with a `dropped` status row (no audit need expressed).
- Metrics counters or dashboards (project has no metrics surface yet).
- Platform-handler-side filtering (single chokepoint in `Gateway.Dispatch` is sufficient).

## Risks and Mitigations

- **Risk**: A future platform receiver delivers a message whose `Content` is empty even though the user attached media (i.e. forgets to call `BuildPlaceholder`). The empty filter would suppress that media message.
  **Mitigation**: The three existing receivers all materialize media into placeholders today; this contract is documented in `docs/feishu-media-spec.md` and `docs/dingtalk-media-technical-spec.md`. Any new receiver must follow the same contract. Reviewers should reject a platform PR that delivers media without a placeholder.

- **Risk**: A worker bot is added to a high-traffic group chat and platform retries trigger many sender calls.
  **Mitigation**: The `seen` dedup runs before the empty check, so the same `PlatformMessageID` cannot produce repeated replies. Distinct messages from a flooding user are not throttled by design.
