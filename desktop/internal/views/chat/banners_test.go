package chat_test

import (
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/theopenbee/openbee/desktop/internal/api"
	"github.com/theopenbee/openbee/desktop/internal/prefs"
	"github.com/theopenbee/openbee/desktop/internal/store"
	"github.com/theopenbee/openbee/desktop/internal/views/chat"
	"github.com/theopenbee/openbee/desktop/locales"
)

const (
	streamPath   = "/api/local/stream"
	graceWindow  = 4 * time.Second
	beforeGrace  = 3 * time.Second
	afterBackoff = graceWindow - beforeGrace
)

func (f *fixture) dropStream() {
	f.t.Helper()
	f.Fake.FailNext(http.MethodGet, streamPath, http.StatusServiceUnavailable, map[string]any{"error": "down"})
	f.Fake.DropStreams()
	f.Fix.Await(f.t, func() bool { return !f.Store.Online })
	f.Fix.Clock.AwaitSleepers(f.t, 2)
}

func (f *fixture) goOffline() {
	f.t.Helper()
	f.dropStream()
	f.passGrace()
}

func (f *fixture) passGrace() {
	f.t.Helper()
	f.Fix.Clock.Advance(beforeGrace)
	f.Fix.Await(f.t, func() bool { return len(f.Fake.Calls(http.MethodGet, streamPath)) == 2 && f.Fix.Clock.Sleepers() == 2 })
	f.settleMotion()
	f.wantHidden(locales.ChatConnectionLost())
	f.Fix.Clock.Advance(afterBackoff)
	f.Fix.Await(f.t, func() bool { return f.Store.Offline })
	f.settleMotion()
}

func occurrences(texts []string, text string) int {
	n := 0
	for _, t := range texts {
		if t == text {
			n++
		}
	}
	return n
}

func lastIndex(texts []string, text string) int {
	for i := len(texts) - 1; i >= 0; i-- {
		if texts[i] == text {
			return i
		}
	}
	return -1
}

func (f *fixture) activityOf(cid string) string {
	for _, sum := range f.Store.Conversations.Data {
		if sum.ID == cid {
			return sum.Activity.State
		}
	}
	return ""
}

func (f *fixture) wantActivityAboveComposer(status, composerHint string) {
	f.t.Helper()
	texts := f.T.Texts()
	if n := occurrences(texts, status); n != 2 {
		f.t.Fatalf("%q shown %d times, want the header and the activity indicator; texts %q", status, n, texts)
	}
	at := lastIndex(texts, status)
	hint := slices.Index(texts, composerHint)
	if hint < 0 || at > hint {
		f.t.Fatalf("activity status at %d, composer hint at %d; want the status above the composer; texts %q", at, hint, texts)
	}
}

func TestHintState(t *testing.T) {
	cases := []struct {
		name      string
		p         prefs.Prefs
		supported bool
		want      chat.Hint
	}{
		{"unsupported", prefs.Prefs{}, false, chat.HintNone},
		{"unsupported and denied", prefs.Prefs{Notifications: prefs.NotificationsOn, NotificationsDenied: true}, false, chat.HintNone},
		{"choice unset", prefs.Prefs{}, true, chat.HintPrompt},
		{"choice on", prefs.Prefs{Notifications: prefs.NotificationsOn}, true, chat.HintNone},
		{"choice off", prefs.Prefs{Notifications: prefs.NotificationsOff}, true, chat.HintNone},
		{"dismissed prompt", prefs.Prefs{NotificationsHintDismissed: true}, true, chat.HintNone},
		{"denied", prefs.Prefs{Notifications: prefs.NotificationsOn, NotificationsDenied: true}, true, chat.HintBlocked},
		{"denied with unset choice", prefs.Prefs{NotificationsDenied: true}, true, chat.HintBlocked},
		{"denied but dismissed", prefs.Prefs{Notifications: prefs.NotificationsOn, NotificationsDenied: true, NotificationsHintDismissed: true}, true, chat.HintNone},
		{"denied but off", prefs.Prefs{Notifications: prefs.NotificationsOff, NotificationsDenied: true}, true, chat.HintNone},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := chat.HintState(c.p, c.supported); got != c.want {
				t.Fatalf("HintState(%+v, %v) = %v, want %v", c.p, c.supported, got, c.want)
			}
		})
	}
}

func TestBannerAfterGrace(t *testing.T) {
	f := newFixture(t, wideW, wideH, "w1", seedChat(team(), workerCID))
	f.wantHidden(locales.ChatConnectionLost())

	f.dropStream()
	f.settleMotion()
	f.wantHidden(locales.ChatConnectionLost())

	f.passGrace()
	f.wantShown(locales.ChatConnectionLost(), locales.ChatConnectionLostDescription(), locales.ChatReconnectNow())
}

func TestBannerReconnectNow(t *testing.T) {
	f := newFixture(t, wideW, wideH, "w1", seedChat(team(), workerCID))
	f.goOffline()
	f.wantShown(locales.ChatConnectionLost(), locales.ChatConnectionLostDescription(), locales.ChatReconnectNow())
	if f.Store.Online {
		t.Fatal("Online while the banner shows")
	}

	f.click(locales.ChatReconnectNow())
	f.Fix.Await(t, func() bool { return f.Store.Online })
	f.settleMotion()
	f.wantHidden(locales.ChatConnectionLost(), locales.ChatReconnectNow())
	if n := len(f.Fake.Calls(http.MethodGet, streamPath)); n != 3 {
		t.Fatalf("%d stream calls, want 3: the first connect, the failed retry and the manual reconnect", n)
	}
}

