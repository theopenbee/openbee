# Linear State-Based Sync Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace Linear's `updatedAt > since` cursor with a state-name filter plus a persistent issue-ID dedup set. First-sight issue dispatch merges title, description, and all non-bot history comments into a single `InboundMessage`.

**Architecture:** A new `SeenIssues` mirrors `SeenComments` (only-grow set, atomic JSON file). The receiver paginates Linear by `state.name ∈ states ∧ label ∧ project ∈ projects` (no time filter), dispatches initial-merged messages for unknown issue IDs and per-comment messages for already-known issues. The state list rides the same yaml→DB-override→`linearcfg.StatesStore` hot-reload pipeline as `linear_projects`. `cursor.go` is deleted.

**Tech Stack:** Go standard library, Linear GraphQL, existing `linearcfg.Store` pattern, `bee_system_configs` SQLite table.

**Spec:** `docs/superpowers/specs/2026-05-04-linear-state-based-sync-design.md`

---

## File Map

| File | Action | What changes |
|------|--------|-------------|
| `internal/platform/linear/seen_issues.go` | **Create** | `SeenIssues` type + `seenIssuesAPI` interface |
| `internal/platform/linear/seen_issues_test.go` | **Create** | Unit tests mirroring `seen_comments_test.go` |
| `internal/domain/linearcfg/store.go` | **Modify** | Add `StatesStore` peer type (own struct, own methods) |
| `internal/domain/linearcfg/store_test.go` | **Modify** | Mirror `Store` tests for `StatesStore` |
| `internal/infra/config/config.go` | **Modify** | Add `States []string` to `LinearConfig` |
| `internal/infra/config/config.yaml.tmpl` | **Modify** | Add `states: [{{.LinearStatesYAML}}]` line under linear |
| `internal/infra/model/system_config.go` | **Modify** | Add `SystemConfigKeyLinearStates = "linear_states"` |
| `internal/platform/linear/client.go` | **Modify** | Replace `IssuesUpdatedSince` with `IssuesInStates`, paginate fully |
| `internal/platform/linear/client_test.go` | **Modify** | Adapt to new GraphQL filter and pagination |
| `internal/platform/linear/handler.go` | **Modify** | Drop cursor, isNewlyOwned, IssueUpdatedAt highWater. Add seenIssues + statesStore. Rewrite tickOnce. Add `buildInitialInbound`. New `NewPlatform` signature |
| `internal/platform/linear/handler_test.go` | **Modify** | Add fakeSeenIssues + StatesStore. Replace existing tests with state-based scenarios. Add merge-dispatch + 2nd-tick + pagination + empty-config tests |
| `internal/platform/linear/cursor.go` | **Delete** | Cursor module gone |
| `internal/platform/linear/cursor_test.go` | **Delete** | Cursor tests gone |
| `internal/api/system_config_handler.go` | **Modify** | Generalize `parseLinearProjects` → `parseStringList`. Add `linear_states` switch case. Add `*linearcfg.StatesStore` constructor parameter |
| `internal/api/system_config_handler_test.go` | **Modify** | Add `linear_states` PUT coverage |
| `internal/app/app.go` | **Modify** | Wire `linearStates`; pass through `buildPlatforms` and `buildAPIServer`; new `NewPlatform` arg |
| `internal/infra/i18n/messages.go` | **Modify** | Add `LinearStates` and `LinearStatesHelp` to `Prompt` struct |
| `internal/infra/i18n/locales/en.yaml` | **Modify** | Add English copy |
| `internal/infra/i18n/locales/zh.yaml` | **Modify** | Add Chinese copy |
| `cmd/openbee/config.go` | **Modify** | Add `LinearStates` / `LinearStatesYAML` to template vals; add wizard prompt; reuse `renderProjectsYAML` |
| `CHANGELOG.md` | **Modify** | Add `Unreleased` entry (English) |

---

## Task 1: `SeenIssues` foundation (TDD)

**Files:**
- Create: `internal/platform/linear/seen_issues_test.go`
- Create: `internal/platform/linear/seen_issues.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/platform/linear/seen_issues_test.go`:

```go
package linear

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestSeenIssues_LoadMissingReturnsEmpty(t *testing.T) {
	s := NewSeenIssues(t.TempDir())
	if err := s.Load(context.Background()); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if s.Contains("anything") {
		t.Error("expected empty set after missing file load")
	}
}

func TestSeenIssues_LoadCorruptFileTreatedAsEmpty(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "seen_issues.json"), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := NewSeenIssues(dir)
	if err := s.Load(context.Background()); err != nil {
		t.Fatalf("Load corrupt: %v", err)
	}
	if s.Contains("anything") {
		t.Error("expected empty set after corrupt file")
	}
}

func TestSeenIssues_AddAndContainsRoundtrip(t *testing.T) {
	dir := t.TempDir()
	s := NewSeenIssues(dir)
	if err := s.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := s.Add(context.Background(), []string{"I1", "I2"}); err != nil {
		t.Fatal(err)
	}
	if !s.Contains("I1") || !s.Contains("I2") {
		t.Error("Contains returned false for added IDs")
	}

	s2 := NewSeenIssues(dir)
	if err := s2.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !s2.Contains("I1") || !s2.Contains("I2") {
		t.Error("post-reload Contains false")
	}
}

func TestSeenIssues_AddEmptySliceIsNoop(t *testing.T) {
	dir := t.TempDir()
	s := NewSeenIssues(dir)
	if err := s.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := s.Add(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "seen_issues.json")); !os.IsNotExist(err) {
		t.Errorf("Add(nil) should not create file; stat err: %v", err)
	}
}
```

- [ ] **Step 2: Run tests to confirm they fail to compile**

Run: `go test ./internal/platform/linear/ -run TestSeenIssues -v`
Expected: compile error — `NewSeenIssues` undefined.

- [ ] **Step 3: Write `seen_issues.go`**

Create `internal/platform/linear/seen_issues.go`:

```go
package linear

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// seenIssuesAPI is satisfied by *SeenIssues and by test fakes.
type seenIssuesAPI interface {
	Load(ctx context.Context) error
	Contains(id string) bool
	Add(ctx context.Context, ids []string) error
}

// SeenIssues persists the set of already-dispatched issue IDs to
// <dir>/seen_issues.json. Writes use tmp+rename for atomicity.
// The set only grows; issues that leave the configured states are not removed.
type SeenIssues struct {
	dir string
	ids map[string]struct{}
}

// NewSeenIssues constructs a SeenIssues that persists to <dir>/seen_issues.json.
// Call Load before using Contains or Add.
func NewSeenIssues(dir string) *SeenIssues {
	return &SeenIssues{dir: dir, ids: make(map[string]struct{})}
}

type seenIssuesFile struct {
	IDs []string `json:"ids"`
}

// Load reads the persisted ID set. ErrNotExist or corrupt JSON yields an
// empty set silently — same fallback as SeenComments.
func (s *SeenIssues) Load(_ context.Context) error {
	data, err := os.ReadFile(filepath.Join(s.dir, "seen_issues.json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var sf seenIssuesFile
	if err := json.Unmarshal(data, &sf); err != nil {
		return nil
	}
	for _, id := range sf.IDs {
		s.ids[id] = struct{}{}
	}
	return nil
}

// Contains reports whether id has already been dispatched.
func (s *SeenIssues) Contains(id string) bool {
	_, ok := s.ids[id]
	return ok
}

// Add records ids as dispatched and atomically persists the full set.
// An empty input is a no-op (no disk write).
func (s *SeenIssues) Add(_ context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
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
	data, err := json.Marshal(seenIssuesFile{IDs: all})
	if err != nil {
		return err
	}
	tmp := filepath.Join(s.dir, "seen_issues.json.tmp")
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(s.dir, "seen_issues.json"))
}
```

