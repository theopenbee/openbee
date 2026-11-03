package worker_test

import (
	"encoding/json"
	"maps"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/api"
	"github.com/theopenbee/openbee/desktop/internal/fakeapi"
	"github.com/theopenbee/openbee/desktop/internal/views/route"
	"github.com/theopenbee/openbee/desktop/locales"
)

const (
	workersPath = "/api/workers"
	newPanel    = "/chat/bee?panel=new"
	tallH       = 2400
)

func (f *fixture) posts() []map[string]any {
	f.t.Helper()
	var out []map[string]any
	for _, call := range f.Fake.Calls(http.MethodPost, workersPath) {
		if call.Path != workersPath {
			continue
		}
		var body map[string]any
		if err := json.Unmarshal(call.Body, &body); err != nil {
			f.t.Fatalf("decode POST %s body %q: %v", workersPath, call.Body, err)
		}
		out = append(out, body)
	}
	return out
}

func (f *fixture) lastPost() map[string]any {
	f.t.Helper()
	bodies := f.posts()
	if len(bodies) == 0 {
		f.t.Fatalf("no POST %s", workersPath)
	}
	return bodies[len(bodies)-1]
}

func (f *fixture) deletes(path string) []fakeapi.Call {
	f.t.Helper()
	var out []fakeapi.Call
	for _, call := range f.Fake.Calls(http.MethodDelete, path) {
		if call.Path == path {
			out = append(out, call)
		}
	}
	return out
}

func (f *fixture) openNew() {
	f.t.Helper()
	f.router.Push(route.ChatPanel(api.BeeConversation, route.PanelNew))
	f.settle()
}

func (f *fixture) wantKeys(body map[string]any, keys ...string) {
	f.t.Helper()
	if got := slices.Sorted(maps.Keys(body)); !slices.Equal(got, keys) {
		f.t.Fatalf("POST keys = %q, want %q; body %v", got, keys, body)
	}
}

func (f *fixture) openAdvanced() {
	f.t.Helper()
	f.click(locales.WorkersFormSectionAdvanced())
}

func newPanelFixture(t *testing.T, seed func(st *fakeapi.State)) *fixture {
	t.Helper()
	if seed == nil {
		seed = crew
	}
	f := newFixture(t, wideW, newPanel, seed)
	f.T.SetSize(wideW, tallH)
	f.settle()
	return f
}

func withSystemEngine(engine string, enabled ...string) func(st *fakeapi.State) {
	if len(enabled) == 0 {
		enabled = []string{"claude", "codex", "pi"}
	}
	return func(st *fakeapi.State) {
		crew(st)
		st.SystemConfigs = map[string]string{"default_engine": engine}
		st.Engines = fakeapi.Engines(enabled...)
	}
}

func TestCreatePanelShowsBasicSection(t *testing.T) {
	f := newPanelFixture(t, nil)
	f.wantShown(locales.ChatNewWorker(), locales.WorkersFormSectionBasic(), locales.WorkersFormName(), locales.WorkersFormNameHelper(),
		locales.WorkersFormRandomName(), locales.WorkersFormEngine(), locales.WorkersFormEngineHelper(),
		locales.WorkersFormDescriptionOptional(), locales.WorkersFormDescriptionCreateHelper(),
		locales.WorkersFormSectionAdvanced(), locales.WorkersFormAdvancedHelper(), locales.CommonCancel(), locales.WorkersCreateWorker())
	f.wantHidden(locales.WorkersFormConstraintsHelper(), locales.WorkersFormWorkDirHelper(), locales.WorkersFormSectionPermissions(),
		locales.WorkersFormAdvancedSet(1))
	if !f.T.Focused(locales.WorkersFormName()) {
		t.Fatal("the name field does not take the focus")
	}
}

func TestCreateDefaultsEngineFromSystem(t *testing.T) {
	f := newPanelFixture(t, withSystemEngine("codex"))
	f.wantShown(codexName)
	f.wantHidden(claudeName, piName)
	f.replaceText(locales.WorkersFormName(), "Nova")
	f.click(locales.WorkersCreateWorker())
	if body := f.lastPost(); body["engine"] != "codex" {
		t.Fatalf("POST body = %v, want the codex engine", body)
	}
}

