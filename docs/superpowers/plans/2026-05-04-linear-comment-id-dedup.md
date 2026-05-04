# Linear Comment ID-Based Deduplication Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the timestamp-based comment double-check in the Linear receiver with a persistent seen-comment-ID set, and remove the GraphQL comment-level `createdAt` filter so all comments under matching issues are fetched and deduplicated by ID alone.

**Architecture:** A new `SeenComments` type (mirroring `Cursor`) manages an in-memory `map[string]struct{}` backed by `~/.openbee/.linear/seen_comments.json`. The receiver loads it at startup, checks every polled comment against it, appends dispatched IDs, and persists after each tick. The cursor narrows to its only remaining concern: which issues to fetch.

**Tech Stack:** Go standard library (`os`, `encoding/json`), same tmp+rename atomicity pattern already used in `cursor.go`.

---

## File Map

| File | Action | What changes |
|------|--------|-------------|
| `internal/platform/linear/seen_comments.go` | **Create** | `seenAPI` interface + `SeenComments` implementation |
| `internal/platform/linear/seen_comments_test.go` | **Create** | Unit tests for `SeenComments` |
| `internal/platform/linear/handler.go` | **Modify** | Add `seenComments seenAPI` field; wire in `NewPlatform`; load in `Start()`; replace timestamp check; collect + persist new IDs; remove comment `highWater` tracking |
| `internal/platform/linear/handler_test.go` | **Modify** | Add `fakeSeen`; add 2 new dedup tests; add `seenComments: newFakeSeen()` to all 7 existing `LinearReceiver` struct literals |
| `internal/platform/linear/client.go` | **Modify** | Remove `createdAt` filter from `comments` sub-query in `issuesQuery` |

---

## Task 1: Create `seen_comments.go` with failing tests first

**Files:**
- Create: `internal/platform/linear/seen_comments_test.go`
- Create: `internal/platform/linear/seen_comments.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/platform/linear/seen_comments_test.go`:

```go
package linear

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestSeenComments_LoadMissingReturnsEmpty(t *testing.T) {
	s := NewSeenComments(t.TempDir())
	if err := s.Load(context.Background()); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if s.Contains("anything") {
		t.Error("expected empty set after missing file load")
	}
}

func TestSeenComments_LoadCorruptFileTreatedAsEmpty(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "seen_comments.json"), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := NewSeenComments(dir)
	if err := s.Load(context.Background()); err != nil {
		t.Fatalf("Load corrupt: %v", err)
	}
	if s.Contains("anything") {
		t.Error("expected empty set after corrupt file")
	}
}

func TestSeenComments_ContainsReturnsFalseForUnknownID(t *testing.T) {
	s := NewSeenComments(t.TempDir())
	if err := s.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	if s.Contains("unknown-id") {
		t.Error("Contains returned true for unknown ID")
	}
}

func TestSeenComments_AddAndContainsRoundtrip(t *testing.T) {
	s := NewSeenComments(t.TempDir())
	if err := s.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := s.Add(context.Background(), []string{"id-1", "id-2"}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if !s.Contains("id-1") || !s.Contains("id-2") {
		t.Error("Contains returned false after Add")
	}
	if s.Contains("id-3") {
		t.Error("Contains returned true for unadded ID")
	}
}

func TestSeenComments_AddPersistsToDisk(t *testing.T) {
	dir := t.TempDir()
	s1 := NewSeenComments(dir)
	if err := s1.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := s1.Add(context.Background(), []string{"id-1"}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	// Fresh instance — must restore id-1 from disk.
	s2 := NewSeenComments(dir)
	if err := s2.Load(context.Background()); err != nil {
		t.Fatalf("Load reload: %v", err)
	}
	if !s2.Contains("id-1") {
		t.Error("id-1 not found after reload")
	}
}

func TestSeenComments_AddLeavesNoTempFile(t *testing.T) {
	dir := t.TempDir()
	s := NewSeenComments(dir)
	if err := s.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := s.Add(context.Background(), []string{"id-1"}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "seen_comments.json.tmp")); !os.IsNotExist(err) {
		t.Error("seen_comments.json.tmp should be removed after rename")
	}
}

func TestSeenComments_AddCreatesDir(t *testing.T) {
	parent := t.TempDir()
	dir := filepath.Join(parent, "nested", "linear")
	s := NewSeenComments(dir)
	if err := s.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := s.Add(context.Background(), []string{"id-1"}); err != nil {
		t.Fatalf("Add into missing dir: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "seen_comments.json")); err != nil {
		t.Errorf("seen_comments.json not written: %v", err)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee2
go test ./internal/platform/linear/... -run TestSeenComments -v 2>&1
```

