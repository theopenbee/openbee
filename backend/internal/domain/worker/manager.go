package worker

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"maps"
	"slices"
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

func (m *Manager) Engines() []ai.EngineInfo {
	return m.engines.Engines()
}

func (m *Manager) DefaultEngine() string {
	return m.engineCfg.Get()
}

func (m *Manager) ModelSettings(ctx context.Context) (defaults, bee map[string]string) {
	return m.loadEngineModels(ctx, model.SystemConfigKeyEngineModelsGlobal),
		m.loadEngineModels(ctx, model.SystemConfigKeyEngineModelsBee)
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

func (m *Manager) ValidateEngineModels(raw map[string]string) error {
	for _, engine := range slices.Sorted(maps.Keys(raw)) {
		if engine == "" {
			return fmt.Errorf("engine_models contains an empty engine name: %w", ErrValidation)
		}
		if err := m.ValidateEngine(engine); err != nil {
			return fmt.Errorf("engine_models[%q]: %w: %w", engine, err, ErrValidation)
		}
		if err := ai.ValidateModel(raw[engine]); err != nil {
			return fmt.Errorf("engine_models[%q]: %w: %w", engine, err, ErrValidation)
		}
	}
	return nil
}

func (m *Manager) resolveModel(ctx context.Context, worker model.Worker, engineName string) string {
	return ai.ResolveEngineModel(engineName,
		ai.ParseEngineModelsJSON(worker.EngineModels),
		m.loadEngineModels(ctx, model.SystemConfigKeyEngineModelsGlobal),
	)
}

func (m *Manager) loadEngineModels(ctx context.Context, key string) map[string]string {
	if m.sysConfigStore == nil {
		return nil
	}
	cfg, found, err := m.sysConfigStore.Get(ctx, key)
	if err != nil || !found {
		return nil
	}
	return ai.ParseEngineModelsJSON(cfg.Value)
}

func (m *Manager) EngineModels(ctx context.Context, engineID, workerID string) ([]ai.ModelInfo, error) {
	engine, ok := m.engines.Engine(engineID)
	if !ok {
		return nil, ai.ErrEngineUnavailable(engineID)
	}
	extraEnv, err := m.modelsEnv(workerID)
	if err != nil {
		return nil, err
	}
	lister, ok := engine.(ai.ModelLister)
	if !ok {
		return []ai.ModelInfo{}, nil
	}
	models, err := lister.Models(ctx, extraEnv)
	if err != nil {
		return nil, &EngineModelsError{Err: err}
	}
	if models == nil {
		models = []ai.ModelInfo{}
	}
	return models, nil
}

func (m *Manager) modelsEnv(workerID string) ([]string, error) {
	if workerID == "" {
		return m.envService.ResolveGlobalEnv()
	}
	if _, err := m.workerStore.GetByID(workerID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get worker: %w", err)
	}
	return m.envService.ResolveWorkerEnv(workerID)
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
