package account

import (
	"cmp"
	"slices"
	"time"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/icons"
	"github.com/theopenbee/openbee/desktop/internal/kit"
	"github.com/theopenbee/openbee/desktop/internal/prefs"
	"github.com/theopenbee/openbee/desktop/internal/store"
	"github.com/theopenbee/openbee/desktop/internal/theme"
	"github.com/theopenbee/openbee/desktop/locales"
)

const (
	defaultName   = "admin"
	triggerHeight = 40
	triggerPad    = 4
	triggerPadEnd = 10
	triggerGap    = 8
	triggerFace   = 32
	triggerName   = 176
	triggerSize   = 14
	triggerArrow  = 16
	pressedMix    = 0.08
	mediumWeight  = 500

	menuWidth    = 288
	menuInset    = 5
	menuPadX     = 11
	menuGap      = 10
	menuSize     = 13
	menuGlyph    = 16
	itemHeight   = 32
	rowHeight    = 36
	rowPadEnd    = 4
	headerPadY   = 8
	headerFace   = 34
	roleSize     = 12
	noteSize     = 11
	separatorGap = 4
	dangerTint   = 0.08
	chinese      = "zh"
	english      = "en"
	themeLight   = "light"
	themeDark    = "dark"
	themeSystem  = "system"
	toggleOff    = 0
	toggleOn     = 1
	fadeDuration = 160 * time.Millisecond
)

var (
	fade   = ui.Animation{Duration: fadeDuration, Ease: ui.CubicBezier(0.22, 1, 0.36, 1)}
	themes = []string{themeLight, themeDark, themeSystem}
)

type entryPoint int

const (
	entryNone entryPoint = iota
	entryFirst
	entryLast
)

type Account struct {
	s        *store.Store
	open     bool
	refocus  bool
	entry    entryPoint
	password passwordForm
}

func New(s *store.Store) *Account {
	return &Account{s: s}
}

func (a *Account) Close() {
	a.open = false
	a.refocus = false
	a.entry = entryNone
	a.password.reset()
}

func (a *Account) name() string {
	return cmp.Or(a.s.Prefs.Username, defaultName)
}

func (a *Account) View(c *ui.Context) {
	tk := theme.Apply(c)
	name := a.name()
	trigger := ui.ButtonBase(c).Label(locales.ArcUserMenuTrigger(name)).Role(ui.RoleMenuButton).Expanded(a.open).
		Height(triggerHeight).MinWidth(0).Gap(triggerGap).Padding(triggerPad, triggerPadEnd, triggerPad, triggerPad).
		Radius(theme.RadiusPill)
	bg := tk.SurfaceMuted.Alpha(0)
	switch {
	case trigger.Pressed():
		bg = tk.SurfaceMuted.Mix(tk.Foreground, pressedMix)
	case trigger.Hovered() || a.open:
		bg = tk.SurfaceMuted
	}
	arrowColor := tk.TextMuted
	if trigger.Hovered() || a.open {
		arrowColor = tk.Foreground
	}
	if a.refocus {
		a.refocus = false
		trigger.Focus()
	}
	trigger.Background(trigger.AnimateColor("bg", bg, fade)).OnClick(func() { a.open = !a.open }).
		OnShortcut(0, ui.KeyDown, func() { a.openAt(entryFirst) }).OnShortcut(0, ui.KeyUp, func() { a.openAt(entryLast) })
	if a.open {
		trigger.OnShortcut(0, ui.KeyTab, a.closeFromTab)
	}
	trigger.Children(func() {
		kit.InitialsAvatar(c, tk, name, kit.AvatarSM).Size(triggerFace, triggerFace).TextColor(tk.TextSecondary)
		ui.Text(c, name).FontSize(triggerSize).FontWeight(mediumWeight).SingleLine().MinWidth(0).MaxWidth(triggerName)
		turn := float32(0)
		if a.open {
			turn = 180
		}
		arrow := ui.Icon(c, icons.Get("chevron-down")).Size(triggerArrow, triggerArrow).Shrink(0).
			TextColor(trigger.AnimateColor("arrow", arrowColor, fade))
		arrow.Rotate(arrow.AnimateTo("turn", turn, ui.Snappy))
	})
	kit.Popover(c, tk, trigger, &a.open, menuWidth, func() { a.menu(c, tk, name) })
}

func (a *Account) openAt(entry entryPoint) {
	a.open = true
	a.entry = entry
}

func (a *Account) closeFromTab() {
	a.open = false
	a.refocus = true
}

func (a *Account) menu(c *ui.Context, tk theme.Tokens, name string) {
	ui.Column(c).FocusGroup(ui.Vertical).Role(ui.RoleMenu).OnShortcut(0, ui.KeyTab, a.closeFromTab).Children(func() {
		a.header(c, tk, name)
		separator(c, tk)
		a.item(c, tk, "key-round", locales.AccountChangePassword(), false, entryFirst, func() {
			a.open = false
			a.password.show()
		})
		separator(c, tk)
		a.themeRow(c, tk)
		a.languageRow(c, tk)
		a.notificationsRow(c, tk)
		separator(c, tk)
		a.item(c, tk, "log-out", locales.ArcUserMenuSignOut(), true, entryLast, func() {
			a.open = false
			a.s.Logout()
		})
	})
}

