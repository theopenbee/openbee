package pluginmgr

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/theopenbee/openbee/internal/infra/logger"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/platform"
	"github.com/theopenbee/openbee/internal/platform/pluginchannel"
	"github.com/theopenbee/openbee/internal/plugin"
)

var log = logger.With(zap.String("component", "pluginmgr"))

const stopTimeout = 30 * time.Second

type State string

const (
	StateNotConfigured State = "not_configured"
	StateDisabled      State = "disabled"
	StateStarting      State = "starting"
	StateRunning       State = "running"
	StateRetrying      State = "retrying"
	StateFailed        State = "failed"
)

type Registry interface {
	List() ([]plugin.Entry, error)
	Get(id string) (plugin.Entry, bool, error)
	DataDir(id string) string
}

type Installer interface {
	Install(ctx context.Context, spec string) (plugin.Entry, plugin.Manifest, error)
	Remove(ctx context.Context, id string) (plugin.Entry, error)
}

type SettingsStore interface {
	Get(ctx context.Context, id string) (model.PluginSettings, bool, error)
	List(ctx context.Context) ([]model.PluginSettings, error)
	Upsert(ctx context.Context, p model.PluginSettings) error
	Delete(ctx context.Context, id string) error
}

type Deps struct {
	Registry        Registry
	Installer       Installer
	Store           SettingsStore
	Codec           *Codec
	Senders         *platform.SenderRegistry
	WrapSender      func(id string, s platform.PlatformSenderAdapter) platform.PlatformSenderAdapter
	Factory         Factory
	WorkerNameTaken func(name string) (bool, error)
	Catalog         CatalogSource
	Version         string
}

type Status struct {
	ID          string
	Package     string
	Source      string
	Version     string
	Path        string
	Name        plugin.LocalizedText
	Description plugin.LocalizedText
	Enabled     bool
	BotName     string
	State       State
	Error       string
	HasSetup    bool
}

type Detail struct {
	Status
	Config     map[string]any
	Fields     []plugin.ConfigField
	SetupLabel plugin.LocalizedText
}

type instance struct {
	gen    int
	cancel context.CancelFunc
	done   chan struct{}
	state  State
	err    string
}

type Manager struct {
	deps Deps

	mu         sync.Mutex
	rootCtx    context.Context
	dispatch   func(platform.InboundMessage)
	onBotNames func(map[string]string)
	instances  map[string]*instance
	startErrs  map[string]string
	locks      map[string]*sync.Mutex
	botNames   map[string]string
	gen        int
	wg         sync.WaitGroup

	pkgMu sync.Mutex
	ops   []*Operation
	opsWG sync.WaitGroup
}

func New(d Deps) *Manager {
	return &Manager{
		deps:      d,
		instances: make(map[string]*instance),
		startErrs: make(map[string]string),
		locks:     make(map[string]*sync.Mutex),
		botNames:  make(map[string]string),
	}
}

func (m *Manager) Start(ctx context.Context, dispatch func(platform.InboundMessage), onBotNames func(map[string]string)) {
	m.mu.Lock()
	m.rootCtx = ctx
	m.dispatch = dispatch
	m.onBotNames = onBotNames
	m.mu.Unlock()
	m.refreshBotNames(ctx)

	entries, err := m.deps.Registry.List()
	if err != nil {
		log.Error("list installed plugins", zap.Error(err))
		return
	}
	for _, e := range entries {
		row, found, err := m.deps.Store.Get(ctx, e.ID)
		if err != nil {
			log.Error("read plugin settings", zap.String("plugin", e.ID), zap.Error(err))
			continue
		}
		if !found || !row.Enabled {
			continue
		}
		unlock := m.lockID(e.ID)
		if err := m.startLocked(ctx, e.ID); err != nil {
			log.Error("start plugin", zap.String("plugin", e.ID), zap.Error(err))
		}
		unlock()
	}
}

