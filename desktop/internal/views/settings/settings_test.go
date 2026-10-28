package settings_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/api"
	"github.com/theopenbee/openbee/desktop/internal/fakeapi"
	"github.com/theopenbee/openbee/desktop/internal/store"
	"github.com/theopenbee/openbee/desktop/internal/views/settings"
	"github.com/theopenbee/openbee/desktop/internal/views/testkit"
	"github.com/theopenbee/openbee/desktop/locales"
)

const (
	wideW, wideH   = 1440, 1000
	systemPath     = "/api/system-configs"
	engineArgsBee  = "engine_args_bee"
	engineArgsAll  = "engine_args_global"
	defaultEngine  = "default_engine"
	navStep        = 38
	paneX, paneY   = 900, 500
	paneTop        = 200
	paneBottom     = 910
	scrollStep     = 300
	scrollTries    = 8
	savedFlash     = 2400 * time.Millisecond
	flashTolerance = 100 * time.Millisecond
)

type fixture struct {
	*testkit.Harness
	t *testing.T
}

func configured(st *fakeapi.State) {
	st.Workers = []api.Worker{{ID: "w1", Name: "Alice", Status: "idle", Engine: "claude"}}
	st.SystemConfigs = map[string]string{defaultEngine: "claude", engineArgsAll: `{"claude":"--model opus"}`}
	st.Envs = []api.EnvConfig{
		{ID: "g1", Scope: "global", Key: "DATABASE_URL", Masked: "post****5432", UpdatedAt: time.Now().Add(-2 * time.Hour).UnixMilli()},
		{ID: "b1", Scope: "bee", ScopeID: new("default"), Key: "BEE_TOKEN", Masked: "bee-****", UpdatedAt: time.Now().Add(-time.Hour).UnixMilli()},
	}
}

func open(t *testing.T, w int, section string, seed func(st *fakeapi.State)) *fixture {
	t.Helper()
	hs := testkit.SignedIn(t, w, wideH, seed)
	f := &fixture{Harness: hs, t: t}
	hs.Root.OpenSettings(section)
	f.settle()
	return f
}

func (f *fixture) settle() {
	f.Settle()
}

func (f *fixture) click(name string) {
	f.t.Helper()
	if err := f.T.Click(name); err != nil {
		f.t.Fatalf("click %q: %v (texts %q)", name, err, f.T.Texts())
	}
	f.settle()
}

func (f *fixture) reveal(name string) {
	f.t.Helper()
	for range scrollTries {
		r := f.rect(name)
		switch {
		case r.Y < paneTop:
			f.T.Scroll(paneX, paneY, 0, -scrollStep)
		case r.Y+r.H > paneBottom:
			f.T.Scroll(paneX, paneY, 0, scrollStep)
		default:
			return
		}
		f.settle()
	}
	f.t.Fatalf("%q does not scroll into view: %+v", name, f.rect(name))
}

func (f *fixture) clickIn(name string) {
	f.t.Helper()
	f.reveal(name)
	f.click(name)
}

func (f *fixture) chooseEngine(downs int) {
	f.t.Helper()
	f.click(locales.WorkersEnginesClaude())
	for range downs {
		f.T.Key(0, ui.KeyDown)
	}
	f.T.Key(0, ui.KeyEnter)
	f.settle()
}

func (f *fixture) typeInto(field, text string) {
	f.t.Helper()
	f.clickIn(field)
	f.T.Command("selectAll")
	f.T.Type(text)
	f.settle()
}

func (f *fixture) fieldText(field string) string {
	f.t.Helper()
	f.clickIn(field)
	f.T.SetClipboard("")
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
			f.t.Fatalf("screen still shows %q; texts %q", text, f.T.Texts())
		}
	}
}

func (f *fixture) rect(text string) ui.Rect {
	f.t.Helper()
	r, ok := f.T.Find(text)
	if !ok {
		f.t.Fatalf("no %q on screen; texts %q", text, f.T.Texts())
	}
	return r
}

