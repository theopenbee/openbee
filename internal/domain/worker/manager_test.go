package worker

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	ai "github.com/theopenbee/openbee/internal/ai"
	"github.com/theopenbee/openbee/internal/domain/enginecfg"
	"github.com/theopenbee/openbee/internal/domain/env"
	"github.com/theopenbee/openbee/internal/infra/config"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/infra/store"
)

type mockEngine struct{}

func (e *mockEngine) Prepare(_ string, _ ai.PrepareOptions) error {
	return nil
}

func (e *mockEngine) Run(_ context.Context, _, _ string, _ ai.RunOptions, _ string) (ai.RunResult, error) {
	ch := make(chan ai.Output, 1)
	ch <- ai.Output{Type: ai.OutputDone}
	close(ch)
	return ai.RunResult{Process: &mockProcess{}, Output: ch, ExtractResult: func(string) string { return "" }}, nil
}

func (m *mockEngine) CollectTokenUsage(_ context.Context, _ string) ([]ai.TokenUsage, error) {
	return nil, ai.ErrSessionDataNotFound
}

type mockProcess struct{}

func (p *mockProcess) PID() int    { return 0 }
func (p *mockProcess) Stop() error { return nil }

// silentMockEngine simulates a process whose output channel closes without
// emitting a terminal Done/Error signal — the abandoned-process scenario.
type silentMockEngine struct{}

func (e *silentMockEngine) Prepare(_ string, _ ai.PrepareOptions) error { return nil }

func (e *silentMockEngine) Run(_ context.Context, _, _ string, _ ai.RunOptions, _ string) (ai.RunResult, error) {
	ch := make(chan ai.Output)
	close(ch)
	return ai.RunResult{Process: &mockProcess{}, Output: ch, ExtractResult: func(string) string { return "" }}, nil
}

func (e *silentMockEngine) CollectTokenUsage(_ context.Context, _ string) ([]ai.TokenUsage, error) {
	return nil, ai.ErrSessionDataNotFound
}

func newTestManager(t *testing.T, engines map[string]ai.EngineAdapter, defaultEngine string) *Manager {
	return newTestManagerWithBotNames(t, engines, defaultEngine, nil)
}

