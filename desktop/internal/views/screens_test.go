package views_test

import (
	"os"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/fakeapi"
	"github.com/theopenbee/openbee/desktop/internal/views/route"
	"github.com/theopenbee/openbee/desktop/internal/views/testkit"
	"github.com/theopenbee/openbee/desktop/locales"
)

const (
	wideW, wideH     = 1440, 900
	narrowW, narrowH = 700, 900
)

type screen struct {
	name    string
	seed    func(st *fakeapi.State)
	prepare func(t *testing.T, h *testkit.Harness)
	want    func() string
}

func uninitialized(st *fakeapi.State) { st.Initialized = false }

func connect(_ *testing.T, h *testkit.Harness) { h.Connect() }

func failedLogin(t *testing.T, h *testkit.Harness) {
	h.Connect()
	for _, field := range []struct{ label, text string }{
		{locales.LoginUsername(), "admin"},
		{locales.LoginPassword(), "wrong"},
	} {
		if err := h.T.Click(field.label); err != nil {
			t.Fatal(err)
		}
		h.T.Type(field.text)
	}
	h.T.Key(0, ui.KeyEnter)
	h.Settle()
}

var authScreens = []screen{
	{name: "connect", want: locales.ConnectTitle},
	{name: "login", prepare: connect, want: locales.LoginDescription},
	{name: "login-error", prepare: failedLogin, want: locales.LoginError401},
	{name: "setup", seed: uninitialized, prepare: connect, want: locales.SetupTitle},
}

func TestScreens(t *testing.T) {
	if os.Getenv(testkit.ScreensEnv) == "" {
		t.Skip(testkit.ScreensEnv + " is not set")
	}
	for _, s := range authScreens {
		t.Run(s.name, func(t *testing.T) {
			h := testkit.New(t, wideW, wideH, s.seed)
			if s.prepare != nil {
				s.prepare(t, h)
			}
			if !h.T.HasText(s.want()) {
				t.Fatalf("%s does not show %q; texts %q", s.name, s.want(), h.T.Texts())
			}
			h.Shot(s.name)
			h.T.SetSize(narrowW, narrowH)
			h.Settle()
			h.Shot(s.name + "-narrow")
		})
	}
}

func TestWideChatKeepsPanelQuery(t *testing.T) {
	h := testkit.SignedIn(t, narrowW, narrowH, nil)
	nav := h.Root.Nav()
	if got := nav.Path(); got != route.ChatPath {
		t.Fatalf("narrow path = %q, want %s", got, route.ChatPath)
	}
	h.Root.NewWorker()
	h.Settle()
	if got := nav.Query(route.PanelQuery); got != route.PanelNew {
		t.Fatalf("panel = %q, want %s", got, route.PanelNew)
	}
	h.T.SetSize(wideW, wideH)
	h.Settle()
	if got := nav.Path(); got != route.Chat("bee") {
		t.Fatalf("wide path = %q, want /chat/bee", got)
	}
	if got := nav.Query(route.PanelQuery); got != route.PanelNew {
		t.Fatalf("panel after the redirect = %q, want %s", got, route.PanelNew)
	}
}

func TestOpenConversationPushesChat(t *testing.T) {
	h := testkit.SignedIn(t, wideW, wideH, nil)
	h.Root.OpenConversation("worker:w1")
	h.Settle()
	if got := h.Root.Nav().Path(); got != "/chat/w1" {
		t.Fatalf("path = %q, want /chat/w1", got)
	}
	h.Store.Logout()
	h.Settle()
	h.Root.OpenConversation("bee")
	h.Settle()
	if got := h.Root.Nav().Path(); got != route.LoginPath {
		t.Fatalf("path after sign out = %q, want %s", got, route.LoginPath)
	}
}
