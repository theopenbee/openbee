package kit

import (
	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/icons"
	"github.com/theopenbee/openbee/desktop/internal/theme"
	"github.com/theopenbee/openbee/desktop/locales"
)

const (
	searchPadX  = 12
	searchGap   = 8
	searchGlyph = 18
	clearBox    = 24
	clearGlyph  = 16
)

type searchKey struct{}

func SearchField(c *ui.Context, tk theme.Tokens, query *string, label, placeholder string) ui.Element {
	field := ui.Row(c).MinHeight(theme.ControlMD).Gap(searchGap).PaddingX(searchPadX).
		Radius(theme.RadiusControl).Background(tk.Surface)
	input := ui.Local(field, searchKey{}, func() ui.Handle { return ui.Handle{} })
	focused := field.FocusWithin()
	border, glyph := tk.BorderStrong, tk.TextMuted
	if focused || field.Hovered() {
		border = tk.Foreground
	}
	if focused || *query != "" {
		glyph = tk.Foreground
	}
	return field.Border(1, field.AnimateColor("border", border, fast)).Children(func() {
		ui.Icon(c, icons.Get("search")).Size(searchGlyph, searchGlyph).Shrink(0).TextColor(field.AnimateColor("glyph", glyph, fast))
		ui.TextInputBase(c, query).Bind(input).Label(label).Placeholder(placeholder).
			FontSize(bodyFontSize).Grow(1).MinWidth(0).AlignSelf(ui.Stretch).FocusRing(false).OnTextKey(clearOnEscape)
		ui.Box(c).Size(clearBox, clearBox).Shrink(0).Children(func() {
			if *query != "" {
				clearSearch(c, tk).OnClick(func() {
					*query = ""
					input.Focus()
				})
			}
		})
	})
}

func clearOnEscape(k ui.TextKey) bool {
	if k.Key != ui.KeyEscape || k.Mods != 0 || k.Text() == "" {
		return false
	}
	k.SetText("")
	return true
}

func clearSearch(c *ui.Context, tk theme.Tokens) ui.Element {
	b := ui.ButtonBase(c).Label(locales.ArcSearchFieldClear()).Size(clearBox, clearBox).Radius(theme.RadiusControl).KeepFocus()
	bg, fg := tk.SurfaceMuted.Alpha(0), tk.TextMuted
	if b.Hovered() || b.Pressed() {
		bg, fg = tk.SurfaceMuted, tk.Foreground
	}
	return b.Background(b.AnimateColor("bg", bg, fast)).TextColor(b.AnimateColor("fg", fg, fast)).Children(func() {
		ui.Icon(c, icons.Get("x")).Size(clearGlyph, clearGlyph)
	})
}
