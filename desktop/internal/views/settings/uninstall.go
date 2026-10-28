package settings

import (
	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/api"
	"github.com/theopenbee/openbee/desktop/internal/kit"
	"github.com/theopenbee/openbee/desktop/internal/l10n"
	"github.com/theopenbee/openbee/desktop/internal/theme"
	"github.com/theopenbee/openbee/desktop/locales"
)

type uninstallState struct {
	open     bool
	id, name string
	err      error
	pending  bool
}

func (u *uninstallState) show(id, name string) {
	*u = uninstallState{open: true, id: id, name: name}
}

func (d *Dialog) uninstallDialog(c *ui.Context, tk theme.Tokens, u *uninstallState) {
	confirm := func() {
		if u.pending {
			return
		}
		u.pending, u.err = true, nil
		id := u.id
		d.s.Plugins().Uninstall(id, func(_ api.PluginOperation, err error) {
			u.pending = false
			if err != nil {
				u.err = failure(err)
				return
			}
			if u.id == id {
				u.open = false
			}
		})
	}
	var body func()
	if u.err != nil {
		body = func() {
			kit.Alert(c, tk, kit.AlertOpts{Tone: kit.ToneDanger, Title: locales.PluginsUninstallDialogError(), Body: l10n.ErrorText(u.err)})
		}
	}
	kit.Dialog(c, tk, &u.open, kit.DialogOpts{
		Title:       locales.PluginsUninstallDialogTitle(u.name),
		Description: locales.PluginsUninstallDialogDescription(),
		Footer: func() {
			kit.Button(c, tk, locales.CommonCancel(), kit.ButtonOpts{Variant: kit.Secondary, Size: kit.SizeMD}).
				OnClick(func() { u.open = false })
			kit.Button(c, tk, locales.PluginsUninstallDialogConfirm(), kit.ButtonOpts{Variant: kit.Danger, Size: kit.SizeMD, Loading: u.pending}).
				OnClick(confirm)
		},
	}, body)
}
