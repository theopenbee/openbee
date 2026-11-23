package worker

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
	return editState{form: model.NewEditForm(w)}
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
				p.modelField(c, tk, installed, followed, w.ID, es.form.EngineModels, submit)
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

func (p *Panel) modelField(c *ui.Context, tk theme.Tokens, installed []api.EngineInfo, engineID, workerID string, all map[string]string, submit func()) {
	engine := model.FindEngine(installed, engineID)
	listed := slices.Contains(model.EngineIDs(installed), engineID)
	value := all[engineID]
	help, placeholder := locales.WorkersFormModelHelper(l10n.EngineName(engine)), locales.WorkersFormModelPlaceholder()
	if fallback := p.s.DefaultModels()[engineID]; fallback != "" {
		help, placeholder = locales.WorkersFormModelHelperDefault(), locales.WorkersFormModelDefault(fallback)
	}
	kit.Field(c, tk, locales.WorkersFormModel(), "", "", func() {
		box := ui.Column(c).Key(engineID).MinWidth(0)
		opts := kit.ComboOpts{Help: help, NoMatches: locales.WorkersFormModelNoMatches()}
		if listed {
			opts = p.modelOptions(box, engineID, workerID, opts)
		}
		box.Children(func() {
			kit.Combobox(c, tk, &value, locales.WorkersFormModel(), placeholder, opts).
				OnChange(func() { all[engineID] = value }).OnSubmit(submit)
		})
	})
}

type modelRetryKey struct{}

func (p *Panel) modelOptions(box ui.Element, engineID, workerID string, opts kit.ComboOpts) kit.ComboOpts {
	list := p.s.EngineModels(engineID, workerID)
	ui.Local(box, modelRetryKey{}, func() bool {
		if list.Err != nil && !list.Loading {
			list.Invalidate()
		}
		return true
	})
	list.Ensure()
	switch {
	case list.Loaded:
		opts.Options = make([]kit.ComboOption, len(list.Data))
		for i, m := range list.Data {
			opts.Options[i] = kit.ComboOption{Value: m.ID, Detail: m.Name}
		}
	case list.Err != nil && !list.Loading:
		opts.Failed = locales.WorkersFormModelListError()
	default:
		opts.Loading = locales.WorkersFormModelLoading()
	}
	return opts
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
