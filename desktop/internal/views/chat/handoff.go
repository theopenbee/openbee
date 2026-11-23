package chat

import (
	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/api"
	"github.com/theopenbee/openbee/desktop/internal/icons"
	"github.com/theopenbee/openbee/desktop/internal/kit"
	"github.com/theopenbee/openbee/desktop/internal/model"
	"github.com/theopenbee/openbee/desktop/internal/theme"
	"github.com/theopenbee/openbee/desktop/internal/views/execution"
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
	runRowHeight    = 44
	runRowGap       = 8
	runLinkGap      = 2
	runGlyph        = 14
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

func (v *View) handoffCard(c *ui.Context, tk theme.Tokens, cid string, m model.Message, workers map[string]api.Worker) {
	info := m.Handoff
	inner := theme.Concentric(theme.RadiusPanel, 1)
	ui.Column(c).WidthPercent(100).MaxWidth(handoffMaxWidth).MinWidth(0).Radius(theme.RadiusPanel).
		Background(tk.Surface).Border(1, tk.Border).Children(func() {
		if info.Run == nil {
			v.handoffTask(c, tk, m, workers, corners(inner, inner))
			return
		}
		v.handoffTask(c, tk, m, workers, corners(inner, 0))
		v.handoffRun(c, tk, cid, *info.Run, info.TaskType == taskScheduled, corners(0, inner))
	})
}

func (v *View) handoffTask(c *ui.Context, tk theme.Tokens, m model.Message, workers map[string]api.Worker, radius []float32) {
	info := m.Handoff
	w, known := workers[info.WorkerID]
	name := locales.ChatDeletedWorker()
	if known {
		name = w.Name
	}
	title := locales.ChatHandedOffTo(name)
	b := ui.ButtonBase(c).Label(title).Disabled(!known).AlignItems(ui.Start).Justify(ui.Start).Gap(handoffGap).
		Padding(handoffPadY, handoffPadX).Radius(radius...).MinWidth(0)
	hovered := known && b.Hovered()
	bg := tk.SurfaceMuted.Alpha(0)
	if hovered {
		bg = tk.SurfaceMuted
	}
	b.Background(b.AnimateColor("bg", bg, colorFade))
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
			kit.WorkerAvatar(c, tk, w.ID, w.Name, w.Status, kit.AvatarMD, false)
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

func corners(top, bottom float32) []float32 {
	return []float32{top, top, bottom, bottom}
}

func (v *View) handoffRun(c *ui.Context, tk theme.Tokens, cid string, run api.ExecutionSummary, latest bool, radius []float32) {
	label := execution.RunLabel(run, latest)
	show := locales.ExecutionShow()
	b := ui.ButtonBase(c).Label(show).Description(label).Height(runRowHeight).MinWidth(0).AlignItems(ui.Center).Justify(ui.Start).
		Gap(runRowGap).PaddingX(handoffPadX).Radius(radius...).BorderWidth(1, 0, 0, 0).BorderColor(tk.Border).
		FontSize(metaSize)
	bg, fg := tk.SurfaceMuted.Alpha(0), tk.TextSecondary
	if b.Hovered() || b.Pressed() {
		bg, fg = tk.SurfaceMuted, tk.Foreground
	}
	failed := run.Status == model.ExecutionFailed
	if failed {
		fg = tk.Danger
	}
	b.Background(b.AnimateColor("bg", bg, colorFade)).OnClick(func() { v.showRun(cid, run.ID) }).Children(func() {
		runStatusIcon(c, tk, run.Status)
		ui.Text(c, label).TextColor(b.AnimateColor("fg", fg, colorFade)).SingleLine().Shrink(1).MinWidth(0)
		ui.Box(c).Grow(1)
		ui.Row(c).Shrink(0).AlignItems(ui.Center).Gap(runLinkGap).TextColor(tk.Foreground).Children(func() {
			ui.Text(c, show).FontWeight(mediumWeight).SingleLine()
			ui.Icon(c, icons.Get("chevron-right")).Size(runGlyph, runGlyph)
		})
	})
}

func runStatusIcon(c *ui.Context, tk theme.Tokens, status string) {
	switch status {
	case model.ExecutionRunning:
		kit.Spinner(c, tk, runGlyph).TextColor(tk.TextSecondary)
	case model.ExecutionCompleted:
		ui.Icon(c, icons.Get("check")).Size(runGlyph, runGlyph).Shrink(0).TextColor(tk.Success)
	default:
		ui.Icon(c, icons.Get("circle-alert")).Size(runGlyph, runGlyph).Shrink(0).TextColor(tk.Danger)
	}
}
