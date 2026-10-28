package kit

import (
	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/theme"
)

const (
	tabsPadding        = 4
	tabsGap            = 4
	tabMinWidth        = 88
	tabPaddingX        = 12
	compactTabPaddingX = 8
	tabRadiusTrim      = 3
)

func Tabs(c *ui.Context, tk theme.Tokens, selected *int, label string, items []string) ui.Element {
	return tabs(c, tk, selected, label, items, tabMinWidth, tabPaddingX)
}

func CompactTabs(c *ui.Context, tk theme.Tokens, selected *int, label string, items []string) ui.Element {
	return tabs(c, tk, selected, label, items, 0, compactTabPaddingX)
}

func tabs(c *ui.Context, tk theme.Tokens, selected *int, label string, items []string, minWidth, padX float32) ui.Element {
	parts := ui.TabsBase(c, selected, len(items))
	radius := theme.Concentric(theme.RadiusControl, tabRadiusTrim)
	list := parts.List.Label(label).AlignSelf(ui.Start).AlignItems(ui.Stretch).Padding(tabsPadding).Gap(tabsGap).
		Radius(theme.RadiusControl).Background(tk.SurfaceMuted).Border(1, tk.Border)
	var chosen ui.Element
	list.Children(func() {
		for i, text := range items {
			on := i == *selected
			tab := parts.Tab(i).MinWidth(minWidth).MinHeight(theme.ControlSM).PaddingX(padX).
				Justify(ui.Center).Radius(radius).FontSize(bodyFontSize).FontWeight(mediumWeight)
			if on {
				chosen = tab
			}
			fg := tk.TextMuted
			switch {
			case on:
				fg = tk.AccentStrong
			case tab.Hovered() || tab.Pressed():
				fg = tk.Foreground
			}
			tab.TextColor(tab.AnimateColor("fg", fg, fast))
			tab.Children(func() {
				ui.Text(c, text).SingleLine()
			})
		}
	})
	if len(items) == 0 {
		return list
	}
	return paintSelection(c, tk, list, chosen, radius)
}
