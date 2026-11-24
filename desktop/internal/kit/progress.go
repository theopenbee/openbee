package kit

import (
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/model"
	"github.com/theopenbee/openbee/desktop/internal/theme"
)

const (
	progressGap   = 8
	progressGlyph = 16
)

func OperationProgress(c *ui.Context, tk theme.Tokens, label string, started time.Time) ui.Element {
	now := c.Now()
	c.After(untilNextSecond(started, now))
	return ui.Row(c).MinHeight(theme.ControlSM).AlignItems(ui.Center).Gap(progressGap).Role(ui.RoleStatus).Children(func() {
		Spinner(c, tk, progressGlyph).TextColor(tk.Accent)
		Shimmer(c, tk, label)
		ui.Text(c, model.Elapsed(started, now)).FontSize(bodyFontSize).FontFeatures("tnum").TextColor(tk.TextSecondary).SingleLine()
	})
}

func untilNextSecond(started, now time.Time) time.Duration {
	if started.IsZero() || now.Before(started) {
		return time.Second
	}
	return time.Second - now.Sub(started)%time.Second
}
