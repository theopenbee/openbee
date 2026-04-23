package worker

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	ai "github.com/theopenbee/openbee/internal/ai"
	"github.com/theopenbee/openbee/internal/domain/enginecfg"
	"github.com/theopenbee/openbee/internal/domain/env"
	"github.com/theopenbee/openbee/internal/infra/auth"
	"github.com/theopenbee/openbee/internal/infra/config"
	"github.com/theopenbee/openbee/internal/infra/logger"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/infra/store"
	"github.com/theopenbee/openbee/internal/infra/utils"
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
	botNames       []string

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
	var botNames []string
	for _, n := range []string{
		bc.Platforms.Feishu.BotName,
		bc.Platforms.DingTalk.BotName,
		bc.Platforms.WeCom.BotName,
		bc.Platforms.Telegram.BotName,
		bc.Platforms.Weixin.BotName,
	} {
		if n != "" {
			botNames = append(botNames, n)
		}
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
		botNames:        botNames,
		activeProcesses: make(map[string]ai.Process),
	}
}

// resolveEngine returns the EngineAdapter for w, falling back to the configured default if w.Engine is empty or unknown.
func (m *Manager) resolveEngine(w model.Worker) (ai.EngineAdapter, error) {
	if w.Engine != "" {
		if e, ok := m.engines[w.Engine]; ok {
			return e, nil
		}
		log.Error("unknown engine on worker, falling back to default",
			zap.String("worker_id", w.ID), zap.String("engine", w.Engine))
	}
	defaultEngine := m.engineCfg.Get()
	if e, ok := m.engines[defaultEngine]; ok {
		return e, nil
	}
	return nil, fmt.Errorf("no engine adapter found (worker engine %q, default %q)", w.Engine, defaultEngine)
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

// CreateWorkerParams holds the inputs for creating a new worker.
type CreateWorkerParams struct {
	Name             string
	Description      string
	Constraints      string
	WorkDir          string
	PermissionScopes string
	Engine           string
}

// UpdateWorkerParams holds the inputs for a partial worker update.
type UpdateWorkerParams struct {
	Name             *string `json:"name"`
	Description      *string `json:"description"`
	Constraints      *string `json:"constraints"`
	PermissionScopes *string `json:"permission_scopes"`
	Engine           *string `json:"engine"`
}

func (p UpdateWorkerParams) HasChanges() bool {
	return p.Name != nil || p.Description != nil || p.Constraints != nil ||
		p.PermissionScopes != nil || p.Engine != nil
}

func (p UpdateWorkerParams) Validate(m *Manager) error {
	if p.PermissionScopes != nil {
		if err := auth.ValidatePermissionScopes(*p.PermissionScopes); err != nil {
			return err
		}
	}
	if p.Engine != nil {
		return m.ValidateEngine(*p.Engine)
	}
	return nil
}

func (p UpdateWorkerParams) ApplyTo(w *model.Worker) {
	if p.Name != nil {
		w.Name = *p.Name
	}
	if p.Description != nil {
		w.Description = *p.Description
	}
	if p.Constraints != nil {
		w.Constraints = *p.Constraints
	}
	if p.PermissionScopes != nil {
		w.PermissionScopes = *p.PermissionScopes
	}
	if p.Engine != nil {
		w.Engine = *p.Engine
	}
}

func (m *Manager) validateWorkerName(name, excludeID string) error {
	name = strings.TrimSpace(name)
	lower := strings.ToLower(name)
	for _, bn := range m.botNames {
		if strings.ToLower(strings.TrimSpace(bn)) == lower {
			return fmt.Errorf("worker name %q conflicts with bot name: %w", name, ErrValidation)
		}
	}
	exists, err := m.workerStore.ExistsByName(name, excludeID)
	if err != nil {
		return fmt.Errorf("check worker name: %w", err)
	}
	if exists {
		return fmt.Errorf("worker name %q is already taken: %w", name, ErrValidation)
	}
	return nil
}

func (m *Manager) UpdateWorker(id string, p UpdateWorkerParams) (model.Worker, error) {
	w, err := m.workerStore.GetByID(id)
	if err != nil {
		return model.Worker{}, fmt.Errorf("worker not found: %w", ErrNotFound)
	}
	if err := p.Validate(m); err != nil {
		return model.Worker{}, err
	}
	if p.Name != nil && !strings.EqualFold(*p.Name, w.Name) {
		if err := m.validateWorkerName(*p.Name, id); err != nil {
			return model.Worker{}, err
		}
	}
	if !p.HasChanges() {
		return w, nil
	}
	p.ApplyTo(&w)
	return m.workerStore.Update(w)
}

func (m *Manager) CreateWorker(p CreateWorkerParams) (model.Worker, error) {
	if err := m.validateWorkerName(p.Name, ""); err != nil {
		return model.Worker{}, err
	}
	id := uuid.New().String()
	if p.WorkDir == "" {
		p.WorkDir = filepath.Join(m.workerBaseDir, id)
	}

	if err := os.MkdirAll(p.WorkDir, 0755); err != nil {
		return model.Worker{}, fmt.Errorf("create work dir: %w", err)
	}

	workerModel := model.Worker{
		ID:               id,
		Name:             p.Name,
		Description:      p.Description,
		Constraints:      p.Constraints,
		WorkDir:          p.WorkDir,
		Engine:           p.Engine,
		PermissionScopes: p.PermissionScopes,
	}
	engine, err := m.resolveEngine(workerModel)
	if err != nil {
		return model.Worker{}, err
	}
	if err := engine.Prepare(p.WorkDir, ai.PrepareOptions{Role: ai.RoleWorker}); err != nil {
		return model.Worker{}, fmt.Errorf("prepare worker workspace: %w", err)
	}

	return m.workerStore.Create(workerModel)
}

// ExecuteWorker runs a worker. When resume is true, the AI engine will attempt
// to resume the session identified by sessionID; otherwise it starts a fresh session.
// sessionID must always be non-empty; callers are responsible for generating it.
func (m *Manager) ExecuteWorker(ctx context.Context, workerID, triggerInput, sessionID string, resume bool) (model.WorkerExecution, error) {
	worker, err := m.workerStore.GetByID(workerID)
	if err != nil {
		return model.WorkerExecution{}, fmt.Errorf("get worker: %w", err)
	}

	exec, err := m.executionStore.Create(workerID, triggerInput, sessionID)
	if err != nil {
		return model.WorkerExecution{}, fmt.Errorf("create execution: %w", err)
	}

	if err := m.workerStore.UpdateStatus(worker.ID, model.WorkerStatusWorking); err != nil {
		log.Error("failed to update worker status", zap.Error(err))
	}

	engine, err := m.resolveEngine(worker)
	if err != nil {
		m.executionStore.UpdateResult(exec.ID, err.Error(), model.ExecStatusFailed)
		m.workerStore.UpdateStatus(worker.ID, model.WorkerStatusError)
		return exec, err
	}
	if err := engine.Prepare(worker.WorkDir, ai.PrepareOptions{Role: ai.RoleWorker}); err != nil {
		log.Error("prepare worker workspace", zap.String("op", "execute"), zap.Error(err))
	}
	timeout := m.workerTimeout

	if err := m.launchRuntime(exec, worker, engine, timeout, triggerInput, resume); err != nil {
		m.executionStore.UpdateResult(exec.ID, err.Error(), model.ExecStatusFailed)
		m.workerStore.UpdateStatus(worker.ID, model.WorkerStatusError)
		return exec, fmt.Errorf("start runtime: %w", err)
	}

	return exec, nil
}

// launchRuntime applies timeout, prepares the log path, starts the invoker,
// registers the process, updates PID, and launches monitoring.
func (m *Manager) launchRuntime(exec model.WorkerExecution, worker model.Worker, engine ai.EngineAdapter, timeout time.Duration, prompt string, resume bool) error {
	logPath, err := m.executionStore.PrepareLogPath(exec.ID, exec.StartedAt)
	if err != nil {
		return fmt.Errorf("prepare log path: %w", err)
	}

	token, err := auth.GenerateWorkerToken(m.tokenSecret, worker.ID, utils.SplitAndTrim(worker.PermissionScopes), m.tokenTTL)
	if err != nil {
		return fmt.Errorf("generate worker token: %w", err)
	}

	var execCtx context.Context
	var cancel context.CancelFunc
	if timeout > 0 {
		execCtx, cancel = context.WithTimeout(context.Background(), timeout)
	} else {
		execCtx, cancel = context.WithCancel(context.Background())
	}

	extraEnv, err := m.envService.ResolveWorkerEnv(worker.ID)
	if err != nil {
		cancel()
		return fmt.Errorf("resolve worker env: %w", err)
	}

	runRes, err := engine.Run(execCtx, worker.WorkDir, prompt, ai.RunOptions{
		SessionID: exec.SessionID,
		Resume:    resume,
		APIKey:    token,
		ExtraEnv:  extraEnv,
	}, logPath)
	if err != nil {
		cancel()
		return err
	}

	m.mu.Lock()
	m.activeProcesses[exec.ID] = runRes.Process
	m.mu.Unlock()

	m.executionStore.UpdatePID(exec.ID, runRes.Process.PID())
	go m.monitorExecution(exec, worker, runRes, cancel, logPath)
	return nil
}

func (m *Manager) monitorExecution(exec model.WorkerExecution, worker model.Worker, runRes ai.RunResult, cancel context.CancelFunc, logPath string) {
	defer cancel()

	for out := range runRes.Output {
		switch out.Type {
		case ai.OutputDone:
			result := runRes.ExtractResult(logPath)
			m.executionStore.UpdateResult(exec.ID, result, model.ExecStatusCompleted)
			m.workerStore.UpdateStatus(worker.ID, model.WorkerStatusIdle)
		case ai.OutputError:
			result := runRes.ExtractResult(logPath)
			if result == "" {
				result = out.Content
			}
			m.executionStore.UpdateResult(exec.ID, result, model.ExecStatusFailed)
			m.workerStore.UpdateStatus(worker.ID, model.WorkerStatusError)
		}
	}

	m.mu.Lock()
	delete(m.activeProcesses, exec.ID)
	m.mu.Unlock()
}

func (m *Manager) DeleteWorker(id string, deleteWorkDir bool) error {
	if deleteWorkDir {
		worker, err := m.workerStore.GetByID(id)
		if err != nil {
			return fmt.Errorf("get worker: %w", err)
		}
		if worker.WorkDir != "" {
			if err := os.RemoveAll(worker.WorkDir); err != nil {
				return fmt.Errorf("remove work dir: %w", err)
			}
		}
	}
	return m.workerStore.Delete(id)
}

func (m *Manager) StopExecution(executionID string) error {
	m.mu.RLock()
	proc, ok := m.activeProcesses[executionID]
	m.mu.RUnlock()

	if !ok {
		return fmt.Errorf("no active process for execution %s", executionID)
	}
	return proc.Stop()
}

// CancelExecution implements task.ExecutionManager.
// It stops the active worker process for the given executionID.
func (m *Manager) CancelExecution(_ context.Context, executionID string) error {
	return m.StopExecution(executionID)
}
