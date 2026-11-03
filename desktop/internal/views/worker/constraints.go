package worker

import (
	"strings"
	"unicode/utf16"

	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/api"
	"github.com/theopenbee/openbee/desktop/internal/icons"
	"github.com/theopenbee/openbee/desktop/internal/kit"
	"github.com/theopenbee/openbee/desktop/internal/theme"
	"github.com/theopenbee/openbee/desktop/locales"
)

const (
	metaSeparator   = " · "
	editorRows      = 14
	editorPad       = 24
	editorGap       = 16
	barGap          = 12
	barPadY         = 16
	barPadX         = 24
	bodyPadY        = 20
	bodyPadX        = 24
	bodyLineHeight  = 2
	templatesGap    = 12
	templateMin     = 64
	templateGap     = 16
	templatePadY    = 16
	templatePadX    = 24
	templateGlyph   = 20
	templateCopyGap = 2
	chevronGlyph    = 16
)

type constraintsState struct {
	editing bool
	draft   string
	saving  bool
	err     error
}

type constraintsKey struct{}

type template struct {
	icon              string
	title, desc, body func() string
}

var templates = []template{
	{icon: "message-square-text", title: locales.WorkerDetailConstraintsPanelTemplatesToneTitle,
		desc: locales.WorkerDetailConstraintsPanelTemplatesToneDesc, body: locales.WorkerDetailConstraintsPanelTemplatesToneBody},
	{icon: "shield", title: locales.WorkerDetailConstraintsPanelTemplatesScopeTitle,
		desc: locales.WorkerDetailConstraintsPanelTemplatesScopeDesc, body: locales.WorkerDetailConstraintsPanelTemplatesScopeBody},
	{icon: "clipboard-check", title: locales.WorkerDetailConstraintsPanelTemplatesDeliveryTitle,
		desc: locales.WorkerDetailConstraintsPanelTemplatesDeliveryDesc, body: locales.WorkerDetailConstraintsPanelTemplatesDeliveryBody},
}

func charCount(s string) int {
	n := 0
	for _, r := range s {
		n += utf16.RuneLen(r)
	}
	return n
}

func (cs *constraintsState) start(draft string) {
	*cs = constraintsState{editing: true, draft: draft}
}

func (cs *constraintsState) cancel() {
	*cs = constraintsState{}
}

func (p *Panel) constraints(c *ui.Context, tk theme.Tokens, w api.Worker) {
	box := ui.Column(c).MinWidth(0).Gap(sectionGap)
	cs := ui.Local(box, constraintsKey{}, func() constraintsState { return constraintsState{} })
	box.Children(func() {
		text := w.Constraints
		switch {
		case cs.editing:
			p.constraintsEditor(c, tk, cs, w.ID)
		case strings.TrimSpace(text) != "":
			constraintsCard(c, tk, cs, text)
		default:
			constraintsEmpty(c, tk, cs, w.Name)
		}
	})
}

func constraintsCard(c *ui.Context, tk theme.Tokens, cs *constraintsState, text string) {
	card(c, tk).Children(func() {
		ui.Row(c).Wrap().AlignItems(ui.Center).Justify(ui.SpaceBetween).Gap(barGap).Padding(barPadY, barPadX).
			BorderWidth(0, 0, 1, 0).BorderColor(tk.BorderSubtle).Children(func() {
			ui.Text(c, locales.WorkerDetailConstraintsPanelCharCount(charCount(text))).TextColor(tk.TextSecondary).FontFeatures("tnum")
			ui.Row(c).MarginStart(ui.Auto).AlignItems(ui.Center).Gap(buttonsGap).Children(func() {
				kit.CopyIconButton(c, tk, text, locales.CommonCopy())
				kit.Button(c, tk, locales.WorkerDetailEditConstraints(), kit.ButtonOpts{Variant: kit.Secondary, Size: kit.SizeSM, Icon: "pencil"}).
					OnClick(func() { cs.start(text) })
			})
		})
		ui.Text(c, text).LineHeight(bodyLineHeight).Padding(bodyPadY, bodyPadX)
	})
}

