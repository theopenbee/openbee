package store_test

import (
	"testing"

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
