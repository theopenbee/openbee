package pluginmgr

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"sort"

	"go.uber.org/zap"

	ai "github.com/theopenbee/openbee/internal/ai"
	"github.com/theopenbee/openbee/internal/ai/pluginengine"
	"github.com/theopenbee/openbee/internal/apperr"
	"github.com/theopenbee/openbee/internal/plugin"
)

var _ ai.EngineSource = (*Manager)(nil)

type engineInstance struct {
	gen    int
	name   plugin.LocalizedText
	runner EngineRunner
	cancel context.CancelFunc
	state  State
	err    string
}

func (m *Manager) LoadEngines(ctx context.Context) {
	m.mu.Lock()
	m.engineCtx = ctx
	m.mu.Unlock()

	entries, err := m.deps.Registry.List()
	if err != nil {
		log.Error("list installed plugins", zap.Error(err))
		return
	}
	for _, e := range entries {
		if !m.isEngine(e) {
			continue
		}
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
			log.Error("load engine", zap.String("plugin", e.ID), zap.Error(err))
		}
		unlock()
	}
}

func (m *Manager) Engine(id string) (ai.EngineAdapter, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	rec := m.engines[id]
	if rec == nil {
		return nil, false
	}
	return rec.runner, true
}

func (m *Manager) Engines() []ai.EngineInfo {
	m.mu.Lock()
	out := make([]ai.EngineInfo, 0, len(m.engines))
	for id, rec := range m.engines {
		out = append(out, ai.EngineInfo{ID: id, Name: maps.Clone(rec.name)})
	}
	m.mu.Unlock()
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (m *Manager) EnabledEngineIDs() []string {
	m.mu.Lock()
	ids := make([]string, 0, len(m.engines))
	for id := range m.engines {
		ids = append(ids, id)
	}
	m.mu.Unlock()
	sort.Strings(ids)
	return ids
}

func (m *Manager) isEngine(e plugin.Entry) bool {
	man, err := m.manifest(e)
	return err == nil && man.Kind() == plugin.TypeEngine
}

func (m *Manager) launchEngine(id string, man plugin.Manifest, cfg map[string]any, dataDir string) error {
	if m.deps.EngineFactory == nil {
		return errors.New("engine plugins are not supported")
	}
	stateDir := ""
	if m.deps.StateDir != nil {
		stateDir = m.deps.StateDir(id)
		if err := os.MkdirAll(stateDir, 0o700); err != nil {
			return fmt.Errorf("create engine state dir: %w", err)
		}
	}
	runner := m.deps.EngineFactory(pluginengine.Spec{
		ID:             id,
		Dir:            man.Dir,
		Command:        man.Command,
		Config:         cfg,
		DataDir:        dataDir,
		StateDir:       stateDir,
		Language:       m.deps.Language,
		OpenbeeVersion: m.deps.Version,
	})
	name := man.Name
	if len(name) == 0 {
		name = plugin.LocalizedText{"en": id}
	}

	m.mu.Lock()
	base := m.rootCtx
	if base == nil {
		base = m.engineCtx
	}
	if base == nil || base.Err() != nil {
		m.mu.Unlock()
		return errors.New("plugin manager is not running")
	}
	m.gen++
	gen := m.gen
	checkCtx, cancel := context.WithCancel(base)
	m.engines[id] = &engineInstance{gen: gen, name: name, runner: runner, cancel: cancel, state: StateStarting}
	m.wg.Add(1)
	m.mu.Unlock()

	go func() {
		defer m.wg.Done()
		defer cancel()
		res, err := runner.Check(checkCtx)
		m.onEngineCheck(id, gen, res, err)
	}()
	log.Info("engine registered", zap.String("plugin", id))
	return nil
}

func (m *Manager) onEngineCheck(id string, gen int, res pluginengine.CheckResult, err error) {
	m.mu.Lock()
	rec := m.engines[id]
	current := rec != nil && rec.gen == gen
	if current {
		if err != nil {
			rec.state, rec.err = StateFailed, err.Error()
		} else {
			rec.state, rec.err = StateReady, ""
		}
	}
	m.mu.Unlock()
	if !current {
		return
	}
	if err != nil {
		log.Warn("engine check failed", zap.String("plugin", id), zap.Error(err))
		return
	}
	log.Info("engine ready", zap.String("plugin", id), zap.String("version", res.Version), zap.String("path", res.Path))
}

func (m *Manager) saveEngineDefaults(ctx context.Context, id string, man plugin.Manifest) error {
	cfg, err := MergeConfig(man, nil, nil)
	if err != nil {
		return errNotConfigured(id, err)
	}
	row, err := m.deps.Codec.Encode(Settings{Enabled: true, Config: cfg}, id)
	if err != nil {
		return err
	}
	return m.deps.Store.Upsert(ctx, row)
}

func (m *Manager) enableNewEngine(ctx context.Context, e plugin.Entry) {
	man, err := m.manifest(e)
	if err != nil || man.Kind() != plugin.TypeEngine {
		return
	}
	if err := m.saveEngineDefaults(ctx, e.ID, man); err != nil {
		if apperr.Code(err) != CodeNotConfigured {
			log.Error("enable engine after install", zap.String("plugin", e.ID), zap.Error(err))
		}
		return
	}
	if err := m.startLocked(ctx, e.ID); err != nil {
		log.Error("start engine after install", zap.String("plugin", e.ID), zap.Error(err))
	}
}
