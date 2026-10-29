package shell_test

import (
	"fmt"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/api"
	"github.com/theopenbee/openbee/desktop/internal/fakeapi"
	"github.com/theopenbee/openbee/desktop/internal/model"
	"github.com/theopenbee/openbee/desktop/internal/theme"
	"github.com/theopenbee/openbee/desktop/internal/views/route"
	"github.com/theopenbee/openbee/desktop/internal/views/shell"
	"github.com/theopenbee/openbee/desktop/internal/views/testkit"
	"github.com/theopenbee/openbee/desktop/locales"
)

const (
	wideW, wideH = 1440, 900
	narrowW      = 700
	drawerW      = 1000
	listX        = 120
	listScroll   = 3000
)

type paneCall struct {
	cid   string
	panel route.Panel
}

type spyPane struct {
	calls int
	last  paneCall
}

func (p *spyPane) View(_ *ui.Context, cid string, panel route.Panel) {
	p.calls++
	p.last = paneCall{cid: cid, panel: panel}
}

type noAccount struct{}

func (noAccount) View(*ui.Context) {}

type fixture struct {
	*testkit.Harness
	t        *testing.T
	router   *ui.Router
	shell    *shell.Shell
	chat     *spyPane
	panel    *spyPane
	settings []string
}

func newFixture(t *testing.T, w, h int, path string, seed func(st *fakeapi.State)) *fixture {
	t.Helper()
	hs := testkit.SignedIn(t, w, h, seed)
	f := &fixture{Harness: hs, t: t, router: ui.NewRouter(path), chat: &spyPane{}, panel: &spyPane{}}
	f.router.Transition = ui.TransitionNone
	f.shell = shell.New(hs.Store, f.router, f.chat, f.panel, noAccount{}, func(section string) {
		f.settings = append(f.settings, section)
	})
	hs.T = testkit.NewTester(f.view, w, h)
	hs.Settle()
	return f
}

func (f *fixture) view(c *ui.Context) {
	tk := theme.Apply(c)
	ui.Column(c).Fill().Background(tk.Background).TextColor(tk.Foreground).Children(func() {
		f.router.View(c, func(rt *ui.Route) {
			switch {
			case rt.Match(route.ChatPath):
				f.shell.View(c, "")
			case rt.Match(route.ChatPath + "/{conversation}"):
				f.shell.View(c, rt.Param("conversation"))
			}
		}).Grow(1)
	})
}

func (f *fixture) click(name string) {
	f.t.Helper()
	if err := f.T.Click(name); err != nil {
		f.t.Fatalf("click %q: %v (texts %q)", name, err, f.T.Texts())
	}
	f.Settle()
}

