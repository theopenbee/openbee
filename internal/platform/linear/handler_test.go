package linear

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/theopenbee/openbee/internal/infra/media"
	"github.com/theopenbee/openbee/internal/platform"
)

func testProjects() []string {
	return []string{"proj-a"}
}

func testStates() []string {
	return []string{"Todo", "In Progress"}
}

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

func (f *fakeClient) Viewer(ctx context.Context) (User, error) { return f.viewer, f.viewerErr }

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

func (f *fakeClient) DownloadAsset(ctx context.Context, url string, maxBytes int) ([]byte, string, error) {
	if data, ok := f.downloads[url]; ok {
		if maxBytes > 0 && len(data) > maxBytes {
			return nil, "", errors.New("asset exceeds max size")
		}
		return data, "image/png", nil
	}
	return nil, "", nil
}

func (f *fakeClient) FileUpload(ctx context.Context, name, mime string, size int) (FileUploadTicket, error) {
	if f.uploadImpl != nil {
		return f.uploadImpl(name, mime, size)
	}
	return FileUploadTicket{}, nil
}

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

type fakeSeenSet struct {
	ids   map[string]struct{}
	added []string
}

func newFakeSeenSet() *fakeSeenSet {
	return &fakeSeenSet{ids: make(map[string]struct{})}
}

func (f *fakeSeenSet) Load(_ context.Context) error { return nil }
func (f *fakeSeenSet) Contains(id string) bool      { _, ok := f.ids[id]; return ok }
func (f *fakeSeenSet) Add(_ context.Context, ids []string) error {
	for _, id := range ids {
		f.ids[id] = struct{}{}
	}
	f.added = append(f.added, ids...)
	return nil
}

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
			{ID: "C1", Body: "Saw it on Safari too", User: User{ID: "U2", Name: "Alice"}},
			{ID: "C-bot", Body: "[openbee-bot]\n\nignore me", User: bot},
			{ID: "C2", Body: "Probably the cookie domain", User: User{ID: "U3", Name: "Bob"}},
		},
	}
	fc := &fakeClient{
		viewer: bot,
		issues: func() ([]Issue, error) { return []Issue{issue}, nil },
	}
	seenIssues := newFakeSeenSet()
	seenComments := newFakeSeenSet()

	r := &LinearReceiver{
		client:       fc,
		seenIssues:   seenIssues,
		seenComments: seenComments,
		labelName:    "openbee",
		pollInterval: time.Hour,
		projectsList: testProjects(),
		statesList:   testStates(),
		resolver: &resolver{
			client:  fc,
			media:   media.NewService(),
			maxSize: 10 * 1024 * 1024,
		}}

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

