package worker

import (
	"context"
	"fmt"
	"time"

	"github.com/theopenbee/openbee/internal/ai/bridge"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/infra/utils"
	"go.uber.org/zap"
)

// ExecuteWorker runs a worker. When resume is true, the AI engine will attempt
// to resume the session identified by sessionID; otherwise it starts a fresh session.
// sessionID must always be non-empty; callers are responsible for generating it.
func (m *Manager) ExecuteWorker(ctx context.Context, workerID, triggerInput, sessionID string, resume bool) (model.WorkerExecution, error) {
	worker, err := m.workerStore.GetByID(workerID)
	if err != nil {
		return model.WorkerExecution{}, fmt.Errorf("get worker: %w", err)
	}

	engineName := m.resolveEngineForWorker(worker)

	exec, err := m.executionStore.Create(workerID, triggerInput, sessionID, engineName)
	if err != nil {
		return model.WorkerExecution{}, fmt.Errorf("create execution: %w", err)
	}

	if err := m.workerStore.UpdateStatus(worker.ID, model.WorkerStatusWorking); err != nil {
		log.Error("failed to update worker status", zap.Error(err))
	}

	var startedAt time.Time
	if exec.StartedAt != nil {
		startedAt = time.UnixMilli(*exec.StartedAt)
	}

	handle, err := m.br.RunWorker(ctx, bridge.WorkerRunRequest{
		WorkerID:         worker.ID,
		PermissionScopes: utils.SplitAndTrim(worker.PermissionScopes),
		ExecutionID:      exec.ID,
		StartedAt:        startedAt,
		EngineHint:       worker.Engine,
		EngineArgs:       worker.EngineArgs,
		WorkDir:          worker.WorkDir,
		Prompt:           triggerInput,
		SessionID:        exec.SessionID,
		Resume:           resume,
		Timeout:          m.workerTimeout,
	})
	if err != nil {
		m.executionStore.UpdateResult(exec.ID, err.Error(), model.ExecStatusFailed)
		m.workerStore.UpdateStatus(worker.ID, model.WorkerStatusError)
		return exec, fmt.Errorf("start runtime: %w", err)
	}

	m.mu.Lock()
	m.activeHandles[exec.ID] = handle
	m.mu.Unlock()

	m.executionStore.UpdatePID(exec.ID, handle.PID())
	go m.monitorExecution(exec, worker, handle)
	return exec, nil
}

func (m *Manager) monitorExecution(exec model.WorkerExecution, worker model.Worker, handle bridge.Handle) {
	outcome, err := handle.Wait(context.Background())
	if err != nil {
		log.Error("worker Wait error", zap.String("execution_id", exec.ID), zap.Error(err))
		m.executionStore.UpdateResult(exec.ID, err.Error(), model.ExecStatusFailed)
		m.workerStore.UpdateStatus(worker.ID, model.WorkerStatusError)
	} else {
		switch outcome.Status {
		case bridge.StatusCompleted:
			m.executionStore.UpdateResult(exec.ID, outcome.Result, model.ExecStatusCompleted)
			m.workerStore.UpdateStatus(worker.ID, model.WorkerStatusIdle)
		case bridge.StatusFailed:
			m.executionStore.UpdateResult(exec.ID, outcome.Result, model.ExecStatusFailed)
			m.workerStore.UpdateStatus(worker.ID, model.WorkerStatusError)
		case bridge.StatusAbandoned:
			if _, err := m.executionStore.MarkAbandoned(context.Background(), exec.ID, outcome.Result); err != nil {
				log.Error("finalize abandoned execution", zap.String("executionID", exec.ID), zap.Error(err))
			}
			m.workerStore.UpdateStatus(worker.ID, model.WorkerStatusError)
		}
	}

	m.mu.Lock()
	delete(m.activeHandles, exec.ID)
	m.mu.Unlock()
}

func (m *Manager) StopExecution(executionID string) error {
	m.mu.RLock()
	h, ok := m.activeHandles[executionID]
	m.mu.RUnlock()

	if !ok {
		return fmt.Errorf("no active process for execution %s", executionID)
	}
	return h.Stop()
}

func (m *Manager) CancelExecution(_ context.Context, executionID string) error {
	return m.StopExecution(executionID)
}
