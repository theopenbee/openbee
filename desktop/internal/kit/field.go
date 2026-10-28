package kit

import (
	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/theme"
)

const (
	fieldGap      = 8
	fieldNoteLift = 2
)

type fieldScope struct{ err string }

type fieldScopeKey struct{}

func scopeOf(c *ui.Context) *fieldScope {
	return ui.Local(c.Root(), fieldScopeKey{}, func() fieldScope { return fieldScope{} })
}

func fieldError(c *ui.Context) string {
	return scopeOf(c).err
}

func Field(c *ui.Context, tk theme.Tokens, label, help, errText string, control func()) ui.Element {
	scope := scopeOf(c)
	return ui.Field(c, label, func() {
		outer := scope.err
		scope.err = errText
		control()
		scope.err = outer
		if help != "" {
			fieldNote(c, help, tk.TextMuted)
		}
		if errText != "" {
			fieldNote(c, errText, tk.Danger)
		}
	}).Gap(fieldGap)
}

func fieldNote(c *ui.Context, text string, color ui.Color) ui.Element {
	return ui.Text(c, text).FontSize(smallFontSize).TextColor(color).Margin(fieldNoteLift, 0, 0, 0)
}
