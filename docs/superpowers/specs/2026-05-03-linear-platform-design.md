# Linear Platform Integration — Design

- **Date**: 2026-05-03
- **Status**: Approved (brainstorming)
- **Owner**: 貂蝉

## 1. Background and Goal

OpenBee already integrates with chat-style platforms (Feishu, DingTalk, WeCom, Weixin, Telegram, local) through a uniform `platform.Platform` interface. We want to extend the same model to **Linear**, treating Linear *issues* (and their comments) as inbound messages so that bee/worker dispatch logic can be reused unchanged.

The user-facing semantics:
- An issue tagged with the configured label (default `openbee`) is "owned" by the bot.
- The issue title + description becomes the **first inbound message** of a session.
- Subsequent comments on that issue are follow-up messages in the same session.
- The first whitespace-delimited token of the message body decides routing:
  - `@workerName` or `{workerName} ` (matching the existing `msgingest` convention) → directly dispatched to that worker.
  - Otherwise → handed to the bee.
- Bee/worker output is posted back as a new comment on the same issue. If the inbound was itself a threaded comment reply, the response is posted into the same Linear comment thread via `parentId`.

## 2. Non-Goals (v0)

- **No webhook endpoint.** v0 is polling-only. A webhook receiver is acceptable future work; the package is structured so it can be added without breaking changes.
- **No media attachments.** Linear comments do not natively carry binary attachments via the GraphQL API; image/file uploads require Linear's separate file-upload flow. v0 returns an error if the outbound message has `MediaPath` set, and logs the issue.
- **No status mutation.** The bot does not change issue status, assignee, priority, or labels. It only reads issues/comments and writes comments.
- **No multi-workspace.** A single Linear API key (= a single workspace + bot user) per OpenBee deployment.

## 3. Architecture Overview

A new package `internal/platform/linear` implements the existing `platform.Platform` interface, mirroring the structure of `internal/platform/telegram`:

```
internal/platform/linear/
├── handler.go        # LinearPlatform / LinearReceiver / LinearSender
├── client.go         # Minimal Linear GraphQL client
├── cursor.go         # lastSyncAt persistence on top of system_configs
├── handler_test.go
├── client_test.go
└── cursor_test.go
```

The Receiver runs a polling goroutine that calls the Linear GraphQL API at a fixed interval, converts each new issue/comment into a `platform.InboundMessage`, and dispatches it through the same channel used by all other platforms. The Sender accepts an `OutboundMessage`, parses the embedded raw JSON to recover the target issue (and optional parent comment) ID, and creates a new comment via the `commentCreate` mutation.

No changes are needed to `msgingest`, `bee/feeder`, `worker/dispatcher`, or to the routing rules — the routing convention `@workerName` / `{workerName} ` already exists and is reused.

## 4. Configuration

### 4.1 Schema additions (`internal/infra/config/config.go`)

```go
type LinearConfig struct {
    Enabled      bool          `yaml:"enabled"`
    APIKey       string        `yaml:"api_key"`        // Linear personal API key (required when enabled)
    LabelName    string        `yaml:"label_name"`     // gating label; default "openbee"
    PollInterval time.Duration `yaml:"poll_interval"`  // default 10s
    BotName      string        `yaml:"bot_name"`       // for ingest @-mention strip; default "openbee"
}
```

`LinearConfig` is added to `PlatformsConfig`. `BotNames()` is updated to include `Linear.BotName`. `applyDefaults` fills `LabelName="openbee"`, `PollInterval=10s`, `BotName="openbee"` when omitted.

`bot_user_id` is intentionally **not** a config field. It is resolved at startup by calling Linear's `viewer` query and cached on the receiver — this avoids drift if the API key is rotated and removes a manual setup step.

### 4.2 YAML template (`config.yaml.tmpl`)

A `linear:` block is added under `platforms:` with `enabled: false` and inline comments documenting each field.

## 5. Components

### 5.1 `client.go` — Linear GraphQL client

A self-contained, dependency-free GraphQL client. ~150–250 LOC.

Public surface:

