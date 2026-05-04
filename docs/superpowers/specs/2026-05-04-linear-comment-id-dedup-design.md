# Linear Comment Deduplication: ID-Based Approach

**Date:** 2026-05-04
**Branch:** feat/linear-platform

## Overview

Replace the handler-side timestamp double-check for comment deduplication with a persistent seen-comment-ID set. Also remove the GraphQL comment-level `createdAt` filter so all comments under a matching issue are fetched and deduplicated solely by ID.

## Problem

The current approach uses two layers:
1. **API layer** — `comments(filter: { createdAt: { gt: $since } })` in the GraphQL query
2. **Handler layer** — `if !c.CreatedAt.After(since) { continue }` in `tickOnce()`

This is fragile: timestamp precision, clock skew, or edge cases where two comments arrive at the exact cursor boundary can cause missed or double-dispatched comments. ID-based deduplication is deterministic and immune to these issues.

## Decisions

| Question | Decision |
|----------|----------|
| Remove API-level comment filter? | Yes — fetch all comments per issue |
| Remove handler-level timestamp check? | Yes — replaced by ID set lookup |
| Retention policy for ID set | Permanent (never evict) |
| Storage approach | Standalone `seen_comments.go`, same pattern as `cursor.go` |

## Architecture

### Files Changed

| File | Change |
|------|--------|
| `internal/platform/linear/seen_comments.go` | **New** — `seenAPI` interface + `SeenComments` implementation |
| `internal/platform/linear/handler.go` | Inject `seenAPI`; replace timestamp check; simplify `highWater` |
| `internal/platform/linear/client.go` | Remove `createdAt` filter from comments sub-query |

### Data Flow (per tick)

```
tickOnce()
  ├─ projects check (unchanged)
  ├─ load cursor last_sync (unchanged, drives issue-level filter)
  ├─ GraphQL: issues(updatedAt > since, label, projects)
  │           + ALL comments per issue (no createdAt filter)
  ├─ for each issue:
  │    ├─ isNewlyOwned? → dispatch issue inbound (unchanged)
  │    └─ for each comment:
  │         ├─ seenComments.Contains(c.ID)? → skip
  │         ├─ body HasPrefix("[openbee-bot]")? → skip
  │         └─ dispatch → append c.ID to newIDs
  ├─ seenComments.Add(ctx, newIDs) → persist
  └─ advance cursor to max(issue.UpdatedAt) if not truncated
```

### Cursor Responsibility

The cursor narrows to a single concern: filtering which **issues** to examine (`issues.updatedAt > since`). It no longer participates in comment deduplication. The `highWater` calculation is simplified to only track `issue.UpdatedAt`; per-comment `highWater` tracking is removed.

## Component Design

### `seen_comments.go` (new file)

**Interface:**

```go
type seenAPI interface {
    Load(ctx context.Context) error
    Contains(id string) bool
    Add(ctx context.Context, ids []string) error
}
```

**Struct:**

```go
type SeenComments struct {
    dir string
    ids map[string]struct{}
}
```

**Disk format** (`~/.openbee/.linear/seen_comments.json`):

```json
{ "ids": ["abc123", "def456"] }
```

**Behavior:**
- `Load()` — reads file; on `ErrNotExist` returns nil with empty map (first run)
- `Contains()` — pure in-memory O(1) lookup; must call `Load` before use
- `Add()` — updates in-memory map and atomically persists via tmp+rename (identical to `cursor.go:Save`)

**Initialization:** `SeenComments` is constructed in `NewPlatform` alongside `Cursor`. `Load()` is called once at the top of `LinearReceiver.Start()`, before the polling loop begins.

### `handler.go` changes

**`LinearReceiver` struct** — add field:

```go
seenComments seenAPI
```

**`Start()`** — call `Load` before poll loop:

```go
if err := r.seenComments.Load(ctx); err != nil {
    return fmt.Errorf("linear receiver: seen comments load: %w", err)
}
```

**`tickOnce()`** — replace timestamp guard with ID check, collect new IDs, persist after tick:

```go
// Remove:
if !c.CreatedAt.After(since) { continue }

// Add:
if r.seenComments.Contains(c.ID) { continue }

// After comment loop, persist new IDs:
if len(newIDs) > 0 {
    if err := r.seenComments.Add(ctx, newIDs); err != nil {
        log.Error("seen comments save", zap.Error(err))
    }
}
```

**`highWater` simplification** — remove per-comment tracking:

```go
// Remove:
if c.CreatedAt.After(highWater) { highWater = c.CreatedAt }

// Keep only:
if issue.UpdatedAt.After(highWater) { highWater = issue.UpdatedAt }
```

### `client.go` changes

Remove the `createdAt` filter from the comments sub-query in `issuesQuery`:

```graphql
// Before:
comments(filter: { createdAt: { gt: $since } }, orderBy: createdAt) {

// After:
comments(orderBy: createdAt) {
```

The `$since` variable remains and still drives the issue-level `updatedAt` filter.

## Error Handling

- `seenComments.Load` failure at startup → `Start()` returns error (same policy as cursor load failure)
- `seenComments.Add` failure mid-tick → logged as error, tick continues; worst case: re-dispatch on next tick (comment will be seen again because its ID wasn't persisted)
- Corrupt `seen_comments.json` → treat as empty (same fallback as `cursor.go`)

## Testing

- Unit test `SeenComments`: empty file, populated file, corrupt file, Contains/Add roundtrip
- Unit test `tickOnce` with fake `seenAPI`: verify already-seen IDs are skipped, new IDs are added
- Existing `handler_test.go` patterns apply — inject fake `seenAPI` via interface

## Non-Goals

- ID set eviction / TTL (permanent retention by design)
- Compression or compaction of the JSON file
- Migrating existing deployments (first run after deploy: seen_comments.json doesn't exist → empty map → all comments in active issues re-evaluated; bot comments still filtered by prefix, so only genuine user comments may re-dispatch once)
