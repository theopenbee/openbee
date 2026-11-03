package envpanel

import (
	"context"
	"errors"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/api"
	"github.com/theopenbee/openbee/desktop/internal/kit"
	"github.com/theopenbee/openbee/desktop/internal/l10n"
	"github.com/theopenbee/openbee/desktop/internal/model"
	"github.com/theopenbee/openbee/desktop/internal/store"
	"github.com/theopenbee/openbee/desktop/internal/theme"
	"github.com/theopenbee/openbee/desktop/locales"
)

const (
	sectionGap      = 16
	titleSize       = 16
	mediumWeight    = 500
	descriptionGap  = 4
	cardBorder      = 1
	rowMin          = 64
	rowPadY         = 12
	rowPadStart     = 24
	rowPadEnd       = 16
	rowGap          = 12
	rowCopyGap      = 4
	metaGap         = 8
	actionsGap      = 4
	keySize         = 14
	metaSize        = 12
	updatedMinWidth = 448
	skeletonRows    = 3
	skeletonKeyW    = 160
	skeletonKeyH    = 16
	skeletonMetaW   = 224
	skeletonMetaH   = 12
	skeletonActionW = 96
	skeletonActionH = 32
	skeletonCopyGap = 8
	metaSeparator   = "·"
	noTime          = "—"
	emptyIcon       = "key-round"
)

type Panel struct {
	s                  *store.Store
	scope, scopeID     string
	title, description string
	inset              bool
}

type state struct {
	add  addState
	edit editState
	del  deleteState
}

type stateKey struct{}

type widthKey struct{}

func New(s *store.Store, scope, scopeID, title, description string, inset bool) *Panel {
	return &Panel{s: s, scope: scope, scopeID: scopeID, title: title, description: description, inset: inset}
}

func (p *Panel) View(c *ui.Context) {
	tk := theme.Apply(c)
	r := p.s.Envs(p.scope, p.scopeID)
	r.Ensure()
	root := ui.Column(c).Key(p.scope + "/" + p.scopeID).MinWidth(0).Gap(sectionGap)
	st := ui.Local(root, stateKey{}, func() state { return state{} })
	failed := r.Err != nil
	loading := !r.Loaded && !failed
	empty := r.Loaded && !failed && len(r.Data) == 0
	root.Children(func() {
		if p.title != "" || p.description != "" || !empty {
			p.header(c, tk, st, !empty && !failed)
		}
		switch {
		case failed:
			kit.Alert(c, tk, kit.AlertOpts{Tone: kit.ToneDanger, Title: locales.EnvConfigLoadError(), Body: l10n.ErrorText(r.Err),
				ActionLabel: locales.CommonRetry(), ActionLoading: r.Loading, OnAction: r.Invalidate})
		default:
			p.list(c, tk, st, r, loading, empty)
		}
		var existing []api.EnvConfig
		if r.Loaded {
			existing = r.Data
		}
		p.addDialog(c, tk, &st.add, existing)
		p.editDialog(c, tk, &st.edit)
		p.deleteDialog(c, tk, &st.del)
	})
}

func (p *Panel) header(c *ui.Context, tk theme.Tokens, st *state, showAdd bool) {
	ui.Row(c).MinWidth(0).AlignItems(ui.Start).Gap(sectionGap).Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Children(func() {
			if p.title != "" {
				ui.Text(c, p.title).FontSize(titleSize).FontWeight(mediumWeight).Role(ui.RoleHeading).Level(2)
			}
			if p.description != "" {
				ui.Text(c, p.description).TextColor(tk.TextSecondary).Margin(descriptionGap, 0, 0, 0)
			}
		})
		if showAdd {
			p.addButton(c, tk, st).Shrink(0).MarginStart(ui.Auto)
		}
	})
}

func (p *Panel) addButton(c *ui.Context, tk theme.Tokens, st *state) ui.Element {
	return kit.Button(c, tk, locales.EnvConfigAdd(), kit.ButtonOpts{Variant: kit.Secondary, Size: kit.SizeSM, Icon: "plus"}).
		OnClick(st.add.show)
}

func (p *Panel) list(c *ui.Context, tk theme.Tokens, st *state, r *store.Resource[[]api.EnvConfig], loading, empty bool) {
	radius := theme.RadiusSurface
	if p.inset {
		radius = theme.RadiusInset
	}
	card := ui.Column(c).MinWidth(0).Clip().Radius(radius).Border(cardBorder, tk.Border).Background(tk.Surface)
	wide := measuredWide(c, card)
	card.Children(func() {
		switch {
		case loading:
			skeletonRowsView(c, tk)
		case empty:
			kit.EmptyState(c, tk, emptyIcon, locales.EnvConfigEmpty(), p.emptyDescription(), func() { p.addButton(c, tk, st) })
		default:
			now := c.Now()
			for i, env := range r.Data {
				p.row(c, tk, st, env, i == 0, wide, now)
			}
		}
	})
}

