# Linear: Remove ReactionDelete Logic Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove the Linear platform's "remove `:eyes:` reaction after reply" feature, including the `pendingReactions sync.Map` coordination and the underlying `Client.DeleteReaction` GraphQL mutation. Keep `addReaction` as a fire-and-forget dispatch acknowledgment.

**Architecture:** Three reviewable commits. (1) Strip sender-side delete logic and its tests. (2) Simplify receiver `addReaction` and drop the shared `pendingReactions` plumbing across `NewPlatform`, `LinearReceiver`, and their tests. (3) Remove the `DeleteReaction` method from the `Client` interface, `*httpClient`, `fakeClient`, and the matching client-level test.

**Tech Stack:** Go 1.x, `sync.Map`, internal `utils.RetryWithBackoff`, `go.uber.org/zap`. Tests use the in-package `fakeClient`.

**Spec:** [`docs/superpowers/specs/2026-05-06-linear-remove-reaction-delete-design.md`](../specs/2026-05-06-linear-remove-reaction-delete-design.md)

---

## Pre-flight

- [ ] **Step 0: Confirm baseline tests pass**

Run: `go test ./internal/platform/linear/...`
Expected: PASS

---

## Task 1: Strip sender-side reaction-delete logic

**Files:**
- Modify: `internal/platform/linear/sender_test.go`
- Modify: `internal/platform/linear/handler.go`

### Step 1.1: Delete sender tests that exercise reaction deletion

- [ ] **Step 1.1.a: Open `internal/platform/linear/sender_test.go`**

Delete these three test functions in their entirety:
- `TestSender_DeletesReactionAfterReply` (currently around lines 87-126)
- `TestSender_NoPendingReaction_StillSucceeds` (currently around lines 128-145)
- `TestSender_ReactionDeleteFails_StillSucceeds` (currently around lines 147-171)

Also remove these now-unused imports from the import block at the top of the file:
- `"errors"`
- `"sync"`
- `"time"`

After edits, the import block should read:
```go
import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/theopenbee/openbee/internal/platform"
)
```

- [ ] **Step 1.1.b: Run sender tests to confirm the file still compiles**

Run: `go test ./internal/platform/linear/ -run TestSender -v`
Expected: PASS for `TestSender_PostsCommentWithParentID` and `TestSender_AppendsUploadedMarkdownToBody`. No other `TestSender_*` tests run.

### Step 1.2: Remove `removeReaction` method and its caller

- [ ] **Step 1.2.a: Open `internal/platform/linear/handler.go`**

Delete the entire `removeReaction` method (currently lines 196-231 — the doc comment block plus the function body):

```go
// removeReaction is invoked by the sender after a reply has been posted. It
// looks up any pending reaction stored under key, waits up to 5s for the
// reactionID, and fires DeleteReaction in a background goroutine. Failures
// are logged and never propagated to the caller.
func (s *LinearSender) removeReaction(key string) {
	if s.pendingReactions == nil {
		return
	}
	val, ok := s.pendingReactions.LoadAndDelete(key)
	if !ok {
		return
	}
	ch, ok := val.(chan string)
	if !ok {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		timer := time.NewTimer(5 * time.Second)
		defer timer.Stop()
		select {
		case reactionID, received := <-ch:
			if !received || reactionID == "" {
				return
			}
			if err := utils.RetryWithBackoff(ctx, func() error {
				return s.client.DeleteReaction(ctx, reactionID)
			}, utils.DefaultRetryCount, utils.DefaultRetryDelay); err != nil {
				log.Warn("linear: remove reaction failed", zap.String("key", key), zap.Error(err))
			}
		case <-timer.C:
			log.Warn("linear: timed out waiting for reaction ID", zap.String("key", key))
		}
	}()
}
```

In the same file, locate `LinearSender.Send` (currently around lines 413-443) and delete the line:

```go
	s.removeReaction(msg.ReplyTo.PlatformMessageID)
```

So the tail of `Send` becomes:

```go
	if err := utils.RetryWithBackoff(ctx, func() error {
		_, err := s.client.CreateComment(ctx, target.IssueID, body, target.ParentCommentID)
		return err
	}, utils.DefaultRetryCount, utils.DefaultRetryDelay); err != nil {
		return err
	}
	return nil
}
```

### Step 1.3: Remove `pendingReactions` field from `LinearSender`

