# Linear Self-Comment Prefix Marker Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the `self_comments.log` ID-tracking mechanism with a body prefix `[openbee-bot]\n\n`. Outbound comments are prefixed when sent; receiver skips inbound comments whose body starts with `[openbee-bot]`.

**Architecture:** A single package-level `const selfMarker = "[openbee-bot]\n\n"` lives in `internal/platform/linear/handler.go`. `LinearSender.Send` prepends it to `msg.Content` before calling `client.CreateComment`. `LinearReceiver.tickOnce` skips comments where `strings.HasPrefix(c.Body, "[openbee-bot]")`. The entire `selfComments` struct, its file persistence, and the wiring through `NewPlatform` are removed.

**Tech Stack:** Go 1.x, standard library only (`strings`).

**Spec:** `docs/superpowers/specs/2026-05-03-linear-self-comment-prefix-design.md`

---

## File Structure

Files modified:
- `internal/platform/linear/handler.go` — add const, modify Sender/Receiver, delete `selfComments` plumbing.
- `internal/platform/linear/handler_test.go` — replace ID-log tests with prefix-based tests.

No new files. No files deleted from disk. No interface changes.

---

### Task 1: TDD — Sender prepends the prefix

**Files:**
- Modify: `internal/platform/linear/handler.go`
- Test: `internal/platform/linear/handler_test.go`

We add the prefix injection in `Send` first. We keep the existing `selfComments` plumbing untouched in this task so the package still compiles. Task 3 will remove it.

- [ ] **Step 1: Write the failing test**

Replace the existing `TestSender_PostsCommentWithParentID` body in `internal/platform/linear/handler_test.go` so it asserts the new prefix and drops the `self.Has("C-new")` check. Replace lines 173-201 with:

```go
func TestSender_PostsCommentWithParentID(t *testing.T) {
	parent := "C0"
	rawBytes, _ := json.Marshal(replyTarget{IssueID: "I1", ParentCommentID: &parent})

	fc := &fakeClient{viewer: User{ID: "BOT"}}
	self, err := newSelfComments(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s := &LinearSender{client: fc, self: self}
	err = s.Send(context.Background(), platform.OutboundMessage{
		Content: "hello",
		ReplyTo: platform.InboundMessage{Raw: string(rawBytes)},
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if len(fc.created) != 1 {
		t.Fatalf("expected 1 CreateComment call, got %d", len(fc.created))
	}
	c := fc.created[0]
	if c.IssueID != "I1" || c.ParentID == nil || *c.ParentID != "C0" {
		t.Errorf("unexpected call: %+v", c)
	}
	// Body must begin with the self-marker prefix and end with the caller's content.
	if c.Body != "[openbee-bot]\n\nhello" {
		t.Errorf("body = %q, want %q", c.Body, "[openbee-bot]\n\nhello")
	}
}
```

- [ ] **Step 2: Run the test and verify it fails**

```bash
go test ./internal/platform/linear/ -run TestSender_PostsCommentWithParentID -v
```

Expected: FAIL with body mismatch — current Sender posts `"hello"` but test wants `"[openbee-bot]\n\nhello"`.

- [ ] **Step 3: Add the const and prepend in Sender**

In `internal/platform/linear/handler.go`, add this const right after the existing `PlatformID` const (around line 23):

```go
// selfMarker is prepended to every outbound comment body. The receiver
// recognises bot-authored comments by checking HasPrefix(body, "[openbee-bot]").
const selfMarker = "[openbee-bot]\n\n"
```

Then in `LinearSender.Send` (around line 263), change the `client.CreateComment` call inside the retry closure from:

```go
c, err := s.client.CreateComment(ctx, target.IssueID, msg.Content, target.ParentCommentID)
```

to:

```go
c, err := s.client.CreateComment(ctx, target.IssueID, selfMarker+msg.Content, target.ParentCommentID)
```

Leave the surrounding `s.self.Add(c.ID)` callback in place for now.

- [ ] **Step 4: Run the test and verify it passes**

