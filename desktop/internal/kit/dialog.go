package kit

import (
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/icons"
	"github.com/theopenbee/openbee/desktop/internal/theme"
	"github.com/theopenbee/openbee/desktop/locales"
)

type DialogOpts struct {
	Title, Description string
	Width              float32
	Height             float32
	Flush              bool
	Locked             bool
	Footer             func()
}

const (
	dialogInset       = 16
	dialogPadding     = 24
	dialogHeaderGap   = 20
	dialogTitleSize   = 18
	dialogDescGap     = 8
	dialogFooterGap   = 16
	dialogButtonGap   = 8
	dialogRise        = 8
	dialogScale       = 0.96
	dialogRiseTime    = 400 * time.Millisecond
	closeBox          = 32
	closeGlyph        = 16
	closeOffset       = (dialogTitleSize*theme.BodyLineHeight - closeBox) / 2
	dialogCloseRest   = 0.5
	closePressedScale = 0.96
)

func Dialog(c *ui.Context, tk theme.Tokens, open *bool, o DialogOpts, body func()) ui.Element {
	w, h := c.Size()
	width := o.Width
	if width <= 0 {
		width = theme.DialogWidth
	}
	width = min(width, w-2*dialogInset)
	return ui.DialogBase(c, open, func(backdrop, panel ui.Element) {
		if o.Locked && panel.Dismissed() {
			*open = true
		}
		backdrop.Background(tk.Scrim.Alpha(fadeIn(backdrop)))
		rise := panel.Keyframes("rise", []ui.Keyframe{{At: 0, Value: 0}, {At: dialogRiseTime, Value: 1, Ease: easeEnter}})
		scale := dialogScale + (1-dialogScale)*rise
		frame := floatingPanel(panel, tk, theme.RadiusSurface).Label(o.Title).Description(o.Description).
			Width(width).MaxHeight(h-2*dialogInset).Clip().
			Opacity(panel.Keyframes("fade", []ui.Keyframe{{At: 0, Value: 0}, {At: durationFast, Value: 1, Ease: easeEnter}})).
			Translate(0, dialogRise*(1-rise)).Scale(scale, scale)
		if o.Height > 0 {
			frame.Height(min(o.Height, h-2*dialogInset))
		}
		header := func() {
			overlayHeader(c, tk, o.Title, o.Description, func() {
				closeButton(c, tk, locales.ArcDialogClose(), dialogCloseRest).Disabled(o.Locked).
					MarginY(closeOffset).OnClick(func() { *open = false })
			})
		}
		if o.Flush {
			ui.Column(c).Fill().Children(func() {
				header()
				ui.Column(c).Grow(1).Shrink(1).MinWidth(0).MinHeight(0).FontSize(bodyFontSize).Children(body)
			})
			return
		}
		ui.Scroll(c).Shrink(1).MinHeight(0).Children(func() {
			header()
			ui.Column(c).Padding(dialogPadding).FontSize(bodyFontSize).Children(func() {
				if body != nil {
					body()
				}
				if o.Footer != nil {
					footer := ui.Row(c).Justify(ui.End).Gap(dialogButtonGap)
					if body != nil {
						footer.Margin(dialogFooterGap, 0, 0, 0)
					}
					footer.Children(o.Footer)
				}
			})
		})
	})
}

func overlayHeader(c *ui.Context, tk theme.Tokens, title, description string, close func()) ui.Element {
	return ui.Row(c).Shrink(0).AlignItems(ui.Start).Justify(ui.SpaceBetween).Gap(dialogHeaderGap).Padding(dialogPadding).
		BorderWidth(0, 0, 1, 0).BorderColor(tk.Border).Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Children(func() {
			ui.Text(c, title).FontSize(dialogTitleSize).FontWeight(mediumWeight).Role(ui.RoleHeading).Level(2)
			if description != "" {
				ui.Text(c, description).FontSize(bodyFontSize).TextColor(tk.TextSecondary).Margin(dialogDescGap, 0, 0, 0)
			}
		})
		close()
	})
}

func closeButton(c *ui.Context, tk theme.Tokens, label string, rest float32) ui.Element {
	b := ui.ButtonBase(c).Label(label).Size(closeBox, closeBox).Shrink(0).Radius(theme.RadiusControl)
	active := b.Hovered() || b.Pressed()
	bg, fg := tk.SurfaceMuted.Alpha(rest), tk.TextSecondary
	if active {
		bg, fg = tk.SurfaceMuted, tk.Foreground
	}
	b.Background(b.AnimateColor("bg", bg, fast)).Border(1, tk.Border).TextColor(b.AnimateColor("fg", fg, fast))
	scale := float32(1)
	if b.Pressed() {
		scale = closePressedScale
	}
	scale = b.AnimateTo("press", scale, ui.Snappy)
	return b.Scale(scale, scale).Children(func() {
		ui.Icon(c, icons.Get("x")).Size(closeGlyph, closeGlyph)
	})
}

func fadeIn(e ui.Element) float32 {
	return e.Keyframes("fade-in", []ui.Keyframe{{At: 0, Value: 0}, {At: durationStandard, Value: 1, Ease: easeEnter}})
}