func (m *Manager) Wait(ctx context.Context) error {
	done := make(chan struct{})
	go func() {
		m.wg.Wait()
		m.opsWG.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (m *Manager) lockID(id string) func() {
	m.mu.Lock()
	l := m.locks[id]
	if l == nil {
		l = &sync.Mutex{}
		m.locks[id] = l
	}
	m.mu.Unlock()
	l.Lock()
	return l.Unlock
}

func (m *Manager) entry(id string) (plugin.Entry, error) {
	e, ok, err := m.deps.Registry.Get(id)
	if err != nil {
		return plugin.Entry{}, err
	}
	if !ok {
		return plugin.Entry{}, errNotFound(id)
	}
	return e, nil
}

func (m *Manager) manifest(e plugin.Entry) (plugin.Manifest, error) {
	man, err := plugin.LoadManifest(e.Path)
	if err != nil {
		return plugin.Manifest{}, err
	}
	if man.ID != e.ID {
		return plugin.Manifest{}, fmt.Errorf("plugin at %s declares id %q, expected %q", e.Path, man.ID, e.ID)
	}
	return man, nil
}

func isDone(ch chan struct{}) bool {
	select {
	case <-ch:
		return true
	default:
		return false
	}
}

func (m *Manager) startLocked(ctx context.Context, id string) error {
	m.mu.Lock()
	rec := m.instances[id]
	m.mu.Unlock()
	if rec != nil {
		if !isDone(rec.done) {
			return nil
		}
		m.stopLocked(id)
	}
	err := m.launch(ctx, id)
	m.mu.Lock()
	if err != nil {
		m.startErrs[id] = err.Error()
	} else {
		delete(m.startErrs, id)
	}
	m.mu.Unlock()
	return err
}

func (m *Manager) launch(ctx context.Context, id string) error {
	e, err := m.entry(id)
	if err != nil {
		return err
	}
	man, err := m.manifest(e)
	if err != nil {
		return err
	}
	row, found, err := m.deps.Store.Get(ctx, id)
	if err != nil {
		return err
	}
	if !found {
		return errNotConfigured(id, errors.New("no settings saved"))
	}
	s, err := m.deps.Codec.Decode(row)
	if err != nil {
		return err
	}
	cfg, err := plugin.ResolveConfig(man, s.Config)
	if err != nil {
		return errNotConfigured(id, err)
	}
	dataDir := m.deps.Registry.DataDir(id)
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return fmt.Errorf("create plugin data dir: %w", err)
	}
	if err := os.Chmod(dataDir, 0o700); err != nil {
		return fmt.Errorf("secure plugin data dir: %w", err)
	}

	m.mu.Lock()
	rootCtx, dispatch := m.rootCtx, m.dispatch
	if rootCtx == nil {
		m.mu.Unlock()
		return errors.New("plugin manager is not started")
	}
	m.gen++
	gen := m.gen
	m.mu.Unlock()

	inst := m.deps.Factory(InstanceSpec{
		ID:       id,
		Manifest: man,
		Config:   cfg,
		BotName:  s.BotName,
		DataDir:  dataDir,
		OnState:  func(st pluginchannel.State, err error) { m.onState(id, gen, st, err) },
	})
	runCtx, cancel := context.WithCancel(rootCtx)
	rec := &instance{gen: gen, cancel: cancel, done: make(chan struct{}), state: StateStarting}
	m.mu.Lock()
	m.instances[id] = rec
	m.mu.Unlock()

	sender := inst.Sender()
	if m.deps.WrapSender != nil {
		sender = m.deps.WrapSender(id, sender)
	}
	m.deps.Senders.Set(id, sender)
	platform.RegisterExtractor(id, pluginchannel.ExtractContextFor(id))

	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		defer close(rec.done)
		inst.Run(runCtx, dispatch)
	}()
	log.Info("plugin started", zap.String("plugin", id))
	return nil
}

