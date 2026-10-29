package store_test

import (
	"context"
	"net/http"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/theopenbee/openbee/desktop/internal/api"
	"github.com/theopenbee/openbee/desktop/internal/fakeapi"
	"github.com/theopenbee/openbee/desktop/internal/prefs"
	"github.com/theopenbee/openbee/desktop/internal/store"
	"github.com/theopenbee/openbee/desktop/internal/store/storetest"
)

func call(t *testing.T, fix *storetest.Fixture, action func(done func(error))) error {
	t.Helper()
	var got error
	calls := 0
	action(func(err error) {
		got = err
		calls++
	})
	fix.Settle(t)
	if calls != 1 {
		t.Fatalf("done called %d times, want 1", calls)
	}
	return got
}

func started(t *testing.T, p prefs.Prefs, seed func(st *fakeapi.State)) (*store.Store, *fakeapi.Server, *storetest.Fixture) {
	t.Helper()
	st, fake, fix := storetest.New(t, seed)
	p.ServerURL = fake.URL
	if err := fix.Prefs.Save(p); err != nil {
		t.Fatalf("save prefs: %v", err)
	}
	st.Start()
	fix.Settle(t)
	return st, fake, fix
}

func signedIn(t *testing.T, seed func(st *fakeapi.State)) (*store.Store, *fakeapi.Server, *storetest.Fixture) {
	t.Helper()
	st, fake, fix := started(t, prefs.Prefs{Language: "en"}, seed)
	if err := call(t, fix, func(done func(error)) { st.Login("admin", "secret", done) }); err != nil {
		t.Fatalf("Login: %v", err)
	}
	if st.Phase != store.PhaseReady {
		t.Fatalf("Phase after login = %v, want PhaseReady", st.Phase)
	}
	return st, fake, fix
}

func savedTokens(t *testing.T, fix *storetest.Fixture, server string) prefs.Tokens {
	t.Helper()
	tokens, err := fix.Tokens.Load(server)
	if err != nil {
		t.Fatalf("load tokens: %v", err)
	}
	return tokens
}

func workersResource(st *store.Store) *store.Resource[[]api.Worker] {
	return store.NewResource(st, func(ctx context.Context, c *api.Client) ([]api.Worker, error) {
		return c.Workers(ctx)
	})
}

func TestStartWithoutServerGoesToConnect(t *testing.T) {
	st, fake, fix := storetest.New(t, nil)
	if st.Phase != store.PhaseBoot {
		t.Fatalf("Phase before Start = %v, want PhaseBoot", st.Phase)
	}
	st.Start()
	fix.Settle(t)
	if st.Phase != store.PhaseConnect {
		t.Fatalf("Phase = %v, want PhaseConnect", st.Phase)
	}
	if st.Server != "" {
		t.Fatalf("Server = %q, want empty", st.Server)
	}
	if !slices.Equal(fix.Locales, []string{"en"}) {
		t.Fatalf("SetLocale calls = %v, want [en]", fix.Locales)
	}
	if calls := fake.Calls("", ""); len(calls) != 0 {
		t.Fatalf("network calls = %v, want none", calls)
	}
}

func TestThemeDefaultsToDark(t *testing.T) {
	st, _, fix := storetest.New(t, nil)
	st.Start()
	fix.Settle(t)
	if !slices.Equal(fix.Themes, []string{"dark"}) {
		t.Fatalf("SetThemeSource calls = %v, want [dark]", fix.Themes)
	}
	if got := st.ThemePref(); got != "dark" {
		t.Fatalf("ThemePref = %q, want dark", got)
	}

	st.SetTheme("light")
	fix.Settle(t)
	if got := st.ThemePref(); got != "light" {
		t.Fatalf("ThemePref after SetTheme(light) = %q", got)
	}
	if got := fix.Prefs.Load().Theme; got != "light" {
		t.Fatalf("saved theme = %q, want light", got)
	}

	st.SetTheme("sepia")
	fix.Settle(t)
	if !slices.Equal(fix.Themes, []string{"dark", "light", "dark"}) {
		t.Fatalf("SetThemeSource calls = %v, want [dark light dark]", fix.Themes)
	}

	saved, _, savedFix := storetest.New(t, nil)
	if err := savedFix.Prefs.Save(prefs.Prefs{Theme: "system"}); err != nil {
		t.Fatalf("save prefs: %v", err)
	}
	saved.Start()
	if !slices.Equal(savedFix.Themes, []string{"system"}) {
		t.Fatalf("SetThemeSource with saved system = %v, want [system]", savedFix.Themes)
	}
}