func TestReceiver_Start_ViewerFailureDoesNotStopPolling(t *testing.T) {
	polled := make(chan struct{}, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	fc := &fakeClient{
		viewerErr: errors.New("temporary viewer failure"),
		issues: func() ([]Issue, error) {
			select {
			case polled <- struct{}{}:
			default:
			}
			cancel()
			return nil, nil
		},
	}

	r := &LinearReceiver{
		client:       fc,
		seenIssues:   newFakeSeenSet(),
		seenComments: newFakeSeenSet(),
		labelName:    "openbee",
		pollInterval: 5 * time.Millisecond,
		projectsList: testProjects(),
		statesList:   testStates(),
		resolver: &resolver{
			client:  fc,
			media:   media.NewService(),
			maxSize: 10 * 1024 * 1024,
		},
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- r.Start(ctx, func(platform.InboundMessage) {})
	}()

	select {
	case <-polled:
	case err := <-errCh:
		t.Fatalf("Start returned before polling after viewer failure: %v", err)
	case <-time.After(200 * time.Millisecond):
		t.Fatal("receiver did not poll after viewer failure")
	}

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Start after cancellation returned error: %v", err)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("receiver did not stop after context cancellation")
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
			{ID: "C1", Body: "old comment", User: User{ID: "U2"}},
			{ID: "C2", Body: "new comment", User: User{ID: "U3"}},
		},
	}
	fc := &fakeClient{
		viewer: bot,
		issues: func() ([]Issue, error) { return []Issue{issue}, nil },
	}
	seenIssues := newFakeSeenSet()
	seenIssues.ids["I1"] = struct{}{}
	seenComments := newFakeSeenSet()
	seenComments.ids["C1"] = struct{}{}

	r := &LinearReceiver{
		client:       fc,
		seenIssues:   seenIssues,
		seenComments: seenComments,
		labelName:    "openbee",
		pollInterval: time.Hour,
		projectsList: testProjects(),
		statesList:   testStates(),
		resolver: &resolver{
			client:  fc,
			media:   media.NewService(),
			maxSize: 10 * 1024 * 1024,
		}}

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
			{ID: "C-bot-A", Body: "[openbee-bot]\n\nx", User: bot},
		},
	}
	// Issue is known — per-comment dispatch path.
	issueB := Issue{
		ID: "IB", Identifier: "ENG-2", Title: "B", Team: Team{Key: "ENG"}, Creator: User{ID: "U2"},
		Comments: []Comment{
			{ID: "C-bot-B", Body: "[openbee-bot]\n\ny", User: bot},
		},
	}
	fc := &fakeClient{viewer: bot, issues: func() ([]Issue, error) { return []Issue{issueA, issueB}, nil }}
	seenIssues := newFakeSeenSet()
	seenIssues.ids["IB"] = struct{}{}

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
		}}

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
		seenIssues:   newFakeSeenSet(),
		seenComments: newFakeSeenSet(),
		labelName:    "openbee",
		pollInterval: time.Hour,
		projectsList: testProjects(),
		statesList:   nil, // empty
		resolver: &resolver{
			client:  fc,
			media:   media.NewService(),
			maxSize: 10 * 1024 * 1024,
		}}
	r.tickOnce(context.Background(), func(platform.InboundMessage) {})
}

func TestReceiver_TickOnce_EmptyProjectsSkipsTick(t *testing.T) {
	fc := &fakeClient{viewer: User{ID: "BOT"}, issues: func() ([]Issue, error) {
		t.Fatal("issues should not be queried when projects is empty")
		return nil, nil
	}}
	r := &LinearReceiver{
		client:       fc,
		seenIssues:   newFakeSeenSet(),
		seenComments: newFakeSeenSet(),
		labelName:    "openbee",
		pollInterval: time.Hour,
		projectsList: nil, // empty
		statesList:   testStates(),
		resolver: &resolver{
			client:  fc,
			media:   media.NewService(),
			maxSize: 10 * 1024 * 1024,
		}}
	r.tickOnce(context.Background(), func(platform.InboundMessage) {})
}

func TestReceiver_TickOnce_FiltersEmptyConfiguredValues(t *testing.T) {
	fc := &fakeClient{
		viewer: User{ID: "BOT"},
		issues: func() ([]Issue, error) {
			return nil, nil
		},
	}
	r := &LinearReceiver{
		client:       fc,
		seenIssues:   newFakeSeenSet(),
		seenComments: newFakeSeenSet(),
		labelName:    "openbee",
		pollInterval: time.Hour,
		projectsList: cleanStringList([]string{"", "proj-a", ""}),
		statesList:   cleanStringList([]string{"Todo", "", "In Progress"}),
	}

	r.tickOnce(context.Background(), func(platform.InboundMessage) {})

	if fc.calls != 1 {
		t.Fatalf("expected one Linear query, got %d", fc.calls)
	}
	if got, want := strings.Join(fc.lastProjects, ","), "proj-a"; got != want {
		t.Errorf("projects = %q, want %q", got, want)
	}
	if got, want := strings.Join(fc.lastStates, ","), "Todo,In Progress"; got != want {
		t.Errorf("states = %q, want %q", got, want)
	}
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
		}}
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
			{ID: "C1", Body: "hi", User: User{ID: "U2", Name: "Alice"}},
		},
	}
	fc := &fakeClient{viewer: User{ID: "BOT"}, issues: func() ([]Issue, error) { return []Issue{issue}, nil }}
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
		}}
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