func TestActivityWhileBusy(t *testing.T) {
	f := newFixture(t, wideW, wideH, "w1", both(seedChat(team(), workerCID, message("m1", "worker", "w1", "Draft is ready", time.Now())), activity("idle")))
	idle := f.T.Texts()
	if n := occurrences(idle, locales.ChatStatusRunning()); n != 0 {
		t.Fatalf("running status shown %d times while idle; texts %q", n, idle)
	}

	f.Fake.Emit(api.ChatEvent{Type: "task", WorkerID: "w1", State: "running"})
	f.Fix.Await(t, func() bool { return f.activityOf(workerCID) == "running" })
	f.settleMotion()
	f.wantActivityAboveComposer(locales.ChatStatusRunning(), locales.ChatComposerHintPlain())

	f.Fake.Emit(api.ChatEvent{Type: "task", WorkerID: "w1", State: "queued", QueuedAhead: 2})
	f.Fix.Await(t, func() bool { return f.activityOf(workerCID) == "queued" })
	f.settleMotion()
	f.wantActivityAboveComposer(locales.ChatStatusQueued(2), locales.ChatComposerHintPlain())

	f.Fake.Emit(api.ChatEvent{Type: "task", WorkerID: "w1", State: "idle"})
	f.Fix.Await(t, func() bool { return f.activityOf(workerCID) == "idle" })
	f.settleMotion()
	f.wantHidden(locales.ChatStatusRunning(), locales.ChatStatusQueued(2))
}

func TestBeeBusyActivity(t *testing.T) {
	f := newFixture(t, wideW, wideH, api.BeeConversation, seedChat(team(), api.BeeConversation, message("m1", "bee", "", "Morning", time.Now())))
	f.wantHidden(locales.ChatStatusRunning())

	f.Store.Chat(api.BeeConversation).BeeBusy = true
	f.settleMotion()
	f.wantActivityAboveComposer(locales.ChatStatusRunning(), locales.ChatComposerHint())

	f.Fake.Reply(api.BeeConversation, api.ChatMessage{ID: "b1", Sender: api.ChatSender{Type: "bee"}, Content: "On it"})
	f.Fix.Await(t, func() bool { return !f.Store.Chat(api.BeeConversation).BeeBusy })
	f.settleMotion()
	f.wantHidden(locales.ChatStatusRunning())
}

func TestHintPromptShowsAndDismisses(t *testing.T) {
	f := newFixture(t, wideW, wideH, "w1", seedChat(team(), workerCID))
	f.wantShown(locales.NotificationsPromptTitle(), locales.NotificationsPromptDescription(), locales.NotificationsEnable())
	f.wantHidden(locales.NotificationsBlockedDesktopTitle())

	f.click(locales.ArcAlertDismiss(locales.NotificationsPromptTitle()))
	f.Fix.Settle(t)
	f.wantHidden(locales.NotificationsPromptTitle())
	if !f.Store.Prefs.NotificationsHintDismissed || !f.Fix.Prefs.Load().NotificationsHintDismissed {
		t.Fatalf("prefs = %+v / saved %+v, want the hint dismissed", f.Store.Prefs, f.Fix.Prefs.Load())
	}
}

func TestHintEnableTurnsOn(t *testing.T) {
	f := newFixture(t, wideW, wideH, "w1", seedChat(team(), workerCID))

	f.click(locales.NotificationsEnable())
	f.Fix.Settle(t)
	if f.Store.Prefs.Notifications != prefs.NotificationsOn || f.Fix.Prefs.Load().Notifications != prefs.NotificationsOn {
		t.Fatalf("choice = %q / saved %q, want %q", f.Store.Prefs.Notifications, f.Fix.Prefs.Load().Notifications, prefs.NotificationsOn)
	}
	if shown := f.Fix.Notifier.Shown; len(shown) != 1 || shown[0].Title != locales.NotificationsEnabledTitle() {
		t.Fatalf("shown = %+v, want one %q confirmation", shown, locales.NotificationsEnabledTitle())
	}
	f.wantHidden(locales.NotificationsPromptTitle(), locales.NotificationsBlockedDesktopTitle())
}

func TestHintBlockedAfterDenial(t *testing.T) {
	f := newFixture(t, wideW, wideH, "w1", seedChat(team(), workerCID))
	f.Fix.Notifier.ShowErr = store.ErrNotificationsDenied

	f.click(locales.NotificationsEnable())
	f.Fix.Settle(t)
	f.settleMotion()
	f.wantShown(locales.NotificationsBlockedDesktopTitle(), locales.NotificationsBlockedDesktopDescription())
	f.wantHidden(locales.NotificationsPromptTitle(), locales.NotificationsEnable(), locales.NotificationsBlockedTitle())

	f.click(locales.ArcAlertDismiss(locales.NotificationsBlockedDesktopTitle()))
	f.Fix.Settle(t)
	f.wantHidden(locales.NotificationsBlockedDesktopTitle())
}

func TestHintHiddenWithoutSupport(t *testing.T) {
	f := newFixture(t, wideW, wideH, "w1", seedChat(team(), workerCID))
	f.wantShown(locales.NotificationsPromptTitle())

	f.Fix.Notifier.Unsupported = true
	f.settleMotion()
	f.wantHidden(locales.NotificationsPromptTitle())

	f.Store.Prefs.NotificationsDenied = true
	f.settleMotion()
	f.wantHidden(locales.NotificationsBlockedDesktopTitle())
}
