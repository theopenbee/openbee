package store_test

import (
	"testing"
	"time"

	"github.com/theopenbee/openbee/desktop/internal/prefs"
	"github.com/theopenbee/openbee/desktop/internal/store"
	"github.com/theopenbee/openbee/desktop/locales"
)

func TestSetNotificationsOnConfirms(t *testing.T) {
	st, _, fix := loaded(t, team)

	st.SetNotifications(true)
	fix.Settle(t)

	if st.Prefs.Notifications != prefs.NotificationsOn || fix.Prefs.Load().Notifications != prefs.NotificationsOn {
		t.Fatalf("choice = %q / saved %q, want %q", st.Prefs.Notifications, fix.Prefs.Load().Notifications, prefs.NotificationsOn)
	}
	if len(fix.Notifier.Shown) != 1 || fix.Notifier.Shown[0].Title != locales.In("en").NotificationsEnabledTitle() {
		t.Fatalf("shown = %+v, want one confirmation titled %q", fix.Notifier.Shown, locales.In("en").NotificationsEnabledTitle())
	}
	if shown := fix.Notifier.Shown[0]; shown.ConversationID != "" || shown.Group != "" {
		t.Fatalf("confirmation = %+v, want no conversation", shown)
	}
}

func TestSetNotificationsOnDenied(t *testing.T) {
	st, _, fix := loaded(t, team)
	fix.Notifier.ShowErr = store.ErrNotificationsDenied

	st.SetNotifications(true)
	fix.Settle(t)

	if st.Prefs.Notifications != prefs.NotificationsOn || !st.Prefs.NotificationsDenied || !fix.Prefs.Load().NotificationsDenied {
		t.Fatalf("prefs = %+v / saved %+v, want on and denied", st.Prefs, fix.Prefs.Load())
	}

	fix.Notifier.ShowErr = nil
	st.SetNotifications(true)
	fix.Settle(t)
	if st.Prefs.NotificationsDenied || fix.Prefs.Load().NotificationsDenied {
		t.Fatal("NotificationsDenied kept after the confirmation was shown")
	}
}

func TestSetNotificationsOffStaysQuiet(t *testing.T) {
	st, _, fix := loaded(t, team)
	st.Prefs.Notifications = prefs.NotificationsOn

	st.SetNotifications(false)
	fix.Settle(t)

	if st.Prefs.Notifications != prefs.NotificationsOff || fix.Prefs.Load().Notifications != prefs.NotificationsOff {
		t.Fatalf("choice = %q / saved %q, want %q", st.Prefs.Notifications, fix.Prefs.Load().Notifications, prefs.NotificationsOff)
	}
	if len(fix.Notifier.Shown) != 0 {
		t.Fatalf("shown = %+v, want nothing when turning off", fix.Notifier.Shown)
	}
}

func TestSetNotificationsUnsupportedSkipsConfirmation(t *testing.T) {
	st, _, fix := loaded(t, team)
	fix.Notifier.Unsupported = true

	st.SetNotifications(true)
	fix.Settle(t)

	if st.NotificationsSupported() {
		t.Fatal("NotificationsSupported true for an unsupported notifier")
	}
	if len(fix.Notifier.Shown) != 0 {
		t.Fatalf("shown = %+v, want no confirmation without support", fix.Notifier.Shown)
	}
}

func TestDismissNotificationsHintPersists(t *testing.T) {
	st, _, fix := loaded(t, team)

	st.DismissNotificationsHint()
	fix.Settle(t)

	if !st.Prefs.NotificationsHintDismissed || !fix.Prefs.Load().NotificationsHintDismissed {
		t.Fatalf("prefs = %+v / saved %+v, want the hint dismissed", st.Prefs, fix.Prefs.Load())
	}
}

type heldNotifier struct {
	shown   []store.Notification
	answers []func(error)
}

func (n *heldNotifier) Supported() bool { return true }

func (n *heldNotifier) Show(x store.Notification, done func(error)) {
	n.shown = append(n.shown, x)
	n.answers = append(n.answers, done)
}

func (n *heldNotifier) CloseConversation(string) {}

func (n *heldNotifier) SetUnread(int) {}

func (n *heldNotifier) Foreground() bool { return false }

func TestNotificationsDeniedWhenShowAnswersLater(t *testing.T) {
	saved := prefs.NewMemoryStore(prefs.Prefs{})
	n := &heldNotifier{}
	st := store.New(store.Options{Post: func(fn func()) { fn() }, Prefs: saved, Tokens: prefs.NewMemoryTokenStore(), Notifier: n})

	st.SetNotifications(true)
	if len(n.shown) != 1 || n.shown[0].Title != locales.In("en").NotificationsEnabledTitle() {
		t.Fatalf("shown = %+v, want the confirmation", n.shown)
	}
	if st.Prefs.NotificationsDenied {
		t.Fatal("NotificationsDenied set before Show answered")
	}

	n.answers[0](store.ErrNotificationsDenied)
	if !st.Flush(time.Second) {
		t.Fatal("Flush timed out")
	}
	if !st.Prefs.NotificationsDenied || !saved.Load().NotificationsDenied {
		t.Fatalf("prefs = %+v / saved %+v, want denied after the late answer", st.Prefs, saved.Load())
	}

	st.SetNotifications(true)
	n.answers[1](nil)
	if !st.Flush(time.Second) {
		t.Fatal("Flush timed out")
	}
	if st.Prefs.NotificationsDenied || saved.Load().NotificationsDenied {
		t.Fatal("NotificationsDenied kept after a later Show succeeded")
	}
}
