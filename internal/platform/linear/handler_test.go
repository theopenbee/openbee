package linear

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/theopenbee/openbee/internal/domain/linearcfg"
	"github.com/theopenbee/openbee/internal/platform"
)

// testProjectStore returns a linearcfg.Store seeded with one project so the
// tickOnce path is not gated out by the empty-list policy.
func testProjectStore() *linearcfg.Store {
	return linearcfg.NewStore([]string{"proj-a"})
}

// fakeClient is a Client that returns canned data per call.
type fakeClient struct {
	mu           sync.Mutex
	viewer       User
	calls        int
	lastProjects []string
	issues       func(since time.Time) ([]Issue, bool, error)
	created      []struct {
		IssueID, Body string
		ParentID      *string
	}
}

func (f *fakeClient) Viewer(ctx context.Context) (User, error) { return f.viewer, nil }

func (f *fakeClient) IssuesUpdatedSince(ctx context.Context, since time.Time, label string, projects []string) ([]Issue, bool, error) {
	f.mu.Lock()
	f.calls++
	f.lastProjects = append([]string(nil), projects...)
	f.mu.Unlock()
	return f.issues(since)
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

type fakeCursor struct {
	last  time.Time
	saved time.Time
}

func (c *fakeCursor) Load(ctx context.Context) (time.Time, error) { return c.last, nil }
func (c *fakeCursor) Save(ctx context.Context, t time.Time) error { c.saved = t; return nil }

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

func mustParse(t *testing.T, s string) time.Time {
	t.Helper()
	v, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestReceiver_TickOnce_DispatchesIssueAndComments(t *testing.T) {
	bot := User{ID: "BOT"}
	since := mustParse(t, "2026-05-02T09:00:00Z")
	issue := Issue{
		ID:          "I1",
		Identifier:  "ENG-42",
		Title:       "Title",
		Description: "Body",
		CreatedAt:   mustParse(t, "2026-05-02T10:00:00Z"),
		UpdatedAt:   mustParse(t, "2026-05-02T11:30:00Z"),
		Team:        Team{Key: "ENG"},
		Creator:     User{ID: "U2"},
		Labels: []IssueLabel{
			{Name: "openbee", CreatedAt: mustParse(t, "2026-05-02T10:30:00Z")},
		},
		Comments: []Comment{
			{ID: "C1", Body: "first", CreatedAt: mustParse(t, "2026-05-02T11:00:00Z"), User: User{ID: "U2"}, IssueID: "I1"},
			{ID: "C-bot", Body: "[openbee-bot]\n\nignore me", CreatedAt: mustParse(t, "2026-05-02T11:15:00Z"), User: bot, IssueID: "I1"},
			{ID: "C2", Body: "second", CreatedAt: mustParse(t, "2026-05-02T11:30:00Z"), User: User{ID: "U2"}, IssueID: "I1"},
		},
	}
	fc := &fakeClient{
		viewer: bot,
		issues: func(_ time.Time) ([]Issue, bool, error) { return []Issue{issue}, false, nil },
	}
	cur := &fakeCursor{last: since}

	r := &LinearReceiver{
		client:       fc,
		cursor:       cur,
		labelName:    "openbee",
		projectStore: testProjectStore(),
		seenComments: newFakeSeen(),
	}

	var got []platform.InboundMessage
	r.tickOnce(context.Background(), func(m platform.InboundMessage) { got = append(got, m) })

	// Expect 3 dispatches: issue body, C1, C2 (C-bot filtered via body prefix).
	if len(got) != 3 {
		t.Fatalf("dispatched %d messages, want 3: %+v", len(got), got)
	}
	// Sort for stable assertions (dispatch order should already be chronological).
	sort.Slice(got, func(i, j int) bool { return got[i].MessageTime < got[j].MessageTime })
	if got[0].PlatformMessageID != "issue:I1" {
		t.Errorf("first dispatch should be issue body: %+v", got[0])
	}
	if got[0].SessionKey != "linear:ENG:ENG-42" {
		t.Errorf("session key: %q", got[0].SessionKey)
	}
	if got[1].PlatformMessageID != "comment:C1" || got[2].PlatformMessageID != "comment:C2" {
		t.Errorf("comment IDs out of order: %+v", got)
	}
	// Cursor advanced to issue.UpdatedAt or last comment, whichever later.
	if !cur.saved.Equal(issue.UpdatedAt) {
		t.Errorf("cursor saved = %v, want %v", cur.saved, issue.UpdatedAt)
	}
}

func TestReceiver_TickOnce_ErrorDoesNotAdvanceCursor(t *testing.T) {
	cur := &fakeCursor{last: mustParse(t, "2026-05-02T09:00:00Z")}
	fc := &fakeClient{
		viewer: User{ID: "BOT"},
		issues: func(_ time.Time) ([]Issue, bool, error) { return nil, false, errors.New("boom") },
	}
	r := &LinearReceiver{client: fc, cursor: cur, labelName: "openbee", projectStore: testProjectStore(), seenComments: newFakeSeen()}

	r.tickOnce(context.Background(), func(platform.InboundMessage) {})
	if !cur.saved.IsZero() {
		t.Errorf("cursor advanced on error: %v", cur.saved)
	}
}

func TestReceiver_TickOnce_TruncatedDoesNotAdvanceCursor(t *testing.T) {
	since := mustParse(t, "2026-05-02T09:00:00Z")
	issue := Issue{
		ID: "I1", Identifier: "ENG-1", Title: "T", Team: Team{Key: "ENG"},
		Creator:   User{ID: "U2"},
		CreatedAt: mustParse(t, "2026-05-02T10:00:00Z"),
		UpdatedAt: mustParse(t, "2026-05-02T11:00:00Z"),
	}
	cur := &fakeCursor{last: since}
	fc := &fakeClient{
		viewer: User{ID: "BOT"},
		issues: func(_ time.Time) ([]Issue, bool, error) { return []Issue{issue}, true, nil },
	}
	r := &LinearReceiver{client: fc, cursor: cur, labelName: "openbee", projectStore: testProjectStore(), seenComments: newFakeSeen()}

	r.tickOnce(context.Background(), func(platform.InboundMessage) {})
	if !cur.saved.IsZero() {
		t.Errorf("cursor advanced under truncated page: %v", cur.saved)
	}
}

func TestSender_PostsCommentWithParentID(t *testing.T) {
	parent := "C0"
	rawBytes, _ := json.Marshal(replyTarget{IssueID: "I1", ParentCommentID: &parent})

	fc := &fakeClient{viewer: User{ID: "BOT"}}
	s := &LinearSender{client: fc}
	err := s.Send(context.Background(), platform.OutboundMessage{
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

func TestSender_RejectsMediaPath(t *testing.T) {
	rawBytes, _ := json.Marshal(replyTarget{IssueID: "I1"})
	s := &LinearSender{client: &fakeClient{}}
	err := s.Send(context.Background(), platform.OutboundMessage{
		Content:   "x",
		MediaPath: "/tmp/foo.png",
		ReplyTo:   platform.InboundMessage{Raw: string(rawBytes)},
	})
	if err == nil {
		t.Error("expected error for MediaPath")
	}
}

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
	r := &LinearReceiver{client: fc, cursor: cur, labelName: "openbee", projectStore: testProjectStore(), seenComments: newFakeSeen()}

	var got []platform.InboundMessage
	r.tickOnce(context.Background(), func(m platform.InboundMessage) { got = append(got, m) })

	if len(got) != 1 {
		t.Fatalf("dispatched %d, want 1: %+v", len(got), got)
	}
	if got[0].PlatformMessageID != "comment:C-user" {
		t.Errorf("unexpected dispatch: %+v", got[0])
	}
}

// TestReceiver_TickOnce_EmptyProjectsSkipsAPI verifies the policy that an
// empty project allow-list causes the receiver to skip the tick entirely
// (no API call, no cursor advance).
func TestReceiver_TickOnce_EmptyProjectsSkipsAPI(t *testing.T) {
	cur := &fakeCursor{last: mustParse(t, "2026-05-02T09:00:00Z")}
	fc := &fakeClient{
		viewer: User{ID: "BOT"},
		issues: func(_ time.Time) ([]Issue, bool, error) {
			t.Fatal("IssuesUpdatedSince should not be called when projects is empty")
			return nil, false, nil
		},
	}
	r := &LinearReceiver{
		client:       fc,
		cursor:       cur,
		labelName:    "openbee",
		projectStore: linearcfg.NewStore(nil),
		seenComments: newFakeSeen(),
	}
	r.tickOnce(context.Background(), func(platform.InboundMessage) {
		t.Fatal("dispatch should not be called when projects is empty")
	})
	if !cur.saved.IsZero() {
		t.Errorf("cursor advanced when projects is empty: %v", cur.saved)
	}
	if fc.calls != 0 {
		t.Errorf("client called %d times, want 0", fc.calls)
	}
}

// TestReceiver_TickOnce_ForwardsProjectsToClient verifies that the project
// allow-list is passed through to the GraphQL client unchanged.
func TestReceiver_TickOnce_ForwardsProjectsToClient(t *testing.T) {
	cur := &fakeCursor{last: mustParse(t, "2026-05-02T09:00:00Z")}
	fc := &fakeClient{
		viewer: User{ID: "BOT"},
		issues: func(_ time.Time) ([]Issue, bool, error) { return nil, false, nil },
	}
	r := &LinearReceiver{
		client:       fc,
		cursor:       cur,
		labelName:    "openbee",
		projectStore: linearcfg.NewStore([]string{"alpha", "beta"}),
		seenComments: newFakeSeen(),
	}
	r.tickOnce(context.Background(), func(platform.InboundMessage) {})
	if got := fc.lastProjects; len(got) != 2 || got[0] != "alpha" || got[1] != "beta" {
		t.Errorf("client received projects = %v, want [alpha beta]", got)
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
	r := &LinearReceiver{client: fc, cursor: cur, labelName: "openbee", projectStore: testProjectStore(), seenComments: newFakeSeen()}

	var got []platform.InboundMessage
	r.tickOnce(context.Background(), func(m platform.InboundMessage) { got = append(got, m) })

	if len(got) != 1 {
		t.Fatalf("dispatched %d, want 1 (the user quote): %+v", len(got), got)
	}
	if got[0].PlatformMessageID != "comment:C-user" {
		t.Errorf("unexpected dispatch: %+v", got[0])
	}
}

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
