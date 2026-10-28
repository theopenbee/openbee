package kit

import (
	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/icons"
	"github.com/theopenbee/openbee/desktop/internal/theme"
	"github.com/theopenbee/openbee/desktop/locales"
)

type SelectOption struct{ Value, Text string }

const (
	selectGap           = 12
	selectPaddingX      = 12
	selectChevron       = 16
	selectOffset        = 4
	selectPopupPadding  = 6
	selectViewportPadY  = 2
	selectPopupMaxH     = 320
	selectPopupMaxW     = 384
	selectWindowEdge    = 20
	selectPopupRise     = -6
	selectItemHeight    = 36
	selectItemPadStart  = 11
	selectItemPadEnd    = 34
	selectIndicatorEnd  = 10
	selectItemRadiusCut = 4
)

func Select(c *ui.Context, tk theme.Tokens, value *string, label string, options []SelectOption, placeholder string) ui.Element {
	if placeholder == "" {
		placeholder = locales.ArcSelectPlaceholder()
	}
	shown, muted := placeholder, true
	if *value != "" {
		shown, muted = "", false
		for _, o := range options {
			if o.Value == *value {
				shown = o.Text
				break
			}
		}
	}
	sel := ui.SelectBase(c, value)
	open := sel.Open()
	t := sel.Trigger.Label(label).MinHeight(theme.ControlMD).PaddingX(selectPaddingX).Gap(selectGap).
		Justify(ui.SpaceBetween).AlignItems(ui.Center).Radius(theme.RadiusControl).FontSize(bodyFontSize).TextColor(tk.Foreground)
	bg, border := tk.Surface, tk.Border
	if open || t.Hovered() || t.Pressed() || t.FocusVisible() {
		bg, border = tk.SurfaceMuted, tk.BorderStrong
	}
	t.Background(t.AnimateColor("bg", bg, fast)).Border(1, t.AnimateColor("border", border, fast))
	t.Children(func() {
		text := ui.Text(c, shown).SingleLine().Grow(1).MinWidth(0)
		if muted {
			text.TextColor(tk.TextMuted)
		}
		turn := float32(0)
		if open {
			turn = 180
		}
		chevron := ui.Icon(c, icons.Get("chevron-down")).Size(selectChevron, selectChevron).Shrink(0).TextColor(tk.TextMuted)
		chevron.Rotate(chevron.AnimateTo("turn", turn, ui.Snappy))
	})
	w, _ := c.Size()
	popupRadius := theme.Concentric(theme.RadiusPanel, selectPopupPadding)
	itemRadius := theme.Concentric(theme.RadiusControl, selectItemRadiusCut)
	sel.Popup(func(panel ui.Element) {
		floatingPanel(panel, tk, popupRadius).Margin(selectOffset, 0, 0, 0).Padding(selectPopupPadding).
			MaxWidth(min(selectPopupMaxW, w-selectWindowEdge)).MaxHeight(selectPopupMaxH).Clip()
		appear(panel, durationStandard, selectPopupRise, popoverScale)
		ui.Scroll(c).Shrink(1).MinHeight(0).Children(func() {
			ui.Column(c).PaddingY(selectViewportPadY).Children(func() {
				for _, o := range options {
					item := sel.Item(o.Value).MinHeight(selectItemHeight).Padding(0, selectItemPadEnd, 0, selectItemPadStart).
						Radius(itemRadius).FontSize(bodyFontSize).TextColor(tk.Foreground)
					item.Background(item.AnimateColor("bg", highlight(tk, item.Highlighted()), instant))
					item.Children(func() {
						ui.Text(c, o.Text).SingleLine().MinWidth(0)
						if o.Value == *value {
							ui.Icon(c, icons.Get("check")).Size(selectChevron, selectChevron).
								Attach(ui.AnchorRight, ui.AnchorRight).Right(selectIndicatorEnd)
						}
					})
				}
			})
		})
	})
	return t
}

func highlight(tk theme.Tokens, on bool) ui.Color {
	if on {
		return tk.SurfaceMuted
	}
	return tk.SurfaceMuted.Alpha(0)
}
