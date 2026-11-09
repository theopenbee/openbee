package settings

import (
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/icons"
	"github.com/theopenbee/openbee/desktop/internal/theme"
	"github.com/theopenbee/openbee/desktop/locales"
)

const (
	sidebarBorder   = 1
	sidebarPad      = 12
	sidebarTopGap   = 8
	navGap          = 2
	navStripPadX    = 16
	navStripPadY    = 12
	navStripGap     = 4
	navItemHeight   = 36
	navItemPadX     = 12
	navItemGap      = 12
	navItemSize     = 14
	navIconSize     = 16
	headerPadNarrow = 12
	headerPadWide   = 20
	headerGap       = 12
	headerBackBox   = 40
	headerBackGlyph = 20
	headerTitleSize = 16
	navFadeTime     = 150 * time.Millisecond
)

var navFade = ui.Animation{Duration: navFadeTime, Ease: ui.CubicBezier(0.22, 1, 0.36, 1)}

func (d *Page) sidebar(c *ui.Context, tk theme.Tokens, cur section) {
	ui.Column(c).Width(theme.SidebarWidth).Shrink(0).BorderWidth(0, sidebarBorder, 0, 0).BorderColor(tk.Border).Children(func() {
		top := max(sidebarPad, c.TitleBar().Height+sidebarTopGap)
		ui.Column(c).Shrink(0).Padding(top, sidebarPad, sidebarPad, sidebarPad).DragWindow().Children(func() {
			d.sidebarItem(c, tk, "arrow-left", locales.NavBackToChat(), false).OnClick(d.leave)
		})
		ui.Column(c).Padding(0, sidebarPad, sidebarPad, sidebarPad).Gap(navGap).Children(func() {
			for _, s := range sections {
				d.navItem(c, tk, s, s.id == cur.id)
			}
		})
	})
}

func (d *Page) nav(c *ui.Context, tk theme.Tokens, cur section) {
	ui.ScrollHorizontal(c).Padding(navStripPadY, navStripPadX).Gap(navStripGap).BorderWidth(0, 0, 1, 0).
		Shrink(0).BorderColor(tk.BorderSubtle).Children(func() {
		for _, s := range sections {
			d.navItem(c, tk, s, s.id == cur.id)
		}
	})
}

func (d *Page) navItem(c *ui.Context, tk theme.Tokens, s section, active bool) {
	b := d.sidebarItem(c, tk, s.icon, s.nav(), active).OnClick(func() { d.choose(s.id) })
	if !d.wide() && active && d.revealed != s.id {
		d.revealed = s.id
		b.ScrollIntoView()
	}
}

func (d *Page) sidebarItem(c *ui.Context, tk theme.Tokens, icon, label string, active bool) ui.Element {
	b := ui.ButtonBase(c).Label(label).Role(ui.RoleLink).Height(navItemHeight).PaddingX(navItemPadX).Gap(navItemGap).
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
	return b.Background(b.AnimateColor("bg", bg, navFade)).TextColor(b.AnimateColor("fg", fg, navFade)).Children(func() {
		glyph := ui.Icon(c, icons.Get(icon)).Size(navIconSize, navIconSize).Shrink(0)
		if active {
			glyph.TextColor(tk.AccentStrong)
		}
		ui.Text(c, label).SingleLine().MinWidth(0)
	})
}

func (d *Page) header(c *ui.Context, tk theme.Tokens, cur section) {
	pad, start := float32(headerPadWide), float32(headerPadWide)
	if !d.wide() {
		pad = headerPadNarrow
		start = max(pad, c.TitleBar().Left)
	}
	ui.Row(c).Height(theme.HeaderHeight).Shrink(0).AlignItems(ui.Center).Gap(headerGap).PaddingStart(start).PaddingEnd(pad).
		BorderWidth(0, 0, 1, 0).BorderColor(tk.Border).DragWindow().Children(func() {
		if !d.wide() {
			d.headerBack(c, tk)
		}
		ui.Text(c, cur.title()).FontSize(headerTitleSize).FontWeight(mediumWeight).SingleLine().MinWidth(0).
			Role(ui.RoleHeading).Level(1)
	})
}

func (d *Page) headerBack(c *ui.Context, tk theme.Tokens) {
	b := ui.ButtonBase(c).Label(locales.NavBackToChat()).Size(headerBackBox, headerBackBox).Shrink(0).Radius(theme.RadiusControl)
	bg, fg := tk.SurfaceMuted.Alpha(0), tk.TextSecondary
	if b.Hovered() || b.Pressed() {
		bg, fg = tk.SurfaceMuted, tk.Foreground
	}
	b.Background(b.AnimateColor("bg", bg, navFade)).TextColor(b.AnimateColor("fg", fg, navFade)).OnClick(d.leave).Children(func() {
		ui.Icon(c, icons.Get("chevron-left")).Size(headerBackGlyph, headerBackGlyph)
	})
}