func (f *fixture) puts() map[string]string {
	f.t.Helper()
	out := map[string]string{}
	for _, call := range f.Fake.Calls(http.MethodPut, systemPath) {
		var body map[string]string
		if err := json.Unmarshal(call.Body, &body); err != nil {
			f.t.Fatalf("decode PUT body %q: %v", call.Body, err)
		}
		out[call.Path] = body["value"]
	}
	return out
}

func beeInput(engine string) string {
	return locales.SystemSettingsBeeArgsSectionTitle() + ": " + engine
}

func globalInput(engine string) string {
	return locales.SystemSettingsGlobalArgsSectionTitle() + ": " + engine
}

func TestSettingsOpensAtSection(t *testing.T) {
	f := open(t, wideW, settings.SectionEnv, configured)
	f.wantShown(locales.EnvConfigTitle(), locales.EnvConfigPageDescription(), locales.EnvConfigGlobalTitle(), locales.EnvConfigBeeTitle(),
		"DATABASE_URL", "BEE_TOKEN", locales.EnvConfigNavTitle(), locales.PluginsTitle(), locales.ArcDialogClose())
	f.wantHidden(locales.SystemSettingsSubtitle())

	f.Root.OpenSettings(settings.SectionSystem)
	f.settle()
	f.wantShown(locales.SystemSettingsSubtitle(), locales.SystemSettingsEngineSectionTitle(), locales.SystemSettingsArgsSectionTitle())
	f.wantHidden(locales.EnvConfigPageDescription(), "DATABASE_URL")

	f.Root.OpenSettings(settings.SectionPlugins)
	f.settle()
	f.wantShown(locales.PluginsSubtitle())
	f.wantHidden(locales.SystemSettingsSubtitle())

	f.Root.OpenSettings("unknown")
	f.settle()
	f.wantShown(locales.EnvConfigPageDescription())
}

func TestSettingsEscapeCloses(t *testing.T) {
	f := open(t, wideW, settings.SectionEnv, configured)
	f.wantShown(locales.EnvConfigPageDescription())
	f.T.Key(0, ui.KeyEscape)
	f.settle()
	f.wantHidden(locales.EnvConfigPageDescription(), locales.ArcDialogClose())

	f.Root.OpenSettings(settings.SectionEnv)
	f.settle()
	f.click(locales.ArcDialogClose())
	f.wantHidden(locales.EnvConfigPageDescription())

	f.Root.OpenSettings(settings.SectionEnv)
	f.settle()
	f.T.ClickAt(5, 5)
	f.settle()
	f.wantHidden(locales.EnvConfigPageDescription())
}

func TestSettingsEscapeClosesOnlyTheTopOverlay(t *testing.T) {
	f := open(t, wideW, settings.SectionEnv, configured)
	f.click(locales.EnvConfigEditLabel("DATABASE_URL"))
	f.wantShown(locales.EnvConfigEditDescription())
	f.T.Key(0, ui.KeyEscape)
	f.settle()
	f.wantHidden(locales.EnvConfigEditDescription())
	f.wantShown(locales.EnvConfigPageDescription(), "DATABASE_URL")
}

func TestSettingsNavSwitchesSections(t *testing.T) {
	f := open(t, wideW, settings.SectionEnv, configured)
	env := f.rect(locales.EnvConfigNavTitle())
	plugins := f.rect(locales.PluginsTitle())
	if plugins.X != env.X || plugins.Y <= env.Y {
		t.Fatalf("wide nav is not a column: env %+v, plugins %+v", env, plugins)
	}
	f.click(locales.PluginsTitle())
	f.wantShown(locales.PluginsSubtitle())
	f.wantHidden(locales.EnvConfigPageDescription())
	f.click(locales.EnvConfigNavTitle())
	f.wantShown(locales.EnvConfigPageDescription(), "DATABASE_URL")

	f.T.ClickAt(env.X+env.W/2, env.Y+env.H/2+navStep)
	f.settle()
	f.wantShown(locales.SystemSettingsSubtitle(), locales.SystemSettingsEngineSectionTitle())
	f.wantHidden(locales.EnvConfigPageDescription())
}

