package settings

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/api"
	"github.com/theopenbee/openbee/desktop/internal/kit"
	"github.com/theopenbee/openbee/desktop/internal/l10n"
	"github.com/theopenbee/openbee/desktop/internal/model"
	"github.com/theopenbee/openbee/desktop/internal/theme"
	"github.com/theopenbee/openbee/desktop/locales"
)

const (
	keyDefaultEngine = "default_engine"
	keyArgsGlobal    = "engine_args_global"
	keyArgsBee       = "engine_args_bee"
)

const (
	savedFlash    = 2400 * time.Millisecond
	formGap       = 40
	groupGap      = 16
	headingGap    = 4
	headingSize   = 16
	rowTitleSize  = 14
	mediumWeight  = 500
	rowPadY       = 20
	rowPadXNarrow = 16
	rowPadXWide   = 24
	rowGapNarrow  = 16
	rowGapWide    = 32
	rowLabelWidth = 224
	rowTextGap    = 4
	argsGap       = 16
	containerLG   = 512
	container2XL  = 672
	cardBorder    = 1
	skeletonHead  = 128
	skeletonHeadH = 20
	skeletonName  = 96
	skeletonLine  = 160
	skeletonLineH = 16
	skeletonText  = 8
	skeletonField = 44
	argsLabelJoin = ": "
)

type saveStatus int

const (
	statusIdle saveStatus = iota
	statusSaving
	statusSaved
	statusError
)

type systemState struct {
	engine    string
	engineSet bool
	global    map[string]string
	bee       map[string]string
	status    saveStatus
	savedAt   time.Time
	err       error
}

type systemKey struct{}

type values struct {
	engine string
	global map[string]string
	bee    map[string]string
}

func (st *systemState) edited() {
	if st.status == statusSaved || st.status == statusError {
		st.status, st.err = statusIdle, nil
	}
}

func (st *systemState) dirty(saved values) (engine, global, bee bool) {
	return st.engineSet && st.engine != saved.engine,
		st.global != nil && !maps.Equal(model.CleanEngineArgs(st.global), saved.global),
		st.bee != nil && !maps.Equal(model.CleanEngineArgs(st.bee), saved.bee)
}

func (d *Dialog) system(c *ui.Context, tk theme.Tokens) {
	r := d.s.SystemConfigs
	r.Ensure()
	root := ui.Column(c).Key(SectionSystem).MinWidth(0)
	st := ui.Local(root, systemKey{}, func() systemState { return systemState{} })
	root.Children(func() {
		switch {
		case r.Err != nil:
			kit.Alert(c, tk, kit.AlertOpts{Tone: kit.ToneDanger, Title: locales.SystemSettingsLoadError(), Body: l10n.ErrorText(r.Err),
				ActionLabel: locales.CommonRetry(), ActionLoading: r.Loading, OnAction: r.Invalidate})
		case !r.Loaded:
			d.systemSkeleton(c, tk)
		default:
			d.systemForm(c, tk, st, values{
				engine: r.Data[keyDefaultEngine],
				global: model.CleanEngineArgs(model.ParseEngineArgsConfig(r.Data[keyArgsGlobal])),
				bee:    model.CleanEngineArgs(model.ParseEngineArgsConfig(r.Data[keyArgsBee])),
			})
		}
	})
}

func (d *Dialog) systemForm(c *ui.Context, tk theme.Tokens, st *systemState, saved values) {
	engines := d.s.EnabledEngines()
	saving := st.status == statusSaving
	engine := saved.engine
	if st.engineSet {
		engine = st.engine
	}
	global, bee := saved.global, saved.bee
	if st.global != nil {
		global = st.global
	}
	if st.bee != nil {
		bee = st.bee
	}
	engineDirty, globalDirty, beeDirty := st.dirty(saved)
	dirty := engineDirty || globalDirty || beeDirty
	submit := func() {
		if !dirty || saving {
			return
		}
		changes := map[string]string{}
		if engineDirty {
			changes[keyDefaultEngine] = engine
		}
		if globalDirty {
			changes[keyArgsGlobal] = argsJSON(global)
		}
		if beeDirty {
			changes[keyArgsBee] = argsJSON(bee)
		}
		st.status, st.err = statusSaving, nil
		d.s.SaveSystemConfigs(changes, func(err error) { st.finish(err) })
	}
	ui.Column(c).MinWidth(0).Gap(formGap).Children(func() {
		d.engineGroup(c, tk, st, &engine, saved.engine, engines, saving)
		d.argsGroup(c, tk, st, engines, global, bee, saving, submit)
		if st.status == statusError {
			kit.Alert(c, tk, kit.AlertOpts{Tone: kit.ToneDanger, Title: locales.SystemSettingsSaveError(), Body: l10n.ErrorText(st.err)})
		}
		d.saveButton(c, tk, st, dirty, submit)
	})
}

