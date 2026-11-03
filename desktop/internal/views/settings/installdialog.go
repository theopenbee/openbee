package settings

import (
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/api"
	"github.com/theopenbee/openbee/desktop/internal/kit"
	"github.com/theopenbee/openbee/desktop/internal/l10n"
	"github.com/theopenbee/openbee/desktop/internal/model"
	"github.com/theopenbee/openbee/desktop/internal/theme"
	"github.com/theopenbee/openbee/desktop/locales"
)

const installGap = 20

type installState struct {
	open    bool
	spec    string
	opID    string
	err     error
	sending bool
	gen     int
}

func (s *installState) show() {
	*s = installState{open: true, gen: s.gen + 1}
}

func (s *installState) reset() {
	*s = installState{gen: s.gen + 1}
}

func (d *Dialog) installDialog(c *ui.Context, tk theme.Tokens, st *installState) {
	if !st.open && (st.spec != "" || st.opID != "" || st.err != nil || st.sending) {
		st.reset()
	}
	p := d.s.Plugins()
	var op api.PluginOperation
	if i := slices.IndexFunc(p.Ops.Data, func(o api.PluginOperation) bool { return o.ID == st.opID }); st.opID != "" && i >= 0 {
		op = p.Ops.Data[i]
	}
	running := op.ID != "" && opPending(op)
	succeeded := op.ID != "" && op.Status == opSucceeded
	failed := op.ID != "" && op.Status == opFailed
	pluginID := ""
	if succeeded {
		pluginID = p.OperationPluginID(op)
	}
	engine := d.installedEngine(pluginID)
	submit := func() {
		spec := strings.TrimSpace(st.spec)
		if spec == "" || running || succeeded || st.sending {
			return
		}
		st.err, st.opID, st.sending = nil, "", true
		gen := st.gen
		p.Install(spec, func(created api.PluginOperation, err error) {
			if st.gen != gen {
				return
			}
			st.sending = false
			if err != nil {
				st.err = failure(err)
				return
			}
			st.opID = created.ID
		})
	}
	kit.Dialog(c, tk, &st.open, kit.DialogOpts{
		Title:       locales.PluginsInstallDialogTitle(),
		Description: locales.PluginsInstallDialogDescription(),
		Footer: func() {
			dismiss := locales.CommonCancel()
			if running || succeeded {
				dismiss = locales.PluginsInstallDialogClose()
			}
			kit.Button(c, tk, dismiss, kit.ButtonOpts{Variant: kit.Secondary, Size: kit.SizeMD}).OnClick(func() { st.open = false })
			if pluginID != "" {
				open := locales.PluginsInstallDialogConfigure()
				if engine {
					open = locales.PluginsInstallDialogView()
				}
				kit.Button(c, tk, open, kit.ButtonOpts{Variant: kit.Primary, Size: kit.SizeMD}).
					OnClick(func() {
						st.open = false
						d.Open(SectionPlugins, pluginID)
					})
				return
			}
			blank := strings.TrimSpace(st.spec) == ""
			kit.Button(c, tk, locales.PluginsInstallDialogSubmit(), kit.ButtonOpts{Variant: kit.Primary, Size: kit.SizeMD,
				Loading: st.sending, Disabled: blank || running || succeeded}).OnClick(submit)
		},
	}, func() {
		ui.Column(c).Gap(installGap).Children(func() {
			label := locales.PluginsInstallDialogSpec()
			kit.Field(c, tk, label, locales.PluginsInstallDialogSpecHelp(), "", func() {
				kit.TextInput(c, tk, &st.spec, label, locales.PluginsInstallDialogPlaceholder()).
					AutoFocus().Disabled(running || succeeded).OnSubmit(submit)
			})
			switch {
			case running:
				operationProgress(c, tk, locales.PluginsInstallDialogInstalling(), op.CreatedAt)
			case succeeded:
				hint := locales.PluginsInstallDialogDoneHint()
				if engine {
					hint = locales.PluginsInstallDialogDoneHintEngine()
				}
				kit.Alert(c, tk, kit.AlertOpts{Tone: kit.ToneSuccess, Title: locales.PluginsInstallDialogDone(), Body: hint})
			}
			if failed || st.err != nil {
				text := op.Error
				if st.err != nil {
					text = l10n.ErrorText(st.err)
				}
				kit.Alert(c, tk, kit.AlertOpts{Tone: kit.ToneDanger, Title: locales.PluginsInstallDialogFailed(), Body: text, Mono: true})
			}
		})
	})
}

func (d *Dialog) installedEngine(id string) bool {
	if id == "" {
		return false
	}
	p := d.s.Plugins()
	if i := slices.IndexFunc(p.List.Data, func(s api.PluginStatus) bool { return s.ID == id }); i >= 0 {
		return model.IsEngine(p.List.Data[i].Type)
	}
	entry, listed := catalogEntry(p.Catalog.Data, id)
	return listed && model.IsEngine(entry.Category)
}
