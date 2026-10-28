package worker_test

import (
	"encoding/json"
	"maps"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/api"
	"github.com/theopenbee/openbee/desktop/internal/fakeapi"
	"github.com/theopenbee/openbee/desktop/internal/kit"
	"github.com/theopenbee/openbee/desktop/internal/theme"
	"github.com/theopenbee/openbee/desktop/internal/views/route"
	"github.com/theopenbee/openbee/desktop/internal/views/shell"
	"github.com/theopenbee/openbee/desktop/internal/views/testkit"
	"github.com/theopenbee/openbee/desktop/internal/views/worker"
	"github.com/theopenbee/openbee/desktop/locales"
)

const (
	wideW, wideH  = 1440, 1200
	drawerW       = 1000
	aliceRules    = "Reply in English.\nKeep answers short."
	alicePath     = "/api/workers/w1"
	bobPath       = "/api/workers/w2"
	randomPath    = "/api/workers/random-name"
	motionSettled = time.Second
)

type noPane struct{}

func (noPane) View(*ui.Context, string, route.Panel) {}

type noAccount struct{}

func (noAccount) View(*ui.Context) {}

type fixture struct {
	*testkit.Harness
	t       *testing.T
	router  *ui.Router
	panel   *worker.Panel
	deleted []api.Worker
}

func crew(st *fakeapi.State) {
	st.Workers = []api.Worker{
		{ID: "w1", Name: "Alice", Description: "Writes the release notes", Constraints: aliceRules, Status: "idle",
			Engine: "claude", EngineArgs: map[string]string{"claude": "--model opus"}, WorkDir: "/srv/alice", PermissionScopes: "read:tasks"},
		{ID: "w2", Name: "Bob", Status: "working", Engine: "codex", WorkDir: "/srv/bob"},
	}
	st.Conversations = []api.ConversationSummary{chatted(api.BeeConversation, "bee"), chatted("w1", "worker"), chatted("w2", "worker")}
}

func chatted(id, kind string) api.ConversationSummary {
	sum := api.ConversationSummary{ID: id, Kind: kind, Activity: api.ConversationActivity{State: "idle"},
		LastMessage: &api.LastMessage{Sender: api.ChatSender{Type: kind}, Kind: "message", Preview: "Done", TS: time.Now().UnixMilli()}}
	if kind == "worker" {
		sum.ID, sum.WorkerID = api.WorkerConversation(id), id
	}
	return sum
}

func newFixture(t *testing.T, w int, path string, seed func(st *fakeapi.State)) *fixture {
	t.Helper()
	hs := testkit.SignedIn(t, w, wideH, seed)
	f := &fixture{Harness: hs, t: t, router: ui.NewRouter(path)}
	f.router.Transition = ui.TransitionNone
	f.panel = worker.New(hs.Store, f.router)
	f.panel.SetOnDelete(func(w api.Worker) { f.deleted = append(f.deleted, w) })
	sh := shell.New(hs.Store, f.router, noPane{}, f.panel, noAccount{}, func(string) {})
	hs.T = ui.NewTester(func(c *ui.Context) {
		tk := theme.Apply(c)
		ui.Column(c).Fill().Background(tk.Background).TextColor(tk.Foreground).Children(func() {
			f.router.View(c, func(rt *ui.Route) {
				if rt.Match(route.ChatPath + "/{conversation}") {
					sh.View(c, rt.Param("conversation"))
				}
			}).Grow(1)
		})
		kit.ShowToasts(c, tk, &hs.Store.Toasts)
	}, w, wideH)
	f.settle()
	return f
}

func (f *fixture) settle() {
	f.Settle()
	f.T.Advance(motionSettled)
	f.Settle()
}

func (f *fixture) click(name string) {
	f.t.Helper()
	if err := f.T.Click(name); err != nil {
		f.t.Fatalf("click %q: %v (texts %q)", name, err, f.T.Texts())
	}
	f.settle()
}

func (f *fixture) replaceText(field, text string) {
	f.t.Helper()
	f.click(field)
	f.T.Command("selectAll")
	f.T.Type(text)
	f.settle()
}