func (st *systemState) finish(err error) {
	switch {
	case errors.Is(err, context.Canceled):
		st.status = statusIdle
	case err != nil:
		st.status, st.err = statusError, err
	default:
		st.engineSet, st.global, st.bee = false, nil, nil
		st.status, st.savedAt = statusSaved, time.Time{}
	}
}

func argsJSON(args map[string]string) string {
	out, _ := json.Marshal(model.CleanEngineArgs(args))
	return string(out)
}

func (d *Dialog) saveButton(c *ui.Context, tk theme.Tokens, st *systemState, dirty bool, submit func()) {
	showSaved := false
	if st.status == statusSaved {
		if st.savedAt.IsZero() {
			st.savedAt = c.Now()
		}
		if left := savedFlash - c.Now().Sub(st.savedAt); left > 0 {
			showSaved = true
			c.After(left)
		}
	}
	opts := kit.ButtonOpts{Loading: st.status == statusSaving, Disabled: !dirty && st.status != statusError}
	text := locales.SystemSettingsSave()
	if showSaved {
		text, opts.Icon = locales.SystemSettingsSaved(), "check"
	}
	ui.Row(c).Justify(ui.End).Children(func() {
		kit.Button(c, tk, text, opts).OnClick(submit)
	})
}

func (d *Dialog) engineGroup(c *ui.Context, tk theme.Tokens, st *systemState, engine *string, saved string, engines []api.EngineInfo, saving bool) {
	options := kit.EngineOptions(engines, saved)
	help := ""
	if len(options) == 0 {
		help = locales.WorkersFormNoEngine()
	}
	ui.Column(c).MinWidth(0).Gap(groupGap).Children(func() {
		heading(c, locales.SystemSettingsEngineSectionTitle())
		card(c, tk, func() {
			d.row(c, tk, true, locales.SystemSettingsEngineSectionRowTitle(), locales.SystemSettingsEngineSectionDescription(), func() {
				kit.Field(c, tk, locales.SystemSettingsEngineSectionLabel(), help, "", func() {
					kit.Select(c, tk, engine, locales.SystemSettingsEngineSectionLabel(), options, "").Disabled(saving || len(options) == 0).
						OnChange(func() {
							st.engine, st.engineSet = *engine, true
							st.edited()
						})
				})
			})
		})
	})
}

func (d *Dialog) argsGroup(c *ui.Context, tk theme.Tokens, st *systemState, engines []api.EngineInfo, global, bee map[string]string, saving bool, submit func()) {
	ui.Column(c).MinWidth(0).Gap(groupGap).Children(func() {
		ui.Column(c).MinWidth(0).Gap(headingGap).Children(func() {
			heading(c, locales.SystemSettingsArgsSectionTitle())
			ui.Text(c, locales.SystemSettingsArgsSectionDescription()).TextColor(tk.TextSecondary)
		})
		card(c, tk, func() {
			d.row(c, tk, true, locales.SystemSettingsGlobalArgsSectionTitle(), locales.SystemSettingsGlobalArgsSectionDescription(), func() {
				d.engineArgs(c, tk, locales.SystemSettingsGlobalArgsSectionTitle(), engines, global, saving, submit, func(args map[string]string) {
					st.global = args
					st.edited()
				})
			})
			d.row(c, tk, false, locales.SystemSettingsBeeArgsSectionTitle(), locales.SystemSettingsBeeArgsSectionDescription(), func() {
				d.engineArgs(c, tk, locales.SystemSettingsBeeArgsSectionTitle(), engines, bee, saving, submit, func(args map[string]string) {
					st.bee = args
					st.edited()
				})
			})
		})
	})
}

