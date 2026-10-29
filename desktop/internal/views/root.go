package views

import (
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/api"
	"github.com/theopenbee/openbee/desktop/internal/kit"
	"github.com/theopenbee/openbee/desktop/internal/store"
	"github.com/theopenbee/openbee/desktop/internal/theme"
	"github.com/theopenbee/openbee/desktop/internal/views/account"
	"github.com/theopenbee/openbee/desktop/internal/views/auth"
	"github.com/theopenbee/openbee/desktop/internal/views/chat"
	"github.com/theopenbee/openbee/desktop/internal/views/route"
	"github.com/theopenbee/openbee/desktop/internal/views/settings"
	"github.com/theopenbee/openbee/desktop/internal/views/shell"
	"github.com/theopenbee/openbee/desktop/internal/views/worker"
)

type Root struct {
	s        *store.Store
	router   *ui.Router
	auth     *auth.Screens
	shell    *shell.Shell
	settings *settings.Dialog
	account  *account.Account
}

func NewRoot(s *store.Store) *Root {
	r := &Root{s: s, router: ui.NewRouter("/"), settings: settings.New(s), account: account.New(s)}
	r.router.Transition = ui.TransitionNone
	r.auth = auth.New(s, r.router)
	r.shell = shell.New(s, r.router, chat.New(s, r.router, r.togglePanel), worker.New(s, r.router), r.account, r.OpenSettings)
	return r
}

func (r *Root) togglePanel(cid string) {
	param := route.Param(cid)
	if route.ParsePanel(param, r.router.Query(route.PanelQuery)).Mode == route.ModeWorker {
		r.router.Push(route.Chat(param))
		return
	}
	r.router.Push(route.ChatPanel(param, route.PanelConstraints))
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
	} else {
		r.settings.Close()
		r.account.Close()
	}
	r.s.PruneToasts()
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
	case r.s.Phase != store.PhaseReady:
	case rt.Match(route.ChatPath):
		r.shell.View(c, "")
	case rt.Match(route.ChatPath + "/{conversation}"):
		r.shell.View(c, rt.Param("conversation"))
	}
}

func (r *Root) follow(c *ui.Context) {
	path := r.router.Path()
	if target, move := phaseRoute(r.s.Phase, path); move {
		r.router.Replace(target)
		path = target
	}
	if param := conversationParam(path); r.s.Phase == store.PhaseReady && param != "" && route.ConversationID(param) == "" {
		path = route.Chat(api.BeeConversation)
		r.router.Replace(path)
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