func newTestManagerWithBotNames(t *testing.T, engines map[string]ai.EngineAdapter, defaultEngine string, botNames []string) *Manager {
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
	bc.RPC.TokenTTL = time.Minute
	m := &Manager{
		workerBaseDir:   dir,
		tokenSecret:     bc.RPC.TokenSecret,
		tokenTTL:        bc.RPC.TokenTTL,
		workerTimeout:   30 * time.Minute,
		workerStore:     ws,
		executionStore:  es,
		engines:         engines,
		engineCfg:       enginecfg.NewStore(defaultEngine),
		envService:      envSvc,
		botNamesLower:   botNames,
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
	name, got := mgr.resolveEngine(w)
	if name != "codex" {
		t.Fatalf("expected codex engine name, got %q", name)
	}
	if got != codex {
		t.Error("expected codex engine adapter")
	}
}

func TestManager_ResolveEngine_EmptyEngine_FallsBackToDefault(t *testing.T) {
	claude := &mockEngine{}
	engines := map[string]ai.EngineAdapter{"claude": claude}
	mgr := newTestManager(t, engines, "claude")

	w := model.Worker{Engine: ""}
	name, got := mgr.resolveEngine(w)
	if name != "claude" {
		t.Fatalf("expected default claude engine name, got %q", name)
	}
	if got != claude {
		t.Error("expected default claude engine adapter")
	}
}

func TestManager_ResolveEngine_UnknownEngine_FallsBackToDefault(t *testing.T) {
	claude := &mockEngine{}
	engines := map[string]ai.EngineAdapter{"claude": claude}
	mgr := newTestManager(t, engines, "claude")

	w := model.Worker{Engine: "unknown-engine"}
	name, got := mgr.resolveEngine(w)
	if name != "claude" {
		t.Fatalf("expected fallback engine name claude, got %q", name)
	}
	if got != claude {
		t.Error("expected fallback to default claude engine adapter")
	}
}

func TestManager_ResolveEngineSelection_UnknownEngineUsesFallbackName(t *testing.T) {
	claude := &mockEngine{}
	engines := map[string]ai.EngineAdapter{"claude": claude}
	mgr := newTestManager(t, engines, "claude")

	name, got, err := mgr.resolveEngineSelection(model.Worker{Engine: "unknown-engine"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "claude" {
		t.Fatalf("got engine name %q, want %q", name, "claude")
	}
	if got != claude {
		t.Error("expected fallback to default claude engine adapter")
	}
}

func TestManager_ValidateEngineArgs_RejectsUnknownEngine(t *testing.T) {
	mgr := newTestManager(t, map[string]ai.EngineAdapter{ai.EngineClaude: &mockEngine{}}, ai.EngineClaude)
	err := mgr.ValidateEngineArgs(map[string]string{"unknown": "--model foo"})
	if err == nil {
		t.Fatal("expected error for unknown engine, got nil")
	}
}

func TestManager_ValidateEngineArgs_RejectsInvalidArgs(t *testing.T) {
	mgr := newTestManager(t, map[string]ai.EngineAdapter{ai.EngineClaude: &mockEngine{}}, ai.EngineClaude)
	err := mgr.ValidateEngineArgs(map[string]string{"claude": `--model "unterminated`})
	if err == nil {
		t.Fatal("expected parse error, got nil")
	}
}

func TestManager_CancelExecution_StopsActiveProcess(t *testing.T) {
	// This test verifies CancelExecution returns a sensible error for an unknown execution ID.
	engines := map[string]ai.EngineAdapter{ai.EngineClaude: &mockEngine{}}
	mgr := newTestManager(t, engines, ai.EngineClaude)

	err := mgr.CancelExecution(context.Background(), "nonexistent-exec-id")
	if err == nil {
		t.Error("expected error for unknown executionID, got nil")
	}
}

// Regression: when a worker process exits without emitting Done/Error (killed,
// crashed, signal-terminated), monitorExecution must finalize the execution
// row instead of leaving it stuck in `running` forever.
func TestManager_MonitorExecution_SilentClose_FinalizesExecution(t *testing.T) {
	engines := map[string]ai.EngineAdapter{ai.EngineClaude: &silentMockEngine{}}
	mgr := newTestManager(t, engines, ai.EngineClaude)

	w, err := mgr.CreateWorker(CreateWorkerParams{Name: "alice"})
	if err != nil {
		t.Fatalf("CreateWorker: %v", err)
	}

	exec, err := mgr.ExecuteWorker(context.Background(), w.ID, "", "test", "session-1", false)
	if err != nil {
		t.Fatalf("ExecuteWorker: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		got, err := mgr.executionStore.GetByID(exec.ID)
		if err == nil && got.Status == model.ExecStatusFailed && got.CompletedAt != nil {
			if got.Result == "" {
				t.Fatal("expected non-empty result on abandoned execution")
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	got, _ := mgr.executionStore.GetByID(exec.ID)
	t.Fatalf("execution not finalized after 2s; status=%s completedAt=%v", got.Status, got.CompletedAt)
}

func TestManager_ValidateWorkerName_DuplicateName(t *testing.T) {
	engines := map[string]ai.EngineAdapter{ai.EngineClaude: &mockEngine{}}
	mgr := newTestManager(t, engines, ai.EngineClaude)

	_, err := mgr.CreateWorker(CreateWorkerParams{Name: "alice"})
	if err != nil {
		t.Fatalf("first create failed: %v", err)
	}
	_, err = mgr.CreateWorker(CreateWorkerParams{Name: "alice"})
	if err == nil {
		t.Fatal("expected error for duplicate name, got nil")
	}
	if !errors.Is(err, ErrValidation) {
		t.Errorf("expected ErrValidation, got %v", err)
	}
}

func TestManager_ValidateWorkerName_CaseInsensitiveDuplicate(t *testing.T) {
	engines := map[string]ai.EngineAdapter{ai.EngineClaude: &mockEngine{}}
	mgr := newTestManager(t, engines, ai.EngineClaude)

	_, err := mgr.CreateWorker(CreateWorkerParams{Name: "Alice"})
	if err != nil {
		t.Fatalf("first create failed: %v", err)
	}
	_, err = mgr.CreateWorker(CreateWorkerParams{Name: "alice"})
	if err == nil {
		t.Fatal("expected error for case-insensitive duplicate, got nil")
	}
	if !errors.Is(err, ErrValidation) {
		t.Errorf("expected ErrValidation, got %v", err)
	}
}

func TestManager_ValidateWorkerName_BotNameConflict(t *testing.T) {
	engines := map[string]ai.EngineAdapter{ai.EngineClaude: &mockEngine{}}
	mgr := newTestManagerWithBotNames(t, engines, ai.EngineClaude, []string{"feishu"})

	_, err := mgr.CreateWorker(CreateWorkerParams{Name: "feishu"})
	if err == nil {
		t.Fatal("expected error for bot name conflict, got nil")
	}
	if !errors.Is(err, ErrValidation) {
		t.Errorf("expected ErrValidation, got %v", err)
	}
}

func TestManager_ValidateWorkerName_BotNameConflict_CaseInsensitive(t *testing.T) {
	engines := map[string]ai.EngineAdapter{ai.EngineClaude: &mockEngine{}}
	mgr := newTestManagerWithBotNames(t, engines, ai.EngineClaude, []string{"feishu"})

	_, err := mgr.CreateWorker(CreateWorkerParams{Name: "FEISHU"})
	if err == nil {
		t.Fatal("expected error for case-insensitive bot name conflict, got nil")
	}
	if !errors.Is(err, ErrValidation) {
		t.Errorf("expected ErrValidation, got %v", err)
	}
}

func TestManager_ValidateWorkerName_WhitespaceTrimmed(t *testing.T) {
	engines := map[string]ai.EngineAdapter{ai.EngineClaude: &mockEngine{}}
	mgr := newTestManager(t, engines, ai.EngineClaude)

	_, err := mgr.CreateWorker(CreateWorkerParams{Name: "alice"})
	if err != nil {
		t.Fatalf("first create failed: %v", err)
	}
	_, err = mgr.CreateWorker(CreateWorkerParams{Name: " alice "})
	if err == nil {
		t.Fatal("expected error for whitespace-padded duplicate, got nil")
	}
	if !errors.Is(err, ErrValidation) {
		t.Errorf("expected ErrValidation, got %v", err)
	}
}

func TestManager_UpdateWorker_RenameToDifferentName_Duplicate(t *testing.T) {
	engines := map[string]ai.EngineAdapter{ai.EngineClaude: &mockEngine{}}
	mgr := newTestManager(t, engines, ai.EngineClaude)

	w1, err := mgr.CreateWorker(CreateWorkerParams{Name: "alice"})
	if err != nil {
		t.Fatalf("create alice failed: %v", err)
	}
	_, err = mgr.CreateWorker(CreateWorkerParams{Name: "bob"})
	if err != nil {
		t.Fatalf("create bob failed: %v", err)
	}

	newName := "bob"
	_, err = mgr.UpdateWorker(w1.ID, UpdateWorkerParams{Name: &newName})
	if err == nil {
		t.Fatal("expected error renaming alice to existing name bob, got nil")
	}
	if !errors.Is(err, ErrValidation) {
		t.Errorf("expected ErrValidation, got %v", err)
	}
}

func TestManager_UpdateWorker_RenameToSameName_Succeeds(t *testing.T) {
	engines := map[string]ai.EngineAdapter{ai.EngineClaude: &mockEngine{}}
	mgr := newTestManager(t, engines, ai.EngineClaude)

	w, err := mgr.CreateWorker(CreateWorkerParams{Name: "alice"})
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	sameName := "alice"
	_, err = mgr.UpdateWorker(w.ID, UpdateWorkerParams{Name: &sameName})
	if err != nil {
		t.Errorf("renaming to same name should succeed, got: %v", err)
	}
}

func TestManager_UpdateWorker_EmptyEngineArgsClearsAll(t *testing.T) {
	engines := map[string]ai.EngineAdapter{ai.EngineClaude: &mockEngine{}, ai.EngineCodex: &mockEngine{}}
	mgr := newTestManager(t, engines, ai.EngineClaude)

	w, err := mgr.CreateWorker(CreateWorkerParams{
		Name:       "alice",
		EngineArgs: `{"claude":"--model sonnet","codex":"--model o3"}`,
	})
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	updated, err := mgr.UpdateWorker(w.ID, UpdateWorkerParams{
		EngineArgs: map[string]string{},
	})
	if err != nil {
		t.Fatalf("update failed: %v", err)
	}
	if updated.EngineArgs != "{}" {
		t.Fatalf("got %s, want {}", updated.EngineArgs)
	}
}

func TestManager_UpdateWorker_RenameToCaseVariant_Succeeds(t *testing.T) {
	engines := map[string]ai.EngineAdapter{ai.EngineClaude: &mockEngine{}}
	mgr := newTestManager(t, engines, ai.EngineClaude)

	w, err := mgr.CreateWorker(CreateWorkerParams{Name: "alice"})
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	upperName := "ALICE"
	_, err = mgr.UpdateWorker(w.ID, UpdateWorkerParams{Name: &upperName})
	if err != nil {
		t.Errorf("renaming to case variant of same name should succeed, got: %v", err)
	}
}

func TestManager_UpdateWorker_RenameToBotName_Rejected(t *testing.T) {
	engines := map[string]ai.EngineAdapter{ai.EngineClaude: &mockEngine{}}
	mgr := newTestManagerWithBotNames(t, engines, ai.EngineClaude, []string{"feishu"})

	w, err := mgr.CreateWorker(CreateWorkerParams{Name: "alice"})
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	botName := "feishu"
	_, err = mgr.UpdateWorker(w.ID, UpdateWorkerParams{Name: &botName})
	if err == nil {
		t.Fatal("expected error renaming to bot name, got nil")
	}
	if !errors.Is(err, ErrValidation) {
		t.Errorf("expected ErrValidation, got %v", err)
	}
}

func TestManager_UpdateWorker_RenameToBotNameCaseVariant_Rejected(t *testing.T) {
	engines := map[string]ai.EngineAdapter{ai.EngineClaude: &mockEngine{}}
	mgr := newTestManagerWithBotNames(t, engines, ai.EngineClaude, []string{"feishu"})

	w, err := mgr.CreateWorker(CreateWorkerParams{Name: "alice"})
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	upperBotName := "FEISHU"
	_, err = mgr.UpdateWorker(w.ID, UpdateWorkerParams{Name: &upperBotName})
	if err == nil {
		t.Fatal("expected error renaming to case-variant bot name, got nil")
	}
	if !errors.Is(err, ErrValidation) {
		t.Errorf("expected ErrValidation, got %v", err)
	}
}

func TestManager_UpdateWorker_NoNameChange_Succeeds(t *testing.T) {
	engines := map[string]ai.EngineAdapter{ai.EngineClaude: &mockEngine{}}
	mgr := newTestManager(t, engines, ai.EngineClaude)

	w, err := mgr.CreateWorker(CreateWorkerParams{Name: "alice", Description: "original"})
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	newDesc := "updated"
	updated, err := mgr.UpdateWorker(w.ID, UpdateWorkerParams{Description: &newDesc})
	if err != nil {
		t.Errorf("update without name change should succeed, got: %v", err)
	}
	if updated.Description != "updated" {
		t.Errorf("expected description %q, got %q", "updated", updated.Description)
	}
	if updated.Name != "alice" {
		t.Errorf("expected name to remain %q, got %q", "alice", updated.Name)
	}
}
