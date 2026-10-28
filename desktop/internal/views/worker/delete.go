package worker

import (
	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/api"
	"github.com/theopenbee/openbee/desktop/internal/kit"
	"github.com/theopenbee/openbee/desktop/internal/theme"
	"github.com/theopenbee/openbee/desktop/internal/views/route"
	"github.com/theopenbee/openbee/desktop/locales"
)

const deleteGap = 16

type deleteState struct {
	open     bool
	target   api.Worker
	typed    string
	workDir  bool
	err      error
	deleting bool
}

func (d *deleteState) show(w api.Worker) {
	*d = deleteState{open: true, target: w}
}

func (d *deleteState) confirmed() bool {
	return d.typed == d.target.Name
}

func (p *Panel) deleteDialog(c *ui.Context, tk theme.Tokens, d *deleteState) {
	confirm := func() { p.confirmDelete(d) }
	kit.Dialog(c, tk, &d.open, kit.DialogOpts{
		Title:       locales.WorkersDeleteDialogTitle(d.target.Name),
		Description: locales.WorkersDeleteDialogDescription(),
		Locked:      d.deleting,
		Footer: func() {
			kit.Button(c, tk, locales.CommonCancel(), kit.ButtonOpts{Variant: kit.Secondary, Size: kit.SizeMD, Disabled: d.deleting}).
				OnClick(func() { d.open = false })
			kit.Button(c, tk, locales.WorkersDeleteDialogConfirm(), kit.ButtonOpts{
				Variant: kit.Danger, Size: kit.SizeMD, Loading: d.deleting, Disabled: !d.confirmed()}).OnClick(confirm)
		},
	}, func() {
		ui.Column(c).Gap(deleteGap).Children(func() {
			kit.Alert(c, tk, kit.AlertOpts{Tone: kit.ToneWarning, Title: locales.WorkersDeleteDialogChatWarning()})
			ui.Column(c).Gap(deleteGap).Disabled(d.deleting).Children(func() {
				label := locales.WorkersDeleteDialogConfirmNameLabel(d.target.Name)
				kit.Field(c, tk, label, "", "", func() {
					kit.TextInput(c, tk, &d.typed, label, locales.WorkersDeleteDialogConfirmNamePlaceholder()).
						AutoFocus().OnSubmit(confirm)
				})
				hint := ""
				if d.target.WorkDir != "" {
					hint = locales.WorkersDeleteDialogDeleteWorkDirHint(d.target.WorkDir)
				}
				kit.CheckboxRow(c, tk, &d.workDir, locales.WorkersDeleteDialogDeleteWorkDir(), hint)
			})
			errorAlert(c, tk, locales.WorkersDeleteDialogError(), d.err)
		})
	})
}

func (p *Panel) confirmDelete(d *deleteState) {
	if d.deleting || !d.confirmed() {
		return
	}
	d.deleting, d.err = true, nil
	p.s.DeleteWorker(d.target.ID, d.workDir, func(err error) {
		d.deleting = false
		if err != nil {
			d.err = failure(err)
			return
		}
		d.open = false
		p.nav.Replace(route.Chat(api.BeeConversation))
	})
}