func TestSettingsSectionRemountsOnSwitch(t *testing.T) {
	f := open(t, wideW, settings.SectionSystem, configured)
	field := beeInput(locales.WorkersEnginesCodex())
	f.typeInto(field, "--draft")
	if got := f.fieldText(field); got != "--draft" {
		t.Fatalf("bee codex args = %q before leaving, want the draft", got)
	}
	f.click(locales.EnvConfigNavTitle())
	f.wantShown(locales.EnvConfigPageDescription())
	env := f.rect(locales.EnvConfigNavTitle())
	f.T.ClickAt(env.X+env.W/2, env.Y+env.H/2+navStep)
	f.settle()
	if got := f.fieldText(field); got != "" {
		t.Fatalf("bee codex args = %q after switching away and back, want the form reset", got)
	}
	f.clickIn(locales.SystemSettingsSave())
	if n := len(f.puts()); n != 0 {
		t.Fatalf("saving a reset form sent %d writes", n)
	}
}

func TestSettingsNarrowNavIsAStrip(t *testing.T) {
	f := open(t, 700, settings.SectionEnv, configured)
	env := f.rect(locales.EnvConfigNavTitle())
	plugins := f.rect(locales.PluginsTitle())
	if plugins.Y != env.Y || plugins.X <= env.X {
		t.Fatalf("narrow nav is not a strip: env %+v, plugins %+v", env, plugins)
	}
	f.click(locales.PluginsTitle())
	f.wantShown(locales.PluginsSubtitle())
}

func TestSettingsClosesOnSignOut(t *testing.T) {
	f := open(t, wideW, settings.SectionEnv, configured)
	f.wantShown(locales.EnvConfigPageDescription())
	f.Store.Logout()
	f.settle()
	f.wantHidden(locales.EnvConfigPageDescription())
	if f.Store.Phase == store.PhaseReady {
		t.Fatalf("phase = %v after sign-out", f.Store.Phase)
	}
	snap := f.Fake.Snapshot()
	f.Store.Login(snap.Username, snap.Password, func(error) {})
	f.settle()
	f.wantHidden(locales.EnvConfigPageDescription(), locales.ArcDialogClose())
}

func TestSystemShowsSavedValues(t *testing.T) {
	f := open(t, wideW, settings.SectionSystem, configured)
	f.wantShown(locales.SystemSettingsEngineSectionRowTitle(), locales.SystemSettingsEngineSectionDescription(),
		locales.SystemSettingsEngineSectionLabel(), locales.WorkersEnginesClaude(),
		locales.SystemSettingsArgsSectionDescription(),
		locales.SystemSettingsGlobalArgsSectionTitle(), locales.SystemSettingsGlobalArgsSectionDescription(),
		locales.SystemSettingsBeeArgsSectionTitle(), locales.SystemSettingsBeeArgsSectionDescription(),
		globalInput(locales.WorkersEnginesClaude()), globalInput(locales.WorkersEnginesCodex()), globalInput(locales.WorkersEnginesPi()),
		beeInput(locales.WorkersEnginesPi()), locales.SystemSettingsSave())
	if got := f.fieldText(globalInput(locales.WorkersEnginesClaude())); got != "--model opus" {
		t.Fatalf("global claude args = %q, want the saved flags", got)
	}
}

func TestSystemOnlyEnabledEnginesGetFields(t *testing.T) {
	f := open(t, wideW, settings.SectionSystem, func(st *fakeapi.State) {
		configured(st)
		st.EnabledEngines = []string{"codex"}
	})
	f.wantShown(globalInput(locales.WorkersFormEngineArgs()), beeInput(locales.WorkersFormEngineArgs()),
		locales.WorkersFormEngineArgsHelper(locales.WorkersEnginesCodex()))
	f.wantHidden(globalInput(locales.WorkersEnginesClaude()), globalInput(locales.WorkersEnginesPi()))
}

