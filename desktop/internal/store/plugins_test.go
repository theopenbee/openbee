package store_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/theopenbee/openbee/desktop/internal/api"
	"github.com/theopenbee/openbee/desktop/internal/fakeapi"
	"github.com/theopenbee/openbee/desktop/internal/kit"
	"github.com/theopenbee/openbee/desktop/internal/model"
	"github.com/theopenbee/openbee/desktop/internal/store"
	"github.com/theopenbee/openbee/desktop/internal/store/storetest"
	"github.com/theopenbee/openbee/desktop/locales"
)

const (
	pluginsPath    = "/api/plugins"
	opsPath        = "/api/plugins/operations"
	catalogPath    = "/api/plugins/catalog"
	installPath    = "/api/plugins/install"
	linearPackage  = "@openbee/plugin-linear"
	telegramPkg    = "@openbee/plugin-telegram"
	missingPackage = "@acme/plugin-missing"
	wakeGrace      = 20 * time.Millisecond
)

func pluginCatalog(st *fakeapi.State) {
	st.Catalog = api.PluginCatalog{
		SchemaVersion: 1,
		Tags:          []api.CatalogTag{{ID: "chat", Name: api.LocalizedText{"en": "Chat"}}},
		Plugins: []api.CatalogPlugin{
			{ID: "linear", Package: linearPackage, Version: "0.4.0", Name: api.LocalizedText{"en": "Linear", "zh": "Linear 项目"}, Installable: true},
			{ID: "telegram", Package: telegramPkg, Version: "1.3.0", Name: api.LocalizedText{"en": "Telegram"}, Tags: []string{"chat"},
				Installed: true, InstalledVersion: "1.2.0", UpdateAvailable: true, Installable: true},
		},
	}
	st.Plugins = []api.PluginDetail{{PluginStatus: api.PluginStatus{ID: "telegram", Package: telegramPkg, Source: "npm", Version: "1.2.0",
		Name: api.LocalizedText{"en": "Telegram"}, Enabled: true, State: "running"}}}
}

func withPluginOps(st *fakeapi.State) {
	team(st)
	pluginCatalog(st)
	st.Operations = []api.PluginOperation{{ID: "op-seed", Kind: "install", Spec: linearPackage + "@0.4.0", Status: "running", CreatedAt: time.Now()}}
}

func withStartingPlugin(st *fakeapi.State) {
	team(st)
	pluginCatalog(st)
	st.Plugins[0].State = "starting"
}

func activePlugins(t *testing.T, seed func(st *fakeapi.State)) (*store.PluginsState, *fakeapi.Server, *storetest.Fixture, *store.Store) {
	t.Helper()
	st, fake, fix := loaded(t, seed)
	p := st.Plugins()
	if st.Plugins() != p {
		t.Fatal("Plugins returned a new state on the second call")
	}
	p.SetActive(true)
	p.List.Ensure()
	p.Catalog.Ensure()
	p.Ops.Ensure()
	fix.Settle(t)
	return p, fake, fix, st
}

func tick(t *testing.T, fix *storetest.Fixture, fake *fakeapi.Server, d time.Duration, path string) {
	t.Helper()
	n := countCalls(fake, http.MethodGet, path)
	fix.Clock.Advance(d)
	fix.Await(t, func() bool { return countCalls(fake, http.MethodGet, path) > n })
}

func unchanged(t *testing.T, fix *storetest.Fixture, fake *fakeapi.Server, d time.Duration, paths ...string) {
	t.Helper()
	fix.Settle(t)
	before := make([]int, len(paths))
	for i, path := range paths {
		before[i] = countCalls(fake, http.MethodGet, path)
	}
	fix.Clock.Advance(d)
	time.Sleep(wakeGrace)
	fix.Settle(t)
	for i, path := range paths {
		if n := countCalls(fake, http.MethodGet, path); n != before[i] {
			t.Fatalf("GET %s ran %d more times, want none", path, n-before[i])
		}
	}
}

func quiet(t *testing.T, fix *storetest.Fixture, fake *fakeapi.Server, d time.Duration, paths ...string) {
	t.Helper()
	unchanged(t, fix, fake, d, paths...)
	if n := fix.Clock.Sleepers(); n != 0 {
		t.Fatalf("%d timers still sleeping after polling should have stopped", n)
	}
}

