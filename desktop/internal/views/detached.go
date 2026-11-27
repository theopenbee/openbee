package views

import (
	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/api"
	"github.com/theopenbee/openbee/desktop/internal/kit"
	"github.com/theopenbee/openbee/desktop/internal/store"
	"github.com/theopenbee/openbee/desktop/internal/theme"
	"github.com/theopenbee/openbee/desktop/internal/views/chat"
	"github.com/theopenbee/openbee/desktop/internal/views/route"
	"github.com/theopenbee/openbee/desktop/locales"
)

type DetachedOptions struct {
	ShowMain func()
	Close    func()
	SetTitle func(title string)
}

type Detached struct {
	s     *store.Store
	cid   string
	chat  *chat.View
	o     DetachedOptions
	title string
}

type mainNav struct {
	nav  route.Nav
	show func()
}

func (n mainNav) Push(target string) {
	n.show()
	n.nav.Push(target)
}

func (n mainNav) Replace(target string) {
	n.show()
	n.nav.Replace(target)
}

func (n mainNav) Path() string             { return n.nav.Path() }
func (n mainNav) Query(name string) string { return n.nav.Query(name) }

func (r *Root) NewDetached(cid string, o DetachedOptions) *Detached {
	nav := mainNav{nav: r.router, show: o.ShowMain}
	return &Detached{s: r.s, cid: cid, chat: chat.NewDetached(r.s, nav, o.Close), o: o}
}

func (d *Detached) Title() string {
	id, isWorker := api.ConversationWorkerID(d.cid)
	if !isWorker {
		return locales.ChatBeeLabel()
	}
	if w, ok := d.s.Worker(id); ok {
		return w.Name
	}
	return ""
}

func (d *Detached) View(c *ui.Context) {
	tk := theme.Apply(c)
	ui.Column(c).Fill().Background(tk.Background).TextColor(tk.Foreground).Children(func() {
		if d.s.Phase == store.PhaseReady {
			d.chat.View(c, d.cid, route.Panel{})
		}
	})
	if title := d.Title(); title != "" && title != d.title {
		d.title = title
		if d.o.SetTitle != nil {
			d.o.SetTitle(title)
		}
	}
	if d.s.FocusedDetached() == d.cid {
		d.s.PruneToasts()
		kit.ShowToasts(c, tk, &d.s.Toasts)
	}
}
