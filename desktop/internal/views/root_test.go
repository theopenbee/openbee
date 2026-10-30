package views_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/theopenbee/openbee/desktop/internal/kit"
	"github.com/theopenbee/openbee/desktop/internal/views/route"
	"github.com/theopenbee/openbee/desktop/internal/views/testkit"
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
