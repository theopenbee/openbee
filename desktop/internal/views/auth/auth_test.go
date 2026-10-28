package auth_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/fakeapi"
	"github.com/theopenbee/openbee/desktop/internal/store"
	"github.com/theopenbee/openbee/desktop/internal/views/testkit"
	"github.com/theopenbee/openbee/desktop/locales"
)

const (
	width  = 1280
	height = 800
)

func uninitialized(st *fakeapi.State) { st.Initialized = false }

func newHarness(t *testing.T, seed func(st *fakeapi.State)) *testkit.Harness {
	t.Helper()
	return testkit.New(t, width, height, seed)
}

func connected(t *testing.T, seed func(st *fakeapi.State)) *testkit.Harness {
	t.Helper()
	h := newHarness(t, seed)
	h.Connect()
	return h
}

func click(t *testing.T, h *testkit.Harness, name string) {
	t.Helper()
	if err := h.T.Click(name); err != nil {
		t.Fatalf("click %q: %v (texts %q)", name, err, h.T.Texts())
	}
	h.Settle()
}

func fill(t *testing.T, h *testkit.Harness, label, text string) {
	t.Helper()
	if err := h.T.Click(label); err != nil {
		t.Fatalf("focus %q: %v", label, err)
	}
	h.T.Command("selectAll")
	h.T.Type(text)
}

func value(t *testing.T, h *testkit.Harness, label string) string {
	t.Helper()
	if err := h.T.Click(label); err != nil {
		t.Fatalf("focus %q: %v", label, err)
	}
	h.T.Command("selectAll")
	h.T.Command("copy")
	return h.T.Clipboard()
}

func pressSubmit(t *testing.T, h *testkit.Harness, name string) {
	t.Helper()
	for range 4 {
		if h.T.Focused(name) {
			h.T.Key(0, ui.KeySpace)
			h.Settle()
			return
		}
		h.T.Key(0, ui.KeyTab)
	}
	t.Fatalf("Tab never reached the %q button", name)
}

func wantText(t *testing.T, h *testkit.Harness, text string) {
	t.Helper()
	if !h.T.HasText(text) {
		t.Fatalf("screen does not show %q; texts %q", text, h.T.Texts())
	}
}

func wantNoText(t *testing.T, h *testkit.Harness, text string) {
	t.Helper()
	if h.T.HasText(text) {
		t.Fatalf("screen shows %q", text)
	}
}

func TestConnectScreenDefaultsLocalhost(t *testing.T) {
	h := newHarness(t, nil)
	wantText(t, h, locales.ConnectTitle())
	if !h.T.Focused(locales.ConnectServerLabel()) {
		t.Error("server address input does not have the focus")
	}
	if got := value(t, h, locales.ConnectServerLabel()); got != "http://localhost:8080" {
		t.Fatalf("server address = %q, want http://localhost:8080", got)
	}
}

func TestConnectShowsUnreachable(t *testing.T) {
	gone := httptest.NewServer(http.NotFoundHandler())
	url := gone.URL
	gone.Close()
	h := newHarness(t, nil)
	fill(t, h, locales.ConnectServerLabel(), url)
	click(t, h, locales.ConnectSubmit())
	wantText(t, h, locales.ConnectErrorUnreachable(url))
	if h.Store.Phase != store.PhaseConnect {
		t.Fatalf("Phase = %v, want PhaseConnect", h.Store.Phase)
	}
	fill(t, h, locales.ConnectServerLabel(), "http://")
	wantNoText(t, h, locales.ConnectErrorUnreachable(url))
	h.T.Key(0, ui.KeyEnter)
	h.Settle()
	wantText(t, h, locales.ConnectErrorInvalid())
}

func TestConnectShowsNotOpenBee(t *testing.T) {
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<!doctype html><title>Router</title>"))
	}))
	t.Cleanup(other.Close)
	h := newHarness(t, nil)
	fill(t, h, locales.ConnectServerLabel(), other.URL)
	click(t, h, locales.ConnectSubmit())
	wantText(t, h, locales.ConnectErrorNotOpenBee(other.URL))
}