func TestCreateEngineFallsBackToFirstEnabled(t *testing.T) {
	f := newPanelFixture(t, withSystemEngine("pi", "claude", "codex"))
	f.wantShown(claudeName)
	f.wantHidden(piName, codexName)
	f.replaceText(locales.WorkersFormName(), "Nova")
	f.click(locales.WorkersCreateWorker())
	if body := f.lastPost(); body["engine"] != "claude" {
		t.Fatalf("POST body = %v, want the first enabled engine", body)
	}
}

func TestCreateWorksWhenSystemConfigsFail(t *testing.T) {
	f := newFixture(t, wideW, route.ChatPanel(api.BeeConversation, ""), withSystemEngine("codex"))
	f.Fake.FailNext(http.MethodGet, "/api/system-configs", http.StatusInternalServerError, map[string]string{"error": "configs down"})
	f.openNew()
	f.wantShown(locales.WorkersFormSectionBasic(), claudeName)
	f.wantHidden(codexName)
	f.replaceText(locales.WorkersFormName(), "Nova")
	f.click(locales.WorkersCreateWorker())
	if body := f.lastPost(); body["engine"] != "claude" {
		t.Fatalf("POST body = %v, want the first enabled engine", body)
	}
}

func TestCreateEngineSelectChangesPayload(t *testing.T) {
	f := newPanelFixture(t, withSystemEngine("claude"))
	f.click(locales.WorkersFormEngine())
	f.click(piName)
	f.wantShown(piName)
	f.replaceText(locales.WorkersFormName(), "Nova")
	f.click(locales.WorkersCreateWorker())
	if body := f.lastPost(); body["engine"] != "pi" {
		t.Fatalf("POST body = %v, want the pi engine", body)
	}
}

func TestCreateOmitsEmptyOptional(t *testing.T) {
	f := newPanelFixture(t, nil)
	f.replaceText(locales.WorkersFormName(), "  Nova  ")
	f.openAdvanced()
	f.replaceText(locales.WorkersFormEngineArgs(), "   ")
	f.click(locales.WorkersCreateWorker())
	body := f.lastPost()
	f.wantKeys(body, "description", "engine", "name")
	if body["name"] != "Nova" || body["description"] != "" || body["engine"] != "claude" {
		t.Fatalf("POST body = %v, want the trimmed name, an empty description and claude", body)
	}
	if n := len(f.posts()); n != 1 {
		t.Fatalf("created %d workers, want 1", n)
	}
}

func TestCreateSendsFilledOptional(t *testing.T) {
	f := newPanelFixture(t, nil)
	f.replaceText(locales.WorkersFormName(), "Nova")
	f.replaceText(locales.WorkersFormDescriptionOptional(), "Reviews pull requests")
	f.openAdvanced()
	f.replaceText(locales.WorkersFormConstraints(), "Be brief.")
	f.replaceText(locales.WorkersFormEngineArgs(), "--model opus")
	f.replaceText(locales.WorkersFormWorkDir(), "/srv/nova")
	f.click(locales.ScopesReadTasksTitle())
	f.click(locales.ScopesReadWorkersTitle())
	f.click(locales.WorkersCreateWorker())
	body := f.lastPost()
	f.wantKeys(body, "constraints", "description", "engine", "engine_args", "name", "permission_scopes", "work_dir")
	args, _ := body["engine_args"].(map[string]any)
	if body["name"] != "Nova" || body["description"] != "Reviews pull requests" || body["constraints"] != "Be brief." ||
		body["work_dir"] != "/srv/nova" || body["permission_scopes"] != "read:tasks,read:workers" ||
		len(args) != 1 || args["claude"] != "--model opus" {
		t.Fatalf("POST body = %v", body)
	}
}

func TestCreateDisabledWithoutName(t *testing.T) {
	f := newPanelFixture(t, nil)
	for _, blank := range []string{"", "   "} {
		f.replaceText(locales.WorkersFormName(), blank)
		f.click(locales.WorkersCreateWorker())
		f.T.Key(0, ui.KeyEnter)
		f.settle()
		if n := len(f.posts()); n != 0 {
			t.Fatalf("name %q: created %d workers, want none", blank, n)
		}
	}
	f.replaceText(locales.WorkersFormName(), "Nova")
	f.T.Key(0, ui.KeyEnter)
	f.settle()
	if n := len(f.posts()); n != 1 {
		t.Fatalf("created %d workers after Enter on a named form, want 1", n)
	}
}

