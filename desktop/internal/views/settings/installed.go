package settings

import (
	"strconv"

	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/api"
	"github.com/theopenbee/openbee/desktop/internal/icons"
	"github.com/theopenbee/openbee/desktop/internal/kit"
	"github.com/theopenbee/openbee/desktop/internal/l10n"
	"github.com/theopenbee/openbee/desktop/internal/theme"
	"github.com/theopenbee/openbee/desktop/locales"
)

const (
	tablePadX       = 24
	tableHeadTop    = 20
	tableHeadBottom = 16
	tableHeadGap    = 16
	tableBottom     = 8
	tableColumnsPad = 8
	tableGap        = 16
	tableRowPadY    = 16
	tableRowGap     = 12
	versionColumn   = 88
	statusColumn    = 192
	botColumn       = 160
	actionsColumn   = 176
	actionsGap      = 8
	nameGap         = 12
	detailsGapX     = 12
	detailsGapY     = 6
	errorGap        = 4
	botGlyph        = 14
	botGap          = 6
	rowMenuWidth    = 192
	rowMenuGlyph    = 16
	menuItemHeight  = 36
	menuItemPadX    = 11
	menuItemGap     = 10
	menuItemInset   = 6
	menuSeparatorY  = 4
	menuDangerTint  = 0.08
	menuDisabled    = 0.5
	skeletonRows    = 4
	skeletonColumns = 5
	skeletonColumnW = 80
	skeletonColumnH = 12
	skeletonFace    = 32
	skeletonNameW   = 128
	skeletonCellW   = 80
	skeletonCellH   = 16
	skeletonGapX    = 32
	skeletonFaceGap = 12
)

type installedState struct {
	remove uninstallState
}

type installedKey struct{}

type menuKey struct{}

func (d *Dialog) installedTab(c *ui.Context, tk theme.Tokens) {
	r := d.s.Plugins().List
	root := ui.Column(c).MinWidth(0)
	st := ui.Local(root, installedKey{}, func() installedState { return installedState{} })
	root.Children(func() {
		switch {
		case r.Err != nil:
			kit.Alert(c, tk, kit.AlertOpts{Tone: kit.ToneDanger, Title: locales.PluginsInstalledError(), Body: l10n.ErrorText(r.Err),
				ActionLabel: locales.CommonRetry(), ActionLoading: r.Loading, OnAction: r.Invalidate})
		case !r.Loaded:
			installedSkeleton(c, tk)
		case len(r.Data) == 0:
			insetCard(c, tk).Children(func() {
				kit.EmptyState(c, tk, "puzzle", locales.PluginsEmptyInstalledTitle(), locales.PluginsEmptyInstalledDescription(), func() {
					kit.Button(c, tk, locales.PluginsEmptyInstalledAction(), kit.ButtonOpts{Variant: kit.Primary, Size: kit.SizeMD}).
						OnClick(func() { d.pluginsTab = tabMarket })
				})
			})
		default:
			d.installedPanel(c, tk, st, r.Data)
		}
		d.uninstallDialog(c, tk, &st.remove)
	})
}

func (d *Dialog) installedPanel(c *ui.Context, tk theme.Tokens, st *installedState, list []api.PluginStatus) {
	running := 0
	for _, plugin := range list {
		if plugin.State == stateRunning {
			running++
		}
	}
	wide := d.contentWidth() >= container3XL
	insetCard(c, tk).Clip().Padding(0, 0, tableBottom, 0).Children(func() {
		ui.Row(c).MinWidth(0).AlignItems(ui.Start).Justify(ui.SpaceBetween).Gap(tableHeadGap).
			Padding(tableHeadTop, tablePadX, tableHeadBottom, tablePadX).Children(func() {
			heading(c, locales.PluginsInstalledTitle()).SingleLine().MinWidth(0)
			ui.Text(c, locales.PluginsInstalledSummary(len(list), strconv.Itoa(running))).FontSize(metaSize).FontFeatures("tnum").
				TextColor(tk.TextSecondary).SingleLine().Shrink(0)
		})
		if wide {
			tableColumns(c, tk)
		}
		for _, plugin := range list {
			d.installedRow(c, tk, st, plugin, wide)
		}
	})
}

func tableColumns(c *ui.Context, tk theme.Tokens) {
	ui.Row(c).MinWidth(0).Gap(tableGap).Padding(0, tablePadX, tableColumnsPad, tablePadX).TextColor(tk.TextSecondary).Children(func() {
		column := func(text string, width float32) {
			cell := ui.Text(c, text).FontSize(metaSize).SingleLine().MinWidth(0)
			if width > 0 {
				cell.Width(width).Shrink(0)
			} else {
				cell.Grow(1).Basis(0)
			}
		}
		column(locales.PluginsColumnsPlugin(), 0)
		column(locales.PluginsColumnsVersion(), versionColumn)
		column(locales.PluginsColumnsStatus(), statusColumn)
		column(locales.PluginsColumnsBotName(), botColumn)
		ui.Box(c).Width(actionsColumn).Shrink(0)
	})
}