| Method | Purpose |
|---|---|
| `NewClient(apiKey string) *Client` | Constructor; pins endpoint `https://api.linear.app/graphql`. |
| `Viewer(ctx) (User, error)` | Returns the authenticated user; called once at startup to learn `bot_user_id`. |
| `IssuesUpdatedSince(ctx, since time.Time, label string) ([]Issue, error)` | Returns issues matching `labels.name == label && updatedAt > since`, ordered by `updatedAt` ascending. Each issue carries its `comments` connection (also filtered by `createdAt > since`). |
| `CreateComment(ctx, issueID, body string, parentID *string) (Comment, error)` | Posts a comment via `commentCreate`. |

Internal:

- `do(ctx, query, vars, &out)` — single GraphQL POST helper; sets `Authorization: <api_key>` header (Linear accepts the API key directly without `Bearer`).
- Strongly typed response structs for each call (`Issue`, `Comment`, `User`, `Team`).

Errors are wrapped with the operation name to make logs actionable.

### 5.2 `cursor.go` — `lastSyncAt` persistence

Reuses the existing `system_configs` SQLite table via `store.SystemConfigStore`. Key constant: `linear.last_sync_at`. Value is RFC3339 timestamp.

API:

```go
type Cursor struct { store *store.SystemConfigStore }

func (c *Cursor) Load(ctx) (time.Time, error)   // missing key → time.Now().Add(-1*time.Hour)
func (c *Cursor) Save(ctx, t time.Time) error
```

The 1-hour bootstrap window prevents replaying ancient history on first run. It is also the failure-tolerance buffer if the deployment is restarted.

### 5.3 `handler.go` — Platform/Receiver/Sender

Structure mirrors `internal/platform/telegram/handler.go`:

```go
const PlatformID = "linear"

type LinearPlatform struct { receiver *LinearReceiver; sender *LinearSender }

func NewPlatform(cfg config.LinearConfig, sysCfg *store.SystemConfigStore) platform.Platform
func (p *LinearPlatform) ID() string                                  { return PlatformID }
func (p *LinearPlatform) Receiver() platform.PlatformReceiverAdapter  { return p.receiver }
func (p *LinearPlatform) Sender() platform.PlatformSenderAdapter      { return p.sender }
```

## 6. Polling Receiver Flow

```
Start(ctx, dispatch):
    botUser  = client.Viewer(ctx)        # fail-fast on auth error
    lastSync = cursor.Load(ctx)
    ticker   = time.NewTicker(cfg.PollInterval)
    for {
        select {
        case <-ctx.Done(): return nil
        case <-ticker.C:
            tickOnce(ctx, &lastSync, botUser, dispatch)  # errors logged, cursor not advanced on failure
        }
    }

tickOnce:
    issues, err = client.IssuesUpdatedSince(ctx, lastSync, cfg.LabelName)
    if err != nil { log; return }
    highWater = lastSync
    for issue in issues (sorted asc by updatedAt):
        if issueIsNewlyOwned(issue, lastSync):
            dispatch(buildIssueInbound(issue))
        for comment in issue.Comments where comment.CreatedAt > lastSync && comment.User.ID != botUser.ID:
            dispatch(buildCommentInbound(issue, comment))
            highWater = max(highWater, comment.CreatedAt)
        highWater = max(highWater, issue.UpdatedAt)
    cursor.Save(ctx, highWater)
```

`issueIsNewlyOwned` returns true when the issue label was added after `lastSync` (or the issue itself was created after `lastSync` already carrying the label). Linear's GraphQL `issue.labels(...)` connection exposes `IssueLabel.createdAt`, which we use to make this determination without storing extra state.

**Cursor safety.** On any error inside `tickOnce`, the cursor is *not* advanced; the next tick re-pulls the same window. Idempotency is provided by `msg_store`'s unique constraint on `PlatformMessageID`, so duplicates are rejected at ingest time without side effects.

## 7. InboundMessage Mapping

| Field | Issue body | Comment |
|---|---|---|
| `Platform` | `linear` | `linear` |
| `SenderID` | `issue.creator.id` | `comment.user.id` |
| `SessionKey` | `linear:<TEAM_KEY>:<ISSUE_IDENTIFIER>` | same as issue |
| `Content` | `issue.title + "\n\n" + issue.description` | `comment.body` |
| `RawContent` | same as `Content` | same as `Content` |
| `Raw` | JSON of the issue object (used by Sender) | JSON of `{issue: {id}, comment: {id, parentId}}` |
| `PlatformMessageID` | `issue:<id>` | `comment:<id>` |
| `MessageTime` | `issue.createdAt` ms | `comment.createdAt` ms |

