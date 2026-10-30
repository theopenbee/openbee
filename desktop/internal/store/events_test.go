package store_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/theopenbee/openbee/desktop/internal/api"
	"github.com/theopenbee/openbee/desktop/internal/fakeapi"
	"github.com/theopenbee/openbee/desktop/internal/prefs"
	"github.com/theopenbee/openbee/desktop/internal/store"
	"github.com/theopenbee/openbee/desktop/internal/store/storetest"
	"github.com/theopenbee/openbee/desktop/locales"
)

const streamPath = "/api/local/stream"

func previewOf(st *store.Store, cid string) string {
	for _, c := range st.Conversations.Data {
		if c.ID == cid && c.LastMessage != nil {
			return c.LastMessage.Preview
		}
	}
	return ""
}

func dropStream(t *testing.T, fake *fakeapi.Server, fix *storetest.Fixture, st *store.Store) {
	t.Helper()
	fake.DropStreams()
	fix.Await(t, func() bool { return !st.Online })
	fix.Clock.AwaitSleepers(t, 2)
}

func notifying(t *testing.T, seed func(st *fakeapi.State)) (*store.Store, *fakeapi.Server, *storetest.Fixture) {
	t.Helper()
	st, fake, fix := loaded(t, seed)
	st.Prefs.Notifications = "on"
	return st, fake, fix
}

func TestEventMessageUpdatesPreviewAndUnread(t *testing.T) {
	st, fake, fix := loaded(t, team)
	var got []string
	off := st.OnMessage(func(cid string, m api.ChatMessage) { got = append(got, cid+"/"+m.ID) })

	fake.Reply("worker:w1", api.ChatMessage{ID: "m1", Content: "[file] a.png\nHello\n\n  there"})
	fix.Await(t, func() bool { return len(got) == 1 })
	sum := summaryOf(t, st, "worker:w1")
	if sum.LastMessage == nil || sum.LastMessage.Preview != "Hello there" || sum.Unread != 1 {
		t.Fatalf("summary after message = %+v / %+v, want preview %q and unread 1", sum, sum.LastMessage, "Hello there")
	}
	if got[0] != "worker:w1/m1" {
		t.Fatalf("message listener got %v", got)
	}
	if n := countCalls(fake, "GET", "/api/local/conversations"); n != 1 {
		t.Fatalf("conversation list calls = %d, want the event applied without a refetch", n)
	}

	off()
	fake.Reply("worker:w1", api.ChatMessage{ID: "m2", Content: "again"})
	fix.Await(t, func() bool { return previewOf(st, "worker:w1") == "again" })
	if len(got) != 1 {
		t.Fatalf("listener called after off: %v", got)
	}
	if sum := summaryOf(t, st, "worker:w1"); sum.Unread != 2 {
		t.Fatalf("unread = %d, want 2", sum.Unread)
	}
}

func TestEventForActiveVisibleConversationKeepsUnread(t *testing.T) {
	st, fake, fix := loaded(t, team)
	st.SetActive("worker:w1")
	st.SetForeground(true)
	if st.Active() != "worker:w1" {
		t.Fatalf("Active = %q", st.Active())
	}

	fake.Reply("worker:w1", api.ChatMessage{ID: "m1", Content: "seen"})
	fake.Reply("worker:w2", api.ChatMessage{ID: "m2", Content: "unseen"})
	fix.Await(t, func() bool { return previewOf(st, "worker:w2") == "unseen" })
	if sum := summaryOf(t, st, "worker:w1"); previewOf(st, "worker:w1") != "seen" || sum.Unread != 0 {
		t.Fatalf("active visible conversation = %+v, want preview updated and unread 0", sum)
	}
	if sum := summaryOf(t, st, "worker:w2"); sum.Unread != 1 {
		t.Fatalf("other conversation unread = %d, want 1", sum.Unread)
	}

	st.SetForeground(false)
	fake.Reply("worker:w1", api.ChatMessage{ID: "m3", Content: "hidden"})
	fix.Await(t, func() bool { return previewOf(st, "worker:w1") == "hidden" })
	if sum := summaryOf(t, st, "worker:w1"); sum.Unread != 1 {
		t.Fatalf("active hidden conversation unread = %d, want 1", sum.Unread)
	}
}