func TestCreateRandomNameFillsAndExhausts(t *testing.T) {
	f := newPanelFixture(t, func(st *fakeapi.State) {
		crew(st)
		st.RandomNames = []string{"Bob", "Nova"}
	})
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
}

func TestCreateAdvancedBadgeCount(t *testing.T) {
	f := newPanelFixture(t, nil)
	f.wantHidden(locales.WorkersFormAdvancedSet(1))
	f.openAdvanced()
	f.wantShown(locales.WorkersFormConstraintsHelper(), locales.WorkersFormWorkDirHelper(), locales.WorkersFormSectionPermissions(),
		locales.WorkersFormPermissionsHelper(), locales.ScopesReadWorkersDescription(), locales.ScopesReadTasksDescription(),
		locales.ScopesReadMessagesDescription(), locales.WorkersFormEngineArgsHelper(claudeName))
	f.wantHidden(locales.WorkersFormAdvancedSet(1))
	f.replaceText(locales.WorkersFormConstraints(), "   ")
	f.openAdvanced()
	f.wantHidden(locales.WorkersFormAdvancedSet(1), locales.WorkersFormConstraintsHelper())

	steps := []struct {
		field, text string
		want        int
	}{
		{locales.WorkersFormConstraints(), "Be brief.", 1},
		{locales.WorkersFormWorkDir(), "/srv/nova", 2},
		{locales.WorkersFormEngineArgs(), "--model opus", 3},
	}
	for _, step := range steps {
		f.openAdvanced()
		f.replaceText(step.field, step.text)
		f.wantHidden(locales.WorkersFormAdvancedSet(step.want))
		f.openAdvanced()
		f.wantShown(locales.WorkersFormAdvancedSet(step.want))
		f.wantHidden(locales.WorkersFormConstraintsHelper())
	}
	f.openAdvanced()
	f.click(locales.ScopesReadMessagesTitle())
	f.click(locales.ScopesReadTasksTitle())
	f.openAdvanced()
	f.wantShown(locales.WorkersFormAdvancedSet(4))
	f.openAdvanced()
	f.click(locales.ScopesReadMessagesTitle())
	f.click(locales.ScopesReadTasksTitle())
	f.openAdvanced()
	f.wantShown(locales.WorkersFormAdvancedSet(3))
}

func TestCreateEngineArgsFollowEngine(t *testing.T) {
	f := newPanelFixture(t, withSystemEngine("claude"))
	f.openAdvanced()
	f.replaceText(locales.WorkersFormEngineArgs(), "--model opus")
	f.openAdvanced()
	f.wantShown(locales.WorkersFormAdvancedSet(1))
	f.openAdvanced()
	f.click(locales.WorkersFormEngine())
	f.click(codexName)
	f.wantShown(locales.WorkersFormEngineArgsHelper(codexName))
	if got := f.fieldText(locales.WorkersFormEngineArgs()); got != "" {
		t.Fatalf("engine args = %q after switching engine, want none", got)
	}
	f.openAdvanced()
	f.wantHidden(locales.WorkersFormAdvancedSet(1))
	f.openAdvanced()
	f.replaceText(locales.WorkersFormName(), "Nova")
	f.click(locales.WorkersCreateWorker())
	body := f.lastPost()
	f.wantKeys(body, "description", "engine", "engine_args", "name")
	args, _ := body["engine_args"].(map[string]any)
	if body["engine"] != "codex" || len(args) != 1 || args["claude"] != "--model opus" {
		t.Fatalf("POST body = %v, want codex with the claude args kept as typed", body)
	}
}

