# Linear Reaction Support Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a `:eyes:` reaction lifecycle to the Linear platform that mirrors the Feishu pattern: receiver adds the reaction when an inbound message is dispatched, sender removes it after the reply comment is posted.

**Architecture:** Add `CreateReaction` / `DeleteReaction` to the existing Linear `Client` interface (GraphQL `reactionCreate` / `reactionDelete`). Allocate a single `*sync.Map` in `NewPlatform`, share it between `LinearReceiver` and `LinearSender`. The receiver fires the create call in a goroutine after each `dispatch(...)`, stores the resulting reaction ID in a buffered channel keyed by `PlatformMessageID`. The sender, after `CreateComment` succeeds, looks up the channel, reads the ID with a 5s timeout, and fires `DeleteReaction` in a background goroutine so the reply path is never blocked.

**Tech Stack:** Go, GraphQL over HTTP, `sync.Map`, `time.AfterFunc`, `utils.RetryWithBackoff`, table-driven Go tests with `httptest`.

**Spec:** `docs/superpowers/specs/2026-05-05-linear-reaction-design.md`

---

## File Structure

| Path | Role | Change |
|------|------|--------|
| `internal/platform/linear/client.go` | GraphQL client + `Client` interface | Add `ReactionTarget`, `CreateReaction`, `DeleteReaction` |
| `internal/platform/linear/client_test.go` | Client unit tests | Add reaction mutation tests |
| `internal/platform/linear/handler.go` | Receiver + sender + `LinearPlatform` | Wire `pendingReactions sync.Map`, receiver goroutines, sender delete-after-send, `reactionEmoji` constant |
| `internal/platform/linear/handler_test.go` | Receiver behavior tests | Extend `fakeClient` to track reaction calls, add lifecycle tests |
| `internal/platform/linear/sender_test.go` | Sender behavior tests | Add reaction-delete tests |
| `CHANGELOG.md` | Changelog | English entry under unreleased section |

---

## Task 1: Add `CreateReaction` to `*httpClient`

**Files:**
- Modify: `internal/platform/linear/client.go` (struct definition + new method)
- Test: `internal/platform/linear/client_test.go` (new test functions)

This task ships the concrete GraphQL call **without yet adding it to the `Client` interface** — that comes in Task 3 once the implementation is verified. Keeps `fakeClient` compiling between commits.

- [ ] **Step 1: Write failing test for create-on-comment**

Append to `internal/platform/linear/client_test.go`:

```go
func TestClient_CreateReaction_OnComment(t *testing.T) {
	_, c := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		s := string(body)
		if !strings.Contains(s, "reactionCreate") {
			t.Errorf("query missing reactionCreate: %s", s)
		}
		if !strings.Contains(s, `"commentId":"C9"`) {
			t.Errorf("variables missing commentId: %s", s)
		}
		if !strings.Contains(s, `"emoji":":eyes:"`) {
			t.Errorf("variables missing emoji: %s", s)
		}
		if strings.Contains(s, `"issueId":`) {
			t.Errorf("variables should not contain issueId when commentId is set: %s", s)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"reactionCreate": map[string]any{
					"reaction": map[string]any{"id": "R1"},
				},
			},
		})
	})

	id, err := c.CreateReaction(context.Background(), ReactionTarget{CommentID: "C9"}, ":eyes:")
	if err != nil {
		t.Fatalf("CreateReaction: %v", err)
	}
	if id != "R1" {
		t.Errorf("reactionID = %q, want R1", id)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/platform/linear/ -run TestClient_CreateReaction_OnComment -v`
Expected: FAIL with `undefined: ReactionTarget` and/or `c.CreateReaction undefined`.

- [ ] **Step 3: Add `ReactionTarget` and `CreateReaction` impl**

In `internal/platform/linear/client.go`, after the `FileUploadTicket` struct (around line 47) add:

```go
// ReactionTarget identifies what to react on. Exactly one of CommentID or
// IssueID must be non-empty; CommentID takes precedence when both are set.
type ReactionTarget struct {
	CommentID string
	IssueID   string
}
```

Below the existing mutations (after `fileUploadMutation`), add:

