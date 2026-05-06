# Linear Reaction Support — Design

Date: 2026-05-05
Status: Approved (brainstorming complete, ready for implementation plan)

## Goal

Add a reaction lifecycle to the Linear platform that mirrors the existing Feishu (`internal/platform/feishu/handler.go`) flow: when the bot receives an inbound message it adds an `:eyes:` reaction to the trigger, and when the bot's reply is posted the reaction is removed. The user-visible effect is "bot saw this → bot replied → reaction disappears".

## Non-Goals

- Persisting in-flight reactions across process restarts (orphan reactions on crash are accepted, matching the Feishu implementation).
- Surfacing user reactions back into the bot processing pipeline.
- Supporting reactions on `projectUpdate` or any Linear entity beyond `Issue` / `Comment`.
- Configurable emoji per workspace; the emoji is a single hard-coded constant for now.

## Reference Implementation

Feishu, `internal/platform/feishu/handler.go`:

- Receiver hook (lines 132–173): on inbound message arrival, fires `MessageReaction.Create` (emoji `Typing`) in a goroutine and stores the resulting `reactionID` in `pendingReactions sync.Map`, keyed by `messageID`. A 10-minute timer cleans up orphan entries.
- Sender hook (lines 479–512): when `Send` runs, it `LoadAndDelete`s the entry, reads `reactionID` from the channel with a 5s timeout, and calls `MessageReaction.Delete`. Failure to delete is logged and does not fail the send.

## Linear-Specific Context

- `internal/platform/linear/handler.go` runs in **polling** mode (no webhook). `LinearReceiver.tickOnce` periodically calls `IssuesInStates` and dispatches two kinds of `InboundMessage`:
  - `buildInitialInbound`: first-seen issue (issue body + non-bot comments merged). `PlatformMessageID = "issue:" + issue.ID`.
  - `buildCommentInbound`: a new comment on an already-seen issue. `PlatformMessageID = "comment:" + c.ID`.
- `LinearSender.Send` posts a reply via `commentCreate` mutation.
- Linear's GraphQL API supports `reactionCreate(input: ReactionCreateInput!)` and `reactionDelete(id: String!)`. `ReactionCreateInput` accepts `commentId`, `issueId`, and a string `emoji` (e.g. `":eyes:"`).

## Design

### Reaction target

The reaction is placed on whatever **triggered** the dispatch:

- Initial-issue inbound → react on the issue (`issueId = issue.ID`).
- Comment inbound → react on that comment (`commentId = comment.ID`).

### Emoji

Single constant `reactionEmoji = ":eyes:"` defined in `internal/platform/linear/handler.go`. Chosen because `:eyes:` is the de-facto "I saw this / on it" signal in Linear / GitHub / Slack.

### Data flow

```
LinearReceiver.tickOnce
    │
    ├──► dispatch(InboundMessage)              (main path; bee processing starts immediately)
    │
    └──► go createReaction(target)             (side-effect goroutine)
             │
             └─► pendingReactions.Store(PlatformMessageID, chan reactionID)
                 + 10-minute AfterFunc cleanup

bee processing finishes
    │
    └──► LinearSender.Send(OutboundMessage)
            │
            ├──► CreateComment(...)                                   (post the reply)
            │
            └──► pendingReactions.LoadAndDelete(msg.ReplyTo.PlatformMessageID)
                    │
                    └─► read reactionID from channel (5s timeout)
                            │
                            └─► DeleteReaction(reactionID)            (log on failure, no error propagation)
```

`PlatformMessageID` (`issue:<id>` / `comment:<id>`) is already a stable identifier shared between receiver and sender; it serves as the sync.Map key without any new field on `InboundMessage`.

### Component changes

#### `internal/platform/linear/client.go`

Extend the `Client` interface with two methods, plus a small target struct:

```go
type ReactionTarget struct {
    CommentID string // takes precedence when both set
    IssueID   string
}

type Client interface {
    // ...existing methods unchanged...
    CreateReaction(ctx context.Context, target ReactionTarget, emoji string) (reactionID string, err error)
    DeleteReaction(ctx context.Context, reactionID string) error
}
```

GraphQL mutations:

```graphql
mutation ReactionCreate($input: ReactionCreateInput!) {
  reactionCreate(input: $input) { reaction { id } }
}

mutation ReactionDelete($id: String!) {
  reactionDelete(id: $id) { success }
}
```

`CreateReaction` builds `input` from `ReactionTarget`: prefer `commentId` if non-empty, else `issueId`; reject when both empty.

#### `internal/platform/linear/handler.go`

