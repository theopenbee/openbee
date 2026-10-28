package worker

import (
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/api"
	"github.com/theopenbee/openbee/desktop/internal/icons"
	"github.com/theopenbee/openbee/desktop/internal/kit"
	"github.com/theopenbee/openbee/desktop/internal/l10n"
	"github.com/theopenbee/openbee/desktop/internal/model"
	"github.com/theopenbee/openbee/desktop/internal/theme"
	"github.com/theopenbee/openbee/desktop/internal/views/route"
	"github.com/theopenbee/openbee/desktop/locales"
)

const (
	constraintsRows      = 5
	constraintsMinHeight = 164
	toggleBleed          = 8
	togglePadY           = 4
	toggleGap            = 8
	toggleSize           = 16
	advancedHeadGap      = 4
	helperSize           = 14
	groupGap             = 8
	permissionsNoteSize  = 12
	checkboxBleed        = -12
	checkboxesGap        = 4
	skeletonHeadingW     = 112
	skeletonHeadingH     = 20
	skeletonLabelW       = 80
	skeletonLabelH       = 16
	skeletonFields       = 3
	skeletonFieldGap     = 8
)

type createState struct {
	form     model.CreateForm
	name     nameState
	seeded   bool
	advanced bool
	creating bool
	err      error
	reveal   bool
}

type createKey struct{}

func newCreate() createState {
	return createState{form: model.CreateForm{EngineArgs: map[string]string{}}}
}

func (cs *createState) fail(err error) {
	cs.err, cs.reveal = err, true
}

func (cs *createState) named() bool {
	return strings.TrimSpace(cs.form.Name) != ""
}

func (cs *createState) toggleScope(scope string, on bool) {
	cs.form.Scopes = slices.DeleteFunc(slices.Clone(cs.form.Scopes), func(s string) bool { return s == scope })
	if on {
		cs.form.Scopes = append(cs.form.Scopes, scope)
	}
}

func (p *Panel) createView(c *ui.Context) {
	tk := theme.Apply(c)
	configs := p.s.SystemConfigs
	configs.Ensure()
	root := ui.Column(c).Key(createKey{}).MinWidth(0).Gap(formGap)
	cs := ui.Local(root, createKey{}, newCreate)
	if !configs.Loaded && configs.Err == nil {
		root.Children(func() { createSkeleton(c, tk) })
		return
	}
	enabled := p.s.EnabledEngines()
	if !cs.seeded {
		cs.seeded = true
		cs.form.Engine = model.DefaultEngine(configs.Data["default_engine"], enabled)
	}
	cs.form.Engine = model.AvailableEngine(cs.form.Engine, enabled)
	submit := func() { p.createWorker(cs) }
	root.Children(func() {
		p.basicSection(c, tk, cs, enabled, submit)
		p.advancedSection(c, tk, cs, submit)
		if cs.err != nil {
			alert := kit.Alert(c, tk, kit.AlertOpts{Tone: kit.ToneDanger, Title: locales.WorkersFormCreateError(), Body: l10n.ErrorText(cs.err)})
			if cs.reveal {
				cs.reveal = false
				alert.ScrollIntoView()
			}
		}
		formFooter(c, tk, cs.creating, p.closePanel,
			locales.WorkersCreateWorker(), kit.ButtonOpts{Loading: cs.creating, Disabled: !cs.named()}, submit)
	})
}

func (p *Panel) basicSection(c *ui.Context, tk theme.Tokens, cs *createState, enabled []string, submit func()) {
	ui.Column(c).MinWidth(0).Gap(fieldsGap).Disabled(cs.creating).Children(func() {
		heading(c, locales.WorkersFormSectionBasic(), 2)
		p.nameField(c, tk, &cs.form.Name, &cs.name, cs.fail, submit)
		kit.Field(c, tk, locales.WorkersFormEngine(), locales.WorkersFormEngineHelper(), "", func() {
			kit.Select(c, tk, &cs.form.Engine, locales.WorkersFormEngine(), engineOptions(enabled), locales.WorkersFormEngineDefault())
		})
		kit.Field(c, tk, locales.WorkersFormDescriptionOptional(), locales.WorkersFormDescriptionCreateHelper(), "", func() {
			kit.TextArea(c, tk, &cs.form.Description, locales.WorkersFormDescriptionOptional(), locales.WorkersFormDescriptionPlaceholder(),
				descriptionRows, descriptionRows).MinHeight(descriptionMinHeight)
		})
	})
}

