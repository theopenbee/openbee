# Linear: Remove ReactionDelete Logic

**Date:** 2026-05-06
**Status:** Draft
**Predecessor spec:** [2026-05-05-linear-reaction-design.md](2026-05-05-linear-reaction-design.md)

## Problem

The Linear platform currently implements a "typing acknowledgment" pattern modeled after Feishu:

1. On inbound message arrival, `LinearReceiver.addReaction` creates a `:eyes:` reaction on the issue/comment and stores the resulting `reactionID` in a `pendingReactions sync.Map` keyed by the inbound `PlatformMessageID`. A 10-minute `AfterFunc` cleans up orphan entries.
2. After `LinearSender.Send` posts the reply comment, `removeReaction` does a `LoadAndDelete` on the map, waits up to 5s on a buffered channel for the `reactionID`, then calls `client.DeleteReaction` to remove the `:eyes:`.

We are removing the deletion half of this flow, plus the underlying GraphQL `reactionDelete` mutation. The `:eyes:` reaction will be created on dispatch and left in place; nothing will retract it.

## Scope

In scope:
- Drop sender-side reaction removal and the `pendingReactions` coordination apparatus (sync.Map, buffered channel, TTL cleanup).
- Drop the `Client.DeleteReaction` interface method, the `*httpClient` implementation, the `reactionDeleteMutation` constant, and the `TestClient_DeleteReaction` test.
- Simplify `addReaction` to a fire-and-forget `CreateReaction` goroutine — keep retry + warn-on-failure, drop all coordination state.
- Update tests in `handler_test.go` / `sender_test.go` to match the slimmer surface.

Out of scope:
- `CreateReaction`, `reactionCreateMutation`, the `:eyes:` constant — all unchanged.
- Feishu's analogous `pendingReactions` plumbing — unchanged.
- The historical specs/plans under `docs/superpowers/` — left as-is.

## Design

### Component changes

#### `internal/platform/linear/client.go`

- Remove from `Client` interface:
  ```go
  DeleteReaction(ctx context.Context, reactionID string) error
  ```
- Remove `httpClient.DeleteReaction` method.
- Remove the `reactionDeleteMutation` constant.

#### `internal/platform/linear/handler.go`

- Remove `reactionCleanupTTL` constant.
- Remove `pendingReactions *sync.Map` field from:
  - `LinearPlatform` (the field is plumbed via `NewPlatform`).
  - `LinearReceiver`.
  - `LinearSender`.
- In `NewPlatform`, remove the `pending := &sync.Map{}` allocation and the two field assignments.
- Rewrite `addReaction` to be fire-and-forget:
  ```go
  func (r *LinearReceiver) addReaction(ctx context.Context, target ReactionTarget) {
      go func() {
          if err := utils.RetryWithBackoff(ctx, func() error {
              _, e := r.client.CreateReaction(ctx, target, reactionEmoji)
              return e
          }, utils.DefaultRetryCount, utils.DefaultRetryDelay); err != nil {
              log.Warn("linear: add reaction failed", zap.Error(err))
          }
      }()
  }
  ```
  - Drop the `key` parameter (only used for the map). Update call sites in `tickOnce` accordingly.
- Remove `LinearSender.removeReaction` entirely.
- In `LinearSender.Send`, remove the `s.removeReaction(msg.ReplyTo.PlatformMessageID)` call.
- Drop the `sync` import if no other code in the file uses it.

#### `internal/platform/linear/client_test.go`

- Delete `TestClient_DeleteReaction`.

#### `internal/platform/linear/handler_test.go`

- Remove the `DeleteReaction` method from the `fakeClient` type (interface contracted).
- Delete tests that assert against `pendingReactions` storage or the sender's reaction removal flow.
- Keep / simplify the `addReaction` test to assert `CreateReaction` is invoked (and that failures are swallowed).

#### `internal/platform/linear/sender_test.go`

- Delete the `Send` tests that verify `pendingReactions` interaction or `DeleteReaction` calls.
- Keep tests covering the comment-creation happy path and error handling.

### Behavior after change

- Inbound dispatch still triggers an asynchronous `:eyes:` reaction (best-effort).
- The reaction is **never** removed by the bot — it stays on the comment/issue indefinitely. Operators who want it gone delete it manually in Linear.
- `Send` returns as soon as `CreateComment` (and any media upload) succeed. No background reaction goroutine is spawned at send time.
- No `pendingReactions` map exists, so there is no memory growth, no orphan-cleanup TTL, and no coordination channel.

### Why fire-and-forget for `addReaction`

`addReaction` previously needed the `pendingReactions` map only because the sender wanted the `reactionID` later. With the sender no longer interested, the goroutine has nothing to publish — calling `CreateReaction` and logging on failure is the entire job.

## Testing

- `go build ./...`
- `go test ./internal/platform/linear/...`
- `go vet ./...`

Test coverage targets:
- `addReaction` happy path: `CreateReaction` invoked with `(target, ":eyes:")`.
- `addReaction` error path: `CreateReaction` returns error, no panic, warning logged (verify via fake or by ensuring no return value is consumed).
- `Send` happy path: `CreateComment` invoked, no reaction-related calls.
- `Send` continues to handle media upload + parent comment threading.

## Risks

- **Pending reactions visible forever.** Acceptable; the user explicitly chose B.
- **Test deletions vs. test coverage drift.** Mitigated by leaving the create-side coverage intact and verifying nothing else in the codebase depends on `Client.DeleteReaction` (already grep-confirmed).
- **Feishu parity drift.** Feishu still uses the same pattern. We are intentionally diverging — Linear's UX choice now differs from Feishu. No code shared between them.

## Rollout

Single PR. No feature flag needed — behavior change is bounded to the Linear platform and is a strict reduction in functionality.