- [ ] **Step 4: Run tests to confirm they pass**

Run: `go test ./internal/platform/linear/ -run TestSeenIssues -v`
Expected: PASS for all four.

- [ ] **Step 5: Commit**

```bash
git add internal/platform/linear/seen_issues.go internal/platform/linear/seen_issues_test.go
git commit -m "feat(linear): add SeenIssues for issue ID-based dedup"
```

---

## Task 2: `linearcfg.StatesStore` (TDD)

**Files:**
- Modify: `internal/domain/linearcfg/store_test.go`
- Modify: `internal/domain/linearcfg/store.go`

- [ ] **Step 1: Add failing tests**

Append to `internal/domain/linearcfg/store_test.go`:

```go
func TestStatesStore_GetReturnsCloneOfInitial(t *testing.T) {
	s := NewStatesStore([]string{"Todo", "In Progress"})
	got := s.Get()
	if len(got) != 2 || got[0] != "Todo" || got[1] != "In Progress" {
		t.Fatalf("Get returned %v", got)
	}
	got[0] = "Mutated"
	if again := s.Get(); again[0] != "Todo" {
		t.Errorf("Get did not return a defensive copy; %v", again)
	}
}

func TestStatesStore_SetReplacesAndDropsEmpty(t *testing.T) {
	s := NewStatesStore(nil)
	s.Set([]string{"Todo", "", "In Review"})
	got := s.Get()
	if len(got) != 2 || got[0] != "Todo" || got[1] != "In Review" {
		t.Fatalf("Set did not drop empty entries: %v", got)
	}
}

func TestStatesStore_NewStoreFiltersEmptyInitial(t *testing.T) {
	s := NewStatesStore([]string{"", "Todo", ""})
	got := s.Get()
	if len(got) != 1 || got[0] != "Todo" {
		t.Errorf("NewStatesStore did not filter empty initial: %v", got)
	}
}
```

- [ ] **Step 2: Run tests to confirm they fail**

Run: `go test ./internal/domain/linearcfg/ -run TestStatesStore -v`
Expected: compile error — `NewStatesStore` undefined.

- [ ] **Step 3: Append `StatesStore` to `store.go`**

Append at the end of `internal/domain/linearcfg/store.go`:

```go
// StatesStore holds the current Linear workflow-state name allow-list.
// Safe for concurrent use. Operates identically to Store but tracks state names
// instead of project names.
type StatesStore struct {
	mu     sync.RWMutex
	states []string
}

// NewStatesStore returns a StatesStore seeded with the given state names.
// Empty entries in initial are dropped.
func NewStatesStore(initial []string) *StatesStore {
	return &StatesStore{states: cloneNonEmpty(initial)}
}

// Get returns a copy of the current state name allow-list.
func (s *StatesStore) Get() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneNonEmpty(s.states)
}

// Set replaces the state name allow-list.
func (s *StatesStore) Set(states []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.states = cloneNonEmpty(states)
}
```

- [ ] **Step 4: Run tests to confirm they pass**

