package kit

import (
	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/theme"
)

const (
	panelPaddingX   = 24
	panelHeaderTop  = 20
	panelHeaderEnd  = 16
	panelBodyBottom = 24
	panelTitleSize  = 16
	panelHeaderGap  = 16
	panelTrailGap   = 8
	panelDescGap    = 4
)

func Panel(c *ui.Context, tk theme.Tokens, title, description string, trailing, body func()) ui.Element {
	return ui.Column(c).MinWidth(0).Clip().Radius(theme.RadiusSurface).Background(tk.Surface).Border(1, tk.Border).Children(func() {
		ui.Row(c).AlignItems(ui.Start).Gap(panelHeaderGap).
			Padding(panelHeaderTop, panelPaddingX, panelHeaderEnd, panelPaddingX).Children(func() {
			ui.Column(c).Grow(1).MinWidth(0).Children(func() {
				ui.Text(c, title).FontSize(panelTitleSize).FontWeight(mediumWeight).SingleLine().Role(ui.RoleHeading).Level(2)
				if description != "" {
					ui.Text(c, description).TextColor(tk.TextSecondary).Margin(panelDescGap, 0, 0, 0)
				}
			})
			if trailing != nil {
				ui.Row(c).Shrink(0).Gap(panelTrailGap).Children(trailing)
			}
		})
		if body != nil {
			ui.Column(c).Padding(0, panelPaddingX, panelBodyBottom, panelPaddingX).Children(body)
		}
	})
}