func TestReceiver_TickOnce_MixedNewAndKnownIssues(t *testing.T) {
	bot := User{ID: "BOT"}
	// New issue with a non-bot comment → merged-initial dispatch
	issueA := Issue{
		ID: "IA", Identifier: "ENG-1",
		Title: "A title", Description: "A desc",
		Team: Team{Key: "ENG"}, Creator: User{ID: "U2", Name: "Alice"},
		Comments: []Comment{
			{ID: "CA1", Body: "from history", User: User{ID: "U2", Name: "Alice"}},
		},
	}
	// Known issue with one already-seen comment and one new comment
	issueB := Issue{
		ID: "IB", Identifier: "ENG-2",
		Title: "B title",
		Team:  Team{Key: "ENG"}, Creator: User{ID: "U2"},
		Comments: []Comment{
			{ID: "CB-old", Body: "already seen", User: User{ID: "U3"}},
			{ID: "CB-new", Body: "fresh", User: User{ID: "U3", Name: "Bob"}},
		},
	}

	seenIssues := newFakeSeenSet()
	seenIssues.ids["IB"] = struct{}{}
	seenComments := newFakeSeenSet()
	seenComments.ids["CB-old"] = struct{}{}

	fc := &fakeClient{viewer: bot, issues: func() ([]Issue, error) { return []Issue{issueA, issueB}, nil }}
	r := &LinearReceiver{
		client:       fc,
		seenIssues:   seenIssues,
		seenComments: seenComments,
		labelName:    "openbee",
		pollInterval: time.Hour,
		projectsList: testProjects(),
		statesList:   testStates(),
		resolver: &resolver{
			client:  fc,
			media:   media.NewService(),
			maxSize: 10 * 1024 * 1024,
		}}

	var received []platform.InboundMessage
	r.tickOnce(context.Background(), func(m platform.InboundMessage) { received = append(received, m) })

	if len(received) != 2 {
		t.Fatalf("expected 2 dispatches (issue:IA + comment:CB-new), got %d", len(received))
	}
	if received[0].PlatformMessageID != "issue:IA" {
		t.Errorf("first dispatch PlatformMessageID = %q, want issue:IA", received[0].PlatformMessageID)
	}
	if received[1].PlatformMessageID != "comment:CB-new" {
		t.Errorf("second dispatch PlatformMessageID = %q, want comment:CB-new", received[1].PlatformMessageID)
	}
	if !seenIssues.Contains("IA") {
		t.Error("seenIssues missing IA")
	}
	if !seenComments.Contains("CA1") {
		t.Error("seenComments missing folded CA1")
	}
	if !seenComments.Contains("CB-new") {
		t.Error("seenComments missing CB-new")
	}
}

func TestReceiver_TickOnce_KnownIssueCommentRetainsParentID(t *testing.T) {
	bot := User{ID: "BOT"}
	parent := "C-parent"
	issue := Issue{
		ID: "I1", Identifier: "ENG-42", Title: "T",
		Team: Team{Key: "ENG"}, Creator: User{ID: "U2"},
		Comments: []Comment{
			{ID: "C-reply", Body: "thread reply", User: User{ID: "U3"}, ParentID: &parent},
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
		}}

	var received []platform.InboundMessage
	r.tickOnce(context.Background(), func(m platform.InboundMessage) { received = append(received, m) })

	if len(received) != 1 {
		t.Fatalf("expected 1 dispatch, got %d", len(received))
	}
	var got replyTarget
	if err := json.Unmarshal([]byte(received[0].Raw), &got); err != nil {
		t.Fatalf("unmarshal Raw: %v", err)
	}
	if got.IssueID != "I1" {
		t.Errorf("Raw IssueID = %q, want I1", got.IssueID)
	}
	if got.ParentCommentID == nil || *got.ParentCommentID != "C-parent" {
		t.Errorf("Raw ParentCommentID = %v, want \"C-parent\"", got.ParentCommentID)
	}
}

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

