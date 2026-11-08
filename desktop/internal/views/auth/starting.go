package auth

import (
	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/kit"
	"github.com/theopenbee/openbee/desktop/internal/theme"
	"github.com/theopenbee/openbee/desktop/locales"
)

const startingSpinner = 24

func (a *Screens) Starting(c *ui.Context, tk theme.Tokens) {
	a.layout(c, tk, locales.StartingTitle(), locales.StartingDescription(), func() {
		ui.Row(c).Justify(ui.Center).Role(ui.RoleStatus).Children(func() {
			kit.Spinner(c, tk, startingSpinner)
		})
	})
}
