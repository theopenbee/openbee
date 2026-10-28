package views_test

import (
	"os"
	"slices"
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
	titleNudge       = 10
)

type shots int

const (
	wideAndNarrow shots = iota
	wideOnly
	narrowOnly
)

type screen struct {
	name     string
	seed     func(st *fakeapi.State)
	signedIn bool
	prepare  func(t *testing.T, h *testkit.Harness)
	narrow   func(t *testing.T, h *testkit.Harness)
	shots    shots
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

func said(id, senderType, senderID, content string, at time.Time) api.ChatMessage {
	return api.ChatMessage{ID: id, Sender: api.ChatSender{Type: senderType, ID: senderID}, Kind: "message", Content: content, TS: at.UnixMilli()}
}

func handedOff(id, workerID, taskType, cron, content string, at time.Time) api.ChatMessage {
	m := said(id, "bee", "", content, at)
	m.Kind = "handoff"
	m.Handoff = &api.HandoffInfo{WorkerID: workerID, TaskID: "task-" + id, TaskType: taskType, CronExpr: cron,
		ConversationID: api.WorkerConversation(workerID)}
	return m
}

func busy(st *fakeapi.State, workerID string) {
	for i := range st.Conversations {
		if st.Conversations[i].WorkerID == workerID {
			st.Conversations[i].Activity = api.ConversationActivity{State: "running"}
		}
	}
}

func chatHistory(st *fakeapi.State) {
	staffed(st)
	now := time.Now()
	yesterday := now.AddDate(0, 0, -1)
	st.Messages = map[string][]api.ChatMessage{
		api.WorkerConversation("ada"): {
			said("a1", "user", "", "Can you pull the February numbers before the board call?", yesterday.Add(-3*time.Hour)),
			said("a2", "worker", "ada", "Done. Revenue is up 4.2% month over month; churn held at 1.9%.", yesterday.Add(-3*time.Hour+4*time.Minute)),
			{ID: "a3", Sender: api.ChatSender{Type: "system"}, Kind: "reset", TS: yesterday.Add(-2 * time.Hour).UnixMilli()},
			said("a4", "bee", "", "Prepare the monthly report for March: revenue by region, churn, and the five largest accounts.", now.Add(-58*time.Minute)),
			said("a5", "worker", "ada", "On it. I'll use the warehouse snapshot from this morning.\nFirst draft in about 20 minutes.", now.Add(-55*time.Minute)),
			said("a6", "user", "", "Can you rerun the March report with the new filters?", now.Add(-47*time.Minute)),
			said("a7", "user", "", "Exclude the trial accounts this time.", now.Add(-46*time.Minute)),
			said("a8", "user", "", "/status", now.Add(-40*time.Minute)),
			said("a9", "system", "", "Ada Lovelace is working on 1 task. 3 tasks finished today.", now.Add(-40*time.Minute)),
			said("a10", "worker", "ada", "Rerunning with trial accounts excluded. Revenue by region:\n\n• North America: $1.42M (+3.1%)\n• Europe: $0.98M (+5.6%)\n• APAC: $0.61M (+8.9%)\n\nThe churn table is next.", now.Add(-12*time.Minute)),
		},
		api.BeeConversation: {
			said("b1", "user", "", "@Ada Lovelace please prepare the monthly report every Monday at 9.", now.Add(-35*time.Minute)),
			said("b2", "bee", "", "Got it. I scheduled the report for Ada and asked 张伟 to rerun the integration tests.", now.Add(-34*time.Minute)),
			handedOff("b3", "ada", "scheduled", "0 9 * * 1", "Prepare the monthly report: revenue by region, churn and top accounts.\n\nShare it in this chat.\nFlag anything unusual.\nKeep it under two pages.", now.Add(-34*time.Minute)),
			handedOff("b4", "zhang", "countdown", "", "接口改完之后重新跑一遍集成测试", now.Add(-33*time.Minute)),
			handedOff("b5", "retired", "immediate", "", "Archive the 1.3 build artifacts", now.Add(-32*time.Minute)),
			said("b6", "worker", "zhang", "集成测试全部通过，可以合并了。", now.Add(-9*time.Minute)),
			said("b7", "bee", "", "Morning! Two workers finished their overnight tasks.", now.Add(-9*time.Minute)),
		},
		api.WorkerConversation("zhang"): {
			said("z1", "user", "", "接口改完之后跑一下集成测试", now.Add(-20*time.Minute)),
			said("z2", "worker", "zhang", "好的，正在跑。预计十分钟。", now.Add(-19*time.Minute)),
		},
	}
	st.ActiveTasks = map[string][]api.ActiveTask{
		api.WorkerConversation("zhang"): {
			{ID: "t1", Instruction: "接口改完之后重新跑一遍集成测试", Type: "countdown"},
			{ID: "t2", Instruction: "Post the nightly build summary to the release channel and tag the owners of every failing job", Type: "scheduled"},
			{ID: "t3", Instruction: "Review the open pull requests", Type: "immediate"},
		},
	}
	busy(st, "ada")
	busy(st, "zhang")
}

func emptyBee(st *fakeapi.State) {
	staffed(st)
	st.Conversations[0].LastMessage = nil
	st.Conversations[0].Unread = 0
}

func openChat(param string) func(t *testing.T, h *testkit.Harness) {
	return func(_ *testing.T, h *testkit.Harness) {
		h.Root.Nav().Push(route.Chat(param))
		h.Settle()
		h.T.Advance(time.Second)
		h.Settle()
	}
}

func clickAndSettle(t *testing.T, h *testkit.Harness, name string, dx float32) {
	t.Helper()
	r, ok := h.T.Find(name)
	if !ok {
		t.Fatalf("no %q; texts %q", name, h.T.Texts())
	}
	h.T.ClickAt(r.X+r.W/2+dx, r.Y+r.H/2)
	h.Settle()
	h.T.Advance(time.Second)
	h.Settle()
}

func resetConflict(t *testing.T, h *testkit.Harness) {
	openChat("zhang")(t, h)
	clickAndSettle(t, h, locales.ChatNewConversation(), 0)
	r, _ := h.T.Find(locales.CommonCancel())
	clickAndSettle(t, h, locales.CommonCancel(), r.W/2+40)
}

var chatScreens = []screen{
	{name: "chat-worker", seed: chatHistory, signedIn: true, prepare: openChat("ada"), shots: wideOnly,
		want: func() string { return "Exclude the trial accounts this time." }},
	{name: "chat-narrow", seed: chatHistory, signedIn: true, prepare: openChat("bee"), shots: narrowOnly,
		want: func() string { return locales.ChatHandedOffTo("Ada Lovelace") }},
	{name: "chat-bee-empty", seed: emptyBee, signedIn: true, prepare: openChat("bee"), shots: wideOnly, want: locales.ChatNoMessagesTitle},
	{name: "chat-worker-intro", seed: staffed, signedIn: true, prepare: openChat("grace"), shots: wideOnly,
		want: func() string { return locales.ChatEmptyWorker("Grace Hopper") }},
	{name: "chat-reset-dialog", seed: chatHistory, signedIn: true, prepare: resetConflict, shots: wideOnly, want: locales.ChatStopAndRestart},
}

func TestScreens(t *testing.T) {
	if os.Getenv(testkit.ScreensEnv) == "" {
		t.Skip(testkit.ScreensEnv + " is not set")
	}
	for _, s := range slices.Concat(authScreens, shellScreens, chatScreens) {
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
			if s.shots != narrowOnly {
				h.Shot(s.name)
			}
			if s.shots == wideOnly {
				return
			}
			h.T.SetSize(narrowW, narrowH)
			h.Settle()
			if s.narrow != nil {
				s.narrow(t, h)
			}
			name := s.name + "-narrow"
			if s.shots == narrowOnly {
				name = s.name
			}
			h.Shot(name)
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

func TestMalformedConversationRedirectsToBee(t *testing.T) {
	h := testkit.SignedIn(t, wideW, wideH, staffed)
	h.Root.Nav().Push(route.ChatPath + "/worker:ada")
	h.Settle()
	if got := h.Root.Nav().Path(); got != route.Chat(api.BeeConversation) {
		t.Fatalf("path = %q, want /chat/bee", got)
	}
}

func TestChatTitleTogglesPanel(t *testing.T) {
	h := testkit.SignedIn(t, wideW, wideH, func(st *fakeapi.State) {
		staffed(st)
		for i := range st.Conversations {
			if st.Conversations[i].WorkerID == "ada" {
				st.Conversations[i].Activity = api.ConversationActivity{State: "queued", QueuedAhead: 2}
			}
		}
	})
	nav := h.Root.Nav()
	nav.Push(route.Chat("ada"))
	h.Settle()
	for _, want := range []string{route.PanelConstraints, ""} {
		status, ok := h.T.Find(locales.ChatStatusQueued(2))
		if !ok {
			t.Fatalf("no status line; texts %q", h.T.Texts())
		}
		h.T.ClickAt(status.X+titleNudge, status.Y-titleNudge)
		h.Settle()
		h.T.Advance(time.Second)
		h.Settle()
		if got := nav.Query(route.PanelQuery); got != want {
			t.Fatalf("panel = %q, want %q", got, want)
		}
	}
}

func TestOpenConversationPushesChat(t *testing.T) {
	h := testkit.SignedIn(t, wideW, wideH, func(st *fakeapi.State) { st.Workers = []api.Worker{staffWorker("w1", "Alice", "", "idle")} })
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