- [ ] **Step 1.3.a: In `internal/platform/linear/handler.go`, edit the `LinearSender` struct (currently around lines 406-411)**

Before:
```go
// LinearSender posts replies as Linear comments.
type LinearSender struct {
	client           Client
	uploader         *uploader
	pendingReactions *sync.Map
}
```

After:
```go
// LinearSender posts replies as Linear comments.
type LinearSender struct {
	client   Client
	uploader *uploader
}
```

- [ ] **Step 1.3.b: In the same file, update `NewPlatform` (currently around lines 79-87)**

Before:
```go
		sender: &LinearSender{
			client: client,
			uploader: &uploader{
				client:  client,
				maxSize: maxSize,
				http:    &http.Client{Timeout: uploadPutTimeout + 30*time.Second},
			},
			pendingReactions: pending,
		},
```

After:
```go
		sender: &LinearSender{
			client: client,
			uploader: &uploader{
				client:  client,
				maxSize: maxSize,
				http:    &http.Client{Timeout: uploadPutTimeout + 30*time.Second},
			},
		},
```

(The `pending` variable is still referenced by the receiver literal — leave it alone for now; Task 2 removes it.)

### Step 1.4: Verify build and tests

- [ ] **Step 1.4.a: Build**

Run: `go build ./...`
Expected: PASS

- [ ] **Step 1.4.b: Run linear package tests**

Run: `go test ./internal/platform/linear/...`
Expected: PASS for all remaining tests.

### Step 1.5: Commit

- [ ] **Step 1.5.a: Stage and commit**

```bash
git add internal/platform/linear/handler.go internal/platform/linear/sender_test.go
git commit -m "$(cat <<'EOF'
refactor(linear): drop sender-side ReactionDelete flow

Remove LinearSender.removeReaction and its Send-time invocation, plus
the pendingReactions field on LinearSender. Tests covering the deletion
behavior are also removed.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: Simplify `addReaction` and drop receiver `pendingReactions`

**Files:**
- Modify: `internal/platform/linear/handler.go`
- Modify: `internal/platform/linear/handler_test.go`

### Step 2.1: Simplify `addReaction`

- [ ] **Step 2.1.a: In `internal/platform/linear/handler.go`, replace `addReaction` (currently lines 160-194)**

Before:
```go
// addReaction asynchronously creates a reaction on target and stores the
// resulting ID in pendingReactions under key. A buffered channel coordinates
// with sender's LoadAndDelete so the sender can wait for the ID even when
// the API call has not yet returned.
func (r *LinearReceiver) addReaction(ctx context.Context, key string, target ReactionTarget) {
	if r.pendingReactions == nil {
		return
	}
	ch := make(chan string, 1)
	r.pendingReactions.Store(key, ch)
	go func() {
		defer time.AfterFunc(reactionCleanupTTL, func() {
			r.pendingReactions.Delete(key)
		})
		var reactionID string
		err := utils.RetryWithBackoff(ctx, func() error {
			id, e := r.client.CreateReaction(ctx, target, reactionEmoji)
			if e != nil {
				return e
			}
			reactionID = id
			return nil
		}, utils.DefaultRetryCount, utils.DefaultRetryDelay)
		if err != nil {
			log.Warn("linear: add reaction failed", zap.String("key", key), zap.Error(err))
			close(ch)
			return
		}
		if reactionID == "" {
			close(ch)
			return
		}
		ch <- reactionID
	}()
}
```

After:
```go
// addReaction asynchronously creates the dispatch acknowledgment reaction on
// target. Failures are logged and never propagated; the call site does not
// observe completion.
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

### Step 2.2: Update `tickOnce` call sites

- [ ] **Step 2.2.a: In `internal/platform/linear/handler.go`, locate the two `addReaction` calls in `tickOnce` (currently lines 265 and 287)**

Before (line 265):
```go
				r.addReaction(ctx, "issue:"+issue.ID, ReactionTarget{IssueID: issue.ID})
```
After:
```go
				r.addReaction(ctx, ReactionTarget{IssueID: issue.ID})
```

Before (line 287):
```go
				r.addReaction(ctx, "comment:"+c.ID, ReactionTarget{CommentID: c.ID})
```
After:
```go
				r.addReaction(ctx, ReactionTarget{CommentID: c.ID})
```