func (p *Panel) advancedSection(c *ui.Context, tk theme.Tokens, cs *createState, submit func()) {
	ui.Column(c).MinWidth(0).Gap(fieldsGap).Children(func() {
		ui.Column(c).MinWidth(0).Gap(advancedHeadGap).Children(func() {
			advancedToggle(c, tk, cs)
			ui.Text(c, locales.WorkersFormAdvancedHelper()).FontSize(helperSize).TextColor(tk.TextSecondary)
		})
		if !cs.advanced {
			return
		}
		ui.Column(c).MinWidth(0).Gap(fieldsGap).Disabled(cs.creating).Children(func() {
			kit.Field(c, tk, locales.WorkersFormConstraints(), locales.WorkersFormConstraintsHelper(), "", func() {
				kit.TextArea(c, tk, &cs.form.Constraints, locales.WorkersFormConstraints(), locales.WorkersFormConstraintsPlaceholder(),
					constraintsRows, constraintsRows).MinHeight(constraintsMinHeight)
			})
			engineArgsField(c, tk, cs.form.Engine, cs.form.EngineArgs, submit)
			kit.Field(c, tk, locales.WorkersFormWorkDir(), locales.WorkersFormWorkDirHelper(), "", func() {
				kit.TextInput(c, tk, &cs.form.WorkDir, locales.WorkersFormWorkDir(), locales.WorkersFormWorkDirPlaceholder()).OnSubmit(submit)
			})
			permissionsGroup(c, tk, cs)
		})
	})
}

func advancedToggle(c *ui.Context, tk theme.Tokens, cs *createState) {
	b := ui.ButtonBase(c).Label(locales.WorkersFormSectionAdvanced()).Expanded(cs.advanced).PaddingX(toggleBleed).MarginX(-toggleBleed).
		PaddingY(togglePadY).Gap(toggleGap).AlignItems(ui.Center).Justify(ui.Start).Radius(theme.RadiusControl).
		FontSize(headingSize).FontWeight(mediumWeight)
	bg := tk.SurfaceMuted.Alpha(0)
	if b.Hovered() || b.Pressed() {
		bg = tk.SurfaceMuted
	}
	b.Background(b.AnimateColor("bg", bg, fast)).OnClick(func() { cs.advanced = !cs.advanced }).Children(func() {
		ui.Text(c, locales.WorkersFormSectionAdvanced()).SingleLine().TextColor(tk.Foreground)
		if count := cs.form.AdvancedCount(); !cs.advanced && count > 0 {
			kit.SmallBadge(c, tk, locales.WorkersFormAdvancedSet(count), kit.ToneInfo)
		}
		turn := float32(0)
		if cs.advanced {
			turn = 180
		}
		chevron := ui.Icon(c, icons.Get("chevron-down")).Size(toggleSize, toggleSize).Shrink(0).MarginStart(ui.Auto).TextColor(tk.TextMuted)
		chevron.Rotate(chevron.AnimateTo("turn", turn, ui.Snappy))
	})
}

func permissionsGroup(c *ui.Context, tk theme.Tokens, cs *createState) {
	ui.Column(c).MinWidth(0).Gap(groupGap).Label(locales.WorkersFormSectionPermissions()).Children(func() {
		ui.Text(c, locales.WorkersFormSectionPermissions()).FontWeight(mediumWeight)
		ui.Text(c, locales.WorkersFormPermissionsHelper()).FontSize(permissionsNoteSize).TextColor(tk.TextMuted)
		ui.Column(c).MinWidth(0).MarginStart(checkboxBleed).Gap(checkboxesGap).Children(func() {
			for _, scope := range scopeRows {
				on := slices.Contains(cs.form.Scopes, scope.id)
				kit.CheckboxRow(c, tk, &on, scope.title(), scope.description()).OnChange(func() { cs.toggleScope(scope.id, on) })
			}
		})
	})
}

func createSkeleton(c *ui.Context, tk theme.Tokens) {
	ui.Column(c).MinWidth(0).Gap(formGap).Children(func() {
		ui.Column(c).MinWidth(0).Gap(fieldsGap).Children(func() {
			kit.SkeletonBar(c, tk, skeletonHeadingW, skeletonHeadingH)
			for range skeletonFields {
				ui.Column(c).MinWidth(0).Gap(skeletonFieldGap).Children(func() {
					kit.SkeletonBar(c, tk, skeletonLabelW, skeletonLabelH)
					kit.SkeletonBar(c, tk, 0, theme.ControlMD)
				})
			}
		})
		kit.SkeletonBar(c, tk, skeletonHeadingW, skeletonHeadingH)
	})
}

func (p *Panel) createWorker(cs *createState) {
	if cs.creating || !cs.named() {
		return
	}
	cs.creating, cs.err = true, nil
	p.s.CreateWorker(cs.form.Request(), func(w api.Worker, err error) {
		cs.creating = false
		if err != nil {
			if cs.err = failure(err); cs.err != nil {
				cs.reveal = true
				if cs.form.AdvancedCount() > 0 {
					cs.advanced = true
				}
			}
			return
		}
		p.nav.Replace(route.ChatPanel(w.ID, route.PanelConstraints))
	})
}

func (p *Panel) closePanel() {
	param, ok := strings.CutPrefix(p.nav.Path(), route.ChatPath+"/")
	if !ok {
		param = ""
	}
	p.nav.Push(route.Chat(param))
}