func TestConnectThenLogin(t *testing.T) {
	h := newHarness(t, nil)
	fill(t, h, locales.ConnectServerLabel(), h.Fake.URL)
	click(t, h, locales.ConnectSubmit())
	if h.Store.Phase != store.PhaseLogin {
		t.Fatalf("Phase = %v, want PhaseLogin", h.Store.Phase)
	}
	wantText(t, h, locales.LoginTitle())
	wantText(t, h, locales.LoginServer(h.Fake.URL))
	if got := h.Root.Nav().Path(); got != "/login" {
		t.Fatalf("path = %q, want /login", got)
	}
}

func TestConnectThenSetup(t *testing.T) {
	h := newHarness(t, uninitialized)
	fill(t, h, locales.ConnectServerLabel(), h.Fake.URL)
	h.T.Key(0, ui.KeyEnter)
	h.Settle()
	wantText(t, h, locales.SetupTitle())
	if got := h.Root.Nav().Path(); got != "/setup" {
		t.Fatalf("path = %q, want /setup", got)
	}
	if !h.T.Focused(locales.SetupUsername()) {
		t.Error("username input does not have the focus")
	}
}

func TestLoginValidation(t *testing.T) {
	h := connected(t, nil)
	h.T.Key(0, ui.KeyEnter)
	h.Settle()
	wantText(t, h, locales.LoginUsernameRequired())
	wantText(t, h, locales.LoginPasswordRequired())
	if !h.T.Focused(locales.LoginUsername()) {
		t.Error("focus did not move to the first invalid field")
	}
	if calls := h.Fake.Calls(http.MethodPost, "/api/auth/login"); len(calls) != 0 {
		t.Fatalf("login requests = %d, want none", len(calls))
	}
	h.T.Type("admin")
	h.Settle()
	wantNoText(t, h, locales.LoginUsernameRequired())
	wantText(t, h, locales.LoginPasswordRequired())
	h.T.Key(0, ui.KeyEnter)
	h.Settle()
	if !h.T.Focused(locales.LoginPassword()) {
		t.Error("focus did not move to the password field")
	}
}

func TestLogin401ShowsAlert(t *testing.T) {
	h := connected(t, nil)
	fill(t, h, locales.LoginUsername(), "admin")
	fill(t, h, locales.LoginPassword(), "wrong")
	h.T.Key(0, ui.KeyEnter)
	h.Settle()
	wantText(t, h, locales.LoginErrorTitle())
	wantText(t, h, locales.LoginError401())
	if h.Store.Phase != store.PhaseLogin {
		t.Fatalf("Phase = %v, want PhaseLogin", h.Store.Phase)
	}
	if !h.T.Focused(locales.LoginPassword()) {
		t.Error("password field does not have the focus after a 401")
	}
}

func TestLogin429ShowsAlert(t *testing.T) {
	h := connected(t, nil)
	h.Fake.FailNext(http.MethodPost, "/api/auth/login", http.StatusTooManyRequests, map[string]string{"error": "slow down"})
	fill(t, h, locales.LoginUsername(), "admin")
	fill(t, h, locales.LoginPassword(), "secret")
	h.T.Key(0, ui.KeyEnter)
	h.Settle()
	wantText(t, h, locales.LoginError429())
}

func TestLoginPrefillFocusesPassword(t *testing.T) {
	h := testkit.SignedIn(t, width, height, nil)
	h.Store.Logout()
	h.Settle()
	wantText(t, h, locales.LoginTitle())
	if !h.T.Focused(locales.LoginPassword()) {
		t.Error("password field does not have the focus with a remembered username")
	}
	if got := value(t, h, locales.LoginUsername()); got != "admin" {
		t.Fatalf("username = %q, want the remembered admin", got)
	}
}