func (d *Dialog) engineArgs(c *ui.Context, tk theme.Tokens, group string, engines []api.EngineInfo, args map[string]string, saving bool, submit func(), changed func(map[string]string)) {
	ui.Column(c).MinWidth(0).Gap(argsGap).Children(func() {
		if len(engines) == 0 {
			ui.Text(c, locales.WorkersFormNoEngine()).TextColor(tk.TextSecondary)
		}
		for _, info := range engines {
			engine := info.ID
			name := l10n.EngineName(info)
			label, help := name, ""
			if len(engines) == 1 {
				label, help = locales.WorkersFormEngineArgs(), locales.WorkersFormEngineArgsHelper(name)
			}
			value := args[engine]
			ui.Column(c).Key(engine).MinWidth(0).Children(func() {
				kit.Field(c, tk, label, help, "", func() {
					kit.TextInput(c, tk, &value, group+argsLabelJoin+label, locales.WorkersFormEngineArgsPlaceholder()).
						Disabled(saving).OnSubmit(submit).OnChange(func() {
						next := maps.Clone(args)
						if next == nil {
							next = map[string]string{}
						}
						next[engine] = value
						changed(next)
					})
				})
			})
		}
	})
}

func heading(c *ui.Context, text string) ui.Element {
	return ui.Text(c, text).FontSize(headingSize).FontWeight(mediumWeight).Role(ui.RoleHeading).Level(2)
}

func card(c *ui.Context, tk theme.Tokens, rows func()) ui.Element {
	return ui.Column(c).MinWidth(0).Clip().Radius(theme.RadiusInset).Border(cardBorder, tk.Border).Background(tk.Surface).Children(rows)
}

func (d *Dialog) row(c *ui.Context, tk theme.Tokens, first bool, title, description string, control func()) {
	cw := d.contentWidth()
	padX := float32(rowPadXNarrow)
	if cw >= containerLG {
		padX = rowPadXWide
	}
	var row ui.Element
	if cw >= container2XL {
		row = ui.Row(c).AlignItems(ui.Start).Gap(rowGapWide)
	} else {
		row = ui.Column(c).Gap(rowGapNarrow)
	}
	row.MinWidth(0).Padding(rowPadY, padX)
	if !first {
		row.BorderWidth(cardBorder, 0, 0, 0).BorderColor(tk.BorderSubtle)
	}
	row.Children(func() {
		text := ui.Column(c).MinWidth(0).Gap(rowTextGap)
		if cw >= container2XL {
			text.Width(rowLabelWidth).Shrink(1)
		}
		text.Children(func() {
			ui.Text(c, title).FontSize(rowTitleSize).FontWeight(mediumWeight).Role(ui.RoleHeading).Level(3)
			ui.Text(c, description).TextColor(tk.TextSecondary)
		})
		ui.Column(c).MinWidth(0).Grow(1).Children(control)
	})
}

func (d *Dialog) systemSkeleton(c *ui.Context, tk theme.Tokens) {
	wide := d.contentWidth() >= container2XL
	ui.Column(c).MinWidth(0).Gap(formGap).Children(func() {
		for rows := 1; rows <= 2; rows++ {
			ui.Column(c).Key(rows).MinWidth(0).Gap(groupGap).Children(func() {
				kit.SkeletonBar(c, tk, skeletonHead, skeletonHeadH)
				card(c, tk, func() {
					for i := range rows {
						d.skeletonRow(c, tk, i, wide)
					}
				})
			})
		}
	})
}

func (d *Dialog) skeletonRow(c *ui.Context, tk theme.Tokens, index int, wide bool) {
	var row ui.Element
	gap := float32(rowGapNarrow)
	if wide {
		row, gap = ui.Row(c).AlignItems(ui.Start), rowGapWide
	} else {
		row = ui.Column(c)
	}
	row.Key(index).MinWidth(0).Gap(gap).Padding(rowPadY, rowPadXWide)
	if index > 0 {
		row.BorderWidth(cardBorder, 0, 0, 0).BorderColor(tk.BorderSubtle)
	}
	row.Children(func() {
		text := ui.Column(c).Gap(skeletonText)
		if wide {
			text.Width(rowLabelWidth).Shrink(0)
		}
		text.Children(func() {
			kit.SkeletonBar(c, tk, skeletonName, skeletonLineH)
			kit.SkeletonBar(c, tk, skeletonLine, skeletonLineH)
		})
		field := kit.SkeletonBar(c, tk, 0, skeletonField)
		if wide {
			field.Grow(1)
		}
	})
}
