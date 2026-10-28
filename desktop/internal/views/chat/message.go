package chat

import (
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/api"
	"github.com/theopenbee/openbee/desktop/internal/icons"
	"github.com/theopenbee/openbee/desktop/internal/kit"
	"github.com/theopenbee/openbee/desktop/internal/model"
	"github.com/theopenbee/openbee/desktop/internal/theme"
	"github.com/theopenbee/openbee/desktop/locales"
)

const (
	metaSize        = 12
	metaGapX        = 8
	metaGapY        = 4
	userMetaBelow   = 8
	senderMetaBelow = 6
	avatarColumn    = 28
	avatarGap       = 12
	systemGlyph     = 14
	bubblePadX      = 16
	bubblePadY      = 10
	bodyLineHeight  = 1.65
	userBubbleShare = 85
	userBubbleMax   = 576
	userCopyGap     = 4
	senderCopyGap   = 2
	copyTop         = 4
	collapseHeight  = 640
	collapseFade    = 64
	toggleTop       = 4
	togglePadY      = 4
	senderUser      = "user"
	senderBee       = "bee"
	senderWorker    = "worker"
	senderSystem    = "system"
	kindReset       = "reset"
	kindHandoff     = "handoff"
)

type rowContext struct {
	pane
	tk      theme.Tokens
	workers map[string]api.Worker
	now     time.Time
	lang    string
}

type sender struct {
	kind, name, workerID string
	viaBee               bool
}

type measureKey struct{}

func senderDisplay(m api.ChatMessage, cid string, workers map[string]api.Worker) sender {
	chatWorker, inWorkerChat := api.ConversationWorkerID(cid)
	switch m.Sender.Type {
	case senderUser:
		return sender{kind: senderUser, name: locales.ChatYou()}
	case senderBee:
		return sender{kind: senderBee, name: locales.ChatBeeLabel(), viaBee: inWorkerChat}
	case senderWorker:
		id := m.Sender.ID
		if id == "" {
			id = chatWorker
		}
		name := locales.ChatDeletedWorker()
		if w, ok := workers[id]; ok {
			name = w.Name
		}
		return sender{kind: senderWorker, name: name, workerID: id}
	}
	return sender{kind: senderSystem, name: locales.ChatSystem()}
}

func (v *View) message(c *ui.Context, rc rowContext, i int) {
	m := rc.cs.msgs[i]
	who := senderDisplay(m.ChatMessage, rc.cid, rc.workers)
	stamp := model.MessageTime(m.TS, rc.now, rc.lang)
	if who.kind == senderUser {
		v.userMessage(c, rc, m, stamp, model.StartsGroup(rc.cs.msgs, i))
		return
	}
	v.senderMessage(c, rc, m, who, stamp)
}

func (v *View) userMessage(c *ui.Context, rc rowContext, m model.Message, stamp string, starts bool) {
	tk := rc.tk
	group := ui.Column(c).AlignItems(ui.End).MinWidth(0)
	reveal := group.Hovered() || group.FocusWithin()
	group.Children(func() {
		if starts && stamp != "" {
			ui.Row(c).Label(locales.ChatYou()).Margin(0, 0, userMetaBelow, 0).Children(func() {
				ui.Text(c, stamp).FontSize(metaSize).TextColor(tk.TextMuted).FontFeatures("tnum").SingleLine()
			})
		}
		v.attachments(c, rc.tk, m, true)
		if hasText(m.Content) {
			ui.Column(c).MaxWidthPercent(userBubbleShare).MinWidth(0).Children(func() {
				ui.Column(c).MaxWidth(userBubbleMax).MinWidth(0).Children(func() {
					v.bubble(c, rc, m, reveal, userCopyGap)
				})
			})
		}
	})
}

func (v *View) senderMessage(c *ui.Context, rc rowContext, m model.Message, who sender, stamp string) {
	tk := rc.tk
	row := ui.Row(c).AlignItems(ui.Start).Gap(avatarGap).MinWidth(0)
	reveal := row.Hovered() || row.FocusWithin()
	row.Children(func() {
		ui.Box(c).Width(avatarColumn).Shrink(0).Children(func() { senderAvatar(c, tk, who, rc.workers) })
		ui.Column(c).Grow(1).MinWidth(0).AlignItems(ui.Start).Children(func() {
			ui.Row(c).MinHeight(avatarColumn).MinWidth(0).GapX(metaGapX).GapY(metaGapY).Wrap().
				Margin(0, 0, senderMetaBelow, 0).FontSize(metaSize).Children(func() {
				ui.Text(c, who.name).FontWeight(mediumWeight).TextColor(tk.Foreground).SingleLine().MinWidth(0)
				if who.viaBee {
					kit.SmallBadge(c, tk, locales.ChatFromBee(), kit.ToneNeutral)
				}
				if stamp != "" {
					ui.Text(c, stamp).TextColor(tk.TextMuted).FontFeatures("tnum").SingleLine()
				}
			})
			switch {
			case m.Kind == kindHandoff && m.Handoff != nil:
				v.handoffCard(c, tk, m, rc.workers)
			default:
				v.attachments(c, tk, m, false)
				if hasText(m.Content) {
					ui.Column(c).MaxWidthPercent(100).MinWidth(0).Children(func() {
						v.bubble(c, rc, m, reveal, senderCopyGap)
					})
				}
			}
		})
	})
}