func TestSystemSaveOnlyChanged(t *testing.T) {
	f := open(t, wideW, settings.SectionSystem, configured)
	f.typeInto(beeInput(locales.WorkersEnginesCodex()), "--x")
	f.clickIn(locales.SystemSettingsSave())
	want := map[string]string{systemPath + "/" + engineArgsBee: `{"codex":"--x"}`}
	if got := f.puts(); len(got) != len(want) || got[systemPath+"/"+engineArgsBee] != want[systemPath+"/"+engineArgsBee] {
		t.Fatalf("PUTs = %v, want %v", got, want)
	}
	if got := f.Store.SystemConfigs.Data[engineArgsBee]; got != `{"codex":"--x"}` {
		t.Fatalf("store bee args = %q after save", got)
	}
}

func TestSystemSaveSendsEveryChangedKey(t *testing.T) {
	f := open(t, wideW, settings.SectionSystem, configured)
	f.chooseEngine(2)
	f.typeInto(globalInput(locales.WorkersEnginesClaude()), "")
	f.typeInto(globalInput(locales.WorkersEnginesPi()), "--think")
	f.clickIn(locales.SystemSettingsSave())
	got := f.puts()
	want := map[string]string{
		systemPath + "/" + defaultEngine: "pi",
		systemPath + "/" + engineArgsAll: `{"pi":"--think"}`,
	}
	if len(got) != len(want) {
		t.Fatalf("PUTs = %v, want %v", got, want)
	}
	for path, value := range want {
		if got[path] != value {
			t.Fatalf("PUT %s = %q, want %q (all: %v)", path, got[path], value, got)
		}
	}
	f.wantShown(locales.SystemSettingsSaved(), locales.WorkersEnginesPi())
}

func TestSystemSaveDisabledWhenClean(t *testing.T) {
	f := open(t, wideW, settings.SectionSystem, configured)
	f.clickIn(locales.SystemSettingsSave())
	if n := len(f.puts()); n != 0 {
		t.Fatalf("saving a clean form sent %d writes", n)
	}
	f.typeInto(beeInput(locales.WorkersEnginesCodex()), "   ")
	f.clickIn(locales.SystemSettingsSave())
	if n := len(f.puts()); n != 0 {
		t.Fatalf("a blank-only edit sent %d writes", n)
	}
	f.typeInto(beeInput(locales.WorkersEnginesCodex()), "--x")
	f.typeInto(beeInput(locales.WorkersEnginesCodex()), "")
	f.clickIn(locales.SystemSettingsSave())
	if n := len(f.puts()); n != 0 {
		t.Fatalf("reverting an edit sent %d writes", n)
	}
	f.typeInto(beeInput(locales.WorkersEnginesCodex()), "--x")
	f.clickIn(locales.SystemSettingsSave())
	if n := len(f.puts()); n != 1 {
		t.Fatalf("saving a real change sent %d writes, want 1", n)
	}
}

func TestSystemSavedFlash(t *testing.T) {
	f := open(t, wideW, settings.SectionSystem, configured)
	f.typeInto(beeInput(locales.WorkersEnginesPi()), "--x")
	f.clickIn(locales.SystemSettingsSave())
	f.wantShown(locales.SystemSettingsSaved())
	f.wantHidden(locales.SystemSettingsSave())
	f.T.Advance(savedFlash - flashTolerance)
	f.wantShown(locales.SystemSettingsSaved())
	f.T.Advance(flashTolerance)
	f.wantShown(locales.SystemSettingsSave())
	f.wantHidden(locales.SystemSettingsSaved())
	f.clickIn(locales.SystemSettingsSave())
	if n := len(f.puts()); n != 1 {
		t.Fatalf("the idle save button sent %d more writes", n-1)
	}
}

func TestSystemEditingEndsTheSavedFlash(t *testing.T) {
	f := open(t, wideW, settings.SectionSystem, configured)
	f.typeInto(beeInput(locales.WorkersEnginesPi()), "--x")
	f.clickIn(locales.SystemSettingsSave())
	f.wantShown(locales.SystemSettingsSaved())
	f.typeInto(beeInput(locales.WorkersEnginesPi()), "--y")
	f.wantShown(locales.SystemSettingsSave())
	f.wantHidden(locales.SystemSettingsSaved())
}