func (m *Manager) onState(id string, gen int, st pluginchannel.State, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	rec := m.instances[id]
	if rec == nil || rec.gen != gen {
		return
	}
	switch st {
	case pluginchannel.StateStarting:
		rec.state = StateStarting
	case pluginchannel.StateRunning:
		rec.state = StateRunning
		rec.err = ""
	case pluginchannel.StateRetrying:
		rec.state = StateRetrying
		rec.err = errText(err)
	case pluginchannel.StateFailed:
		rec.state = StateFailed
		rec.err = errText(err)
	}
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func (m *Manager) stopLocked(id string) {
	m.mu.Lock()
	rec := m.instances[id]
	delete(m.instances, id)
	delete(m.startErrs, id)
	m.mu.Unlock()
	m.deps.Senders.Delete(id)
	if rec == nil {
		return
	}
	rec.cancel()
	select {
	case <-rec.done:
	case <-time.After(stopTimeout):
		log.Warn("plugin did not stop in time", zap.String("plugin", id))
	}
	log.Info("plugin stopped", zap.String("plugin", id))
}

func (m *Manager) UpdateSettings(ctx context.Context, id string, botName *string, update map[string]any) error {
	unlock := m.lockID(id)
	defer unlock()
	e, err := m.entry(id)
	if err != nil {
		return err
	}
	man, err := m.manifest(e)
	if err != nil {
		return err
	}
	cur := Settings{Config: map[string]any{}}
	row, found, err := m.deps.Store.Get(ctx, id)
	if err != nil {
		return err
	}
	if found {
		cur.Enabled, cur.BotName = row.Enabled, row.BotName
		if s, err := m.deps.Codec.Decode(row); err == nil {
			cur.Config = s.Config
		}
	}
	merged, err := MergeConfig(man, cur.Config, update)
	if err != nil {
		return err
	}
	next := Settings{Enabled: cur.Enabled, BotName: cur.BotName, Config: merged}
	if botName != nil {
		next.BotName = strings.TrimSpace(*botName)
	}
	if next.BotName != "" && next.BotName != cur.BotName && m.deps.WorkerNameTaken != nil {
		taken, err := m.deps.WorkerNameTaken(next.BotName)
		if err != nil {
			return err
		}
		if taken {
			return errBotNameConflict(next.BotName)
		}
	}
	enc, err := m.deps.Codec.Encode(next, id)
	if err != nil {
		return err
	}
	if err := m.deps.Store.Upsert(ctx, enc); err != nil {
		return err
	}
	m.refreshBotNames(ctx)
	if !next.Enabled {
		return nil
	}
	m.stopLocked(id)
	return m.startLocked(ctx, id)
}

func (m *Manager) Enable(ctx context.Context, id string) error {
	unlock := m.lockID(id)
	defer unlock()
	e, err := m.entry(id)
	if err != nil {
		return err
	}
	man, err := m.manifest(e)
	if err != nil {
		return err
	}
	row, found, err := m.deps.Store.Get(ctx, id)
	if err != nil {
		return err
	}
	if !found {
		return errNotConfigured(id, errors.New("no settings saved"))
	}
	s, err := m.deps.Codec.Decode(row)
	if err != nil {
		return err
	}
	if _, err := plugin.ResolveConfig(man, s.Config); err != nil {
		return errNotConfigured(id, err)
	}
	if !row.Enabled {
		row.Enabled = true
		if err := m.deps.Store.Upsert(ctx, row); err != nil {
			return err
		}
	}
	return m.startLocked(ctx, id)
}

func (m *Manager) Disable(ctx context.Context, id string) error {
	unlock := m.lockID(id)
	defer unlock()
	if _, err := m.entry(id); err != nil {
		return err
	}
	row, found, err := m.deps.Store.Get(ctx, id)
	if err != nil {
		return err
	}
	if found && row.Enabled {
		row.Enabled = false
		if err := m.deps.Store.Upsert(ctx, row); err != nil {
			return err
		}
	}
	m.stopLocked(id)
	return nil
}

func (m *Manager) Restart(ctx context.Context, id string) error {
	unlock := m.lockID(id)
	defer unlock()
	if _, err := m.entry(id); err != nil {
		return err
	}
	row, found, err := m.deps.Store.Get(ctx, id)
	if err != nil {
		return err
	}
	if !found || !row.Enabled {
		return errNotConfigured(id, errors.New("plugin is disabled"))
	}
	m.stopLocked(id)
	return m.startLocked(ctx, id)
}

func (m *Manager) List(ctx context.Context) ([]Status, error) {
	entries, err := m.deps.Registry.List()
	if err != nil {
		return nil, err
	}
	rows, err := m.deps.Store.List(ctx)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]model.PluginSettings, len(rows))
	for _, r := range rows {
		byID[r.ID] = r
	}
	out := make([]Status, 0, len(entries))
	for _, e := range entries {
		row, found := byID[e.ID]
		st, _ := m.status(e, row, found)
		out = append(out, st)
	}
	return out, nil
}

