# Linear Issue Project Info Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** When a Linear issue is dispatched for the first time, prepend `[Project: <name>]` to the message content if the issue belongs to a project.

**Architecture:** Extend the `Issue` struct with an optional `*Project` field, add `project { id name }` to the GraphQL query, and update `mergeIssueContent` to prepend the project header when non-nil. No routing, SeenSet, or session changes required.

**Tech Stack:** Go, Linear GraphQL API

---

## File Map

| File | Change |
|------|--------|
| `internal/platform/linear/client.go` | Add `Project` struct; add `Project *Project` to `Issue`; add `project { id name }` to `issuesQuery`; include `Project` in node→Issue mapping |
| `internal/platform/linear/handler.go` | Update `mergeIssueContent` to prepend `[Project: <name>]\n\n` when `issue.Project != nil` |
| `internal/platform/linear/handler_test.go` | Add `TestMergeIssueContent_WithProject` and `TestMergeIssueContent_WithoutProject` |

---

## Task 1: Extend `Issue` struct, GraphQL query, and node mapping in `client.go`

**Files:**
- Modify: `internal/platform/linear/client.go`

- [ ] **Step 1: Add `Project` struct after the `Team` struct definition (around line 21)**

In `internal/platform/linear/client.go`, after the `Team` struct, add:

```go
// Project is the subset of Linear's Project type we care about.
type Project struct {
    ID   string `json:"id"`
    Name string `json:"name"`
}
```

- [ ] **Step 2: Add `Project *Project` field to the `Issue` struct**

In `internal/platform/linear/client.go`, update the `Issue` struct (currently ending at line 45) to add the Project field:

```go
// Issue is the subset of Linear's Issue type we care about.
type Issue struct {
    ID          string    `json:"id"`
    Identifier  string    `json:"identifier"` // e.g. "ENG-42"
    Title       string    `json:"title"`
    Description string    `json:"description"`
    CreatedAt   time.Time `json:"createdAt"`
    UpdatedAt   time.Time `json:"updatedAt"`
    Team        Team      `json:"team"`
    Creator     User      `json:"creator"`
    Project     *Project  `json:"project,omitempty"` // nil when issue has no project
    Comments    []Comment `json:"-"` // unwrapped from comments.nodes
}
```

- [ ] **Step 3: Add `project { id name }` to `issuesQuery`**

In `internal/platform/linear/client.go`, replace the `issuesQuery` constant (lines 160–182) with:

```go
const issuesQuery = `
query Issues($states: [String!]!, $label: String!, $projects: [String!]!, $first: Int!, $after: String) {
  issues(
    filter: {
      state:   { name: { in: $states } },
      labels:  { name: { eq: $label } },
      project: { name: { in: $projects } }
    }
    orderBy: createdAt
    first: $first
    after: $after
  ) {
    pageInfo { hasNextPage endCursor }
    nodes {
      id identifier title description createdAt updatedAt
      team { key }
      creator { id name email }
      project { id name }
      comments(orderBy: createdAt) {
        nodes { id body createdAt user { id name email } parentId }
      }
    }
  }
}`
```

- [ ] **Step 4: Add `Project` field to the anonymous node struct and map it into `Issue`**

In `IssuesInStates` (around line 200), the anonymous `Nodes` struct needs a `Project` field. Replace the Nodes struct definition:

```go
Nodes []struct {
    ID          string    `json:"id"`
    Identifier  string    `json:"identifier"`
    Title       string    `json:"title"`
    Description string    `json:"description"`
    CreatedAt   time.Time `json:"createdAt"`
    UpdatedAt   time.Time `json:"updatedAt"`
    Team        Team      `json:"team"`
    Creator     User      `json:"creator"`
    Project     *Project  `json:"project"`
    Comments    struct {
        Nodes []Comment `json:"nodes"`
    } `json:"comments"`
} `json:"nodes"`
```

Then update the issue construction (around line 224) to include Project:

```go
issue := Issue{
    ID:          n.ID,
    Identifier:  n.Identifier,
    Title:       n.Title,
    Description: n.Description,
    CreatedAt:   n.CreatedAt,
    UpdatedAt:   n.UpdatedAt,
    Team:        n.Team,
    Creator:     n.Creator,
    Project:     n.Project,
    Comments:    n.Comments.Nodes,
}
```