func TestEventTaskSetsActivity(t *testing.T) {
	st, fake, fix := loaded(t, team)
	steps := []struct {
		state string
		ahead int
		want  api.ConversationActivity
	}{
		{"queued", 2, api.ConversationActivity{State: "queued", QueuedAhead: 2}},
		{"running", 0, api.ConversationActivity{State: "running"}},
		{"completed", 0, api.ConversationActivity{State: "idle"}},
	}
	for _, step := range steps {
		fake.Emit(api.ChatEvent{Type: "task", ConversationID: "worker:w1", WorkerID: "w1", TaskID: "t1", State: step.state, QueuedAhead: step.ahead})
		fix.Await(t, func() bool { return summaryOf(t, st, "worker:w1").Activity == step.want })
	}
	if n := countCalls(fake, "GET", "/api/local/conversations"); n != 1 {
		t.Fatalf("conversation list calls = %d, want 1", n)
	}
}

func TestEventWorkerStatusPatchesWorker(t *testing.T) {
	st, fake, fix := loaded(t, team)
	detail := st.WorkerDetail("w1")
	detail.Ensure()
	fix.Settle(t)

	fake.Emit(api.ChatEvent{Type: "worker_status", WorkerID: "w1", Status: "working"})
	fix.Await(t, func() bool {
		w, _ := st.Worker("w1")
		return w.Status == "working"
	})
	if detail.Data.Status != "working" {
		t.Fatalf("worker detail status = %q, want working", detail.Data.Status)
	}
	if w, _ := st.Worker("w2"); w.Status != "idle" {
		t.Fatalf("other worker status = %q, want idle", w.Status)
	}
	if n, d := countCalls(fake, "GET", "/api/workers"), countCalls(fake, "GET", "/api/workers/w1"); n != 1 || d != 1 {
		t.Fatalf("worker calls list=%d detail=%d, want the status patched without refetching", n, d)
	}
}

func TestEventUnknownWorkerRefetches(t *testing.T) {
	st, fake, fix := loaded(t, team)
	fake.Update(func(s *fakeapi.State) {
		s.Workers = append(s.Workers, api.Worker{ID: "w3", Name: "Cleo", Status: "idle"})
	})

	fake.Emit(api.ChatEvent{Type: "worker_status", WorkerID: "w3", Status: "working"})
	fix.Await(t, func() bool {
		_, ok := st.Worker("w3")
		return ok
	})
	if w, _ := st.Worker("w3"); w.Status != "working" {
		t.Fatalf("refetched worker status = %q, want working", w.Status)
	}
	if n := countCalls(fake, "GET", "/api/workers"); n != 2 {
		t.Fatalf("worker list calls = %d, want 2", n)
	}
}

func TestStreamResyncAfterReconnect(t *testing.T) {
	st, fake, fix := loaded(t, team)
	resyncs := 0
	off := st.OnResync(func() { resyncs++ })
	defer off()

	dropStream(t, fake, fix, st)
	if st.Offline {
		t.Fatal("Offline right after the drop, want a grace period")
	}
	fix.Clock.Advance(2 * time.Second)
	fix.Await(t, func() bool { return st.Online })

	if w, c := countCalls(fake, "GET", "/api/workers"), countCalls(fake, "GET", "/api/local/conversations"); w != 2 || c != 2 {
		t.Fatalf("calls after reconnect workers=%d conversations=%d, want one refetch each", w, c)
	}
	if resyncs != 1 {
		t.Fatalf("resync listeners called %d times, want 1", resyncs)
	}
	if n := fake.Streams(); n != 1 {
		t.Fatalf("open streams = %d, want 1", n)
	}
	if st.Offline {
		t.Fatal("Offline after reconnect")
	}
}

