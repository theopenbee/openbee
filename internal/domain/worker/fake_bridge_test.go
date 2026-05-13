package worker

import (
	"context"

	"github.com/theopenbee/openbee/internal/ai/bridge"
)

// fakeBridge implements bridge.Bridge for unit tests.
type fakeBridge struct {
	runWorker func(ctx context.Context, req bridge.WorkerRunRequest) (bridge.Handle, error)
	runBee    func(ctx context.Context, req bridge.BeeRunRequest) (bridge.Handle, error)

	allEngines     []string
	enabledEngines []string
	resolveWorker  func(workerID, hint string) string
	resolveBee     func() string
	validateEngine func(name string) error
	validateArgs   func(line string) error
	collectUsage   func(ctx context.Context, engineName, sid string) ([]bridge.Usage, error)
}

func (f *fakeBridge) RunWorker(ctx context.Context, req bridge.WorkerRunRequest) (bridge.Handle, error) {
	if f.runWorker == nil {
		return nil, nil
	}
	return f.runWorker(ctx, req)
}
func (f *fakeBridge) RunBee(ctx context.Context, req bridge.BeeRunRequest) (bridge.Handle, error) {
	if f.runBee == nil {
		return nil, nil
	}
	return f.runBee(ctx, req)
}
func (f *fakeBridge) AllEngines() []string     { return f.allEngines }
func (f *fakeBridge) EnabledEngines() []string { return f.enabledEngines }
func (f *fakeBridge) IsEnabled(name string) bool {
	for _, n := range f.enabledEngines {
		if n == name {
			return true
		}
	}
	return false
}
func (f *fakeBridge) ValidateEngine(name string) error {
	if f.validateEngine != nil {
		return f.validateEngine(name)
	}
	return nil
}
func (f *fakeBridge) ValidateEngineArgs(line string) error {
	if f.validateArgs != nil {
		return f.validateArgs(line)
	}
	return nil
}
func (f *fakeBridge) ResolveEngineForWorker(workerID, hint string) string {
	if f.resolveWorker != nil {
		return f.resolveWorker(workerID, hint)
	}
	return hint
}
func (f *fakeBridge) ResolveEngineForBee() string {
	if f.resolveBee != nil {
		return f.resolveBee()
	}
	return ""
}
func (f *fakeBridge) CollectUsage(ctx context.Context, engineName, sid string) ([]bridge.Usage, error) {
	if f.collectUsage != nil {
		return f.collectUsage(ctx, engineName, sid)
	}
	return nil, nil
}
