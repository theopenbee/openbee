package store

import (
	"errors"
	"log"

	"github.com/theopenbee/openbee/desktop/internal/api"
	"github.com/theopenbee/openbee/desktop/internal/model"
	"github.com/theopenbee/openbee/desktop/internal/prefs"
	"github.com/theopenbee/openbee/desktop/locales"
)

const confirmationID = "notifications-enabled"

var ErrNotificationsDenied = errors.New("notifications are not allowed")

func (s *Store) SetActive(conversationID string) {
	s.active = conversationID
	if s.o.Notifier.Foreground() {
		s.closeActive()
	}
}

func (s *Store) Active() string {
	return s.active
}

func (s *Store) UnreadTotal() int {
	return model.UnreadTotal(s.summaries())
}

func (s *Store) closeActive() {
	if s.active != "" {
		s.o.Notifier.CloseConversation(s.active)
	}
}

func (s *Store) syncUnread() {
	s.setUnread(s.UnreadTotal())
}

func (s *Store) setUnread(n int) {
	if n == s.unread {
		return
	}
	s.unread = n
	s.o.Notifier.SetUnread(n)
}

func (s *Store) attend(ev api.ChatEvent, unknownWorker bool) {
	if !s.wantsNotification(ev) {
		return
	}
	if unknownWorker || !s.Workers.Loaded {
		s.Workers.refetch(func() {
			if s.wantsNotification(ev) {
				s.notify(ev)
			}
		})
		return
	}
	s.notify(ev)
}

func (s *Store) wantsNotification(ev api.ChatEvent) bool {
	n := s.o.Notifier
	return s.Prefs.Notifications == prefs.NotificationsOn && n.Supported() && model.ShouldNotify(ev, n.Foreground())
}

func (s *Store) notify(ev api.ChatEvent) {
	s.show(s.notification(ev))
}

func (s *Store) show(n Notification) {
	s.o.Notifier.Show(n, s.notificationShown)
}

func (s *Store) notificationShown(err error) {
	switch {
	case errors.Is(err, ErrNotificationsDenied):
		s.setNotificationsDenied(true)
	case err != nil:
		log.Printf("store: show notification: %v", err)
	default:
		s.setNotificationsDenied(false)
	}
}

func (s *Store) NotificationsSupported() bool {
	return s.o.Notifier.Supported()
}

func (s *Store) SetNotifications(on bool) {
	choice := prefs.NotificationsOff
	if on {
		choice = prefs.NotificationsOn
	}
	s.Prefs.Notifications = choice
	s.savePrefs()
	if on && s.NotificationsSupported() {
		s.show(Notification{ID: confirmationID, Title: locales.In(s.language()).NotificationsEnabledTitle()})
	}
}

func (s *Store) DismissNotificationsHint() {
	s.Prefs.NotificationsHintDismissed = true
	s.savePrefs()
}

func (s *Store) setNotificationsDenied(denied bool) {
	if s.Prefs.NotificationsDenied == denied {
		return
	}
	s.Prefs.NotificationsDenied = denied
	s.savePrefs()
}

func (s *Store) notification(ev api.ChatEvent) Notification {
	msgs := locales.In(s.language())
	cid := ev.ConversationID
	m := *ev.Message
	workers := s.WorkerMap()
	title := msgs.ChatBeeLabel()
	if id, ok := api.ConversationWorkerID(cid); ok {
		title = msgs.ChatDeletedWorker()
		if w, found := workers[id]; found {
			title = w.Name
		}
	}
	last := &api.LastMessage{Sender: m.Sender, Kind: m.Kind, Preview: model.PreviewText(m.Content), TS: m.TS}
	if m.Handoff != nil {
		last.HandoffWorkerID = m.Handoff.WorkerID
	}
	body := previewBody(msgs, model.ConversationPreview(api.ConversationSummary{ID: cid, LastMessage: last}, workers))
	if body == "" {
		body = msgs.NotificationsNewMessage()
	}
	return Notification{ID: cid + "/" + m.ID, Group: cid, Title: title, Body: body, ConversationID: cid}
}

func previewBody(msgs locales.Messages, p model.Preview) string {
	switch p.Kind {
	case model.PreviewHandoff:
		name := p.WorkerName
		if p.WorkerDeleted {
			name = msgs.ChatDeletedWorker()
		}
		return msgs.ChatHandedOffTo(name)
	case model.PreviewYou:
		return msgs.ChatYouPrefix() + p.Text
	}
	return p.Text
}
