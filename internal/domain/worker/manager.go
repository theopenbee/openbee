package worker

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/theopenbee/openbee/internal/ai/bridge"
	"github.com/theopenbee/openbee/internal/infra/config"
	"github.com/theopenbee/openbee/internal/infra/logger"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/infra/store"
	"go.uber.org/zap"
)

var log = logger.With(zap.String("component", "worker"))

type Manager struct {
	workerBaseDir  string
	workerTimeout  time.Duration
	workerStore    *store.WorkerStore
	executionStore *store.ExecutionStore
	br             bridge.Bridge
	botNamesLower  []string

	activeHandles map[string]bridge.Handle // execution_id -> handle
	mu            sync.RWMutex
}

func NewManager(
	workerBaseDir string,
	bc config.BeeConfig,
	ws *store.WorkerStore,
	es *store.ExecutionStore,
	br bridge.Bridge,
) *Manager {
	rawBotNames := bc.Platforms.BotNames()
	botNames := make([]string, len(rawBotNames))
	for i, n := range rawBotNames {
		botNames[i] = strings.ToLower(strings.TrimSpace(n))
	}
	return &Manager{
		workerBaseDir:  workerBaseDir,
		workerTimeout:  bc.WorkerTimeout(),
		workerStore:    ws,
		executionStore: es,
		br:             br,
		botNamesLower:  botNames,
		activeHandles:  make(map[string]bridge.Handle),
	}
}

func (m *Manager) resolveEngineForWorker(w model.Worker) string {
	return m.br.ResolveEngineForWorker(w.ID, w.Engine)
}

func (m *Manager) EnabledEngines() []string {
	return m.br.EnabledEngines()
}

func (m *Manager) ValidateEngine(name string) error {
	return m.br.ValidateEngine(name)
}

func (m *Manager) ValidateEngineArgs(raw map[string]string) error {
	if len(raw) == 0 {
		return nil
	}
	for engine, line := range raw {
		if engine == "" {
			return fmt.Errorf("engine_args contains an empty engine name: %w", ErrValidation)
		}
		if err := m.br.ValidateEngine(engine); err != nil {
			return fmt.Errorf("engine_args[%q]: %w", engine, err)
		}
		if err := m.br.ValidateEngineArgs(line); err != nil {
			return fmt.Errorf("engine_args[%q]: %w", engine, err)
		}
	}
	return nil
}
