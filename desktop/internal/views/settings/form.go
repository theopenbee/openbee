package settings

import (
	"encoding/json"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/api"
	"github.com/theopenbee/openbee/desktop/internal/kit"
	"github.com/theopenbee/openbee/desktop/internal/l10n"
	"github.com/theopenbee/openbee/desktop/internal/model"
	"github.com/theopenbee/openbee/desktop/internal/theme"
	"github.com/theopenbee/openbee/desktop/locales"
)

const (
	formFieldGap     = 24
	formActionsTop   = 20
	formActionsGap   = 8
	listMinLines     = 3
	listMaxLines     = 8
	checkboxOutdent  = 12
	detailSavedFlash = 2500 * time.Millisecond
	fieldBool        = "bool"
	fieldList        = "list"
	fieldSecret      = "secret"
	listJoin         = ","
)

type fieldInput struct {
	text   string
	on     bool
	reveal bool
}

type formState struct {
	fields  map[string]*fieldInput
	botName string
	invalid []string
	focus   string
}

type formKey struct{}

func newForm(detail api.PluginDetail) formState {
	values, bools := model.InitialFieldValues(detail, nil)
	fields := make(map[string]*fieldInput, len(detail.Fields))
	for _, f := range detail.Fields {
		fields[f.Key] = &fieldInput{text: values[f.Key], on: bools[f.Key]}
	}
	return formState{fields: fields, botName: detail.BotName}
}

func (fs *formState) values() (model.FieldValues, map[string]bool) {
	values := model.FieldValues{}
	bools := map[string]bool{}
	for key, in := range fs.fields {
		values[key] = in.text
		bools[key] = in.on
	}
	return values, bools
}

func formIdentity(reset int, detail api.PluginDetail) string {
	config, _ := json.Marshal(detail.Config)
	fields, _ := json.Marshal(detail.Fields)
	return strconv.Itoa(reset) + ":" + string(config) + ":" + string(fields) + ":" + detail.BotName
}

func (d *Dialog) settingsForm(c *ui.Context, tk theme.Tokens, st *detailState, detail api.PluginDetail, name string) {
	form := ui.Column(c).Key(formIdentity(st.reset, detail)).MinWidth(0).Gap(formFieldGap)
	fs := ui.Local(form, formKey{}, func() formState { return newForm(detail) })
	submit := func() { d.saveSettings(st, fs, detail, name) }
	form.Children(func() {
		for _, f := range detail.Fields {
			if in := fs.fields[f.Key]; in != nil {
				settingsField(c, tk, fs, in, f, detail.Config, submit)
			}
		}
		label := locales.PluginsDetailBotName()
		kit.Field(c, tk, label, locales.PluginsDetailBotNameHelp(), "", func() {
			kit.TextInput(c, tk, &fs.botName, label, "").OnSubmit(submit)
		})
		ui.Row(c).MinWidth(0).Wrap().AlignItems(ui.Center).Justify(ui.End).Gap(formActionsGap).
			Padding(formActionsTop, 0, 0, 0).BorderWidth(cardBorder, 0, 0, 0).BorderColor(tk.BorderSubtle).Children(func() {
			kit.Button(c, tk, locales.PluginsDetailReset(), kit.ButtonOpts{Variant: kit.Ghost, Size: kit.SizeMD, Disabled: st.saving}).
				OnClick(func() { st.reset++ })
			saveButton(c, tk, st).OnClick(submit)
		})
	})
}

func saveButton(c *ui.Context, tk theme.Tokens, st *detailState) ui.Element {
	shown := false
	if st.saved {
		if st.savedAt.IsZero() {
			st.savedAt = c.Now()
		}
		if left := detailSavedFlash - c.Now().Sub(st.savedAt); left > 0 {
			shown = true
			c.After(left)
		} else {
			st.saved = false
		}
	}
	opts := kit.ButtonOpts{Variant: kit.Primary, Size: kit.SizeMD, Loading: st.saving}
	text := locales.PluginsDetailSave()
	if shown {
		text, opts.Icon = locales.PluginsDetailSaved(), "check"
	}
	return kit.Button(c, tk, text, opts)
}

func (d *Dialog) saveSettings(st *detailState, fs *formState, detail api.PluginDetail, name string) {
	if st.saving {
		return
	}
	values, bools := fs.values()
	fs.invalid = model.ValidateInts(detail.Fields, values)
	if len(fs.invalid) > 0 {
		fs.focus = fs.invalid[0]
		return
	}
	update, changed := model.BuildSettingsPayload(detail, values, bools, fs.botName)
	if !changed {
		d.s.Toast(kit.Toast{Tone: kit.ToneInfo, Title: locales.PluginsDetailNoChanges()})
		return
	}
	st.saving = true
	enabled := detail.Enabled
	d.s.Plugins().SaveSettings(detail.ID, update, func(err error) {
		st.saving = false
		if err != nil {
			d.toastError(err)
			return
		}
		st.saved, st.savedAt = true, time.Time{}
		title := locales.PluginsToastSavedEnableHint()
		if enabled {
			title = locales.PluginsToastSavedRestarting(name)
		}
		d.s.Toast(kit.Toast{Tone: kit.ToneSuccess, Title: title})
	})
}

func settingsField(c *ui.Context, tk theme.Tokens, fs *formState, in *fieldInput, f api.PluginConfigField, config map[string]any, submit func()) {
	lang := i18n.Locale()
	label := l10n.Localized(f.Label, lang)
	if label == "" {
		label = f.Key
	}
	title := label
	if f.Required {
		title = locales.PluginsDetailRequiredLabel(label)
	}
	help := l10n.Localized(f.Help, lang)
	hint := ""
	if text := defaultText(f.Default); text != "" {
		hint = locales.PluginsDetailDefaultValue(text)
	}
	box := ui.Column(c).Key(f.Key).MinWidth(0)
	if f.Type == fieldBool {
		box.Margin(0, 0, 0, -checkboxOutdent)
	}
	box.Children(func() {
		switch f.Type {
		case fieldBool:
			kit.CheckboxRow(c, tk, &in.on, title, help)
		case fieldList:
			if help == "" {
				help = locales.PluginsDetailListHelp()
			}
			kit.Field(c, tk, title, help, "", func() {
				kit.TextArea(c, tk, &in.text, title, locales.PluginsDetailListHelp(), listMinLines, listMaxLines)
			})
		case fieldSecret:
			placeholder := hint
			if mask, ok := config[f.Key].(string); ok && mask != "" {
				placeholder = locales.PluginsDetailSecretSaved(mask)
			}
			kit.Field(c, tk, title, help, "", func() {
				kit.PasswordInput(c, tk, &in.text, &in.reveal, title, placeholder).OnSubmit(submit)
			})
		default:
			errText := ""
			if slices.Contains(fs.invalid, f.Key) {
				errText = locales.PluginsDetailIntInvalid(label)
			}
			kit.Field(c, tk, title, help, errText, func() {
				input := kit.TextInput(c, tk, &in.text, title, hint).OnSubmit(submit)
				if input.Changed() && errText != "" {
					fs.invalid = slices.DeleteFunc(fs.invalid, func(key string) bool { return key == f.Key })
				}
				if fs.focus == f.Key {
					fs.focus = ""
					input.Focus()
				}
			})
		}
	})
}

func defaultText(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case []any:
		parts := make([]string, len(x))
		for i, item := range x {
			parts[i] = defaultText(item)
		}
		return strings.Join(parts, listJoin)
	}
	return model.ScalarString(v)
}
