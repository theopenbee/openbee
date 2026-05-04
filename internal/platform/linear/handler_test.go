package linear

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

// fakeClient is a Client that returns canned data per call.
type fakeClient struct {
	mu      sync.Mutex
	viewer  User
	calls   int
	issues  func(since time.Time) ([]Issue, bool, error)
	created []struct {
		IssueID, Body string
		ParentID      *string
	}
}

func (f *fakeClient) Viewer(ctx context.Context) (User, error) { return f.viewer, nil }

func (f *fakeClient) IssuesUpdatedSince(ctx context.Context, since time.Time, label string) ([]Issue, bool, error) {
	f.mu.Lock()
	f.calls++
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
			{ID: "C-bot", Body: "ignore me", CreatedAt: mustParse(t, "2026-05-02T11:15:00Z"), User: bot, IssueID: "I1"},
			{ID: "C2", Body: "second", CreatedAt: mustParse(t, "2026-05-02T11:30:00Z"), User: User{ID: "U2"}, IssueID: "I1"},
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
	self.Add("C-bot")
	r := &LinearReceiver{
		client:    fc,
		cursor:    cur,
		labelName: "openbee",
		self:      self,
	}

	var got []platform.InboundMessage
	r.tickOnce(context.Background(), func(m platform.InboundMessage) { got = append(got, m) })

	// Expect 3 dispatches: issue body, C1, C2 (C-bot filtered via self set).
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
	self, err := newSelfComments(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	r := &LinearReceiver{client: fc, cursor: cur, labelName: "openbee", self: self}

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
	self, err := newSelfComments(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	r := &LinearReceiver{client: fc, cursor: cur, labelName: "openbee", self: self}

	r.tickOnce(context.Background(), func(platform.InboundMessage) {})
	if !cur.saved.IsZero() {
		t.Errorf("cursor advanced under truncated page: %v", cur.saved)
	}
}

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

// When the bot uses the user's own personal API key (a common configuration),
// human comments share the bot's user ID. They must still be dispatched —
// only IDs the sender recorded as outbound should be skipped.
func TestReceiver_TickOnce_DispatchesHumanCommentSharingBotUserID(t *testing.T) {
	bot := User{ID: "U1"}
	since := mustParse(t, "2026-05-02T10:30:00Z")
	issue := Issue{
		ID:         "I1",
		Identifier: "ENG-42",
		Title:      "T",
		Team:       Team{Key: "ENG"},
		Creator:    User{ID: "U1"},
		// Issue and label predate `since` so the issue itself isn't re-dispatched.
		CreatedAt: mustParse(t, "2026-05-02T10:00:00Z"),
		UpdatedAt: mustParse(t, "2026-05-02T11:30:00Z"),
		Labels: []IssueLabel{
			{Name: "openbee", CreatedAt: mustParse(t, "2026-05-02T10:00:00Z")},
		},
		Comments: []Comment{
			// Bot's own outbound comment — registered in the self set.
			{ID: "C-bot", Body: "bot reply", CreatedAt: mustParse(t, "2026-05-02T11:00:00Z"), User: bot},
			// Human comment posted via the same Linear account.
			{ID: "C-human", Body: "现在有哪些员工?", CreatedAt: mustParse(t, "2026-05-02T11:15:00Z"), User: bot},
			// Human reply nested under bot's comment.
			{ID: "C-reply", Body: "你好呀", CreatedAt: mustParse(t, "2026-05-02T11:30:00Z"), User: bot, ParentID: strPtr("C-bot")},
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
	self.Add("C-bot")

	r := &LinearReceiver{client: fc, cursor: cur, labelName: "openbee", self: self}

	var got []platform.InboundMessage
	r.tickOnce(context.Background(), func(m platform.InboundMessage) { got = append(got, m) })

	if len(got) != 2 {
		t.Fatalf("dispatched %d, want 2 (C-human, C-reply): %+v", len(got), got)
	}
	sort.Slice(got, func(i, j int) bool { return got[i].MessageTime < got[j].MessageTime })
	if got[0].PlatformMessageID != "comment:C-human" || got[1].PlatformMessageID != "comment:C-reply" {
		t.Errorf("unexpected dispatch IDs: %+v", got)
	}
}

func strPtr(s string) *string { return &s }

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

func TestSelfComments_PersistsAcrossRestart(t *testing.T) {
	dir := t.TempDir()

	a, err := newSelfComments(dir)
	if err != nil {
		t.Fatalf("newSelfComments: %v", err)
	}
	a.Add("C1")
	a.Add("C2")
	a.Add("C1") // duplicate — must not double-write
	if !a.Has("C1") || !a.Has("C2") {
		t.Fatal("set missing IDs after Add")
	}

	// Simulate restart: a fresh instance pointed at the same dir must
	// reload the previously-recorded IDs.
	b, err := newSelfComments(dir)
	if err != nil {
		t.Fatalf("newSelfComments restart: %v", err)
	}
	if !b.Has("C1") || !b.Has("C2") {
		t.Errorf("after restart, set missing recorded IDs")
	}
	if b.Has("C-other") {
		t.Errorf("unexpected ID survived restart")
	}

	// File should contain exactly two lines (no duplicates from the duplicate Add).
	data, err := os.ReadFile(filepath.Join(dir, "self_comments.log"))
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) != 2 {
		t.Errorf("expected 2 lines, got %d: %q", len(lines), data)
	}
}

func TestSelfComments_ConcurrentAddsAreAtomic(t *testing.T) {
	dir := t.TempDir()
	s, err := newSelfComments(dir)
	if err != nil {
		t.Fatalf("newSelfComments: %v", err)
	}
	const N = 200
	var wg sync.WaitGroup
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			s.Add(fmt.Sprintf("C%d", i))
		}(i)
	}
	wg.Wait()
	for i := 0; i < N; i++ {
		if !s.Has(fmt.Sprintf("C%d", i)) {
			t.Fatalf("missing ID C%d", i)
		}
	}
	data, err := os.ReadFile(filepath.Join(dir, "self_comments.log"))
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) != N {
		t.Errorf("expected %d lines, got %d", N, len(lines))
	}
	seen := map[string]struct{}{}
	for _, l := range lines {
		if _, dup := seen[l]; dup {
			t.Errorf("duplicate line in log: %q", l)
		}
		seen[l] = struct{}{}
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