Run: `go test ./internal/domain/linearcfg/ -v`
Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/domain/linearcfg/store.go internal/domain/linearcfg/store_test.go
git commit -m "feat(linearcfg): add StatesStore peer to project allow-list Store"
```

---

## Task 3: Config + system_config key

**Files:**
- Modify: `internal/infra/config/config.go`
- Modify: `internal/infra/model/system_config.go`

- [ ] **Step 1: Add `States` field**

In `internal/infra/config/config.go`, modify `LinearConfig`:

```go
type LinearConfig struct {
	Enabled      bool          `yaml:"enabled"`
	APIKey       string        `yaml:"api_key"`
	LabelName    string        `yaml:"label_name"`
	PollInterval time.Duration `yaml:"poll_interval"`
	Projects     []string      `yaml:"projects"`
	States       []string      `yaml:"states"` // workflow-state name allow-list; empty = skip
}
```

Do NOT add a default in `applyDefaults()`. Empty list = skip is intentional.

- [ ] **Step 2: Add system config key**

In `internal/infra/model/system_config.go`, append:

```go
// SystemConfigKeyLinearStates is the key for the Linear workflow-state name
// allow-list. Stored as a JSON-encoded array of state name strings.
const SystemConfigKeyLinearStates = "linear_states"
```

- [ ] **Step 3: Verify the project still builds**

Run: `go build ./...`
Expected: success.

- [ ] **Step 4: Commit**

```bash
git add internal/infra/config/config.go internal/infra/model/system_config.go
git commit -m "feat(config): add LinearConfig.States and linear_states system config key"
```

---

## Task 4: GraphQL client `IssuesInStates` (TDD)

**Files:**
- Modify: `internal/platform/linear/client.go`
- Modify: `internal/platform/linear/client_test.go`

- [ ] **Step 1: Replace existing client tests with new-API tests**

Open `internal/platform/linear/client_test.go`. The existing tests target `IssuesUpdatedSince`. Replace each test with the corresponding new-API version. Read the file end-to-end first; preserve helper functions (test server fixture, JSON canned responses) but update assertions and call sites.

Concretely:

1. Rename every `IssuesUpdatedSince` call to `IssuesInStates(ctx, []string{"Todo", "In Progress"}, label, projects)`.
2. Drop the `since time.Time` argument.
3. Drop the `truncated bool` second return — the new method returns `([]Issue, error)`.
4. In the canned GraphQL response handler, assert the request body contains `"states":["Todo","In Progress"]` and does **not** contain the string `"updatedAt"`.
5. Add a new test `TestIssuesInStates_FullPagination` that returns two pages with `hasNextPage = true` then `false`, with one issue per page, and asserts the result contains both issues.

If you find a piece of test machinery in the file that refers to `since`, delete that machinery — the new code path has no time component.

- [ ] **Step 2: Run tests to confirm they fail**

Run: `go test ./internal/platform/linear/ -run TestIssues -v`
Expected: compile error — `IssuesInStates` undefined.

- [ ] **Step 3: Rewrite client query and method**

In `internal/platform/linear/client.go`, replace the `Client` interface entry and the implementation:

```go
type Client interface {
	Viewer(ctx context.Context) (User, error)
	// IssuesInStates returns every issue whose state.name is in `states`,
	// carrying label `label`, and belonging to one of the given `projects`.
	// Empty `states` or `projects` is rejected by policy at the platform layer.
	// The returned slice contains all pages materialized in chronological
	// (createdAt-ascending) order.
	IssuesInStates(ctx context.Context, states []string, label string, projects []string) ([]Issue, error)
	CreateComment(ctx context.Context, issueID, body string, parentID *string) (Comment, error)
}
```

Replace the GraphQL query constant:

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

Replace the method body with a paginated loop:

```go
func (c *httpClient) IssuesInStates(ctx context.Context, states []string, label string, projects []string) ([]Issue, error) {
	var all []Issue
	var after *string
	for {
		vars := map[string]any{
			"states":   states,
			"label":    label,
			"projects": projects,
			"first":    issuesPageSize,
			"after":    after,
		}
		var data struct {
			Issues struct {
				PageInfo struct {
					HasNextPage bool   `json:"hasNextPage"`
					EndCursor   string `json:"endCursor"`
				} `json:"pageInfo"`
				Nodes []struct {
					ID          string    `json:"id"`
					Identifier  string    `json:"identifier"`
					Title       string    `json:"title"`
					Description string    `json:"description"`
					CreatedAt   time.Time `json:"createdAt"`
					UpdatedAt   time.Time `json:"updatedAt"`
					Team        Team      `json:"team"`
					Creator     User      `json:"creator"`
					Labels      struct {
						Nodes []IssueLabel `json:"nodes"`
					} `json:"labels"`
					Comments struct {
						Nodes []Comment `json:"nodes"`
					} `json:"comments"`
				} `json:"nodes"`
			} `json:"issues"`
		}
		if err := c.do(ctx, "issues", issuesQuery, vars, &data); err != nil {
			return nil, err
		}
		log.Info("linear: graphql issues page",
			zap.Int("returned", len(data.Issues.Nodes)),
			zap.Bool("has_next_page", data.Issues.PageInfo.HasNextPage),
		)
		for _, n := range data.Issues.Nodes {
			issue := Issue{
				ID:          n.ID,
				Identifier:  n.Identifier,
				Title:       n.Title,
				Description: n.Description,
				CreatedAt:   n.CreatedAt,
				UpdatedAt:   n.UpdatedAt,
				Team:        n.Team,
				Creator:     n.Creator,
				Labels:      n.Labels.Nodes,
				Comments:    n.Comments.Nodes,
			}
			for i := range issue.Comments {
				issue.Comments[i].IssueID = issue.ID
			}
			all = append(all, issue)
		}
		if !data.Issues.PageInfo.HasNextPage {
			return all, nil
		}
		end := data.Issues.PageInfo.EndCursor
		after = &end
	}
}
```

Delete the old `IssuesUpdatedSince` method entirely.

- [ ] **Step 4: Run tests to confirm pass**

Run: `go test ./internal/platform/linear/ -run TestIssues -v`
Expected: all PASS, including `TestIssuesInStates_FullPagination`.

- [ ] **Step 5: Commit**

```bash
git add internal/platform/linear/client.go internal/platform/linear/client_test.go
git commit -m "feat(linear): replace cursor-based query with state-filter + full pagination"
```

---

## Task 5: Receiver — handler test scaffolding

This task and Task 6 together rewrite `handler.go` and `handler_test.go` and delete `cursor.go`. They are split so commits stay reviewable.

**Files:**
- Modify: `internal/platform/linear/handler_test.go`

- [ ] **Step 1: Replace `fakeClient`, drop `fakeCursor`, add `fakeSeenIssues`**

Open `internal/platform/linear/handler_test.go`. At the top of the file (where the helpers live, lines 22–80 in the current file), make these replacements:

Replace the `fakeClient` block:

```go
// fakeClient is a Client that returns canned data per call.
type fakeClient struct {
	mu           sync.Mutex
	viewer       User
	calls        int
	lastStates   []string
	lastProjects []string
	issues       func() ([]Issue, error)
	created      []struct {
		IssueID, Body string
		ParentID      *string
	}
}

func (f *fakeClient) Viewer(ctx context.Context) (User, error) { return f.viewer, nil }

func (f *fakeClient) IssuesInStates(ctx context.Context, states []string, label string, projects []string) ([]Issue, error) {
	f.mu.Lock()
	f.calls++
	f.lastStates = append([]string(nil), states...)
	f.lastProjects = append([]string(nil), projects...)
	f.mu.Unlock()
	return f.issues()
}

func (f *fakeClient) CreateComment(ctx context.Context, issueID, body string, parentID *string) (Comment, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.created = append(f.created, struct {
		IssueID, Body string
		ParentID      *string
	}{issueID, body, parentID})
	return Comment{ID: "C-new"}, nil
}
```

Delete the entire `fakeCursor` block (lines starting `type fakeCursor struct` through its `Save` method).

Rename `fakeSeen` → `fakeSeenComments` (search-and-replace within this file only). The existing `newFakeSeen` becomes `newFakeSeenComments`. Keep its implementation.

Add a parallel fake for issues right below:

```go
type fakeSeenIssues struct {
	ids   map[string]struct{}
	added []string
}

func newFakeSeenIssues() *fakeSeenIssues {
	return &fakeSeenIssues{ids: make(map[string]struct{})}
}

