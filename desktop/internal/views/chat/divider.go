package chat

import (
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/model"
	"github.com/theopenbee/openbee/desktop/internal/theme"
	"github.com/theopenbee/openbee/desktop/locales"
)

const (
	dividerLineGap = 12
	dividerDotGap  = 5
	dividerDot     = 3
	dividerRule    = 1
)

func divider(c *ui.Context, tk theme.Tokens, ts int64, now time.Time, lang string) {
	stamp := model.MessageTime(ts, now, lang)
	ui.Row(c).Gap(dividerLineGap).FontSize(metaSize).TextColor(tk.TextMuted).Children(func() {
		ui.Box(c).Height(dividerRule).Grow(1).Background(tk.Border)
		ui.Row(c).Shrink(0).Gap(dividerDotGap).Children(func() {
			ui.Text(c, locales.ChatNewConversation()).SingleLine()
			if stamp != "" {
				ui.Box(c).Size(dividerDot, dividerDot).Radius(theme.RadiusPill).Background(tk.TextMuted)
				ui.Text(c, stamp).FontFeatures("tnum").SingleLine()
			}
		})
		ui.Box(c).Height(dividerRule).Grow(1).Background(tk.Border)
	})
}
