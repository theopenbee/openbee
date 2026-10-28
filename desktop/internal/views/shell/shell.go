package shell

import (
	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/kit"
	"github.com/theopenbee/openbee/desktop/internal/store"
	"github.com/theopenbee/openbee/desktop/internal/theme"
	"github.com/theopenbee/openbee/desktop/internal/views/route"
	"github.com/theopenbee/openbee/desktop/locales"
)

const (
	settingsSection = "env"
	panelPadStart   = 24
	panelPadEnd     = 12
	panelHeaderGap  = 12
	panelBodyPad    = 24
	panelTitleSize  = 16
	mediumWeight    = 500
	panelKey        = "panel"
)

var panelMotion = ui.ElementTransition{
	Animation: &ui.Smooth,
	Enter:     &ui.Motion{Collapse: true},
	Exit:      &ui.Motion{Collapse: true},
}

type ChatPane interface {
	View(c *ui.Context, conversationID string, panel route.Panel)
}

type PanelPane interface {
	View(c *ui.Context, conversationID string, panel route.Panel)
}

type AccountTrigger interface {
	View(c *ui.Context)
}

type Shell struct {
	s            *store.Store
	nav          route.Nav
	chat         ChatPane
	panel        PanelPane
	account      AccountTrigger
	openSettings func(section string)
	list         list
	drawerOpen   bool
	drawerShown  bool
}

func New(s *store.Store, nav route.Nav, chat ChatPane, panel PanelPane, account AccountTrigger, openSettings func(section string)) *Shell {
	sh := &Shell{s: s, nav: nav, chat: chat, panel: panel, account: account, openSettings: openSettings}
	sh.list.init()
	return sh
}

func (sh *Shell) View(c *ui.Context, conversationParam string) {
	tk := theme.Apply(c)
	cid := route.ConversationID(conversationParam)
	if sh.s.Active() != cid {
		sh.s.SetActive(cid)
	}
	panel := route.ParsePanel(conversationParam, sh.nav.Query(route.PanelQuery))
	w, _ := c.Size()
	twoPane := w >= theme.BreakTwoPane
	ui.Row(c).Fill().AlignItems(ui.Stretch).Background(tk.Background).Children(func() {
		side := ui.Column(c).MinWidth(0).Hidden(!twoPane && cid != "")
		if twoPane {
			side.Width(theme.SidebarWidth).Shrink(0).BorderWidth(0, 1, 0, 0).BorderColor(tk.Border)
		} else {
			side.Grow(1)
		}
		side.Children(func() { sh.listView(c, tk, conversationParam, cid) })
		if twoPane || cid != "" {
			ui.Column(c).Grow(1).MinWidth(0).MinHeight(0).Children(func() {
				if cid != "" {
					sh.chat.View(c, cid, panel)
				}
			})
		}
		sh.panelView(c, tk, w, conversationParam, cid, panel)
	})
}

func (sh *Shell) openNewWorker(param string) {
	sh.nav.Push(route.ChatPanel(param, route.PanelNew))
}

func (sh *Shell) closePanel(param string) {
	sh.nav.Push(route.Chat(param))
}

func panelTitle(p route.Panel) string {
	if p.Mode == route.ModeNew {
		return locales.ChatNewWorker()
	}
	return locales.ChatPanelTitle()
}

func (sh *Shell) panelView(c *ui.Context, tk theme.Tokens, w float32, param, cid string, p route.Panel) {
	if w >= theme.BreakInlinePanel {
		sh.drawerOpen, sh.drawerShown = false, false
		if p.Mode != "" {
			sh.inlinePanel(c, tk, param, cid, p)
		}
		return
	}
	open := p.Mode != ""
	if open && sh.drawerShown && !sh.drawerOpen {
		sh.closePanel(param)
		open = false
	}
	sh.drawerOpen, sh.drawerShown = open, open
	kit.Drawer(c, tk, &sh.drawerOpen, panelTitle(p), 0, func() { sh.panel.View(c, cid, p) })
	if open && !sh.drawerOpen {
		sh.closePanel(param)
		sh.drawerShown = false
	}
}

func (sh *Shell) inlinePanel(c *ui.Context, tk theme.Tokens, param, cid string, p route.Panel) {
	ui.Column(c).Key(panelKey).Width(theme.PanelWidth).Shrink(0).Clip().Label(panelTitle(p)).
		Background(tk.Surface).BorderWidth(0, 0, 0, 1).BorderColor(tk.Border).Transition(panelMotion).Children(func() {
		ui.Row(c).Height(theme.HeaderHeight).Shrink(0).Gap(panelHeaderGap).PaddingStart(panelPadStart).PaddingEnd(panelPadEnd).
			BorderWidth(0, 0, 1, 0).BorderColor(tk.Border).Children(func() {
			ui.Text(c, panelTitle(p)).FontSize(panelTitleSize).FontWeight(mediumWeight).SingleLine().Grow(1).MinWidth(0)
			kit.IconButton(c, tk, "x", locales.ChatPanelClose(), kit.ButtonOpts{Variant: kit.Ghost, Size: kit.SizeSM}).
				OnClick(func() { sh.closePanel(param) })
		})
		ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
			ui.Column(c).Padding(panelBodyPad).Children(func() { sh.panel.View(c, cid, p) })
		})
	})
}

func (sh *Shell) settings() {
	sh.openSettings(settingsSection)
}