func senderAvatar(c *ui.Context, tk theme.Tokens, who sender, workers map[string]api.Worker) {
	switch who.kind {
	case senderBee:
		kit.BeeAvatar(c, tk, kit.AvatarSM)
	case senderWorker:
		if w, ok := workers[who.workerID]; ok {
			kit.WorkerAvatar(c, tk, w.ID, w.Name, w.Status, kit.AvatarSM, true)
		} else {
			kit.UnknownAvatar(c, tk, kit.AvatarSM)
		}
	default:
		ui.Box(c).Size(avatarColumn, avatarColumn).Center().Radius(theme.RadiusPill).Background(tk.SurfaceMuted).
			TextColor(tk.TextSecondary).Children(func() {
			ui.Icon(c, icons.Get("info")).Size(systemGlyph, systemGlyph)
		})
	}
}

func (v *View) bubble(c *ui.Context, rc rowContext, m model.Message, reveal bool, copyGap float32) {
	tk := rc.tk
	ui.Column(c).MinWidth(0).Children(func() {
		ui.Column(c).MinWidth(0).Radius(theme.RadiusPanel).Background(tk.SurfaceMuted).Padding(bubblePadY, bubblePadX).
			Children(func() { v.expandable(c, rc, m) })
		copyButton := kit.CopyIconButton(c, tk, m.Content, locales.ChatCopyMessage()).
			Absolute().Top(copyTop).RightPercent(100).Translate(-copyGap, 0)
		shown := float32(0)
		if reveal {
			shown = 1
		}
		copyButton.Opacity(copyButton.AnimateTo("reveal", shown, colorFade))
	})
}

func (v *View) expandable(c *ui.Context, rc rowContext, m model.Message) {
	tk, cs := rc.tk, rc.cs
	expanded := cs.expanded[m.ID]
	overflows := false
	clip := ui.Column(c).MinWidth(0).Clip()
	if !expanded {
		clip.MaxHeight(collapseHeight)
	}
	clip.Children(func() {
		inner := ui.Column(c).Shrink(0).MinWidth(0)
		overflows = measuredOverflow(c, inner)
		inner.Children(func() { v.renderBody(c, m) })
		if overflows && !expanded {
			ui.Box(c).Absolute().Left(0).Right(0).Bottom(0).Height(collapseFade).
				Gradient(tk.SurfaceMuted.Alpha(0), tk.SurfaceMuted, 180)
		}
	})
	if !overflows {
		return
	}
	label := locales.CommonShowMore()
	if expanded {
		label = locales.CommonShowLess()
	}
	b := ui.ButtonBase(c).Label(label).Expanded(expanded).AlignSelf(ui.Start).Margin(toggleTop, 0, 0, 0).PaddingY(togglePadY).
		Radius(theme.RadiusControl).FontSize(metaSize).FontWeight(mediumWeight)
	fg := tk.TextSecondary
	if b.Hovered() || b.Pressed() {
		fg = tk.Foreground
	}
	b.TextColor(b.AnimateColor("fg", fg, colorFade)).OnClick(func() {
		if cs.list.AtEnd() && cs.scroll.Y > 0 {
			cs.scroll.Y--
		}
		cs.expanded[m.ID] = !expanded
	}).Children(func() { ui.Text(c, label).SingleLine() })
}

func measuredOverflow(c *ui.Context, inner ui.Element) bool {
	over := ui.Local(inner, measureKey{}, func() bool { return false })
	if b := inner.LayoutBounds(); b.H > 0 {
		*over = b.H > collapseHeight
	}
	decided, services := *over, c.Services()
	inner.Draw(func(_ *ui.Painter, r ui.Rect) {
		if r.H > collapseHeight != decided {
			services.Invalidate()
		}
	})
	return decided
}

func hasText(s string) bool {
	return strings.TrimSpace(s) != ""
}
