package store_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/theopenbee/openbee/desktop/internal/api"
	"github.com/theopenbee/openbee/desktop/internal/fakeapi"
	"github.com/theopenbee/openbee/desktop/internal/prefs"
	"github.com/theopenbee/openbee/desktop/internal/store"
	"github.com/theopenbee/openbee/desktop/internal/store/storetest"
)

func uninitialized(st *fakeapi.State) { st.Initialized = false }

func connectFresh(t *testing.T, seed func(st *fakeapi.State)) (*store.Store, *fakeapi.Server, *storetest.Fixture) {
	t.Helper()
	st, fake, fix := storetest.New(t, seed)
	st.Start()
	fix.Settle(t)
	if err := call(t, fix, func(done func(error)) { st.Connect(fake.URL, done) }); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	return st, fake, fix
}

func issueTokens(t *testing.T, fake *fakeapi.Server) prefs.Tokens {
	t.Helper()
	pair, err := api.New(api.Options{BaseURL: fake.URL}).Login(context.Background(), "admin", "secret")
	if err != nil {
		t.Fatalf("issue tokens: %v", err)
	}
	return prefs.Tokens{Access: pair.AccessToken, Refresh: pair.RefreshToken}
}

func TestConnectInvalidURL(t *testing.T) {
	st, fake, fix := storetest.New(t, nil)
	st.Start()
	fix.Settle(t)
	for _, raw := range []string{"", "   ", "ftp://bee.example.com", "http://"} {
		err := call(t, fix, func(done func(error)) { st.Connect(raw, done) })
		if !errors.Is(err, store.ErrInvalidURL) {
			t.Fatalf("Connect(%q) = %v, want ErrInvalidURL", raw, err)
		}
	}
	if st.Phase != store.PhaseConnect || st.Server != "" {
		t.Fatalf("Phase=%v Server=%q, want PhaseConnect without server", st.Phase, st.Server)
	}
	if calls := fake.Calls("", ""); len(calls) != 0 {
		t.Fatalf("network calls = %v, want none", calls)
	}
}

func TestConnectNotOpenBee(t *testing.T) {
	html := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, "<!doctype html><title>Router</title>")
	}))
	t.Cleanup(html.Close)
	st, _, fix := storetest.New(t, nil)
	st.Start()
	fix.Settle(t)

	err := call(t, fix, func(done func(error)) { st.Connect(html.URL, done) })
	if !errors.Is(err, api.ErrNotOpenBee) {
		t.Fatalf("Connect = %v, want ErrNotOpenBee", err)
	}
	if st.Phase != store.PhaseConnect || st.Server != "" {
		t.Fatalf("Phase=%v Server=%q, want PhaseConnect without server", st.Phase, st.Server)
	}
	if got := fix.Prefs.Load().ServerURL; got != "" {
		t.Fatalf("saved server = %q, want empty", got)
	}
}

func TestConnectUninitializedGoesToSetup(t *testing.T) {
	st, fake, fix := storetest.New(t, uninitialized)
	st.Start()
	fix.Settle(t)
	if err := call(t, fix, func(done func(error)) { st.Connect("  "+fake.URL+"/api/ ", done) }); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if st.Phase != store.PhaseSetup {
		t.Fatalf("Phase = %v, want PhaseSetup", st.Phase)
	}
	if st.Server != fake.URL || st.Client().BaseURL() != fake.URL {
		t.Fatalf("Server = %q, client base = %q, want %q", st.Server, st.Client().BaseURL(), fake.URL)
	}
	if got := fix.Prefs.Load().ServerURL; got != fake.URL {
		t.Fatalf("saved server = %q, want %q", got, fake.URL)
	}
	if !st.Config.Loaded {
		t.Fatal("Config not seeded from the probe")
	}
}