func TestSeqGapResyncs(t *testing.T) {
	st, fake, fix := loaded(t, team)
	resyncs := 0
	off := st.OnResync(func() { resyncs++ })
	defer off()

	fake.Reply("worker:w1", api.ChatMessage{ID: "m1", Content: "one"})
	fix.Await(t, func() bool { return previewOf(st, "worker:w1") == "one" })
	if resyncs != 0 {
		t.Fatalf("resync without a gap")
	}

	fake.SkipSeq(2)
	fake.Reply("worker:w1", api.ChatMessage{ID: "m2", Content: "two"})
	fix.Await(t, func() bool { return resyncs == 1 })
	if got := previewOf(st, "worker:w1"); got != "two" {
		t.Fatalf("preview after gap = %q, want two", got)
	}
	if w, c := countCalls(fake, "GET", "/api/workers"), countCalls(fake, "GET", "/api/local/conversations"); w != 2 || c != 2 {
		t.Fatalf("calls after gap workers=%d conversations=%d, want one refetch each", w, c)
	}

	fake.Reply("worker:w1", api.ChatMessage{ID: "m3", Content: "three"})
	fix.Await(t, func() bool { return previewOf(st, "worker:w1") == "three" })
	if resyncs != 1 {
		t.Fatalf("resyncs = %d after a consecutive event, want 1", resyncs)
	}
}

func TestOfflineAfterGrace(t *testing.T) {
	st, fake, fix := loaded(t, team)
	fake.FailNext("GET", streamPath, http.StatusServiceUnavailable, map[string]any{"error": "down"})
	dropStream(t, fake, fix, st)

	fix.Clock.Advance(3 * time.Second)
	fix.Await(t, func() bool { return countCalls(fake, "GET", streamPath) == 2 && fix.Clock.Sleepers() == 2 })
	if st.Offline || st.Online {
		t.Fatalf("after 3 s Online=%v Offline=%v, want down but within the grace period", st.Online, st.Offline)
	}

	fix.Clock.Advance(time.Second)
	fix.Await(t, func() bool { return st.Offline })
	if st.Online {
		t.Fatal("Online while offline")
	}
}

func TestReconnectNowHoldsFeedback(t *testing.T) {
	st, fake, fix := loaded(t, team)
	fake.FailNext("GET", streamPath, http.StatusServiceUnavailable, map[string]any{"error": "down"})
	dropStream(t, fake, fix, st)
	fix.Clock.Advance(3 * time.Second)
	fix.Await(t, func() bool { return countCalls(fake, "GET", streamPath) == 2 && fix.Clock.Sleepers() == 2 })
	fix.Clock.Advance(time.Second)
	fix.Await(t, func() bool { return st.Offline })
	if st.Reconnecting || st.ReconnectFailed {
		t.Fatalf("before a retry Reconnecting=%v ReconnectFailed=%v, want both false", st.Reconnecting, st.ReconnectFailed)
	}

	fake.FailNext("GET", streamPath, http.StatusServiceUnavailable, map[string]any{"error": "down"})
	st.ReconnectNow()
	if !st.Reconnecting {
		t.Fatal("Reconnecting = false right after ReconnectNow")
	}
	fix.Await(t, func() bool { return countCalls(fake, "GET", streamPath) == 3 })
	fix.Settle(t)
	if !st.Reconnecting || st.ReconnectFailed {
		t.Fatalf("after a fast failure Reconnecting=%v ReconnectFailed=%v, want the feedback held for a second", st.Reconnecting, st.ReconnectFailed)
	}

	fake.FailNext("GET", streamPath, http.StatusServiceUnavailable, map[string]any{"error": "down"})
	st.ReconnectNow()
	fix.Await(t, func() bool { return countCalls(fake, "GET", streamPath) == 4 })
	if !st.Reconnecting || st.ReconnectFailed {
		t.Fatalf("after a retry within the held second Reconnecting=%v ReconnectFailed=%v, want still held", st.Reconnecting, st.ReconnectFailed)
	}

	fix.Clock.Advance(time.Second)
	fix.Await(t, func() bool { return st.ReconnectFailed })
	if st.Reconnecting {
		t.Fatal("Reconnecting after the held second")
	}

	st.ReconnectNow()
	fix.Await(t, func() bool { return st.Online })
	if st.Reconnecting || st.ReconnectFailed || st.Offline {
		t.Fatalf("online with Reconnecting=%v ReconnectFailed=%v Offline=%v, want all false", st.Reconnecting, st.ReconnectFailed, st.Offline)
	}
}