func (f *fakeSeenIssues) Load(_ context.Context) error { return nil }
func (f *fakeSeenIssues) Contains(id string) bool      { _, ok := f.ids[id]; return ok }
func (f *fakeSeenIssues) Add(_ context.Context, ids []string) error {
	for _, id := range ids {
		f.ids[id] = struct{}{}
	}
	f.added = append(f.added, ids...)
	return nil
}
```

Add a helper for state store:

```go
func testStatesStore() *linearcfg.StatesStore {
	return linearcfg.NewStatesStore([]string{"Todo", "In Progress"})
}
```

- [ ] **Step 2: Replace existing tests with new scenarios**

Delete every existing `TestReceiver_*` and `TestIsNewlyOwned_*` test in this file. They tested the cursor model and are replaced by the suite below.

> Some legacy tests reused `mustParse` — keep that helper if you want timestamps in fixtures, but the receiver no longer cares about timestamps for filtering.

Add the new test suite (at the end of `handler_test.go`):

```go
func TestReceiver_TickOnce_FirstSightDispatchesMergedInitial(t *testing.T) {
	bot := User{ID: "BOT"}
	issue := Issue{
		ID:          "I1",
		Identifier:  "ENG-42",
		Title:       "Fix login",
		Description: "Users get 401 sporadically.",
		Team:        Team{Key: "ENG"},
		Creator:     User{ID: "U2", Name: "Alice"},
		Comments: []Comment{
			{ID: "C1", Body: "Saw it on Safari too", User: User{ID: "U2", Name: "Alice"}, IssueID: "I1"},
			{ID: "C-bot", Body: "[openbee-bot]\n\nignore me", User: bot, IssueID: "I1"},
			{ID: "C2", Body: "Probably the cookie domain", User: User{ID: "U3", Name: "Bob"}, IssueID: "I1"},
		},
	}
	fc := &fakeClient{
		viewer: bot,
		issues: func() ([]Issue, error) { return []Issue{issue}, nil },
	}
	seenIssues := newFakeSeenIssues()
	seenComments := newFakeSeenComments()

	r := &LinearReceiver{
		client:       fc,
		seenIssues:   seenIssues,
		seenComments: seenComments,
		labelName:    "openbee",
		pollInterval: time.Hour,
		projectStore: testProjectStore(),
		statesStore:  testStatesStore(),
	}

	var received []platform.InboundMessage
	r.tickOnce(context.Background(), func(m platform.InboundMessage) { received = append(received, m) })

	if len(received) != 1 {
		t.Fatalf("expected exactly 1 InboundMessage, got %d", len(received))
	}
	got := received[0]
	if got.PlatformMessageID != "issue:I1" {
		t.Errorf("PlatformMessageID = %q", got.PlatformMessageID)
	}
	wantContent := "Fix login\n\nUsers get 401 sporadically.\n\n---\nComments (2):\n\n[Alice]: Saw it on Safari too\n[Bob]: Probably the cookie domain"
	if got.Content != wantContent {
		t.Errorf("Content mismatch.\nwant:\n%s\n\ngot:\n%s", wantContent, got.Content)
	}
	if !seenIssues.Contains("I1") {
		t.Error("seenIssues missing I1 after dispatch")
	}
	if !seenComments.Contains("C1") || !seenComments.Contains("C2") {
		t.Error("seenComments missing folded comment IDs after merged dispatch")
	}
	if seenComments.Contains("C-bot") {
		t.Error("seenComments wrongly contains bot comment ID")
	}
}

func TestReceiver_TickOnce_KnownIssueDispatchesNewCommentsOnly(t *testing.T) {
	bot := User{ID: "BOT"}
	issue := Issue{
		ID:         "I1",
		Identifier: "ENG-42",
		Title:      "Fix login",
		Team:       Team{Key: "ENG"},
		Creator:    User{ID: "U2"},
		Comments: []Comment{
			{ID: "C1", Body: "old comment", User: User{ID: "U2"}, IssueID: "I1"},
			{ID: "C2", Body: "new comment", User: User{ID: "U3"}, IssueID: "I1"},
		},
	}
	fc := &fakeClient{
		viewer: bot,
		issues: func() ([]Issue, error) { return []Issue{issue}, nil },
	}
	seenIssues := newFakeSeenIssues()
	seenIssues.ids["I1"] = struct{}{}
	seenComments := newFakeSeenComments()
	seenComments.ids["C1"] = struct{}{}

	r := &LinearReceiver{
		client:       fc,
		seenIssues:   seenIssues,
		seenComments: seenComments,
		labelName:    "openbee",
		pollInterval: time.Hour,
		projectStore: testProjectStore(),
		statesStore:  testStatesStore(),
	}

	var received []platform.InboundMessage
	r.tickOnce(context.Background(), func(m platform.InboundMessage) { received = append(received, m) })

	if len(received) != 1 {
		t.Fatalf("expected 1 InboundMessage for new comment, got %d", len(received))
	}
	if received[0].PlatformMessageID != "comment:C2" {
		t.Errorf("expected comment:C2, got %s", received[0].PlatformMessageID)
	}
}

func TestReceiver_TickOnce_BotCommentExcludedFromMergedAndPerComment(t *testing.T) {
	bot := User{ID: "BOT"}
	// Issue is unknown — merged dispatch path.
	issueA := Issue{
		ID: "IA", Identifier: "ENG-1", Title: "A", Team: Team{Key: "ENG"}, Creator: User{ID: "U2"},
		Comments: []Comment{
			{ID: "C-bot-A", Body: "[openbee-bot]\n\nx", User: bot, IssueID: "IA"},
		},
	}
	// Issue is known — per-comment dispatch path.
	issueB := Issue{
		ID: "IB", Identifier: "ENG-2", Title: "B", Team: Team{Key: "ENG"}, Creator: User{ID: "U2"},
		Comments: []Comment{
			{ID: "C-bot-B", Body: "[openbee-bot]\n\ny", User: bot, IssueID: "IB"},
		},
	}
	fc := &fakeClient{viewer: bot, issues: func() ([]Issue, error) { return []Issue{issueA, issueB}, nil }}
	seenIssues := newFakeSeenIssues()
	seenIssues.ids["IB"] = struct{}{}

	r := &LinearReceiver{
		client:       fc,
		seenIssues:   seenIssues,
		seenComments: newFakeSeenComments(),
		labelName:    "openbee",
		pollInterval: time.Hour,
		projectStore: testProjectStore(),
		statesStore:  testStatesStore(),
	}

	var received []platform.InboundMessage
	r.tickOnce(context.Background(), func(m platform.InboundMessage) { received = append(received, m) })

	// Issue A: merged dispatch but with zero non-bot comments → still dispatch
	// the title/description (the issue itself is new).
	if len(received) != 1 {
		t.Fatalf("expected exactly 1 dispatch (issue A initial, no per-comment), got %d", len(received))
	}
	if received[0].PlatformMessageID != "issue:IA" {
		t.Errorf("got %s", received[0].PlatformMessageID)
	}
}

func TestReceiver_TickOnce_EmptyStatesSkipsTick(t *testing.T) {
	fc := &fakeClient{viewer: User{ID: "BOT"}, issues: func() ([]Issue, error) {
		t.Fatal("issues should not be queried when states is empty")
		return nil, nil
	}}
	r := &LinearReceiver{
		client:       fc,
		seenIssues:   newFakeSeenIssues(),
		seenComments: newFakeSeenComments(),
		labelName:    "openbee",
		pollInterval: time.Hour,
		projectStore: testProjectStore(),
		statesStore:  linearcfg.NewStatesStore(nil), // empty
	}
	r.tickOnce(context.Background(), func(platform.InboundMessage) {})
}

func TestReceiver_TickOnce_EmptyProjectsSkipsTick(t *testing.T) {
	fc := &fakeClient{viewer: User{ID: "BOT"}, issues: func() ([]Issue, error) {
		t.Fatal("issues should not be queried when projects is empty")
		return nil, nil
	}}
	r := &LinearReceiver{
		client:       fc,
		seenIssues:   newFakeSeenIssues(),
		seenComments: newFakeSeenComments(),
		labelName:    "openbee",
		pollInterval: time.Hour,
		projectStore: linearcfg.NewStore(nil), // empty
		statesStore:  testStatesStore(),
	}
	r.tickOnce(context.Background(), func(platform.InboundMessage) {})
}