```go
const reactionCreateMutation = `
mutation ReactionCreate($input: ReactionCreateInput!) {
  reactionCreate(input: $input) { reaction { id } }
}`

func (c *httpClient) CreateReaction(ctx context.Context, target ReactionTarget, emoji string) (string, error) {
	input := map[string]any{"emoji": emoji}
	switch {
	case target.CommentID != "":
		input["commentId"] = target.CommentID
	case target.IssueID != "":
		input["issueId"] = target.IssueID
	default:
		return "", fmt.Errorf("linear: CreateReaction requires CommentID or IssueID")
	}
	vars := map[string]any{"input": input}
	var data struct {
		ReactionCreate struct {
			Reaction struct {
				ID string `json:"id"`
			} `json:"reaction"`
		} `json:"reactionCreate"`
	}
	if err := c.do(ctx, "reactionCreate", reactionCreateMutation, vars, &data); err != nil {
		return "", err
	}
	return data.ReactionCreate.Reaction.ID, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/platform/linear/ -run TestClient_CreateReaction_OnComment -v`
Expected: PASS.

- [ ] **Step 5: Add issue-target test + empty-target test**

Append to `internal/platform/linear/client_test.go`:

```go
func TestClient_CreateReaction_OnIssue(t *testing.T) {
	_, c := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		s := string(body)
		if !strings.Contains(s, `"issueId":"I1"`) {
			t.Errorf("variables missing issueId: %s", s)
		}
		if strings.Contains(s, `"commentId":`) {
			t.Errorf("variables should not contain commentId when only issueId is set: %s", s)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"reactionCreate": map[string]any{
					"reaction": map[string]any{"id": "R2"},
				},
			},
		})
	})

	id, err := c.CreateReaction(context.Background(), ReactionTarget{IssueID: "I1"}, ":eyes:")
	if err != nil {
		t.Fatalf("CreateReaction: %v", err)
	}
	if id != "R2" {
		t.Errorf("reactionID = %q, want R2", id)
	}
}

func TestClient_CreateReaction_RejectsEmptyTarget(t *testing.T) {
	_, c := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("HTTP should not be called when target is empty")
	})
	_, err := c.CreateReaction(context.Background(), ReactionTarget{}, ":eyes:")
	if err == nil {
		t.Fatal("expected error on empty target, got nil")
	}
}
```

- [ ] **Step 6: Run all reaction tests**

Run: `go test ./internal/platform/linear/ -run TestClient_CreateReaction -v`
Expected: PASS for all three subtests.

- [ ] **Step 7: Commit**

```bash
git add internal/platform/linear/client.go internal/platform/linear/client_test.go
git commit -m "feat(linear): add CreateReaction GraphQL mutation"
```

---

## Task 2: Add `DeleteReaction` to `*httpClient`

**Files:**
- Modify: `internal/platform/linear/client.go` (new method)
- Test: `internal/platform/linear/client_test.go` (new test function)

- [ ] **Step 1: Write failing test**

Append to `internal/platform/linear/client_test.go`:

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

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/platform/linear/ -run TestClient_DeleteReaction -v`
Expected: FAIL with `c.DeleteReaction undefined`.

- [ ] **Step 3: Add `DeleteReaction` impl**

Append to `internal/platform/linear/client.go` (after `CreateReaction`):

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

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/platform/linear/ -run TestClient_DeleteReaction -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/platform/linear/client.go internal/platform/linear/client_test.go
git commit -m "feat(linear): add DeleteReaction GraphQL mutation"
```

---

## Task 3: Promote reaction methods to the `Client` interface and extend `fakeClient`

**Files:**
- Modify: `internal/platform/linear/client.go` (`Client` interface)
- Modify: `internal/platform/linear/handler_test.go` (`fakeClient`)

After this task, `fakeClient` satisfies the new interface contract with default no-op behavior so all existing tests continue to compile and pass. No production behavior change yet.

- [ ] **Step 1: Add reaction methods to `Client` interface**

In `internal/platform/linear/client.go`, edit the `Client` interface (around lines 65-81). Add these two lines at the end (before the closing `}`):

