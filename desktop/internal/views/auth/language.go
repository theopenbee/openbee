package auth

import (
	"time"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/icons"
	"github.com/theopenbee/openbee/desktop/internal/kit"
	"github.com/theopenbee/openbee/desktop/internal/theme"
	"github.com/theopenbee/openbee/desktop/locales"
)

type language struct{ code, tag string }

var languages = []language{{code: "en", tag: "en"}, {code: "zh", tag: "zh-Hans"}}

const (
	triggerPadX   = 12
	triggerGap    = 8
	triggerIcon   = 16
	triggerArrow  = 15
	triggerWeight = 500
	menuWidth     = 192
	menuInset     = 6
	itemHeight    = 36
	itemPadX      = 11
	itemGap       = 10
	itemIconBox   = 17
	itemIcon      = 16
)

var quick = ui.Animation{Duration: 160 * time.Millisecond, Ease: ui.CubicBezier(0.22, 1, 0.36, 1)}

func currentLanguage() language {
	if i18n.Base(i18n.Locale()) == "zh" {
		return languages[1]
	}
	return languages[0]
}

func (a *Screens) languageMenu(c *ui.Context, tk theme.Tokens) {
	current := currentLanguage()
	trigger := ui.ButtonBase(c).Label(locales.ArcUserMenuLanguage()).Role(ui.RoleMenuButton).Expanded(a.langOpen).
		MinHeight(theme.ControlSM).PaddingX(triggerPadX).Gap(triggerGap).Radius(theme.RadiusControl).
		FontSize(bodySize).FontWeight(triggerWeight).TextColor(tk.Foreground)
	active := a.langOpen || trigger.Hovered() || trigger.Pressed()
	bg, border := tk.Surface, tk.Border
	if active {
		bg, border = tk.SurfaceMuted, tk.BorderStrong
	}
	trigger.Background(trigger.AnimateColor("bg", bg, quick)).Border(1, trigger.AnimateColor("border", border, quick))
	if trigger.Hovered() && !a.langOpen && !trigger.Pressed() {
		theme.WithShadow(trigger, tk.ShadowResting)
	}
	trigger.OnClick(func() { a.langOpen = !a.langOpen })
	trigger.Children(func() {
		ui.Icon(c, icons.Get("languages")).Size(triggerIcon, triggerIcon).TextColor(tk.TextSecondary)
		ui.Text(c, i18n.Autonym(current.tag)).SingleLine()
		turn := float32(0)
		if a.langOpen {
			turn = 180
		}
		arrow := ui.Icon(c, icons.Get("chevron-down")).Size(triggerArrow, triggerArrow).TextColor(tk.TextMuted)
		arrow.Rotate(arrow.AnimateTo("turn", turn, ui.Snappy))
	})
	panel := kit.Popover(c, tk, trigger, &a.langOpen, menuWidth, func() {
		ui.Column(c).FocusGroup(ui.Vertical).Role(ui.RoleMenu).Children(func() {
			for _, l := range languages {
				a.languageItem(c, tk, l, l == current)
			}
		})
	})
	if a.langOpen {
		panel.AttachTo(trigger, ui.AnchorBottomEnd, ui.AnchorTopEnd)
	}
}

func (a *Screens) languageItem(c *ui.Context, tk theme.Tokens, l language, selected bool) {
	name := i18n.Autonym(l.tag)
	item := ui.ButtonBase(c).Label(name).Role(ui.RoleMenuItemRadio).Checked(selected).
		MinHeight(itemHeight).PaddingX(itemPadX).Gap(itemGap).Justify(ui.Start).
		Radius(theme.Concentric(theme.RadiusPanel, menuInset)).FontSize(bodySize).TextColor(tk.Foreground)
	highlight := tk.SurfaceMuted.Alpha(0)
	if item.Hovered() || item.FocusVisible() || item.Pressed() {
		highlight = tk.SurfaceMuted
	}
	item.Background(item.AnimateColor("bg", highlight, quick)).FocusRing(false)
	item.OnClick(func() {
		a.langOpen = false
		a.s.SetLanguage(l.code)
	})
	item.Children(func() {
		ui.Box(c).Width(itemIconBox).TextColor(tk.TextSecondary).Children(func() {
			if selected {
				ui.Icon(c, icons.Get("check")).Size(itemIcon, itemIcon)
			}
		})
		ui.Text(c, name).SingleLine()
	})
}