func TestConnectClearsPreviousServerTokens(t *testing.T) {
	st, fake, fix := storetest.New(t, nil)
	gone := httptest.NewServer(http.NotFoundHandler())
	old := gone.URL
	gone.Close()
	if err := fix.Prefs.Save(prefs.Prefs{ServerURL: old, Language: "en"}); err != nil {
		t.Fatalf("save prefs: %v", err)
	}
	if err := fix.Tokens.Save(old, prefs.Tokens{Access: "a", Refresh: "r"}); err != nil {
		t.Fatalf("save tokens: %v", err)
	}
	st.Start()
	fix.Settle(t)
	if st.Phase != store.PhaseReady {
		t.Fatalf("Phase = %v, want PhaseReady from saved tokens", st.Phase)
	}
	if err := call(t, fix, func(done func(error)) { st.Connect(fake.URL, done) }); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if st.Phase != store.PhaseLogin {
		t.Fatalf("Phase = %v, want PhaseLogin", st.Phase)
	}
	if got := savedTokens(t, fix, old); got != (prefs.Tokens{}) {
		t.Fatalf("tokens of the previous server = %+v, want cleared", got)
	}
}

func TestSetupSignsIn(t *testing.T) {
	st, fake, fix := connectFresh(t, uninitialized)
	err := call(t, fix, func(done func(error)) {
		st.Setup(api.SetupRequest{Username: "ada", Password: "secret1", DisplayName: "Ada"}, done)
	})
	if err != nil {
		t.Fatalf("Setup: %v", err)
	}
	if st.Phase != store.PhaseReady {
		t.Fatalf("Phase = %v, want PhaseReady", st.Phase)
	}
	tokens := savedTokens(t, fix, fake.URL)
	if tokens.Access == "" || tokens.Refresh == "" {
		t.Fatalf("saved tokens = %+v, want both", tokens)
	}
	if got := fix.Prefs.Load().Username; got != "ada" {
		t.Fatalf("saved username = %q, want ada", got)
	}
	if _, err := st.Client().Workers(context.Background()); err != nil {
		t.Fatalf("authenticated request after setup: %v", err)
	}
}

func TestLoginWrongPassword(t *testing.T) {
	st, fake, fix := started(t, prefs.Prefs{}, nil)
	if st.Phase != store.PhaseLogin {
		t.Fatalf("Phase = %v, want PhaseLogin", st.Phase)
	}
	err := call(t, fix, func(done func(error)) { st.Login("admin", "wrong", done) })
	if !api.IsStatus(err, http.StatusUnauthorized) {
		t.Fatalf("Login = %v, want 401", err)
	}
	if st.Phase != store.PhaseLogin {
		t.Fatalf("Phase = %v, want PhaseLogin", st.Phase)
	}
	if got := savedTokens(t, fix, fake.URL); got != (prefs.Tokens{}) {
		t.Fatalf("tokens saved after a failed login: %+v", got)
	}
}

func TestLoginSuccessPersistsTokens(t *testing.T) {
	st, fake, fix := signedIn(t, nil)
	tokens := savedTokens(t, fix, fake.URL)
	if tokens.Access == "" || tokens.Refresh == "" {
		t.Fatalf("saved tokens = %+v, want both", tokens)
	}
	if got := fix.Prefs.Load().Username; got != "admin" {
		t.Fatalf("saved username = %q, want admin", got)
	}
	if st.Prefs.Username != "admin" {
		t.Fatalf("Prefs.Username = %q, want admin", st.Prefs.Username)
	}
	r := workersResource(st)
	r.Ensure()
	fix.Settle(t)
	if r.Err != nil || !r.Loaded {
		t.Fatalf("authenticated resource Err=%v Loaded=%v", r.Err, r.Loaded)
	}
}

func TestStartWithTokensIsReady(t *testing.T) {
	st, fake, fix := storetest.New(t, nil)
	if err := fix.Prefs.Save(prefs.Prefs{ServerURL: fake.URL, Username: "admin", Language: "en"}); err != nil {
		t.Fatalf("save prefs: %v", err)
	}
	if err := fix.Tokens.Save(fake.URL, issueTokens(t, fake)); err != nil {
		t.Fatalf("save tokens: %v", err)
	}
	st.Start()
	fix.Settle(t)
	if st.Phase != store.PhaseReady {
		t.Fatalf("Phase = %v, want PhaseReady", st.Phase)
	}
	if calls := fake.Calls("GET", "/api/setup/status"); len(calls) != 0 {
		t.Fatalf("setup status calls = %d, want 0", len(calls))
	}
	for _, c := range fake.Calls("GET", "") {
		if c.Path != "/api/local/stream" {
			t.Fatalf("GET %s made at start, want only the chat stream", c.Path)
		}
	}
	if _, err := st.Client().Workers(context.Background()); err != nil {
		t.Fatalf("request with loaded tokens: %v", err)
	}
}