func (f *fixture) fieldText(field string) string {
	f.t.Helper()
	if !f.T.Focused(field) {
		f.click(field)
	}
	f.T.Command("selectAll")
	f.T.Command("copy")
	return f.T.Clipboard()
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

func (f *fixture) wantPanel(tab string) {
	f.t.Helper()
	if got := f.router.Query(route.PanelQuery); got != tab {
		f.t.Fatalf("panel = %q, want %q", got, tab)
	}
}

func (f *fixture) puts(path string) []map[string]any {
	f.t.Helper()
	var out []map[string]any
	for _, call := range f.Fake.Calls(http.MethodPut, path) {
		if call.Path != path {
			continue
		}
		var body map[string]any
		if err := json.Unmarshal(call.Body, &body); err != nil {
			f.t.Fatalf("decode PUT %s body %q: %v", path, call.Body, err)
		}
		out = append(out, body)
	}
	return out
}

func (f *fixture) lastPut(path string) map[string]any {
	f.t.Helper()
	bodies := f.puts(path)
	if len(bodies) == 0 {
		f.t.Fatalf("no PUT %s", path)
	}
	return bodies[len(bodies)-1]
}

func TestPanelShowsProfile(t *testing.T) {
	f := newFixture(t, wideW, route.ChatPanel("w1", route.PanelConstraints), crew)
	f.wantShown(locales.ChatPanelTitle(), "Writes the release notes", locales.WorkersEnginesClaude(),
		locales.WorkerDetailEditInfo(), locales.WorkersRowActions("Alice"),
		locales.ChatPanelTabsConstraints(), locales.ChatPanelTabsPermissions(), locales.ChatPanelTabsEnv(),
		locales.WorkerDetailConstraintsPanelCharCount(len(aliceRules)), aliceRules, locales.WorkerDetailEditConstraints())
	f.wantHidden(locales.CommonNoDescription(), locales.WorkerDetailEditInfoTitle())

	f.router.Push(route.ChatPanel("w2", route.PanelConstraints))
	f.settle()
	f.wantShown(locales.CommonNoDescription(), locales.WorkersEnginesCodex(),
		locales.WorkerDetailConstraintsPanelEmptyTitle("Bob"), locales.WorkerDetailConstraintsPanelTemplatesToneTitle())
	f.wantHidden("Writes the release notes", locales.WorkersEnginesClaude())
}

func TestPanelLoadErrorRetry(t *testing.T) {
	f := newFixture(t, wideW, route.Chat("w1"), crew)
	f.Fake.FailNext(http.MethodGet, alicePath, http.StatusInternalServerError, map[string]string{"error": "detail down"})
	f.router.Push(route.ChatPanel("w1", route.PanelConstraints))
	f.settle()
	f.wantShown(locales.WorkerDetailLoadError(), "detail down")
	f.wantHidden(locales.WorkerDetailEditInfo())
	f.click(locales.CommonRetry())
	f.wantShown(locales.WorkerDetailEditInfo(), "Writes the release notes")
	f.wantHidden(locales.WorkerDetailLoadError())
}

func TestTabSwitchReplacesHistory(t *testing.T) {
	f := newFixture(t, wideW, route.ChatPanel("w1", route.PanelConstraints), crew)
	if f.router.CanGoBack() {
		t.Fatal("history has an entry before the panel")
	}
	for _, tab := range []struct{ label, query string }{
		{locales.ChatPanelTabsPermissions(), route.PanelPermissions},
		{locales.ChatPanelTabsEnv(), route.PanelEnv},
		{locales.ChatPanelTabsConstraints(), route.PanelConstraints},
	} {
		f.click(tab.label)
		f.wantPanel(tab.query)
		if got := f.router.Path(); got != "/chat/w1" {
			t.Fatalf("path = %q after the %s tab, want /chat/w1", got, tab.query)
		}
		if f.router.CanGoBack() {
			t.Fatalf("switching to the %s tab pushed a history entry", tab.query)
		}
	}
	f.click(locales.ChatPanelTabsPermissions())
	f.wantShown(locales.ScopesReadWorkersTitle(), locales.ScopesReadTasksDescription())
	f.wantHidden(aliceRules)
}

func TestEditSavesOnlyChanged(t *testing.T) {
	f := newFixture(t, wideW, route.ChatPanel("w1", route.PanelConstraints), crew)
	f.click(locales.WorkerDetailEditInfo())
	f.wantShown(locales.WorkerDetailEditInfoTitle(), locales.WorkersFormDescriptionHelper(), locales.WorkersFormWorkDirEditHelper(),
		locales.WorkersFormEngineArgsHelper(locales.WorkersEnginesClaude()))
	f.wantHidden(locales.ChatPanelTabsPermissions())
	if got := f.fieldText(locales.WorkersFormName()); got != "Alice" {
		t.Fatalf("name field = %q, want Alice", got)
	}
	f.click(locales.WorkerDetailSaveInfo())
	if n := len(f.puts(alicePath)); n != 0 {
		t.Fatalf("saving an unchanged form sent %d updates", n)
	}
	f.wantHidden(locales.WorkerDetailEditInfoTitle())

	f.click(locales.WorkerDetailEditInfo())
	f.replaceText(locales.WorkersFormDescription(), "Ships every release")
	f.click(locales.WorkerDetailSaveInfo())
	body := f.lastPut(alicePath)
	if keys := slices.Sorted(maps.Keys(body)); !slices.Equal(keys, []string{"description", "engine", "engine_args"}) {
		t.Fatalf("PUT keys = %q, want description, engine, engine_args; body %v", keys, body)
	}
	args, _ := body["engine_args"].(map[string]any)
	if body["description"] != "Ships every release" || body["engine"] != "claude" || len(args) != 1 || args["claude"] != "--model opus" {
		t.Fatalf("PUT body = %v", body)
	}
	f.wantHidden(locales.WorkerDetailEditInfoTitle())
	f.wantShown("Ships every release", locales.ChatPanelTabsPermissions())
}

func TestEditSaveErrorShowsAlert(t *testing.T) {
	f := newFixture(t, wideW, route.ChatPanel("w1", route.PanelConstraints), crew)
	f.click(locales.WorkerDetailEditInfo())
	f.replaceText(locales.WorkersFormName(), "Bob")
	f.click(locales.WorkerDetailSaveInfo())
	f.wantShown(locales.WorkersFormSaveError(), locales.WorkerDetailEditInfoTitle())
	if body := f.lastPut(alicePath); body["name"] != "Bob" {
		t.Fatalf("PUT body = %v, want the new name", body)
	}
}

func TestEditSaveDisabledWithoutWorkDir(t *testing.T) {
	f := newFixture(t, wideW, route.ChatPanel("w1", route.PanelConstraints), crew)
	f.click(locales.WorkerDetailEditInfo())
	for _, blank := range []string{"", "   "} {
		f.replaceText(locales.WorkersFormWorkDir(), blank)
		f.click(locales.WorkerDetailSaveInfo())
		f.T.Key(0, ui.KeyEnter)
		f.settle()
		if n := len(f.puts(alicePath)); n != 0 {
			t.Fatalf("work dir %q: sent %d updates, want none", blank, n)
		}
		f.wantShown(locales.WorkerDetailEditInfoTitle())
	}
	f.replaceText(locales.WorkersFormWorkDir(), "  /srv/alice-2  ")
	f.T.Key(0, ui.KeyEnter)
	f.settle()
	body := f.lastPut(alicePath)
	if body["work_dir"] != "/srv/alice-2" {
		t.Fatalf("PUT body = %v, want the trimmed work dir", body)
	}
	if _, ok := body["name"]; ok {
		t.Fatalf("PUT body = %v sends the unchanged name", body)
	}
	f.wantHidden(locales.WorkerDetailEditInfoTitle())
}

func TestRandomNameFillsAndExhausts(t *testing.T) {
	f := newFixture(t, wideW, route.ChatPanel("w1", route.PanelConstraints), func(st *fakeapi.State) {
		crew(st)
		st.RandomNames = []string{"Bob", "Nova"}
	})
	f.click(locales.WorkerDetailEditInfo())
	f.wantShown(locales.WorkersFormNameHelper())
	f.click(locales.WorkersFormRandomName())
	if got := f.fieldText(locales.WorkersFormName()); got != "Nova" {
		t.Fatalf("name field = %q after shuffle, want Nova", got)
	}
	f.Fake.Update(func(st *fakeapi.State) { st.RandomNames = []string{"Bob"} })
	f.click(locales.WorkersFormRandomName())
	f.wantShown(locales.WorkersFormRandomNameExhausted())
	f.wantHidden(locales.WorkersFormNameHelper())
	if got := f.fieldText(locales.WorkersFormName()); got != "Nova" {
		t.Fatalf("name field = %q after exhaustion, want Nova kept", got)
	}
	calls := len(f.Fake.Calls(http.MethodGet, randomPath))
	f.click(locales.WorkersFormRandomName())
	if n := len(f.Fake.Calls(http.MethodGet, randomPath)); n != calls {
		t.Fatalf("random-name calls = %d after clicking the exhausted button, want %d", n, calls)
	}

	f.click(locales.CommonCancel())
	f.click(locales.WorkerDetailEditInfo())
	f.wantShown(locales.WorkersFormNameHelper())
	f.wantHidden(locales.WorkersFormRandomNameExhausted())
	if got := f.fieldText(locales.WorkersFormName()); got != "Alice" {
		t.Fatalf("name field = %q after reopening, want Alice", got)
	}
}

func TestConstraintsTemplateOpensEditor(t *testing.T) {
	f := newFixture(t, wideW, route.ChatPanel("w2", route.PanelConstraints), crew)
	f.Store.SetLanguage("zh")
	f.settle()
	zh := locales.In("zh")
	f.wantShown(zh.WorkerDetailConstraintsPanelTemplatesLabel(), zh.WorkerDetailConstraintsPanelTemplatesToneDesc())
	f.click(zh.WorkerDetailConstraintsPanelTemplatesScopeTitle())
	want := zh.WorkerDetailConstraintsPanelTemplatesScopeBody()
	if want == locales.In("en").WorkerDetailConstraintsPanelTemplatesScopeBody() {
		t.Fatal("the scope template has no Chinese text")
	}
	if !f.T.Focused(zh.WorkerDetailEditConstraints()) {
		t.Fatal("the constraints editor does not take the focus")
	}
	if got := f.fieldText(zh.WorkerDetailEditConstraints()); got != want {
		t.Fatalf("editor = %q, want the Chinese scope template %q", got, want)
	}
	f.wantShown(zh.WorkerDetailConstraintsPanelSave())
	f.wantHidden(zh.WorkerDetailConstraintsPanelEmptyTitle("Bob"))
}

func TestConstraintsCmdEnterSaves(t *testing.T) {
	f := newFixture(t, wideW, route.ChatPanel("w2", route.PanelConstraints), crew)
	f.click(locales.WorkerDetailConstraintsPanelAdd())
	f.T.Type("Be brief.")
	f.settle()
	f.wantShown(locales.WorkerDetailConstraintsPanelCharCount(9) + " · " + locales.WorkerDetailConstraintsPanelShortcuts())
	f.T.Key(ui.Cmd, ui.KeyEnter)
	f.settle()
	body := f.lastPut(bobPath)
	if len(body) != 1 || body["constraints"] != "Be brief." {
		t.Fatalf("PUT body = %v, want only the constraints", body)
	}
	f.wantHidden(locales.WorkerDetailConstraintsPanelSave())
	f.wantShown("Be brief.", locales.WorkerDetailConstraintsPanelCharCount(9))
}

func TestConstraintsSaveErrorKeepsEditor(t *testing.T) {
	f := newFixture(t, wideW, route.ChatPanel("w2", route.PanelConstraints), crew)
	f.Fake.FailNext(http.MethodPut, bobPath, http.StatusInternalServerError, map[string]string{"error": "disk full"})
	f.click(locales.WorkerDetailConstraintsPanelAdd())
	f.T.Type("Be brief.")
	f.click(locales.WorkerDetailConstraintsPanelSave())
	f.wantShown(locales.WorkerDetailConstraintsPanelSaveError(), "disk full", locales.WorkerDetailConstraintsPanelSave())
	if got := f.fieldText(locales.WorkerDetailEditConstraints()); got != "Be brief." {
		t.Fatalf("editor = %q after a failed save, want the draft kept", got)
	}
}

func TestConstraintsEscapeCancels(t *testing.T) {
	f := newFixture(t, wideW, route.ChatPanel("w1", route.PanelConstraints), crew)
	f.click(locales.WorkerDetailEditConstraints())
	f.wantShown(locales.WorkerDetailConstraintsPanelSave())
	if got := f.fieldText(locales.WorkerDetailEditConstraints()); got != aliceRules {
		t.Fatalf("editor = %q, want the current constraints", got)
	}
	f.T.Type("Something else")
	f.T.Key(0, ui.KeyEscape)
	f.settle()
	f.wantHidden(locales.WorkerDetailConstraintsPanelSave())
	f.wantShown(aliceRules)
	if n := len(f.puts(alicePath)); n != 0 {
		t.Fatalf("Escape sent %d updates", n)
	}
}

func TestPermissionsToggleSavesImmediately(t *testing.T) {
	f := newFixture(t, wideW, route.ChatPanel("w1", route.PanelPermissions), crew)
	f.click(locales.ScopesReadWorkersTitle())
	if body := f.lastPut(alicePath); len(body) != 1 || body["permission_scopes"] != "read:tasks,read:workers" {
		t.Fatalf("PUT body = %v, want read:tasks,read:workers", body)
	}
	f.click(locales.ScopesReadTasksTitle())
	if body := f.lastPut(alicePath); body["permission_scopes"] != "read:workers" {
		t.Fatalf("PUT body = %v, want read:workers", body)
	}
	if got := f.Fake.Snapshot().Workers[0].PermissionScopes; got != "read:workers" {
		t.Fatalf("server scopes = %q, want read:workers", got)
	}
}

func TestPermissionsRevertOnError(t *testing.T) {
	f := newFixture(t, wideW, route.ChatPanel("w1", route.PanelPermissions), crew)
	f.Fake.FailNext(http.MethodPut, alicePath, http.StatusInternalServerError, map[string]string{"error": "scopes locked"})
	f.click(locales.ScopesReadMessagesTitle())
	f.wantShown(locales.WorkerDetailPermissionsSaveError(), "scopes locked")
	f.click(locales.ScopesReadMessagesTitle())
	if body := f.lastPut(alicePath); body["permission_scopes"] != "read:tasks,read:messages" {
		t.Fatalf("PUT body = %v after the failed toggle, want the scope added again", body)
	}
	f.wantHidden(locales.WorkerDetailPermissionsSaveError())
	if n := len(f.puts(alicePath)); n != 2 {
		t.Fatalf("updates = %d, want 2", n)
	}
}

func TestPanelCloseButton(t *testing.T) {
	f := newFixture(t, wideW, route.ChatPanel("w1", route.PanelConstraints), crew)
	f.click(locales.WorkerDetailEditInfo())
	f.click(locales.ChatPanelClose())
	f.wantPanel("")
	if got := f.router.Path(); got != "/chat/w1" {
		t.Fatalf("path = %q, want /chat/w1", got)
	}
	if !f.router.CanGoBack() {
		t.Fatal("closing the panel did not push a history entry")
	}
	f.wantHidden(locales.ChatPanelTitle(), locales.WorkerDetailEditInfoTitle())
	f.router.Back()
	f.settle()
	f.wantPanel(route.PanelConstraints)
	f.wantShown(locales.WorkerDetailEditInfo(), aliceRules)
	f.wantHidden(locales.WorkerDetailEditInfoTitle())
}

func TestMoreMenuDeleteCallsHook(t *testing.T) {
	f := newFixture(t, wideW, route.ChatPanel("w1", route.PanelConstraints), crew)
	f.wantHidden(locales.WorkersDeleteWorker())
	f.click(locales.WorkersRowActions("Alice"))
	f.click(locales.WorkersDeleteWorker())
	if len(f.deleted) != 1 || f.deleted[0].ID != "w1" || f.deleted[0].WorkDir != "/srv/alice" {
		t.Fatalf("delete hook calls = %+v, want Alice once", f.deleted)
	}
	f.wantHidden(locales.WorkersDeleteWorker())
}

func TestDrawerShowsProfile(t *testing.T) {
	f := newFixture(t, drawerW, route.ChatPanel("w1", route.PanelConstraints), crew)
	f.wantShown(locales.ArcDrawerClose(), "Writes the release notes", locales.WorkerDetailEditInfo())
	f.wantHidden(locales.ChatPanelClose())
}
