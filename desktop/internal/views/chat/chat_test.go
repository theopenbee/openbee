package chat_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/api"
	"github.com/theopenbee/openbee/desktop/internal/fakeapi"
	"github.com/theopenbee/openbee/desktop/internal/kit"
	"github.com/theopenbee/openbee/desktop/internal/model"
	"github.com/theopenbee/openbee/desktop/internal/theme"
	"github.com/theopenbee/openbee/desktop/internal/views/chat"
	"github.com/theopenbee/openbee/desktop/internal/views/route"
	"github.com/theopenbee/openbee/desktop/internal/views/testkit"
	"github.com/theopenbee/openbee/desktop/locales"
)

const (
	wideW, wideH  = 1440, 900
	narrowW       = 700
	workerCID     = "worker:w1"
	frameBudget   = 16 * time.Millisecond
	scrollFrames  = 20
	historySize   = 2000
	longLines     = 3000
	pageMessages  = 80
	followHistory = 40
	scrollX       = 60
)

type fixture struct {
	*testkit.Harness
	t       *testing.T
	router  *ui.Router
	view    *chat.View
	toggled []string
}

func newFixture(t *testing.T, w, h int, param string, seed func(st *fakeapi.State)) *fixture {
	t.Helper()
	return openFixture(t, w, h, param, seed, nil)
}

func openFixture(t *testing.T, w, h int, param string, seed func(st *fakeapi.State), before func(fake *fakeapi.Server)) *fixture {
	t.Helper()
	hs := testkit.SignedIn(t, w, h, seed)
	if before != nil {
		before(hs.Fake)
	}
	f := &fixture{Harness: hs, t: t, router: ui.NewRouter(route.Chat(param))}
	f.router.Transition = ui.TransitionNone
	f.view = chat.New(hs.Store, f.router, func(cid string) { f.toggled = append(f.toggled, cid) })
	hs.T = ui.NewTester(f.build, w, h)
	f.Settle()
	return f
}

func (f *fixture) build(c *ui.Context) {
	tk := theme.Apply(c)
	ui.Column(c).Fill().Background(tk.Background).TextColor(tk.Foreground).Children(func() {
		f.router.View(c, func(rt *ui.Route) {
			if !rt.Match(route.ChatPath + "/{conversation}") {
				return
			}
			param := rt.Param("conversation")
			cid := route.ConversationID(param)
			if f.Store.Active() != cid {
				f.Store.SetActive(cid)
			}
			f.view.View(c, cid, route.ParsePanel(param, f.router.Query(route.PanelQuery)))
		}).Grow(1)
	})
	kit.ShowToasts(c, tk, &f.Store.Toasts)
}

func (f *fixture) settleMotion() {
	f.Settle()
	f.T.Advance(time.Second)
	f.T.Frame()
}

func (f *fixture) click(name string) {
	f.t.Helper()
	if err := f.T.Click(name); err != nil {
		f.t.Fatalf("click %q: %v (texts %q)", name, err, f.T.Texts())
	}
	f.settleMotion()
}

func (f *fixture) clickRightOf(name string) {
	f.t.Helper()
	r, ok := f.T.Find(name)
	if !ok {
		f.t.Fatalf("no %q to click beside; texts %q", name, f.T.Texts())
	}
	f.T.ClickAt(r.X+r.W+40, r.Y+r.H/2)
	f.settleMotion()
}

func (f *fixture) shows(text string) bool {
	_, ok := f.T.Find(text)
	return ok
}

func (f *fixture) wantShown(texts ...string) {
	f.t.Helper()
	for _, text := range texts {
		if !f.shows(text) {
			f.t.Fatalf("screen does not show %q; texts %q", text, f.T.Texts())
		}
	}
}

func (f *fixture) wantHidden(texts ...string) {
	f.t.Helper()
	for _, text := range texts {
		if f.shows(text) {
			f.t.Fatalf("screen shows %q", text)
		}
	}
}

func (f *fixture) wantPath(path string) {
	f.t.Helper()
	if got := f.router.Path(); got != path {
		f.t.Fatalf("path = %q, want %q", got, path)
	}
}

func (f *fixture) history(cid string) *history {
	return &history{f: f, cid: cid}
}

type history struct {
	f   *fixture
	cid string
}

func (s *history) count() int {
	return len(s.f.Store.Chat(s.cid).List.Msgs)
}

func (s *history) await(n int) {
	s.f.t.Helper()
	s.f.Fix.Await(s.f.t, func() bool { return s.count() == n })
	s.f.settleMotion()
}

