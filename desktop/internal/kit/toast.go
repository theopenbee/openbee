package kit

import (
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/icons"
	"github.com/theopenbee/openbee/desktop/internal/theme"
	"github.com/theopenbee/openbee/desktop/locales"
)

type Toast struct {
	Tone               Tone
	Title, Description string
}

const (
	toastTimeout      = 4500 * time.Millisecond
	toastEdge         = 24
	toastStackGap     = 8
	toastWidth        = 416
	toastGap          = 12
	toastPaddingY     = 14
	toastPaddingEnd   = 14
	toastPaddingStart = 16
	toastIconBox      = 30
	toastIconGlyph    = 16
	toastCopyTop      = 5
	toastCloseBox     = 32
	toastCloseGlyph   = 16
	toastSurfaceMix   = 0.06
	toastIconBorder   = 0.24
	toastIconFill     = 0.10
	toastEnterRise    = 16
	toastEnterScale   = 0.96
	toastExitDrop     = 8
	toastExitScale    = 0.97
)

var toastMotion = ui.ElementTransition{
	Animation: &morph,
	Enter:     &ui.Motion{Y: toastEnterRise, Scale: toastEnterScale},
	Exit:      &ui.Motion{Y: toastExitDrop, Scale: toastExitScale},
}

func (t Tone) kind() string {
	switch t {
	case ToneInfo:
		return "info"
	case ToneSuccess:
		return "success"
	case ToneWarning:
		return "warning"
	case ToneDanger:
		return "error"
	}
	return ""
}

func (t Tone) toastIcon() string {
	switch t {
	case ToneSuccess:
		return "check"
	case ToneWarning:
		return "triangle-alert"
	case ToneDanger:
		return "x"
	}
	return "info"
}

func ShowToasts(c *ui.Context, tk theme.Tokens, queue *[]Toast) {
	for _, t := range *queue {
		c.AddToast(ui.Toast{Title: t.Title, Description: t.Description, Type: t.Tone.kind(), Timeout: toastTimeout, Data: t.Tone})
	}
	*queue = nil
	w, _ := c.Size()
	width := min(toastWidth, w-2*toastEdge)
	ui.ToastViewportBase(c, func(viewport ui.Element, toasts []ui.Toast) {
		viewport.Padding(toastEdge).AlignItems(ui.End).Gap(toastStackGap)
		for _, t := range toasts {
			toastCard(c, tk, t, width)
		}
	})
}

func toastCard(c *ui.Context, tk theme.Tokens, t ui.Toast, width float32) {
	tone, _ := t.Data.(Tone)
	parts := ui.ToastBase(c, t)
	card := parts.Root.Row().AlignItems(ui.Start).Gap(toastGap).Width(width).
		Padding(toastPaddingY, toastPaddingEnd, toastPaddingY, toastPaddingStart).Radius(theme.RadiusPanel).
		Background(tk.Surface.Mix(tk.Background, toastSurfaceMix)).Border(1, tk.Border).TextColor(tk.Foreground).
		Transition(toastMotion)
	theme.WithShadow(card, tk.ShadowFloating)
	card.Children(func() {
		accent := tone.color(tk)
		ui.Box(c).Size(toastIconBox, toastIconBox).Shrink(0).Center().Radius(theme.RadiusPill).
			Background(tk.Surface.Mix(accent, toastIconFill)).Border(1, tk.Border.Mix(accent, toastIconBorder)).
			TextColor(accent).Children(func() {
			ui.Icon(c, icons.Get(tone.toastIcon())).Size(toastIconGlyph, toastIconGlyph)
		})
		ui.Column(c).Grow(1).MinWidth(0).Gap(rowCopyGap).Padding(toastCopyTop, 0, 0, 0).FontSize(bodyFontSize).Children(func() {
			ui.Text(c, t.Title).FontWeight(mediumWeight).SingleLine()
			if t.Description != "" {
				ui.Text(c, t.Description).TextColor(tk.TextSecondary)
			}
		})
		quiet(c, tk, parts.CloseButton(), "x", locales.ArcToastDismiss(), toastCloseBox, toastCloseGlyph, theme.RadiusPill, false).
			Shrink(0).Margin(-1, -2, 0, 0)
	})
}