func TestReceiver_TickOnce_FirstSightWithProjectIncludesProjectHeader(t *testing.T) {
	proj := &Project{ID: "P1", Name: "Backend"}
	issue := Issue{
		ID: "I1", Identifier: "ENG-42",
		Title: "Fix login", Description: "Users get 401.",
		Team: Team{Key: "ENG"}, Creator: User{ID: "U2"},
		Project: proj,
	}
	fc := &fakeClient{viewer: User{ID: "BOT"}, issues: func() ([]Issue, error) { return []Issue{issue}, nil }}
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

	var got []platform.InboundMessage
	r.tickOnce(context.Background(), func(m platform.InboundMessage) { got = append(got, m) })

	if len(got) != 1 {
		t.Fatalf("expected 1 dispatch, got %d", len(got))
	}
	want := "[Project: Backend]\n\nFix login\n\nUsers get 401."
	if got[0].Content != want {
		t.Errorf("Content mismatch.\nwant: %q\ngot:  %q", want, got[0].Content)
	}
}

func TestReceiver_TickOnce_CommentDispatchHasNoProjectHeader(t *testing.T) {
	proj := &Project{ID: "P1", Name: "Backend"}
	issue := Issue{
		ID: "I1", Identifier: "ENG-42", Title: "Fix login",
		Team: Team{Key: "ENG"}, Creator: User{ID: "U2"},
		Project: proj,
		Comments: []Comment{
			{ID: "C1", Body: "new comment", User: User{ID: "U3", Name: "Bob"}},
		},
	}
	seenIssues := newFakeSeenSet()
	seenIssues.ids["I1"] = struct{}{} // already seen — triggers comment path

	fc := &fakeClient{viewer: User{ID: "BOT"}, issues: func() ([]Issue, error) { return []Issue{issue}, nil }}
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

	var got []platform.InboundMessage
	r.tickOnce(context.Background(), func(m platform.InboundMessage) { got = append(got, m) })

	if len(got) != 1 {
		t.Fatalf("expected 1 dispatch, got %d", len(got))
	}
	if got[0].PlatformMessageID != "comment:C1" {
		t.Errorf("expected comment:C1, got %s", got[0].PlatformMessageID)
	}
	if strings.Contains(got[0].Content, "[Project:") {
		t.Errorf("comment dispatch should not contain project header, got: %q", got[0].Content)
	}
}

func TestReceiver_TickOnce_ResolvesAssetURLsInDescriptionAndComments(t *testing.T) {
	bot := User{ID: "BOT"}
	descURL := "https://uploads.linear.app/d/desc.png"
	commURL := "https://uploads.linear.app/d/comm.png"
	issue := Issue{
		ID: "I1", Identifier: "ENG-42",
		Title:       "Fix login",
		Description: "Snapshot ![desc](" + descURL + ")",
		Team:        Team{Key: "ENG"}, Creator: User{ID: "U2", Name: "Alice"},
		Comments: []Comment{
			{ID: "C1", Body: "Repro: ![c1](" + commURL + ")", User: User{ID: "U2", Name: "Alice"}},
		},
	}

	fc := &fakeClient{viewer: bot, issues: func() ([]Issue, error) { return []Issue{issue}, nil }}
	fc.downloads = map[string][]byte{
		descURL: []byte("PNG-D"),
		commURL: []byte("PNG-C"),
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

	var received []platform.InboundMessage
	r.tickOnce(context.Background(), func(m platform.InboundMessage) { received = append(received, m) })
	if len(received) != 1 {
		t.Fatalf("got %d", len(received))
	}
	body := received[0].Content
	if strings.Contains(body, descURL) {
		t.Errorf("description URL not replaced: %q", body)
	}
	if strings.Contains(body, commURL) {
		t.Errorf("comment URL not replaced: %q", body)
	}
	if !strings.Contains(body, "<media:image") {
		t.Errorf("expected placeholders in body: %q", body)
	}
}
