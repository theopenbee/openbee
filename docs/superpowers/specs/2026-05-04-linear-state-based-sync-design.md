# Linear State-Based Sync — Design

Date: 2026-05-04
Branch: feat/linear-platform

## Background & Motivation

The Linear receiver currently polls with a `updatedAt > since` cursor and
persists the high-water mark in `cursor.json`. Investigation of recurring
"lost-comment" incidents has shown three independent failure modes:

1. **Comments dropped.** Adding a comment to an issue does not always advance
   `issue.updatedAt` quickly enough relative to a moving cursor. Combined with
   page truncation, time-zone parsing, and clock skew, the cursor can sail past
   a comment that never gets re-fetched.
2. **Issues dropped on first labeling.** Long-standing backlog issues that
   acquire the gating label later may have `updatedAt` already older than the
   cursor, so they are never picked up.
3. **Operationally opaque.** `cursor.json` is a single timestamp blob. Operators
   cannot inspect "did the bot already process issue X?" or rerun a single
   issue. The only recovery tool is moving the cursor backwards, which produces
   noise and still relies on the same broken filter.

The fix is to drop the time-based cursor entirely and pivot the receiver to a
**state-driven query** with **persistent issue ID and comment ID dedup sets**.

## Goals

- Replace `updatedAt > since` filtering with a workflow-state filter.
- Persist the set of dispatched issue IDs alongside the existing seen-comment
  set, both inspectable as flat JSON files.
- Make the active state list configurable via yaml seed + `bee_system_configs`
  override + in-memory hot-reload, mirroring the existing `linear_projects`
  configuration path.
- Preserve the existing self-comment skip mechanism (`[openbee-bot]` body
  prefix) and outbound flow unchanged.

## Non-Goals

- Web SystemSettings frontend form for `linear_states` (deferred to a separate
  frontend PR).
- Automatic cleanup or migration of stale `cursor.json` files (operators remove
  manually).
- Inferring states by `WorkflowState.type`. Filter is by `WorkflowState.name`,
  per the current operator preference.
- Re-engaging an issue that returns to a configured state after leaving it. The
  seen-issues set is permanent (only-grow); operators clear it manually if they
  want to re-engage.

## Architecture Overview

### Polling Loop (per tick)

```
projects ← projectStore.Get()
states   ← statesStore.Get()
if len(projects) == 0 || len(states) == 0:
    return                        # nothing to do; mirror existing projects policy

issues ← client.IssuesInStates(ctx, states, label, projects)   # fully paginated

for issue in issues:
    if !seenIssues.Contains(issue.ID):
        nonBot ← [c for c in issue.Comments if !hasPrefix(c.Body, "[openbee-bot]")]
        dispatch(buildInitialInbound(issue, nonBot))
        record issue.ID in newIssueIDs
        record c.ID for each c in nonBot in newCommentIDs
    else:
        for c in issue.Comments:
            if seenComments.Contains(c.ID):              continue
            if hasPrefix(c.Body, "[openbee-bot]"):       continue
            dispatch(buildCommentInbound(issue, c))
            record c.ID in newCommentIDs

seenIssues.Add(newIssueIDs)
seenComments.Add(newCommentIDs)
```

Key invariants:

- **No timestamp filter anywhere.** The window is defined entirely by
  `state.name ∈ states ∧ label = label ∧ project.name ∈ projects`.
- **First-sight issue dispatch is a single merged message.** Title, description,
  and every existing non-bot comment ship together so the bee gets full context
  in one shot rather than fragmented per-comment dispatches.
- **Comments folded into the merged dispatch are added to seenComments
  immediately.** Without this they would re-dispatch on the next tick.
- **Both stores commit only after the dispatch loop completes.** A crash mid-tick
  causes the affected items to re-dispatch on the next tick — same crash
  semantics as today's cursor-not-advanced path.

### GraphQL Query Change

```graphql
# Before
filter: {
  updatedAt: { gt: $since },
  labels:    { name: { eq: $label } },
  project:   { name: { in: $projects } }
}
orderBy: updatedAt

# After
filter: {
  state:   { name: { in: $states } },
  labels:  { name: { eq: $label } },
  project: { name: { in: $projects } }
}
orderBy: createdAt   # stable order for pagination
```

### Pagination

Truncation no longer carries reservation semantics (there is no cursor to hold
back). The client paginates fully within each tick using Linear's
`endCursor` / `hasNextPage`, with `pageSize = 50`. Any per-page error aborts
the tick; the next tick restarts from page 1 and dedup keeps it idempotent.

### New Persistence Files

