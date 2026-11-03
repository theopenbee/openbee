package chat

import (
	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/api"
	"github.com/theopenbee/openbee/desktop/internal/icons"
	"github.com/theopenbee/openbee/desktop/internal/kit"
	"github.com/theopenbee/openbee/desktop/internal/model"
	"github.com/theopenbee/openbee/desktop/internal/theme"
	"github.com/theopenbee/openbee/desktop/internal/views/route"
	"github.com/theopenbee/openbee/desktop/locales"
)

const (
	handoffMaxWidth = 448
	handoffPadX     = 16
	handoffPadY     = 14
	handoffGap      = 12
	handoffLineGap  = 4
	handoffTitleGap = 6
	handoffArrow    = 16
	handoffNudge    = 2
	handoffDimmed   = 0.6
	handoffPreview  = 3
	taskRowGap      = 8
	taskRowTop      = 4
	taskCountdown   = "countdown"
	taskScheduled   = "scheduled"
)

func taskTypeLabel(taskType string) string {
	switch taskType {
	case taskCountdown:
		return locales.ChatTaskTypeCountdown()
	case taskScheduled:
		return locales.ChatTaskTypeScheduled()
	}
	return ""
}

func (v *View) handoffCard(c *ui.Context, tk theme.Tokens, m model.Message, workers map[string]api.Worker) {
	info := m.Handoff
	w, known := workers[info.WorkerID]
	name := locales.ChatDeletedWorker()
	if known {
		name = w.Name
	}
	title := locales.ChatHandedOffTo(name)
	b := ui.ButtonBase(c).Label(title).Disabled(!known).AlignItems(ui.Start).Justify(ui.Start).Gap(handoffGap).
		Padding(handoffPadY, handoffPadX).Radius(theme.RadiusPanel).WidthPercent(100).MaxWidth(handoffMaxWidth).MinWidth(0)
	hovered := known && b.Hovered()
	bg, border := tk.Surface, tk.Border
	if hovered {
		bg, border = tk.SurfaceMuted, tk.BorderStrong
	}
	b.Background(b.AnimateColor("bg", bg, colorFade)).Border(1, b.AnimateColor("border", border, colorFade))
	if known {
		param := route.Param(info.ConversationID)
		if param == "" {
			param = info.WorkerID
		}
		b.OnClick(func() { v.nav.Push(route.Chat(param)) })
	} else {
		b.Opacity(handoffDimmed).Cursor(ui.CursorNotAllowed)
	}
	b.Children(func() {
		if known {
			kit.WorkerAvatar(c, tk, w.ID, w.Name, w.Status, kit.AvatarMD, true)
		} else {
			kit.UnknownAvatar(c, tk, kit.AvatarMD)
		}
		ui.Column(c).Grow(1).MinWidth(0).Gap(handoffLineGap).AlignItems(ui.Start).Children(func() {
			ui.Row(c).Gap(handoffTitleGap).MinWidth(0).MaxWidthPercent(100).Children(func() {
				ui.Text(c, title).FontSize(bodySize).FontWeight(mediumWeight).TextColor(tk.Foreground).SingleLine().MinWidth(0)
				if known {
					nudge := float32(0)
					if hovered {
						nudge = handoffNudge
					}
					arrow := ui.Icon(c, icons.Get("arrow-right")).Size(handoffArrow, handoffArrow).Shrink(0).TextColor(tk.TextSecondary)
					arrow.Translate(arrow.AnimateTo("nudge", nudge, colorFade), 0)
				}
			})
			if preview := model.HandoffPreview(m.Content); preview != "" {
				ui.Text(c, preview).FontSize(bodySize).TextColor(tk.TextSecondary).MaxLines(handoffPreview)
			}
			if label := taskTypeLabel(info.TaskType); label != "" {
				ui.Row(c).Gap(taskRowGap).Wrap().MinWidth(0).Margin(taskRowTop, 0, 0, 0).Children(func() {
					kit.SmallBadge(c, tk, label, kit.ToneNeutral)
					if info.CronExpr != "" {
						ui.Text(c, info.CronExpr).Font(theme.FontMono).FontSize(metaSize).TextColor(tk.TextSecondary).SingleLine().MinWidth(0)
					}
				})
			}
		})
	})
}