### Step 2.3: Remove `pendingReactions` field, constants, and `NewPlatform` plumbing

- [ ] **Step 2.3.a: Remove the `reactionCleanupTTL` constant (currently lines 37-39)**

Delete:
```go
// reactionCleanupTTL bounds how long an unresolved pendingReactions entry
// lingers before being swept (memory-leak guard).
const reactionCleanupTTL = 10 * time.Minute
```

- [ ] **Step 2.3.b: Remove `pendingReactions *sync.Map` from `LinearReceiver` (currently line 105)**

Before:
```go
// LinearReceiver polls Linear for issue/comment updates by workflow-state.
type LinearReceiver struct {
	client           Client
	seenIssues       seenAPI
	seenComments     seenAPI
	labelName        string
	pollInterval     time.Duration
	projectsList     []string
	statesList       []string
	resolver         *resolver
	pendingReactions *sync.Map
}
```

After:
```go
// LinearReceiver polls Linear for issue/comment updates by workflow-state.
type LinearReceiver struct {
	client       Client
	seenIssues   seenAPI
	seenComments seenAPI
	labelName    string
	pollInterval time.Duration
	projectsList []string
	statesList   []string
	resolver     *resolver
}
```

- [ ] **Step 2.3.c: Update `NewPlatform` (currently lines 51-89)**

Remove the `pending := &sync.Map{}` line (currently line 62) and the `pendingReactions: pending` field from the receiver literal (currently line 77).

After this change, the receiver literal should look like:
```go
		receiver: &LinearReceiver{
			client:       client,
			seenIssues:   NewSeenSet(dir, "seen_issues.ndjson"),
			seenComments: NewSeenSet(dir, "seen_comments.ndjson"),
			labelName:    cfg.LabelName,
			pollInterval: cfg.PollInterval,
			projectsList: cleanStringList(cfg.Projects),
			statesList:   cleanStringList(cfg.States),
			resolver: &resolver{
				client:  client,
				media:   mediaSvc,
				maxSize: maxSize,
			},
		},
```

- [ ] **Step 2.3.d: Drop the `sync` import from the import block (currently line 12)**

Before:
```go
import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/theopenbee/openbee/internal/infra/config"
	"github.com/theopenbee/openbee/internal/infra/logger"
	"github.com/theopenbee/openbee/internal/infra/media"
	"github.com/theopenbee/openbee/internal/platform"
	"github.com/theopenbee/openbee/internal/utils"
)
```

After:
```go
import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/theopenbee/openbee/internal/infra/config"
	"github.com/theopenbee/openbee/internal/infra/logger"
	"github.com/theopenbee/openbee/internal/infra/media"
	"github.com/theopenbee/openbee/internal/platform"
	"github.com/theopenbee/openbee/internal/utils"
)
```

### Step 2.4: Update receiver tests in `handler_test.go`

The three reaction-related receiver tests must be updated to drop `pendingReactions` references. The pre-edit file contains tests structured around `pending := &sync.Map{}` plus a `pendingReactions: pending` field in the `LinearReceiver` literal.

- [ ] **Step 2.4.a: Replace `TestReceiver_TickOnce_AddsReactionForInitialIssue` (currently lines 771-824)**

Replace the entire function with:

```go
func TestReceiver_TickOnce_AddsReactionForInitialIssue(t *testing.T) {
	bot := User{ID: "BOT"}
	issue := Issue{
		ID: "I1", Identifier: "ENG-42", Title: "T",
		Team: Team{Key: "ENG"}, Creator: User{ID: "U2"},
	}
	fc := &fakeClient{viewer: bot, issues: func() ([]Issue, error) { return []Issue{issue}, nil }}
	r := &LinearReceiver{
		client:       fc,
		seenIssues:   newFakeSeenSet(),
		seenComments: newFakeSeenSet(),
		labelName:    "openbee",
		pollInterval: time.Hour,
		projectsList: testProjects(),
		statesList:   testStates(),
		resolver: &resolver{
			client:  fc,
			media:   media.NewService(),
			maxSize: 10 * 1024 * 1024,
		},
	}

	r.tickOnce(context.Background(), func(platform.InboundMessage) {})

	// reaction goroutine is async; wait briefly for it to record the call.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		fc.mu.Lock()
		n := len(fc.reactionCreated)
		fc.mu.Unlock()
		if n == 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	fc.mu.Lock()
	defer fc.mu.Unlock()
	if len(fc.reactionCreated) != 1 {
		t.Fatalf("expected 1 CreateReaction call, got %d", len(fc.reactionCreated))
	}
	got := fc.reactionCreated[0]
	if got.Target.IssueID != "I1" || got.Target.CommentID != "" {
		t.Errorf("target = %+v, want IssueID=I1", got.Target)
	}
	if got.Emoji != ":eyes:" {
		t.Errorf("emoji = %q, want :eyes:", got.Emoji)
	}
}
```

