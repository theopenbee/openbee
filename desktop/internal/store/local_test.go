package store_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/theopenbee/openbee/desktop/internal/fakeapi"
	"github.com/theopenbee/openbee/desktop/internal/prefs"
	"github.com/theopenbee/openbee/desktop/internal/store"
	"github.com/theopenbee/openbee/desktop/internal/store/storetest"
)

type localStarter struct {
	calls atomic.Int32
	err   atomic.Pointer[error]
}

func (l *localStarter) start(context.Context) error {
	l.calls.Add(1)
	if err := l.err.Load(); err != nil {
		return *err
	}
	return nil
}

func (l *localStarter) fail(err error) {
	l.err.Store(&err)
}

func (l *localStarter) succeed() {
	l.err.Store(nil)
}

func unreachableURL(t *testing.T) string {
	t.Helper()
	gone := httptest.NewServer(http.NotFoundHandler())
	url := gone.URL
	gone.Close()
	return url
}

func TestStartBundledGoesToSetup(t *testing.T) {
	starter := &localStarter{}
	fake := fakeapi.New(t)
	fake.Update(uninitialized)
	st, _, fix := storetest.New(t, nil, func(o *store.Options) {
		o.LocalServer = fake.URL
		o.StartLocal = starter.start
	})
	st.Start()
	if st.Phase != store.PhaseBoot || !st.StartingLocal {
		t.Fatalf("Phase=%v StartingLocal=%v right after Start, want PhaseBoot while starting", st.Phase, st.StartingLocal)
	}
	fix.Settle(t)
	if st.Phase != store.PhaseSetup {
		t.Fatalf("Phase = %v, want PhaseSetup", st.Phase)
	}
	if st.StartingLocal {
		t.Fatal("StartingLocal still set after the server started")
	}
	if st.Server != fake.URL {
		t.Fatalf("Server = %q, want the local server %q", st.Server, fake.URL)
	}
	if n := starter.calls.Load(); n != 1 {
		t.Fatalf("StartLocal called %d times, want 1", n)
	}
}

func TestStartBundledWithTokensSignsIn(t *testing.T) {
	starter := &localStarter{}
	fake := fakeapi.New(t)
	st, _, fix := storetest.New(t, nil, func(o *store.Options) {
		o.LocalServer = fake.URL
		o.StartLocal = starter.start
	})
	if err := fix.Tokens.Save(fake.URL, issueTokens(t, fake)); err != nil {
		t.Fatalf("save tokens: %v", err)
	}
	st.Start()
	fix.Settle(t)
	if st.Phase != store.PhaseReady {
		t.Fatalf("Phase = %v, want PhaseReady", st.Phase)
	}
	if n := starter.calls.Load(); n != 2 {
		t.Fatalf("StartLocal called %d times, want 2: before sign-in and before the event stream", n)
	}
}

func TestStreamRetryStartsLocalServer(t *testing.T) {
	starter := &localStarter{}
	fake := fakeapi.New(t)
	st, _, fix := storetest.New(t, nil, func(o *store.Options) {
		o.LocalServer = fake.URL
		o.StartLocal = starter.start
	})
	if err := fix.Tokens.Save(fake.URL, issueTokens(t, fake)); err != nil {
		t.Fatalf("save tokens: %v", err)
	}
	st.Start()
	fix.Settle(t)
	before := starter.calls.Load()
	st.Reconnect()
	fix.Await(t, func() bool { return starter.calls.Load() > before && st.Online })
}

func TestChosenLanguageAppliesWhileLocalServerStarts(t *testing.T) {
	release := make(chan struct{})
	fake := fakeapi.New(t)
	st, _, fix := storetest.New(t, nil, func(o *store.Options) {
		o.LocalServer = fake.URL
		o.StartLocal = func(context.Context) error {
			<-release
			return nil
		}
	})
	if err := fix.Prefs.Save(prefs.Prefs{Language: "zh"}); err != nil {
		t.Fatalf("save prefs: %v", err)
	}
	st.Start()
	if len(fix.Locales) == 0 || fix.Locales[len(fix.Locales)-1] != "zh" {
		t.Fatalf("locales applied before the server started = %q, want zh", fix.Locales)
	}
	close(release)
	fix.Settle(t)
}

func TestLogoutOnFreshServerOffersSetup(t *testing.T) {
	starter := &localStarter{}
	fake := fakeapi.New(t)
	st, _, fix := storetest.New(t, nil, func(o *store.Options) {
		o.LocalServer = fake.URL
		o.StartLocal = starter.start
	})
	if err := fix.Tokens.Save(fake.URL, issueTokens(t, fake)); err != nil {
		t.Fatalf("save tokens: %v", err)
	}
	st.Start()
	fix.Settle(t)
	fake.Update(uninitialized)
	st.Logout()
	fix.Settle(t)
	if st.Phase != store.PhaseSetup {
		t.Fatalf("Phase = %v after signing out of a server without accounts, want PhaseSetup", st.Phase)
	}
}

