package worker

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	ai "github.com/theopenbee/openbee/internal/ai"
	"github.com/theopenbee/openbee/internal/domain/enginecfg"
	"github.com/theopenbee/openbee/internal/domain/env"
	"github.com/theopenbee/openbee/internal/infra/config"
	"github.com/theopenbee/openbee/internal/infra/logger"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/infra/store"
	"go.uber.org/zap"
)

type systemConfigReader interface {
	Get(ctx context.Context, key string) (model.SystemConfig, bool, error)
}

var log = logger.With(zap.String("component", "worker"))

type Manager struct {
	workerBaseDir  string
	tokenSecret    string
	tokenTTL       time.Duration
	workerTimeout  time.Duration
	workerStore    *store.WorkerStore
	executionStore *store.ExecutionStore
	engines        ai.EngineSource
	engineCfg      *enginecfg.Store
	envService     *env.Service
	sysConfigStore systemConfigReader
	botNames       func() []string
	statusObserver func(workerID string, status model.WorkerStatus)

	activeProcesses map[string]ai.Process // execution_id -> process
	monitorDone     map[string]chan struct{}
	mu              sync.RWMutex
}

func NewManager(
	workerBaseDir string,
	bc config.BeeConfig,
	ws *store.WorkerStore,
	es *store.ExecutionStore,
	engines ai.EngineSource,
	engineCfg *enginecfg.Store,
	envService *env.Service,
	sysConfigStore systemConfigReader,
) *Manager {
	return &Manager{
		workerBaseDir:   workerBaseDir,
		tokenSecret:     bc.RPC.TokenSecret,
		tokenTTL:        bc.RPC.TokenTTL,
		workerTimeout:   bc.WorkerTimeout(),
		workerStore:     ws,
		executionStore:  es,
		engines:         engines,
		engineCfg:       engineCfg,
		envService:      envService,
		sysConfigStore:  sysConfigStore,
		activeProcesses: make(map[string]ai.Process),
		monitorDone:     make(map[string]chan struct{}),
	}
}

func (m *Manager) resolveEngine(w model.Worker) (string, ai.EngineAdapter, error) {
	name := m.engineCfg.Resolve(w.Engine)
	if name == "" {
		return "", nil, ai.ErrNoEngine
	}
	e, ok := m.engines.Engine(name)
	if !ok {
		return name, nil, ai.ErrEngineUnavailable(name)
	}
	return name, e, nil
}

func (m *Manager) EnabledEngines() []string {
	return ai.EngineIDs(m.engines)
}

func (m *Manager) loadEngineArgs(ctx context.Context, key string) ai.EngineArgsMap {
	if m.sysConfigStore == nil {
		return nil
	}
	cfg, found, err := m.sysConfigStore.Get(ctx, key)
	if err != nil || !found {
		return nil
	}
	return ai.ParseEngineArgsJSON(cfg.Value)
}

func (m *Manager) resolveEngineArgs(ctx context.Context, worker model.Worker, engineName string) []string {
	globalMap := m.loadEngineArgs(ctx, model.SystemConfigKeyEngineArgsGlobal)
	workerMap := ai.ParseEngineArgsJSON(worker.EngineArgs)
	merged := ai.MergeEngineArgs(globalMap, workerMap)
	return merged[engineName]
}

func (m *Manager) ValidateEngineArgs(raw map[string]string) error {
	if len(raw) == 0 {
		return nil
	}
	for engine := range raw {
		if engine == "" {
			return fmt.Errorf("engine_args contains an empty engine name: %w", ErrValidation)
		}
		if err := m.ValidateEngine(engine); err != nil {
			return fmt.Errorf("engine_args[%q]: %w", engine, err)
		}
	}
	if _, err := ai.ParseEngineArgs(raw); err != nil {
		return fmt.Errorf("invalid engine_args: %w", err)
	}
	return nil
}

// An empty name is accepted (means "use server default").
func (m *Manager) ValidateEngine(name string) error {
	if name == "" {
		return nil
	}
	if _, ok := m.engines.Engine(name); !ok {
		return ai.ErrEngineUnavailable(name)
	}
	return nil
}

func (m *Manager) SetBotNameSource(fn func() []string) {
	m.mu.Lock()
	m.botNames = fn
	m.mu.Unlock()
}

func (m *Manager) SetStatusObserver(fn func(workerID string, status model.WorkerStatus)) {
	m.mu.Lock()
	m.statusObserver = fn
	m.mu.Unlock()
}

func (m *Manager) setStatus(workerID string, status model.WorkerStatus) error {
	if err := m.workerStore.UpdateStatus(workerID, status); err != nil {
		return err
	}
	m.mu.RLock()
	fn := m.statusObserver
	m.mu.RUnlock()
	if fn != nil {
		fn(workerID, status)
	}
	return nil
}

func (m *Manager) conflictsWithBotName(name string) bool {
	m.mu.RLock()
	fn := m.botNames
	m.mu.RUnlock()
	if fn == nil {
		return false
	}
	lower := strings.ToLower(strings.TrimSpace(name))
	for _, n := range fn() {
		if strings.ToLower(strings.TrimSpace(n)) == lower {
			return true
		}
	}
	return false
}