func (m *Manager) Get(ctx context.Context, id string) (Detail, error) {
	e, err := m.entry(id)
	if err != nil {
		return Detail{}, err
	}
	row, found, err := m.deps.Store.Get(ctx, id)
	if err != nil {
		return Detail{}, err
	}
	st, man := m.status(e, row, found)
	d := Detail{Status: st, Config: map[string]any{}}
	if man == nil {
		return d, nil
	}
	d.Fields = man.Config
	if man.Setup != nil {
		d.SetupLabel = man.Setup.Label
	}
	if found {
		if s, err := m.deps.Codec.Decode(row); err == nil {
			d.Config = MaskConfig(*man, s.Config)
		}
	}
	return d, nil
}

func (m *Manager) status(e plugin.Entry, row model.PluginSettings, found bool) (Status, *plugin.Manifest) {
	st := Status{
		ID:      e.ID,
		Package: e.Package,
		Source:  e.Source,
		Version: e.Version,
		Path:    e.Path,
		Name:    plugin.LocalizedText{"en": e.ID},
	}
	if found {
		st.Enabled = row.Enabled
		st.BotName = row.BotName
	}
	man, merr := m.manifest(e)
	if merr == nil {
		if len(man.Name) > 0 {
			st.Name = man.Name
		}
		st.Description = man.Description
		st.HasSetup = man.Setup != nil
	}

	m.mu.Lock()
	rec := m.instances[e.ID]
	startErr := m.startErrs[e.ID]
	var recState State
	var recErr string
	if rec != nil {
		recState, recErr = rec.state, rec.err
	}
	m.mu.Unlock()

	switch {
	case rec != nil:
		st.State, st.Error = recState, recErr
	case merr != nil:
		st.State, st.Error = StateFailed, merr.Error()
		return st, nil
	case found && row.Enabled && startErr != "":
		st.State, st.Error = StateFailed, startErr
	case !found:
		st.State = StateNotConfigured
	default:
		s, err := m.deps.Codec.Decode(row)
		switch {
		case err != nil:
			st.State, st.Error = StateFailed, err.Error()
		case !configResolves(man, s.Config):
			st.State = StateNotConfigured
		case !row.Enabled:
			st.State = StateDisabled
		default:
			st.State, st.Error = StateFailed, "plugin is enabled but not running"
		}
	}
	if merr != nil {
		return st, nil
	}
	return st, &man
}

func configResolves(man plugin.Manifest, cfg map[string]any) bool {
	_, err := plugin.ResolveConfig(man, cfg)
	return err == nil
}

func (m *Manager) refreshBotNames(ctx context.Context) {
	rows, err := m.deps.Store.List(ctx)
	if err != nil {
		log.Error("list plugin settings", zap.Error(err))
		return
	}
	names := make(map[string]string)
	for _, r := range rows {
		if r.BotName != "" {
			names[r.ID] = r.BotName
		}
	}
	m.mu.Lock()
	m.botNames = names
	cb := m.onBotNames
	m.mu.Unlock()
	if cb != nil {
		cb(copyNames(names))
	}
}

func copyNames(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func (m *Manager) BotNames() map[string]string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return copyNames(m.botNames)
}

func (m *Manager) BotNameList() []string {
	m.mu.Lock()
	out := make([]string, 0, len(m.botNames))
	for _, n := range m.botNames {
		out = append(out, n)
	}
	m.mu.Unlock()
	sort.Strings(out)
	return out
}
