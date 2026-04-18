package ai

import (
	"context"
	"fmt"

	"golang.org/x/sync/errgroup"

	"github.com/theopenbee/openbee/internal/domain/enginecfg"
)

// DynamicAdapter wraps multiple EngineAdapters and routes each Run/ExtractResult
// call to whichever engine enginecfg.Get() returns at call time.
type DynamicAdapter struct {
	engines map[string]EngineAdapter
}

// NewDynamicAdapter constructs a DynamicAdapter from a map of engine adapters.
func NewDynamicAdapter(engines map[string]EngineAdapter) *DynamicAdapter {
	return &DynamicAdapter{engines: engines}
}

// Prepare initialises every engine adapter for the given workDir concurrently.
// All engines share the same workDir, so preparation is independent and safe to parallelise.
func (d *DynamicAdapter) Prepare(workDir string, opts PrepareOptions) error {
	g := &errgroup.Group{}
	for name, e := range d.engines {
		g.Go(func() error {
			if err := e.Prepare(workDir, opts); err != nil {
				return fmt.Errorf("prepare engine %q: %w", name, err)
			}
			return nil
		})
	}
	return g.Wait()
}

// Run executes using the engine currently selected in enginecfg.
func (d *DynamicAdapter) Run(ctx context.Context, workDir, prompt string, opts RunOptions, logPath string) (Process, <-chan Output, error) {
	name := enginecfg.Get()
	e, ok := d.engines[name]
	if !ok {
		return nil, nil, fmt.Errorf("engine %q not available", name)
	}
	return e.Run(ctx, workDir, prompt, opts, logPath)
}

// ExtractResult extracts the result using the engine currently selected in enginecfg.
func (d *DynamicAdapter) ExtractResult(logPath string) string {
	name := enginecfg.Get()
	e, ok := d.engines[name]
	if !ok {
		return ""
	}
	return e.ExtractResult(logPath)
}

// ExtractResultFor extracts the result using the named engine adapter.
// Use this when the engine was snapshotted at Run time to avoid a TOCTOU race
// if the default engine changes between Run and ExtractResult.
func (d *DynamicAdapter) ExtractResultFor(name, logPath string) string {
	e, ok := d.engines[name]
	if !ok {
		return ""
	}
	return e.ExtractResult(logPath)
}
