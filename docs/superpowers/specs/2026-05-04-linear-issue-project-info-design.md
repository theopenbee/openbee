# Design: Include Project Info on Linear Issue First Dispatch

**Date:** 2026-05-04  
**Status:** Approved  
**Branch:** feat/linear-platform

---

## Problem

When a Linear issue is first encountered, the bot receives a merged message containing the issue title, description, and any human comments. However, the message contains no information about which Linear **project** the issue belongs to, even though issues are always filtered by project. This means the bot lacks project context when processing new issues.

---

## Goal

When a Linear issue is dispatched for the first time, prepend a `[Project: <name>]` header to the message content — but only when the issue actually belongs to a project (the field is optional in Linear).

---

## Scope

In scope:
- Returning `project { id name }` from the GraphQL issues query
- Extending the `Issue` struct with an optional `Project` field
- Prepending a project header in `buildInitialInbound()`
- Unit test coverage for both the with-project and no-project cases

Out of scope:
- Changing `SessionKey` format or session routing
- Adding project info to follow-up comment messages
- Showing project info in any other platform or message type

---

## Design

### 1. Data Layer — `internal/platform/linear/client.go`

Add a `Project` struct and an optional `Project` field to `Issue`:

```go
type Project struct {
    ID   string `json:"id"`
    Name string `json:"name"`
}

type Issue struct {
    // ... existing fields ...
    Project *Project `json:"project,omitempty"` // nil when issue has no project
}
```

Using a pointer (`*Project`) models the nullable GraphQL field correctly: Linear returns `null` for issues without a project, which deserializes to `nil`.

### 2. Query Layer — GraphQL query in `client.go`

Extend the `issuesQuery` to return project data on each issue node:

```graphql
project {
  id
  name
}
```

No changes to query variables or pagination logic are required.

### 3. Message Layer — `internal/platform/linear/handler.go`

In `buildInitialInbound()`, prepend a project header when `issue.Project != nil`:

**Output when project exists:**
```
[Project: Backend]

Fix login

Users get 401 sporadically.

---
Comments (2):

[Alice]: Saw it on Safari too
```

**Output when no project (unchanged from today):**
```
Fix login

Users get 401 sporadically.
```

Implementation sketch:
```go
var sb strings.Builder
if issue.Project != nil {
    fmt.Fprintf(&sb, "[Project: %s]\n\n", issue.Project.Name)
}
// ... existing title / description / comments logic
```

---

## Error Handling

- If `project` is absent or `null` in the GraphQL response, `*Project` is `nil` — the header is simply omitted. No error path needed.
- No changes to retry logic, SeenSet, or debounce behavior.

---

## Testing

Update/add tests in `handler_test.go` (or equivalent):

| Case | Expected |
|------|----------|
| Issue with project | Message starts with `[Project: <name>]\n\n` |
| Issue without project (`Project == nil`) | Message starts directly with title (no project line) |
| Existing comment-only dispatch | No project header (project header is initial-message only) |

---

## Files Changed

| File | Change |
|------|--------|
| `internal/platform/linear/client.go` | Add `Project` struct; add `Project *Project` to `Issue`; extend GraphQL query |
| `internal/platform/linear/handler.go` | Conditionally prepend project header in `buildInitialInbound()` |
| `internal/platform/linear/*_test.go` | Add/update test cases for with-project and no-project scenarios |