func install(t *testing.T, fix *storetest.Fixture, p *store.PluginsState, spec string) api.PluginOperation {
	t.Helper()
	var op api.PluginOperation
	var err error
	calls := 0
	p.Install(spec, func(o api.PluginOperation, e error) {
		op, err = o, e
		calls++
	})
	if !p.Requested(model.SpecPackage(spec)) {
		t.Fatalf("install of %s not marked as requested while the POST runs", spec)
	}
	fix.Settle(t)
	if calls != 1 || err != nil {
		t.Fatalf("Install(%s): calls=%d err=%v", spec, calls, err)
	}
	if p.Requested(model.SpecPackage(spec)) {
		t.Fatalf("install of %s still marked as requested after the POST", spec)
	}
	return op
}

func lastToast(t *testing.T, st *store.Store) kit.Toast {
	t.Helper()
	if len(st.Toasts) == 0 {
		t.Fatal("no toast queued")
	}
	return st.Toasts[len(st.Toasts)-1]
}

func awaitToasts(t *testing.T, fix *storetest.Fixture, st *store.Store, n int) {
	t.Helper()
	fix.Await(t, func() bool { return len(st.Toasts) >= n })
	if len(st.Toasts) != n {
		t.Fatalf("toasts = %+v, want %d", st.Toasts, n)
	}
}

func TestOpsPollingStopsWhenDone(t *testing.T) {
	p, fake, fix, _ := activePlugins(t, withPluginOps)
	if n := countCalls(fake, http.MethodGet, opsPath); n != 1 {
		t.Fatalf("operations reads = %d after Ensure, want 1", n)
	}
	tick(t, fix, fake, time.Second, opsPath)
	tick(t, fix, fake, time.Second, opsPath)
	fake.FinishOperation("op-seed", true, "")
	lists, catalogs := countCalls(fake, http.MethodGet, pluginsPath), countCalls(fake, http.MethodGet, catalogPath)
	tick(t, fix, fake, time.Second, opsPath)
	fix.Settle(t)
	if p.Ops.Data[0].Status != "succeeded" {
		t.Fatalf("op status = %q, want succeeded", p.Ops.Data[0].Status)
	}
	if countCalls(fake, http.MethodGet, pluginsPath) != lists+1 || countCalls(fake, http.MethodGet, catalogPath) != catalogs+1 {
		t.Fatal("finishing an op did not refetch the plugin list and the catalog once")
	}
	quiet(t, fix, fake, 5*time.Second, opsPath, pluginsPath)
}

func TestPollingOnlyWhileActive(t *testing.T) {
	st, fake, fix := loaded(t, withPluginOps)
	p := st.Plugins()
	p.Ops.Ensure()
	fix.Settle(t)
	quiet(t, fix, fake, 3*time.Second, opsPath)

	p.SetActive(true)
	fix.Settle(t)
	if n := countCalls(fake, http.MethodGet, opsPath); n != 2 {
		t.Fatalf("operations reads = %d after activation, want a fresh read", n)
	}
	tick(t, fix, fake, time.Second, opsPath)
	fix.Settle(t)

	p.SetActive(false)
	quiet(t, fix, fake, 3*time.Second, opsPath)
}

func TestInstallCompletionToast(t *testing.T) {
	p, fake, fix, st := activePlugins(t, func(s *fakeapi.State) {
		team(s)
		pluginCatalog(s)
	})
	en := locales.In("en")

	op := install(t, fix, p, linearPackage+"@0.4.0")
	var body map[string]string
	lastBody(t, fake, http.MethodPost, installPath, &body)
	if body["spec"] != linearPackage+"@0.4.0" {
		t.Fatalf("install spec = %q", body["spec"])
	}
	if len(p.Ops.Data) == 0 || p.Ops.Data[0].ID != op.ID {
		t.Fatalf("ops = %+v, want the new operation first", p.Ops.Data)
	}
	if len(st.Toasts) != 0 {
		t.Fatalf("toasts before the op finished: %+v", st.Toasts)
	}
	fake.FinishOperation(op.ID, true, "")
	tick(t, fix, fake, time.Second, opsPath)
	awaitToasts(t, fix, st, 1)
	if got := lastToast(t, st); got.Tone != kit.ToneSuccess || got.Title != en.PluginsInstallDone("Linear") {
		t.Fatalf("toast = %+v, want install done for Linear", got)
	}

	op = install(t, fix, p, telegramPkg+"@1.3.0")
	fake.FinishOperation(op.ID, true, "")
	tick(t, fix, fake, time.Second, opsPath)
	awaitToasts(t, fix, st, 2)
	if got := lastToast(t, st); got.Tone != kit.ToneSuccess || got.Title != en.PluginsUpdateDone("Telegram", "1.3.0") {
		t.Fatalf("toast = %+v, want update done for Telegram 1.3.0", got)
	}

	op = install(t, fix, p, missingPackage+"@1.0.0")
	fake.FinishOperation(op.ID, false, "npm ERR! 404")
	tick(t, fix, fake, time.Second, opsPath)
	awaitToasts(t, fix, st, 3)
	if got := lastToast(t, st); got.Tone != kit.ToneDanger || got.Title != en.PluginsInstallFailed(missingPackage, "npm ERR! 404") {
		t.Fatalf("toast = %+v, want install failed named by the package", got)
	}
	quiet(t, fix, fake, 3*time.Second, opsPath)
}