func TestReceiver_TickOnce_MergedFormatOmitsCommentsHeaderWhenZero(t *testing.T) {
	issue := Issue{
		ID: "I1", Identifier: "ENG-42",
		Title: "Title only", Description: "Body line",
		Team: Team{Key: "ENG"}, Creator: User{ID: "U2"},
	}
	fc := &fakeClient{viewer: User{ID: "BOT"}, issues: func() ([]Issue, error) { return []Issue{issue}, nil }}
	r := &LinearReceiver{
		client:       fc,
		seenIssues:   newFakeSeenIssues(),
		seenComments: newFakeSeenComments(),
		labelName:    "openbee",
		pollInterval: time.Hour,
		projectStore: testProjectStore(),
		statesStore:  testStatesStore(),
	}
	var got []platform.InboundMessage
	r.tickOnce(context.Background(), func(m platform.InboundMessage) { got = append(got, m) })
	if len(got) != 1 {
		t.Fatalf("got %d", len(got))
	}
	if got[0].Content != "Title only\n\nBody line" {
		t.Errorf("merged content with no comments should equal title+desc only; got %q", got[0].Content)
	}
}

func TestReceiver_TickOnce_MergedFormatOmitsDescriptionWhenEmpty(t *testing.T) {
	issue := Issue{
		ID: "I1", Identifier: "ENG-42",
		Title: "Title only",
		Team:  Team{Key: "ENG"}, Creator: User{ID: "U2"},
		Comments: []Comment{
			{ID: "C1", Body: "hi", User: User{ID: "U2", Name: "Alice"}, IssueID: "I1"},
		},
	}
	fc := &fakeClient{viewer: User{ID: "BOT"}, issues: func() ([]Issue, error) { return []Issue{issue}, nil }}
	r := &LinearReceiver{
		client:       fc,
		seenIssues:   newFakeSeenIssues(),
		seenComments: newFakeSeenComments(),
		labelName:    "openbee",
		pollInterval: time.Hour,
		projectStore: testProjectStore(),
		statesStore:  testStatesStore(),
	}
	var got []platform.InboundMessage
	r.tickOnce(context.Background(), func(m platform.InboundMessage) { got = append(got, m) })
	if len(got) != 1 {
		t.Fatalf("got %d", len(got))
	}
	want := "Title only\n\n---\nComments (1):\n\n[Alice]: hi"
	if got[0].Content != want {
		t.Errorf("merged content mismatch.\nwant: %q\ngot:  %q", want, got[0].Content)
	}
}

```

After the rewrite, the new tests do not use `time.Time`, `json.Marshal`, `errors.New`, `sort.Strings`, or `mustParse`. The Go compiler will report unused imports and (top-level functions are fine to leave dangling, but you can delete `mustParse` for cleanliness). Trim the import block down to:

```go
import (
    "context"
    "sync"
    "testing"
    "time"

    "github.com/theopenbee/openbee/internal/domain/linearcfg"
    "github.com/theopenbee/openbee/internal/platform"
)
```

`time` stays for `time.Hour` in `pollInterval`. Delete `mustParse` if unused.

- [ ] **Step 3: Run tests to confirm they fail**

Run: `go test ./internal/platform/linear/ -run TestReceiver -v`
Expected: compile error (because `LinearReceiver` doesn't have `seenIssues`, `statesStore` yet, and `IssuesInStates` doesn't exist on the interface used by the receiver).

> Compile errors are progress. Task 6 makes them pass.

- [ ] **Step 4: Commit**

```bash
git add internal/platform/linear/handler_test.go
git commit -m "test(linear): rewrite receiver tests for state-based sync model"
```

---

## Task 6: Receiver — implement state-based tickOnce

**Files:**
- Modify: `internal/platform/linear/handler.go`
- Delete: `internal/platform/linear/cursor.go`
- Delete: `internal/platform/linear/cursor_test.go`

- [ ] **Step 1: Rewrite `handler.go` end-to-end**

Replace `internal/platform/linear/handler.go` with:

```go
package linear

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/theopenbee/openbee/internal/domain/linearcfg"
	"github.com/theopenbee/openbee/internal/infra/config"
	"github.com/theopenbee/openbee/internal/infra/logger"
	"github.com/theopenbee/openbee/internal/platform"
	"github.com/theopenbee/openbee/internal/utils"
)

// PlatformID is the platform identifier used in SessionKey and ingest routing.
const PlatformID = "linear"

// selfMarker is prepended to every outbound comment body. The receiver
// recognises bot-authored comments by checking HasPrefix(body, "[openbee-bot]").
const selfMarker = "[openbee-bot]\n\n"

var log = logger.With(zap.String("component", "linear"))

// LinearPlatform implements platform.Platform.
type LinearPlatform struct {
	receiver *LinearReceiver
	sender   *LinearSender
}

// NewPlatform constructs a Linear platform from configuration. Persistent
// state (seen_issues.json, seen_comments.json) lives in ~/.openbee/.linear/.
// projectStore and statesStore are consulted on every poll tick so runtime
// updates from SystemSettings take effect on the next cycle.
func NewPlatform(cfg config.LinearConfig, projectStore *linearcfg.Store, statesStore *linearcfg.StatesStore) (platform.Platform, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("linear: resolve home dir: %w", err)
	}
	dir := filepath.Join(home, ".openbee", ".linear")
	client := NewClient(cfg.APIKey)
	return &LinearPlatform{
		receiver: &LinearReceiver{
			client:       client,
			seenIssues:   NewSeenIssues(dir),
			seenComments: NewSeenComments(dir),
			labelName:    cfg.LabelName,
			pollInterval: cfg.PollInterval,
			projectStore: projectStore,
			statesStore:  statesStore,
		},
		sender: &LinearSender{client: client},
	}, nil
}

func (p *LinearPlatform) ID() string                                 { return PlatformID }
func (p *LinearPlatform) Receiver() platform.PlatformReceiverAdapter { return p.receiver }
func (p *LinearPlatform) Sender() platform.PlatformSenderAdapter     { return p.sender }

// LinearReceiver polls Linear for issue/comment updates by workflow-state.
type LinearReceiver struct {
	client       Client
	seenIssues   seenIssuesAPI
	seenComments seenAPI
	labelName    string
	pollInterval time.Duration
	projectStore *linearcfg.Store
	statesStore  *linearcfg.StatesStore
}

// Start runs the polling loop until ctx is cancelled.
func (r *LinearReceiver) Start(ctx context.Context, dispatch func(platform.InboundMessage)) error {
	if err := r.seenIssues.Load(ctx); err != nil {
		return fmt.Errorf("linear receiver: seen issues load: %w", err)
	}
	if err := r.seenComments.Load(ctx); err != nil {
		return fmt.Errorf("linear receiver: seen comments load: %w", err)
	}
	viewer, err := r.client.Viewer(ctx)
	if err != nil {
		return fmt.Errorf("linear receiver: viewer: %w", err)
	}
	log.Info("linear receiver started",
		zap.String("viewer_id", viewer.ID),
		zap.String("label", r.labelName),
	)

	ticker := time.NewTicker(r.pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			r.tickOnce(ctx, dispatch)
		}
	}
}