func TestSystemSaveError(t *testing.T) {
	f := open(t, wideW, settings.SectionSystem, configured)
	f.Fake.FailNext(http.MethodPut, systemPath+"/"+engineArgsBee, http.StatusBadRequest, map[string]string{"error": "flags rejected"})
	f.typeInto(beeInput(locales.WorkersEnginesPi()), "--x")
	f.clickIn(locales.SystemSettingsSave())
	f.wantShown(locales.SystemSettingsSaveError(), "flags rejected", locales.SystemSettingsSave())
	f.wantHidden(locales.SystemSettingsSaved())
	if got := f.Store.SystemConfigs.Data[engineArgsBee]; got != "" {
		t.Fatalf("store bee args = %q after a failed save", got)
	}

	f.clickIn(locales.SystemSettingsSave())
	f.wantHidden(locales.SystemSettingsSaveError())
	f.wantShown(locales.SystemSettingsSaved())
	if n := len(f.Fake.Calls(http.MethodPut, systemPath+"/"+engineArgsBee)); n != 2 {
		t.Fatalf("bee writes = %d, want the failed one and the retry", n)
	}

	f.Fake.FailNext(http.MethodPut, systemPath+"/"+engineArgsAll, http.StatusInternalServerError, map[string]string{"error": "again"})
	f.typeInto(globalInput(locales.WorkersEnginesPi()), "--z")
	f.clickIn(locales.SystemSettingsSave())
	f.wantShown(locales.SystemSettingsSaveError(), "again")
	f.typeInto(globalInput(locales.WorkersEnginesPi()), "--w")
	f.wantHidden(locales.SystemSettingsSaveError())
}

func TestSystemLoadErrorRetry(t *testing.T) {
	f := open(t, wideW, "", func(st *fakeapi.State) { configured(st) })
	f.Fake.FailNext(http.MethodGet, systemPath, http.StatusInternalServerError, map[string]string{"error": "configs down"})
	f.Root.OpenSettings(settings.SectionSystem)
	f.settle()
	f.wantShown(locales.SystemSettingsLoadError(), "configs down", locales.CommonRetry())
	f.wantHidden(locales.SystemSettingsEngineSectionTitle(), locales.SystemSettingsSave())

	f.clickIn(locales.CommonRetry())
	f.wantShown(locales.SystemSettingsEngineSectionTitle(), locales.SystemSettingsSave())
	f.wantHidden(locales.SystemSettingsLoadError())
}

func TestSystemLoadingShowsSkeleton(t *testing.T) {
	hs := testkit.SignedIn(t, wideW, wideH, configured)
	f := &fixture{Harness: hs, t: t}
	hs.Store.SystemConfigs.Reset()
	hs.Store.SystemConfigs.Loading = true
	hs.Root.OpenSettings(settings.SectionSystem)
	f.settle()
	f.wantShown(locales.SystemSettingsSubtitle())
	f.wantHidden(locales.SystemSettingsEngineSectionTitle(), locales.SystemSettingsSave(), locales.SystemSettingsLoadError())
	if !f.T.FramePending() {
		t.Fatal("the skeleton does not pulse")
	}
}

func TestSystemRowsFollowTheContentWidth(t *testing.T) {
	for _, tc := range []struct {
		window int
		side   bool
	}{{1000, true}, {960, false}} {
		f := open(t, tc.window, settings.SectionSystem, configured)
		title := f.rect(locales.SystemSettingsEngineSectionRowTitle())
		field := f.rect(locales.SystemSettingsEngineSectionLabel())
		if side := field.X > title.X+title.W && field.Y < title.Y+title.H; side != tc.side {
			t.Fatalf("window %d: title %+v, field label %+v, side by side = %v, want %v", tc.window, title, field, side, tc.side)
		}
	}
}

func TestSystemSelectOffersOnlyEnabledEngines(t *testing.T) {
	f := open(t, wideW, settings.SectionSystem, func(st *fakeapi.State) {
		configured(st)
		st.EnabledEngines = []string{"claude", "pi"}
	})
	f.chooseEngine(1)
	f.clickIn(locales.SystemSettingsSave())
	if got := f.puts()[systemPath+"/"+defaultEngine]; got != "pi" {
		t.Fatalf("default engine PUT = %q, want pi (the option after claude)", got)
	}
}
