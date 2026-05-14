package worker

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/theopenbee/openbee/internal/bridge"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/infra/store"
)

type mockProcess struct{}

func (p *mockProcess) PID() int    { return 0 }
func (p *mockProcess) Stop() error { return nil }

type fakeBridge struct {
	enabled       []string
	resolved      string
	resolveErr    error
	runHandle     bridge.RunHandle
	runErr        error
	runRequest    bridge.WorkerRunRequest
	prepareEngine string
}

func (f *fakeBridge) EnabledEngines() []string { return f.enabled }

func (f *fakeBridge) ValidateEngine(name string) error {
	if name == "" {
		return nil
	}
	for _, engine := range f.enabled {
		if name == engine {
			return nil
		}
	}
	return fmt.Errorf("engine %q is not enabled", name)
}

func (f *fakeBridge) ResolveEngine(workerEngine string) (string, error) {
	if f.resolveErr != nil {
		return "", f.resolveErr
	}
	for _, engine := range f.enabled {
		if workerEngine == engine {
			return workerEngine, nil
		}
	}
	if f.resolved != "" {
		return f.resolved, nil
	}
	if len(f.enabled) > 0 {
		return f.enabled[0], nil
	}
	return "", fmt.Errorf("no engine")
}

func (f *fakeBridge) BuildBeeSessionPrefix() string { return "" }
func (f *fakeBridge) BuildWorkerSessionPrefix(bridge.WorkerPersona) string {
	return ""
}
func (f *fakeBridge) PrepareBeeWorkspace(string) error { return nil }
func (f *fakeBridge) PrepareWorkerWorkspace(_ string, engineName string) error {
	f.prepareEngine = engineName
	return nil
}
func (f *fakeBridge) RunBee(context.Context, bridge.BeeRunRequest) (bridge.RunHandle, error) {
	return bridge.RunHandle{}, nil
}
func (f *fakeBridge) RunWorker(_ context.Context, req bridge.WorkerRunRequest) (bridge.RunHandle, error) {
	f.runRequest = req
	if f.runErr != nil {
		return bridge.RunHandle{}, f.runErr
	}
	if f.runHandle.Events == nil {
		ch := make(chan bridge.LifecycleEvent, 1)
		ch <- bridge.LifecycleEvent{Type: bridge.LifecycleDone}
		close(ch)
		f.runHandle = bridge.RunHandle{
			Engine:        req.WorkerEngine,
			Process:       &mockProcess{},
			Events:        ch,
			ExtractResult: func(string) string { return "" },
		}
	}
	return f.runHandle, nil
}
func (f *fakeBridge) CollectTokenUsage(context.Context, string, string) (bridge.UsageResult, error) {
	return bridge.UsageResult{}, bridge.ErrSessionDataNotFound
}

func newTestBridge(enabled []string, defaultEngine string) *fakeBridge {
	return &fakeBridge{enabled: enabled, resolved: defaultEngine}
}

func newTestManager(t *testing.T, br *fakeBridge) *Manager {
	return newTestManagerWithBotNames(t, br, nil)
}

