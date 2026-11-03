package kit

import (
	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/icons"
	"github.com/theopenbee/openbee/desktop/internal/theme"
)

const (
	emptyPaddingY  = 48
	emptyPaddingX  = 20
	emptyIconBox   = 48
	emptyGlyph     = 24
	emptyTitleSize = 16
	emptyTitleGap  = 20
	emptyBodyGap   = 8
	emptyBodyWidth = 288
	emptyActionGap = 20
	emptyActionsX  = 12
)

func EmptyState(c *ui.Context, tk theme.Tokens, icon, title, body string, action func()) ui.Element {
	return ui.Column(c).AlignItems(ui.Center).Padding(emptyPaddingY, emptyPaddingX).Children(func() {
		if icon != "" {
			ui.Box(c).Size(emptyIconBox, emptyIconBox).Center().Radius(theme.RadiusPanel).
				Background(tk.SurfaceMuted).Border(1, tk.Border).TextColor(tk.TextSecondary).Children(func() {
				ui.Icon(c, icons.Get(icon)).Size(emptyGlyph, emptyGlyph)
			})
		}
		ui.Text(c, title).FontSize(emptyTitleSize).FontWeight(mediumWeight).TextAlign(ui.Center).Margin(emptyTitleGap, 0, 0, 0)
		if body != "" {
			ui.Text(c, body).TextColor(tk.TextSecondary).TextAlign(ui.Center).MaxWidth(emptyBodyWidth).Margin(emptyBodyGap, 0, 0, 0)
		}
		if action != nil {
			ui.Row(c).Gap(emptyActionsX).Wrap().Justify(ui.Center).Margin(emptyActionGap, 0, 0, 0).Children(action)
		}
	})
}