func (r *LinearReceiver) projects() []string {
	if r.projectStore == nil {
		return nil
	}
	return r.projectStore.Get()
}

func (r *LinearReceiver) states() []string {
	if r.statesStore == nil {
		return nil
	}
	return r.statesStore.Get()
}

func (r *LinearReceiver) tickOnce(ctx context.Context, dispatch func(platform.InboundMessage)) {
	projects := r.projects()
	states := r.states()
	if len(projects) == 0 || len(states) == 0 {
		return
	}
	log.Info("tick: start",
		zap.Strings("projects", projects),
		zap.Strings("states", states),
		zap.String("label", r.labelName),
	)
	issues, err := r.client.IssuesInStates(ctx, states, r.labelName, projects)
	if err != nil {
		log.Error("issues fetch", zap.Error(err))
		return
	}
	identifiers := make([]string, 0, len(issues))
	for _, i := range issues {
		identifiers = append(identifiers, i.Identifier)
	}
	log.Info("tick: api result",
		zap.Int("issue_count", len(issues)),
		zap.Strings("identifiers", identifiers),
	)

	var newIssueIDs []string
	var newCommentIDs []string

	for _, issue := range issues {
		if !r.seenIssues.Contains(issue.ID) {
			nonBot := nonBotComments(issue.Comments)
			log.Info("tick: dispatch initial merged",
				zap.String("identifier", issue.Identifier),
				zap.String("issue_id", issue.ID),
				zap.Int("non_bot_comment_count", len(nonBot)),
			)
			dispatch(buildInitialInbound(issue, nonBot))
			newIssueIDs = append(newIssueIDs, issue.ID)
			for _, c := range nonBot {
				newCommentIDs = append(newCommentIDs, c.ID)
			}
			continue
		}
		for _, c := range issue.Comments {
			if r.seenComments.Contains(c.ID) {
				continue
			}
			if strings.HasPrefix(c.Body, "[openbee-bot]") {
				continue
			}
			log.Info("tick: dispatch comment",
				zap.String("identifier", issue.Identifier),
				zap.String("comment_id", c.ID),
				zap.String("user_id", c.User.ID),
			)
			dispatch(buildCommentInbound(issue, c))
			newCommentIDs = append(newCommentIDs, c.ID)
		}
	}

	if len(newIssueIDs) > 0 {
		if err := r.seenIssues.Add(ctx, newIssueIDs); err != nil {
			log.Error("seen issues save", zap.Error(err))
		}
	}
	if len(newCommentIDs) > 0 {
		if err := r.seenComments.Add(ctx, newCommentIDs); err != nil {
			log.Error("seen comments save", zap.Error(err))
		}
	}
}

func nonBotComments(in []Comment) []Comment {
	out := make([]Comment, 0, len(in))
	for _, c := range in {
		if strings.HasPrefix(c.Body, "[openbee-bot]") {
			continue
		}
		out = append(out, c)
	}
	return out
}

func buildSessionKey(teamKey, identifier string) string {
	return fmt.Sprintf("%s:%s:%s", PlatformID, teamKey, identifier)
}

// replyTarget is serialized into InboundMessage.Raw so the sender can resolve
// the comment target without re-querying Linear.
type replyTarget struct {
	IssueID         string  `json:"issue_id"`
	ParentCommentID *string `json:"parent_comment_id,omitempty"`
}

// buildInitialInbound merges title, description, and the supplied non-bot
// comments into a single InboundMessage. The reply target is the issue itself
// (no parent comment), so the bee's reply lands at the top level of the issue.
func buildInitialInbound(issue Issue, comments []Comment) platform.InboundMessage {
	raw, _ := json.Marshal(replyTarget{IssueID: issue.ID})
	content := mergeIssueContent(issue, comments)
	createdAt := issue.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now()
	}
	return platform.InboundMessage{
		Platform:          PlatformID,
		SenderID:          issue.Creator.ID,
		SessionKey:        buildSessionKey(issue.Team.Key, issue.Identifier),
		Content:           content,
		RawContent:        content,
		Raw:               string(raw),
		PlatformMessageID: "issue:" + issue.ID,
		MessageTime:       createdAt.UnixMilli(),
	}
}

