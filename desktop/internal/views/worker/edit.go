package worker

import (
	"maps"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/api"
	"github.com/theopenbee/openbee/desktop/internal/kit"
	"github.com/theopenbee/openbee/desktop/internal/l10n"
	"github.com/theopenbee/openbee/desktop/internal/model"
	"github.com/theopenbee/openbee/desktop/internal/theme"
	"github.com/theopenbee/openbee/desktop/locales"
)

const (
	formGap              = 32
	fieldsGap            = 20
	descriptionRows      = 3
	descriptionMinHeight = 108
	footerPadTop         = 24
)

type editState struct {
	form   model.EditForm
	saving bool
	err    error
	name   nameState
}

type editKey struct{}

func newEdit(w api.Worker) editState {
	args := maps.Clone(w.EngineArgs)
	if args == nil {
		args = map[string]string{}
	}
	return editState{form: model.EditForm{Name: w.Name, Description: w.Description, WorkDir: w.WorkDir, Engine: w.Engine, EngineArgs: args}}
}

func (es *editState) complete() bool {
	return strings.TrimSpace(es.form.Name) != "" && strings.TrimSpace(es.form.WorkDir) != ""
}

func (p *Panel) editForm(c *ui.Context, tk theme.Tokens, st *panelState, w api.Worker) {
	configs := p.s.SystemConfigs
	configs.Ensure()
	installed := p.s.EnabledEngines()
	box := ui.Column(c).Key(editKey{}).MinWidth(0).Gap(formGap)
	es := ui.Local(box, editKey{}, func() editState { return newEdit(w) })
	submit := func() { p.saveInfo(st, es, w) }
	followed := model.FollowedEngine(es.form.Engine, configs.Data["default_engine"], model.EngineIDs(installed))
	placeholder := locales.WorkersFormEngineSystemDefault()
	if followed != "" {
		placeholder = locales.WorkersFormEngineSystemDefaultNamed(l10n.EngineName(model.FindEngine(installed, followed)))
	}
	box.Children(func() {
		ui.Column(c).MinWidth(0).Gap(fieldsGap).Disabled(es.saving).Children(func() {
			heading(c, locales.WorkerDetailEditInfoTitle(), 4)
			p.nameField(c, tk, &es.form.Name, &es.name, func(err error) { es.err = err }, submit)
			kit.Field(c, tk, locales.WorkersFormDescription(), locales.WorkersFormDescriptionHelper(), "", func() {
				kit.TextArea(c, tk, &es.form.Description, locales.WorkersFormDescription(), locales.WorkersFormDescriptionPlaceholder(),
					descriptionRows, descriptionRows).MinHeight(descriptionMinHeight)
			})
			kit.Field(c, tk, locales.WorkersFormWorkDir(), locales.WorkersFormWorkDirEditHelper(), "", func() {
				kit.TextInput(c, tk, &es.form.WorkDir, locales.WorkersFormWorkDir(), locales.WorkersFormWorkDirPlaceholder()).OnSubmit(submit)
			})
			options := kit.EngineOptions(installed, w.Engine)
			kit.Field(c, tk, locales.WorkersFormEngine(), engineHelper(options), "", func() {
				kit.Select(c, tk, &es.form.Engine, locales.WorkersFormEngine(), options, placeholder).Disabled(len(options) == 0)
			})
			if followed != "" {
				engineArgsField(c, tk, model.FindEngine(installed, followed), es.form.EngineArgs, submit)
			}
		})
		errorAlert(c, tk, locales.WorkersFormSaveError(), es.err)
		formFooter(c, tk, es.saving, func() { st.editing = false },
			locales.WorkerDetailSaveInfo(), kit.ButtonOpts{Loading: es.saving, Disabled: !es.complete()}, submit)
	})
}

func formFooter(c *ui.Context, tk theme.Tokens, busy bool, cancel func(), label string, o kit.ButtonOpts, submit func()) {
	ui.Row(c).Justify(ui.End).Gap(buttonsGap).Padding(footerPadTop, 0, 0, 0).BorderWidth(1, 0, 0, 0).BorderColor(tk.BorderSubtle).Children(func() {
		kit.Button(c, tk, locales.CommonCancel(), kit.ButtonOpts{Variant: kit.Secondary, Size: kit.SizeMD, Disabled: busy}).OnClick(cancel)
		o.Size = kit.SizeMD
		kit.Button(c, tk, label, o).OnClick(submit)
	})
}

func engineHelper(options []kit.SelectOption) string {
	if len(options) == 0 {
		return locales.WorkersFormNoEngine()
	}
	return locales.WorkersFormEngineHelper()
}

func engineArgsField(c *ui.Context, tk theme.Tokens, engine api.EngineInfo, all map[string]string, submit func()) {
	args := all[engine.ID]
	kit.Field(c, tk, locales.WorkersFormEngineArgs(), locales.WorkersFormEngineArgsHelper(l10n.EngineName(engine)), "", func() {
		ui.Column(c).Key(engine.ID).MinWidth(0).Children(func() {
			kit.TextInput(c, tk, &args, locales.WorkersFormEngineArgs(), locales.WorkersFormEngineArgsPlaceholder()).
				OnChange(func() { all[engine.ID] = args }).OnSubmit(submit)
		})
	})
}

func (p *Panel) saveInfo(st *panelState, es *editState, w api.Worker) {
	if es.saving || !es.complete() {
		return
	}
	req, changed := model.EditUpdate(w, es.form)
	if !changed {
		st.editing = false
		return
	}
	es.saving, es.err = true, nil
	p.s.UpdateWorker(w.ID, req, func(_ api.Worker, err error) {
		es.saving = false
		if err != nil {
			es.err = failure(err)
			return
		}
		st.editing = false
	})
}