- [ ] **Step 5: Verify existing tests still compile and pass**

Run:
```bash
go test ./internal/platform/linear/... -v
```

Expected: All existing tests PASS. The new `Project` field is `nil` by default in all existing test fixtures, so no content changes occur.

- [ ] **Step 6: Commit**

```bash
git add internal/platform/linear/client.go
git commit -m "feat(linear): add Project field to Issue struct and GraphQL query"
```

---

## Task 2: Write failing tests for project header in `handler_test.go`

**Files:**
- Modify: `internal/platform/linear/handler_test.go`

- [ ] **Step 1: Add two test functions at the bottom of `handler_test.go`**

```go
func TestMergeIssueContent_WithProject(t *testing.T) {
    proj := &Project{ID: "P1", Name: "Backend"}
    issue := Issue{
        ID:          "I1",
        Title:       "Fix login",
        Description: "Users get 401.",
        Project:     proj,
    }
    got := mergeIssueContent(issue, nil)
    want := "[Project: Backend]\n\nFix login\n\nUsers get 401."
    if got != want {
        t.Errorf("mergeIssueContent with project mismatch.\nwant: %q\ngot:  %q", want, got)
    }
}

func TestMergeIssueContent_WithoutProject(t *testing.T) {
    issue := Issue{
        ID:          "I1",
        Title:       "Fix login",
        Description: "Users get 401.",
        Project:     nil,
    }
    got := mergeIssueContent(issue, nil)
    want := "Fix login\n\nUsers get 401."
    if got != want {
        t.Errorf("mergeIssueContent without project mismatch.\nwant: %q\ngot:  %q", want, got)
    }
}
```

- [ ] **Step 2: Run new tests to confirm they fail**

```bash
go test ./internal/platform/linear/... -v -run "TestMergeIssueContent_With"
```

Expected: `TestMergeIssueContent_WithProject` FAILS — content does not start with `[Project: Backend]`. `TestMergeIssueContent_WithoutProject` PASSES (project is nil, no change).

---

## Task 3: Update `mergeIssueContent` to prepend project header

**Files:**
- Modify: `internal/platform/linear/handler.go`

- [ ] **Step 1: Update `mergeIssueContent` to prepend the project header**

In `internal/platform/linear/handler.go`, replace the `mergeIssueContent` function (lines 236–257) with:

```go
// mergeIssueContent renders project header (if any), title, optional description,
// and the supplied non-bot comments into one body. Description is omitted when
// empty; the "Comments (N):" header is omitted when there are no non-bot comments.
func mergeIssueContent(issue Issue, comments []Comment) string {
    var b strings.Builder
    if issue.Project != nil {
        fmt.Fprintf(&b, "[Project: %s]\n\n", issue.Project.Name)
    }
    b.WriteString(issue.Title)
    if issue.Description != "" {
        b.WriteString("\n\n")
        b.WriteString(issue.Description)
    }
    if len(comments) > 0 {
        fmt.Fprintf(&b, "\n\n---\nComments (%d):\n", len(comments))
        for _, c := range comments {
            name := c.User.Name
            if name == "" {
                name = c.User.Email
            }
            if name == "" {
                name = c.User.ID
            }
            fmt.Fprintf(&b, "\n[%s]: %s", name, c.Body)
        }
    }
    return b.String()
}
```

- [ ] **Step 2: Run all tests**

```bash
go test ./internal/platform/linear/... -v
```

Expected: ALL tests PASS including:
- `TestMergeIssueContent_WithProject` — now passes, content starts with `[Project: Backend]\n\nFix login\n\nUsers get 401.`
- `TestMergeIssueContent_WithoutProject` — still passes, nil project → no header
- All pre-existing tests — still pass, their Issue fixtures have `Project: nil` so content is unchanged

- [ ] **Step 3: Commit**

```bash
git add internal/platform/linear/handler.go internal/platform/linear/handler_test.go
git commit -m "feat(linear): prepend project header on first issue dispatch"
```