```go
	// CreateReaction adds a reaction to the given target with the given emoji
	// shortcode (e.g. ":eyes:") and returns the new reaction's ID.
	CreateReaction(ctx context.Context, target ReactionTarget, emoji string) (string, error)
	// DeleteReaction removes a reaction by its ID.
	DeleteReaction(ctx context.Context, reactionID string) error
```

- [ ] **Step 2: Run package tests to confirm they break**

Run: `go test ./internal/platform/linear/ -count=1`
Expected: FAIL — `*fakeClient does not implement Client`.

- [ ] **Step 3: Extend `fakeClient` to satisfy the interface**

In `internal/platform/linear/handler_test.go`, locate the `fakeClient` struct (around line 24) and add fields for tracking reaction calls. Replace the existing struct declaration with:

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

Add the two methods near the other `fakeClient` methods (e.g. after `FileUpload` around line 72):

```go
func (f *fakeClient) CreateReaction(ctx context.Context, target ReactionTarget, emoji string) (string, error) {
	if f.createReactionImpl != nil {
		id, err := f.createReactionImpl(target, emoji)
		if err == nil {
			f.mu.Lock()
			f.reactionCreated = append(f.reactionCreated, struct {
				Target ReactionTarget
				Emoji  string
				ID     string
			}{target, emoji, id})
			f.mu.Unlock()
		}
		return id, err
	}
	f.mu.Lock()
	f.nextReactionID++
	id := fmt.Sprintf("R%d", f.nextReactionID)
	f.reactionCreated = append(f.reactionCreated, struct {
		Target ReactionTarget
		Emoji  string
		ID     string
	}{target, emoji, id})
	f.mu.Unlock()
	return id, nil
}

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

Add `"fmt"` to the imports at the top of `handler_test.go` if it is not already present.

- [ ] **Step 4: Run all linear tests**

Run: `go test ./internal/platform/linear/ -count=1`
Expected: PASS — all existing tests still green; no new tests added yet.

- [ ] **Step 5: Commit**

```bash
git add internal/platform/linear/client.go internal/platform/linear/handler_test.go
git commit -m "refactor(linear): expose CreateReaction/DeleteReaction on Client"
```

---

## Task 4: Wire shared `pendingReactions` into platform / receiver / sender

**Files:**
- Modify: `internal/platform/linear/handler.go`

This task only adds the shared `*sync.Map` field to the receiver and sender and allocates it in `NewPlatform`. It introduces no behavior change; subsequent tasks use the field.

- [ ] **Step 1: Add the field to `LinearReceiver` and `LinearSender`**

In `internal/platform/linear/handler.go`, edit the `LinearReceiver` struct declaration (around lines 84-93). Add `pendingReactions *sync.Map` as the last field:

```go
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

Edit the `LinearSender` struct (around lines 314-318):

```go
type LinearSender struct {
	client           Client
	uploader         *uploader
	pendingReactions *sync.Map
}
```

Add `"sync"` to the import block at the top of `handler.go` if it is not already present.

- [ ] **Step 2: Allocate the map in `NewPlatform` and wire it into both**

In `NewPlatform` (around lines 42-77), replace the body that constructs `&LinearPlatform{...}` to share a single map:

```go
	pending := &sync.Map{}
	return &LinearPlatform{
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
			pendingReactions: pending,
		},
		sender: &LinearSender{
			client: client,
			uploader: &uploader{
				client:  client,
				maxSize: maxSize,
				http:    &http.Client{Timeout: uploadPutTimeout + 30*time.Second},
			},
			pendingReactions: pending,
		},
	}, nil
```

- [ ] **Step 3: Run package tests**

Run: `go test ./internal/platform/linear/ -count=1`
Expected: PASS — existing tests still green; field is unused so no behavior change.

- [ ] **Step 4: Commit**

```bash
git add internal/platform/linear/handler.go
git commit -m "refactor(linear): plumb shared pendingReactions sync.Map"
```

---

## Task 5: Receiver fires `:eyes:` reaction on dispatch

**Files:**
- Modify: `internal/platform/linear/handler.go` (add `reactionEmoji`, `addReaction` helper, call sites in `tickOnce`)
- Test: `internal/platform/linear/handler_test.go` (3 new tests)

