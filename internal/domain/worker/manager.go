package worker

import (
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
) *Manager {
	rawBotNames := bc.Platforms.BotNames()
	botNames := make([]string, len(rawBotNames))
	for i, n := range rawBotNames {
		botNames[i] = strings.ToLower(strings.TrimSpace(n))
	}
	return &Manager{
		workerBaseDir:   workerBaseDir,
		tokenSecret:     bc.MCP.TokenSecret,
		tokenTTL:        bc.MCP.TokenTTL,
		workerTimeout:   bc.WorkerTimeout(),
		workerStore:     ws,
		executionStore:  es,
		engines:         engines,
		engineCfg:       engineCfg,
		envService:      envService,
		botNamesLower:   botNames,
		activeProcesses: make(map[string]ai.Process),
	}
}

func (m *Manager) resolveEngine(w model.Worker) (string, ai.EngineAdapter) {
	if w.Engine != "" {
		if e, ok := m.engines[w.Engine]; ok {
			return w.Engine, e
		}
		log.Error("unknown engine on worker, falling back to default",
			zap.String("worker_id", w.ID), zap.String("engine", w.Engine))
	}
	defaultEngine := m.engineCfg.Get()
	return defaultEngine, m.engines[defaultEngine]
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