func TestForegroundReconnectsClosedStream(t *testing.T) {
	st, fake, fix := loaded(t, team)
	dropStream(t, fake, fix, st)

	st.SetForeground(true)
	fix.Await(t, func() bool { return st.Online })
	if n := fake.Streams(); n != 1 {
		t.Fatalf("open streams = %d, want 1", n)
	}
}

func TestStreamRefreshesExpiredToken(t *testing.T) {
	st, fake, fix := loaded(t, team)
	fake.ExpireAccessTokens()
	dropStream(t, fake, fix, st)

	fix.Clock.Advance(2 * time.Second)
	fix.Await(t, func() bool { return st.Online })
	if st.Phase != store.PhaseReady {
		t.Fatalf("Phase = %v, want PhaseReady", st.Phase)
	}
	if n := countCalls(fake, "POST", "/api/auth/refresh"); n != 1 {
		t.Fatalf("refresh calls = %d, want 1", n)
	}
}

func TestStreamUnauthorizedSignsOut(t *testing.T) {
	st, fake, fix := loaded(t, team)
	fake.RevokeRefreshTokens()
	fake.ExpireAccessTokens()
	dropStream(t, fake, fix, st)

	fix.Clock.Advance(2 * time.Second)
	fix.Await(t, func() bool { return st.Phase == store.PhaseLogin })
	if st.Online || st.Offline {
		t.Fatalf("after sign-out Online=%v Offline=%v, want both false", st.Online, st.Offline)
	}
	if got := savedTokens(t, fix, fake.URL); got != (prefs.Tokens{}) {
		t.Fatalf("tokens after sign-out = %+v, want cleared", got)
	}
}

func TestStreamRejectsNonEventStream(t *testing.T) {
	page := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = io.WriteString(w, "<!doctype html>")
	}))
	defer page.Close()
	st, _, fix := storetest.New(t, nil)
	if err := fix.Prefs.Save(prefs.Prefs{ServerURL: page.URL, Language: "en"}); err != nil {
		t.Fatalf("save prefs: %v", err)
	}
	if err := fix.Tokens.Save(page.URL, prefs.Tokens{Access: "a", Refresh: "r"}); err != nil {
		t.Fatalf("save tokens: %v", err)
	}
	resyncs := 0
	st.OnResync(func() { resyncs++ })

	st.Start()
	fix.Settle(t)
	if st.Phase != store.PhaseReady || st.Online || resyncs != 0 {
		t.Fatalf("Phase=%v Online=%v resyncs=%d, want an HTML page treated as a dropped stream", st.Phase, st.Online, resyncs)
	}
}

