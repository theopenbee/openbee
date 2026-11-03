package settings

import (
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/icons"
	"github.com/theopenbee/openbee/desktop/internal/theme"
)

const (
	navWidth      = 224
	navPad        = 16
	navGap        = 2
	navStripPadX  = 16
	navStripPadY  = 12
	navStripGap   = 4
	navItemHeight = 36
	navItemPadX   = 12
	navItemGap    = 12
	navItemSize   = 14
	navIconSize   = 16
	navFadeTime   = 150 * time.Millisecond
)

var navFade = ui.Animation{Duration: navFadeTime, Ease: ui.CubicBezier(0.22, 1, 0.36, 1)}

func (d *Dialog) nav(c *ui.Context, tk theme.Tokens, cur section) {
	var strip ui.Element
	if d.wide() {
		strip = ui.Column(c).Width(navWidth).Padding(navPad).Gap(navGap).BorderWidth(0, 1, 0, 0)
	} else {
		strip = ui.ScrollHorizontal(c).Padding(navStripPadY, navStripPadX).Gap(navStripGap).BorderWidth(0, 0, 1, 0)
	}
	strip.Shrink(0).BorderColor(tk.BorderSubtle).Children(func() {
		for _, s := range sections {
			d.navItem(c, tk, s, s.id == cur.id)
		}
	})
}

func (d *Dialog) navItem(c *ui.Context, tk theme.Tokens, s section, active bool) {
	b := ui.ButtonBase(c).Label(s.nav()).Role(ui.RoleLink).Height(navItemHeight).PaddingX(navItemPadX).Gap(navItemGap).
		Justify(ui.Start).AlignItems(ui.Center).Radius(theme.RadiusControl).FontSize(navItemSize)
	if d.wide() {
		b.MinWidth(0)
	} else {
		b.Shrink(0)
	}
	bg, fg := tk.SurfaceMuted.Alpha(0), tk.TextSecondary
	switch {
	case active:
		bg, fg = tk.AccentSubtle, tk.Foreground
	case b.Hovered() || b.Pressed():
		bg, fg = tk.SurfaceMuted, tk.Foreground
	}
	b.Background(b.AnimateColor("bg", bg, navFade)).TextColor(b.AnimateColor("fg", fg, navFade)).
		OnClick(func() { d.choose(s.id) })
	if active && d.revealed != s.id {
		d.revealed = s.id
		b.ScrollIntoView()
	}
	b.Children(func() {
		icon := ui.Icon(c, icons.Get(s.icon)).Size(navIconSize, navIconSize).Shrink(0)
		if active {
			icon.TextColor(tk.AccentStrong)
		}
		ui.Text(c, s.nav()).SingleLine().MinWidth(0)
	})
}