func (p *Panel) emptyDescription() string {
	switch p.scope {
	case "global":
		return locales.EnvConfigEmptyGlobal()
	case "bee":
		return locales.EnvConfigEmptyBee()
	case "worker":
		return locales.EnvConfigEmptyWorker()
	}
	return ""
}

func measuredWide(c *ui.Context, card ui.Element) bool {
	wide := ui.Local(card, widthKey{}, func() bool { return true })
	if b := card.LayoutBounds(); b.W > 0 {
		*wide = b.W-2*cardBorder >= updatedMinWidth
	}
	decided, services := *wide, c.Services()
	card.Draw(func(_ *ui.Painter, r ui.Rect) {
		if (r.W-2*cardBorder >= updatedMinWidth) != decided {
			services.Invalidate()
		}
	})
	return decided
}

func (p *Panel) row(c *ui.Context, tk theme.Tokens, st *state, env api.EnvConfig, first, wide bool, now time.Time) {
	row := ui.Row(c).Key(env.ID).MinHeight(rowMin).AlignItems(ui.Center).Gap(rowGap).Padding(rowPadY, rowPadEnd, rowPadY, rowPadStart)
	if !first {
		row.BorderWidth(1, 0, 0, 0).BorderColor(tk.BorderSubtle)
	}
	row.Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(rowCopyGap).Children(func() {
			key := ui.Text(c, env.Key).Font(theme.FontMono).FontSize(keySize).TextColor(tk.Foreground).SingleLine()
			kit.Tooltip(c, tk, key, env.Key)
			ui.Row(c).MinWidth(0).AlignItems(ui.Center).Gap(metaGap).Children(func() {
				ui.Text(c, env.Masked).Font(theme.FontMono).FontSize(metaSize).TextColor(tk.TextSecondary).SingleLine().MinWidth(0)
				if wide {
					ui.Text(c, metaSeparator).FontSize(metaSize).TextColor(tk.TextMuted).Shrink(0)
					ui.Text(c, locales.EnvConfigUpdated(relative(env.UpdatedAt, now))).FontSize(metaSize).TextColor(tk.TextMuted).SingleLine().Shrink(0)
				}
			})
		})
		ui.Row(c).Shrink(0).AlignItems(ui.Center).Gap(actionsGap).Children(func() {
			copyLabel := locales.EnvConfigCopyKey(env.Key)
			kit.Tooltip(c, tk, kit.CopyIconButton(c, tk, env.Key, copyLabel), copyLabel)
			kit.IconButton(c, tk, "pencil", locales.EnvConfigEditAction(), kit.ButtonOpts{Variant: kit.Ghost, Size: kit.SizeSM}).
				Label(locales.EnvConfigEditLabel(env.Key)).OnClick(func() { st.edit.show(env) })
			kit.IconButton(c, tk, "trash-2", locales.EnvConfigDeleteAction(), kit.ButtonOpts{Variant: kit.Ghost, Size: kit.SizeSM}).
				Label(locales.EnvConfigDeleteLabel(env.Key)).OnClick(func() { st.del.show(env) })
		})
	})
}

func skeletonRowsView(c *ui.Context, tk theme.Tokens) {
	for i := range skeletonRows {
		row := ui.Row(c).Key(i).MinHeight(rowMin).AlignItems(ui.Center).Gap(rowGap).Padding(rowPadY, rowPadEnd, rowPadY, rowPadStart)
		if i > 0 {
			row.BorderWidth(1, 0, 0, 0).BorderColor(tk.BorderSubtle)
		}
		row.Children(func() {
			ui.Column(c).Grow(1).MinWidth(0).Gap(skeletonCopyGap).Children(func() {
				kit.SkeletonBar(c, tk, skeletonKeyW, skeletonKeyH).MaxWidthPercent(100)
				kit.SkeletonBar(c, tk, skeletonMetaW, skeletonMetaH).MaxWidthPercent(100)
			})
			kit.SkeletonBar(c, tk, skeletonActionW, skeletonActionH)
		})
	}
}

func relative(ts int64, now time.Time) string {
	r := model.RelativeTime(ts, now)
	switch r.Unit {
	case "now":
		return locales.TimeJustNow()
	case "minutes":
		return locales.TimeMinutesAgo(r.N)
	case "hours":
		return locales.TimeHoursAgo(r.N)
	case "days":
		return locales.TimeDaysAgo(r.N)
	}
	return noTime
}

func failure(err error) error {
	if errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}

func errorAlert(c *ui.Context, tk theme.Tokens, title string, err error) {
	if err != nil {
		kit.Alert(c, tk, kit.AlertOpts{Tone: kit.ToneDanger, Title: title, Body: l10n.ErrorText(err)})
	}
}
