package kit

import (
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/theme"
)

const (
	keyCapHeight   = 20
	keyCapMinWidth = 20
	keyCapPadX     = 5
	keyCapSize     = 11
	keyCapGap      = 3
	keyCapInset    = 4
)

func Keys(c *ui.Context, tk theme.Tokens, caps []string) ui.Element {
	return ui.Row(c).Shrink(0).Gap(keyCapGap).AlignItems(ui.Center).Label(strings.Join(caps, " ")).Children(func() {
		for _, key := range caps {
			ui.Box(c).Height(keyCapHeight).MinWidth(keyCapMinWidth).PaddingX(keyCapPadX).Center().
				Radius(theme.Concentric(theme.RadiusInset, keyCapInset)).Border(1, tk.Border).Background(tk.Surface).Children(func() {
				ui.Text(c, key).FontSize(keyCapSize).FontWeight(mediumWeight).TextColor(tk.TextMuted).LineHeight(1).SingleLine()
			})
		}
	})
}
