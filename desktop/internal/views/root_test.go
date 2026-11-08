package views_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/theopenbee/openbee/desktop/internal/fakeapi"
	"github.com/theopenbee/openbee/desktop/internal/kit"
	"github.com/theopenbee/openbee/desktop/internal/store"
	"github.com/theopenbee/openbee/desktop/internal/store/storetest"
	"github.com/theopenbee/openbee/desktop/internal/theme"
	"github.com/theopenbee/openbee/desktop/internal/views"
	"github.com/theopenbee/openbee/desktop/internal/views/route"
	"github.com/theopenbee/openbee/desktop/internal/views/testkit"
	"github.com/theopenbee/openbee/desktop/locales"
)

func TestSignOutDoesNotBuildTheChatShell(t *testing.T) {
	h := testkit.SignedIn(t, wideW, wideH, staffed)
	if got := h.Store.Active(); got != "bee" {
		t.Fatalf("active conversation = %q, want bee", got)
	}
	before := len(h.Fake.Calls(http.MethodGet, "/api/local/conversations"))
	h.Store.Logout()
	h.Settle()
	if got := h.Root.Nav().Path(); got != route.LoginPath {
		t.Fatalf("path = %q, want %s", got, route.LoginPath)
	}
	if got := h.Store.Active(); got != "" {
		t.Fatalf("active conversation after sign out = %q, want none", got)
	}
	if n := len(h.Fake.Calls(http.MethodGet, "/api/local/conversations")) - before; n != 0 {
		t.Fatalf("%d conversation fetches after sign out, want none", n)
	}
}

func TestExpiredToastsAreNotShown(t *testing.T) {
	h := testkit.SignedIn(t, wideW, wideH, staffed)
	h.Store.Toast(kit.Toast{Title: "Saved while the window was closed"})
	h.Fix.Clock.Advance(kit.ToastTimeout + time.Second)
	h.Store.Toast(kit.Toast{Title: "Saved just now"})
	h.Settle()
	if !h.T.HasText("Saved just now") {
		t.Fatalf("the fresh toast is not shown; texts %q", h.T.Texts())
	}
	if h.T.HasText("Saved while the window was closed") {
		t.Fatal("a toast older than its timeout was shown")
	}
}

func TestStartingScreenWhileLocalServerStarts(t *testing.T) {
	if err := theme.RegisterFonts(); err != nil {
		t.Fatalf("register fonts: %v", err)
	}
	local := fakeapi.New(t)
	local.Update(func(st *fakeapi.State) { st.Initialized = false })
	release := make(chan struct{})
	st, _, fix := storetest.New(t, nil, func(o *store.Options) {
		o.LocalServer = local.URL
		o.StartLocal = func(context.Context) error {
			<-release
			return nil
		}
	})
	root := views.NewRoot(st)
	tt := testkit.NewTester(root.View, wideW, wideH)
	st.Start()
	tt.Frame()
	if !tt.HasText(locales.StartingTitle()) {
		t.Fatalf("starting screen not shown; texts %q", tt.Texts())
	}
	close(release)
	fix.Settle(t)
	tt.Frame()
	fix.Settle(t)
	tt.Frame()
	if !tt.HasText(locales.SetupTitle()) {
		t.Fatalf("setup screen not shown after the server started; texts %q", tt.Texts())
	}
	if tt.HasText(locales.StartingTitle()) {
		t.Fatal("starting screen still shown after the server started")
	}
}
