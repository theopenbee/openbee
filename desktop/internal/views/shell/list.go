package shell

import (
	"time"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/api"
	"github.com/theopenbee/openbee/desktop/internal/icons"
	"github.com/theopenbee/openbee/desktop/internal/kit"
	"github.com/theopenbee/openbee/desktop/internal/model"
	"github.com/theopenbee/openbee/desktop/internal/theme"
	"github.com/theopenbee/openbee/desktop/locales"
)

const (
	clockInterval  = time.Minute
	skeletonRows   = 4
	logoHeight     = 28
	logoAspect     = 1448.0 / 420.0
	headerPadStart = 20
	headerPadEnd   = 12
	headerGap      = 8
	listPadX       = 12
	listPadBottom  = 12
	rowGap         = 2
	noteGap        = 12
	notePadX       = 12
	notePadY       = 16
	bodySize       = 14
	footerPad      = 12
	footerGap      = 8
	nameBarWidth   = 96
	nameBarHeight  = 14
	textBarWidth   = 160
	textBarHeight  = 12
	skeletonGap    = 8
)

type hintKey struct{}

type list struct {
	query    string
	rows     []api.ConversationSummary
	state    ui.ListState
	revealed string
}

func (l *list) init() {
	l.state.KeepOffset = true
	l.state.Key = func(i int) any {
		if i >= len(l.rows) {
			return hintKey{}
		}
		return l.rows[i].ID
	}
}

func (sh *Shell) listView(c *ui.Context, tk theme.Tokens, param, active string) {
	sh.s.Conversations.Ensure()
	sh.s.Workers.Ensure()
	ui.Column(c).Grow(1).MinHeight(0).Background(tk.Background).Children(func() {
		sh.listHeader(c, tk, param)
		ui.Box(c).Shrink(0).Padding(0, listPadX, listPadX, listPadX).Children(func() {
			kit.SearchField(c, tk, &sh.list.query, locales.ChatSearch(), locales.ChatSearch())
		})
		sh.listBody(c, tk, param, active)
		ui.Row(c).Shrink(0).Justify(ui.SpaceBetween).Gap(footerGap).Padding(footerPad).
			BorderWidth(1, 0, 0, 0).BorderColor(tk.BorderSubtle).Children(func() {
			sh.account.View(c)
			kit.IconButton(c, tk, "settings", locales.NavSettings(), kit.ButtonOpts{Variant: kit.Ghost, Size: kit.SizeSM}).
				MarginStart(ui.Auto).OnClick(sh.settings)
		})
	})
}

func (sh *Shell) listHeader(c *ui.Context, tk theme.Tokens, param string) {
	bar := c.TitleBar()
	ui.Row(c).Height(theme.HeaderHeight).Shrink(0).Justify(ui.SpaceBetween).Gap(headerGap).
		PaddingStart(max(headerPadStart, bar.Left)).PaddingEnd(headerPadEnd).DragWindow().Children(func() {
		ui.Image(c, icons.LogoFull(tk)).Size(logoHeight*logoAspect, logoHeight)
		kit.IconButton(c, tk, "plus", locales.ChatNewWorker(), kit.ButtonOpts{Variant: kit.Ghost, Size: kit.SizeSM}).
			OnClick(func() { sh.openNewWorker(param) })
	})
}

func (sh *Shell) listBody(c *ui.Context, tk theme.Tokens, param, active string) {
	convs, workers := sh.s.Conversations, sh.s.Workers
	switch {
	case pending(convs.Loaded, convs.Err) || pending(workers.Loaded, workers.Err):
		listSkeleton(c, tk)
		return
	case failed(convs.Loaded, convs.Err) || failed(workers.Loaded, workers.Err):
		note(c, tk, locales.CommonLoadError(), func() {
			kit.Button(c, tk, locales.CommonRetry(), kit.ButtonOpts{Variant: kit.Secondary, Size: kit.SizeSM}).OnClick(sh.retry)
		})
		return
	}
	l := &sh.list
	byID := sh.s.WorkerMap()
	l.rows = model.FilterConversations(model.SortConversations(model.VisibleConversations(convs.Data, byID), byID), byID, l.query)
	sh.reveal(active)
	now, lang := c.Now(), i18n.Locale()
	c.After(clockInterval)
	n := len(l.rows)
	if len(byID) == 0 {
		n++
	}
	ui.List(c, &l.state, n, func(i int) {
		if i >= len(l.rows) {
			note(c, tk, locales.ChatNoWorkersHint(), func() {
				kit.Button(c, tk, locales.ChatNewWorker(), kit.ButtonOpts{Variant: kit.Secondary, Size: kit.SizeSM, Icon: "plus"}).
					OnClick(func() { sh.openNewWorker(param) })
			})
			return
		}
		conv := l.rows[i]
		sh.row(c, tk, conv, byID, conv.ID == active, now, lang)
	}).Grow(1).MinHeight(0).Gap(rowGap).Padding(0, listPadX, listPadBottom, listPadX).Label(locales.ChatTitle())
}

func (sh *Shell) reveal(active string) {
	l := &sh.list
	if active == l.revealed {
		return
	}
	for i, conv := range l.rows {
		if conv.ID == active {
			l.state.ScrollIntoView(i)
			l.revealed = active
			return
		}
	}
}

func (sh *Shell) retry() {
	if sh.s.Conversations.Err != nil {
		sh.s.Conversations.Invalidate()
	}
	if sh.s.Workers.Err != nil {
		sh.s.Workers.Invalidate()
	}
}

func pending(loaded bool, err error) bool {
	return !loaded && err == nil
}

func failed(loaded bool, err error) bool {
	return !loaded && err != nil
}

func note(c *ui.Context, tk theme.Tokens, text string, action func()) {
	ui.Column(c).AlignItems(ui.Start).Gap(noteGap).Padding(notePadY, notePadX).Children(func() {
		ui.Text(c, text).FontSize(bodySize).TextColor(tk.TextSecondary)
		action()
	})
}

func listSkeleton(c *ui.Context, tk theme.Tokens) {
	ui.Column(c).Grow(1).MinHeight(0).Gap(rowGap).Padding(0, listPadX, listPadBottom, listPadX).Children(func() {
		for range skeletonRows {
			ui.Row(c).Gap(rowContentGap).Padding(rowPadY, rowPadX).Children(func() {
				kit.SkeletonCircle(c, tk, avatarSize)
				ui.Column(c).Grow(1).MinWidth(0).Gap(skeletonGap).Children(func() {
					kit.SkeletonBar(c, tk, nameBarWidth, nameBarHeight)
					kit.SkeletonBar(c, tk, textBarWidth, textBarHeight).MaxWidthPercent(100)
				})
			})
		}
	})
}
