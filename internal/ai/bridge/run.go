package bridge

import (
	"context"
	"fmt"
	"sync"
	"time"

	ai "github.com/theopenbee/openbee/internal/ai"
)

// Status classifies the terminal state of a run.
type Status int

const (
	StatusCompleted Status = iota + 1
	StatusFailed
	StatusAbandoned // process exited without a Done/Error signal
)

// Outcome is the terminal result of a run.
type Outcome struct {
	Status Status
	Result string
}

// Handle is the lifecycle handle for a started run.
type Handle interface {
	PID() int
	EngineName() string
	Stop() error
	Wait(ctx context.Context) (Outcome, error)
}

// WorkerRunRequest carries the inputs required to run a worker.
type WorkerRunRequest struct {
	WorkerID         string
	PermissionScopes []string
	ExecutionID      string
	StartedAt        time.Time
	EngineHint       string
	EngineArgs       string
	WorkDir          string
	Prompt           string
	SessionID        string
	Resume           bool
	Timeout          time.Duration
}

// BeeRunRequest carries the inputs required to run a bee.
type BeeRunRequest struct {
	WorkDir   string
	Prompt    string
	SessionID string
	Resume    bool
	LogPath   string
}

const abandonedPlaceholder = "process exited without completion signal"

type runHandle struct {
	pid        int
	engineName string
	proc       ai.Process
	out        <-chan ai.Output
	extract    func() string

	once    sync.Once
	outcome Outcome
	doneCh  chan struct{}

	cancel context.CancelFunc // cancels the run's internal context (with Timeout)
	stop   sync.Once
}

func (h *runHandle) PID() int           { return h.pid }
func (h *runHandle) EngineName() string { return h.engineName }

func (h *runHandle) Stop() error {
	var err error
	h.stop.Do(func() { err = h.proc.Stop() })
	return err
}

func (h *runHandle) Wait(ctx context.Context) (Outcome, error) {
	select {
	case <-h.doneCh:
		return h.outcome, nil
	case <-ctx.Done():
		return Outcome{}, ctx.Err()
	}
}

// drain reads h.out and produces the single terminal outcome.
func (h *runHandle) drain() {
	defer func() {
		close(h.doneCh)
		if h.cancel != nil {
			h.cancel()
		}
	}()
	finalized := false
	for ev := range h.out {
		switch ev.Type {
		case ai.OutputDone:
			h.once.Do(func() { h.outcome = Outcome{Status: StatusCompleted, Result: h.extract()} })
			finalized = true
		case ai.OutputError:
			h.once.Do(func() {
				res := h.extract()
				if res == "" {
					res = ev.Content
				}
				h.outcome = Outcome{Status: StatusFailed, Result: res}
			})
			finalized = true
		}
	}
	if !finalized {
		h.once.Do(func() {
			res := h.extract()
			if res == "" {
				res = abandonedPlaceholder
			}
			h.outcome = Outcome{Status: StatusAbandoned, Result: res}
		})
	}
}

func (b *bridgeImpl) RunWorker(ctx context.Context, req WorkerRunRequest) (Handle, error) {
	engineName := b.deps.EngineSelector.ForWorker(req.EngineHint)
	engine, ok := b.engines[engineName]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrEngineNotEnabled, engineName)
	}

	logPath, err := b.deps.LogPathProvider.PrepareForWorker(req.ExecutionID, req.StartedAt)
	if err != nil {
		return nil, fmt.Errorf("prepare log path: %w", err)
	}
	token, err := b.deps.TokenIssuer.WorkerToken(req.WorkerID, req.PermissionScopes)
	if err != nil {
		return nil, fmt.Errorf("mint worker token: %w", err)
	}
	env, err := b.deps.EnvResolver.WorkerEnv(req.WorkerID)
	if err != nil {
		return nil, fmt.Errorf("resolve worker env: %w", err)
	}
	args := b.deps.ArgsResolver.ForWorker(ctx, req.EngineArgs, engineName)

	execCtx, cancel := newRunContext(req.Timeout)

	res, err := engine.Run(execCtx, req.WorkDir, req.Prompt, ai.RunOptions{
		SessionID: req.SessionID,
		Resume:    req.Resume,
		APIKey:    token,
		ExtraEnv:  env,
		ExtraArgs: args,
	}, logPath)
	if err != nil {
		cancel()
		return nil, err
	}

	h := &runHandle{
		pid:        res.Process.PID(),
		engineName: engineName,
		proc:       res.Process,
		out:        res.Output,
		extract:    res.ExtractResult,
		doneCh:     make(chan struct{}),
		cancel:     cancel,
	}
	go h.drain()
	return h, nil
}

func (b *bridgeImpl) RunBee(ctx context.Context, req BeeRunRequest) (Handle, error) {
	engineName := b.deps.EngineSelector.ForBee()
	engine, ok := b.engines[engineName]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrEngineNotEnabled, engineName)
	}

	token, err := b.deps.TokenIssuer.BeeToken()
	if err != nil {
		return nil, fmt.Errorf("mint bee token: %w", err)
	}
	env, err := b.deps.EnvResolver.BeeEnv()
	if err != nil {
		return nil, fmt.Errorf("resolve bee env: %w", err)
	}
	args := b.deps.ArgsResolver.ForBee(ctx, engineName)

	execCtx, cancel := newRunContext(0) // bee currently has no explicit timeout

	res, err := engine.Run(execCtx, req.WorkDir, req.Prompt, ai.RunOptions{
		SessionID: req.SessionID,
		Resume:    req.Resume,
		APIKey:    token,
		ExtraEnv:  env,
		ExtraArgs: args,
	}, req.LogPath)
	if err != nil {
		cancel()
		return nil, err
	}
	h := &runHandle{
		pid:        res.Process.PID(),
		engineName: engineName,
		proc:       res.Process,
		out:        res.Output,
		extract:    res.ExtractResult,
		doneCh:     make(chan struct{}),
		cancel:     cancel,
	}
	go h.drain()
	return h, nil
}

func newRunContext(timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout > 0 {
		return context.WithTimeout(context.Background(), timeout)
	}
	return context.WithCancel(context.Background())
}
