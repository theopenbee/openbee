package envpanel

import (
	"errors"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/api"
	"github.com/theopenbee/openbee/desktop/internal/kit"
	"github.com/theopenbee/openbee/desktop/internal/l10n"
	"github.com/theopenbee/openbee/desktop/internal/model"
	"github.com/theopenbee/openbee/desktop/internal/theme"
	"github.com/theopenbee/openbee/desktop/locales"
)

const (
	dialogGap   = 16
	valueGap    = 8
	footerGap   = 8
	footerBleed = 16
	valueRows   = 4
	stackBelow  = 640
	reservedKey = "env_reserved_key"
)

const (
	pendingNone = iota
	pendingClose
	pendingAnother
)

type valueState struct {
	text      string
	shown     bool
	multiline bool
}

type addState struct {
	open     bool
	key      string
	keyErr   string
	value    valueState
	formErr  error
	added    string
	pending  int
	focusKey bool
}

type editState struct {
	open   bool
	target api.EnvConfig
	value  valueState
	err    error
	saving bool
}

type deleteState struct {
	open     bool
	target   api.EnvConfig
	err      error
	deleting bool
}

func (a *addState) show() {
	*a = addState{open: true}
}

func (a *addState) clearFields() {
	a.key, a.keyErr, a.formErr = "", "", nil
	a.value.text = ""
}

func (a *addState) ready(existing []api.EnvConfig) bool {
	return a.key != "" && a.value.text != "" && !model.EnvKeyExists(a.key, existing) && a.pending == pendingNone
}

func (e *editState) show(env api.EnvConfig) {
	*e = editState{open: true, target: env}
}

func (d *deleteState) show(env api.EnvConfig) {
	*d = deleteState{open: true, target: env}
}

func narrowWindow(c *ui.Context) bool {
	w, _ := c.Size()
	return w < stackBelow
}

func (p *Panel) addDialog(c *ui.Context, tk theme.Tokens, a *addState, existing []api.EnvConfig) {
	duplicate := model.EnvKeyExists(a.key, existing)
	submit := func() { p.submitAdd(a, existing, false) }
	another := func() { p.submitAdd(a, existing, true) }
	ready := a.ready(existing)
	narrow := narrowWindow(c)
	kit.Dialog(c, tk, &a.open, kit.DialogOpts{
		Title:       locales.EnvConfigAddTitle(),
		Description: locales.EnvConfigAddDescription(),
		Locked:      a.pending != pendingNone,
		Footer: func() {
			footer(c, narrow, true,
				func() ui.Element {
					return kit.Button(c, tk, locales.CommonCancel(), kit.ButtonOpts{Variant: kit.Ghost, Size: kit.SizeMD}).
						OnClick(func() { a.open = false })
				},
				func() ui.Element {
					return kit.Button(c, tk, locales.EnvConfigSaveAndAddAnother(), kit.ButtonOpts{
						Variant: kit.Secondary, Size: kit.SizeMD, Loading: a.pending == pendingAnother,
						Disabled: !ready && a.pending != pendingAnother}).OnClick(another)
				},
				func() ui.Element {
					return kit.Button(c, tk, locales.EnvConfigAdd(), kit.ButtonOpts{
						Size: kit.SizeMD, Loading: a.pending == pendingClose,
						Disabled: !ready && a.pending != pendingClose}).OnClick(submit)
				})
		},
	}, func() {
		ui.Column(c).Gap(dialogGap).Children(func() {
			if a.added != "" {
				kit.Alert(c, tk, kit.AlertOpts{Tone: kit.ToneSuccess, Title: locales.EnvConfigAdded(a.added), Body: locales.EnvConfigAddedDescription()})
			}
			errorAlert(c, tk, locales.EnvConfigSaveError(), a.formErr)
			p.keyField(c, tk, a, duplicate, submit)
			valueField(c, tk, &a.value, locales.EnvConfigValue(), false, submit, func() { a.formErr = nil })
		})
	})
}

func (p *Panel) keyField(c *ui.Context, tk theme.Tokens, a *addState, duplicate bool, submit func()) {
	help, problem := locales.EnvConfigKeyHint(), a.keyErr
	if duplicate {
		problem = locales.EnvConfigKeyExists(a.key)
	}
	if problem != "" {
		help = ""
	}
	kit.Field(c, tk, locales.EnvConfigKey(), help, problem, func() {
		in := kit.TextInput(c, tk, &a.key, locales.EnvConfigKey(), locales.EnvConfigKeyPlaceholder()).AutoFocus().OnSubmit(submit)
		if in.Changed() {
			normalizeKey(in, &a.key)
			a.keyErr, a.formErr = "", nil
		}
		if a.focusKey {
			a.focusKey = false
			in.Focus()
		}
	})
}

func normalizeKey(in ui.Element, key *string) {
	normalized := model.NormalizeEnvKey(*key)
	if normalized == *key {
		return
	}
	_, caret := in.TextSelection()
	runes := []rune(*key)
	head := model.NormalizeEnvKey(string(runes[:min(caret, len(runes))]))
	*key = normalized
	at := utf8.RuneCountInString(head)
	in.SetTextSelection(at, at)
}

