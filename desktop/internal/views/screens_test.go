package views_test

import (
	"os"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/api"
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
	name     string
	seed     func(st *fakeapi.State)
	signedIn bool
	prepare  func(t *testing.T, h *testkit.Harness)
	narrow   func(t *testing.T, h *testkit.Harness)
	want     func() string
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

func staffWorker(id, name, description, status string) api.Worker {
	return api.Worker{ID: id, Name: name, Description: description, Status: status, Engine: "claude", WorkDir: "/srv/" + id}
}

func lastMessage(sender, preview string, at time.Time) *api.LastMessage {
	return &api.LastMessage{Sender: api.ChatSender{Type: sender}, Kind: "message", Preview: preview, TS: at.UnixMilli()}
}

func workerChat(id string, last *api.LastMessage, unread int) api.ConversationSummary {
	return api.ConversationSummary{ID: api.WorkerConversation(id), Kind: "worker", WorkerID: id, LastMessage: last, Unread: unread,
		Activity: api.ConversationActivity{State: "idle"}}
}

func staffed(st *fakeapi.State) {
	now := time.Now()
	st.Workers = []api.Worker{
		staffWorker("zhang", "张伟", "后端开发与接口维护", "working"),
		staffWorker("ada", "Ada Lovelace", "Data analysis and monthly reports", "idle"),
		staffWorker("li", "李娜", "文档与翻译", "idle"),
		staffWorker("release", "release-bot", "Ships builds for every platform", "error"),
		staffWorker("grace", "Grace Hopper", "Code review and refactoring", "idle"),
	}
	st.Conversations = []api.ConversationSummary{
		{ID: api.BeeConversation, Kind: "bee", Unread: 2, Activity: api.ConversationActivity{State: "idle"},
			LastMessage: lastMessage("bee", "Morning! Two workers finished their overnight tasks.", now.Add(-9*time.Minute))},
		workerChat("zhang", lastMessage("worker", "接口已经改好了，正在跑集成测试", now.Add(-3*time.Minute)), 3),
		workerChat("ada", lastMessage("user", "Can you rerun the March report with the new filters?", now.Add(-47*time.Minute)), 0),
		workerChat("li", &api.LastMessage{Sender: api.ChatSender{Type: "bee"}, Kind: "handoff", HandoffWorkerID: "ada",
			TS: now.AddDate(0, 0, -1).UnixMilli()}, 0),
		workerChat("release", lastMessage("worker", "Build 1.4.2 failed on Windows arm64", now.AddDate(0, 0, -6)), 150),
	}
}

func listOnly(_ *testing.T, h *testkit.Harness) {
	h.Root.Nav().Push(route.ChatPath)
	h.Settle()
}

func stillLoading(_ *testing.T, h *testkit.Harness) {
	for _, r := range []interface{ Reset() }{h.Store.Workers, h.Store.Conversations} {
		r.Reset()
	}
	h.Store.Workers.Loading = true
	h.Store.Conversations.Loading = true
	h.Settle()
}

var shellScreens = []screen{
	{name: "shell-list", seed: staffed, signedIn: true, narrow: listOnly, want: func() string { return "Ada Lovelace" }},
	{name: "shell-loading", seed: staffed, signedIn: true, prepare: stillLoading, narrow: listOnly, want: locales.NavSettings},
}

func TestScreens(t *testing.T) {
	if os.Getenv(testkit.ScreensEnv) == "" {
		t.Skip(testkit.ScreensEnv + " is not set")
	}
	for _, s := range append(authScreens, shellScreens...) {
		t.Run(s.name, func(t *testing.T) {
			open := testkit.New
			if s.signedIn {
				open = testkit.SignedIn
			}
			h := open(t, wideW, wideH, s.seed)
			if s.prepare != nil {
				s.prepare(t, h)
			}
			if !h.T.HasText(s.want()) {
				t.Fatalf("%s does not show %q; texts %q", s.name, s.want(), h.T.Texts())
			}
			h.Shot(s.name)
			h.T.SetSize(narrowW, narrowH)
			h.Settle()
			if s.narrow != nil {
				s.narrow(t, h)
			}
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