- Add `pendingReactions *sync.Map` to `LinearReceiver`. The `*sync.Map` is created in `NewPlatform` and the same pointer is shared with `LinearSender` (new field), so the sender can read what the receiver wrote.
- In `tickOnce`, immediately after each `dispatch(...)` call (one for `buildInitialInbound`, one for `buildCommentInbound`), spawn a goroutine that:
  1. Calls `client.CreateReaction(ctx, target, reactionEmoji)`.
  2. On success, stores a buffered `chan string` (capacity 1) in `pendingReactions` under the inbound's `PlatformMessageID`, sends the `reactionID` into the channel, and schedules `time.AfterFunc(10*time.Minute, …)` to `LoadAndDelete` the entry as a memory-leak guard.
  3. On failure, logs and skips the store (the sender will see a miss and skip delete).
- The target is constructed at the call site:
  - For initial issue: `ReactionTarget{IssueID: issue.ID}`, key `"issue:" + issue.ID`.
  - For new comment: `ReactionTarget{CommentID: c.ID}`, key `"comment:" + c.ID`.
- `LinearSender.Send`: after `CreateComment` succeeds, call `pendingReactions.LoadAndDelete(msg.ReplyTo.PlatformMessageID)`. If present, read the `reactionID` from the channel with a 5-second `select` timeout, then call `client.DeleteReaction(ctx, reactionID)`. Failures are logged; `Send` still returns `nil`.

#### Constants

```go
const reactionEmoji = ":eyes:"
```

defined at the top of `handler.go` next to `botCommentPrefix`.

### Failure handling and edge cases

| Situation | Behavior |
|-----------|----------|
| `reactionCreate` fails (network / permission / rate limit) | Log warning; nothing stored in `pendingReactions`; sender's `LoadAndDelete` misses and skips delete. |
| `reactionDelete` fails | Log warning; `Send` still returns nil success. |
| Process crash / restart | `sync.Map` is in-memory and lost; reaction stays on the Linear entity. Same trade-off as Feishu; accepted. |
| Sender's 5-second wait for the channel times out | Skip delete; an orphan reaction remains on the entity. Logged. |
| Same inbound dispatched twice (logic bug) | `sync.Map.LoadAndDelete` ensures the second call sees nothing; only one delete is attempted. |
| Bot's own reaction shows up in subsequent polling | `IssuesInStates` does not query reactions, so no impact on the receiver's seen-set / dedup logic. |
| Reactor user already reacted with the same emoji | `reactionCreate` returns an error; falls into the "create failed" path. |

### Testing strategy

Mirror existing `linear/*_test.go` patterns (fake `Client` in handler tests, fake GraphQL server in client tests).

`client_test.go`:

- `TestCreateReaction_OnComment` — fake server asserts mutation name + `commentId` payload, decodes returned `reactionID`.
- `TestCreateReaction_OnIssue` — same, with `issueId`.
- `TestCreateReaction_RejectsEmptyTarget` — both fields empty returns error without HTTP call.
- `TestDeleteReaction` — asserts mutation name + `id` payload + success unmarshalling.

`handler_test.go`:

- `TestTickOnce_AddsReactionForInitialIssue` — fake client records `CreateReaction` calls; assert `target.IssueID == issue.ID`, `pendingReactions` has key `"issue:<id>"`.
- `TestTickOnce_AddsReactionForNewComment` — assert `target.CommentID == comment.ID`, key `"comment:<id>"`.
- `TestTickOnce_ReactionCreateFails_DoesNotBlockDispatch` — fake client returns error; assert dispatch still happened, `pendingReactions` has no entry.

`sender_test.go`:

- `TestSend_DeletesReactionAfterReply` — pre-populate `pendingReactions`; assert `DeleteReaction` called with stored ID.
- `TestSend_NoPendingReaction_StillSucceeds` — empty `pendingReactions`; `Send` returns nil; no `DeleteReaction` call.
- `TestSend_ReactionDeleteFails_StillSucceeds` — fake `DeleteReaction` returns error; `Send` still returns nil.

Existing dispatch / dedup tests must continue to pass without modification.

### Out of scope (deferred)

- Configurable emoji per workspace.
- Disk-persisted `pendingReactions` for restart resilience.
- Cleanup sweeper for orphan reactions accumulated by past crashes.
- Reactions on `projectUpdate` entities.

## File-Level Change Summary

- `internal/platform/linear/client.go` — add `ReactionTarget` struct and `CreateReaction` / `DeleteReaction` interface methods + httpClient implementations + GraphQL mutations.
- `internal/platform/linear/handler.go` — add `pendingReactions sync.Map` ownership, receiver-side goroutine after dispatch, sender-side delete-after-send, `reactionEmoji` constant.
- `internal/platform/linear/client_test.go` — add reaction mutation tests.
- `internal/platform/linear/handler_test.go` — add reaction lifecycle tests on receiver path.
- `internal/platform/linear/sender_test.go` — add reaction delete tests on sender path.
- `CHANGELOG.md` — add an English entry under the unreleased section noting Linear reaction support.
