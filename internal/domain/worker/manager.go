package worker

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/theopenbee/openbee/internal/bridge"
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
	bridge         bridge.Bridge
	botNamesLower  []string

	activeProcesses map[string]bridge.ProcessHandle // execution_id -> process
	mu              sync.RWMutex
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
		workerBaseDir:   workerBaseDir,
		workerTimeout:   bc.WorkerTimeout(),
		workerStore:     ws,
		executionStore:  es,
		bridge:          br,
		botNamesLower:   botNames,
		activeProcesses: make(map[string]bridge.ProcessHandle),
	}
}

// resolveEngineSelection returns the resolved engine name for w, falling back
// to the configured default if w.Engine is empty or unknown.
func (m *Manager) resolveEngineSelection(w model.Worker) (string, error) {
	engineName, err := m.bridge.ResolveEngine(w.Engine)
	if err != nil {
		return "", err
	}
	if w.Engine != "" && w.Engine != engineName {
		log.Error("unknown engine on worker, falling back to default",
			zap.String("worker_id", w.ID), zap.String("engine", w.Engine))
	}
	return engineName, nil
}

func (m *Manager) resolveEngine(w model.Worker) (string, error) {
	return m.resolveEngineSelection(w)
}

func (m *Manager) EnabledEngines() []string {
	return m.bridge.EnabledEngines()
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
	if _, err := bridge.ParseEngineArgs(raw); err != nil {
		return fmt.Errorf("invalid engine_args: %w", err)
	}
	return nil
}

// An empty name is accepted (means "use server default").
func (m *Manager) ValidateEngine(name string) error {
	return m.bridge.ValidateEngine(name)
}