func TestCreateErrorReopensAdvanced(t *testing.T) {
	f := newPanelFixture(t, nil)
	f.replaceText(locales.WorkersFormName(), "Nova")
	f.Fake.FailNext(http.MethodPost, workersPath, http.StatusInternalServerError, map[string]string{"error": "quota reached"})
	f.click(locales.WorkersCreateWorker())
	f.wantShown(locales.WorkersFormCreateError(), "quota reached", locales.WorkersCreateWorker())
	f.wantHidden(locales.WorkersFormConstraintsHelper())
	if got := f.router.Path(); got != "/chat/bee" {
		t.Fatalf("path = %q after a failed create, want /chat/bee", got)
	}

	f.openAdvanced()
	f.replaceText(locales.WorkersFormConstraints(), "Be brief.")
	f.openAdvanced()
	f.wantHidden(locales.WorkersFormConstraintsHelper())
	f.Fake.FailNext(http.MethodPost, workersPath, http.StatusInternalServerError, map[string]string{"error": "quota reached again"})
	f.click(locales.WorkersCreateWorker())
	f.wantShown(locales.WorkersFormCreateError(), "quota reached again", locales.WorkersFormConstraintsHelper())
	if got := f.fieldText(locales.WorkersFormConstraints()); got != "Be brief." {
		t.Fatalf("constraints = %q after a failed create, want the draft kept", got)
	}
	if got := f.fieldText(locales.WorkersFormName()); got != "Nova" {
		t.Fatalf("name = %q after a failed create, want Nova", got)
	}
	f.click(locales.WorkersCreateWorker())
	f.wantHidden(locales.WorkersFormCreateError())
	if n := len(f.posts()); n != 3 {
		t.Fatalf("POST calls = %d, want 3", n)
	}
}

func TestCreateSuccessOpensWorkerConstraints(t *testing.T) {
	f := newPanelFixture(t, nil)
	f.replaceText(locales.WorkersFormName(), "Nova")
	f.click(locales.WorkersCreateWorker())
	workers := f.Fake.Snapshot().Workers
	nova := workers[len(workers)-1]
	if nova.Name != "Nova" {
		t.Fatalf("last server worker = %+v, want Nova", nova)
	}
	if got := f.router.Path(); got != route.Chat(nova.ID) {
		t.Fatalf("path = %q, want %q", got, route.Chat(nova.ID))
	}
	f.wantPanel(route.PanelConstraints)
	if f.router.CanGoBack() {
		t.Fatal("creating a worker pushed a history entry")
	}
	f.wantShown(locales.WorkerDetailConstraintsPanelEmptyTitle("Nova"), locales.WorkerDetailEditInfo(), "Nova")
	f.wantHidden(locales.WorkersFormSectionBasic())
	if n := len(f.Fake.Calls(http.MethodGet, "/api/local/conversations")); n < 2 {
		t.Fatalf("conversation list calls = %d, want a refetch after creating", n)
	}
}

func TestCreateCancelClosesPanel(t *testing.T) {
	f := newPanelFixture(t, nil)
	f.replaceText(locales.WorkersFormName(), "Nova")
	f.click(locales.CommonCancel())
	f.wantPanel("")
	if got := f.router.Path(); got != "/chat/bee" {
		t.Fatalf("path = %q, want /chat/bee", got)
	}
	f.wantHidden(locales.WorkersFormSectionBasic())
	if n := len(f.posts()); n != 0 {
		t.Fatalf("cancel created %d workers", n)
	}
	f.router.Back()
	f.settle()
	f.wantPanel(route.PanelNew)
	if got := f.fieldText(locales.WorkersFormName()); got != "" {
		t.Fatalf("name field = %q after reopening, want a fresh form", got)
	}
}

func TestMoreMenuOpensDeleteDialog(t *testing.T) {
	f := newFixture(t, wideW, route.ChatPanel("w1", route.PanelConstraints), crew)
	f.wantHidden(locales.WorkersDeleteWorker(), locales.WorkersDeleteDialogTitle("Alice"))
	f.click(locales.WorkersRowActions("Alice"))
	f.click(locales.WorkersDeleteWorker())
	f.wantShown(locales.WorkersDeleteDialogTitle("Alice"), locales.WorkersDeleteDialogDescription(),
		locales.WorkersDeleteDialogChatWarning(), locales.WorkersDeleteDialogConfirmNameLabel("Alice"),
		locales.WorkersDeleteDialogDeleteWorkDir(), locales.WorkersDeleteDialogDeleteWorkDirHint("/srv/alice"),
		locales.CommonCancel(), locales.WorkersDeleteDialogConfirm())
	if !f.T.Focused(locales.WorkersDeleteDialogConfirmNameLabel("Alice")) {
		t.Fatal("the confirmation field does not take the focus")
	}
	f.click(locales.CommonCancel())
	f.wantHidden(locales.WorkersDeleteDialogTitle("Alice"))
	if n := len(f.deletes(alicePath)); n != 0 {
		t.Fatalf("cancel sent %d deletes", n)
	}
}