- [ ] **Step 1: Write failing test for initial-issue reaction**

Append to `internal/platform/linear/handler_test.go`:

```go
func TestReceiver_TickOnce_AddsReactionForInitialIssue(t *testing.T) {
	bot := User{ID: "BOT"}
	issue := Issue{
		ID: "I1", Identifier: "ENG-42", Title: "T",
		Team: Team{Key: "ENG"}, Creator: User{ID: "U2"},
	}
	fc := &fakeClient{viewer: bot, issues: func() ([]Issue, error) { return []Issue{issue}, nil }}
	pending := &sync.Map{}
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
		pendingReactions: pending,
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
	if _, ok := pending.Load("issue:I1"); !ok {
		t.Error("pendingReactions missing key issue:I1")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/platform/linear/ -run TestReceiver_TickOnce_AddsReactionForInitialIssue -v`
Expected: FAIL — `expected 1 CreateReaction call, got 0`.

- [ ] **Step 3: Add `reactionEmoji` constant and `addReaction` helper, call from `tickOnce`**

In `internal/platform/linear/handler.go`, add the constant near the existing constants (around line 30):

```go
// reactionEmoji is the shortcode used to acknowledge inbound dispatches; it
// is removed by the sender after the reply comment is posted.
const reactionEmoji = ":eyes:"

// reactionCleanupTTL bounds how long an unresolved pendingReactions entry
// lingers before being swept (memory-leak guard).
const reactionCleanupTTL = 10 * time.Minute
```

After `cleanStringList` (around line 141), add a helper method on `LinearReceiver`:

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

In `tickOnce` (around lines 158-198), add reaction calls right after each `dispatch(...)`. Replace the existing `dispatch(buildInitialInbound(...))` call site so that the surrounding block reads:

```go
		if !r.seenIssues.Contains(issue.ID) {
			nonBot := nonBotComments(issue.Comments)
			resolvedIssue := issue
			resolvedIssue.Description = r.resolver.Resolve(ctx, issue.Description)
			resolvedComments := make([]Comment, len(nonBot))
			for i, c := range nonBot {
				rc := c
				rc.Body = r.resolver.Resolve(ctx, c.Body)
				resolvedComments[i] = rc
			}
			log.Debug("tick: dispatch initial merged",
				zap.String("identifier", issue.Identifier),
				zap.String("issue_id", issue.ID),
				zap.Int("non_bot_comment_count", len(nonBot)),
			)
			dispatch(buildInitialInbound(resolvedIssue, resolvedComments))
			r.addReaction(ctx, "issue:"+issue.ID, ReactionTarget{IssueID: issue.ID})
			newIssueIDs = append(newIssueIDs, issue.ID)
			for _, c := range nonBot {
				newCommentIDs = append(newCommentIDs, c.ID)
			}
			continue
		}
```

And replace the per-comment dispatch site so the loop body reads:

```go
		for _, c := range issue.Comments {
			if r.seenComments.Contains(c.ID) {
				continue
			}
			if strings.HasPrefix(c.Body, botCommentPrefix) {
				continue
			}
			rc := c
			rc.Body = r.resolver.Resolve(ctx, c.Body)
			log.Debug("tick: dispatch comment",
				zap.String("identifier", issue.Identifier),
				zap.String("comment_id", c.ID),
				zap.String("user_id", c.User.ID),
			)
			dispatch(buildCommentInbound(issue, rc))
			r.addReaction(ctx, "comment:"+c.ID, ReactionTarget{CommentID: c.ID})
			newCommentIDs = append(newCommentIDs, c.ID)
		}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/platform/linear/ -run TestReceiver_TickOnce_AddsReactionForInitialIssue -v`
Expected: PASS.

- [ ] **Step 5: Add comment-target test**

