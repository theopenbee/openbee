package store

import (
	"context"
	"slices"
	"time"

	"github.com/theopenbee/openbee/desktop/internal/api"
	"github.com/theopenbee/openbee/desktop/internal/kit"
	"github.com/theopenbee/openbee/desktop/internal/l10n"
	"github.com/theopenbee/openbee/desktop/internal/model"
	"github.com/theopenbee/openbee/desktop/locales"
)

const (
	opsPollEvery     = time.Second
	pluginsPollEvery = 2 * time.Second
	catalogRefresh   = 90 * time.Second
	opKindInstall    = "install"
	opKindUninstall  = "uninstall"
	opSucceeded      = "succeeded"
)

type PluginsState struct {
	List    *Resource[[]api.PluginStatus]
	Catalog *Resource[api.PluginCatalog]
	Ops     *Resource[[]api.PluginOperation]
	Details map[string]*Resource[api.PluginDetail]
	Pending map[string]bool

	s         *Store
	active    bool
	requested map[string]bool
	seen      map[string]bool
	stopOps   func()
	stopList  func()
}

func (s *Store) initPlugins() {
	p := &PluginsState{s: s, Details: map[string]*Resource[api.PluginDetail]{}}
	p.List = NewResource(s, func(ctx context.Context, c *api.Client) ([]api.PluginStatus, error) {
		return c.Plugins(ctx)
	})
	p.Catalog = NewResource(s, func(ctx context.Context, c *api.Client) (api.PluginCatalog, error) {
		return c.PluginCatalog(ctx, false)
	})
	p.Ops = NewResource(s, func(ctx context.Context, c *api.Client) ([]api.PluginOperation, error) {
		return c.PluginOperations(ctx)
	})
	p.List.settled = p.schedule
	p.Ops.settled = p.opsSettled
	p.reset()
	s.resets = append(s.resets, p.reset)
	s.plugins = p
}

func (s *Store) Plugins() *PluginsState {
	return s.plugins
}

func (p *PluginsState) reset() {
	p.stop()
	p.active = false
	p.Pending = map[string]bool{}
	p.requested = map[string]bool{}
	p.seen = nil
}

func (p *PluginsState) Detail(id string) *Resource[api.PluginDetail] {
	if r, ok := p.Details[id]; ok {
		return r
	}
	r := NewResource(p.s, func(ctx context.Context, c *api.Client) (api.PluginDetail, error) {
		return c.Plugin(ctx, id)
	})
	r.settled = p.schedule
	p.Details[id] = r
	return r
}

func (p *PluginsState) SetActive(active bool) {
	if p.active == active {
		return
	}
	p.active = active
	if !active {
		p.stop()
		return
	}
	p.List.Invalidate()
	p.Catalog.Invalidate()
	p.Ops.Invalidate()
	p.schedule()
}

func (p *PluginsState) Requested(pkg string) bool {
	return p.requested[pkg]
}

func (p *PluginsState) Uninstalling(id string) bool {
	return slices.ContainsFunc(p.Ops.Data, func(op api.PluginOperation) bool {
		return op.Kind == opKindUninstall && op.PluginID == id && opPending(op)
	})
}

func (p *PluginsState) OperationPluginID(op api.PluginOperation) string {
	if op.PluginID != "" {
		return op.PluginID
	}
	if op.Spec == "" {
		return ""
	}
	pkg := model.SpecPackage(op.Spec)
	if i := slices.IndexFunc(p.Catalog.Data.Plugins, func(c api.CatalogPlugin) bool { return c.Package == pkg }); i >= 0 {
		return p.Catalog.Data.Plugins[i].ID
	}
	if i := slices.IndexFunc(p.List.Data, func(s api.PluginStatus) bool { return s.Package != "" && s.Package == pkg }); i >= 0 {
		return p.List.Data[i].ID
	}
	return ""
}

func (p *PluginsState) RefreshCatalog(done func(error)) {
	done = orNop(done)
	p.s.async(func(ctx context.Context, c *api.Client) func() {
		ctx, cancel := context.WithTimeout(ctx, catalogRefresh)
		defer cancel()
		catalog, err := c.PluginCatalog(ctx, true)
		return func() {
			if err == nil {
				p.Catalog.Set(catalog)
			}
			done(err)
		}
	}, canceledBy(done))
}