func TestNotifyWhenBackground(t *testing.T) {
	st, fake, fix := loaded(t, team)
	msgs := locales.In("en")

	fake.Reply("worker:w1", api.ChatMessage{ID: "m0", Content: "while off"})
	fix.Await(t, func() bool { return previewOf(st, "worker:w1") == "while off" })
	st.Prefs.Notifications = "on"
	fix.Notifier.Unsupported = true
	fake.Reply("worker:w1", api.ChatMessage{ID: "m0b", Content: "unsupported"})
	fix.Await(t, func() bool { return previewOf(st, "worker:w1") == "unsupported" })
	if len(fix.Notifier.Shown) != 0 {
		t.Fatalf("notifications while off or unsupported: %v", fix.Notifier.Shown)
	}

	fix.Notifier.Unsupported = false
	fake.Reply("worker:w1", api.ChatMessage{ID: "m1", Content: "Draft  is\nready"})
	fake.Reply("bee", api.ChatMessage{ID: "m2", Kind: "handoff", Content: "Handing off", Handoff: &api.HandoffInfo{WorkerID: "w2", TaskID: "t1", TaskType: "once", ConversationID: "worker:w2"}})
	fake.Reply("bee", api.ChatMessage{ID: "m3", Content: "[file] a.png\n"})
	fake.Reply("worker:w1", api.ChatMessage{ID: "m4", MediaPaths: []string{"m4_report.pdf"}})
	fix.Await(t, func() bool { return len(fix.Notifier.Shown) == 4 })

	want := []store.Notification{
		{ID: "worker:w1/m1", Group: "worker:w1", Title: "Alice", Body: "Draft is ready", ConversationID: "worker:w1"},
		{ID: "bee/m2", Group: "bee", Title: msgs.ChatBeeLabel(), Body: msgs.ChatHandedOffTo("Bob"), ConversationID: "bee"},
		{ID: "bee/m3", Group: "bee", Title: msgs.ChatBeeLabel(), Body: msgs.NotificationsNewMessage(), ConversationID: "bee"},
		{ID: "worker:w1/m4", Group: "worker:w1", Title: "Alice", Body: msgs.ChatFilePreview(), ConversationID: "worker:w1"},
	}
	if !slices.Equal(fix.Notifier.Shown, want) {
		t.Fatalf("notifications =\n%+v\nwant\n%+v", fix.Notifier.Shown, want)
	}
}

func TestNotifyUnknownWorkerFetchesName(t *testing.T) {
	_, fake, fix := notifying(t, team)
	fake.Update(func(s *fakeapi.State) {
		s.Workers = append(s.Workers, api.Worker{ID: "w3", Name: "Cleo", Status: "idle"})
	})

	fake.Reply("worker:w3", api.ChatMessage{ID: "m1", Content: "hi"})
	fix.Await(t, func() bool { return len(fix.Notifier.Shown) == 1 })
	if got := fix.Notifier.Shown[0].Title; got != "Cleo" {
		t.Fatalf("notification title = %q, want the refetched worker name", got)
	}
	if n := countCalls(fake, "GET", "/api/workers"); n != 2 {
		t.Fatalf("worker list calls = %d, want one shared refetch", n)
	}
}

func TestNoNotifyWhenForeground(t *testing.T) {
	st, fake, fix := notifying(t, team)
	fix.Notifier.Visible = true

	fake.Reply("worker:w1", api.ChatMessage{ID: "m1", Content: "front"})
	fix.Await(t, func() bool { return previewOf(st, "worker:w1") == "front" })
	if len(fix.Notifier.Shown) != 0 {
		t.Fatalf("notification while in front: %v", fix.Notifier.Shown)
	}

	fix.Notifier.Visible = false
	fake.Reply("worker:w1", api.ChatMessage{ID: "m2", Content: "back"})
	fix.Await(t, func() bool { return len(fix.Notifier.Shown) == 1 })
	if got := fix.Notifier.Shown[0].ID; got != "worker:w1/m2" {
		t.Fatalf("notification id = %q, want worker:w1/m2", got)
	}
}

func TestNoNotifyForBeeInWorkerChat(t *testing.T) {
	_, fake, fix := notifying(t, team)

	fake.Reply("worker:w1", api.ChatMessage{ID: "m1", Sender: api.ChatSender{Type: "bee"}, Content: "Please draft"})
	fake.Reply("worker:w1", api.ChatMessage{ID: "m2", Content: "On it"})
	fix.Await(t, func() bool { return len(fix.Notifier.Shown) == 1 })
	if got := fix.Notifier.Shown[0].ID; got != "worker:w1/m2" {
		t.Fatalf("notification id = %q, want only the worker reply", got)
	}
}

