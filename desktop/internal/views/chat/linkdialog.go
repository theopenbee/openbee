package chat

import (
	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/kit"
	"github.com/theopenbee/openbee/desktop/internal/theme"
	"github.com/theopenbee/openbee/desktop/locales"
)

const (
	linkBoxMaxHeight = 128
	linkBoxPadding   = 12
	linkActionsGap   = 8
	linkActionsTop   = 16
)

type linkState struct {
	open bool
	url  string
}

func (v *View) showLink(url string) {
	v.link = linkState{open: true, url: url}
}

func (v *View) linkDialog(c *ui.Context, tk theme.Tokens) {
	l := &v.link
	kit.Dialog(c, tk, &l.open, kit.DialogOpts{Title: locales.LinkDialogTitle(), Description: locales.LinkDialogDescription()}, func() {
		ui.Scroll(c).MaxHeight(linkBoxMaxHeight).Radius(theme.RadiusInset).Background(tk.SurfaceMuted).Children(func() {
			ui.Text(c, l.url).Font(theme.FontMono).FontSize(bodySize).TextColor(tk.Foreground).Padding(linkBoxPadding).Selectable()
		})
		ui.Row(c).Gap(linkActionsGap).Margin(linkActionsTop, 0, 0, 0).Children(func() {
			kit.CopyButton(c, tk, l.url, locales.LinkCopy()).Grow(1).Justify(ui.Center)
			safe := ui.SafeToOpen(l.url)
			open := kit.Button(c, tk, locales.LinkOpen(), kit.ButtonOpts{Variant: kit.Primary, Icon: "external-link", Disabled: !safe}).Grow(1)
			if open.Clicked() && safe {
				c.OpenURL(l.url)
				l.open = false
			}
		})
	})
}
