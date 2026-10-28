package settings

import (
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/kit"
	"github.com/theopenbee/openbee/desktop/internal/theme"
	"github.com/theopenbee/openbee/desktop/locales"
)

const (
	stateDot        = 6
	stateSpinner    = 12
	statePulse      = 1500 * time.Millisecond
	statePulseFloor = 0.3
)

type stateLook struct {
	tone  kit.Tone
	label func() string
	dot   func(theme.Tokens) ui.Color
	pulse bool
}

var stateLooks = map[string]stateLook{
	"running":        {kit.ToneSuccess, locales.PluginsStateRunning, func(tk theme.Tokens) ui.Color { return tk.Success }, false},
	"starting":       {kit.ToneInfo, locales.PluginsStateStarting, func(tk theme.Tokens) ui.Color { return tk.Accent }, true},
	"retrying":       {kit.ToneWarning, locales.PluginsStateRetrying, func(tk theme.Tokens) ui.Color { return tk.Warning }, true},
	"failed":         {kit.ToneDanger, locales.PluginsStateFailed, func(tk theme.Tokens) ui.Color { return tk.Danger }, false},
	"not_configured": {kit.ToneWarning, locales.PluginsStateNotConfigured, func(tk theme.Tokens) ui.Color { return tk.Warning }, false},
	"disabled":       {kit.ToneNeutral, locales.PluginsStateDisabled, func(tk theme.Tokens) ui.Color { return tk.BorderStrong }, false},
}

func stateBadge(c *ui.Context, tk theme.Tokens, state string, uninstalling bool) ui.Element {
	if uninstalling {
		return kit.SmallBadgeIcon(c, tk, locales.PluginsStateUninstalling(), kit.ToneNeutral, func() {
			kit.Spinner(c, tk, stateSpinner).TextColor(tk.TextSecondary)
		})
	}
	look, known := stateLooks[state]
	label := state
	dot := tk.BorderStrong
	if known {
		label, dot = look.label(), look.dot(tk)
	}
	return kit.SmallBadgeIcon(c, tk, label, look.tone, func() {
		mark := ui.Box(c).Size(stateDot, stateDot).Shrink(0).Radius(theme.RadiusPill).Background(dot)
		if look.pulse {
			mark.Opacity(1 - (1-statePulseFloor)*mark.Loop("pulse", statePulse, ui.Bounce(ui.EaseInOut)))
		}
	})
}