func openDelete(f *fixture, actions string) {
	f.t.Helper()
	f.click(locales.WorkersRowActions(actions))
	f.click(locales.WorkersDeleteWorker())
}

func TestDeleteRequiresExactName(t *testing.T) {
	f := newFixture(t, wideW, route.ChatPanel("w1", route.PanelConstraints), crew)
	openDelete(f, "Alice")
	label := locales.WorkersDeleteDialogConfirmNameLabel("Alice")
	for _, attempt := range []string{"", "alice", "ALICE", " Alice", "Alice ", "Alic"} {
		f.replaceText(label, attempt)
		f.T.Key(0, ui.KeyEnter)
		f.settle()
		f.click(locales.WorkersDeleteDialogConfirm())
		if n := len(f.deletes(alicePath)); n != 0 {
			t.Fatalf("name %q: sent %d deletes, want none", attempt, n)
		}
		f.wantShown(locales.WorkersDeleteDialogTitle("Alice"))
	}
	f.replaceText(label, "Alice")
	f.click(locales.WorkersDeleteDialogConfirm())
	if n := len(f.deletes(alicePath)); n != 1 {
		t.Fatalf("exact name sent %d deletes, want 1", n)
	}
}

func TestDeleteEnterSubmits(t *testing.T) {
	f := newFixture(t, wideW, route.ChatPanel("w1", route.PanelConstraints), crew)
	openDelete(f, "Alice")
	f.T.Type("Alice")
	f.T.Key(0, ui.KeyEnter)
	f.settle()
	if n := len(f.deletes(alicePath)); n != 1 {
		t.Fatalf("Enter sent %d deletes, want 1", n)
	}
}

func TestDeleteWithWorkDir(t *testing.T) {
	f := newFixture(t, wideW, route.ChatPanel("w1", route.PanelConstraints), crew)
	openDelete(f, "Alice")
	f.replaceText(locales.WorkersDeleteDialogConfirmNameLabel("Alice"), "Alice")
	f.click(locales.WorkersDeleteDialogDeleteWorkDir())
	f.click(locales.WorkersDeleteDialogConfirm())
	calls := f.deletes(alicePath)
	if len(calls) != 1 || calls[0].Query != "delete_work_dir=true" {
		t.Fatalf("DELETE calls = %+v, want one with delete_work_dir=true", calls)
	}
}

func TestDeleteKeepsWorkDirByDefault(t *testing.T) {
	f := newFixture(t, wideW, route.ChatPanel("w1", route.PanelConstraints), crew)
	openDelete(f, "Alice")
	f.replaceText(locales.WorkersDeleteDialogConfirmNameLabel("Alice"), "Alice")
	f.click(locales.WorkersDeleteDialogConfirm())
	calls := f.deletes(alicePath)
	if len(calls) != 1 || calls[0].Query != "" {
		t.Fatalf("DELETE calls = %+v, want one without a query", calls)
	}
}

func TestDeleteWorkDirChoiceResetsEachOpen(t *testing.T) {
	f := newFixture(t, wideW, route.ChatPanel("w1", route.PanelConstraints), crew)
	openDelete(f, "Alice")
	f.replaceText(locales.WorkersDeleteDialogConfirmNameLabel("Alice"), "Alice")
	f.click(locales.WorkersDeleteDialogDeleteWorkDir())
	f.click(locales.CommonCancel())
	openDelete(f, "Alice")
	if got := f.fieldText(locales.WorkersDeleteDialogConfirmNameLabel("Alice")); got != "" {
		t.Fatalf("confirmation = %q after reopening, want empty", got)
	}
	f.replaceText(locales.WorkersDeleteDialogConfirmNameLabel("Alice"), "Alice")
	f.click(locales.WorkersDeleteDialogConfirm())
	calls := f.deletes(alicePath)
	if len(calls) != 1 || calls[0].Query != "" {
		t.Fatalf("DELETE calls = %+v, want the work directory kept after reopening", calls)
	}
}