func (p *PluginsState) Install(spec string, done func(api.PluginOperation, error)) {
	done = orNopOp(done)
	pkg := model.SpecPackage(spec)
	p.requested[pkg] = true
	p.s.async(func(ctx context.Context, c *api.Client) func() {
		op, err := c.InstallPlugin(ctx, spec)
		return func() {
			delete(p.requested, pkg)
			if err == nil {
				p.track(op)
			}
			done(op, err)
		}
	}, func() { done(api.PluginOperation{}, context.Canceled) })
}

func (p *PluginsState) Uninstall(id string, done func(api.PluginOperation, error)) {
	done = orNopOp(done)
	p.Pending[id] = true
	p.s.async(func(ctx context.Context, c *api.Client) func() {
		op, err := c.UninstallPlugin(ctx, id)
		return func() {
			delete(p.Pending, id)
			if err == nil {
				p.track(op)
			}
			done(op, err)
		}
	}, func() { done(api.PluginOperation{}, context.Canceled) })
}

func (p *PluginsState) Toggle(id string, enable bool, done func(error)) {
	p.showEnabled(id, enable)
	toast := locales.Messages.PluginsToastDisabled
	if enable {
		toast = locales.Messages.PluginsToastEnabled
	}
	p.act(id, toast, done, func(ctx context.Context, c *api.Client) error {
		if enable {
			return c.EnablePlugin(ctx, id)
		}
		return c.DisablePlugin(ctx, id)
	})
}

func (p *PluginsState) Restart(id string, done func(error)) {
	p.act(id, locales.Messages.PluginsToastRestarted, done, func(ctx context.Context, c *api.Client) error {
		return c.RestartPlugin(ctx, id)
	})
}

func (p *PluginsState) SaveSettings(id string, u api.PluginSettingsUpdate, done func(error)) {
	done = orNop(done)
	p.s.async(func(ctx context.Context, c *api.Client) func() {
		err := c.UpdatePluginSettings(ctx, id, u)
		return func() {
			if err == nil {
				p.invalidatePlugins()
			}
			done(err)
		}
	}, canceledBy(done))
}

func (p *PluginsState) act(id string, success func(locales.Messages, string) string, done func(error), call func(ctx context.Context, c *api.Client) error) {
	done = orNop(done)
	p.Pending[id] = true
	p.s.async(func(ctx context.Context, c *api.Client) func() {
		err := call(ctx, c)
		return func() {
			delete(p.Pending, id)
			p.invalidatePlugins()
			if err != nil {
				p.s.Toast(kit.Toast{Tone: kit.ToneDanger, Title: l10n.ErrorText(err)})
			} else {
				p.s.Toast(kit.Toast{Tone: kit.ToneSuccess, Title: success(p.messages(), p.name(id))})
			}
			done(err)
		}
	}, canceledBy(done))
}

func (p *PluginsState) showEnabled(id string, enabled bool) {
	if i := slices.IndexFunc(p.List.Data, func(s api.PluginStatus) bool { return s.ID == id }); i >= 0 {
		list := slices.Clone(p.List.Data)
		list[i].Enabled = enabled
		p.List.patch(list)
	}
	if r, ok := p.Details[id]; ok && r.Loaded {
		d := r.Data
		d.Enabled = enabled
		r.patch(d)
	}
}

func (p *PluginsState) track(op api.PluginOperation) {
	p.Ops.patch(append([]api.PluginOperation{op}, p.Ops.Data...))
	p.observe()
	p.Ops.load()
}

func (p *PluginsState) invalidatePlugins() {
	p.List.Invalidate()
	for _, r := range p.Details {
		r.Invalidate()
	}
}

func (p *PluginsState) opsSettled() {
	if p.Ops.Loaded {
		p.observe()
	}
	p.schedule()
}

func (p *PluginsState) observe() {
	previous := p.seen
	p.seen = map[string]bool{}
	var finished []api.PluginOperation
	for _, op := range p.Ops.Data {
		switch {
		case opPending(op):
			p.seen[op.ID] = true
		case previous[op.ID]:
			finished = append(finished, op)
		}
	}
	if previous == nil || len(finished) == 0 {
		return
	}
	for _, op := range finished {
		p.s.Toast(p.finishedToast(op))
	}
	p.invalidatePlugins()
	p.Catalog.Invalidate()
}

