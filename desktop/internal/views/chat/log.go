package chat

import (
	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/kit"
	"github.com/theopenbee/openbee/desktop/internal/model"
	"github.com/theopenbee/openbee/desktop/internal/theme"
	"github.com/theopenbee/openbee/desktop/locales"
)

const (
	columnPadNarrow = 16
	columnPadWide   = 24
	logPadTop       = 24
	logPadTopWide   = 40
	logPadBottom    = 32
	groupGap        = 24
	continueGap     = 8
	dividerGap      = 32
	loadMoreBelow   = 32
	loadMoreGap     = 8
	bodySize        = 14
	stateGap        = 12
	skeletonGap     = 32
	skeletonLineGap = 8
	skeletonBubble  = 44
	skeletonName    = 12
	skeletonNameW   = 48
	skeletonLine    = 16
)

var skeletonBlocks = []struct {
	bubble float32
	lines  []float32
}{
	{40, []float32{100, 80, 60}},
	{100.0 / 3, []float32{100 * 11 / 12.0, 100 * 2 / 3.0}},
}

func columnPad(c *ui.Context) float32 {
	if w, _ := c.Size(); w >= theme.BreakComposer {
		return columnPadWide
	}
	return columnPadNarrow
}

func column(c *ui.Context, body func()) ui.Element {
	pad := columnPad(c)
	return ui.Column(c).AlignItems(ui.Center).Children(func() {
		ui.Column(c).WidthPercent(100).MaxWidth(theme.ChatColumnMax).PaddingX(pad).Children(body)
	})
}

func (v *View) log(c *ui.Context, tk theme.Tokens, p pane) {
	ch, cs := p.ch, p.cs
	ch.Ensure()
	msgs := ch.List.Msgs
	cs.msgs = msgs
	if len(msgs) > 0 {
		v.messages(c, tk, p)
		return
	}
	cs.list.ScrollToEnd()
	workers := v.s.Workers
	switch {
	case !ch.Loaded && ch.Err == nil || !workers.Loaded && workers.Err == nil:
		logSkeleton(c, tk)
	case !ch.Loaded:
		logState(c, func() {
			ui.Text(c, locales.CommonLoadError()).FontSize(bodySize).TextColor(tk.TextSecondary)
			kit.Button(c, tk, locales.CommonRetry(), kit.ButtonOpts{Variant: kit.Secondary, Size: kit.SizeSM}).OnClick(ch.Refresh)
		})
	case p.worker != nil:
		logState(c, func() { workerIntro(c, tk, *p.worker) })
	default:
		logState(c, func() {
			kit.EmptyState(c, tk, "", locales.ChatNoMessagesTitle(), locales.ChatNoMessagesDescription(), nil)
		})
	}
}

func logTop(c *ui.Context) float32 {
	if w, _ := c.Size(); w >= theme.BreakTwoPane {
		return logPadTopWide
	}
	return logPadTop
}

func logState(c *ui.Context, body func()) {
	ui.Column(c).Grow(1).MinHeight(0).Clip().Padding(logTop(c), 0, logPadBottom, 0).Children(func() {
		column(c, func() {
			ui.Column(c).AlignItems(ui.Center).Gap(stateGap).Children(body)
		}).Grow(1).Justify(ui.Center)
	})
}

func logSkeleton(c *ui.Context, tk theme.Tokens) {
	ui.Column(c).Grow(1).MinHeight(0).Clip().Padding(logTop(c), 0, logPadBottom, 0).Children(func() {
		column(c, func() {
			ui.Column(c).Gap(skeletonGap).Children(func() {
				for _, block := range skeletonBlocks {
					ui.Row(c).Justify(ui.End).Children(func() {
						kit.SkeletonBar(c, tk, 0, skeletonBubble).WidthPercent(block.bubble)
					})
					ui.Column(c).Gap(skeletonLineGap).Children(func() {
						kit.SkeletonBar(c, tk, skeletonNameW, skeletonName)
						for _, line := range block.lines {
							kit.SkeletonBar(c, tk, 0, skeletonLine).WidthPercent(line)
						}
					})
				}
			})
		})
	})
}

func (v *View) messages(c *ui.Context, tk theme.Tokens, p pane) {
	ch, cs := p.ch, p.cs
	area := v.loadMoreSpace(p)
	rows := rowContext{pane: p, tk: tk, workers: v.s.WorkerMap(), now: c.Now(), lang: i18n.Locale()}
	ui.List(c, &cs.list, len(cs.msgs), func(i int) {
		if i == 0 && ch.List.HasMore {
			v.loadMore(c, tk, p, area)
		}
		v.row(c, rows, i)
	}).Grow(1).MinHeight(0).Padding(logTop(c)+area, 0, logPadBottom, 0).TrackScroll(&cs.scroll)
}

func (v *View) loadMoreSpace(p pane) float32 {
	ch, cs := p.ch, p.cs
	if ch.List.HasMore {
		cs.moreSpace = true
	}
	if !cs.moreSpace {
		return 0
	}
	area := loadMoreHeight(ch.OlderErr != nil && !ch.LoadingOlder)
	if ch.List.HasMore {
		return area
	}
	switch {
	case cs.list.AtEnd():
	case cs.scroll.Y >= area:
		cs.scroll.Y -= area
	default:
		return area
	}
	cs.moreSpace = false
	return 0
}

func loadMoreHeight(failed bool) float32 {
	h := float32(theme.ControlSM + loadMoreBelow)
	if failed {
		h += bodySize*theme.BodyLineHeight + loadMoreGap
	}
	return h
}

func (v *View) loadMore(c *ui.Context, tk theme.Tokens, p pane, area float32) {
	ch := p.ch
	failed := ch.OlderErr != nil && !ch.LoadingOlder
	ui.Column(c).Absolute().Top(-area).Left(0).Right(0).Height(area - loadMoreBelow).
		AlignItems(ui.Center).Justify(ui.End).Gap(loadMoreGap).Children(func() {
		if failed {
			ui.Text(c, locales.ChatLoadMoreFailed()).FontSize(bodySize).TextColor(tk.Danger).Role(ui.RoleStatus)
		}
		label := locales.ChatLoadMore()
		if ch.OlderErr != nil {
			label = locales.CommonRetry()
		}
		kit.Button(c, tk, label, kit.ButtonOpts{Variant: kit.Secondary, Size: kit.SizeSM, Loading: ch.LoadingOlder}).
			OnClick(ch.LoadOlder)
	})
}

func (v *View) row(c *ui.Context, rc rowContext, i int) {
	msgs := rc.cs.msgs
	m := msgs[i]
	column(c, func() {
		if m.Kind == kindReset {
			divider(c, rc.tk, m.TS, rc.now, rc.lang)
		} else {
			v.message(c, rc, i)
		}
		if i+1 < len(msgs) {
			ui.Box(c).Height(spaceBefore(msgs, i+1)).Shrink(0)
		}
	})
}

func spaceBefore(msgs []model.Message, i int) float32 {
	m := msgs[i]
	switch {
	case m.Kind == kindReset:
		return dividerGap
	case m.Sender.Type == senderUser && !model.StartsGroup(msgs, i):
		return continueGap
	}
	return groupGap
}
