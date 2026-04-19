package ai

import (
	"context"
	"fmt"

	"github.com/theopenbee/openbee/internal/domain/enginecfg"
)

// DynamicAdapter wraps multiple EngineAdapters and routes each Run call to
// whichever engine cfg.Get() returns at call time. The RunResult's
// ExtractResult closes over the engine that was actually picked, so callers
// processing results asynchronously are immune to later /engine switches.
type DynamicAdapter struct {
	engines map[string]EngineAdapter
	cfg     *enginecfg.Store
}

// NewDynamicAdapter constructs a DynamicAdapter routing through cfg.
func NewDynamicAdapter(engines map[string]EngineAdapter, cfg *enginecfg.Store) *DynamicAdapter {
	return &DynamicAdapter{engines: engines, cfg: cfg}
}

// Prepare initialises every engine adapter for the given workDir.
// Most engines have a no-op Prepare; the only meaningful work (Claude's legacy
// file cleanup) is a single os.Remove, so a sequential loop is sufficient.
func (d *DynamicAdapter) Prepare(workDir string, opts PrepareOptions) error {
	for name, e := range d.engines {
		if err := e.Prepare(workDir, opts); err != nil {
			return fmt.Errorf("prepare engine %q: %w", name, err)
		}
	}
	return nil
}

func (d *DynamicAdapter) Run(ctx context.Context, workDir, prompt string, opts RunOptions, logPath string) (RunResult, error) {
	name := d.cfg.Get()
	e, ok := d.engines[name]
	if !ok {
		return RunResult{}, fmt.Errorf("engine %q not available", name)
	}
	return e.Run(ctx, workDir, prompt, opts, logPath)
}