func TestStartBundledFailureShowsConnect(t *testing.T) {
	starter := &localStarter{}
	boom := errors.New("address already in use")
	starter.fail(boom)
	fake := fakeapi.New(t)
	fake.Update(uninitialized)
	st, _, fix := storetest.New(t, nil, func(o *store.Options) {
		o.LocalServer = fake.URL
		o.StartLocal = starter.start
	})
	st.Start()
	fix.Settle(t)
	if st.Phase != store.PhaseConnect {
		t.Fatalf("Phase = %v, want PhaseConnect", st.Phase)
	}
	var local *store.LocalServerError
	if !errors.As(st.LocalErr, &local) || !errors.Is(st.LocalErr, boom) {
		t.Fatalf("LocalErr = %v, want a LocalServerError wrapping %v", st.LocalErr, boom)
	}

	starter.succeed()
	if err := call(t, fix, func(done func(error)) { st.Connect(fake.URL, done) }); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if st.Phase != store.PhaseSetup || st.LocalErr != nil {
		t.Fatalf("Phase=%v LocalErr=%v after retry, want PhaseSetup without error", st.Phase, st.LocalErr)
	}
	if n := starter.calls.Load(); n != 2 {
		t.Fatalf("StartLocal called %d times, want 2", n)
	}
}

func TestRetryAfterLocalFailureKeepsSignIn(t *testing.T) {
	starter := &localStarter{}
	starter.fail(errors.New("address already in use"))
	fake := fakeapi.New(t)
	st, _, fix := storetest.New(t, nil, func(o *store.Options) {
		o.LocalServer = fake.URL
		o.StartLocal = starter.start
	})
	saved := issueTokens(t, fake)
	if err := fix.Tokens.Save(fake.URL, saved); err != nil {
		t.Fatalf("save tokens: %v", err)
	}
	st.Start()
	fix.Settle(t)
	if st.Phase != store.PhaseConnect {
		t.Fatalf("Phase = %v, want PhaseConnect", st.Phase)
	}
	starter.succeed()
	if err := call(t, fix, func(done func(error)) { st.Connect(fake.URL, done) }); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if st.Phase != store.PhaseReady {
		t.Fatalf("Phase = %v after retry, want PhaseReady from the saved sign-in", st.Phase)
	}
	if got := savedTokens(t, fix, fake.URL); got != saved {
		t.Fatalf("saved tokens after retry = %+v, want %+v", got, saved)
	}
}

func TestStartRemoteSkipsLocalServer(t *testing.T) {
	starter := &localStarter{}
	local := unreachableURL(t)
	st, fake, fix := storetest.New(t, uninitialized, func(o *store.Options) {
		o.LocalServer = local
		o.StartLocal = starter.start
	})
	if err := fix.Prefs.Save(prefs.Prefs{ServerURL: fake.URL, Language: "en"}); err != nil {
		t.Fatalf("save prefs: %v", err)
	}
	st.Start()
	fix.Settle(t)
	if st.Phase != store.PhaseSetup || st.Server != fake.URL {
		t.Fatalf("Phase=%v Server=%q, want PhaseSetup on %q", st.Phase, st.Server, fake.URL)
	}
	if n := starter.calls.Load(); n != 0 {
		t.Fatalf("StartLocal called %d times for a remote server, want 0", n)
	}
	if err := call(t, fix, func(done func(error)) { st.Connect(fake.URL, done) }); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if n := starter.calls.Load(); n != 0 {
		t.Fatalf("StartLocal called %d times connecting to a remote server, want 0", n)
	}
}

func TestConnectToLocalStartsServer(t *testing.T) {
	starter := &localStarter{}
	fake := fakeapi.New(t)
	st, _, fix := storetest.New(t, nil, func(o *store.Options) {
		o.LocalServer = fake.URL + "/"
		o.StartLocal = starter.start
	})
	if err := fix.Prefs.Save(prefs.Prefs{ServerURL: unreachableURL(t), Language: "en"}); err != nil {
		t.Fatalf("save prefs: %v", err)
	}
	st.Start()
	fix.Settle(t)
	st.ChangeServer()
	fix.Settle(t)
	if got := st.LocalServer(); got != fake.URL {
		t.Fatalf("LocalServer() = %q, want the normalized %q", got, fake.URL)
	}
	if err := call(t, fix, func(done func(error)) { st.Connect(fake.URL, done) }); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if st.Phase != store.PhaseLogin {
		t.Fatalf("Phase = %v, want PhaseLogin", st.Phase)
	}
	if n := starter.calls.Load(); n != 1 {
		t.Fatalf("StartLocal called %d times, want 1", n)
	}
}