func (p *PluginsState) finishedToast(op api.PluginOperation) kit.Toast {
	msgs := p.messages()
	pkg := model.SpecPackage(op.Spec)
	id := p.OperationPluginID(op)
	name := p.name(id)
	if name == "" {
		name = pkg
	}
	succeeded := op.Status == opSucceeded
	switch {
	case op.Kind == opKindInstall && succeeded && p.catalogInstalled(id):
		return kit.Toast{Tone: kit.ToneSuccess, Title: msgs.PluginsUpdateDone(name, specVersion(op.Spec, pkg))}
	case op.Kind == opKindInstall && succeeded:
		return kit.Toast{Tone: kit.ToneSuccess, Title: msgs.PluginsInstallDone(name)}
	case op.Kind == opKindInstall:
		return kit.Toast{Tone: kit.ToneDanger, Title: msgs.PluginsInstallFailed(name, op.Error)}
	case succeeded:
		return kit.Toast{Tone: kit.ToneSuccess, Title: msgs.PluginsUninstallDone(name)}
	}
	return kit.Toast{Tone: kit.ToneDanger, Title: msgs.PluginsUninstallFailed(name, op.Error)}
}

func specVersion(spec, pkg string) string {
	if len(spec) > len(pkg) {
		return spec[len(pkg)+1:]
	}
	return ""
}

func (p *PluginsState) catalogInstalled(id string) bool {
	return id != "" && slices.ContainsFunc(p.Catalog.Data.Plugins, func(c api.CatalogPlugin) bool { return c.ID == id && c.Installed })
}

func (p *PluginsState) name(id string) string {
	return model.PluginName(id, &p.Catalog.Data, p.List.Data, p.s.language())
}

func (p *PluginsState) messages() locales.Messages {
	return locales.In(p.s.language())
}

func (p *PluginsState) schedule() {
	if !p.active {
		return
	}
	if p.stopOps == nil && settled(p.Ops) && model.OperationsPending(p.Ops.Data) {
		p.stopOps = p.s.after(opsPollEvery, func() {
			p.stopOps = nil
			p.Ops.Invalidate()
		})
	}
	if p.stopList == nil && p.pluginsTransitional() {
		p.stopList = p.s.after(pluginsPollEvery, p.pollPlugins)
	}
}

func (p *PluginsState) pluginsTransitional() bool {
	if settled(p.List) && model.PluginNeedsPolling(p.List.Data) {
		return true
	}
	for _, r := range p.Details {
		if settled(r) && detailTransitional(r) {
			return true
		}
	}
	return false
}

func (p *PluginsState) pollPlugins() {
	p.stopList = nil
	if model.PluginNeedsPolling(p.List.Data) {
		p.List.Invalidate()
	}
	for _, r := range p.Details {
		if detailTransitional(r) {
			r.Invalidate()
		}
	}
}

func (p *PluginsState) stop() {
	for _, stop := range []func(){p.stopOps, p.stopList} {
		if stop != nil {
			stop()
		}
	}
	p.stopOps, p.stopList = nil, nil
}

func detailTransitional(r *Resource[api.PluginDetail]) bool {
	return r.Loaded && model.PluginNeedsPolling([]api.PluginStatus{r.Data.PluginStatus})
}

func settled[T any](r *Resource[T]) bool {
	return r.Loaded && !r.Loading
}

func opPending(op api.PluginOperation) bool {
	return model.OperationsPending([]api.PluginOperation{op})
}

func orNopOp(done func(api.PluginOperation, error)) func(api.PluginOperation, error) {
	if done == nil {
		return func(api.PluginOperation, error) {}
	}
	return done
}

func (s *Store) after(d time.Duration, fn func()) func() {
	sess := s.sess
	at := s.o.Now().Add(d)
	ctx, cancel := context.WithCancel(sess.ctx)
	go func() {
		if s.o.Sleep(ctx, at.Sub(s.o.Now()), nil) != nil {
			return
		}
		s.o.Post(func() {
			if ctx.Err() != nil || s.sess != sess {
				return
			}
			cancel()
			fn()
		})
	}()
	return cancel
}