func TestUninstallCompletionToast(t *testing.T) {
	p, fake, fix, st := activePlugins(t, func(s *fakeapi.State) {
		team(s)
		pluginCatalog(s)
	})
	en := locales.In("en")
	var op api.PluginOperation
	p.Uninstall("telegram", func(o api.PluginOperation, err error) {
		if err != nil {
			t.Errorf("Uninstall: %v", err)
		}
		op = o
	})
	if !p.Pending["telegram"] {
		t.Fatal("uninstall request not marked pending")
	}
	fix.Settle(t)
	if p.Pending["telegram"] || !p.Uninstalling("telegram") {
		t.Fatalf("pending=%v uninstalling=%v after the DELETE, want the op tracked", p.Pending, p.Uninstalling("telegram"))
	}
	fake.FinishOperation(op.ID, true, "")
	tick(t, fix, fake, time.Second, opsPath)
	awaitToasts(t, fix, st, 1)
	if got := lastToast(t, st); got.Title != en.PluginsUninstallDone("Telegram") {
		t.Fatalf("toast = %+v, want uninstall done", got)
	}
	fix.Await(t, func() bool { return p.List.Loaded && len(p.List.Data) == 0 })
	if p.Uninstalling("telegram") {
		t.Fatal("still uninstalling after the op finished")
	}
}

func TestPluginsPollWhileStarting(t *testing.T) {
	p, fake, fix, _ := activePlugins(t, withStartingPlugin)
	detailPath := pluginsPath + "/telegram"
	d := p.Detail("telegram")
	if p.Detail("telegram") != d {
		t.Fatal("Detail returned a new resource for the same id")
	}
	d.Ensure()
	fix.Settle(t)
	unchanged(t, fix, fake, time.Second, pluginsPath, detailPath)
	fix.Clock.Advance(time.Second)
	fix.Await(t, func() bool {
		return countCalls(fake, http.MethodGet, pluginsPath) == 2 && countCalls(fake, http.MethodGet, detailPath) == 2
	})
	fake.Update(func(s *fakeapi.State) { s.Plugins[0].State = "running" })
	fix.Clock.Advance(2 * time.Second)
	fix.Await(t, func() bool {
		return countCalls(fake, http.MethodGet, pluginsPath) == 3 && countCalls(fake, http.MethodGet, detailPath) == 3
	})
	if p.List.Data[0].State != "running" || d.Data.State != "running" {
		t.Fatalf("states list=%q detail=%q, want running", p.List.Data[0].State, d.Data.State)
	}
	quiet(t, fix, fake, 6*time.Second, pluginsPath, detailPath)
}

