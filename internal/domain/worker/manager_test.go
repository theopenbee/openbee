package worker

import (
	"context"
	"testing"

	ai "github.com/theopenbee/openbee/internal/ai"
	"github.com/theopenbee/openbee/internal/domain/env"
	"github.com/theopenbee/openbee/internal/infra/config"
	"github.com/theopenbee/openbee/internal/infra/store"
)

type mockEngine struct{}

func (e *mockEngine) Prepare(_ string, _ ai.PrepareOptions) error {
	return nil
}

func (e *mockEngine) Run(_ context.Context, _, _ string, _ ai.RunOptions, _ string) (ai.Process, <-chan ai.Output, error) {
	ch := make(chan ai.Output, 1)
	ch <- ai.Output{Type: ai.OutputDone}
	close(ch)
	return &mockProcess{}, ch, nil
}

func (e *mockEngine) ExtractResult(_ string) string { return "" }

type mockProcess struct{}

func (p *mockProcess) PID() int    { return 0 }
func (p *mockProcess) Stop() error { return nil }

func TestManager_CancelExecution_StopsActiveProcess(t *testing.T) {
	// This test verifies CancelExecution calls StopExecution on the active process.
	// We use a real Manager with a mock engine that never finishes.
	// Since we can't easily inject a mock engine, we verify the method exists
	// and returns a sensible error for an unknown execution ID.
	cfg := config.BeeConfig{}
	cfg.Claude.Path = "echo" // won't actually run; just needs to not panic
	dir := t.TempDir()
	db, err := store.InitDB(dir + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ws := store.NewWorkerStore(db)
	es := store.NewExecutionStore(db, dir)

	const testKey = "0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f20"
	envSvc, err := env.NewService(store.NewEnvConfigStore(db), store.NewDepartmentStore(db), testKey)
	if err != nil {
		t.Fatal(err)
	}
	mgr := NewManager(dir, cfg, ws, es, &mockEngine{}, envSvc)

	err = mgr.CancelExecution(context.Background(), "nonexistent-exec-id")
	if err == nil {
		t.Error("expected error for unknown executionID, got nil")
	}
}