func TestUnauthorizedReturnsToLoginOnce(t *testing.T) {
	st, fake, fix := signedIn(t, nil)
	gen := store.Generation(st)
	fake.RevokeRefreshTokens()
	fake.ExpireAccessTokens()

	resources := []interface {
		Ensure()
	}{
		workersResource(st),
		store.NewResource(st, func(ctx context.Context, c *api.Client) ([]api.ConversationSummary, error) {
			return c.Conversations(ctx)
		}),
		store.NewResource(st, func(ctx context.Context, c *api.Client) (map[string]string, error) {
			return c.SystemConfigs(ctx)
		}),
	}
	for _, r := range resources {
		r.Ensure()
	}
	fix.Settle(t)

	if st.Phase != store.PhaseLogin {
		t.Fatalf("Phase = %v, want PhaseLogin", st.Phase)
	}
	if got := store.Generation(st); got != gen+1 {
		t.Fatalf("session generation advanced by %d, want exactly one transition", got-gen)
	}
	if got := savedTokens(t, fix, fake.URL); got != (prefs.Tokens{}) {
		t.Fatalf("tokens after expiry = %+v, want cleared", got)
	}
	if calls := fake.Calls("POST", "/api/auth/refresh"); len(calls) == 0 {
		t.Fatal("no refresh attempt before signing out")
	}
}

func TestLoggedOutUnauthorizedDoesNotEndNextSession(t *testing.T) {
	st, _, fix := started(t, prefs.Prefs{Language: "en"}, nil)
	loggedIn := make(chan struct{})
	late := store.NewResource(st, func(_ context.Context, c *api.Client) ([]api.Worker, error) {
		<-loggedIn
		return c.Workers(context.Background())
	})
	late.Ensure()
	gen := store.Generation(st)
	err := call(t, fix, func(done func(error)) {
		st.Login("admin", "secret", func(err error) {
			done(err)
			close(loggedIn)
		})
	})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if st.Phase != store.PhaseReady {
		t.Fatalf("Phase = %v, want PhaseReady: a 401 from the signed-out session ended the new one", st.Phase)
	}
	if got := store.Generation(st); got != gen+1 {
		t.Fatalf("session generation advanced by %d, want 1", got-gen)
	}
}

func TestRefreshPersistsAccessToken(t *testing.T) {
	st, fake, fix := signedIn(t, nil)
	before := savedTokens(t, fix, fake.URL)
	fake.ExpireAccessTokens()
	r := workersResource(st)
	r.Ensure()
	fix.Settle(t)
	if r.Err != nil || st.Phase != store.PhaseReady {
		t.Fatalf("after refresh Err=%v Phase=%v", r.Err, st.Phase)
	}
	after := savedTokens(t, fix, fake.URL)
	if after.Access == before.Access || after.Refresh != before.Refresh {
		t.Fatalf("saved tokens after refresh = %+v, before %+v; want a new access and the same refresh", after, before)
	}
}

func TestLogoutKeepsUsername(t *testing.T) {
	st, fake, fix := signedIn(t, nil)
	r := workersResource(st)
	r.Ensure()
	fix.Settle(t)
	st.Config.Ensure()
	fix.Settle(t)

	st.Logout()
	fix.Settle(t)
	if st.Phase != store.PhaseLogin {
		t.Fatalf("Phase = %v, want PhaseLogin", st.Phase)
	}
	if st.Prefs.Username != "admin" || fix.Prefs.Load().Username != "admin" {
		t.Fatalf("username = %q / saved %q, want admin kept", st.Prefs.Username, fix.Prefs.Load().Username)
	}
	if got := savedTokens(t, fix, fake.URL); got != (prefs.Tokens{}) {
		t.Fatalf("tokens after logout = %+v, want cleared", got)
	}
	if r.Loaded || r.Data != nil || st.Config.Loaded {
		t.Fatalf("resources kept after logout: workers %v config %v", r.Loaded, st.Config.Loaded)
	}
	if st.Server != fake.URL || fix.Prefs.Load().ServerURL != fake.URL {
		t.Fatalf("server changed by logout: %q", st.Server)
	}
}

