package views_test

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/api"
	"github.com/theopenbee/openbee/desktop/internal/fakeapi"
	"github.com/theopenbee/openbee/desktop/internal/store"
	"github.com/theopenbee/openbee/desktop/internal/views/route"
	"github.com/theopenbee/openbee/desktop/internal/views/testkit"
	"github.com/theopenbee/openbee/desktop/locales"
)

const (
	wideW, wideH     = 1440, 900
	narrowW, narrowH = 700, 900
	titleNudge       = 10
	chartW, chartH   = 360, 88
	chartBar         = 14
	chartGap         = 8
	imageWait        = 3 * time.Second
	chartAlt         = "Coverage by package"
	boardChart       = "board-chart.png"
	chartPicW        = 720
	chartPicH        = 440
	sunSize          = 96
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
	hint     bool
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

func offline(t *testing.T, h *testkit.Harness) {
	openChat("ada")(t, h)
	h.Fake.FailNext(http.MethodGet, "/api/local/stream", http.StatusServiceUnavailable, map[string]any{"error": "down"})
	h.Fake.DropStreams()
	h.Fix.Await(t, func() bool { return !h.Store.Online })
	h.Fix.Clock.AwaitSleepers(t, 2)
	h.Fix.Clock.Advance(3 * time.Second)
	h.Fix.Await(t, func() bool {
		return len(h.Fake.Calls(http.MethodGet, "/api/local/stream")) == 2 && h.Fix.Clock.Sleepers() == 2
	})
	h.Fix.Clock.Advance(time.Second)
	h.Fix.Await(t, func() bool { return h.Store.Offline })
	h.Settle()
	h.T.Advance(time.Second)
	h.Settle()
}

func blockedHint(t *testing.T, h *testkit.Harness) {
	h.Fix.Notifier.ShowErr = store.ErrNotificationsDenied
	openChat("ada")(t, h)
	clickAndSettle(t, h, locales.NotificationsEnable(), 0)
}

func beeBusy(t *testing.T, h *testkit.Harness) {
	openChat("bee")(t, h)
	typeInComposer(t, h, "Please summarise the nightly results.")
	h.T.Key(0, ui.KeyEnter)
	h.Settle()
	h.T.Advance(time.Second)
	h.Settle()
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

const richReply = "## Release 1.4.2 review\n" +
	"The branch is close; see the [release guide](https://docs.openbee.dev/release) for the steps.\n\n" +
	"- Signed builds for **macOS** and *Linux*\n" +
	"- The ~~old~~ new updater feed\n\n" +
	"* [x] Tag the build and update the changelog\n" +
	"* [ ] Publish the notes after `make dist` passes\n\n" +
	"> Windows arm64 still needs a manual retry after signing.\n\n" +
	"```go\n" +
	"if err := retry(ctx, \"sign-arm64\", 3); err != nil {\n" +
	"\treturn fmt.Errorf(\"sign: %w\", err)\n" +
	"}\n" +
	"```\n\n" +
	"| Platform | Status | Size |\n" +
	"| --- | --- | ---: |\n" +
	"| macOS universal | Ready | 48.2 MB |\n" +
	"| Windows arm64 | Retrying | 50.9 MB |\n\n" +
	"![" + chartAlt + "](/api/local/media/coverage.png)"

func coverageChart(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, chartW, chartH))
	draw.Draw(img, img.Bounds(), &image.Uniform{color.RGBA{R: 250, G: 246, B: 240, A: 255}}, image.Point{}, draw.Src)
	for i, share := range []float64{0.92, 0.78, 0.64, 0.41} {
		y := chartGap + i*(chartBar+chartGap/2)
		bar := image.Rect(chartGap, y, chartGap+int(share*float64(chartW-2*chartGap)), y+chartBar)
		draw.Draw(img, bar, &image.Uniform{color.RGBA{R: 244, G: 129 - uint8(i*18), B: 32 + uint8(i*30), A: 255}}, image.Point{}, draw.Src)
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func markdownChat(chart []byte) func(st *fakeapi.State) {
	return func(st *fakeapi.State) {
		staffed(st)
		now := time.Now()
		st.Media = map[string][]byte{"coverage.png": chart}
		st.Messages = map[string][]api.ChatMessage{
			api.WorkerConversation("grace"): {
				said("g1", "user", "", "Where are we with the 1.4.2 release?", now.Add(-30*time.Minute)),
				said("g2", "worker", "grace", richReply, now.Add(-28*time.Minute)),
			},
		}
	}
}

func openRich(t *testing.T, h *testkit.Harness) {
	openChat("grace")(t, h)
	deadline := time.Now().Add(imageWait)
	for {
		h.Settle()
		h.T.Frame()
		if r, ok := h.T.Find(chartAlt); ok && r.W == chartW && r.H == chartH {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("the chart did not load; texts %q", h.T.Texts())
		}
		time.Sleep(5 * time.Millisecond)
	}
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
	{name: "chat-offline", seed: chatHistory, signedIn: true, prepare: offline, want: locales.ChatConnectionLost},
	{name: "chat-hint-prompt", seed: chatHistory, signedIn: true, prepare: openChat("ada"), hint: true, want: locales.NotificationsPromptTitle},
	{name: "chat-hint-blocked", seed: chatHistory, signedIn: true, prepare: blockedHint, hint: true, shots: wideOnly,
		want: locales.NotificationsBlockedTitle},
	{name: "chat-busy", seed: chatHistory, signedIn: true, prepare: beeBusy, shots: wideOnly,
		want: func() string { return "Please summarise the nightly results." }},
}

func markdownScreens(t *testing.T) []screen {
	return []screen{
		{name: "markdown-rich", seed: markdownChat(coverageChart(t)), signedIn: true, prepare: openRich, shots: wideOnly,
			want: func() string { return "Release 1.4.2 review" }},
	}
}

func boardPicture(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, chartPicW, chartPicH))
	for y := range chartPicH {
		mix := float64(y) / chartPicH
		row := color.RGBA{R: uint8(252 - 70*mix), G: uint8(214 - 40*mix), B: uint8(170 + 60*mix), A: 255}
		draw.Draw(img, image.Rect(0, y, chartPicW, y+1), &image.Uniform{row}, image.Point{}, draw.Src)
	}
	for i, h := range []int{120, 190, 150, 260, 210} {
		x := 60 + i*120
		bar := image.Rect(x, chartPicH-40-h, x+72, chartPicH-40)
		draw.Draw(img, bar, &image.Uniform{color.RGBA{R: 38, G: 54, B: uint8(92 + i*24), A: 255}}, image.Point{}, draw.Src)
	}
	sun := image.Rect(chartPicW-sunSize-40, 36, chartPicW-40, 36+sunSize)
	draw.Draw(img, sun, &image.Uniform{color.RGBA{R: 255, G: 244, B: 214, A: 255}}, image.Point{}, draw.Src)
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func attachmentChat(picture []byte) func(st *fakeapi.State) {
	files := []string{"march-report.pdf", "source-data.zip", "export.py", "walkthrough.mp4", "memo.bin"}
	return func(st *fakeapi.State) {
		staffed(st)
		now := time.Now()
		st.Media = map[string][]byte{boardChart: picture}
		for _, name := range files {
			st.Media[name] = []byte(name)
		}
		upload := said("u1", "user", "", "Here is the board chart, can you check the numbers against it?", now.Add(-30*time.Minute))
		upload.MediaPaths = []string{boardChart}
		reply := said("u2", "worker", "ada", "Checked. The report, the raw data and my script are attached.", now.Add(-25*time.Minute))
		reply.MediaPaths = files
		st.Messages = map[string][]api.ChatMessage{api.WorkerConversation("ada"): {upload, reply}}
	}
}

func openAttachments(t *testing.T, h *testkit.Harness) {
	openChat("ada")(t, h)
	label := locales.ChatViewImage(boardChart)
	deadline := time.Now().Add(imageWait)
	for {
		h.Settle()
		h.T.Frame()
		if _, ok := h.T.Find(label); ok {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("the thumbnail did not load; texts %q", h.T.Texts())
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func openLightbox(t *testing.T, h *testkit.Harness) {
	openAttachments(t, h)
	clickAndSettle(t, h, locales.ChatViewImage(boardChart), 0)
}

func attachmentScreens(t *testing.T) []screen {
	seed := attachmentChat(boardPicture(t))
	return []screen{
		{name: "chat-attachments", seed: seed, signedIn: true, prepare: openAttachments, shots: wideOnly,
			want: func() string { return "march-report.pdf" }},
		{name: "lightbox", seed: seed, signedIn: true, prepare: openLightbox, shots: wideAndNarrow,
			want: func() string { return boardChart }},
	}
}

func typeInComposer(t *testing.T, h *testkit.Harness, text string) {
	t.Helper()
	if err := h.T.Click(locales.ChatMessageLabel()); err != nil {
		t.Fatal(err)
	}
	h.T.Type(text)
	h.Settle()
}

func mentionPopup(t *testing.T, h *testkit.Harness) {
	openChat("bee")(t, h)
	typeInComposer(t, h, "Please ask @a")
	h.T.Key(0, ui.KeyDown)
	h.Settle()
}

func pendingAttachments(t *testing.T, h *testkit.Harness) {
	openChat("ada")(t, h)
	dir := t.TempDir()
	var paths []string
	for _, name := range []string{"march-report-final-version-with-appendix.pdf", "revenue-by-region.csv", "board-chart.png"} {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(name), 0o600); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, path)
	}
	paths = append(paths, filepath.Join(dir, "missing.zip"))
	typeInComposer(t, h, "Here are the files for the March report.")
	r, _ := h.T.Find(locales.ChatMessageLabel())
	if !h.T.DropFiles(r.X+r.W/2, r.Y+r.H/2, paths...) {
		t.Fatal("the composer did not take the files")
	}
	h.Settle()
}

var composerScreens = []screen{
	{name: "composer-mention", seed: chatHistory, signedIn: true, prepare: mentionPopup, shots: wideOnly,
		want: func() string { return "Grace Hopper" }},
	{name: "composer-attachments", seed: chatHistory, signedIn: true, prepare: pendingAttachments,
		want: func() string { return locales.ChatUploadError(1) }},
}

func TestScreens(t *testing.T) {
	if os.Getenv(testkit.ScreensEnv) == "" {
		t.Skip(testkit.ScreensEnv + " is not set")
	}
	for _, s := range slices.Concat(authScreens, shellScreens, chatScreens, markdownScreens(t), attachmentScreens(t), composerScreens) {
		t.Run(s.name, func(t *testing.T) {
			open := testkit.New
			if s.signedIn {
				open = testkit.SignedIn
			}
			h := open(t, wideW, wideH, s.seed)
			if s.signedIn && !s.hint {
				h.Fix.Notifier.Unsupported = true
				h.Settle()
			}
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
