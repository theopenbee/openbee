package settings

import (
	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/api"
	"github.com/theopenbee/openbee/desktop/internal/views/envpanel"
	"github.com/theopenbee/openbee/desktop/locales"
)

const (
	panelsGap   = 40
	scopeGlobal = "global"
	scopeBee    = "bee"
)

func (d *Dialog) env(c *ui.Context) {
	ui.Column(c).MinWidth(0).Gap(panelsGap).Children(func() {
		envpanel.New(d.s, scopeGlobal, "", locales.EnvConfigGlobalTitle(), locales.EnvConfigGlobalDescription(), true).View(c)
		envpanel.New(d.s, scopeBee, api.DefaultBeeID, locales.EnvConfigBeeTitle(), locales.EnvConfigBeeDescription(), true).View(c)
	})
}
