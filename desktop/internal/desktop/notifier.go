package desktop

import (
	"errors"
	"slices"
	"strings"

	"github.com/egoist/mygo"
	"github.com/theopenbee/openbee/desktop/internal/model"
	"github.com/theopenbee/openbee/desktop/internal/store"
)

const appTitle = "OpenBee"

type Notifier struct {
	win       func() *mygo.Window
	open      func(cid string)
	checked   bool
	supported bool
	unread    int
	shown     map[string][]string
}

func NewNotifier(win func() *mygo.Window, open func(cid string)) *Notifier {
	return &Notifier{win: win, open: open, shown: map[string][]string{}}
}

func TitleWithUnread(n int) string {
	if badge := model.BadgeText(n); badge != "" {
		return "(" + badge + ") " + appTitle
	}
	return appTitle
}

func (n *Notifier) Supported() bool {
	if !n.checked {
		n.supported = mygo.NotificationsSupported()
		n.checked = true
	}
	return n.supported
}

func (n *Notifier) Show(x store.Notification, done func(error)) {
	if x.ConversationID != "" {
		n.track(x.ConversationID, x.ID)
	}
	note := mygo.NewNotification(mygo.NotificationOptions{ID: x.ID, Group: x.Group, Title: x.Title, Body: x.Body})
	go func() {
		err := note.Show()
		if errors.Is(err, mygo.ErrNotificationsDenied) {
			err = store.ErrNotificationsDenied
		}
		if w := n.win(); w != nil {
			w.Update(func() { done(err) })
		}
	}()
}

func (n *Notifier) CloseConversation(cid string) {
	for _, id := range n.shown[cid] {
		closeNotification(id)
	}
	delete(n.shown, cid)
}

func (n *Notifier) SetUnread(total int) {
	n.unread = total
	mygo.App.SetBadgeCount(total)
	if w := n.win(); w != nil {
		w.SetTitle(TitleWithUnread(total))
	}
}

func (n *Notifier) Foreground() bool {
	w := n.win()
	return w != nil && mygo.App.IsActive() && w.IsFocused()
}

func (n *Notifier) Click(id string) {
	cid, _, found := strings.Cut(id, "/")
	if !found || cid == "" {
		closeNotification(id)
		n.open("")
		return
	}
	n.track(cid, id)
	n.open(cid)
}

func (n *Notifier) track(cid, id string) {
	if !slices.Contains(n.shown[cid], id) {
		n.shown[cid] = append(n.shown[cid], id)
	}
}

func closeNotification(id string) {
	mygo.NewNotification(mygo.NotificationOptions{ID: id}).Close()
}