func TestNotifyDeniedPersists(t *testing.T) {
	st, fake, fix := notifying(t, team)
	fix.Notifier.ShowErr = store.ErrNotificationsDenied

	fake.Reply("worker:w1", api.ChatMessage{ID: "m1", Content: "one"})
	fix.Await(t, func() bool { return st.Prefs.NotificationsDenied })
	if !fix.Prefs.Load().NotificationsDenied {
		t.Fatal("NotificationsDenied not saved")
	}

	fix.Notifier.ShowErr = nil
	fake.Reply("worker:w1", api.ChatMessage{ID: "m2", Content: "two"})
	fix.Await(t, func() bool { return len(fix.Notifier.Shown) == 1 })
	if st.Prefs.NotificationsDenied || fix.Prefs.Load().NotificationsDenied {
		t.Fatal("NotificationsDenied kept after a notification was shown")
	}
}

func TestUnreadBadgeUpdates(t *testing.T) {
	st, fake, fix := loaded(t, func(s *fakeapi.State) {
		team(s)
		s.Conversations = []api.ConversationSummary{
			{ID: "bee", Kind: "bee", Unread: 2, Activity: api.ConversationActivity{State: "idle"}},
			{ID: "worker:w1", Kind: "worker", WorkerID: "w1", Unread: 1, Activity: api.ConversationActivity{State: "idle"}},
		}
	})
	if fix.Notifier.Unread != 3 || st.UnreadTotal() != 3 {
		t.Fatalf("badge = %d, UnreadTotal = %d, want 3", fix.Notifier.Unread, st.UnreadTotal())
	}

	fake.Reply("worker:w2", api.ChatMessage{ID: "m1", Content: "new"})
	fix.Await(t, func() bool { return fix.Notifier.Unread == 4 })
	if st.UnreadTotal() != 4 {
		t.Fatalf("UnreadTotal = %d, want 4", st.UnreadTotal())
	}

	st.Logout()
	fix.Settle(t)
	if fix.Notifier.Unread != 0 {
		t.Fatalf("badge after logout = %d, want 0", fix.Notifier.Unread)
	}
}

func TestSetActiveClosesNotifications(t *testing.T) {
	st, _, fix := loaded(t, team)

	st.SetActive("worker:w1")
	if len(fix.Notifier.Closed) != 0 {
		t.Fatalf("closed in the background: %v", fix.Notifier.Closed)
	}
	fix.Notifier.Visible = true
	st.SetActive("worker:w2")
	st.SetActive("")
	if !slices.Equal(fix.Notifier.Closed, []string{"worker:w2"}) {
		t.Fatalf("closed = %v, want [worker:w2]", fix.Notifier.Closed)
	}

	st.SetActive("bee")
	st.SetForeground(false)
	st.SetForeground(true)
	if !slices.Equal(fix.Notifier.Closed, []string{"worker:w2", "bee", "bee"}) {
		t.Fatalf("closed = %v, want the active conversation closed on foreground", fix.Notifier.Closed)
	}
}

func TestSilentStreamReconnects(t *testing.T) {
	st, fake, fix := signedIn(t, team, func(o *store.Options) { o.StreamIdle = 150 * time.Millisecond })
	resyncs := 0
	off := st.OnResync(func() { resyncs++ })
	defer off()

	fix.Await(t, func() bool { return !st.Online })
	fix.Clock.AwaitSleepers(t, 2)
	fix.Await(t, func() bool { return fake.Streams() == 0 })
	fix.Clock.Advance(2 * time.Second)
	fix.Await(t, func() bool { return resyncs == 1 })
	if n := countCalls(fake, "GET", streamPath); n != 2 {
		t.Fatalf("stream requests = %d, want a reconnect after the silent stream", n)
	}
}

func TestReconnectRestartsLiveStream(t *testing.T) {
	st, fake, fix := loaded(t, team)
	resyncs := 0
	off := st.OnResync(func() { resyncs++ })
	defer off()

	st.Reconnect()
	fix.Await(t, func() bool { return resyncs == 1 && st.Online })
	if n := countCalls(fake, "GET", streamPath); n != 2 {
		t.Fatalf("stream requests = %d, want a new connection", n)
	}
	fix.Await(t, func() bool { return fake.Streams() == 1 })
	if st.Offline {
		t.Fatal("Offline after a forced reconnect")
	}
}