- [ ] **Step 2.4.b: Replace `TestReceiver_TickOnce_AddsReactionForNewComment` (currently lines 826-880)**

Replace the entire function with:

```go
func TestReceiver_TickOnce_AddsReactionForNewComment(t *testing.T) {
	bot := User{ID: "BOT"}
	issue := Issue{
		ID: "I1", Identifier: "ENG-42", Title: "T",
		Team: Team{Key: "ENG"}, Creator: User{ID: "U2"},
		Comments: []Comment{
			{ID: "C1", Body: "new", User: User{ID: "U3"}},
		},
	}
	seenIssues := newFakeSeenSet()
	seenIssues.ids["I1"] = struct{}{}
	fc := &fakeClient{viewer: bot, issues: func() ([]Issue, error) { return []Issue{issue}, nil }}
	r := &LinearReceiver{
		client:       fc,
		seenIssues:   seenIssues,
		seenComments: newFakeSeenSet(),
		labelName:    "openbee",
		pollInterval: time.Hour,
		projectsList: testProjects(),
		statesList:   testStates(),
		resolver: &resolver{
			client:  fc,
			media:   media.NewService(),
			maxSize: 10 * 1024 * 1024,
		},
	}

	r.tickOnce(context.Background(), func(platform.InboundMessage) {})

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		fc.mu.Lock()
		n := len(fc.reactionCreated)
		fc.mu.Unlock()
		if n == 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	fc.mu.Lock()
	defer fc.mu.Unlock()
	if len(fc.reactionCreated) != 1 {
		t.Fatalf("expected 1 CreateReaction call, got %d", len(fc.reactionCreated))
	}
	got := fc.reactionCreated[0]
	if got.Target.CommentID != "C1" || got.Target.IssueID != "" {
		t.Errorf("target = %+v, want CommentID=C1", got.Target)
	}
}
```

- [ ] **Step 2.4.c: Replace `TestReceiver_TickOnce_ReactionCreateFails_DoesNotBlockDispatch` (currently lines 882-942)**

Replace the entire function with:

```go
func TestReceiver_TickOnce_ReactionCreateFails_DoesNotBlockDispatch(t *testing.T) {
	issue := Issue{
		ID: "I1", Identifier: "ENG-42", Title: "T",
		Team: Team{Key: "ENG"}, Creator: User{ID: "U2"},
	}

	// Cancel ctx from inside createReactionImpl so RetryWithBackoff aborts
	// between retries — keeps the test fast without coupling to retry count.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	fc := &fakeClient{
		viewer: User{ID: "BOT"},
		issues: func() ([]Issue, error) { return []Issue{issue}, nil },
		createReactionImpl: func(target ReactionTarget, emoji string) (string, error) {
			cancel()
			return "", errors.New("boom")
		},
	}
	r := &LinearReceiver{
		client:       fc,
		seenIssues:   newFakeSeenSet(),
		seenComments: newFakeSeenSet(),
		labelName:    "openbee",
		pollInterval: time.Hour,
		projectsList: testProjects(),
		statesList:   testStates(),
		resolver: &resolver{
			client:  fc,
			media:   media.NewService(),
			maxSize: 10 * 1024 * 1024,
		},
	}

	var dispatched []platform.InboundMessage
	r.tickOnce(ctx, func(m platform.InboundMessage) { dispatched = append(dispatched, m) })

	if len(dispatched) != 1 {
		t.Fatalf("dispatch must run regardless of reaction failure; got %d", len(dispatched))
	}
}
```

(Justification: `tickOnce` invokes `dispatch` synchronously before kicking off the reaction goroutine. The contract under test is "reaction failure does not block dispatch" — synchronous dispatch length already proves it. We no longer need to wait on a channel that no longer exists.)

- [ ] **Step 2.4.d: Drop the `sync` import from `handler_test.go` if unused**

