package chat

import (
	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/kit"
	"github.com/theopenbee/openbee/desktop/internal/prefs"
	"github.com/theopenbee/openbee/desktop/internal/theme"
	"github.com/theopenbee/openbee/desktop/locales"
)

const bannerGap = 12

type hint int

const (
	hintNone hint = iota
	hintPrompt
	hintBlocked
)

type connectionKey struct{}

type hintKey struct{}

var bannerMotion = ui.ElementTransition{
	Animation: &ui.Smooth,
	Enter:     &ui.Motion{Collapse: true},
	Exit:      &ui.Motion{Collapse: true},
}

func hintState(p prefs.Prefs, supported bool) hint {
	switch {
	case !supported, p.NotificationsHintDismissed, p.Notifications == prefs.NotificationsOff:
		return hintNone
	case p.NotificationsDenied:
		return hintBlocked
	case p.Notifications == "":
		return hintPrompt
	}
	return hintNone
}

func (v *View) connectionBanner(c *ui.Context, tk theme.Tokens) {
	if !v.s.Offline {
		return
	}
	ui.Column(c).Key(connectionKey{}).Shrink(0).Transition(bannerMotion).Children(func() {
		column(c, func() {
			kit.Alert(c, tk, kit.AlertOpts{
				Tone:        kit.ToneWarning,
				Title:       locales.ChatConnectionLost(),
				Body:        locales.ChatConnectionLostDescription(),
				ActionLabel: locales.ChatReconnectNow(),
				OnAction:    v.s.ReconnectNow,
			}).Margin(0, 0, bannerGap, 0)
		})
	})
}

func (v *View) notificationHint(c *ui.Context, tk theme.Tokens) {
	state := hintState(v.s.Prefs, v.s.NotificationsSupported())
	if state == hintNone {
		return
	}
	alert := kit.AlertOpts{OnDismiss: v.s.DismissNotificationsHint}
	if state == hintBlocked {
		alert.Tone = kit.ToneWarning
		alert.Title = locales.NotificationsBlockedTitle()
		alert.Body = locales.NotificationsBlockedDesktopDescription()
	} else {
		alert.Tone = kit.ToneInfo
		alert.Title = locales.NotificationsPromptTitle()
		alert.Body = locales.NotificationsPromptDescription()
		alert.ActionLabel = locales.NotificationsEnable()
		alert.OnAction = func() { v.s.SetNotifications(true) }
	}
	ui.Column(c).Key(hintKey{}).Shrink(0).Transition(bannerMotion).Children(func() {
		column(c, func() {
			kit.Alert(c, tk, alert).Margin(bannerGap, 0, 0, 0)
		})
	})
}
