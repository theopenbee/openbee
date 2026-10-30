package kit

import (
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/theme"
)

const (
	durationInstant  = 120 * time.Millisecond
	durationStandard = 240 * time.Millisecond
	popoverOffset    = 6
	popoverPadding   = 5
	popoverRise      = -5
	popoverScale     = 0.97
	tooltipMaxWidth  = 240
	tooltipPaddingY  = 12
	tooltipPaddingX  = 16
	tooltipOffset    = 8
	tooltipBorderMix = 0.14
	regularWeight    = 400
)

var (
	easeEnter = ui.CubicBezier(0.16, 1, 0.3, 1)
	morph     = ui.Animation{Spring: ui.SpringOptions{Duration: 420 * time.Millisecond}}
	instant   = ui.Animation{Duration: durationInstant, Ease: easeStandard}
	standard  = ui.Animation{Duration: durationStandard, Ease: easeStandard}
)

func Popover(c *ui.Context, tk theme.Tokens, anchor ui.Element, open *bool, width float32, body func()) ui.Element {
	return ui.PopoverBase(c, anchor, open, func(panel ui.Element) {
		if width > 0 {
			panel.Width(width)
		} else {
			panel.MinWidth(anchor.Bounds().W)
		}
		floatingPanel(panel, tk, theme.RadiusPanel).Margin(popoverOffset, 0, 0, 0).Padding(popoverPadding)
		appear(panel, durationFast, popoverRise, popoverScale)
		body()
	})
}

func Tooltip(c *ui.Context, tk theme.Tokens, anchor ui.Element, text string) ui.Element {
	if text == "" {
		return anchor
	}
	ui.TooltipBase(c, anchor, func(tip ui.Element) {
		tip.AttachTo(anchor, ui.AnchorTop, ui.AnchorBottom).Margin(0, 0, tooltipOffset, 0).
			MaxWidth(tooltipMaxWidth).Padding(tooltipPaddingY, tooltipPaddingX).Radius(theme.RadiusControl).
			Background(tk.Foreground).Border(1, tk.Foreground.Mix(tk.Background, tooltipBorderMix)).
			TextColor(tk.Background).FontSize(bodyFontSize).FontWeight(regularWeight)
		theme.WithShadow(tip, tk.ShadowRaised)
		appear(tip, durationFast, 0, popoverScale)
		tip.Children(func() { ui.Text(c, text) })
	})
	return anchor.Description(text)
}

func floatingPanel(panel ui.Element, tk theme.Tokens, radius float32) ui.Element {
	panel.Radius(radius).Background(tk.SurfaceRaised).Border(1, tk.Border).TextColor(tk.Foreground)
	return theme.WithShadow(panel, tk.ShadowFloating)
}

func appear(e ui.Element, d time.Duration, rise, from float32) ui.Element {
	p := e.Keyframes("appear", []ui.Keyframe{{At: 0, Value: 0}, {At: d, Value: 1, Ease: easeEnter}})
	scale := from + (1-from)*p
	return e.Opacity(p).Translate(0, rise*(1-p)).Scale(scale, scale)
}
