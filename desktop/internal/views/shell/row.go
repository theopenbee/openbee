package shell

import (
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/api"
	"github.com/theopenbee/openbee/desktop/internal/kit"
	"github.com/theopenbee/openbee/desktop/internal/model"
	"github.com/theopenbee/openbee/desktop/internal/theme"
	"github.com/theopenbee/openbee/desktop/internal/views/route"
	"github.com/theopenbee/openbee/desktop/locales"
)

const (
	avatarSize    = 36
	rowPadX       = 12
	rowPadY       = 10
	rowContentGap = 12
	lineGap       = 2
	lineInnerGap  = 8
	previewHeight = 20
	nameSize      = 14
	metaSize      = 12
)

var rowFade = ui.Animation{Duration: 150 * time.Millisecond, Ease: ui.CubicBezier(0.22, 1, 0.36, 1)}

func (sh *Shell) row(c *ui.Context, tk theme.Tokens, conv api.ConversationSummary, workers map[string]api.Worker, active bool, now time.Time, lang string) {
	w, isWorker := workers[conv.WorkerID]
	name := locales.ChatBeeLabel()
	if isWorker {
		name = w.Name
	}
	b := ui.ButtonBase(c).Label(name).Role(ui.RoleLink).Justify(ui.Start).AlignItems(ui.Center).Gap(rowContentGap).
		Padding(rowPadY, rowPadX).Radius(theme.RadiusControl).MinWidth(0)
	bg := tk.SurfaceMuted.Alpha(0)
	switch {
	case active:
		bg = tk.AccentSubtle
	case b.Hovered() || b.Pressed():
		bg = tk.SurfaceMuted
	}
	b.Background(b.AnimateColor("bg", bg, rowFade)).OnClick(func() { sh.nav.Push(route.Chat(route.Param(conv.ID))) })
	b.Children(func() {
		if isWorker {
			kit.WorkerAvatar(c, tk, w.ID, w.Name, w.Status, kit.AvatarMD, true)
		} else {
			kit.BeeAvatar(c, tk, kit.AvatarMD)
		}
		ui.Column(c).Grow(1).MinWidth(0).Gap(lineGap).Children(func() {
			ui.Row(c).AlignItems(ui.Baseline).Gap(lineInnerGap).Children(func() {
				ui.Text(c, name).FontSize(nameSize).FontWeight(mediumWeight).TextColor(tk.Foreground).SingleLine().MinWidth(0)
				if last := conv.LastMessage; last != nil {
					ui.Text(c, listTime(last.TS, now, lang)).FontSize(metaSize).TextColor(tk.TextMuted).
						FontFeatures("tnum").SingleLine().Shrink(0).MarginStart(ui.Auto)
				}
			})
			ui.Row(c).Height(previewHeight).Gap(lineInnerGap).Children(func() {
				color := tk.TextSecondary
				if conv.LastMessage == nil {
					color = tk.TextMuted
				}
				ui.Text(c, previewText(model.ConversationPreview(conv, workers))).FontSize(metaSize).TextColor(color).
					SingleLine().MinWidth(0)
				if conv.Unread > 0 {
					kit.SmallBadge(c, tk, model.BadgeText(conv.Unread), kit.ToneInfo).MarginStart(ui.Auto)
				}
			})
		})
	})
}

func listTime(ts int64, now time.Time, lang string) string {
	text, yesterday := model.ListTime(ts, now, lang)
	if yesterday {
		return locales.ChatYesterday()
	}
	return text
}

func previewText(p model.Preview) string {
	switch p.Kind {
	case model.PreviewHandoff:
		name := p.WorkerName
		if p.WorkerDeleted {
			name = locales.ChatDeletedWorker()
		}
		return locales.ChatHandedOffTo(name)
	case model.PreviewYou:
		return locales.ChatYouPrefix() + p.Text
	}
	return p.Text
}