The `SessionKey` keys off `<TEAM_KEY>:<ISSUE_IDENTIFIER>` (e.g. `linear:ENG:ENG-42`) so it remains stable across issue title edits and is human-readable in logs.

## 8. Sender Flow

```
Send(ctx, msg):
    raw = parseRaw(msg.ReplyTo.Raw)            # extract issueID and optional parentCommentID
    if msg.MediaPath != "" {
        return error("linear: media attachments not supported in v0")
    }
    _, err = client.CreateComment(ctx, raw.IssueID, msg.Content, raw.ParentCommentID)
    return err
```

`raw.ParentCommentID` is `nil` when the inbound was the issue body, the source comment id when the inbound was a top-level comment, and the source comment's `parentId` when the inbound was already a thread reply (so subsequent replies stay in the same Linear thread rather than starting a new one).

## 9. Loop Prevention and Deduplication

1. **Primary filter:** Receiver drops any comment whose `user.id == botUserID` before dispatching.
2. **Backstop:** `msg_store.PlatformMessageID` uniqueness rejects any duplicate that slips through (e.g. a manual replay or a polling overlap during cursor rollback).
3. **Sender does not echo:** the Sender never re-injects its own writes back into the dispatch path; it only calls Linear.

## 10. Startup Registration (`internal/app/app.go`)

- `buildPlatforms` gains a branch:
  ```go
  if lc.Enabled {
      result = append(result, linear.NewPlatform(lc, s.systemConfigStore))
  }
  ```
  Note this is the first platform constructor that takes `*store.SystemConfigStore`; the signature of `buildPlatforms` is widened by one parameter.
- `msgingest.WithPlatformBotNames` map gains `linear.PlatformID: cfg.Bee.Platforms.Linear.BotName`.
- No `RegisterExtractor` is needed — Linear bodies are markdown text and require no platform-specific @-tag rewriting.

## 11. Testing Strategy

- **`client_test.go`** — `httptest.Server` stands in for `api.linear.app/graphql`. Verifies request body (query + variables), headers, and response decoding for `Viewer`, `IssuesUpdatedSince`, and `CreateComment`. Uses fixture JSON.
- **`cursor_test.go`** — Round-trip Save/Load against an in-memory `SystemConfigStore`; missing-key returns `now − 1h`.
- **`handler_test.go`** — Mock `Client` interface (extracted in `client.go` so it can be substituted). Cases:
  - First-tick bootstrap when cursor is empty.
  - Issue created with label after `lastSync` → emits issue body.
  - Existing labeled issue with new comments → emits in chronological order, skips bot-authored comments.
  - GraphQL error inside `tickOnce` → cursor not advanced, next tick re-pulls.
  - Sender resolves `parentId` correctly for issue-trigger vs comment-trigger vs thread-reply trigger.

## 12. Files Touched

**New**
- `internal/platform/linear/handler.go`
- `internal/platform/linear/client.go`
- `internal/platform/linear/cursor.go`
- `internal/platform/linear/handler_test.go`
- `internal/platform/linear/client_test.go`
- `internal/platform/linear/cursor_test.go`
- `docs/superpowers/specs/2026-05-03-linear-platform-design.md` (this file)

**Modified**
- `internal/infra/config/config.go` — `LinearConfig`, `PlatformsConfig.Linear`, `BotNames`, `applyDefaults`.
- `internal/infra/config/config.yaml.tmpl` — `platforms.linear` block.
- `internal/app/app.go` — `buildPlatforms` signature/body, `WithPlatformBotNames` map.
- `CHANGELOG.md` — English entry under the next release.

## 13. Open Questions / Future Work

- **Webhook receiver.** Once the polling path is validated, a `POST /webhook/linear` route can be added on the existing HTTP server. The Receiver's polling loop and webhook handler can coexist (or be made mutually exclusive via a config switch).
- **Status / assignee feedback.** Future flag to optionally re-assign the issue to its original creator after the bot finishes, or change status to `In Review`. Out of scope for v0.
- **Attachment support.** Linear's file-upload flow (`fileUpload` mutation → `attachmentLinkURL`) can be wired in once a concrete user need surfaces.