Expected: compile error — `NewSeenComments` undefined.

- [ ] **Step 3: Create `seen_comments.go`**

Create `internal/platform/linear/seen_comments.go`:

```go
package linear

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// seenAPI is satisfied by *SeenComments and by test fakes.
type seenAPI interface {
	Load(ctx context.Context) error
	Contains(id string) bool
	Add(ctx context.Context, ids []string) error
}

// SeenComments persists the set of already-dispatched comment IDs to
// <dir>/seen_comments.json. Writes use tmp+rename for atomicity.
type SeenComments struct {
	dir string
	ids map[string]struct{}
}

// NewSeenComments constructs a SeenComments that persists to <dir>/seen_comments.json.
// Call Load before using Contains or Add.
func NewSeenComments(dir string) *SeenComments {
	return &SeenComments{dir: dir, ids: make(map[string]struct{})}
}

type seenFile struct {
	IDs []string `json:"ids"`
}

// Load reads the persisted ID set from disk. On ErrNotExist or corrupt JSON
// it silently starts with an empty set (same fallback pattern as Cursor.Load).
func (s *SeenComments) Load(_ context.Context) error {
	data, err := os.ReadFile(filepath.Join(s.dir, "seen_comments.json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var sf seenFile
	if err := json.Unmarshal(data, &sf); err != nil {
		return nil // corrupt → empty
	}
	for _, id := range sf.IDs {
		s.ids[id] = struct{}{}
	}
	return nil
}

// Contains reports whether id has already been dispatched.
func (s *SeenComments) Contains(id string) bool {
	_, ok := s.ids[id]
	return ok
}

// Add records ids as dispatched and atomically persists the full set to disk.
func (s *SeenComments) Add(_ context.Context, ids []string) error {
	for _, id := range ids {
		s.ids[id] = struct{}{}
	}
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return err
	}
	all := make([]string, 0, len(s.ids))
	for id := range s.ids {
		all = append(all, id)
	}
	data, err := json.Marshal(seenFile{IDs: all})
	if err != nil {
		return err
	}
	tmp := filepath.Join(s.dir, "seen_comments.json.tmp")
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(s.dir, "seen_comments.json"))
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/platform/linear/... -run TestSeenComments -v 2>&1
```

Expected: all 7 `TestSeenComments_*` tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/platform/linear/seen_comments.go internal/platform/linear/seen_comments_test.go
git commit -m "feat(linear): add SeenComments for comment ID-based dedup"
```

---

## Task 2: Add `fakeSeen` and new dedup tests in `handler_test.go`

**Files:**
- Modify: `internal/platform/linear/handler_test.go`

- [ ] **Step 1: Add `fakeSeen` and two new failing tests**

Open `internal/platform/linear/handler_test.go`. After the `fakeCursor` block (after line 61), insert:

```go
type fakeSeen struct {
	ids   map[string]struct{}
	added []string
}

func newFakeSeen() *fakeSeen {
	return &fakeSeen{ids: make(map[string]struct{})}
}

