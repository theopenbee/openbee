package shortcuts

import (
	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/kit"
	"github.com/theopenbee/openbee/desktop/internal/theme"
	"github.com/theopenbee/openbee/desktop/locales"
)

const (
	groupGap     = 20
	headingGap   = 6
	headingSize  = 12
	rowHeight    = 40
	rowGap       = 12
	labelSize    = 14
	mediumWeight = 500
)

type entry struct {
	label    func() string
	shortcut Shortcut
}

type group struct {
	title   func() string
	entries []entry
}

func groups() []group {
	return []group{
		{locales.ShortcutsGeneral, []entry{
			{locales.ShortcutsQuickSearch, QuickSearch},
			{locales.ShortcutsShowShortcuts, ShowShortcuts},
			{locales.ShortcutsOpenSettings, OpenSettings},
			{locales.ShortcutsNewWorker, NewWorker},
			{locales.ShortcutsCloseDialog, Close},
		}},
		{locales.ShortcutsChat, []entry{
			{locales.ShortcutsPreviousConversation, PreviousConversation},
			{locales.ShortcutsNextConversation, NextConversation},
			{locales.ShortcutsNewConversation, NewConversation},
			{locales.ShortcutsSendMessage, Send},
			{locales.ShortcutsNewLine, NewLine},
		}},
	}
}

type Help struct {
	Open   bool
	OnSwap func()
}

func (h *Help) Toggle() {
	h.Open = !h.Open
}

func (h *Help) View(c *ui.Context, tk theme.Tokens) {
	panel := kit.Dialog(c, tk, &h.Open, kit.DialogOpts{Title: locales.ShortcutsTitle()}, func() {
		ui.Column(c).Gap(groupGap).Children(func() {
			for _, g := range groups() {
				ui.Column(c).Children(func() {
					ui.Text(c, g.title()).FontSize(headingSize).FontWeight(mediumWeight).TextColor(tk.TextMuted).
						Margin(0, 0, headingGap, 0).Role(ui.RoleHeading).Level(3)
					for i, e := range g.entries {
						row := ui.Row(c).Height(rowHeight).AlignItems(ui.Center).Gap(rowGap)
						if i > 0 {
							row.BorderWidth(1, 0, 0, 0).BorderColor(tk.BorderSubtle)
						}
						row.Children(func() {
							ui.Text(c, e.label()).FontSize(labelSize).TextColor(tk.TextSecondary).SingleLine().Grow(1).MinWidth(0)
							kit.Keys(c, tk, e.shortcut.Caps())
						})
					}
				})
			}
		})
	})
	ShowShortcuts.On(panel, func() { h.Open = false })
	if h.OnSwap != nil {
		QuickSearch.On(panel, func() {
			h.Open = false
			h.OnSwap()
		})
	}
}
