package kit

import (
	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/theme"
	"github.com/theopenbee/openbee/desktop/locales"
)

const (
	inputPadding  = 12
	revealBox     = 30
	revealGlyph   = 18
	revealInset   = (theme.ControlMD - 2 - revealBox) / 2
	revealSpacing = 8
)

func TextInput(c *ui.Context, tk theme.Tokens, value *string, label, placeholder string) ui.Element {
	var in ui.Element
	inputShell(c, tk, ui.Row(c).MinHeight(theme.ControlMD).AlignItems(ui.Stretch)).Children(func() {
		in = editable(c, ui.TextInputBase(c, value), label, placeholder).PaddingX(inputPadding).Grow(1)
	})
	return in
}

func PasswordInput(c *ui.Context, tk theme.Tokens, value *string, shown *bool, label, placeholder string) ui.Element {
	var in ui.Element
	inputShell(c, tk, ui.Row(c).MinHeight(theme.ControlMD).AlignItems(ui.Stretch)).Children(func() {
		in = editable(c, ui.TextInputBase(c, value), label, placeholder).
			PaddingStart(inputPadding).PaddingEnd(revealSpacing).Grow(1)
		icon, name := "eye", locales.ArcPasswordFieldShow()
		if *shown {
			icon, name = "eye-off", locales.ArcPasswordFieldHide()
		} else {
			in.Password()
		}
		quietButton(c, tk, icon, name, revealBox, revealGlyph, theme.RadiusControl, *shown).
			AlignSelf(ui.Center).MarginEnd(revealInset).KeepFocus().
			OnClick(func() { *shown = !*shown })
	})
	return in
}

func TextArea(c *ui.Context, tk theme.Tokens, value *string, label, placeholder string, minLines, maxLines int) ui.Element {
	var in ui.Element
	inputShell(c, tk, ui.Column(c)).Children(func() {
		in = editable(c, ui.TextAreaBase(c, value), label, placeholder).Padding(inputPadding).Lines(minLines, maxLines)
	})
	return in
}

func inputShell(c *ui.Context, tk theme.Tokens, shell ui.Element) ui.Element {
	border := tk.BorderStrong
	switch {
	case fieldError(c) != "":
		border = tk.Danger
	case shell.FocusWithin() || shell.Hovered():
		border = tk.Foreground
	}
	return shell.Radius(theme.RadiusControl).Background(tk.Surface).Clip().
		Border(1, shell.AnimateColor("border", border, fast))
}

func editable(c *ui.Context, in ui.Element, label, placeholder string) ui.Element {
	return in.Label(label).Placeholder(placeholder).FontSize(bodyFontSize).MinWidth(0).
		FocusRing(false).Error(fieldError(c))
}