Check whether anything else in the file references `sync` after the rewrites above. If not, remove it from the import block.

The import block should end up as:
```go
import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/theopenbee/openbee/internal/infra/media"
	"github.com/theopenbee/openbee/internal/platform"
)
```

### Step 2.5: Verify build and tests

- [ ] **Step 2.5.a: Build**

Run: `go build ./...`
Expected: PASS

- [ ] **Step 2.5.b: Run linear package tests**

Run: `go test ./internal/platform/linear/...`
Expected: PASS

### Step 2.6: Commit

- [ ] **Step 2.6.a: Stage and commit**

```bash
git add internal/platform/linear/handler.go internal/platform/linear/handler_test.go
git commit -m "$(cat <<'EOF'
refactor(linear): simplify addReaction to fire-and-forget

Drop the pendingReactions sync.Map plumbing now that nothing consumes
the reactionID. addReaction no longer needs a key, channel, or TTL
cleanup; receiver tests are updated accordingly.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: Remove `DeleteReaction` from `Client` interface, `httpClient`, fake, and tests

**Files:**
- Modify: `internal/platform/linear/client.go`
- Modify: `internal/platform/linear/client_test.go`
- Modify: `internal/platform/linear/handler_test.go`

### Step 3.1: Remove `DeleteReaction` from the `Client` interface

- [ ] **Step 3.1.a: In `internal/platform/linear/client.go`, edit the `Client` interface (currently around lines 89-94)**

Before:
```go
	// CreateReaction adds a reaction to the given target with the given emoji
	// shortcode (e.g. ":eyes:") and returns the new reaction's ID.
	CreateReaction(ctx context.Context, target ReactionTarget, emoji string) (string, error)
	// DeleteReaction removes a reaction by its ID.
	DeleteReaction(ctx context.Context, reactionID string) error
}
```

After:
```go
	// CreateReaction adds a reaction to the given target with the given emoji
	// shortcode (e.g. ":eyes:") and returns the new reaction's ID.
	CreateReaction(ctx context.Context, target ReactionTarget, emoji string) (string, error)
}
```

### Step 3.2: Remove the `reactionDeleteMutation` constant and `httpClient.DeleteReaction`

- [ ] **Step 3.2.a: In `internal/platform/linear/client.go`, delete the block currently at lines 280-302**

Delete in full:
```go
const reactionDeleteMutation = `
mutation ReactionDelete($id: String!) {
  reactionDelete(id: $id) { success }
}`