func newTestManagerWithBotNames(t *testing.T, br *fakeBridge, botNames []string) *Manager {
	t.Helper()
	dir := t.TempDir()
	db, err := store.InitDB(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	ws := store.NewWorkerStore(db)
	es := store.NewExecutionStore(db, dir)
	m := &Manager{
		workerBaseDir:   dir,
		workerTimeout:   30 * time.Minute,
		workerStore:     ws,
		executionStore:  es,
		bridge:          br,
		botNamesLower:   botNames,
		activeProcesses: make(map[string]bridge.ProcessHandle),
	}
	return m
}

func TestManager_ResolveEngine_KnownEngine(t *testing.T) {
	mgr := newTestManager(t, newTestBridge([]string{bridge.EngineClaude, bridge.EngineCodex}, bridge.EngineClaude))

	w := model.Worker{Engine: "codex"}
	name, err := mgr.resolveEngine(w)
	if err != nil {
		t.Fatalf("resolveEngine: %v", err)
	}
	if name != "codex" {
		t.Fatalf("expected codex engine name, got %q", name)
	}
}

func TestManager_ResolveEngine_EmptyEngine_FallsBackToDefault(t *testing.T) {
	mgr := newTestManager(t, newTestBridge([]string{bridge.EngineClaude}, bridge.EngineClaude))

	w := model.Worker{Engine: ""}
	name, err := mgr.resolveEngine(w)
	if err != nil {
		t.Fatalf("resolveEngine: %v", err)
	}
	if name != "claude" {
		t.Fatalf("expected default claude engine name, got %q", name)
	}
}

func TestManager_ResolveEngine_UnknownEngine_FallsBackToDefault(t *testing.T) {
	mgr := newTestManager(t, newTestBridge([]string{bridge.EngineClaude}, bridge.EngineClaude))

	w := model.Worker{Engine: "unknown-engine"}
	name, err := mgr.resolveEngine(w)
	if err != nil {
		t.Fatalf("resolveEngine: %v", err)
	}
	if name != "claude" {
		t.Fatalf("expected fallback engine name claude, got %q", name)
	}
}

func TestManager_ResolveEngineSelection_UnknownEngineUsesFallbackName(t *testing.T) {
	mgr := newTestManager(t, newTestBridge([]string{bridge.EngineClaude}, bridge.EngineClaude))

	name, err := mgr.resolveEngineSelection(model.Worker{Engine: "unknown-engine"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "claude" {
		t.Fatalf("got engine name %q, want %q", name, "claude")
	}
}

func TestManager_ValidateEngineArgs_RejectsUnknownEngine(t *testing.T) {
	mgr := newTestManager(t, newTestBridge([]string{bridge.EngineClaude}, bridge.EngineClaude))
	err := mgr.ValidateEngineArgs(map[string]string{"unknown": "--model foo"})
	if err == nil {
		t.Fatal("expected error for unknown engine, got nil")
	}
}

func TestManager_ValidateEngineArgs_RejectsInvalidArgs(t *testing.T) {
	mgr := newTestManager(t, newTestBridge([]string{bridge.EngineClaude}, bridge.EngineClaude))
	err := mgr.ValidateEngineArgs(map[string]string{"claude": `--model "unterminated`})
	if err == nil {
		t.Fatal("expected parse error, got nil")
	}
}

func TestManager_CancelExecution_StopsActiveProcess(t *testing.T) {
	// This test verifies CancelExecution returns a sensible error for an unknown execution ID.
	mgr := newTestManager(t, newTestBridge([]string{bridge.EngineClaude}, bridge.EngineClaude))

	err := mgr.CancelExecution(context.Background(), "nonexistent-exec-id")
	if err == nil {
		t.Error("expected error for unknown executionID, got nil")
	}
}

func TestManager_ExecuteWorker_StoresAndRunsResolvedEngine(t *testing.T) {
	br := newTestBridge([]string{bridge.EngineClaude, bridge.EngineCodex}, bridge.EngineCodex)
	mgr := newTestManager(t, br)

	w, err := mgr.CreateWorker(CreateWorkerParams{Name: "alice", Engine: "missing-engine"})
	if err != nil {
		t.Fatalf("CreateWorker: %v", err)
	}

	exec, err := mgr.ExecuteWorker(context.Background(), w.ID, "test", "session-1", false)
	if err != nil {
		t.Fatalf("ExecuteWorker: %v", err)
	}

	if exec.Engine != bridge.EngineCodex {
		t.Fatalf("execution stored engine %q, want %q", exec.Engine, bridge.EngineCodex)
	}
	if br.runRequest.WorkerEngine != bridge.EngineCodex {
		t.Fatalf("RunWorker request engine %q, want %q", br.runRequest.WorkerEngine, bridge.EngineCodex)
	}
}

// Regression: when a worker process exits without emitting Done/Error (killed,
// crashed, signal-terminated), monitorExecution must finalize the execution
// row instead of leaving it stuck in `running` forever.
func TestManager_MonitorExecution_SilentClose_FinalizesExecution(t *testing.T) {
	br := newTestBridge([]string{bridge.EngineClaude}, bridge.EngineClaude)
	ch := make(chan bridge.LifecycleEvent)
	close(ch)
	br.runHandle = bridge.RunHandle{
		Engine:        bridge.EngineClaude,
		Process:       &mockProcess{},
		Events:        ch,
		ExtractResult: func(string) string { return "" },
	}
	mgr := newTestManager(t, br)

	w, err := mgr.CreateWorker(CreateWorkerParams{Name: "alice"})
	if err != nil {
		t.Fatalf("CreateWorker: %v", err)
	}

	exec, err := mgr.ExecuteWorker(context.Background(), w.ID, "test", "session-1", false)
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
	mgr := newTestManager(t, newTestBridge([]string{bridge.EngineClaude}, bridge.EngineClaude))

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
	mgr := newTestManager(t, newTestBridge([]string{bridge.EngineClaude}, bridge.EngineClaude))

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
	mgr := newTestManagerWithBotNames(t, newTestBridge([]string{bridge.EngineClaude}, bridge.EngineClaude), []string{"feishu"})

	_, err := mgr.CreateWorker(CreateWorkerParams{Name: "feishu"})
	if err == nil {
		t.Fatal("expected error for bot name conflict, got nil")
	}
	if !errors.Is(err, ErrValidation) {
		t.Errorf("expected ErrValidation, got %v", err)
	}
}

func TestManager_ValidateWorkerName_BotNameConflict_CaseInsensitive(t *testing.T) {
	mgr := newTestManagerWithBotNames(t, newTestBridge([]string{bridge.EngineClaude}, bridge.EngineClaude), []string{"feishu"})

	_, err := mgr.CreateWorker(CreateWorkerParams{Name: "FEISHU"})
	if err == nil {
		t.Fatal("expected error for case-insensitive bot name conflict, got nil")
	}
	if !errors.Is(err, ErrValidation) {
		t.Errorf("expected ErrValidation, got %v", err)
	}
}

func TestManager_ValidateWorkerName_WhitespaceTrimmed(t *testing.T) {
	mgr := newTestManager(t, newTestBridge([]string{bridge.EngineClaude}, bridge.EngineClaude))

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
	mgr := newTestManager(t, newTestBridge([]string{bridge.EngineClaude}, bridge.EngineClaude))

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
	mgr := newTestManager(t, newTestBridge([]string{bridge.EngineClaude}, bridge.EngineClaude))

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
	mgr := newTestManager(t, newTestBridge([]string{bridge.EngineClaude, bridge.EngineCodex}, bridge.EngineClaude))

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
	mgr := newTestManager(t, newTestBridge([]string{bridge.EngineClaude}, bridge.EngineClaude))

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
	mgr := newTestManagerWithBotNames(t, newTestBridge([]string{bridge.EngineClaude}, bridge.EngineClaude), []string{"feishu"})

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
	mgr := newTestManagerWithBotNames(t, newTestBridge([]string{bridge.EngineClaude}, bridge.EngineClaude), []string{"feishu"})

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
	mgr := newTestManager(t, newTestBridge([]string{bridge.EngineClaude}, bridge.EngineClaude))

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
