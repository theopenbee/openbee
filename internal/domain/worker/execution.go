package worker

import (
	"context"
	"fmt"
	"os"
	"time"

	ai "github.com/theopenbee/openbee/internal/ai"
	"github.com/theopenbee/openbee/internal/infra/auth"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/infra/store"
	"github.com/theopenbee/openbee/internal/infra/utils"
	"go.uber.org/zap"
)

// ExecuteRequest bundles the inputs for ExecuteWorker. SessionID must always
// be non-empty; callers generate it. Resume tells the AI engine to resume the
// session identified by SessionID instead of starting fresh.
type ExecuteRequest struct {
	WorkerID     string
	TaskID       string
	TriggerInput string
	SessionID    string
	Resume       bool
}

// ExecuteWorker runs a worker against req.
func (m *Manager) ExecuteWorker(ctx context.Context, req ExecuteRequest) (model.WorkerExecution, error) {
	worker, err := m.workerStore.GetByID(req.WorkerID)
	if err != nil {
		return model.WorkerExecution{}, fmt.Errorf("get worker: %w", err)
	}

	engineName, engine := m.resolveEngine(worker)

	exec, err := m.executionStore.Create(store.ExecutionCreate{
		WorkerID:     req.WorkerID,
		TaskID:       req.TaskID,
		TriggerInput: req.TriggerInput,
		SessionID:    req.SessionID,
		Engine:       engineName,
	})
	if err != nil {
		return model.WorkerExecution{}, fmt.Errorf("create execution: %w", err)
	}

	if err := m.setStatus(worker.ID, model.WorkerStatusWorking); err != nil {
		log.Error("failed to update worker status", zap.Error(err))
	}

	if err := os.MkdirAll(worker.WorkDir, 0o755); err != nil {
		log.Error("ensure worker workdir", zap.String("op", "execute"), zap.String("work_dir", worker.WorkDir), zap.Error(err))
		m.executionStore.UpdateResult(exec.ID, err.Error(), model.ExecStatusFailed)
		m.setStatus(worker.ID, model.WorkerStatusError)
		return exec, fmt.Errorf("ensure worker workdir: %w", err)
	}
	// Prepare is best-effort; the runtime below surfaces the real error if it matters.
	if err := engine.Prepare(worker.WorkDir, ai.PrepareOptions{Role: ai.RoleWorker}); err != nil {
		log.Error("prepare worker workspace", zap.String("op", "execute"), zap.Error(err))
	}
	timeout := m.workerTimeout

	if err := m.launchRuntime(ctx, exec, worker, engine, engineName, timeout, req.TriggerInput, req.Resume); err != nil {
		m.executionStore.UpdateResult(exec.ID, err.Error(), model.ExecStatusFailed)
		m.setStatus(worker.ID, model.WorkerStatusError)
		return exec, fmt.Errorf("start runtime: %w", err)
	}

	return exec, nil
}

func (m *Manager) launchRuntime(ctx context.Context, exec model.WorkerExecution, worker model.Worker, engine ai.EngineAdapter, engineName string, timeout time.Duration, prompt string, resume bool) error {
	logPath, err := m.executionStore.PrepareLogPath(exec.ID, exec.StartedAt)
	if err != nil {
		return fmt.Errorf("prepare log path: %w", err)
	}

	token, err := auth.GenerateWorkerToken(m.tokenSecret, worker.ID, utils.SplitAndTrim(worker.PermissionScopes), m.tokenTTL)
	if err != nil {
		return fmt.Errorf("generate worker token: %w", err)
	}

	// execCtx inherits the caller's context so dispatcher-side cancellation
	// (task cancel, /clear, shutdown) actually kills the worker process.
	var execCtx context.Context
	var cancel context.CancelFunc
	if timeout > 0 {
		execCtx, cancel = context.WithTimeout(ctx, timeout)
	} else {
		execCtx, cancel = context.WithCancel(ctx)
	}

	extraEnv, err := m.envService.ResolveWorkerEnv(worker.ID)
	if err != nil {
		cancel()
		return fmt.Errorf("resolve worker env: %w", err)
	}

	extraArgs := m.resolveEngineArgs(ctx, worker, engineName)

	runRes, err := engine.Run(execCtx, worker.WorkDir, prompt, ai.RunOptions{
		SessionID: exec.SessionID,
		Resume:    resume,
		APIKey:    token,
		ExtraEnv:  extraEnv,
		ExtraArgs: extraArgs,
	}, logPath)
	if err != nil {
		cancel()
		return err
	}

	m.mu.Lock()
	m.activeProcesses[exec.ID] = runRes.Process
	m.monitorDone[exec.ID] = make(chan struct{})
	m.mu.Unlock()

	m.executionStore.UpdatePID(exec.ID, runRes.Process.PID())
	go m.monitorExecution(exec, worker, runRes, cancel, logPath)
	return nil
}

func (m *Manager) monitorExecution(exec model.WorkerExecution, worker model.Worker, runRes ai.RunResult, cancel context.CancelFunc, logPath string) {
	defer cancel()

	finalized := false
	for out := range runRes.Output {
		switch out.Type {
		case ai.OutputDone:
			result := runRes.ExtractResult(logPath)
			m.executionStore.UpdateResult(exec.ID, result, model.ExecStatusCompleted)
			m.setStatus(worker.ID, model.WorkerStatusIdle)
			finalized = true
		case ai.OutputError:
			result := runRes.ExtractResult(logPath)
			if result == "" {
				result = out.Content
			}
			m.executionStore.UpdateResult(exec.ID, result, model.ExecStatusFailed)
			m.setStatus(worker.ID, model.WorkerStatusError)
			finalized = true
		}
	}

	if !finalized {
		// Output channel closed without a terminal Done/Error signal —
		// process was killed, crashed, or signal-terminated. Without this
		// fallback the execution would stay in `running` forever.
		result := runRes.ExtractResult(logPath)
		if result == "" {
			result = "process exited without completion signal"
		}
		if _, err := m.executionStore.MarkAbandoned(context.Background(), exec.ID, result); err != nil {
			log.Error("finalize abandoned execution", zap.String("executionID", exec.ID), zap.Error(err))
		}
		m.setStatus(worker.ID, model.WorkerStatusError)
	}

	m.mu.Lock()
	delete(m.activeProcesses, exec.ID)
	if done, ok := m.monitorDone[exec.ID]; ok {
		close(done)
		delete(m.monitorDone, exec.ID)
	}
	m.mu.Unlock()
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

func (m *Manager) SettleStopped(ctx context.Context, executionID, workerID string) {
	m.mu.RLock()
	done := m.monitorDone[executionID]
	m.mu.RUnlock()
	if done != nil {
		select {
		case <-done:
		case <-ctx.Done():
			return
		}
	}
	busy, err := m.executionStore.HasActiveExecutionsByWorkerID(ctx, workerID)
	if err != nil {
		log.Error("check active executions", zap.String("worker_id", workerID), zap.Error(err))
		return
	}
	if busy {
		return
	}
	if err := m.setStatus(workerID, model.WorkerStatusIdle); err != nil {
		log.Error("settle stopped worker status", zap.String("worker_id", workerID), zap.Error(err))
	}
}

// CancelExecution implements task.ExecutionManager.
// It stops the active worker process for the given executionID.
func (m *Manager) CancelExecution(_ context.Context, executionID string) error {
	return m.StopExecution(executionID)
}