func (c *httpClient) DeleteReaction(ctx context.Context, reactionID string) error {
	if reactionID == "" {
		return fmt.Errorf("linear: DeleteReaction requires reactionID")
	}
	vars := map[string]any{"id": reactionID}
	var data struct {
		ReactionDelete struct {
			Success bool `json:"success"`
		} `json:"reactionDelete"`
	}
	if err := c.do(ctx, "reactionDelete", reactionDeleteMutation, vars, &data); err != nil {
		return err
	}
	if !data.ReactionDelete.Success {
		return fmt.Errorf("linear: reactionDelete not successful")
	}
	return nil
}
```

### Step 3.3: Remove `TestClient_DeleteReaction`

- [ ] **Step 3.3.a: In `internal/platform/linear/client_test.go`, delete `TestClient_DeleteReaction` (currently lines 499-518)**

Delete the entire function:
```go
func TestClient_DeleteReaction(t *testing.T) {
	_, c := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		s := string(body)
		if !strings.Contains(s, "reactionDelete") {
			t.Errorf("query missing reactionDelete: %s", s)
		}
		if !strings.Contains(s, `"id":"R1"`) {
			t.Errorf("variables missing id: %s", s)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"reactionDelete": map[string]any{"success": true},
			},
		})
	})
	if err := c.DeleteReaction(context.Background(), "R1"); err != nil {
		t.Fatalf("DeleteReaction: %v", err)
	}
}
```

### Step 3.4: Strip `DeleteReaction` and related state from `fakeClient`

- [ ] **Step 3.4.a: In `internal/platform/linear/handler_test.go`, edit the `fakeClient` struct (currently lines 26-53)**

Before:
```go
// fakeClient is a Client that returns canned data per call.
type fakeClient struct {
	mu           sync.Mutex
	viewer       User
	viewerErr    error
	calls        int
	lastStates   []string
	lastProjects []string
	issues       func() ([]Issue, error)
	created      []struct {
		IssueID, Body string
		ParentID      *string
	}
	downloads  map[string][]byte
	uploadImpl func(name, mime string, size int) (FileUploadTicket, error)

	// Reaction tracking. Tests can inject behavior via createReactionImpl /
	// deleteReactionImpl; otherwise the defaults record calls and return
	// monotonically increasing reaction IDs.
	createReactionImpl func(target ReactionTarget, emoji string) (string, error)
	deleteReactionImpl func(id string) error
	reactionCreated    []struct {
		Target ReactionTarget
		Emoji  string
		ID     string
	}
	reactionDeleted []string
	nextReactionID  int
}
```

After:
```go
// fakeClient is a Client that returns canned data per call.
type fakeClient struct {
	mu           sync.Mutex
	viewer       User
	viewerErr    error
	calls        int
	lastStates   []string
	lastProjects []string
	issues       func() ([]Issue, error)
	created      []struct {
		IssueID, Body string
		ParentID      *string
	}
	downloads  map[string][]byte
	uploadImpl func(name, mime string, size int) (FileUploadTicket, error)

	// Reaction tracking. Tests can inject behavior via createReactionImpl;
	// the default records calls and returns monotonically increasing reaction
	// IDs.
	createReactionImpl func(target ReactionTarget, emoji string) (string, error)
	reactionCreated    []struct {
		Target ReactionTarget
		Emoji  string
		ID     string
	}
	nextReactionID int
}
```

(Note: `sync` is still needed for `sync.Mutex` even though `sync.Map` was dropped — keep the import.)

- [ ] **Step 3.4.b: Delete the `DeleteReaction` method on `fakeClient` (currently lines 119-133)**

Delete:
```go
func (f *fakeClient) DeleteReaction(ctx context.Context, reactionID string) error {
	if f.deleteReactionImpl != nil {
		err := f.deleteReactionImpl(reactionID)
		if err == nil {
			f.mu.Lock()
			f.reactionDeleted = append(f.reactionDeleted, reactionID)
			f.mu.Unlock()
		}
		return err
	}
	f.mu.Lock()
	f.reactionDeleted = append(f.reactionDeleted, reactionID)
	f.mu.Unlock()
	return nil
}
```

### Step 3.5: Verify build and tests

- [ ] **Step 3.5.a: Build entire module**

Run: `go build ./...`
Expected: PASS

- [ ] **Step 3.5.b: Run linear package tests**

Run: `go test ./internal/platform/linear/...`
Expected: PASS

- [ ] **Step 3.5.c: `go vet`**

Run: `go vet ./...`
Expected: no warnings.

### Step 3.6: Commit

- [ ] **Step 3.6.a: Stage and commit**

```bash
git add internal/platform/linear/client.go internal/platform/linear/client_test.go internal/platform/linear/handler_test.go
git commit -m "$(cat <<'EOF'
refactor(linear): remove Client.DeleteReaction

Drop the DeleteReaction method from the Client interface, the *httpClient
implementation, the reactionDeleteMutation constant, the fakeClient
shim, and the matching client-level test. No remaining caller.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: Final verification

- [ ] **Step 4.1: Run the full module build, linear tests, and vet one more time**

```bash
go build ./... && go test ./internal/platform/linear/... && go vet ./...
```
Expected: all pass.

- [ ] **Step 4.2: Confirm git log shows the three refactor commits on top of the spec commit**

Run: `git log --oneline -5`
Expected: the three commits from Tasks 1-3 plus the earlier `docs(linear): spec for removing ReactionDelete logic` commit.

---

## Notes for the implementer

- The Linear platform is the only consumer of `Client.DeleteReaction` — already grep-confirmed during design. No other package needs to change.
- Feishu has analogous `pendingReactions` plumbing under `internal/platform/feishu/`. Leave it alone — Linear is intentionally diverging.
- Existing changelog and historical specs/plans (`docs/superpowers/specs/2026-05-05-linear-reaction-design.md`, `docs/superpowers/plans/2026-05-05-linear-reaction.md`, `CHANGELOG`) stay as historical records. Do not retroactively edit them.
- The `reactionEmoji` constant (`":eyes:"`), `CreateReaction` API, `reactionCreateMutation`, and the `ReactionTarget` type all remain untouched.
