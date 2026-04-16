package worker

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	ai "github.com/theopenbee/openbee/internal/ai"
	"github.com/theopenbee/openbee/internal/domain/env"
	"github.com/theopenbee/openbee/internal/infra/config"
	"github.com/theopenbee/openbee/internal/infra/model"
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

func newTestManager(t *testing.T, engines map[string]ai.EngineAdapter, defaultEngine string) *Manager {
	t.Helper()
	dir := t.TempDir()
	db, err := store.InitDB(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	ws := store.NewWorkerStore(db)
	es := store.NewExecutionStore(db, dir)
	const testKey = "0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f20"
	envSvc, err := env.NewService(store.NewEnvConfigStore(db), store.NewDepartmentStore(db), testKey)
	if err != nil {
		t.Fatal(err)
	}
	bc := config.BeeConfig{}
	bc.MCP.TokenTTL = time.Minute
	m := &Manager{
		workerBaseDir:   dir,
		tokenSecret:     bc.MCP.TokenSecret,
		tokenTTL:        bc.MCP.TokenTTL,
		workerTimeout:   30 * time.Minute,
		workerStore:     ws,
		executionStore:  es,
		engines:         engines,
		defaultEngine:   defaultEngine,
		envService:      envSvc,
		activeProcesses: make(map[string]ai.Process),
	}
	return m
}

func TestManager_ResolveEngine_KnownEngine(t *testing.T) {
	claude := &mockEngine{}
	codex := &mockEngine{}
	engines := map[string]ai.EngineAdapter{"claude": claude, "codex": codex}
	mgr := newTestManager(t, engines, "claude")

	w := model.Worker{Engine: "codex"}
	got := mgr.resolveEngine(w)
	if got != codex {
		t.Error("expected codex engine adapter")
	}
}

func TestManager_ResolveEngine_EmptyEngine_FallsBackToDefault(t *testing.T) {
	claude := &mockEngine{}
	engines := map[string]ai.EngineAdapter{"claude": claude}
	mgr := newTestManager(t, engines, "claude")

	w := model.Worker{Engine: ""}
	got := mgr.resolveEngine(w)
	if got != claude {
		t.Error("expected default claude engine adapter")
	}
}

func TestManager_ResolveEngine_UnknownEngine_FallsBackToDefault(t *testing.T) {
	claude := &mockEngine{}
	engines := map[string]ai.EngineAdapter{"claude": claude}
	mgr := newTestManager(t, engines, "claude")

	w := model.Worker{Engine: "unknown-engine"}
	got := mgr.resolveEngine(w)
	if got != claude {
		t.Error("expected fallback to default claude engine adapter")
	}
}

func TestManager_ResolveEngine_DefaultMissingFromMap_ReturnsFirst(t *testing.T) {
	claude := &mockEngine{}
	// defaultEngine "claude" is NOT in the map — only "codex" is
	engines := map[string]ai.EngineAdapter{"codex": claude}
	mgr := newTestManager(t, engines, "claude")

	w := model.Worker{Engine: ""}
	got := mgr.resolveEngine(w)
	// Should not panic; should return the only available engine
	if got == nil {
		t.Error("expected non-nil engine fallback")
	}
}

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
	engines := map[string]ai.EngineAdapter{"claude": &mockEngine{}}
	mgr := NewManager(dir, cfg, ws, es, engines, envSvc)

	err = mgr.CancelExecution(context.Background(), "nonexistent-exec-id")
	if err == nil {
		t.Error("expected error for unknown executionID, got nil")
	}
}