func TestLanguageFromServerWhenUnset(t *testing.T) {
	zh := func(st *fakeapi.State) { st.Language = "zh-CN" }

	_, _, fix := started(t, prefs.Prefs{}, zh)
	if !slices.Equal(fix.Locales, []string{"zh"}) {
		t.Fatalf("SetLocale calls on start = %v, want [zh]", fix.Locales)
	}

	st, fake, connectFix := storetest.New(t, zh)
	st.Start()
	fix = connectFix
	if err := call(t, fix, func(done func(error)) { st.Connect(fake.URL, done) }); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if !slices.Equal(fix.Locales, []string{"en", "zh"}) {
		t.Fatalf("SetLocale calls after connect = %v, want [en zh]", fix.Locales)
	}
	if got := fix.Prefs.Load().Language; got != "" {
		t.Fatalf("saved language = %q, want unset", got)
	}
}

func TestSavedLanguageWins(t *testing.T) {
	st, fake, fix := started(t, prefs.Prefs{Language: "en_US"}, func(st *fakeapi.State) { st.Language = "zh" })
	if !slices.Equal(fix.Locales, []string{"en"}) {
		t.Fatalf("SetLocale calls = %v, want [en]", fix.Locales)
	}
	if calls := fake.Calls("GET", "/api/config"); len(calls) != 0 {
		t.Fatalf("config calls = %d, want 0 with a saved language", len(calls))
	}

	st.SetLanguage("ZH-Hans")
	fix.Settle(t)
	if !slices.Equal(fix.Locales, []string{"en", "zh"}) {
		t.Fatalf("SetLocale calls after SetLanguage = %v, want [en zh]", fix.Locales)
	}
	if got := fix.Prefs.Load().Language; got != "zh" {
		t.Fatalf("saved language = %q, want zh", got)
	}

	st.SetLanguage("fr")
	fix.Settle(t)
	if got := fix.Locales[len(fix.Locales)-1]; got != "en" {
		t.Fatalf("SetLocale after SetLanguage(fr) = %q, want en", got)
	}
}

func TestResourceDedupesInflight(t *testing.T) {
	st, fake, fix := signedIn(t, nil)
	r := workersResource(st)

	r.Ensure()
	r.Ensure()
	if !r.Loading {
		t.Fatal("Loading = false right after Ensure")
	}
	fix.Settle(t)
	if !r.Loaded || r.Loading || r.Err != nil {
		t.Fatalf("after settle Loaded=%v Loading=%v Err=%v", r.Loaded, r.Loading, r.Err)
	}
	if r.FetchedAt.IsZero() {
		t.Fatal("FetchedAt not set")
	}
	r.Ensure()
	fix.Settle(t)
	if calls := fake.Calls("GET", "/api/workers"); len(calls) != 1 {
		t.Fatalf("workers calls = %d, want 1", len(calls))
	}
}