```bash
go test ./internal/platform/linear/ -run TestSender_PostsCommentWithParentID -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/platform/linear/handler.go internal/platform/linear/handler_test.go
git commit -m "$(cat <<'EOF'
feat(linear): prepend [openbee-bot] marker to outbound comments

Adds a package-level selfMarker const and prepends it in
LinearSender.Send. Receiver-side detection comes in the next commit.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

### Task 2: TDD — Receiver skips comments with the prefix

**Files:**
- Modify: `internal/platform/linear/handler.go`
- Test: `internal/platform/linear/handler_test.go`

We add the prefix-based skip alongside (logically OR'd with) the existing `r.self.Has(c.ID)` check, plus a new test that verifies a mid-string occurrence does NOT skip. The `self_comments.log` plumbing stays in place; Task 3 removes it.

- [ ] **Step 1: Write the failing tests**

Append to the bottom of `internal/platform/linear/handler_test.go`:

```go
// TestReceiver_TickOnce_SkipsPrefixedSelfComment verifies the new
// prefix-based identification: a polled comment whose body starts with
// "[openbee-bot]" must be skipped even when it is NOT in the self-comment
// ID set (simulates a fresh state file or an upgraded process).
func TestReceiver_TickOnce_SkipsPrefixedSelfComment(t *testing.T) {
	bot := User{ID: "BOT"}
	since := mustParse(t, "2026-05-02T09:00:00Z")
	issue := Issue{
		ID:         "I1",
		Identifier: "ENG-42",
		Title:      "T",
		Team:       Team{Key: "ENG"},
		Creator:    User{ID: "U2"},
		// Issue and label predate `since`; only comments are new.
		CreatedAt: mustParse(t, "2026-05-02T08:00:00Z"),
		UpdatedAt: mustParse(t, "2026-05-02T11:00:00Z"),
		Labels: []IssueLabel{
			{Name: "openbee", CreatedAt: mustParse(t, "2026-05-02T08:30:00Z")},
		},
		Comments: []Comment{
			// Bot's own outbound — has the marker, NOT in the self set.
			{ID: "C-bot", Body: "[openbee-bot]\n\nhi there", CreatedAt: mustParse(t, "2026-05-02T10:00:00Z"), User: bot},
			// Real user comment — must be dispatched.
			{ID: "C-user", Body: "what's up?", CreatedAt: mustParse(t, "2026-05-02T10:30:00Z"), User: User{ID: "U2"}},
		},
	}
	fc := &fakeClient{
		viewer: bot,
		issues: func(_ time.Time) ([]Issue, bool, error) { return []Issue{issue}, false, nil },
	}
	cur := &fakeCursor{last: since}
	self, err := newSelfComments(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	r := &LinearReceiver{client: fc, cursor: cur, labelName: "openbee", self: self}

	var got []platform.InboundMessage
	r.tickOnce(context.Background(), func(m platform.InboundMessage) { got = append(got, m) })

	if len(got) != 1 {
		t.Fatalf("dispatched %d, want 1: %+v", len(got), got)
	}
	if got[0].PlatformMessageID != "comment:C-user" {
		t.Errorf("unexpected dispatch: %+v", got[0])
	}
}

// TestReceiver_TickOnce_DispatchesCommentContainingMarkerMidString verifies the
// prefix check is HasPrefix, not Contains: a user quoting "[openbee-bot]" later
// in their reply should still be dispatched.
func TestReceiver_TickOnce_DispatchesCommentContainingMarkerMidString(t *testing.T) {
	since := mustParse(t, "2026-05-02T09:00:00Z")
	issue := Issue{
		ID:         "I1",
		Identifier: "ENG-42",
		Title:      "T",
		Team:       Team{Key: "ENG"},
		Creator:    User{ID: "U2"},
		CreatedAt:  mustParse(t, "2026-05-02T08:00:00Z"),
		UpdatedAt:  mustParse(t, "2026-05-02T10:30:00Z"),
		Labels: []IssueLabel{
			{Name: "openbee", CreatedAt: mustParse(t, "2026-05-02T08:30:00Z")},
		},
		Comments: []Comment{
			{ID: "C-user", Body: "i saw [openbee-bot] in your reply", CreatedAt: mustParse(t, "2026-05-02T10:00:00Z"), User: User{ID: "U2"}},
		},
	}
	fc := &fakeClient{
		viewer: User{ID: "BOT"},
		issues: func(_ time.Time) ([]Issue, bool, error) { return []Issue{issue}, false, nil },
	}
	cur := &fakeCursor{last: since}
	self, err := newSelfComments(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	r := &LinearReceiver{client: fc, cursor: cur, labelName: "openbee", self: self}

	var got []platform.InboundMessage
	r.tickOnce(context.Background(), func(m platform.InboundMessage) { got = append(got, m) })

	if len(got) != 1 {
		t.Fatalf("dispatched %d, want 1 (the user quote): %+v", len(got), got)
	}
	if got[0].PlatformMessageID != "comment:C-user" {
		t.Errorf("unexpected dispatch: %+v", got[0])
	}
}
```

- [ ] **Step 2: Run the new tests and verify they fail**

```bash
go test ./internal/platform/linear/ -run 'TestReceiver_TickOnce_SkipsPrefixedSelfComment|TestReceiver_TickOnce_DispatchesCommentContainingMarkerMidString' -v
```

Expected: `TestReceiver_TickOnce_SkipsPrefixedSelfComment` FAILS — the bot comment is dispatched because nothing in the receiver looks at the body. `TestReceiver_TickOnce_DispatchesCommentContainingMarkerMidString` PASSES (no skip applied).

- [ ] **Step 3: Add the prefix check in the receiver**

In `internal/platform/linear/handler.go`, in `LinearReceiver.tickOnce` (around line 170-181), change the inner comment loop from:

```go
		for _, c := range issue.Comments {
			if !c.CreatedAt.After(since) {
				continue
			}
			if r.self.Has(c.ID) {
				continue
			}
			dispatch(buildCommentInbound(issue, c))
			if c.CreatedAt.After(highWater) {
				highWater = c.CreatedAt
			}
		}
```

to:

```go
		for _, c := range issue.Comments {
			if !c.CreatedAt.After(since) {
				continue
			}
			if strings.HasPrefix(c.Body, "[openbee-bot]") {
				continue
			}
			if r.self.Has(c.ID) {
				continue
			}
			dispatch(buildCommentInbound(issue, c))
			if c.CreatedAt.After(highWater) {
				highWater = c.CreatedAt
			}
		}
```

`strings` is already imported in this file (line 10), so no import changes are needed.

- [ ] **Step 4: Run the new tests and verify they pass**

```bash
go test ./internal/platform/linear/ -run 'TestReceiver_TickOnce_SkipsPrefixedSelfComment|TestReceiver_TickOnce_DispatchesCommentContainingMarkerMidString' -v
```

Expected: both PASS.

- [ ] **Step 5: Run the whole package test suite to confirm no regressions**

```bash
go test ./internal/platform/linear/...
```

Expected: PASS (the existing tests still use `self.Add("C-bot")` and dispatch correctly because both checks are present).

- [ ] **Step 6: Commit**

```bash
git add internal/platform/linear/handler.go internal/platform/linear/handler_test.go
git commit -m "$(cat <<'EOF'
feat(linear): skip inbound comments with [openbee-bot] body prefix

Receiver now recognises bot-authored comments by body prefix in
addition to the existing ID-set check. Next commit removes the ID
set entirely.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

### Task 3: Remove the `selfComments` mechanism

**Files:**
- Modify: `internal/platform/linear/handler.go`
- Modify: `internal/platform/linear/handler_test.go`

The prefix check now fully covers self-comment identification. Remove the struct, fields, callers, and tests in one commit so the test suite stays green throughout.

- [ ] **Step 1: Delete the obsolete tests**

In `internal/platform/linear/handler_test.go`:

1. Delete the entire `TestSelfComments_PersistsAcrossRestart` function (lines 270-306 in the current file).
2. Delete the entire `TestSelfComments_ConcurrentAddsAreAtomic` function (lines 308-344 in the current file).
3. Delete the entire `TestReceiver_TickOnce_DispatchesHumanCommentSharingBotUserID` function (lines 206-253). The scenario it covered (human comment sharing the bot's user ID) is irrelevant now: identification is purely body-based, never user-ID-based.
4. Delete the `strPtr` helper at line 255 — it was only used by the deleted test.

- [ ] **Step 2: Drop `self` plumbing from the remaining tests**

In each of the still-present receiver/sender tests, remove the `newSelfComments(...)`, the `self.Add(...)` call (where present), and pass the now-simplified struct literal:

In `TestReceiver_TickOnce_DispatchesIssueAndComments` (around lines 67-128), replace the block:

```go
	self, err := newSelfComments(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	self.Add("C-bot")
	r := &LinearReceiver{
		client:    fc,
		cursor:    cur,
		labelName: "openbee",
		self:      self,
	}
```

with:

```go
	r := &LinearReceiver{
		client:    fc,
		cursor:    cur,
		labelName: "openbee",
	}
```

Also update the comment in this test from:
```go
	// Expect 3 dispatches: issue body, C1, C2 (C-bot filtered via self set).
```
to:
```go
	// Expect 3 dispatches: issue body, C1, C2 (C-bot filtered via body prefix).
```

And update the bot comment fixture in this test (around line 84) so its body now carries the marker instead of relying on the ID set:
```go
		{ID: "C-bot", Body: "ignore me", CreatedAt: mustParse(t, "2026-05-02T11:15:00Z"), User: bot, IssueID: "I1"},
```
becomes:
```go
		{ID: "C-bot", Body: "[openbee-bot]\n\nignore me", CreatedAt: mustParse(t, "2026-05-02T11:15:00Z"), User: bot, IssueID: "I1"},
```

In `TestReceiver_TickOnce_ErrorDoesNotAdvanceCursor` (around lines 130-146), replace:

```go
	self, err := newSelfComments(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	r := &LinearReceiver{client: fc, cursor: cur, labelName: "openbee", self: self}
```

with:

```go
	r := &LinearReceiver{client: fc, cursor: cur, labelName: "openbee"}
```

In `TestReceiver_TickOnce_TruncatedDoesNotAdvanceCursor` (around lines 148-171), make the same replacement.

In `TestReceiver_TickOnce_SkipsPrefixedSelfComment` and `TestReceiver_TickOnce_DispatchesCommentContainingMarkerMidString` (added in Task 2), make the same replacement.

In `TestSender_PostsCommentWithParentID` (currently uses `self`), replace:

```go
	self, err := newSelfComments(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s := &LinearSender{client: fc, self: self}
```

with:

```go
	s := &LinearSender{client: fc}
```

- [ ] **Step 3: Trim unused imports in the test file**

In `internal/platform/linear/handler_test.go`, the import block at the top currently is:

```go
import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/theopenbee/openbee/internal/platform"
)
```

After deletion, `fmt`, `os`, `path/filepath`, `strings`, and `sync` are no longer referenced from test code (the `fakeClient` mutex stays — wait, verify: `fakeClient` uses `sync.Mutex`, so `sync` IS still needed). Trim only what is truly unused:

```go
import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/theopenbee/openbee/internal/platform"
)
```

(Removed: `fmt`, `os`, `path/filepath`, `strings`. Kept: `sync` for `fakeClient.mu`.)

- [ ] **Step 4: Delete the `selfComments` struct and plumbing in handler.go**

In `internal/platform/linear/handler.go`:

1. Delete the entire `selfComments` block (the comment, the struct definition, `newSelfComments`, `Add`, and `Has`) — currently lines 33-88.

2. Remove the `self` field from `LinearReceiver` and `LinearSender`. The `LinearReceiver` struct (around lines 126-132) becomes:

```go
type LinearReceiver struct {
	client       Client
	cursor       cursorAPI
	labelName    string
	pollInterval time.Duration
}
```

The `LinearSender` struct (around lines 257-260) becomes:

```go
type LinearSender struct {
	client Client
}
```

3. In `NewPlatform` (around lines 98-119), remove the `newSelfComments` call and the `self` arguments. The function becomes:

```go
func NewPlatform(cfg config.LinearConfig) (platform.Platform, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("linear: resolve home dir: %w", err)
	}
	dir := filepath.Join(home, ".openbee", ".linear")
	client := NewClient(cfg.APIKey)
	return &LinearPlatform{
		receiver: &LinearReceiver{
			client:       client,
			cursor:       NewCursor(dir),
			labelName:    cfg.LabelName,
			pollInterval: cfg.PollInterval,
		},
		sender: &LinearSender{client: client},
	}, nil
}
```

4. In `LinearReceiver.tickOnce`, remove the now-redundant `r.self.Has(c.ID)` block. The comment loop becomes:

```go
		for _, c := range issue.Comments {
			if !c.CreatedAt.After(since) {
				continue
			}
			if strings.HasPrefix(c.Body, "[openbee-bot]") {
				continue
			}
			dispatch(buildCommentInbound(issue, c))
			if c.CreatedAt.After(highWater) {
				highWater = c.CreatedAt
			}
		}
```

5. In `LinearSender.Send`, remove the `s.self.Add(c.ID)` block from inside the retry closure. The closure becomes:

```go
	return utils.RetryWithBackoff(ctx, func() error {
		_, err := s.client.CreateComment(ctx, target.IssueID, selfMarker+msg.Content, target.ParentCommentID)
		return err
	}, utils.DefaultRetryCount, utils.DefaultRetryDelay)
```

(Note: we drop the `c` variable since we no longer use it.)

- [ ] **Step 5: Trim unused imports in handler.go**

After the deletion, audit the import block at the top of `internal/platform/linear/handler.go`. Remove any import that no longer has a referent. The likely changes:

- `errors` — still used by `errors.New` in Send and possibly elsewhere; keep if so, remove if not.
- `sync` — was only used by `selfComments.mu`; remove.

Verify by running `goimports` or simply by relying on the compiler to flag unused imports.

The expected final import block for handler.go:

```go
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

	"github.com/theopenbee/openbee/internal/infra/config"
	"github.com/theopenbee/openbee/internal/infra/logger"
	"github.com/theopenbee/openbee/internal/platform"
	"github.com/theopenbee/openbee/internal/utils"
)
```

(Removed: `sync`.)

- [ ] **Step 6: Run the full package test suite**

```bash
go test ./internal/platform/linear/...
```

Expected: PASS.

- [ ] **Step 7: Run the whole repo to catch downstream breakage**

```bash
go build ./...
go test ./...
```

Expected: build succeeds and all tests pass. There should be no consumer of the removed types — `selfComments` was unexported and only used inside this package.

- [ ] **Step 8: Commit**

```bash
git add internal/platform/linear/handler.go internal/platform/linear/handler_test.go
git commit -m "$(cat <<'EOF'
refactor(linear): drop self_comments.log in favour of body prefix

The [openbee-bot] body prefix is now the sole mechanism for skipping
the bot's own comments on the next poll. Removes the selfComments
struct, its file persistence, the self field on LinearReceiver and
LinearSender, the newSelfComments call in NewPlatform, and the
related unit tests.

Existing ~/.openbee/.linear/self_comments.log files on disk are
left in place, harmless, and may be deleted manually.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

## Verification After All Tasks

Run from repo root:

```bash
go vet ./internal/platform/linear/...
go test ./internal/platform/linear/... -v
go build ./...
go test ./...
```

All four must succeed.

Acceptance check from the spec:

- [ ] No reference to `selfComments`, `newSelfComments`, or `self_comments.log` remains in `internal/platform/linear/handler.go` or `handler_test.go`. Verify:
  ```bash
  grep -nE 'selfComments|newSelfComments|self_comments' internal/platform/linear/
  ```
  Expected: no matches.

- [ ] `selfMarker` is referenced in exactly one place in `handler.go` (the Sender) and the bare `[openbee-bot]` literal appears in exactly one place in `handler.go` (the Receiver `HasPrefix`). Verify:
  ```bash
  grep -n 'openbee-bot' internal/platform/linear/handler.go
  ```
  Expected: two lines — one defining `selfMarker`, one the `HasPrefix` check.