| File | Purpose | Shape |
|---|---|---|
| `~/.openbee/.linear/seen_issues.json` | Set of dispatched issue IDs (only-grow) | `{"ids":["…","…"]}` |
| `~/.openbee/.linear/seen_comments.json` | Set of dispatched comment IDs (existing) | `{"ids":["…","…"]}` |

`seen_issues.json` mirrors `seen_comments.json` exactly: atomic `tmp+rename`,
silent fallback to empty set on `ErrNotExist` or corrupt JSON.

### Config Plumbing

Mirrors the existing `linear_projects` chain end-to-end:

```
config.yaml          (seed)
   └─→ LinearConfig.States []string

bee_system_configs   (DB override)
   └─→ key = "linear_states"
   └─→ value = JSON array of state name strings

linearcfg.StatesStore  (in-memory, hot-reload)
   └─→ read by LinearReceiver every tick
   └─→ written by SystemConfigHandler on PUT
```

**Empty list policy.** As with `linear_projects`, `states = []` causes the
receiver to skip the tick entirely. This is intentional: the operator must
explicitly opt in to a state list. Default yaml value is empty, with a
commented example `states: ["Todo", "In Progress"]` in `config.yaml.tmpl`.

## Component Inventory

### New Files

- `internal/platform/linear/seen_issues.go` — `SeenIssues` type, parallel to
  `SeenComments`. Same atomic write pattern. Permanent only-grow set.
- `internal/platform/linear/seen_issues_test.go` — mirror of
  `seen_comments_test.go`.

### Removed Files

- `internal/platform/linear/cursor.go`
- `internal/platform/linear/cursor_test.go`

### Modified Files

| File | Change |
|---|---|
| `internal/platform/linear/client.go` | New `IssuesInStates(ctx, states, label, projects) ([]Issue, error)` replacing `IssuesUpdatedSince`; new GraphQL filter; full pagination loop; truncation flag dropped from return signature |
| `internal/platform/linear/client_test.go` | Update fixtures for new filter and pagination |
| `internal/platform/linear/handler.go` | Drop `cursor` field, `cursorAPI`, `isNewlyOwned`. Add `seenIssues seenIssuesAPI` and `statesStore *linearcfg.StatesStore`. Rewrite `tickOnce` to the merged-first-dispatch flow. Replace `buildIssueInbound` with `buildInitialInbound(issue, nonBotComments)` |
| `internal/platform/linear/handler_test.go` | Cover six scenarios listed under "Test Plan" |
| `internal/infra/config/config.go` | Add `States []string` to `LinearConfig`. No default applied (empty = skip) |
| `internal/infra/config/config.yaml.tmpl` | Add commented `states:` block with example |
| `internal/infra/model/system_config.go` | Add `SystemConfigKeyLinearStates = "linear_states"` |
| `internal/domain/linearcfg/store.go` | Add `StatesStore` peer type (own struct, own methods; no shared abstraction) |
| `internal/domain/linearcfg/store_test.go` | Mirror `Store` tests for `StatesStore` |
| `internal/app/app.go` | Construct `linearStates` immediately after `linearCfg`, identical seed→DB-override flow; pass to receiver and SystemConfigHandler |
| `internal/api/system_config_handler.go` | Generalize `parseLinearProjects` → `parseStringList` (same validation rules); add `linear_states` case; route through `linearStates.Set` after persist |
| `internal/api/system_config_handler_test.go` | Add coverage for `linear_states` PUT (valid, malformed, empty) |
| `internal/infra/i18n/messages.go` + `messages.*.yaml` | Add `LinearStates` and `LinearStatesHelp` keys |
| `CHANGELOG.md` | Add entry under `Unreleased` (English, per project convention) |

### Builder Signatures

```go
// Replaces buildIssueInbound. Comments contains only non-bot comments,
// in chronological order. SenderID = issue.Creator.ID.
// PlatformMessageID = "issue:" + issue.ID.
// Raw = replyTarget{IssueID: issue.ID} (reply lands at issue top level).
func buildInitialInbound(issue Issue, comments []Comment) platform.InboundMessage
```

Merged content format (option A from brainstorming):

```
{title}

{description}

---
Comments ({n}):

[{user.name}]: {body}
[{user.name}]: {body}
…
```

The "---\nComments ({n}):\n\n" header is omitted when `n == 0`.
Description is omitted when empty (no leading blank line before separator).

`buildCommentInbound` is unchanged.

## Data Flow