func (f *fakeSeen) Load(_ context.Context) error { return nil }
func (f *fakeSeen) Contains(id string) bool      { _, ok := f.ids[id]; return ok }
func (f *fakeSeen) Add(_ context.Context, ids []string) error {
	for _, id := range ids {
		f.ids[id] = struct{}{}
	}
	f.added = append(f.added, ids...)
	return nil
}
```

Then append two new test functions at the end of the file:

```go
func TestReceiver_TickOnce_SkipsAlreadySeenCommentID(t *testing.T) {
	since := mustParse(t, "2026-05-02T09:00:00Z")
	issue := Issue{
		ID:         "I1",
		Identifier: "ENG-42",
		Title:      "T",
		Team:       Team{Key: "ENG"},
		Creator:    User{ID: "U2"},
		CreatedAt:  mustParse(t, "2026-05-02T08:00:00Z"),
		UpdatedAt:  mustParse(t, "2026-05-02T11:00:00Z"),
		Labels: []IssueLabel{
			{Name: "openbee", CreatedAt: mustParse(t, "2026-05-02T08:30:00Z")},
		},
		Comments: []Comment{
			{ID: "C-seen", Body: "already dispatched", CreatedAt: mustParse(t, "2026-05-02T10:00:00Z"), User: User{ID: "U2"}},
			{ID: "C-new", Body: "new comment", CreatedAt: mustParse(t, "2026-05-02T10:30:00Z"), User: User{ID: "U2"}},
		},
	}
	fc := &fakeClient{
		viewer: User{ID: "BOT"},
		issues: func(_ time.Time) ([]Issue, bool, error) { return []Issue{issue}, false, nil },
	}
	cur := &fakeCursor{last: since}
	seen := newFakeSeen()
	seen.ids["C-seen"] = struct{}{} // pre-populate as already dispatched
	r := &LinearReceiver{client: fc, cursor: cur, labelName: "openbee", projectStore: testProjectStore(), seenComments: seen}

	var got []platform.InboundMessage
	r.tickOnce(context.Background(), func(m platform.InboundMessage) { got = append(got, m) })

	if len(got) != 1 {
		t.Fatalf("dispatched %d, want 1: %+v", len(got), got)
	}
	if got[0].PlatformMessageID != "comment:C-new" {
		t.Errorf("unexpected dispatch: %+v", got[0])
	}
}