func (d *Dialog) installedRow(c *ui.Context, tk theme.Tokens, st *installedState, plugin api.PluginStatus, wide bool) {
	p := d.s.Plugins()
	name := displayName(plugin.Name, plugin.ID)
	busy := p.Uninstalling(plugin.ID)
	var row ui.Element
	if wide {
		row = ui.Row(c).AlignItems(ui.Center).Gap(tableGap)
	} else {
		row = ui.Column(c).Gap(tableRowGap)
	}
	row.Key(plugin.ID).MinWidth(0).Padding(tableRowPadY, tablePadX).BorderWidth(cardBorder, 0, 0, 0).BorderColor(tk.BorderSubtle)
	row.Children(func() {
		if wide {
			d.pluginLink(c, tk, plugin, name).Grow(1).Basis(0)
			versionCell(c, tk, plugin).Width(versionColumn).Shrink(0)
			stateCell(c, tk, plugin, busy).Width(statusColumn).Shrink(0)
			botCell(c, tk, plugin, false).Width(botColumn).Shrink(0)
			d.rowActions(c, tk, st, plugin, name, busy).Width(actionsColumn).Shrink(0)
			return
		}
		ui.Row(c).MinWidth(0).AlignItems(ui.Center).Gap(tableGap).Children(func() {
			d.pluginLink(c, tk, plugin, name).Grow(1)
			d.rowActions(c, tk, st, plugin, name, busy).Shrink(0)
		})
		ui.Row(c).MinWidth(0).Wrap().AlignItems(ui.Center).GapX(detailsGapX).GapY(detailsGapY).Children(func() {
			state := stateCell(c, tk, plugin, busy)
			if showsError(plugin) {
				state.WidthPercent(100)
			}
			versionCell(c, tk, plugin)
			botCell(c, tk, plugin, true)
		})
	})
}

func (d *Dialog) pluginLink(c *ui.Context, tk theme.Tokens, plugin api.PluginStatus, name string) ui.Element {
	b := ui.ButtonBase(c).Label(name).MinWidth(0).AlignItems(ui.Center).Justify(ui.Start).Gap(nameGap)
	fg := tk.Foreground
	if b.Hovered() || b.Pressed() {
		fg = tk.AccentStrong
	}
	return b.OnClick(func() { d.Open(SectionPlugins, plugin.ID) }).Children(func() {
		kit.InitialsAvatar(c, tk, name, kit.AvatarMD)
		ui.Column(c).MinWidth(0).Shrink(1).Children(func() {
			ui.Text(c, name).FontSize(textSize).FontWeight(mediumWeight).TextColor(b.AnimateColor("fg", fg, navFade)).SingleLine().MinWidth(0)
			ui.Text(c, plugin.ID).Font(theme.FontMono).FontSize(metaSize).TextColor(tk.TextSecondary).SingleLine().MinWidth(0)
		})
	})
}

func versionCell(c *ui.Context, tk theme.Tokens, plugin api.PluginStatus) ui.Element {
	text := noValue
	if plugin.Version != "" {
		text = versionPrefix + plugin.Version
	}
	return ui.Text(c, text).Font(theme.FontMono).FontSize(textSize).FontFeatures("tnum").TextColor(tk.TextSecondary).SingleLine().MinWidth(0)
}

func showsError(plugin api.PluginStatus) bool {
	return plugin.Error != "" && (plugin.State == "failed" || plugin.State == "retrying")
}

func stateCell(c *ui.Context, tk theme.Tokens, plugin api.PluginStatus, uninstalling bool) ui.Element {
	return ui.Column(c).MinWidth(0).Gap(errorGap).AlignItems(ui.Start).Children(func() {
		stateBadge(c, tk, plugin.State, uninstalling)
		if showsError(plugin) {
			text := ui.Text(c, plugin.Error).FontSize(metaSize).TextColor(tk.Danger).SingleLine().MinWidth(0).MaxWidthPercent(100)
			kit.Tooltip(c, tk, text, plugin.Error)
		}
	})
}

func botCell(c *ui.Context, tk theme.Tokens, plugin api.PluginStatus, icon bool) ui.Element {
	return ui.Row(c).MinWidth(0).AlignItems(ui.Center).Gap(botGap).Children(func() {
		if icon {
			ui.Icon(c, icons.Get("bot")).Size(botGlyph, botGlyph).Shrink(0).TextColor(tk.TextMuted)
		}
		if plugin.BotName == "" {
			ui.Text(c, noValue).FontSize(textSize).TextColor(tk.TextMuted)
			return
		}
		ui.Text(c, plugin.BotName).FontSize(textSize).TextColor(tk.Foreground).SingleLine().MinWidth(0)
	})
}

