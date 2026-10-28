package chat

import (
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
	"github.com/theopenbee/openbee/desktop/internal/api"
	"github.com/theopenbee/openbee/desktop/internal/model"
	"github.com/theopenbee/openbee/desktop/internal/store"
	"github.com/theopenbee/openbee/desktop/internal/theme"
	"github.com/theopenbee/openbee/desktop/internal/views/route"
)

const (
	mountRise     = 4
	mountDuration = 240 * time.Millisecond
	mediumWeight  = 500
	stateIdle     = "idle"
	stateRunning  = "running"
)

var (
	easeEnter    = ui.CubicBezier(0.16, 1, 0.3, 1)
	easeStandard = ui.CubicBezier(0.22, 1, 0.36, 1)
	colorFade    = ui.Animation{Duration: 150 * time.Millisecond, Ease: easeStandard}
)

type mountKey struct{}

type pendingKey struct{}

type View struct {
	s           *store.Store
	nav         route.Nav
	togglePanel func(cid string)
	md          *markdown.Options
	loads       *imageLoads
	themes      [2]*markdown.Theme
	images      *imageCache
	mediaGen    uint64
	tk          theme.Tokens
	link        linkState
	table       tableModal
	light       lightboxState
	photos      *photoLoads
	opening     map[string]bool
	openPath    func(path string) error
	convs       map[string]*convState
	foreground  bool
	save        func(name string, data []byte) (string, error)
}

type convState struct {
	list      ui.ListState
	scroll    ui.ScrollState
	msgs      []model.Message
	expanded  map[string]bool
	reset     resetState
	refetched bool
	moreSpace bool
}

func New(s *store.Store, nav route.Nav, togglePanel func(cid string)) *View {
	v := &View{s: s, nav: nav, togglePanel: togglePanel, convs: map[string]*convState{}, foreground: true, save: saveWithDialog,
		openPath: mygo.Shell.OpenPath}
	v.resetMedia()
	s.OnSessionReset(func() {
		v.convs = map[string]*convState{}
		v.link, v.table, v.light = linkState{}, tableModal{}, lightboxState{}
		v.resetMedia()
	})
	s.OnResync(func() {
		v.recycleMarkdown(true)
		v.photos.retryFailed()
	})
	return v
}

func (v *View) Markdown() *markdown.Options {
	return v.md
}

func (v *View) resetMedia() {
	v.mediaGen++
	v.images = newImageCache(v.mediaImages(v.mediaGen), fetchExternalImage)
	v.photos, v.opening = newPhotoLoads(), map[string]bool{}
	v.useMarkdown(newImageLoads(nil))
}

func (v *View) recycleMarkdown(retryFailed bool) {
	if v.loads.due(retryFailed) {
		v.useMarkdown(newImageLoads(v.loads))
	}
}

func (v *View) useMarkdown(loads *imageLoads) {
	v.loads, v.md = loads, newMarkdown(v, loads)
}

func (v *View) state(cid string) *convState {
	if cs, ok := v.convs[cid]; ok {
		return cs
	}
	cs := &convState{expanded: map[string]bool{}}
	cs.list.Key = func(i int) any { return cs.msgs[i].ID }
	cs.list.FollowEnd = true
	cs.list.FollowAnimation = &ui.Smooth
	v.convs[cid] = cs
	return cs
}

func (v *View) View(c *ui.Context, cid string, panel route.Panel) {
	v.recycleMarkdown(false)
	tk := theme.Apply(c)
	v.tk = tk
	v.md.Theme = v.markdownTheme(tk)
	cs := v.state(cid)
	w, ok := v.resolve(cid, cs)
	if !ok {
		ui.Column(c).Key(pendingKey{}).Grow(1).MinHeight(0).Children(func() {
			v.headerBar(c, tk, nil)
			logSkeleton(c, tk)
		})
		return
	}
	ch := v.s.Chat(cid)
	root := ui.Column(c).Key(cid).Grow(1).MinHeight(0)
	ui.Local(root, mountKey{}, func() bool {
		ch.Ensure()
		ch.MarkRead(0)
		return true
	})
	v.followForeground(ch)
	shown := root.Keyframes("mount", []ui.Keyframe{{At: 0, Value: 0}, {At: mountDuration, Value: 1, Ease: easeEnter}})
	root.Opacity(shown).Translate(0, mountRise*(1-shown)).Children(func() {
		p := pane{cid: cid, worker: w, ch: ch, cs: cs}
		v.header(c, tk, p, panel.Mode == route.ModeWorker)
		v.log(c, tk, p)
		v.resetDialog(c, tk, p)
		v.tableDialog(c, tk)
		v.linkDialog(c, tk)
		v.lightbox(c, tk)
	})
}

type pane struct {
	cid    string
	worker *api.Worker
	ch     *store.Chat
	cs     *convState
}

func (v *View) resolve(cid string, cs *convState) (*api.Worker, bool) {
	id, isWorker := api.ConversationWorkerID(cid)
	if !isWorker {
		return nil, isBee(cid)
	}
	workers := v.s.Workers
	workers.Ensure()
	if !workers.Loaded {
		return nil, false
	}
	if w, found := v.s.Worker(id); found {
		cs.refetched = false
		return &w, true
	}
	switch {
	case !cs.refetched:
		cs.refetched = true
		workers.Invalidate()
	case !workers.Loading && workers.Err == nil:
		cs.refetched = false
		v.nav.Replace(route.Chat(api.BeeConversation))
	}
	return nil, false
}

func (v *View) followForeground(ch *store.Chat) {
	fg := v.s.Foreground()
	if fg && !v.foreground {
		ch.MarkRead(0)
	}
	v.foreground = fg
}

func (v *View) activity(cid string) api.ConversationActivity {
	if isBee(cid) {
		if v.s.Chat(cid).BeeBusy {
			return api.ConversationActivity{State: stateRunning}
		}
		return api.ConversationActivity{State: stateIdle}
	}
	if v.s.Conversations.Loaded {
		for _, sum := range v.s.Conversations.Data {
			if sum.ID == cid {
				return sum.Activity
			}
		}
	}
	return api.ConversationActivity{State: stateIdle}
}

func isBee(cid string) bool {
	return cid == api.BeeConversation
}