func TestLoginSucceedsToChat(t *testing.T) {
	h := connected(t, nil)
	fill(t, h, locales.LoginUsername(), "admin")
	fill(t, h, locales.LoginPassword(), "secret")
	pressSubmit(t, h, locales.LoginSubmit())
	if h.Store.Phase != store.PhaseReady {
		t.Fatalf("Phase = %v, want PhaseReady", h.Store.Phase)
	}
	wantText(t, h, locales.ChatBeeLabel())
	if got := h.Root.Nav().Path(); got != "/chat/bee" {
		t.Fatalf("path = %q, want /chat/bee on a wide window", got)
	}
}

func TestSetupValidation(t *testing.T) {
	h := connected(t, uninitialized)
	fill(t, h, locales.SetupUsername(), "ada")
	fill(t, h, locales.SetupPassword(), "12345")
	click(t, h, locales.SetupSubmit())
	wantText(t, h, locales.SetupErrorPasswordLength())
	wantNoText(t, h, locales.SetupPasswordHint())
	if calls := h.Fake.Calls(http.MethodPost, "/api/setup"); len(calls) != 0 {
		t.Fatalf("setup requests = %d, want none", len(calls))
	}
	fill(t, h, locales.SetupUsername(), "  ")
	click(t, h, locales.SetupSubmit())
	wantText(t, h, locales.SetupUsernameRequired())
}

func TestSetupSignsIn(t *testing.T) {
	h := connected(t, uninitialized)
	fill(t, h, locales.SetupUsername(), " ada ")
	fill(t, h, locales.SetupDisplayName(), "Ada")
	fill(t, h, locales.SetupPassword(), "secret1")
	click(t, h, locales.SetupSubmit())
	if h.Store.Phase != store.PhaseReady {
		t.Fatalf("Phase = %v, want PhaseReady", h.Store.Phase)
	}
	if snap := h.Fake.Snapshot(); snap.Username != "ada" {
		t.Fatalf("server username = %q, want ada", snap.Username)
	}
	wantText(t, h, locales.ChatBeeLabel())
}

func TestSetup409OffersLogin(t *testing.T) {
	h := connected(t, uninitialized)
	h.Fake.Update(func(st *fakeapi.State) { st.Initialized = true })
	fill(t, h, locales.SetupUsername(), "ada")
	fill(t, h, locales.SetupPassword(), "secret1")
	click(t, h, locales.SetupSubmit())
	wantText(t, h, locales.SetupErrorConflict())
	click(t, h, locales.SetupGoToLogin())
	wantText(t, h, locales.LoginTitle())
	if got := h.Root.Nav().Path(); got != "/login" {
		t.Fatalf("path = %q, want /login", got)
	}
}

func TestChangeServerFromLogin(t *testing.T) {
	h := connected(t, nil)
	click(t, h, locales.LoginChangeServer())
	if h.Store.Phase != store.PhaseConnect {
		t.Fatalf("Phase = %v, want PhaseConnect", h.Store.Phase)
	}
	wantText(t, h, locales.ConnectTitle())
	if got := h.Fix.Prefs.Load().ServerURL; got != "" {
		t.Fatalf("saved server = %q, want empty", got)
	}
}

func TestChangeServerFromSetup(t *testing.T) {
	h := connected(t, uninitialized)
	wantText(t, h, locales.SetupTitle())
	wantText(t, h, locales.LoginServer(h.Fake.URL))
	click(t, h, locales.LoginChangeServer())
	if h.Store.Phase != store.PhaseConnect {
		t.Fatalf("Phase = %v, want PhaseConnect", h.Store.Phase)
	}
	wantText(t, h, locales.ConnectTitle())
	if got := h.Fix.Prefs.Load().ServerURL; got != "" {
		t.Fatalf("saved server = %q, want empty", got)
	}
}

func TestLanguageMenuSwitchesToChinese(t *testing.T) {
	h := connected(t, nil)
	wantText(t, h, "Sign in")
	click(t, h, locales.ArcUserMenuLanguage())
	click(t, h, "简体中文")
	wantText(t, h, "登录")
	if got := h.Store.Prefs.Language; got != "zh" {
		t.Fatalf("saved language = %q, want zh", got)
	}
	if got := h.Fix.Locales[len(h.Fix.Locales)-1]; got != "zh" {
		t.Fatalf("last SetLocale = %q, want zh", got)
	}
}
