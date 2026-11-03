package kit

import (
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/icons"
	"github.com/theopenbee/openbee/desktop/internal/theme"
	"github.com/theopenbee/openbee/desktop/locales"
)

const (
	copyFeedback = 1900 * time.Millisecond
	copyIcon     = 16
	copyPaddingX = 12
)

func CopyButton(c *ui.Context, tk theme.Tokens, text, label string) ui.Element {
	if label == "" {
		label = locales.ArcCopyButtonCopy()
	}
	copiedText := locales.ArcCopyButtonCopied()
	b := ui.ButtonBase(c).Label(label).MinHeight(theme.ControlSM).PaddingX(copyPaddingX).Gap(buttonGap).
		Radius(theme.RadiusControl).FontSize(bodyFontSize).FontWeight(mediumWeight).TextColor(tk.Foreground)
	copied := copyFeedbackOn(c, b, text, label)
	bg, border := tk.Surface, tk.Border
	if b.Hovered() || b.Pressed() {
		bg, border = tk.SurfaceMuted, tk.BorderStrong
	}
	b.Background(b.AnimateColor("bg", bg, fast)).Border(1, b.AnimateColor("border", border, fast))
	squeezeOnPress(b, pressScale)
	labelWidth := max(textWidth(c, label), textWidth(c, copiedText))
	return b.Children(func() {
		shown := label
		if copied {
			shown = copiedText
		}
		copyGlyph(c, tk, copied)
		ui.Text(c, shown).SingleLine().MinWidth(labelWidth)
	})
}

func CopyIconButton(c *ui.Context, tk theme.Tokens, text, label string) ui.Element {
	if label == "" {
		label = locales.ArcCopyButtonCopy()
	}
	b := ui.ButtonBase(c).Label(label).Size(theme.ControlSM, theme.ControlSM).Shrink(0).Radius(theme.RadiusControl)
	copied := copyFeedbackOn(c, b, text, label)
	bg, fg := tk.SurfaceMuted.Alpha(0), tk.TextSecondary
	if b.Hovered() || b.Pressed() {
		bg, fg = tk.SurfaceMuted, tk.Foreground
	}
	b.Background(b.AnimateColor("bg", bg, fast)).TextColor(b.AnimateColor("fg", fg, fast))
	squeezeOnPress(b, iconPressScale)
	return b.Children(func() { copyGlyph(c, tk, copied) })
}

func copyFeedbackOn(c *ui.Context, b ui.Element, text, label string) bool {
	copiedAt := ui.Local(b, "copied", func() time.Time { return time.Time{} })
	if b.Clicked() {
		c.WriteClipboard(text)
		*copiedAt = c.Now()
		c.Announce(locales.ArcCopyButtonCopiedAnnouncement(label))
	}
	left := copyFeedback - c.Now().Sub(*copiedAt)
	copied := !copiedAt.IsZero() && left > 0
	if copied {
		c.After(left)
	}
	return copied
}

func squeezeOnPress(b ui.Element, squeeze float32) {
	scale := float32(1)
	if b.Pressed() {
		scale = squeeze
	}
	scale = b.AnimateTo("press", scale, ui.Snappy)
	b.Scale(scale, scale)
}

func copyGlyph(c *ui.Context, tk theme.Tokens, copied bool) {
	icon := "copy"
	if copied {
		icon = "check"
	}
	glyph := ui.Icon(c, icons.Get(icon)).Size(copyIcon, copyIcon).Shrink(0)
	if copied {
		glyph.TextColor(tk.Success)
	}
}

func textWidth(c *ui.Context, s string) float32 {
	w, _ := c.MeasureText(0, ui.Span{Text: s, Size: bodyFontSize, Weight: mediumWeight})
	return w
}