// mergeIssueContent renders the merged-initial body. Format (option A from
// the design doc):
//
//	{title}
//
//	{description}        ← omitted when empty
//
//	---                  ← omitted when no non-bot comments
//	Comments (N):
//
//	[user.name]: body
//	[user.name]: body
func mergeIssueContent(issue Issue, comments []Comment) string {
	var b strings.Builder
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

func buildCommentInbound(issue Issue, c Comment) platform.InboundMessage {
	parent := c.ParentID
	if parent == nil {
		id := c.ID
		parent = &id
	}
	raw, _ := json.Marshal(replyTarget{IssueID: issue.ID, ParentCommentID: parent})
	return platform.InboundMessage{
		Platform:          PlatformID,
		SenderID:          c.User.ID,
		SessionKey:        buildSessionKey(issue.Team.Key, issue.Identifier),
		Content:           c.Body,
		RawContent:        c.Body,
		Raw:               string(raw),
		PlatformMessageID: "comment:" + c.ID,
		MessageTime:       c.CreatedAt.UnixMilli(),
	}
}

// LinearSender posts replies as Linear comments.
type LinearSender struct {
	client Client
}

func (s *LinearSender) Send(ctx context.Context, msg platform.OutboundMessage) error {
	if msg.MediaPath != "" {
		return errors.New("linear: media attachments not supported in v0")
	}
	var target replyTarget
	if err := json.Unmarshal([]byte(msg.ReplyTo.Raw), &target); err != nil {
		return fmt.Errorf("linear: parse reply target: %w", err)
	}
	if target.IssueID == "" {
		return errors.New("linear: reply target missing issue_id")
	}
	return utils.RetryWithBackoff(ctx, func() error {
		_, err := s.client.CreateComment(ctx, target.IssueID, selfMarker+msg.Content, target.ParentCommentID)
		return err
	}, utils.DefaultRetryCount, utils.DefaultRetryDelay)
}

var _ platform.Platform = (*LinearPlatform)(nil)
var _ platform.PlatformReceiverAdapter = (*LinearReceiver)(nil)
var _ platform.PlatformSenderAdapter = (*LinearSender)(nil)
```

- [ ] **Step 2: Delete the cursor module**

```bash
git rm internal/platform/linear/cursor.go internal/platform/linear/cursor_test.go
```

- [ ] **Step 3: Run the receiver tests to confirm pass**

Run: `go test ./internal/platform/linear/ -run TestReceiver -v`
Expected: all `TestReceiver_*` tests PASS.

- [ ] **Step 4: Run the full package tests to catch any leftovers**

Run: `go test ./internal/platform/linear/ -v`
Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/platform/linear/handler.go
git commit -m "feat(linear): replace cursor with state-based sync and merged-initial dispatch"
```

---

## Task 7: System config handler — `linear_states` route (TDD)

**Files:**
- Modify: `internal/api/system_config_handler.go`
- Modify: `internal/api/system_config_handler_test.go`

- [ ] **Step 1: Add failing tests**

Append to `internal/api/system_config_handler_test.go` (alongside the existing `linear_projects` PUT tests):

```go
func TestSystemConfigHandler_SetLinearStates_Valid(t *testing.T) {
	store := newFakeSystemConfigStore()
	statesStore := linearcfg.NewStatesStore(nil)
	h := NewSystemConfigHandler(store, fakeEngineValidator{}, enginecfg.NewStore("claude"), linearcfg.NewStore(nil), statesStore)
	router := gin.New()
	router.PUT("/api/system-configs/:key", h.Set)

	body := []byte(`{"value":"[\"Todo\",\"In Progress\"]"}`)
	req := httptest.NewRequest(http.MethodPut, "/api/system-configs/"+model.SystemConfigKeyLinearStates, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	if got := store.vals[model.SystemConfigKeyLinearStates]; got != `["Todo","In Progress"]` {
		t.Errorf("stored value = %q", got)
	}
	if got := statesStore.Get(); len(got) != 2 || got[0] != "Todo" {
		t.Errorf("statesStore.Get() = %v", got)
	}
}

func TestSystemConfigHandler_SetLinearStates_Empty(t *testing.T) {
	store := newFakeSystemConfigStore()
	statesStore := linearcfg.NewStatesStore([]string{"Todo"})
	h := NewSystemConfigHandler(store, fakeEngineValidator{}, enginecfg.NewStore("claude"), linearcfg.NewStore(nil), statesStore)
	router := gin.New()
	router.PUT("/api/system-configs/:key", h.Set)

	body := []byte(`{"value":"[]"}`)
	req := httptest.NewRequest(http.MethodPut, "/api/system-configs/"+model.SystemConfigKeyLinearStates, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	if got := statesStore.Get(); len(got) != 0 {
		t.Errorf("expected empty after clear, got %v", got)
	}
}

func TestSystemConfigHandler_SetLinearStates_Malformed(t *testing.T) {
	store := newFakeSystemConfigStore()
	statesStore := linearcfg.NewStatesStore(nil)
	h := NewSystemConfigHandler(store, fakeEngineValidator{}, enginecfg.NewStore("claude"), linearcfg.NewStore(nil), statesStore)
	router := gin.New()
	router.PUT("/api/system-configs/:key", h.Set)

	body := []byte(`{"value":"not json"}`)
	req := httptest.NewRequest(http.MethodPut, "/api/system-configs/"+model.SystemConfigKeyLinearStates, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}
```

> Each existing test that calls `NewSystemConfigHandler(store, validator, engineCfg, linearCfg)` needs an extra `statesStore` argument. Search-and-replace within `system_config_handler_test.go` to add `linearcfg.NewStatesStore(nil)` as the final argument.

- [ ] **Step 2: Run tests to confirm they fail**

Run: `go test ./internal/api/ -run TestSystemConfigHandler -v`
Expected: compile error — `NewSystemConfigHandler` signature mismatch / `linear_states` case unknown.

- [ ] **Step 3: Update `system_config_handler.go`**

Edit `internal/api/system_config_handler.go`:

1. Add `statesStore *linearcfg.StatesStore` field to `SystemConfigHandler`.
2. Update `NewSystemConfigHandler` signature: `(store sysConfigStore, validator engineValidatorForSys, engineCfg *enginecfg.Store, linearCfg *linearcfg.Store, statesStore *linearcfg.StatesStore)`.
3. Add `model.SystemConfigKeyLinearStates` to the keys list in `Get`.
4. Generalize `parseLinearProjects` to `parseStringList(value string) ([]string, error)` (move it next to `errInvalidLinearProjects` and rename the error to `errInvalidStringList = errStringList("value must be a JSON array of non-empty strings")`). Both `linear_projects` and `linear_states` cases call `parseStringList`.
5. Add a switch case for `linear_states` mirroring `linear_projects`:

```go
case model.SystemConfigKeyLinearStates:
    states, err := parseStringList(req.Value)
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }
    if err := h.store.Set(c.Request.Context(), key, req.Value); err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    if h.statesStore != nil {
        h.statesStore.Set(states)
    }
```

The full diff is straightforward; preserve all existing behavior for `linear_projects` (it now calls `parseStringList` instead of `parseLinearProjects`).

- [ ] **Step 4: Run tests to confirm pass**

Run: `go test ./internal/api/ -run TestSystemConfigHandler -v`
Expected: all PASS, including the three new `LinearStates` tests.

- [ ] **Step 5: Commit**

```bash
git add internal/api/system_config_handler.go internal/api/system_config_handler_test.go
git commit -m "feat(api): wire linear_states system config key"
```

---

## Task 8: Wire `linearStates` through `app.go` and `buildPlatforms`

**Files:**
- Modify: `internal/app/app.go`

- [ ] **Step 1: Construct `linearStates` next to `linearCfg`**

In `internal/app/app.go`, locate the block at line 135 that constructs `linearCfg`. Immediately after that block, add a parallel one:

```go
// Initialize the Linear workflow-state allow-list from yaml, then override
// with DB if the system config has been written.
linearStates := linearcfg.NewStatesStore(cfg.Bee.Platforms.Linear.States)
if dbStates, found, err := s.systemConfigStore.Get(context.Background(), model.SystemConfigKeyLinearStates); err != nil {
    logger.Warn("failed to load linear states from DB, falling back to config", zap.Error(err))
} else if found {
    var raw []string
    if err := json.Unmarshal([]byte(dbStates.Value), &raw); err != nil {
        logger.Warn("DB linear states value is not a JSON array, falling back to config",
            zap.String("db_value", dbStates.Value), zap.Error(err))
    } else {
        linearStates.Set(raw)
    }
}
```

- [ ] **Step 2: Thread `linearStates` to `buildPlatforms` and `buildAPIServer`**

Find every call site of `buildPlatforms` and `buildAPIServer` and add `linearStates` to the argument list. Update the function signatures:

```go
func buildPlatforms(
    fc config.FeishuConfig,
    dc config.DingTalkConfig,
    wc config.WeComConfig,
    tc config.TelegramConfig,
    wxc config.WeixinConfig,
    lc config.LinearConfig,
    linearCfg *linearcfg.Store,
    linearStates *linearcfg.StatesStore,
    mc config.MediaConfig,
) ([]platform.Platform, error) {
```

Inside, the linear branch becomes:

```go
if lc.Enabled {
    p, err := linear.NewPlatform(lc, linearCfg, linearStates)
    if err != nil {
        return nil, fmt.Errorf("init linear platform: %w", err)
    }
    result = append(result, p)
}
```

`buildAPIServer` adds `linearStates *linearcfg.StatesStore` as a parameter and passes it to `api.NewSystemConfigHandler`:

```go
SystemConfigs: api.NewSystemConfigHandler(s.systemConfigStore, mgr, engineCfg, linearCfg, linearStates),
```

- [ ] **Step 3: Build to verify**

Run: `go build ./...`
Expected: success.

- [ ] **Step 4: Run all touched packages' tests**

Run: `go test ./internal/app/... ./internal/api/... ./internal/platform/linear/... ./internal/domain/linearcfg/...`
Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/app/app.go
git commit -m "feat(app): wire Linear states store through platform and API constructors"
```

---

## Task 9: Config init wizard + yaml template + i18n

**Files:**
- Modify: `internal/infra/config/config.yaml.tmpl`
- Modify: `cmd/openbee/config.go`
- Modify: `internal/infra/i18n/messages.go`
- Modify: `internal/infra/i18n/locales/en.yaml`
- Modify: `internal/infra/i18n/locales/zh.yaml`

- [ ] **Step 1: Add `states:` line to yaml template**

In `internal/infra/config/config.yaml.tmpl`, find the linear block and append below `projects:`:

```yaml
      projects: [{{.LinearProjectsYAML}}]
      states: [{{.LinearStatesYAML}}]
```

- [ ] **Step 2: Add template values + wizard prompt**

In `cmd/openbee/config.go`:

1. In the struct around line 85, add fields:

```go
LinearStates     string // comma-separated user input
LinearStatesYAML string // rendered into the YAML inline list
```

2. Around line 174 (where `LinearProjects` is initialized from the existing config), add:

```go
LinearStates: strings.Join(cfg.Bee.Platforms.Linear.States, ","),
```

3. Around line 556 (where the `LinearProjects` survey lives), add a parallel survey block immediately after:

```go
fmt.Println(i18n.M.Prompt.LinearStatesHelp)
if err := survey.AskOne(&survey.Input{
    Message: i18n.M.Prompt.LinearStates,
    Default: vals.LinearStates,
}, &vals.LinearStates); err != nil {
    return handleSurveyErr(err)
}
```

4. Around line 741, add: `vals.LinearStatesYAML = renderProjectsYAML(vals.LinearStates)` (the helper is generic over comma-separated → quoted YAML inline list — reuse it; do not write a near-duplicate).

- [ ] **Step 3: Add i18n keys**

In `internal/infra/i18n/messages.go`, after the `LinearProjectsHelp` line:

```go
LinearStates       string `yaml:"linear_states"`
LinearStatesHelp   string `yaml:"linear_states_help"`
```

In `internal/infra/i18n/locales/en.yaml`, after the `linear_projects` lines:

```yaml
  linear_states: "Linear workflow-state allow-list (comma-separated state names; empty = process nothing):"
  linear_states_help: "Issues whose state name is not in this list are skipped, even if they carry the gating label."
```

In `internal/infra/i18n/locales/zh.yaml`, after the `linear_projects` lines:

```yaml
  linear_states: "Linear 状态白名单（按状态名逗号分隔；为空则不处理任何 issue）："
  linear_states_help: "状态名不在该列表中的 issue 即使携带触发标签也会被忽略。"
```

- [ ] **Step 4: Verify the project still builds and tests pass**

Run: `go build ./... && go test ./internal/infra/i18n/... ./cmd/openbee/...`
Expected: success.

- [ ] **Step 5: Commit**

```bash
git add internal/infra/config/config.yaml.tmpl cmd/openbee/config.go internal/infra/i18n/messages.go internal/infra/i18n/locales/en.yaml internal/infra/i18n/locales/zh.yaml
git commit -m "feat(config): add linear states wizard prompt, yaml template field, and i18n copy"
```

---

## Task 10: Changelog + final verification

**Files:**
- Modify: `CHANGELOG.md`

- [ ] **Step 1: Add `Unreleased` entry (English, per project convention)**

Open `CHANGELOG.md`. Under the `Unreleased` section, add bullets:

```markdown
### Changed
- Linear receiver now polls by workflow-state name (`linear_states` config) instead of an `updatedAt` cursor. Configurable via yaml or the `linear_states` system config key.
- First-sight Linear issues now dispatch a single merged `InboundMessage` containing title, description, and all existing non-bot comments, instead of fragmented per-comment messages.

### Removed
- `cursor.json` and the timestamp-based polling cursor in the Linear receiver. Operators may delete the stale file: `rm ~/.openbee/.linear/cursor.json`.
```

If the existing changelog uses a different section structure (e.g. flat list under `## Unreleased`), match the local style. The constraint is English copy, in `Unreleased`.

- [ ] **Step 2: Run the full test suite**

Run: `go test ./...`
Expected: all PASS.

- [ ] **Step 3: Run `go vet` and `gofmt -l`**

```bash
go vet ./...
gofmt -l . | grep -v vendor | grep -v node_modules
```

Expected: no vet warnings; `gofmt -l` prints nothing.

- [ ] **Step 4: Commit**

```bash
git add CHANGELOG.md
git commit -m "docs(changelog): note Linear state-based sync rewrite"
```

- [ ] **Step 5: End-to-end smoke check (manual, optional)**

If a Linear sandbox is available:

1. Start the openbee server with `linear.enabled = true`, `projects = ["sandbox"]`, `states = ["Todo", "In Progress"]`.
2. Watch the receiver log for `tick: dispatch initial merged` against existing in-window issues.
3. Add a comment to one issue → next tick logs `tick: dispatch comment`.
4. Move an issue to "Done" → next tick: no further events for that issue.
5. Move it back to "In Progress" → no re-dispatch (seen_issues is permanent).

If a sandbox is not available, the test suite is the verification authority.

---

## Self-Review Notes

- Spec coverage:
  - GraphQL filter change → Task 4
  - SeenIssues + atomic write → Task 1
  - Empty-list skip policy (states OR projects) → Task 6 + tests in Task 5
  - Merged initial dispatch including folded comment IDs into seenComments → Task 6 + tests in Task 5
  - StatesStore + hot reload → Tasks 2, 7, 8
  - System config key + handler + DB override → Tasks 3, 7, 8
  - yaml template + wizard + i18n → Task 9
  - cursor.go deletion + cursor.json migration note → Task 6 (delete) + Task 10 (changelog)
  - Test scenarios 1–6 from spec → Task 1 (seen_issues), Task 4 (pagination), Task 5 (receiver six scenarios)
- No placeholders. Every step is concrete code or a concrete command.
- Type/method names are consistent across tasks: `IssuesInStates`, `seenIssuesAPI`, `SeenIssues`, `linearcfg.StatesStore`, `buildInitialInbound`, `mergeIssueContent`, `nonBotComments`, `SystemConfigKeyLinearStates`.