func TestSessionResetHooks(t *testing.T) {
	st, _, fix := signedIn(t, nil)
	r := workersResource(st)
	r.Ensure()
	fix.Settle(t)
	if !r.Loaded {
		t.Fatal("workers not loaded before the reset")
	}
	resets, loadedInHook := 0, true
	off := st.OnSessionReset(func() {
		resets++
		loadedInHook = r.Loaded
	})

	st.Logout()
	fix.Settle(t)
	if resets != 1 || loadedInHook {
		t.Fatalf("after logout: hook ran %d times, resources still loaded in hook %v; want 1 and false", resets, loadedInHook)
	}
	if err := call(t, fix, func(done func(error)) { st.Login("admin", "secret", done) }); err != nil {
		t.Fatalf("Login: %v", err)
	}
	if resets != 2 {
		t.Fatalf("after login: hook ran %d times, want 2", resets)
	}
	off()
	st.ChangeServer()
	fix.Settle(t)
	if resets != 2 {
		t.Fatalf("hook ran %d times after being removed, want 2", resets)
	}
}

func TestChangeServerClearsTokens(t *testing.T) {
	st, fake, fix := signedIn(t, nil)
	st.ChangeServer()
	fix.Settle(t)
	if st.Phase != store.PhaseConnect {
		t.Fatalf("Phase = %v, want PhaseConnect", st.Phase)
	}
	if st.Server != "" || fix.Prefs.Load().ServerURL != "" {
		t.Fatalf("server = %q / saved %q, want cleared", st.Server, fix.Prefs.Load().ServerURL)
	}
	if got := savedTokens(t, fix, fake.URL); got != (prefs.Tokens{}) {
		t.Fatalf("tokens after change server = %+v, want cleared", got)
	}
	if got := fix.Prefs.Load().Username; got != "admin" {
		t.Fatalf("saved username = %q, want admin kept", got)
	}
}

func TestSessionChangeDropsResults(t *testing.T) {
	st, _, fix := signedIn(t, nil)
	r := workersResource(st)
	r.Ensure()
	var loginErr error
	loginDone := false
	st.Logout()
	st.Login("admin", "secret", func(err error) {
		loginErr = err
		loginDone = true
	})
	st.ChangeServer()
	fix.Settle(t)
	if r.Loaded || r.Loading {
		t.Fatalf("result of the old session applied: Loaded=%v Loading=%v", r.Loaded, r.Loading)
	}
	if !loginDone || !errors.Is(loginErr, context.Canceled) {
		t.Fatalf("login done=%v err=%v, want context.Canceled", loginDone, loginErr)
	}
	if st.Phase != store.PhaseConnect {
		t.Fatalf("Phase = %v, want PhaseConnect", st.Phase)
	}
}

func TestChangeServerWhileBootingDropsTokenLoad(t *testing.T) {
	st, fake, fix := storetest.New(t, nil)
	if err := fix.Prefs.Save(prefs.Prefs{ServerURL: fake.URL, Language: "en"}); err != nil {
		t.Fatalf("save prefs: %v", err)
	}
	if err := fix.Tokens.Save(fake.URL, issueTokens(t, fake)); err != nil {
		t.Fatalf("save tokens: %v", err)
	}
	st.Start()
	if st.Phase != store.PhaseBoot {
		t.Fatalf("Phase = %v, want PhaseBoot while tokens load", st.Phase)
	}
	st.ChangeServer()
	fix.Settle(t)
	if st.Phase != store.PhaseConnect {
		t.Fatalf("Phase = %v, want PhaseConnect: the token load of the old session was applied", st.Phase)
	}
}

func TestChangePassword(t *testing.T) {
	st, _, fix := signedIn(t, nil)
	err := call(t, fix, func(done func(error)) { st.ChangePassword("nope", "secret2", done) })
	var apiErr *api.Error
	if !errors.As(err, &apiErr) || apiErr.Code != "old_password_incorrect" {
		t.Fatalf("ChangePassword with a wrong password = %v", err)
	}
	if st.Phase != store.PhaseReady {
		t.Fatalf("Phase = %v, want PhaseReady", st.Phase)
	}
	if err := call(t, fix, func(done func(error)) { st.ChangePassword("secret", "secret2", done) }); err != nil {
		t.Fatalf("ChangePassword: %v", err)
	}
}
