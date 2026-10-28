package kit

import (
	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/theme"
)

const (
	badgeHeight        = 26
	badgePaddingX      = 10
	smallBadgeHeight   = 22
	smallBadgePaddingX = 8
	smallBadgeFontSize = 11
	badgeTracking      = -0.01
)

func badgeColors(tk theme.Tokens, tone Tone) (fg, bg, border ui.Color) {
	switch tone {
	case ToneInfo:
		return tk.AccentStrong, tk.AccentSubtle, tk.Border.Mix(tk.Accent, 0.24)
	case ToneSuccess:
		return tk.Success, tk.Surface.Mix(tk.Success, 0.10), tk.Border.Mix(tk.Success, 0.25)
	case ToneWarning:
		return tk.Warning, tk.Surface.Mix(tk.Warning, 0.11), tk.Border.Mix(tk.Warning, 0.27)
	case ToneDanger:
		return tk.Danger, tk.Surface.Mix(tk.Danger, 0.10), tk.Border.Mix(tk.Danger, 0.26)
	}
	return tk.TextSecondary, tk.SurfaceMuted, tk.Border
}

func Badge(c *ui.Context, tk theme.Tokens, text string, tone Tone) ui.Element {
	return badge(c, tk, text, tone, badgeHeight, badgePaddingX, smallFontSize)
}

func SmallBadge(c *ui.Context, tk theme.Tokens, text string, tone Tone) ui.Element {
	return badge(c, tk, text, tone, smallBadgeHeight, smallBadgePaddingX, smallBadgeFontSize)
}

func badge(c *ui.Context, tk theme.Tokens, text string, tone Tone, height, padX, size float32) ui.Element {
	fg, bg, border := badgeColors(tk, tone)
	return ui.Row(c).MinHeight(height).PaddingX(padX).Shrink(0).
		Radius(theme.RadiusPill).Background(bg).Border(1, border).Children(func() {
		ui.Text(c, text).FontSize(size).FontWeight(mediumWeight).TextColor(fg).
			LetterSpacing(badgeTracking * size).FontFeatures("tnum").LineHeight(1).SingleLine()
	})
}
