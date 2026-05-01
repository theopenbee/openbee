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
	engines        map[string]ai.EngineAdapter
	engineCfg      *enginecfg.Store
	envService     *env.Service
	sysConfigStore systemConfigReader
	botNamesLower  []string

	activeProcesses map[string]ai.Process // execution_id -> process
	mu              sync.RWMutex
}

func NewManager(
	workerBaseDir string,
	bc config.BeeConfig,
	ws *store.WorkerStore,
	es *store.ExecutionStore,
	engines map[string]ai.EngineAdapter,
	engineCfg *enginecfg.Store,
	envService *env.Service,
	sysConfigStore systemConfigReader,
) *Manager {
	rawBotNames := bc.Platforms.BotNames()
	botNames := make([]string, len(rawBotNames))
	for i, n := range rawBotNames {
		botNames[i] = strings.ToLower(strings.TrimSpace(n))
	}
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
		botNamesLower:   botNames,
		activeProcesses: make(map[string]ai.Process),
	}
}

// resolveEngineSelection returns the engine name/adapter pair for w, falling
// back to the configured default if w.Engine is empty or unknown.
func (m *Manager) resolveEngineSelection(w model.Worker) (string, ai.EngineAdapter, error) {
	if w.Engine != "" {
		if e, ok := m.engines[w.Engine]; ok {
			return w.Engine, e, nil
		}
		log.Error("unknown engine on worker, falling back to default",
			zap.String("worker_id", w.ID), zap.String("engine", w.Engine))
	}
	defaultEngine := m.engineCfg.Get()
	if e, ok := m.engines[defaultEngine]; ok {
		return defaultEngine, e, nil
	}
	return "", nil, fmt.Errorf("no engine adapter found (worker engine %q, default %q)", w.Engine, defaultEngine)
}

func (m *Manager) resolveEngine(w model.Worker) (string, ai.EngineAdapter) {
	name, engine, _ := m.resolveEngineSelection(w)
	return name, engine
}

func (m *Manager) EnabledEngines() []string {
	enabled := make([]string, 0, len(m.engines))
	for _, name := range ai.AllEngines() {
		if _, ok := m.engines[name]; ok {
			enabled = append(enabled, name)
		}
	}
	return enabled
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
	if _, ok := m.engines[name]; !ok {
		return fmt.Errorf("engine %q is not enabled", name)
	}
	return nil
}