func constraintsEmpty(c *ui.Context, tk theme.Tokens, cs *constraintsState, name string) {
	card(c, tk).Children(func() {
		kit.EmptyState(c, tk, "scroll-text", locales.WorkerDetailConstraintsPanelEmptyTitle(name),
			locales.WorkerDetailConstraintsPanelEmptyDescription(), func() {
				kit.Button(c, tk, locales.WorkerDetailConstraintsPanelAdd(), kit.ButtonOpts{Size: kit.SizeMD, Icon: "plus"}).
					OnClick(func() { cs.start("") })
			})
	})
	ui.Column(c).MinWidth(0).Gap(templatesGap).Children(func() {
		heading(c, locales.WorkerDetailConstraintsPanelTemplatesLabel(), 2)
		card(c, tk).Children(func() {
			for i, tpl := range templates {
				templateRow(c, tk, tpl, i > 0, cs)
			}
		})
	})
}

func templateRow(c *ui.Context, tk theme.Tokens, tpl template, divided bool, cs *constraintsState) {
	title := tpl.title()
	b := ui.ButtonBase(c).Label(title).MinHeight(templateMin).Gap(templateGap).Padding(templatePadY, templatePadX).AlignItems(ui.Center)
	if divided {
		b.BorderWidth(1, 0, 0, 0).BorderColor(tk.BorderSubtle)
	}
	bg := tk.SurfaceMuted.Alpha(0)
	if b.Hovered() || b.Pressed() {
		bg = tk.SurfaceMuted
	}
	b.Background(b.AnimateColor("bg", bg, fast)).OnClick(func() { cs.start(tpl.body()) }).Children(func() {
		ui.Icon(c, icons.Get(tpl.icon)).Size(templateGlyph, templateGlyph).Shrink(0).TextColor(tk.TextSecondary)
		ui.Column(c).Grow(1).MinWidth(0).Gap(templateCopyGap).Children(func() {
			ui.Text(c, title).FontWeight(mediumWeight).TextColor(tk.Foreground)
			ui.Text(c, tpl.desc()).TextColor(tk.TextSecondary)
		})
		ui.Icon(c, icons.Get("chevron-right")).Size(chevronGlyph, chevronGlyph).Shrink(0).TextColor(tk.TextMuted)
	})
}

func (p *Panel) constraintsEditor(c *ui.Context, tk theme.Tokens, cs *constraintsState, id string) {
	save := func() { p.saveConstraints(cs, id) }
	card(c, tk).Padding(editorPad).Gap(editorGap).Children(func() {
		hint := locales.WorkerDetailConstraintsPanelCharCount(charCount(cs.draft)) + metaSeparator + locales.WorkerDetailConstraintsPanelShortcuts()
		kit.Field(c, tk, locales.WorkerDetailEditConstraints(), hint, "", func() {
			kit.TextArea(c, tk, &cs.draft, locales.WorkerDetailEditConstraints(), locales.WorkersFormConstraintsPlaceholder(), editorRows, editorRows).
				AutoFocus().Disabled(cs.saving).OnTextKey(func(k ui.TextKey) bool {
				switch {
				case k.Key == ui.KeyEscape:
					cs.cancel()
				case k.Key == ui.KeyEnter && k.Mods&(ui.Super|ui.Ctrl) != 0:
					cs.draft = k.Text()
					save()
				default:
					return false
				}
				return true
			})
		})
		errorAlert(c, tk, locales.WorkerDetailConstraintsPanelSaveError(), cs.err)
		ui.Row(c).Justify(ui.End).Gap(buttonsGap).Children(func() {
			kit.Button(c, tk, locales.CommonCancel(), kit.ButtonOpts{Variant: kit.Secondary, Size: kit.SizeMD, Disabled: cs.saving}).
				OnClick(cs.cancel)
			kit.Button(c, tk, locales.WorkerDetailConstraintsPanelSave(), kit.ButtonOpts{Size: kit.SizeMD, Loading: cs.saving}).OnClick(save)
		})
	})
}

func (p *Panel) saveConstraints(cs *constraintsState, id string) {
	if cs.saving {
		return
	}
	draft := cs.draft
	cs.saving, cs.err = true, nil
	p.s.UpdateWorker(id, api.UpdateWorkerRequest{Constraints: &draft}, func(_ api.Worker, err error) {
		cs.saving = false
		if err != nil {
			cs.err = failure(err)
			return
		}
		cs.editing = false
	})
}