func worker(id, name, description string) api.Worker {
	return api.Worker{ID: id, Name: name, Description: description, Status: "idle", Engine: "claude", WorkDir: "/srv/" + id}
}

func message(id, senderType, senderID, content string, at time.Time) api.ChatMessage {
	return api.ChatMessage{ID: id, Sender: api.ChatSender{Type: senderType, ID: senderID}, Kind: "message", Content: content, TS: at.UnixMilli()}
}

func reset(id string, at time.Time) api.ChatMessage {
	return api.ChatMessage{ID: id, Sender: api.ChatSender{Type: "system"}, Kind: "reset", TS: at.UnixMilli()}
}

func handoff(id, workerID, taskType, cron, content string, at time.Time) api.ChatMessage {
	m := message(id, "bee", "", content, at)
	m.Kind = "handoff"
	m.Handoff = &api.HandoffInfo{WorkerID: workerID, TaskID: "task-" + id, TaskType: taskType, CronExpr: cron,
		ConversationID: api.WorkerConversation(workerID)}
	return m
}

func seedChat(workers []api.Worker, cid string, msgs ...api.ChatMessage) func(st *fakeapi.State) {
	return func(st *fakeapi.State) {
		st.Workers = workers
		if len(msgs) > 0 {
			st.Messages = map[string][]api.ChatMessage{cid: msgs}
		}
	}
}

func team() []api.Worker {
	return []api.Worker{worker("w1", "Alice", "Writes the release notes"), worker("w2", "Bob", "Reviews pull requests")}
}

func replies(n int, start time.Time) []api.ChatMessage {
	msgs := make([]api.ChatMessage, n)
	for i := range msgs {
		msgs[i] = message(fmt.Sprintf("r%03d", i), "worker", "w1", fmt.Sprintf("reply %02d", i), start.Add(time.Duration(i)*time.Minute))
	}
	return msgs
}

func activity(state string) func(st *fakeapi.State) {
	return func(st *fakeapi.State) {
		st.Conversations = append(st.Conversations, api.ConversationSummary{ID: workerCID, Kind: "worker", WorkerID: "w1",
			Activity: api.ConversationActivity{State: state, QueuedAhead: 2}})
	}
}

func both(seeds ...func(st *fakeapi.State)) func(st *fakeapi.State) {
	return func(st *fakeapi.State) {
		for _, seed := range seeds {
			seed(st)
		}
	}
}