func TestToggleIsOptimisticAndToasts(t *testing.T) {
	p, fake, fix, st := activePlugins(t, func(s *fakeapi.State) {
		team(s)
		pluginCatalog(s)
	})
	en := locales.In("en")
	err := call(t, fix, func(done func(error)) {
		p.Toggle("telegram", false, done)
		if !p.Pending["telegram"] || p.List.Data[0].Enabled {
			t.Errorf("toggle off: pending=%v enabled=%v, want pending and shown off", p.Pending, p.List.Data[0].Enabled)
		}
	})
	if err != nil {
		t.Fatalf("Toggle: %v", err)
	}
	fix.Settle(t)
	if countCalls(fake, http.MethodPost, pluginsPath+"/telegram/disable") != 1 {
		t.Fatal("disable not posted")
	}
	if got := lastToast(t, st); got.Tone != kit.ToneSuccess || got.Title != en.PluginsToastDisabled("Telegram") {
		t.Fatalf("toast = %+v", got)
	}
	if p.Pending["telegram"] || p.List.Data[0].State != "disabled" {
		t.Fatalf("pending=%v state=%q after the refetch", p.Pending, p.List.Data[0].State)
	}

	fake.FailNext(http.MethodPost, pluginsPath+"/telegram/restart", http.StatusBadRequest, map[string]string{"error": "plugin is disabled"})
	if err := call(t, fix, func(done func(error)) { p.Restart("telegram", done) }); err == nil {
		t.Fatal("Restart error not reported")
	}
	if got := lastToast(t, st); got.Tone != kit.ToneDanger || got.Title != "plugin is disabled" {
		t.Fatalf("toast = %+v, want the error message", got)
	}
}

func TestRefreshCatalogReplacesData(t *testing.T) {
	p, fake, fix, _ := activePlugins(t, func(s *fakeapi.State) {
		team(s)
		pluginCatalog(s)
	})
	fake.Update(func(s *fakeapi.State) { s.Catalog.Plugins = s.Catalog.Plugins[:1] })
	if err := call(t, fix, func(done func(error)) { p.RefreshCatalog(done) }); err != nil {
		t.Fatalf("RefreshCatalog: %v", err)
	}
	calls := fake.Calls(http.MethodGet, catalogPath)
	if last := calls[len(calls)-1]; last.Query != "refresh=1" {
		t.Fatalf("refresh query = %q", last.Query)
	}
	if len(p.Catalog.Data.Plugins) != 1 {
		t.Fatalf("catalog has %d plugins after refresh, want 1", len(p.Catalog.Data.Plugins))
	}
	fake.FailNext(http.MethodGet, catalogPath, http.StatusBadGateway, map[string]string{"error": "registry down"})
	if err := call(t, fix, func(done func(error)) { p.RefreshCatalog(done) }); err == nil {
		t.Fatal("refresh error not reported")
	}
	if len(p.Catalog.Data.Plugins) != 1 || p.Catalog.Err != nil {
		t.Fatalf("a failed refresh changed the cached catalog: %+v err=%v", p.Catalog.Data, p.Catalog.Err)
	}
}

func TestSaveSettingsRefetches(t *testing.T) {
	p, fake, fix, _ := activePlugins(t, func(s *fakeapi.State) {
		team(s)
		pluginCatalog(s)
	})
	d := p.Detail("telegram")
	d.Ensure()
	fix.Settle(t)
	lists, details := countCalls(fake, http.MethodGet, pluginsPath), countCalls(fake, http.MethodGet, pluginsPath+"/telegram")
	name := "openbee_bot"
	if err := call(t, fix, func(done func(error)) {
		p.SaveSettings("telegram", api.PluginSettingsUpdate{BotName: &name}, done)
	}); err != nil {
		t.Fatalf("SaveSettings: %v", err)
	}
	fix.Settle(t)
	var body map[string]json.RawMessage
	lastBody(t, fake, http.MethodPut, pluginsPath+"/telegram/settings", &body)
	if string(body["bot_name"]) != `"openbee_bot"` {
		t.Fatalf("settings body = %s", body)
	}
	if countCalls(fake, http.MethodGet, pluginsPath) != lists+1 || countCalls(fake, http.MethodGet, pluginsPath+"/telegram") != details+1 {
		t.Fatal("saving settings did not refetch the list and the detail")
	}
}

func TestPluginsResetOnSignOut(t *testing.T) {
	p, _, fix, st := activePlugins(t, withPluginOps)
	if !p.Ops.Loaded || len(p.Ops.Data) != 1 {
		t.Fatalf("ops not loaded: %+v", p.Ops.Data)
	}
	st.Logout()
	fix.Settle(t)
	if p.Ops.Loaded || p.List.Loaded || p.Catalog.Loaded || len(p.Pending) != 0 {
		t.Fatal("plugin state survived sign-out")
	}
	fix.Await(t, func() bool { return fix.Clock.Sleepers() == 0 })
}