func TestResourceInvalidateKeepsData(t *testing.T) {
	st, _, fix := signedIn(t, nil)
	var mu sync.Mutex
	calls, inflight, peak := 0, 0, 0
	release := make(chan struct{})
	r := store.NewResource(st, func(ctx context.Context, _ *api.Client) (int, error) {
		mu.Lock()
		calls++
		n := calls
		inflight++
		peak = max(peak, inflight)
		mu.Unlock()
		defer func() {
			mu.Lock()
			inflight--
			mu.Unlock()
		}()
		if n > 1 {
			<-release
		}
		return n, nil
	})
	r.Ensure()
	fix.Settle(t)
	if r.Data != 1 || !r.Loaded {
		t.Fatalf("Data=%v Loaded=%v, want first fetch", r.Data, r.Loaded)
	}
	first := r.FetchedAt

	r.Invalidate()
	if !r.Loading || !r.Loaded || r.Data != 1 {
		t.Fatalf("during refetch Loading=%v Loaded=%v Data=%v, want old data kept", r.Loading, r.Loaded, r.Data)
	}
	r.Invalidate()
	r.Invalidate()
	close(release)
	fix.Settle(t)
	if r.Loading || r.Data != 3 || r.FetchedAt.Before(first) {
		t.Fatalf("after refetch Loading=%v Data=%v", r.Loading, r.Data)
	}
	mu.Lock()
	defer mu.Unlock()
	if calls != 3 || peak != 1 {
		t.Fatalf("fetches = %d with %d in flight at once, want 3 sequential (load, refetch, one queued refetch)", calls, peak)
	}
}

func TestResourceErrorWaitsForInvalidate(t *testing.T) {
	st, fake, fix := signedIn(t, nil)
	fake.FailNext("GET", "/api/workers", http.StatusInternalServerError, map[string]any{"error": "boom"})
	r := workersResource(st)
	r.Ensure()
	fix.Settle(t)
	if r.Err == nil || r.Loaded {
		t.Fatalf("after failure Err=%v Loaded=%v", r.Err, r.Loaded)
	}
	r.Ensure()
	fix.Settle(t)
	if calls := fake.Calls("GET", "/api/workers"); len(calls) != 1 {
		t.Fatalf("Ensure retried an errored resource: %d calls", len(calls))
	}
	r.Invalidate()
	fix.Settle(t)
	if r.Err != nil || !r.Loaded {
		t.Fatalf("after Invalidate Err=%v Loaded=%v", r.Err, r.Loaded)
	}
}

func TestResourceSetAndReset(t *testing.T) {
	st, fake, fix := signedIn(t, nil)
	r := workersResource(st)
	if !r.Stale(0) {
		t.Fatal("unloaded resource is not stale")
	}
	r.Ensure()
	r.Set([]api.Worker{{ID: "local"}})
	fix.Settle(t)
	if len(r.Data) != 1 || r.Data[0].ID != "local" || !r.Loaded {
		t.Fatalf("in-flight result overwrote Set: %v", r.Data)
	}
	if r.Stale(1 << 40) {
		t.Fatal("freshly set resource is stale")
	}
	r.Invalidate()
	r.Reset()
	fix.Settle(t)
	if r.Loaded || r.Loading || r.Data != nil || !r.FetchedAt.IsZero() {
		t.Fatalf("after Reset Loaded=%v Loading=%v Data=%v", r.Loaded, r.Loading, r.Data)
	}
	if calls := fake.Calls("GET", "/api/workers"); len(calls) != 2 {
		t.Fatalf("workers calls = %d, want 2", len(calls))
	}
}

type gatedPrefs struct {
	prefs.Store
	gate chan struct{}
}

func (g gatedPrefs) Save(p prefs.Prefs) error {
	<-g.gate
	return g.Store.Save(p)
}

func TestFlushWaitsForPendingWrites(t *testing.T) {
	saved := gatedPrefs{Store: prefs.NewMemoryStore(prefs.Prefs{}), gate: make(chan struct{})}
	st := store.New(store.Options{Post: func(fn func()) { fn() }, Prefs: saved, Tokens: prefs.NewMemoryTokenStore()})
	if !st.Flush(0) {
		t.Fatal("Flush with nothing queued = false, want true")
	}

	st.SetTheme("light")
	st.SetLanguage("zh")
	if st.Flush(10 * time.Millisecond) {
		t.Fatal("Flush = true while a write was blocked")
	}

	close(saved.gate)
	if !st.Flush(time.Second) {
		t.Fatal("Flush timed out after the writes were released")
	}
	if got := saved.Load(); got.Theme != "light" || got.Language != "zh" {
		t.Fatalf("saved = %+v, want both writes applied in order", got)
	}
}