func (p *Panel) submitAdd(a *addState, existing []api.EnvConfig, another bool) {
	if !a.ready(existing) {
		return
	}
	key := a.key
	a.keyErr, a.formErr = "", nil
	a.pending = pendingClose
	if another {
		a.pending = pendingAnother
	}
	req := api.CreateEnvRequest{Scope: p.scope, ScopeID: p.scopeID, Key: key, Value: a.value.text}
	p.s.CreateEnv(req, func(err error) {
		a.pending = pendingNone
		var apiErr *api.Error
		switch {
		case err == nil && another:
			a.clearFields()
			a.added, a.focusKey = key, true
		case err == nil || failure(err) == nil:
			a.open = false
		case errors.As(err, &apiErr) && apiErr.Code == reservedKey:
			a.keyErr = l10n.ErrorText(err)
		default:
			a.formErr = err
		}
	})
}

func (p *Panel) editDialog(c *ui.Context, tk theme.Tokens, e *editState) {
	save := func() { p.saveEdit(e) }
	narrow := narrowWindow(c)
	kit.Dialog(c, tk, &e.open, kit.DialogOpts{
		Title:       locales.EnvConfigEditTitle(e.target.Key),
		Description: locales.EnvConfigEditDescription(),
		Locked:      e.saving,
		Footer: func() {
			footer(c, narrow, false,
				func() ui.Element {
					return kit.Button(c, tk, locales.CommonCancel(), kit.ButtonOpts{Variant: kit.Secondary, Size: kit.SizeMD, Disabled: e.saving}).
						OnClick(func() { e.open = false })
				},
				func() ui.Element {
					return kit.Button(c, tk, locales.EnvConfigSaveValue(), kit.ButtonOpts{
						Size: kit.SizeMD, Loading: e.saving, Disabled: e.value.text == "" && !e.saving}).OnClick(save)
				})
		},
	}, func() {
		ui.Column(c).Gap(dialogGap).Children(func() {
			errorAlert(c, tk, locales.EnvConfigSaveError(), e.err)
			valueField(c, tk, &e.value, locales.EnvConfigNewValue(), true, save, func() { e.err = nil })
		})
	})
}

func (p *Panel) saveEdit(e *editState) {
	if e.saving || e.value.text == "" {
		return
	}
	e.saving, e.err = true, nil
	p.s.UpdateEnv(p.scope, p.scopeID, e.target.ID, e.value.text, func(err error) {
		e.saving = false
		if err = failure(err); err != nil {
			e.err = err
			return
		}
		e.open = false
	})
}

func (p *Panel) deleteDialog(c *ui.Context, tk theme.Tokens, d *deleteState) {
	narrow := narrowWindow(c)
	var body func()
	if d.err != nil {
		body = func() { errorAlert(c, tk, locales.EnvConfigDeleteError(), d.err) }
	}
	kit.Dialog(c, tk, &d.open, kit.DialogOpts{
		Title:       locales.EnvConfigDeleteTitle(d.target.Key),
		Description: locales.EnvConfigDeleteConfirm(),
		Locked:      d.deleting,
		Footer: func() {
			footer(c, narrow, false,
				func() ui.Element {
					return kit.Button(c, tk, locales.CommonCancel(), kit.ButtonOpts{Variant: kit.Secondary, Size: kit.SizeMD, Disabled: d.deleting}).
						OnClick(func() { d.open = false })
				},
				func() ui.Element {
					return kit.Button(c, tk, locales.EnvConfigDeleteAction(), kit.ButtonOpts{Variant: kit.Danger, Size: kit.SizeMD, Loading: d.deleting}).
						OnClick(func() { p.confirmDelete(d) })
				})
		},
	}, body)
}

func (p *Panel) confirmDelete(d *deleteState) {
	if d.deleting {
		return
	}
	d.deleting, d.err = true, nil
	p.s.DeleteEnv(p.scope, p.scopeID, d.target.ID, func(err error) {
		d.deleting = false
		if err = failure(err); err != nil {
			d.err = err
			return
		}
		d.open = false
	})
}

func footer(c *ui.Context, narrow, spreadFirst bool, buttons ...func() ui.Element) {
	if narrow {
		ui.Column(c).Grow(1).AlignItems(ui.Stretch).Gap(footerGap).Children(func() {
			for i := len(buttons) - 1; i >= 0; i-- {
				buttons[i]().AlignSelf(ui.Stretch)
			}
		})
		return
	}
	ui.Row(c).Grow(1).Justify(ui.End).Gap(footerGap).Children(func() {
		for i, build := range buttons {
			button := build()
			if i == 0 && spreadFirst {
				button.MarginStart(-footerBleed).MarginEnd(ui.Auto)
			}
		}
	})
}

func valueField(c *ui.Context, tk theme.Tokens, v *valueState, label string, autoFocus bool, submit, changed func()) {
	ui.Column(c).Gap(valueGap).Children(func() {
		kit.Field(c, tk, label, valueNote(v.text), "", func() {
			var in ui.Element
			if v.multiline {
				in = kit.TextArea(c, tk, &v.text, label, locales.EnvConfigValuePlaceholder(), valueRows, valueRows)
			} else {
				in = kit.PasswordInputNamed(c, tk, &v.text, &v.shown, label, "", locales.EnvConfigShowValue(), locales.EnvConfigHideValue()).
					OnSubmit(submit)
			}
			in.OnChange(changed)
			if autoFocus {
				in.AutoFocus()
			}
		})
		kit.SwitchRow(c, tk, &v.multiline, locales.EnvConfigMultiline(), "", false)
	})
}

func valueNote(value string) string {
	switch {
	case value == "":
		return ""
	case model.HasEdgeSpace(value):
		return locales.EnvConfigValueLengthSpace(utf16Length(value))
	}
	return locales.EnvConfigValueLength(utf16Length(value))
}

func utf16Length(s string) int {
	n := 0
	for _, r := range s {
		n += utf16.RuneLen(r)
	}
	return n
}
