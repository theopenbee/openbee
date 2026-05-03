package linear

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

	self := newSelfComments()
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
	r := &LinearReceiver{client: fc, cursor: cur, labelName: "openbee", self: newSelfComments()}

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
	r := &LinearReceiver{client: fc, cursor: cur, labelName: "openbee", self: newSelfComments()}

	r.tickOnce(context.Background(), func(platform.InboundMessage) {})
	if !cur.saved.IsZero() {
		t.Errorf("cursor advanced under truncated page: %v", cur.saved)
	}
}

func TestSender_PostsCommentWithParentID(t *testing.T) {
	parent := "C0"
	rawBytes, _ := json.Marshal(replyTarget{IssueID: "I1", ParentCommentID: &parent})

	fc := &fakeClient{viewer: User{ID: "BOT"}}
	self := newSelfComments()
	s := &LinearSender{client: fc, self: self}
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
	if c.IssueID != "I1" || c.Body != "hello" || c.ParentID == nil || *c.ParentID != "C0" {
		t.Errorf("unexpected call: %+v", c)
	}
	// Sender must register the returned comment ID so the receiver skips it.
	if !self.Has("C-new") {
		t.Error("sender did not record outbound comment ID in self set")
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
	self := newSelfComments()
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