func TestSenderDisplay(t *testing.T) {
	workers := map[string]api.Worker{"w1": worker("w1", "Lin", "")}
	from := func(senderType, id string) api.ChatMessage {
		return message("m", senderType, id, "hi", time.UnixMilli(0))
	}
	cases := []struct {
		name string
		m    api.ChatMessage
		cid  string
		want chat.Sender
	}{
		{"bee handoff inside a worker chat", from("bee", ""), workerCID, chat.Sender{Kind: "bee", Name: locales.ChatBeeLabel(), ViaBee: true}},
		{"bee in the bee chat", from("bee", ""), api.BeeConversation, chat.Sender{Kind: "bee", Name: locales.ChatBeeLabel()}},
		{"deleted worker", from("worker", "gone"), api.BeeConversation, chat.Sender{Kind: "worker", Name: locales.ChatDeletedWorker(), WorkerID: "gone"}},
		{"known worker", from("worker", "w1"), api.BeeConversation, chat.Sender{Kind: "worker", Name: "Lin", WorkerID: "w1"}},
		{"worker without id falls back to the chat's worker", from("worker", ""), workerCID, chat.Sender{Kind: "worker", Name: "Lin", WorkerID: "w1"}},
		{"user", from("user", ""), workerCID, chat.Sender{Kind: "user", Name: locales.ChatYou()}},
		{"system", from("system", ""), workerCID, chat.Sender{Kind: "system", Name: locales.ChatSystem()}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := chat.SenderOf(tc.m, tc.cid, workers); got != tc.want {
				t.Fatalf("sender = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestTaskTypeLabel(t *testing.T) {
	for taskType, want := range map[string]string{
		"countdown": locales.ChatTaskTypeCountdown(),
		"scheduled": locales.ChatTaskTypeScheduled(),
		"immediate": "",
	} {
		if got := chat.TaskTypeLabel(taskType); got != want {
			t.Fatalf("label(%q) = %q, want %q", taskType, got, want)
		}
	}
}

func TestUserMessagesGroup(t *testing.T) {
	base := time.Now().Add(-2 * time.Hour).Truncate(time.Minute)
	for _, tc := range []struct {
		name    string
		gap     time.Duration
		headers int
	}{
		{"one minute apart", time.Minute, 1},
		{"six minutes apart", 6 * time.Minute, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			later := base.Add(tc.gap)
			f := newFixture(t, wideW, wideH, api.BeeConversation, seedChat(nil, api.BeeConversation,
				message("u1", "user", "", "first question", base),
				message("u2", "user", "", "second question", later)))
			f.wantShown("first question", "second question")
			now := f.T.Now()
			headers := 0
			for _, at := range []time.Time{base, later} {
				if f.shows(model.MessageTime(at.UnixMilli(), now, "en")) {
					headers++
				}
			}
			if headers != tc.headers {
				t.Fatalf("%d time headers, want %d; texts %q", headers, tc.headers, f.T.Texts())
			}
		})
	}
}

func TestBeeMessageInWorkerChatHasBadge(t *testing.T) {
	at := time.Now().Add(-time.Hour)
	f := newFixture(t, wideW, wideH, "w1", seedChat(team(), workerCID,
		message("b1", "bee", "", "Please draft the release notes", at)))
	f.wantShown("Please draft the release notes", locales.ChatBeeLabel(), locales.ChatFromBee())

	g := newFixture(t, wideW, wideH, api.BeeConversation, seedChat(team(), api.BeeConversation,
		message("b1", "bee", "", "Morning summary", at)))
	g.wantShown("Morning summary", locales.ChatBeeLabel())
	g.wantHidden(locales.ChatFromBee())
}

func TestDeletedWorkerSender(t *testing.T) {
	f := newFixture(t, wideW, wideH, api.BeeConversation, seedChat(team(), api.BeeConversation,
		message("x1", "worker", "gone", "Last words of a removed worker", time.Now().Add(-time.Hour))))
	f.wantShown("Last words of a removed worker", locales.ChatDeletedWorker(), "?")
}

func TestSystemMessage(t *testing.T) {
	f := newFixture(t, wideW, wideH, "w1", seedChat(team(), workerCID,
		message("s1", "system", "", "Engine switched to codex", time.Now().Add(-time.Hour))))
	f.wantShown("Engine switched to codex", locales.ChatSystem())
	f.wantHidden(locales.ChatFromBee())
}

func TestHandoffCardOpensWorker(t *testing.T) {
	f := newFixture(t, wideW, wideH, api.BeeConversation, seedChat(team(), api.BeeConversation,
		handoff("h1", "w1", "scheduled", "0 9 * * 1", "  Write the notes \n\n for 1.4\nthird\nfourth", time.Now().Add(-time.Hour))))
	title := locales.ChatHandedOffTo("Alice")
	f.wantShown(title, "Write the notes\nfor 1.4\nthird", locales.ChatTaskTypeScheduled(), "0 9 * * 1")
	f.click(title)
	f.wantPath("/chat/w1")
}

func TestHandoffCardDisabledForDeletedWorker(t *testing.T) {
	f := newFixture(t, wideW, wideH, api.BeeConversation, seedChat(team(), api.BeeConversation,
		handoff("h1", "gone", "countdown", "", "Archive the old builds", time.Now().Add(-time.Hour))))
	title := locales.ChatHandedOffTo(locales.ChatDeletedWorker())
	f.wantShown(title, locales.ChatTaskTypeCountdown())
	f.click(title)
	f.wantPath(route.Chat(api.BeeConversation))
}

func TestResetDivider(t *testing.T) {
	base := time.Now().Add(-3 * time.Hour).Truncate(time.Minute)
	cleared := base.Add(20 * time.Minute)
	f := newFixture(t, wideW, wideH, "w1", seedChat(team(), workerCID,
		message("u1", "user", "", "old topic", base),
		reset("z1", cleared),
		message("u2", "user", "", "new topic", cleared.Add(20*time.Minute))))
	f.wantShown("old topic", "new topic", model.MessageTime(cleared.UnixMilli(), f.T.Now(), "en"))
	dividers := 0
	for _, text := range f.T.Texts() {
		if text == locales.ChatNewConversation() {
			dividers++
		}
	}
	if dividers < 2 {
		t.Fatalf("no divider label beside the header button; texts %q", f.T.Texts())
	}
}

func TestWorkerIntroWhenEmpty(t *testing.T) {
	f := newFixture(t, wideW, wideH, "w1", seedChat(team(), workerCID))
	f.wantShown("Writes the release notes", locales.ChatEmptyWorker("Alice"))
	f.wantHidden(locales.ChatNoMessagesTitle())
}

func TestBeeEmptyState(t *testing.T) {
	f := newFixture(t, wideW, wideH, api.BeeConversation, seedChat(team(), api.BeeConversation))
	f.wantShown(locales.ChatNoMessagesTitle(), locales.ChatNoMessagesDescription())
}

func TestLoadErrorRetry(t *testing.T) {
	f := openFixture(t, wideW, wideH, "w1", seedChat(team(), workerCID,
		message("w", "worker", "w1", "Back online", time.Now().Add(-time.Hour))), func(fake *fakeapi.Server) {
		fake.FailNext(http.MethodGet, "/api/local/conversations/"+workerCID+"/messages", http.StatusInternalServerError, map[string]string{"error": "boom"})
	})
	f.wantShown(locales.CommonLoadError())
	f.click(locales.CommonRetry())
	f.wantShown("Back online")
}

func TestLoadEarlierButton(t *testing.T) {
	start := time.Now().Add(-48 * time.Hour)
	f := newFixture(t, wideW, wideH, "w1", seedChat(team(), workerCID, replies(pageMessages, start)...))
	state := f.history(workerCID)
	if n := state.count(); n != 50 || !f.Store.Chat(workerCID).List.HasMore {
		t.Fatalf("first page has %d messages, has_more %v", n, f.Store.Chat(workerCID).List.HasMore)
	}
	list := f.view.List(workerCID)
	list.ScrollTo(0, ui.Start)
	f.settleMotion()
	f.wantShown(locales.ChatLoadMore())
	anchor := "reply 30"
	before, ok := f.T.Find(anchor)
	if !ok {
		t.Fatalf("first loaded message is not visible; texts %q", f.T.Texts())
	}
	f.click(locales.ChatLoadMore())
	if n := state.count(); n != pageMessages {
		t.Fatalf("%d messages after loading earlier, want %d", n, pageMessages)
	}
	if got := f.Store.Chat(workerCID).List.Msgs[0].Content; got != "reply 00" {
		t.Fatalf("first message = %q, want the oldest of the older page", got)
	}
	after, ok := f.T.Find(anchor)
	if !ok || after.Y < before.Y-1 || after.Y > before.Y+1 {
		t.Fatalf("%q moved from y=%v to y=%v (shown %v)", anchor, before.Y, after.Y, ok)
	}
	if first, last := list.Visible(); first > 30 || last < 30 {
		t.Fatalf("rows %d..%d are visible, want the first loaded message (row 30) among them", first, last)
	}
	f.wantHidden(locales.ChatLoadMore())
}

func TestNewMessageFollowsWhenAtBottom(t *testing.T) {
	f := newFixture(t, wideW, wideH, "w1", seedChat(team(), workerCID, replies(followHistory, time.Now().Add(-2*time.Hour))...))
	list := f.view.List(workerCID)
	if !list.AtEnd() {
		t.Fatal("the log does not open at its end")
	}
	f.Fake.Reply(workerCID, api.ChatMessage{Content: "fresh reply"})
	f.history(workerCID).await(followHistory + 1)
	if !list.AtEnd() {
		t.Fatal("the log stopped following its end after a new message")
	}
	if r, ok := f.T.Find("fresh reply"); !ok || r.Y+r.H > wideH {
		t.Fatalf("new message is not in view: %v %v", r, ok)
	}
}

func TestNoFollowWhenScrolledUp(t *testing.T) {
	f := newFixture(t, wideW, wideH, "w1", seedChat(team(), workerCID, replies(followHistory, time.Now().Add(-2*time.Hour))...))
	list := f.view.List(workerCID)
	f.T.Wheel(wideW/2, wideH/2, 0, -600)
	f.settleMotion()
	if list.AtEnd() {
		t.Fatal("the log is still at its end after scrolling up")
	}
	f.Fake.Reply(workerCID, api.ChatMessage{Content: "fresh reply"})
	f.history(workerCID).await(followHistory + 1)
	if list.AtEnd() {
		t.Fatal("the log followed a new message while scrolled up")
	}
}

func TestStopButtonOnlyWhenBusy(t *testing.T) {
	f := newFixture(t, wideW, wideH, "w1", both(seedChat(team(), workerCID), activity("running")))
	f.wantShown(locales.ChatStop(), locales.ChatStatusRunning())
	f.click(locales.ChatStop())
	if calls := f.Fake.Calls(http.MethodPost, "/api/local/conversations/"+workerCID+"/stop"); len(calls) != 1 {
		t.Fatalf("%d stop calls, want 1", len(calls))
	}

	idle := newFixture(t, wideW, wideH, "w1", both(seedChat(team(), workerCID), activity("idle")))
	idle.wantHidden(locales.ChatStop())
	idle.wantShown(locales.StatusesIdle())

	bee := newFixture(t, wideW, wideH, api.BeeConversation, seedChat(team(), api.BeeConversation))
	bee.Store.Chat(api.BeeConversation).BeeBusy = true
	bee.settleMotion()
	bee.wantShown(locales.ChatStatusRunning())
	bee.wantHidden(locales.ChatStop())
}

func TestStopFailureToast(t *testing.T) {
	f := newFixture(t, wideW, wideH, "w1", both(seedChat(team(), workerCID), activity("running")))
	f.Fake.FailNext(http.MethodPost, "/api/local/conversations/"+workerCID+"/stop", http.StatusInternalServerError, map[string]string{"error": "engine stuck"})
	f.click(locales.ChatStop())
	f.wantShown(locales.ChatStopFailed())
}

func TestQueuedStatus(t *testing.T) {
	f := newFixture(t, wideW, wideH, "w1", both(seedChat(team(), workerCID), activity("queued")))
	f.wantShown(locales.ChatStatusQueued(2), locales.ChatStop())
}

func TestResetDialogActiveTasks(t *testing.T) {
	f := newFixture(t, wideW, wideH, "w1", both(seedChat(team(), workerCID,
		message("u1", "user", "", "Please ship it", time.Now().Add(-time.Hour))), func(st *fakeapi.State) {
		st.ActiveTasks = map[string][]api.ActiveTask{workerCID: {
			{ID: "t1", Instruction: "Ship the 1.4 release notes", Type: "countdown"},
			{ID: "t2", Instruction: "Post the nightly report", Type: "immediate"},
		}}
	}))
	f.click(locales.ChatNewConversation())
	f.wantShown(locales.ChatNewConversationConfirm(), locales.CommonCancel())
	f.clickRightOf(locales.CommonCancel())
	f.wantShown(locales.ChatNewConversationActive(), "Ship the 1.4 release notes", "Post the nightly report",
		locales.ChatTaskTypeCountdown(), locales.ChatStopAndRestart())
	f.click(locales.ChatStopAndRestart())
	calls := f.Fake.Calls(http.MethodPost, "/api/local/conversations/"+workerCID+"/clear")
	if len(calls) != 2 {
		t.Fatalf("%d clear calls, want 2", len(calls))
	}
	if body := strings.TrimSpace(string(calls[0].Body)); body != "" && body != "null" {
		t.Fatalf("first clear body = %q, want none", body)
	}
	var second struct {
		Force bool `json:"force"`
	}
	if err := json.Unmarshal(calls[1].Body, &second); err != nil || !second.Force {
		t.Fatalf("second clear body = %q, want force:true", calls[1].Body)
	}
	f.history(workerCID).await(2)
	f.wantHidden(locales.ChatNewConversationActive(), locales.ChatStopAndRestart())
}

func TestResetDialogShowsFailure(t *testing.T) {
	f := newFixture(t, wideW, wideH, api.BeeConversation, seedChat(team(), api.BeeConversation,
		message("u1", "user", "", "hello", time.Now().Add(-time.Hour))))
	f.Fake.FailNext(http.MethodPost, "/api/local/conversations/bee/clear", http.StatusInternalServerError, map[string]string{"error": "disk full"})
	f.click(locales.ChatNewConversation())
	f.wantShown(locales.ChatNewConversationConfirmBee())
	f.clickRightOf(locales.CommonCancel())
	f.wantShown(locales.ChatNewConversationFailed(), "disk full")
}

func TestCopyMessage(t *testing.T) {
	f := newFixture(t, wideW, wideH, "w1", seedChat(team(), workerCID,
		message("w", "worker", "w1", "Copy me, please", time.Now().Add(-time.Hour))))
	f.click(locales.ChatCopyMessage())
	if got := f.T.Clipboard(); got != "Copy me, please" {
		t.Fatalf("clipboard = %q", got)
	}
}

func TestLongMessageCollapses(t *testing.T) {
	lines := make([]string, longLines)
	for i := range lines {
		lines[i] = fmt.Sprintf("line %d of the build log", i)
	}
	f := newFixture(t, wideW, wideH, "w1", seedChat(team(), workerCID,
		message("w", "worker", "w1", strings.Join(lines, "\n"), time.Now().Add(-time.Hour))))
	f.wantShown(locales.CommonShowMore())
	f.click(locales.CommonShowMore())
	f.wantShown(locales.CommonShowLess())
	f.wantHidden(locales.CommonShowMore())
}

func TestUnknownWorkerRedirectsToBee(t *testing.T) {
	f := newFixture(t, wideW, wideH, "nobody", seedChat(team(), api.BeeConversation))
	f.wantPath(route.Chat(api.BeeConversation))
	if calls := f.Fake.Calls(http.MethodGet, "/api/workers"); len(calls) < 2 {
		t.Fatalf("%d worker fetches, want a refetch before giving up", len(calls))
	}
}

func TestUnknownWorkerStaysWhenRefetchFails(t *testing.T) {
	f := openFixture(t, narrowW, wideH, "nobody", seedChat(team(), api.BeeConversation), func(fake *fakeapi.Server) {
		fake.FailNext(http.MethodGet, "/api/workers", http.StatusInternalServerError, map[string]string{"error": "down"})
	})
	f.wantPath(route.Chat("nobody"))
	f.wantShown(locales.ChatBack())
	f.wantHidden(locales.ChatNewConversation())
}

func TestMarkReadOnMountAndForeground(t *testing.T) {
	f := newFixture(t, wideW, wideH, "w1", seedChat(team(), workerCID,
		message("w", "worker", "w1", "unread reply", time.Now().Add(-time.Hour))))
	reads := func() int {
		return len(f.Fake.Calls(http.MethodPost, "/api/local/conversations/"+workerCID+"/read"))
	}
	if n := reads(); n != 1 {
		t.Fatalf("%d read marks on mount, want 1", n)
	}
	f.Store.SetForeground(false)
	f.Settle()
	f.Store.SetForeground(true)
	f.Settle()
	if n := reads(); n != 2 {
		t.Fatalf("%d read marks after returning to the foreground, want 2", n)
	}
}

func TestTitleTogglesPanel(t *testing.T) {
	f := newFixture(t, narrowW, wideH, "w1", seedChat(team(), workerCID))
	f.click("Alice")
	if !slices.Equal(f.toggled, []string{workerCID}) {
		t.Fatalf("toggled %q, want [worker:w1]", f.toggled)
	}
}

func TestNarrowBackButton(t *testing.T) {
	f := newFixture(t, narrowW, wideH, "w1", seedChat(team(), workerCID))
	f.click(locales.ChatBack())
	f.wantPath(route.ChatPath)

	wide := newFixture(t, wideW, wideH, "w1", seedChat(team(), workerCID))
	wide.wantHidden(locales.ChatBack())
}

func TestLongHistoryStaysFast(t *testing.T) {
	f := newFixture(t, wideW, wideH, "w1", seedChat(team(), workerCID))
	msgs := make([]model.Message, historySize)
	start := time.Now().Add(-30 * 24 * time.Hour)
	for i := range msgs {
		senderType, id := "worker", "w1"
		if i%3 == 0 {
			senderType, id = "user", ""
		}
		text := fmt.Sprintf("message %d: %s", i, strings.Repeat("status update ", 1+i%9))
		msgs[i] = model.Message{ChatMessage: message(fmt.Sprintf("m%04d", i), senderType, id, text, start.Add(time.Duration(i)*time.Minute))}
	}
	ch := f.Store.Chat(workerCID)
	ch.List = model.List{Msgs: msgs}
	ch.Loaded = true
	f.settleMotion()
	if !f.shows(msgs[historySize-1].Content) {
		t.Fatalf("the newest message is not shown; texts %q", f.T.Texts())
	}
	times := make([]time.Duration, scrollFrames)
	for i := range times {
		began := time.Now()
		f.T.Scroll(scrollX, wideH/2, 0, -240)
		f.T.Frame()
		times[i] = time.Since(began)
	}
	slices.Sort(times)
	median := times[len(times)/2]
	t.Logf("scroll frames: median %v, fastest %v, slowest %v", median, times[0], times[len(times)-1])
	if median >= frameBudget {
		t.Fatalf("median scroll frame %v, want under %v", median, frameBudget)
	}
	if list := f.view.List(workerCID); list.AtEnd() {
		t.Fatal("scrolling did not move the log")
	}
}
