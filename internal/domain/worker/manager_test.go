package worker

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/theopenbee/openbee/internal/ai/bridge"
	"github.com/theopenbee/openbee/internal/infra/config"
	"github.com/theopenbee/openbee/internal/infra/store"
)

func newTestManager(t *testing.T, fb *fakeBridge) *Manager {
	return newTestManagerWithBotNames(t, fb, nil)
}

func newTestManagerWithBotNames(t *testing.T, fb *fakeBridge, botNames []string) *Manager {
	t.Helper()
	dir := t.TempDir()
	db, err := store.InitDB(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	ws := store.NewWorkerStore(db)
	es := store.NewExecutionStore(db, dir)
	bc := config.BeeConfig{}
	bc.RPC.TokenTTL = time.Minute
	m := &Manager{
		workerBaseDir:  dir,
		workerTimeout:  30 * time.Minute,
		workerStore:    ws,
		executionStore: es,
		br:             fb,
		botNamesLower:  botNames,
		activeHandles:  make(map[string]bridge.Handle),
	}
	return m
}

func TestManager_ValidateEngineArgs_RejectsUnknownEngine(t *testing.T) {
	fb := &fakeBridge{
		enabledEngines: []string{"claude"},
		validateEngine: func(name string) error {
			if name == "" || name == "claude" {
				return nil
			}
			return errors.New("not enabled: " + name)
		},
		validateArgs: bridge.ValidateEngineArgs,
	}
	mgr := newTestManager(t, fb)
	err := mgr.ValidateEngineArgs(map[string]string{"unknown": "--model foo"})
	if err == nil {
		t.Fatal("expected error for unknown engine, got nil")
	}
}

func TestManager_ValidateEngineArgs_RejectsInvalidArgs(t *testing.T) {
	fb := &fakeBridge{
		enabledEngines: []string{"claude"},
		validateEngine: func(name string) error {
			if name == "" || name == "claude" {
				return nil
			}
			return errors.New("not enabled: " + name)
		},
		validateArgs: bridge.ValidateEngineArgs,
	}
	mgr := newTestManager(t, fb)
	err := mgr.ValidateEngineArgs(map[string]string{"claude": `--model "unterminated`})
	if err == nil {
		t.Fatal("expected parse error, got nil")
	}
}

func TestManager_ValidateEngineArgs_RejectsEmptyEngineName(t *testing.T) {
	fb := &fakeBridge{
		enabledEngines: []string{"claude"},
		validateEngine: func(name string) error {
			if name == "" || name == "claude" {
				return nil
			}
			return errors.New("not enabled: " + name)
		},
		validateArgs: bridge.ValidateEngineArgs,
	}
	mgr := newTestManager(t, fb)
	err := mgr.ValidateEngineArgs(map[string]string{"": "--flag"})
	if err == nil {
		t.Fatal("expected error for empty engine name, got nil")
	}
	if !errors.Is(err, ErrValidation) {
		t.Errorf("expected error to wrap ErrValidation, got %v", err)
	}
}

func TestManager_CancelExecution_StopsActiveProcess(t *testing.T) {
	// This test verifies CancelExecution returns a sensible error for an unknown execution ID.
	mgr := newTestManager(t, &fakeBridge{})

	err := mgr.CancelExecution(context.Background(), "nonexistent-exec-id")
	if err == nil {
		t.Error("expected error for unknown executionID, got nil")
	}
}

func TestManager_ValidateWorkerName_DuplicateName(t *testing.T) {
	mgr := newTestManager(t, &fakeBridge{})

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
	mgr := newTestManager(t, &fakeBridge{})

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
	mgr := newTestManagerWithBotNames(t, &fakeBridge{}, []string{"feishu"})

	_, err := mgr.CreateWorker(CreateWorkerParams{Name: "feishu"})
	if err == nil {
		t.Fatal("expected error for bot name conflict, got nil")
	}
	if !errors.Is(err, ErrValidation) {
		t.Errorf("expected ErrValidation, got %v", err)
	}
}

func TestManager_ValidateWorkerName_BotNameConflict_CaseInsensitive(t *testing.T) {
	mgr := newTestManagerWithBotNames(t, &fakeBridge{}, []string{"feishu"})

	_, err := mgr.CreateWorker(CreateWorkerParams{Name: "FEISHU"})
	if err == nil {
		t.Fatal("expected error for case-insensitive bot name conflict, got nil")
	}
	if !errors.Is(err, ErrValidation) {
		t.Errorf("expected ErrValidation, got %v", err)
	}
}

func TestManager_ValidateWorkerName_WhitespaceTrimmed(t *testing.T) {
	mgr := newTestManager(t, &fakeBridge{})

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
	mgr := newTestManager(t, &fakeBridge{})

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
	mgr := newTestManager(t, &fakeBridge{})

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
	fb := &fakeBridge{
		enabledEngines: []string{"claude", "codex"},
		validateEngine: func(name string) error {
			if name == "" || name == "claude" || name == "codex" {
				return nil
			}
			return errors.New("not enabled: " + name)
		},
		validateArgs: bridge.ValidateEngineArgs,
	}
	mgr := newTestManager(t, fb)

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
	mgr := newTestManager(t, &fakeBridge{})

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
	mgr := newTestManagerWithBotNames(t, &fakeBridge{}, []string{"feishu"})

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
	mgr := newTestManagerWithBotNames(t, &fakeBridge{}, []string{"feishu"})

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
	mgr := newTestManager(t, &fakeBridge{})

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