func TestReceiver_TickOnce_AddsDispatchedIDsToSeenSet(t *testing.T) {
	since := mustParse(t, "2026-05-02T09:00:00Z")
	issue := Issue{
		ID:         "I1",
		Identifier: "ENG-42",
		Title:      "T",
		Team:       Team{Key: "ENG"},
		Creator:    User{ID: "U2"},
		CreatedAt:  mustParse(t, "2026-05-02T08:00:00Z"),
		UpdatedAt:  mustParse(t, "2026-05-02T11:00:00Z"),
		Labels: []IssueLabel{
			{Name: "openbee", CreatedAt: mustParse(t, "2026-05-02T08:30:00Z")},
		},
		Comments: []Comment{
			{ID: "C1", Body: "hello", CreatedAt: mustParse(t, "2026-05-02T10:00:00Z"), User: User{ID: "U2"}},
		},
	}
	fc := &fakeClient{
		viewer: User{ID: "BOT"},
		issues: func(_ time.Time) ([]Issue, bool, error) { return []Issue{issue}, false, nil },
	}
	cur := &fakeCursor{last: since}
	seen := newFakeSeen()
	r := &LinearReceiver{client: fc, cursor: cur, labelName: "openbee", projectStore: testProjectStore(), seenComments: seen}

	r.tickOnce(context.Background(), func(platform.InboundMessage) {})

	if !seen.Contains("C1") {
		t.Error("C1 not added to seen set after dispatch")
	}
}
```

- [ ] **Step 2: Run new tests to verify they fail**

```bash
go test ./internal/platform/linear/... -run "TestReceiver_TickOnce_SkipsAlreadySeenCommentID|TestReceiver_TickOnce_AddsDispatchedIDsToSeenSet" -v 2>&1
```

Expected: compile error — `seenComments` field not yet defined on `LinearReceiver`.

- [ ] **Step 3: Commit failing tests**

```bash
git add internal/platform/linear/handler_test.go
git commit -m "test(linear): add failing tests for ID-based comment dedup"
```

---

## Task 3: Update `handler.go` to wire `seenComments`

**Files:**
- Modify: `internal/platform/linear/handler.go`
- Modify: `internal/platform/linear/handler_test.go` (update 7 existing struct literals)

- [ ] **Step 1: Add `seenComments` field and update `NewPlatform`**

In `handler.go`, add `seenComments seenAPI` to `LinearReceiver`:

```go
type LinearReceiver struct {
	client       Client
	cursor       cursorAPI
	seenComments seenAPI
	labelName    string
	pollInterval time.Duration
	projectStore *linearcfg.Store
}
```

In `NewPlatform`, wire `NewSeenComments(dir)`:

```go
return &LinearPlatform{
	receiver: &LinearReceiver{
		client:       client,
		cursor:       NewCursor(dir),
		seenComments: NewSeenComments(dir),
		labelName:    cfg.LabelName,
		pollInterval: cfg.PollInterval,
		projectStore: projectStore,
	},
	sender: &LinearSender{client: client},
}, nil
```

- [ ] **Step 2: Load `seenComments` at startup in `Start()`**

In `Start()`, add the load call before the `Viewer` call:

```go
func (r *LinearReceiver) Start(ctx context.Context, dispatch func(platform.InboundMessage)) error {
	if err := r.seenComments.Load(ctx); err != nil {
		return fmt.Errorf("linear receiver: seen comments load: %w", err)
	}
	viewer, err := r.client.Viewer(ctx)
	if err != nil {
		return fmt.Errorf("linear receiver: viewer: %w", err)
	}
	log.Info("linear receiver started", zap.String("viewer_id", viewer.ID), zap.String("label", r.labelName))
	// ... rest unchanged
```

- [ ] **Step 3: Replace timestamp check and simplify `tickOnce`**

Replace the entire `tickOnce` body (lines 112–160) with:

```go
func (r *LinearReceiver) tickOnce(ctx context.Context, dispatch func(platform.InboundMessage)) {
	projects := r.projects()
	if len(projects) == 0 {
		return
	}
	since, err := r.cursor.Load(ctx)
	if err != nil {
		log.Error("cursor load", zap.Error(err))
		return
	}
	issues, truncated, err := r.client.IssuesUpdatedSince(ctx, since, r.labelName, projects)
	if err != nil {
		log.Error("issues fetch", zap.Error(err))
		return
	}
	highWater := since
	var newIDs []string
	for _, issue := range issues {
		if isNewlyOwned(issue, since, r.labelName) {
			dispatch(buildIssueInbound(issue))
		}
		for _, c := range issue.Comments {
			if r.seenComments.Contains(c.ID) {
				continue
			}
			if strings.HasPrefix(c.Body, "[openbee-bot]") {
				continue
			}
			dispatch(buildCommentInbound(issue, c))
			newIDs = append(newIDs, c.ID)
		}
		if issue.UpdatedAt.After(highWater) {
			highWater = issue.UpdatedAt
		}
	}
	if len(newIDs) > 0 {
		if err := r.seenComments.Add(ctx, newIDs); err != nil {
			log.Error("seen comments save", zap.Error(err))
		}
	}
	if truncated {
		return
	}
	if highWater.After(since) {
		if err := r.cursor.Save(ctx, highWater); err != nil {
			log.Error("cursor save", zap.Error(err))
		}
	}
}
```

- [ ] **Step 4: Update all 7 existing `LinearReceiver` struct literals in `handler_test.go`**

Each of the 7 places that construct `LinearReceiver` must add `seenComments: newFakeSeen()`. Find them with:

```bash
grep -n "LinearReceiver{" internal/platform/linear/handler_test.go
```

The 7 tests to update are:
1. `TestReceiver_TickOnce_DispatchesIssueAndComments` (line ~99)
2. `TestReceiver_TickOnce_ErrorDoesNotAdvanceCursor` (line ~136)
3. `TestReceiver_TickOnce_TruncatedDoesNotAdvanceCursor` (line ~157)
4. `TestReceiver_TickOnce_SkipsPrefixedSelfComment` (line ~235)
5. `TestReceiver_TickOnce_EmptyProjectsSkipsAPI` (line ~260)
6. `TestReceiver_TickOnce_ForwardsProjectsToClient` (line ~284)
7. `TestReceiver_TickOnce_DispatchesCommentContainingMarkerMidString` (line ~321)

For each, change e.g.:

```go
// Before:
r := &LinearReceiver{client: fc, cursor: cur, labelName: "openbee", projectStore: testProjectStore()}

// After:
r := &LinearReceiver{client: fc, cursor: cur, labelName: "openbee", projectStore: testProjectStore(), seenComments: newFakeSeen()}
```

- [ ] **Step 5: Run all linear tests to verify they pass**

```bash
go test ./internal/platform/linear/... -v 2>&1
```

Expected: all tests PASS including the 2 new dedup tests.

- [ ] **Step 6: Commit**

```bash
git add internal/platform/linear/handler.go internal/platform/linear/handler_test.go
git commit -m "feat(linear): replace timestamp dedup with ID-based SeenComments"
```

---

## Task 4: Remove comment `createdAt` filter from GraphQL query

**Files:**
- Modify: `internal/platform/linear/client.go`

- [ ] **Step 1: Update `issuesQuery` constant**

In `client.go`, find the `issuesQuery` constant (line ~170). Change the comments sub-query from:

```graphql
comments(filter: { createdAt: { gt: $since } }, orderBy: createdAt) {
```

to:

```graphql
comments(orderBy: createdAt) {
```

The full updated `issuesQuery` constant becomes:

```go
const issuesQuery = `
query Issues($since: DateTimeOrDuration!, $label: String!, $projects: [String!]!, $first: Int!) {
  issues(
    filter: {
      updatedAt: { gt: $since },
      labels: { name: { eq: $label } },
      project: { name: { in: $projects } }
    }
    orderBy: updatedAt
    first: $first
  ) {
    pageInfo { hasNextPage }
    nodes {
      id identifier title description createdAt updatedAt
      team { key }
      creator { id name email }
      labels(filter: { name: { eq: $label } }) {
        nodes { name createdAt }
      }
      comments(orderBy: createdAt) {
        nodes { id body createdAt user { id name email } parentId }
      }
    }
  }
}`
```

- [ ] **Step 2: Run all linear tests to verify nothing broke**

```bash
go test ./internal/platform/linear/... -v 2>&1
```

Expected: all tests PASS. The `TestClient_IssuesUpdatedSince` test doesn't assert on the comments filter string, so it passes unchanged.

- [ ] **Step 3: Commit**

```bash
git add internal/platform/linear/client.go
git commit -m "feat(linear): remove comment createdAt filter from GraphQL query"
```

---

## Task 5: Final verification

- [ ] **Step 1: Run full test suite**

```bash
go test ./... 2>&1
```

Expected: all packages PASS with no failures.

- [ ] **Step 2: Verify seen_comments.json is in .gitignore**

```bash
grep -r "seen_comments" .gitignore .gitignore.local 2>/dev/null || echo "not in gitignore (expected — it's a runtime file in ~/.openbee, not in repo)"
```

The file lives at `~/.openbee/.linear/seen_comments.json` (outside the repo), so no `.gitignore` change is needed.

- [ ] **Step 3: Commit final state if any loose files remain**

```bash
git status
```

If clean, nothing to do. The feature is complete.