Append to `internal/platform/linear/handler_test.go`:

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
	pending := &sync.Map{}
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
		pendingReactions: pending,
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
	if _, ok := pending.Load("comment:C1"); !ok {
		t.Error("pendingReactions missing key comment:C1")
	}
}
```

- [ ] **Step 6: Run new test**

Run: `go test ./internal/platform/linear/ -run TestReceiver_TickOnce_AddsReactionForNewComment -v`
Expected: PASS.

- [ ] **Step 7: Add failure-path test**

Append to `internal/platform/linear/handler_test.go`:

```go
func TestReceiver_TickOnce_ReactionCreateFails_DoesNotBlockDispatch(t *testing.T) {
	issue := Issue{
		ID: "I1", Identifier: "ENG-42", Title: "T",
		Team: Team{Key: "ENG"}, Creator: User{ID: "U2"},
	}
	fc := &fakeClient{
		viewer: User{ID: "BOT"},
		issues: func() ([]Issue, error) { return []Issue{issue}, nil },
		createReactionImpl: func(target ReactionTarget, emoji string) (string, error) {
			return "", errors.New("boom")
		},
	}
	pending := &sync.Map{}
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
		pendingReactions: pending,
	}

	var dispatched []platform.InboundMessage
	r.tickOnce(context.Background(), func(m platform.InboundMessage) { dispatched = append(dispatched, m) })

	if len(dispatched) != 1 {
		t.Fatalf("dispatch must run regardless of reaction failure; got %d", len(dispatched))
	}

	// Allow the reaction goroutine to finish before asserting.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		v, ok := pending.Load("issue:I1")
		if ok {
			if ch, ok := v.(chan string); ok {
				select {
				case _, open := <-ch:
					if !open {
						return // closed channel: failure path completed cleanly
					}
				default:
				}
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("reaction failure goroutine did not close channel within 2s")
}
```

Add `"errors"` to the import block of `handler_test.go` if not already present.

- [ ] **Step 8: Run all reaction receiver tests**

Run: `go test ./internal/platform/linear/ -run TestReceiver_TickOnce -v`
Expected: PASS — all existing receiver tests + 3 new ones.

- [ ] **Step 9: Run full package**

Run: `go test ./internal/platform/linear/ -count=1`
Expected: PASS.

- [ ] **Step 10: Commit**

```bash
git add internal/platform/linear/handler.go internal/platform/linear/handler_test.go
git commit -m "feat(linear): add :eyes: reaction on dispatch"
```

---

## Task 6: Sender removes reaction after reply

**Files:**
- Modify: `internal/platform/linear/handler.go` (sender path)
- Test: `internal/platform/linear/sender_test.go` (3 new tests)

- [ ] **Step 1: Write failing test for happy-path delete**

Append to `internal/platform/linear/sender_test.go`:

```go
func TestSender_DeletesReactionAfterReply(t *testing.T) {
	rawBytes, _ := json.Marshal(replyTarget{IssueID: "I1"})
	pending := &sync.Map{}
	ch := make(chan string, 1)
	ch <- "R7"
	pending.Store("issue:I1", ch)

	fc := &fakeClient{viewer: User{ID: "BOT"}}
	s := &LinearSender{client: fc, pendingReactions: pending}
	err := s.Send(context.Background(), platform.OutboundMessage{
		Content: "hi",
		ReplyTo: platform.InboundMessage{
			Raw:               string(rawBytes),
			PlatformMessageID: "issue:I1",
		},
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		fc.mu.Lock()
		n := len(fc.reactionDeleted)
		fc.mu.Unlock()
		if n == 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	fc.mu.Lock()
	defer fc.mu.Unlock()
	if len(fc.reactionDeleted) != 1 || fc.reactionDeleted[0] != "R7" {
		t.Errorf("reactionDeleted = %v, want [R7]", fc.reactionDeleted)
	}
	if _, ok := pending.Load("issue:I1"); ok {
		t.Error("pendingReactions still has key issue:I1 after Send")
	}
}
```

Add `"sync"` and `"time"` to the import block of `sender_test.go` if they are not already present.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/platform/linear/ -run TestSender_DeletesReactionAfterReply -v`
Expected: FAIL — `reactionDeleted = [], want [R7]`.

- [ ] **Step 3: Add `removeReaction` helper and invoke from `Send`**

In `internal/platform/linear/handler.go`, after the existing `addReaction` helper, add a sender-side helper:

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

In `LinearSender.Send` (around lines 320-346), invoke `removeReaction` immediately after the successful `CreateComment` returns. Replace the `return utils.RetryWithBackoff(...)` block at the end with:

```go
	if err := utils.RetryWithBackoff(ctx, func() error {
		_, err := s.client.CreateComment(ctx, target.IssueID, body, target.ParentCommentID)
		return err
	}, utils.DefaultRetryCount, utils.DefaultRetryDelay); err != nil {
		return err
	}
	s.removeReaction(msg.ReplyTo.PlatformMessageID)
	return nil
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/platform/linear/ -run TestSender_DeletesReactionAfterReply -v`
Expected: PASS.

- [ ] **Step 5: Add no-pending-entry test**

Append to `internal/platform/linear/sender_test.go`:

```go
func TestSender_NoPendingReaction_StillSucceeds(t *testing.T) {
	rawBytes, _ := json.Marshal(replyTarget{IssueID: "I1"})
	fc := &fakeClient{viewer: User{ID: "BOT"}}
	s := &LinearSender{client: fc, pendingReactions: &sync.Map{}}
	err := s.Send(context.Background(), platform.OutboundMessage{
		Content: "hi",
		ReplyTo: platform.InboundMessage{
			Raw:               string(rawBytes),
			PlatformMessageID: "issue:I1",
		},
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if len(fc.reactionDeleted) != 0 {
		t.Errorf("reactionDeleted = %v, want empty", fc.reactionDeleted)
	}
}
```

- [ ] **Step 6: Add delete-failure-tolerated test**

Append to `internal/platform/linear/sender_test.go`:

```go
func TestSender_ReactionDeleteFails_StillSucceeds(t *testing.T) {
	rawBytes, _ := json.Marshal(replyTarget{IssueID: "I1"})
	pending := &sync.Map{}
	ch := make(chan string, 1)
	ch <- "R8"
	pending.Store("issue:I1", ch)

	fc := &fakeClient{
		viewer: User{ID: "BOT"},
		deleteReactionImpl: func(id string) error {
			return errors.New("boom")
		},
	}
	s := &LinearSender{client: fc, pendingReactions: pending}
	err := s.Send(context.Background(), platform.OutboundMessage{
		Content: "hi",
		ReplyTo: platform.InboundMessage{
			Raw:               string(rawBytes),
			PlatformMessageID: "issue:I1",
		},
	})
	if err != nil {
		t.Fatalf("Send must not propagate reaction delete failure; got %v", err)
	}
}
```

Add `"errors"` to the import block of `sender_test.go` if not already present.

- [ ] **Step 7: Run all sender tests**

Run: `go test ./internal/platform/linear/ -run TestSender -v`
Expected: PASS — existing 2 sender tests + 3 new ones.

- [ ] **Step 8: Run full package**

Run: `go test ./internal/platform/linear/ -count=1`
Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add internal/platform/linear/handler.go internal/platform/linear/sender_test.go
git commit -m "feat(linear): remove reaction after reply comment posts"
```

---

## Task 7: Changelog entry

**Files:**
- Modify: `CHANGELOG.md`

User memory (`feedback_changelog_language.md`) requires changelog entries to be in **English**.

- [ ] **Step 1: Add entry**

Open `CHANGELOG.md`. Under the existing `## [Unreleased]` → `### Added` section, append a new bullet at the bottom of the list:

```markdown
- Linear platform now adds a `:eyes:` reaction to inbound issues and comments and removes it after the bot reply is posted, mirroring the Feishu reaction lifecycle.
```

- [ ] **Step 2: Build and run all tests one more time**

Run: `go build ./... && go test ./... -count=1`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add CHANGELOG.md
git commit -m "docs(changelog): note Linear reaction support"
```

---

## Final Verification

- [ ] **Step 1: Run full repo build + tests**

Run: `go build ./... && go test ./... -count=1`
Expected: PASS.

- [ ] **Step 2: Confirm git log shape**

Run: `git log --oneline main..HEAD`
Expected: 7 new commits, each independently meaningful.

- [ ] **Step 3: Sanity check the spec**

Re-read `docs/superpowers/specs/2026-05-05-linear-reaction-design.md`. Confirm every section under "Component changes" and "Failure handling and edge cases" has a corresponding implementation or test. If any gap exists, add a follow-up task and complete it.
