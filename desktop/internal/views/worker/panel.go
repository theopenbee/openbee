package worker

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/api"
	"github.com/theopenbee/openbee/desktop/internal/kit"
	"github.com/theopenbee/openbee/desktop/internal/l10n"
	"github.com/theopenbee/openbee/desktop/internal/store"
	"github.com/theopenbee/openbee/desktop/internal/theme"
	"github.com/theopenbee/openbee/desktop/internal/views/envpanel"
	"github.com/theopenbee/openbee/desktop/internal/views/route"
	"github.com/theopenbee/openbee/desktop/locales"
)

const (
	sectionGap   = 24
	tabsGap      = 16
	headingSize  = 16
	mediumWeight = 500
	buttonsGap   = 8
	envCardPad   = 24
)

var (
	fast   = ui.Animation{Duration: 160 * time.Millisecond, Ease: ui.CubicBezier(0.22, 1, 0.36, 1)}
	tabIDs = []string{route.PanelConstraints, route.PanelPermissions, route.PanelEnv}
)

type Panel struct {
	s   *store.Store
	nav route.Nav
}

type panelState struct {
	editing  bool
	menuOpen bool
	del      deleteState
}

type panelKey struct{}

type tabsKey struct{}

func New(s *store.Store, nav route.Nav) *Panel {
	return &Panel{s: s, nav: nav}
}

func (p *Panel) View(c *ui.Context, conversationID string, panel route.Panel) {
	switch panel.Mode {
	case route.ModeNew:
		p.createView(c)
	case route.ModeWorker:
		if id, ok := api.ConversationWorkerID(conversationID); ok {
			p.workerView(c, theme.Apply(c), id, panel.Tab)
		}
	}
}

func (p *Panel) workerView(c *ui.Context, tk theme.Tokens, id, tab string) {
	detail := p.s.WorkerDetail(id)
	detail.Ensure()
	root := ui.Column(c).Key(id).MinWidth(0).Gap(sectionGap)
	st := ui.Local(root, panelKey{}, func() panelState { return panelState{} })
	root.Children(func() {
		switch {
		case detail.Err != nil && !detail.Loaded:
			kit.Alert(c, tk, kit.AlertOpts{Tone: kit.ToneDanger, Title: locales.WorkerDetailLoadError(), Body: l10n.ErrorText(detail.Err),
				ActionLabel: locales.CommonRetry(), OnAction: detail.Invalidate})
		case !detail.Loaded:
			skeleton(c, tk)
		default:
			w := detail.Data
			p.header(c, tk, st, w)
			if st.editing {
				p.editForm(c, tk, st, w)
			} else {
				p.tabs(c, tk, w, tab)
			}
			p.deleteDialog(c, tk, &st.del)
		}
	})
}

func (p *Panel) tabs(c *ui.Context, tk theme.Tokens, w api.Worker, tab string) {
	selected := max(0, slices.Index(tabIDs, tab))
	ui.Column(c).Key(tabsKey{}).MinWidth(0).Gap(tabsGap).Children(func() {
		labels := []string{locales.ChatPanelTabsConstraints(), locales.ChatPanelTabsPermissions(), locales.ChatPanelTabsEnv()}
		kit.CompactTabs(c, tk, &selected, locales.WorkerDetailSectionNav(), labels).
			OnChange(func() { p.nav.Replace(route.ChatPanel(w.ID, tabIDs[selected])) })
		shown := tabIDs[selected]
		ui.Column(c).Key(shown).MinWidth(0).Children(func() {
			switch shown {
			case route.PanelConstraints:
				p.constraints(c, tk, w)
			case route.PanelPermissions:
				p.permissions(c, tk, w)
			case route.PanelEnv:
				p.env(c, tk, w)
			}
		})
	})
}

func (p *Panel) env(c *ui.Context, tk theme.Tokens, w api.Worker) {
	ui.Column(c).MinWidth(0).Gap(sectionGap).Children(func() {
		card(c, tk).Padding(envCardPad).Children(func() {
			envpanel.New(p.s, "worker", w.ID, locales.EnvConfigWorkerTitle(), "", true).View(c)
		})
		envpanel.Effective(c, p.s, w.ID)
	})
}

func card(c *ui.Context, tk theme.Tokens) ui.Element {
	return ui.Column(c).MinWidth(0).Clip().Radius(theme.RadiusSurface).Background(tk.Surface).Border(1, tk.Border)
}

func heading(c *ui.Context, text string, level int) ui.Element {
	return ui.Text(c, text).FontSize(headingSize).FontWeight(mediumWeight).Role(ui.RoleHeading).Level(level)
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
