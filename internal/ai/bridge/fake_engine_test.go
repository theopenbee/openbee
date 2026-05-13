package bridge

import (
	"context"
	"sync/atomic"

	ai "github.com/theopenbee/openbee/internal/ai"
)

// fakeProcess satisfies ai.Process for tests.
type fakeProcess struct {
	pid     int
	stopped atomic.Bool
	onStop  func()
}

func (f *fakeProcess) PID() int { return f.pid }
func (f *fakeProcess) Stop() error {
	f.stopped.Store(true)
	if f.onStop != nil {
		f.onStop()
	}
	return nil
}

// fakeEngine drives all six lifecycle invariants from tests.
type fakeEngine struct {
	pid        int
	scriptedCh chan ai.Output           // closed by test to signal end
	extract    func() string            // returns final extracted result
	onRun      func(opts ai.RunOptions) // observe RunOptions passed to engine
	runErr     error                    // returned from Run when non-nil
	usages     []ai.TokenUsage
	usagesErr  error
	proc       *fakeProcess
	gotCtx     context.Context // captures the ctx engine.Run received
}

func (f *fakeEngine) Run(ctx context.Context, _ string, _ string, opts ai.RunOptions, _ string) (ai.RunResult, error) {
	f.gotCtx = ctx
	if f.runErr != nil {
		return ai.RunResult{}, f.runErr
	}
	if f.onRun != nil {
		f.onRun(opts)
	}
	if f.proc == nil {
		f.proc = &fakeProcess{pid: f.pid}
	}
	extract := f.extract
	if extract == nil {
		extract = func() string { return "" }
	}
	return ai.RunResult{
		Process:       f.proc,
		Output:        f.scriptedCh,
		ExtractResult: extract,
	}, nil
}

func (f *fakeEngine) CollectTokenUsage(context.Context, string) ([]ai.TokenUsage, error) {
	return f.usages, f.usagesErr
}