func (d *Dialog) rowActions(c *ui.Context, tk theme.Tokens, st *installedState, plugin api.PluginStatus, name string, busy bool) ui.Element {
	p := d.s.Plugins()
	acting := p.Acting() || p.Pending[plugin.ID]
	return ui.Row(c).AlignItems(ui.Center).Justify(ui.End).Gap(actionsGap).Children(func() {
		enabled := p.Enabled(plugin)
		locked := busy || acting || (!plugin.Enabled && plugin.State == stateNoConfig)
		kit.Switch(c, tk, &enabled, locales.PluginsActionToggle(name), locked).
			OnChange(func() { p.Toggle(plugin.ID, enabled, nil) })
		if d.contentWidth() >= containerLG {
			kit.Button(c, tk, locales.PluginsActionConfigure(), kit.ButtonOpts{Variant: kit.Secondary, Size: kit.SizeSM, Disabled: busy}).
				OnClick(func() { d.Open(SectionPlugins, plugin.ID) })
		}
		d.rowMenu(c, tk, st, plugin, name, busy, acting)
	})
}

func (d *Dialog) rowMenu(c *ui.Context, tk theme.Tokens, st *installedState, plugin api.PluginStatus, name string, busy, acting bool) {
	b := ui.ButtonBase(c).Label(locales.PluginsActionMoreFor(name)).Role(ui.RoleMenuButton).Disabled(busy).
		Size(theme.ControlSM, theme.ControlSM).Shrink(0).Center().Radius(theme.RadiusControl)
	open := ui.Local(b, menuKey{}, func() bool { return false })
	if busy {
		*open = false
	}
	bg, fg := tk.SurfaceMuted.Alpha(0), tk.TextSecondary
	switch {
	case busy:
		fg = tk.TextMuted
	case b.Hovered() || b.Pressed():
		bg, fg = tk.SurfaceMuted, tk.Foreground
	case *open:
		bg = tk.SurfaceMuted
	}
	b.Expanded(*open).Background(b.AnimateColor("bg", bg, navFade)).TextColor(b.AnimateColor("fg", fg, navFade)).
		OnClick(func() { *open = !*open }).Children(func() {
		ui.Icon(c, icons.Get("ellipsis")).Size(rowMenuGlyph, rowMenuGlyph)
	})
	p := d.s.Plugins()
	kit.Popover(c, tk, b, open, rowMenuWidth, func() {
		menuItem(c, tk, "rotate-cw", locales.PluginsActionRestart(), false, !plugin.Enabled || acting, func() {
			*open = false
			p.Restart(plugin.ID, nil)
		})
		ui.Box(c).Height(cardBorder).Margin(menuSeparatorY, -menuSeparatorY).Background(tk.BorderSubtle)
		menuItem(c, tk, "trash-2", locales.PluginsActionUninstall(), true, false, func() {
			*open = false
			st.remove.show(plugin.ID, name)
		})
	})
}

func menuItem(c *ui.Context, tk theme.Tokens, icon, text string, danger, disabled bool, onClick func()) {
	item := ui.ButtonBase(c).Label(text).Disabled(disabled).MinHeight(menuItemHeight).PaddingX(menuItemPadX).Gap(menuItemGap).
		AlignItems(ui.Center).Justify(ui.Start).Radius(theme.Concentric(theme.RadiusPanel, menuItemInset)).FontSize(textSize)
	fg, glyph, tint := tk.Foreground, tk.TextSecondary, tk.SurfaceMuted
	if danger {
		fg, glyph, tint = tk.Danger, tk.Danger, tk.Surface.Mix(tk.Danger, menuDangerTint)
	}
	bg := tint.Alpha(0)
	if !disabled && (item.Hovered() || item.Pressed() || item.FocusVisible()) {
		bg = tint
	}
	if disabled {
		item.Opacity(menuDisabled)
	}
	item.Background(item.AnimateColor("bg", bg, navFade)).TextColor(fg).OnClick(onClick).Children(func() {
		ui.Icon(c, icons.Get(icon)).Size(rowMenuGlyph, rowMenuGlyph).Shrink(0).TextColor(glyph)
		ui.Text(c, text).SingleLine()
	})
}

func installedSkeleton(c *ui.Context, tk theme.Tokens) {
	insetCard(c, tk).Clip().Children(func() {
		ui.Row(c).Gap(skeletonGapX).Padding(tableRowPadY, tablePadX).BorderWidth(0, 0, cardBorder, 0).BorderColor(tk.BorderSubtle).Children(func() {
			for i := range skeletonColumns {
				kit.SkeletonBar(c, tk, skeletonColumnW, skeletonColumnH).Key(i)
			}
		})
		for i := range skeletonRows {
			row := ui.Row(c).Key(i).AlignItems(ui.Center).Gap(skeletonGapX).Padding(tableRowPadY, tablePadX)
			if i > 0 {
				row.BorderWidth(cardBorder, 0, 0, 0).BorderColor(tk.BorderSubtle)
			}
			row.Children(func() {
				ui.Row(c).AlignItems(ui.Center).Gap(skeletonFaceGap).Children(func() {
					kit.SkeletonCircle(c, tk, skeletonFace)
					kit.SkeletonBar(c, tk, skeletonNameW, skeletonCellH)
				})
				for j := range skeletonColumns - 1 {
					kit.SkeletonBar(c, tk, skeletonCellW, skeletonCellH).Key(j)
				}
			})
		}
	})
}