func TestDeleteHintNeedsWorkDir(t *testing.T) {
	f := newFixture(t, wideW, route.ChatPanel("w2", route.PanelConstraints), func(st *fakeapi.State) {
		crew(st)
		st.Workers[1].WorkDir = ""
	})
	openDelete(f, "Bob")
	f.wantShown(locales.WorkersDeleteDialogTitle("Bob"), locales.WorkersDeleteDialogDeleteWorkDir())
	for _, text := range f.T.Texts() {
		if strings.HasPrefix(text, "Removes ") {
			t.Fatalf("dialog shows the path hint %q for a worker without a work directory", text)
		}
	}
}

func TestDeleteNavigatesToBee(t *testing.T) {
	f := newFixture(t, wideW, route.ChatPanel("w1", route.PanelConstraints), crew)
	conversationCalls := len(f.Fake.Calls(http.MethodGet, "/api/local/conversations"))
	openDelete(f, "Alice")
	f.replaceText(locales.WorkersDeleteDialogConfirmNameLabel("Alice"), "Alice")
	f.click(locales.WorkersDeleteDialogConfirm())
	if got := f.router.Path(); got != "/chat/bee" {
		t.Fatalf("path = %q after deleting, want /chat/bee", got)
	}
	f.wantPanel("")
	if f.router.CanGoBack() {
		t.Fatal("deleting a worker pushed a history entry")
	}
	f.wantHidden(locales.WorkersDeleteDialogTitle("Alice"), locales.WorkerDetailEditInfo())
	if _, ok := f.Store.Worker("w1"); ok {
		t.Fatal("the deleted worker is still in the list")
	}
	if n := len(f.Fake.Calls(http.MethodGet, "/api/local/conversations")); n <= conversationCalls {
		t.Fatalf("conversation list calls = %d, want a refetch after deleting", n)
	}
	if n := len(f.Fake.Calls(http.MethodGet, alicePath)); n != 1 {
		t.Fatalf("deleted worker detail calls = %d, want only the first load", n)
	}
}

func TestDeleteErrorKeepsDialog(t *testing.T) {
	f := newFixture(t, wideW, route.ChatPanel("w1", route.PanelConstraints), crew)
	f.Fake.FailNext(http.MethodDelete, alicePath, http.StatusInternalServerError, map[string]string{"error": "worker is busy"})
	openDelete(f, "Alice")
	f.replaceText(locales.WorkersDeleteDialogConfirmNameLabel("Alice"), "Alice")
	f.click(locales.WorkersDeleteDialogConfirm())
	f.wantShown(locales.WorkersDeleteDialogError(), "worker is busy", locales.WorkersDeleteDialogTitle("Alice"))
	if got := f.router.Path(); got != "/chat/w1" {
		t.Fatalf("path = %q after a failed delete, want /chat/w1", got)
	}
	f.click(locales.WorkersDeleteDialogConfirm())
	f.wantHidden(locales.WorkersDeleteDialogTitle("Alice"), locales.WorkersDeleteDialogError())
	if got := f.router.Path(); got != "/chat/bee" {
		t.Fatalf("path = %q after the retry, want /chat/bee", got)
	}
}

func TestDeleteLockedWhilePending(t *testing.T) {
	f := newFixture(t, wideW, route.ChatPanel("w1", route.PanelConstraints), crew)
	openDelete(f, "Alice")
	f.replaceText(locales.WorkersDeleteDialogConfirmNameLabel("Alice"), "Alice")
	if err := f.T.Click(locales.WorkersDeleteDialogConfirm()); err != nil {
		t.Fatal(err)
	}
	f.T.Frame()
	f.T.Key(0, ui.KeyEscape)
	f.T.Frame()
	if err := f.T.Click(locales.CommonCancel()); err != nil {
		t.Fatal(err)
	}
	f.T.Frame()
	f.wantShown(locales.WorkersDeleteDialogTitle("Alice"))
	f.settle()
	f.wantHidden(locales.WorkersDeleteDialogTitle("Alice"))
	if got := f.router.Path(); got != "/chat/bee" {
		t.Fatalf("path = %q, want /chat/bee", got)
	}
	if n := len(f.deletes(alicePath)); n != 1 {
		t.Fatalf("sent %d deletes, want 1", n)
	}
}