func (f *fixture) search(text string) {
	f.t.Helper()
	if err := f.T.Click(locales.ChatSearch()); err != nil {
		f.t.Fatalf("focus the search field: %v", err)
	}
	f.T.Command("selectAll")
	f.T.Type(text)
	f.Settle()
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

func (f *fixture) wantOrder(names ...string) {
	f.t.Helper()
	top := float32(-1)
	for _, name := range names {
		r, ok := f.T.Find(name)
		if !ok {
			f.t.Fatalf("no row %q; texts %q", name, f.T.Texts())
		}
		if r.Y <= top {
			f.t.Fatalf("row %q at y=%v is not below the previous row (y=%v); want order %q", name, r.Y, top, names)
		}
		top = r.Y
	}
}

func (f *fixture) wantPath(path string) {
	f.t.Helper()
	if got := f.router.Path(); got != path {
		f.t.Fatalf("path = %q, want %q", got, path)
	}
}

func worker(id, name, description string) api.Worker {
	return api.Worker{ID: id, Name: name, Description: description, Status: "idle", Engine: "claude", WorkDir: "/srv/" + id}
}

func conversation(workerID string, last *api.LastMessage, unread int) api.ConversationSummary {
	sum := api.ConversationSummary{ID: api.BeeConversation, Kind: "bee", LastMessage: last, Unread: unread,
		Activity: api.ConversationActivity{State: "idle"}}
	if workerID != "" {
		sum.ID, sum.Kind, sum.WorkerID = api.WorkerConversation(workerID), "worker", workerID
	}
	return sum
}

func said(sender, preview string, at time.Time) *api.LastMessage {
	return &api.LastMessage{Sender: api.ChatSender{Type: sender}, Kind: "message", Preview: preview, TS: at.UnixMilli()}
}

func handedOff(to string, at time.Time) *api.LastMessage {
	return &api.LastMessage{Sender: api.ChatSender{Type: "bee"}, Kind: "handoff", HandoffWorkerID: to, TS: at.UnixMilli()}
}

func seed(workers []api.Worker, conversations ...api.ConversationSummary) func(st *fakeapi.State) {
	return func(st *fakeapi.State) {
		st.Workers = workers
		st.Conversations = append([]api.ConversationSummary{conversation("", nil, 0)}, conversations...)
	}
}

func team() []api.Worker {
	return []api.Worker{
		worker("w1", "Alice", "Writes the release notes"),
		worker("w2", "Bob", "Reviews pull requests"),
		worker("w3", "Carol", "Triage"),
	}
}

func TestListOrdersBeeFirstThenRecent(t *testing.T) {
	now := time.Now()
	workers := append(team(), worker("w4", "Abe", ""))
	f := newFixture(t, wideW, wideH, route.Chat("bee"), seed(workers,
		conversation("w1", said("worker", "old news", now.Add(-2*time.Hour)), 0),
		conversation("w2", said("worker", "fresh news", now.Add(-time.Minute)), 0),
	))
	f.wantOrder(locales.ChatBeeLabel(), "Bob", "Alice", "Abe", "Carol")
}

func TestListHidesDeletedWorkers(t *testing.T) {
	f := newFixture(t, wideW, wideH, route.Chat("bee"), seed(team(),
		conversation("gone", said("worker", "ghost reply", time.Now()), 4),
	))
	f.wantShown("Alice", "Bob", "Carol")
	if f.T.HasText("ghost reply") {
		t.Fatal("the list shows a conversation of a deleted worker")
	}
}

func TestSearchFiltersByNameAndDescription(t *testing.T) {
	f := newFixture(t, wideW, wideH, route.Chat("bee"), seed(team()))
	f.search("pull")
	f.wantShown(locales.ChatBeeLabel(), "Bob")
	f.wantHidden("Alice", "Carol")
	f.search("ALI")
	f.wantShown(locales.ChatBeeLabel(), "Alice")
	f.wantHidden("Bob", "Carol")
	f.click(locales.ArcSearchFieldClear())
	f.wantShown("Alice", "Bob", "Carol")
}

func TestRowShowsUnreadBadgeCapped(t *testing.T) {
	now := time.Now()
	f := newFixture(t, wideW, wideH, route.Chat("bee"), seed(team(),
		conversation("w1", said("worker", "a lot", now), 150),
		conversation("w2", said("worker", "a few", now), 7),
		conversation("w3", said("worker", "none", now), 0),
	))
	f.wantShown("99+", "7")
	f.wantHidden("150", "0")
}

func TestRowPreviewYouPrefix(t *testing.T) {
	now := time.Now()
	f := newFixture(t, wideW, wideH, route.Chat("bee"), seed(team(),
		conversation("w1", said("user", "Please rerun the report", now), 0),
		conversation("w2", said("worker", "Done", now), 0),
	))
	f.wantShown(locales.ChatYouPrefix()+"Please rerun the report", "Done")
	f.wantHidden(locales.ChatYouPrefix() + "Done")
}

func TestRowPreviewHandoffAndDescription(t *testing.T) {
	now := time.Now()
	f := newFixture(t, wideW, wideH, route.Chat("bee"), func(st *fakeapi.State) {
		seed(team(), conversation("w2", handedOff("gone", now), 0))(st)
		st.Conversations[0].LastMessage = handedOff("w1", now)
	})
	f.wantShown(locales.ChatHandedOffTo("Alice"), locales.ChatHandedOffTo(locales.ChatDeletedWorker()), "Triage")
}

func TestRowShowsListTime(t *testing.T) {
	now := time.Now()
	earlier := now.AddDate(0, 0, -3)
	f := newFixture(t, wideW, wideH, route.Chat("bee"), seed(team(),
		conversation("w1", said("worker", "yesterday", now.AddDate(0, 0, -1)), 0),
		conversation("w2", said("worker", "earlier", earlier), 0),
	))
	want, _ := model.ListTime(earlier.UnixMilli(), f.T.Now(), "en")
	f.wantShown(locales.ChatYesterday(), want)
}

func TestClickRowOpensChat(t *testing.T) {
	f := newFixture(t, wideW, wideH, route.Chat("bee"), seed(team()))
	if f.Store.Active() != api.BeeConversation {
		t.Fatalf("active = %q, want bee", f.Store.Active())
	}
	f.click("Alice")
	f.wantPath("/chat/w1")
	if got := f.Store.Active(); got != "worker:w1" {
		t.Fatalf("active = %q, want worker:w1", got)
	}
	if f.chat.last.cid != "worker:w1" {
		t.Fatalf("chat pane shows %q, want worker:w1", f.chat.last.cid)
	}
}

func TestNarrowShowsListOnly(t *testing.T) {
	f := newFixture(t, narrowW, wideH, route.ChatPath, seed(team()))
	f.wantShown(locales.ChatSearch(), locales.ChatBeeLabel(), "Alice")
	if f.chat.calls != 0 {
		t.Fatalf("chat pane built %d times on the narrow list", f.chat.calls)
	}
	if got := f.Store.Active(); got != "" {
		t.Fatalf("active = %q with no conversation open, want none", got)
	}
	f.click("Alice")
	f.wantPath("/chat/w1")
	if f.chat.last.cid != "worker:w1" {
		t.Fatalf("chat pane shows %q, want worker:w1", f.chat.last.cid)
	}
	f.wantHidden(locales.ChatSearch(), "Bob")
}

func TestNarrowBackReturnsToList(t *testing.T) {
	h := testkit.SignedIn(t, narrowW, wideH, seed(team()))
	h.Root.OpenConversation("worker:w1")
	h.Settle()
	if err := h.T.Click(locales.ChatBack()); err != nil {
		t.Fatalf("click back: %v (texts %q)", err, h.T.Texts())
	}
	h.Settle()
	if got := h.Root.Nav().Path(); got != route.ChatPath {
		t.Fatalf("path = %q, want %s", got, route.ChatPath)
	}
	if _, ok := h.T.Find("Alice"); !ok {
		t.Fatal("the conversation list is not shown after going back")
	}
}

func TestWideRedirectsToBee(t *testing.T) {
	h := testkit.SignedIn(t, wideW, wideH, seed(team()))
	if got := h.Root.Nav().Path(); got != route.Chat(api.BeeConversation) {
		t.Fatalf("path = %q, want /chat/bee", got)
	}
	if got := h.Store.Active(); got != api.BeeConversation {
		t.Fatalf("active = %q, want bee", got)
	}
	for _, text := range []string{locales.ChatSearch(), "Alice"} {
		if _, ok := h.T.Find(text); !ok {
			t.Fatalf("wide chat does not show %q beside the chat; texts %q", text, h.T.Texts())
		}
	}
}

func TestListErrorRetry(t *testing.T) {
	f := newFixture(t, wideW, wideH, route.Chat("bee"), seed(team()))
	f.Fake.FailNext(http.MethodGet, "/api/workers", http.StatusInternalServerError, map[string]string{"error": "boom"})
	f.Store.Workers.Reset()
	f.Settle()
	f.wantShown(locales.CommonLoadError())
	f.wantHidden("Alice")
	f.click(locales.CommonRetry())
	f.wantShown("Alice")
	f.wantHidden(locales.CommonLoadError())
}

func TestNoWorkersHint(t *testing.T) {
	f := newFixture(t, wideW, wideH, route.Chat("bee"), nil)
	f.wantShown(locales.ChatBeeLabel())
	hint, ok := f.T.Find(locales.ChatNoWorkersHint())
	if !ok {
		t.Fatalf("no workers hint missing; texts %q", f.T.Texts())
	}
	f.T.ClickAt(hint.X+24, hint.Y+hint.H+12+theme.ControlSM/2)
	f.Settle()
	if got := f.router.Query(route.PanelQuery); got != route.PanelNew {
		t.Fatalf("panel = %q after the hint button, want %s", got, route.PanelNew)
	}
}

func TestNoWorkersHintHiddenWithWorkers(t *testing.T) {
	f := newFixture(t, wideW, wideH, route.Chat("bee"), seed(team()))
	f.wantHidden(locales.ChatNoWorkersHint())
}

func TestGearOpensSettingsEnv(t *testing.T) {
	f := newFixture(t, wideW, wideH, route.Chat("bee"), seed(team()))
	f.click(locales.NavSettings())
	if !slices.Equal(f.settings, []string{"env"}) {
		t.Fatalf("openSettings calls = %q, want [env]", f.settings)
	}
}

func TestPlusOpensNewWorkerPanel(t *testing.T) {
	f := newFixture(t, wideW, wideH, route.Chat("bee"), seed(team()))
	f.click(locales.ChatNewWorker())
	f.wantPath("/chat/bee")
	if got := f.router.Query(route.PanelQuery); got != route.PanelNew {
		t.Fatalf("panel = %q, want %s", got, route.PanelNew)
	}
	want := paneCall{cid: api.BeeConversation, panel: route.Panel{Mode: route.ModeNew}}
	if f.panel.last != want {
		t.Fatalf("panel pane = %+v, want %+v", f.panel.last, want)
	}
	f.T.Advance(time.Second)
	f.click(locales.ChatPanelClose())
	if got := f.router.Query(route.PanelQuery); got != "" {
		t.Fatalf("panel = %q after closing, want none", got)
	}
	f.wantPath("/chat/bee")
}

func TestSearchAndPlusShareRowBelowWindowControls(t *testing.T) {
	const controlsHeight, controlsLeft = 28, 70
	f := newFixture(t, wideW, wideH, route.Chat("bee"), seed(team()))
	f.T.SetTitleBar(ui.TitleBar{Height: controlsHeight, Left: controlsLeft})
	f.Settle()
	search, ok := f.T.Find(locales.ChatSearch())
	if !ok {
		t.Fatalf("no search field; texts %q", f.T.Texts())
	}
	plus, ok := f.T.Find(locales.ChatNewWorker())
	if !ok {
		t.Fatalf("no new worker button; texts %q", f.T.Texts())
	}
	if search.Y < controlsHeight {
		t.Fatalf("search field at y=%v, want it below the %v px of window controls", search.Y, controlsHeight)
	}
	if got, want := plus.Y+plus.H/2, search.Y+search.H/2; got != want {
		t.Fatalf("new worker button centered at y=%v, want the search field's %v", got, want)
	}
	if plus.X < search.X+search.W {
		t.Fatalf("new worker button at x=%v, want it right of the search field ending at %v", plus.X, search.X+search.W)
	}
}

func TestPanelDrawerBelowInlineBreakpoint(t *testing.T) {
	f := newFixture(t, drawerW, wideH, route.ChatPanel("w1", route.PanelConstraints), seed(team()))
	want := paneCall{cid: "worker:w1", panel: route.Panel{Mode: route.ModeWorker, Tab: route.PanelConstraints}}
	if f.panel.last != want {
		t.Fatalf("panel pane = %+v, want %+v", f.panel.last, want)
	}
	f.wantShown(locales.ChatPanelTitle(), locales.ArcDrawerClose())
	f.wantHidden(locales.ChatPanelClose())
	f.T.Key(0, ui.KeyEscape)
	f.Settle()
	if got := f.router.Query(route.PanelQuery); got != "" {
		t.Fatalf("panel = %q after Escape, want none", got)
	}
	f.wantPath("/chat/w1")
	f.wantHidden(locales.ArcDrawerClose())
	f.router.Push(route.ChatPanel("w1", route.PanelNew))
	f.Settle()
	f.wantShown(locales.ChatNewWorker(), locales.ArcDrawerClose())
	f.T.Advance(time.Second)
	f.click(locales.ArcDrawerClose())
	if got := f.router.Query(route.PanelQuery); got != "" {
		t.Fatalf("panel = %q after the close button, want none", got)
	}
	f.wantHidden(locales.ArcDrawerClose())
}

func TestRevealsActiveRow(t *testing.T) {
	workers := make([]api.Worker, 0, 40)
	for i := range 40 {
		workers = append(workers, worker(fmt.Sprintf("w%02d", i), fmt.Sprintf("Worker %02d", i), ""))
	}
	f := newFixture(t, wideW, wideH, route.Chat("bee"), seed(workers))
	f.wantHidden("Worker 39")
	f.router.Push(route.Chat("w39"))
	f.Settle()
	r, ok := f.T.Find("Worker 39")
	if !ok {
		t.Fatalf("active row is not revealed; texts %q", f.T.Texts())
	}
	if r.Y < 0 || r.Y+r.H > wideH {
		t.Fatalf("active row at %+v is outside the window", r)
	}
}

func TestSessionResetClearsSearchAndScroll(t *testing.T) {
	workers := make([]api.Worker, 0, 40)
	for i := range 40 {
		workers = append(workers, worker(fmt.Sprintf("w%02d", i), fmt.Sprintf("Worker %02d", i), ""))
	}
	f := newFixture(t, wideW, wideH, route.Chat("bee"), seed(workers))
	f.search("Worker")
	f.T.Scroll(listX, wideH/2, 0, listScroll)
	f.Settle()
	f.wantHidden("Worker 00")

	f.Store.Logout()
	f.Settle()
	snap := f.Fake.Snapshot()
	var err error
	f.Store.Login(snap.Username, snap.Password, func(e error) { err = e })
	f.Settle()
	if err != nil {
		t.Fatalf("sign in again: %v", err)
	}
	f.wantShown(locales.ChatBeeLabel(), "Worker 00")
	if err := f.T.Click(locales.ChatSearch()); err != nil {
		t.Fatal(err)
	}
	f.T.SetClipboard("")
	f.T.Command("selectAll")
	f.T.Command("copy")
	if got := f.T.Clipboard(); got != "" {
		t.Fatalf("search after signing in again = %q, want empty", got)
	}
}