func (a *Account) header(c *ui.Context, tk theme.Tokens, name string) {
	ui.Row(c).Gap(menuGap).Padding(headerPadY, menuPadX).Children(func() {
		kit.InitialsAvatar(c, tk, name, kit.AvatarMD).Size(headerFace, headerFace).TextColor(tk.TextSecondary)
		ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
			ui.Text(c, name).FontSize(menuSize).FontWeight(mediumWeight).SingleLine().MinWidth(0)
			ui.Text(c, locales.AccountRole()).FontSize(roleSize).TextColor(tk.TextMuted).SingleLine().MinWidth(0)
		})
	})
}

func separator(c *ui.Context, tk theme.Tokens) {
	ui.Box(c).Height(1).Margin(separatorGap, -menuInset).Background(tk.BorderSubtle)
}

func (a *Account) item(c *ui.Context, tk theme.Tokens, icon, text string, danger bool, entry entryPoint, onClick func()) {
	item := ui.ButtonBase(c).Label(text).Role(ui.RoleMenuItem).MinHeight(itemHeight).PaddingX(menuPadX).Gap(menuGap).
		AlignItems(ui.Center).Justify(ui.Start).Radius(theme.Concentric(theme.RadiusPanel, menuInset)).FontSize(menuSize)
	fg, glyph, tint := tk.Foreground, tk.TextSecondary, tk.SurfaceMuted
	if danger {
		fg, glyph, tint = tk.Danger, tk.Danger, tk.SurfaceRaised.Mix(tk.Danger, dangerTint)
	}
	bg := tint.Alpha(0)
	if item.Hovered() || item.Pressed() || item.FocusVisible() {
		bg = tint
	}
	if a.entry == entry {
		a.entry = entryNone
		item.Focus()
	}
	item.Background(item.AnimateColor("bg", bg, fade)).TextColor(fg).FocusRing(false).OnClick(onClick).Children(func() {
		ui.Icon(c, icons.Get(icon)).Size(menuGlyph, menuGlyph).Shrink(0).TextColor(glyph)
		ui.Text(c, text).SingleLine()
	})
}

func (a *Account) row(c *ui.Context, tk theme.Tokens, icon, label, note string, control func()) {
	ui.Row(c).Height(rowHeight).AlignItems(ui.Center).Gap(menuGap).Padding(0, rowPadEnd, 0, menuPadX).Children(func() {
		ui.Icon(c, icons.Get(icon)).Size(menuGlyph, menuGlyph).Shrink(0).TextColor(tk.TextSecondary)
		ui.Column(c).Grow(1).MinWidth(0).Children(func() {
			ui.Text(c, label).FontSize(menuSize).SingleLine().MinWidth(0)
			if note != "" {
				ui.Text(c, note).FontSize(noteSize).TextColor(tk.TextMuted).SingleLine().MinWidth(0)
			}
		})
		control()
	})
}

func (a *Account) themeRow(c *ui.Context, tk theme.Tokens) {
	selected := max(0, slices.Index(themes, a.s.ThemePref()))
	items := []kit.SegmentItem{
		{Icon: "sun", Tooltip: locales.ArcUserMenuThemesLight()},
		{Icon: "moon", Tooltip: locales.ArcUserMenuThemesDark()},
		{Icon: "monitor", Tooltip: locales.ArcUserMenuThemesSystem()},
	}
	a.row(c, tk, "sun-moon", locales.ArcUserMenuTheme(), "", func() {
		kit.SegmentedCompact(c, tk, &selected, locales.ArcUserMenuTheme(), items, kit.CompactOpts{}).AlignSelf(ui.Center).Shrink(0).OnChange(func() {
			a.s.SetTheme(themes[selected])
		})
	})
}

func (a *Account) languageRow(c *ui.Context, tk theme.Tokens) {
	codes := []string{english, chinese}
	selected := 0
	if i18n.Base(i18n.Locale()) == chinese {
		selected = 1
	}
	items := []kit.SegmentItem{{Text: "EN"}, {Text: "中文"}}
	a.row(c, tk, "languages", locales.ArcUserMenuLanguage(), "", func() {
		kit.SegmentedCompact(c, tk, &selected, locales.ArcUserMenuLanguage(), items, kit.CompactOpts{}).AlignSelf(ui.Center).Shrink(0).OnChange(func() {
			a.s.SetLanguage(codes[selected])
		})
	})
}

func (a *Account) notificationsRow(c *ui.Context, tk theme.Tokens) {
	note := ""
	switch {
	case !a.s.NotificationsSupported():
		note = locales.NotificationsUnavailableDesktop()
	case a.s.Prefs.NotificationsDenied:
		note = locales.NotificationsUnavailableDeniedDesktop()
	}
	selected := toggleOff
	if note == "" && a.s.Prefs.Notifications == prefs.NotificationsOn {
		selected = toggleOn
	}
	items := []kit.SegmentItem{{Text: locales.ArcUserMenuTogglesOff()}, {Text: locales.ArcUserMenuTogglesOn()}}
	label := locales.ArcUserMenuNotifications()
	controlLabel := label
	if note != "" {
		controlLabel = label + ", " + note
	}
	a.row(c, tk, "bell", label, note, func() {
		kit.SegmentedCompact(c, tk, &selected, controlLabel, items, kit.CompactOpts{Wide: true, Disabled: note != ""}).
			AlignSelf(ui.Center).Shrink(0).OnChange(func() {
			a.s.SetNotifications(selected == toggleOn)
		})
	})
}
