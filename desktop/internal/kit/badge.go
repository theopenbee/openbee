package kit

import (
	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/theme"
)

const (
	badgeHeight   = 26
	badgePaddingX = 10
	badgeTracking = -0.01 * smallFontSize
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
	fg, bg, border := badgeColors(tk, tone)
	return ui.Row(c).MinHeight(badgeHeight).PaddingX(badgePaddingX).Shrink(0).
		Radius(theme.RadiusPill).Background(bg).Border(1, border).Children(func() {
		ui.Text(c, text).FontSize(smallFontSize).FontWeight(mediumWeight).TextColor(fg).
			LetterSpacing(badgeTracking).FontFeatures("tnum").LineHeight(1).SingleLine()
	})
}