```
Linear GraphQL
      │  (state ∈ states ∧ label = label ∧ project ∈ projects)
      ▼
LinearClient.IssuesInStates
      │  paginated; full materialization
      ▼
LinearReceiver.tickOnce
      ├─ seenIssues.Contains(id) ─ no ─→ buildInitialInbound ─→ dispatch
      │                                  ↳ accumulate issue ID + folded comment IDs
      └─ seenIssues.Contains(id) ─ yes ─→ for each new comment ─→ buildCommentInbound ─→ dispatch
                                              ↳ accumulate comment IDs
      ▼
seenIssues.Add(newIssueIDs)
seenComments.Add(newCommentIDs)
```

## Configuration

### `config.yaml.tmpl`

```yaml
bee:
  platforms:
    linear:
      enabled: false
      api_key: ""
      label_name: openbee
      poll_interval: 10s
      projects: []
      # State name allow-list. Empty list = skip every tick. Match Linear UI
      # state names exactly (case-sensitive). Example:
      # states:
      #   - Todo
      #   - In Progress
      states: []
```

### `bee_system_configs`

| Key | Value Format | Effect |
|---|---|---|
| `linear_projects` | JSON array of strings | Existing |
| `linear_states` | JSON array of strings | New — same validator and Set semantics |

### Hot Reload

Both `linearCfg.Set` and `linearStates.Set` are called by
`SystemConfigHandler.Set` after the row is persisted. The receiver consults
both stores at the start of every tick, so changes take effect on the next
tick boundary with no restart required.

## Migration & Cold Start

Per operator decision (option B from brainstorming): no priming.

- New code starts with empty `seen_issues.json`.
- The first tick after deploy enumerates every currently-windowed issue and
  dispatches a merged initial message for each. Operators are aware of and
  accept the one-time noise.
- `cursor.json` is left in place untouched; new code never reads or writes it.
  Operators may delete it manually:

  ```sh
  rm ~/.openbee/.linear/cursor.json
  ```

- No automatic migration code is written. The cursor module is deleted
  outright; deployment is the migration boundary.

## Test Plan

`handler_test.go` must cover, with TDD (write failing test, then implement):

1. **First-sight merged dispatch.** Unknown issue with title, description, and
   three non-bot comments produces exactly one `InboundMessage` containing all
   four pieces in the documented format. `seen_issues` and `seen_comments` are
   both populated.
2. **Subsequent tick on known issue.** Same issue ID returns from the next tick
   with a fourth comment appended. Only one new `InboundMessage` is dispatched
   for the new comment. The first three comments are not re-dispatched.
3. **Bot self-comment exclusion.** A comment whose body starts with
   `[openbee-bot]` is filtered both from the merged-first dispatch and from
   subsequent per-comment dispatches.
4. **Empty config skip.** With either `projects = []` or `states = []`, the
   tick performs no GraphQL call and produces no dispatches.
5. **Pagination.** Mock client returns two pages; receiver materializes both
   before processing. No issue from page 2 is missed.
6. **Restart with primed seen sets.** Receiver starts with
   `seen_issues.json` already containing the ID of issue X. The next tick
   sees X in the GraphQL result and produces no `InboundMessage`, but does
   dispatch any of X's comments whose IDs are not in `seen_comments.json`.

`seen_issues_test.go` mirrors `seen_comments_test.go`:

- Load on missing file returns empty without error.
- Load on corrupt JSON returns empty without error.
- Add appends, deduplicates, and survives round-trip.
- Concurrent Add does not corrupt the file (atomic rename).

`store_test.go` adds equivalent `StatesStore` tests.

`system_config_handler_test.go` adds:

- PUT `/api/system-configs/linear_states` with valid array → 200, store updated.
- PUT with non-array body → 400, store unchanged.
- PUT with `[]` → 200, store cleared.

## Out of Scope

- Web SystemSettings frontend form for `linear_states`.
- Automatic deletion of `cursor.json`.
- Re-engagement on state re-entry (seen-issues set is permanent by design).
- `WorkflowState.type`-based filtering or any kind of dynamic state inference.
- End-to-end tests against the real Linear API.

## Risks & Mitigations

| Risk | Mitigation |
|---|---|
| First-tick noise after deploy | Documented and accepted; operators choose timing |
| `state.name` drift (team renames "In Progress" → "Doing") | Operator updates `linear_states` via SystemSettings; no code change required |
| Long-running issue accumulating thousands of comments | Merged dispatch carries all of them once, then per-comment thereafter — bounded by Linear's per-issue comment cap |
| `seen_issues.json` growing unbounded | Permanent only-grow set is intentional; size remains O(processed issues) which is small relative to disk |
| Wrong filter accidentally matches all backlog | `states = []` policy + explicit yaml comment guides operators; no default state list ships |