func TestMoreMenuWorksFromTheKeyboard(t *testing.T) {
	f := newFixture(t, wideW, route.ChatPanel("w1", route.PanelConstraints), crew)
	f.click(locales.WorkersRowActions("Alice"))
	f.T.Key(0, ui.KeyTab)
	f.settle()
	if !f.T.Focused(locales.WorkersDeleteWorker()) {
		t.Fatal("Tab did not move into the worker menu")
	}
	f.T.Key(0, ui.KeyDown)
	f.settle()
	if !f.T.Focused(locales.WorkersDeleteWorker()) {
		t.Fatal("the down arrow left the only menu item")
	}
	f.T.Key(0, ui.KeyEnter)
	f.settle()
	f.wantShown(locales.WorkersDeleteDialogTitle("Alice"))
}

func TestCreateOffersPluginEngines(t *testing.T) {
	f := newPanelFixture(t, func(st *fakeapi.State) {
		crew(st)
		st.SystemConfigs = map[string]string{"default_engine": "gemini"}
		st.Engines = []api.EngineInfo{
			{ID: "pi", Name: api.LocalizedText{"en": piName}},
			{ID: "gemini", Name: api.LocalizedText{"en": "Gemini CLI"}},
		}
	})
	f.wantShown("Gemini CLI")
	f.click(locales.WorkersFormEngine())
	f.wantShown(piName)
	f.click(piName)
	f.replaceText(locales.WorkersFormName(), "Nova")
	f.click(locales.WorkersCreateWorker())
	if body := f.lastPost(); body["engine"] != "pi" {
		t.Fatalf("POST body = %v, want the picked engine", body)
	}
}

func TestCreateNamesEnginesOfAnOlderServerByTheirIds(t *testing.T) {
	f := newPanelFixture(t, func(st *fakeapi.State) {
		crew(st)
		st.SystemConfigs = map[string]string{"default_engine": "codex"}
		st.Engines = fakeapi.Engines("claude", "codex")
		st.LegacyConfig = true
	})
	f.wantShown("codex")
	f.wantHidden(codexName)
	f.replaceText(locales.WorkersFormName(), "Nova")
	f.click(locales.WorkersCreateWorker())
	if body := f.lastPost(); body["engine"] != "codex" {
		t.Fatalf("POST body = %v, want the default engine", body)
	}
}

func TestCreateWithoutAnEngineExplainsWhy(t *testing.T) {
	f := newPanelFixture(t, func(st *fakeapi.State) {
		crew(st)
		st.Engines = nil
	})
	f.wantShown(locales.WorkersFormNoEngine())
	f.wantHidden(locales.WorkersFormEngineHelper(), claudeName, codexName, piName)
	f.openAdvanced()
	f.wantHidden(locales.WorkersFormEngineArgs())
	f.replaceText(locales.WorkersFormName(), "Nova")
	f.click(locales.WorkersCreateWorker())
	if body := f.lastPost(); body["engine"] != nil {
		t.Fatalf("POST body = %v, want no engine", body)
	}
}

func TestCreateWaitsForTheEnginesBeforeSeeding(t *testing.T) {
	f := newFixture(t, wideW, route.ChatPanel(api.BeeConversation, ""), withSystemEngine("codex"))
	f.Store.SystemConfigs.Ensure()
	f.settle()
	f.Store.Config.Reset()
	release := f.Fake.HoldNext(http.MethodGet, "/api/config")
	f.router.Push(route.ChatPanel(api.BeeConversation, route.PanelNew))
	f.T.Advance(motionSettled)
	f.T.Frame()
	f.wantShown(locales.ChatNewWorker())
	f.wantHidden(locales.WorkersFormEngine(), locales.WorkersFormNoEngine())
	release()
	f.settle()
	f.wantShown(locales.WorkersFormEngine(), codexName)
	f.wantHidden(locales.WorkersFormNoEngine(), claudeName)
	f.replaceText(locales.WorkersFormName(), "Nova")
	f.click(locales.WorkersCreateWorker())
	if body := f.lastPost(); body["engine"] != "codex" {
		t.Fatalf("POST body = %v, want the system default engine", body)
	}
}
