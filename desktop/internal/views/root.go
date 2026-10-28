package views

import (
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/api"
	"github.com/theopenbee/openbee/desktop/internal/kit"
	"github.com/theopenbee/openbee/desktop/internal/store"
	"github.com/theopenbee/openbee/desktop/internal/theme"
	"github.com/theopenbee/openbee/desktop/internal/views/auth"
	"github.com/theopenbee/openbee/desktop/internal/views/route"
	"github.com/theopenbee/openbee/desktop/locales"
)

type settingsDialog interface {
	Open(section, pluginID string)
	View(c *ui.Context)
}

type accountDialogs interface {
	Dialogs(c *ui.Context)
}

type noDialogs struct{}

func (noDialogs) Open(string, string) {}
func (noDialogs) View(*ui.Context)    {}
func (noDialogs) Dialogs(*ui.Context) {}

type Root struct {
	s        *store.Store
	router   *ui.Router
	auth     *auth.Screens
	settings settingsDialog
	account  accountDialogs
}

func NewRoot(s *store.Store) *Root {
	r := &Root{s: s, router: ui.NewRouter("/"), settings: noDialogs{}, account: noDialogs{}}
	r.router.Transition = ui.TransitionNone
	r.auth = auth.New(s, r.router)
	return r
}

func (r *Root) Nav() route.Nav {
	return r.router
}

func (r *Root) View(c *ui.Context) {
	tk := theme.Apply(c)
	r.follow(c)
	ui.Column(c).Fill().Background(tk.Background).TextColor(tk.Foreground).Children(func() {
		if r.s.Phase == store.PhaseBoot {
			return
		}
		r.router.View(c, func(rt *ui.Route) { r.page(c, tk, rt) }).Grow(1)
	})
	if r.s.Phase == store.PhaseReady {
		r.settings.View(c)
		r.account.Dialogs(c)
	}
	kit.ShowToasts(c, tk, &r.s.Toasts)
}

func (r *Root) page(c *ui.Context, tk theme.Tokens, rt *ui.Route) {
	switch {
	case rt.Match(route.ConnectPath):
		r.auth.Connect(c, tk, rt.Page())
	case rt.Match(route.SetupPath):
		r.auth.Setup(c, tk, rt.Page())
	case rt.Match(route.LoginPath):
		r.auth.Login(c, tk, rt.Page())
	case rt.Match(route.ChatPath), rt.Match(route.ChatPath + "/{conversation}"):
		r.chat(c, tk)
	}
}

func (r *Root) chat(c *ui.Context, tk theme.Tokens) {
	ui.Column(c).Fill().Center().Children(func() {
		ui.Text(c, locales.ChatBeeLabel()).TextColor(tk.TextSecondary)
	})
}

func (r *Root) follow(c *ui.Context) {
	path := r.router.Path()
	if target, move := phaseRoute(r.s.Phase, path); move {
		r.router.Replace(target)
		path = target
	}
	if w, _ := c.Size(); r.s.Phase == store.PhaseReady && path == route.ChatPath && w >= theme.BreakTwoPane {
		target := route.Chat(api.BeeConversation)
		if _, query, ok := strings.Cut(r.router.Location(), "?"); ok {
			target += "?" + query
		}
		r.router.Replace(target)
	}
}

func phaseRoute(phase store.Phase, path string) (string, bool) {
	switch phase {
	case store.PhaseConnect:
		return route.ConnectPath, path != route.ConnectPath
	case store.PhaseSetup:
		return route.SetupPath, path != route.SetupPath && path != route.LoginPath
	case store.PhaseLogin:
		return route.LoginPath, path != route.LoginPath
	case store.PhaseReady:
		return route.ChatPath, conversationParam(path) == "" && path != route.ChatPath
	}
	return "", false
}

func conversationParam(path string) string {
	param, ok := strings.CutPrefix(path, route.ChatPath+"/")
	if !ok || strings.Contains(param, "/") {
		return ""
	}
	return param
}

func (r *Root) OpenSettings(section string) {
	if r.s.Phase == store.PhaseReady {
		r.settings.Open(section, "")
	}
}

func (r *Root) OpenConversation(cid string) {
	if param := route.Param(cid); param != "" && r.s.Phase == store.PhaseReady {
		r.router.Push(route.Chat(param))
	}
}

func (r *Root) NewWorker() {
	if r.s.Phase == store.PhaseReady {
		r.router.Push(route.ChatPanel(conversationParam(r.router.Path()), route.PanelNew))
	}
}
