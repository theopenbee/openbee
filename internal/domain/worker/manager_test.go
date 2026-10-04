package worker

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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

// waitForMonitors blocks until every monitorExecution goroutine started by m
// has finished. monitorExecution removes its activeProcesses entry only after
// its final DB writes, so an empty map means no write can race t.Cleanup's
// db.Close and TempDir removal (which otherwise fails with "directory not
// empty" when SQLite recreates its journal mid-cleanup).
func waitForMonitors(t *testing.T, m *Manager) {
	t.Helper()
	require.Eventually(t, func() bool {
		m.mu.RLock()
		defer m.mu.RUnlock()
		return len(m.activeProcesses) == 0
	}, 5*time.Second, 5*time.Millisecond, "monitorExecution goroutines did not finish")
}

func newTestManager(t *testing.T, engines map[string]ai.EngineAdapter, defaultEngine string) *Manager {
	return newTestManagerWithBotNames(t, engines, defaultEngine, nil)
}

func newTestManagerWithBotNames(t *testing.T, engines map[string]ai.EngineAdapter, defaultEngine string, botNames []string) *Manager {
	t.Helper()
	dir := t.TempDir()
	db, err := store.InitDB(filepath.Join(dir, "test.db"))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	ws := store.NewWorkerStore(db)
	es := store.NewExecutionStore(db, dir)
	const testKey = "0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f20"
	envSvc, err := env.NewService(store.NewEnvConfigStore(db), store.NewDepartmentStore(db), testKey)
	require.NoError(t, err)
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

// TestManager_ResolveEngine merges the engine-resolution scenarios that cover
// both resolveEngine and resolveEngineSelection. Each case builds its own
// manager and engine map.
func TestManager_ResolveEngine(t *testing.T) {
	cases := []struct {
		name string
		run  func(t *testing.T) (gotName string, gotEngine ai.EngineAdapter, err error, wantName string, wantEngine ai.EngineAdapter)
	}{
		{
			name: "KnownEngine",
			run: func(t *testing.T) (string, ai.EngineAdapter, error, string, ai.EngineAdapter) {
				claude := &mockEngine{}
				codex := &mockEngine{}
				mgr := newTestManager(t, map[string]ai.EngineAdapter{"claude": claude, "codex": codex}, "claude")
				name, got := mgr.resolveEngine(model.Worker{Engine: "codex"})
				return name, got, nil, "codex", codex
			},
		},
		{
			name: "EmptyEngine_FallsBackToDefault",
			run: func(t *testing.T) (string, ai.EngineAdapter, error, string, ai.EngineAdapter) {
				claude := &mockEngine{}
				mgr := newTestManager(t, map[string]ai.EngineAdapter{"claude": claude}, "claude")
				name, got := mgr.resolveEngine(model.Worker{Engine: ""})
				return name, got, nil, "claude", claude
			},
		},
		{
			name: "UnknownEngineUsesFallbackName",
			run: func(t *testing.T) (string, ai.EngineAdapter, error, string, ai.EngineAdapter) {
				claude := &mockEngine{}
				mgr := newTestManager(t, map[string]ai.EngineAdapter{"claude": claude}, "claude")
				name, got, err := mgr.resolveEngineSelection(model.Worker{Engine: "unknown-engine"})
				return name, got, err, "claude", claude
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotName, gotEngine, err, wantName, wantEngine := tc.run(t)
			require.NoError(t, err)
			require.Equal(t, wantName, gotName)
			assert.Same(t, wantEngine, gotEngine)
		})
	}
}

func TestManager_ResolveEngine_UnknownEngine_FallsBackToDefault(t *testing.T) {
	claude := &mockEngine{}
	engines := map[string]ai.EngineAdapter{"claude": claude}
	mgr := newTestManager(t, engines, "claude")

	w := model.Worker{Engine: "unknown-engine"}
	name, got := mgr.resolveEngine(w)
	require.Equal(t, "claude", name)
	assert.Same(t, claude, got)
}

func TestManager_ValidateEngineArgs_RejectsUnknownEngine(t *testing.T) {
	mgr := newTestManager(t, map[string]ai.EngineAdapter{ai.EngineClaude: &mockEngine{}}, ai.EngineClaude)
	err := mgr.ValidateEngineArgs(map[string]string{"unknown": "--model foo"})
	require.Error(t, err)
}

func TestManager_ValidateEngineArgs_RejectsInvalidArgs(t *testing.T) {
	mgr := newTestManager(t, map[string]ai.EngineAdapter{ai.EngineClaude: &mockEngine{}}, ai.EngineClaude)
	err := mgr.ValidateEngineArgs(map[string]string{"claude": `--model "unterminated`})
	require.Error(t, err)
}

func TestManager_CancelExecution_StopsActiveProcess(t *testing.T) {
	// This test verifies CancelExecution returns a sensible error for an unknown execution ID.
	engines := map[string]ai.EngineAdapter{ai.EngineClaude: &mockEngine{}}
	mgr := newTestManager(t, engines, ai.EngineClaude)

	err := mgr.CancelExecution(context.Background(), "nonexistent-exec-id")
	assert.Error(t, err)
}

// Regression: when a worker process exits without emitting Done/Error (killed,
// crashed, signal-terminated), monitorExecution must finalize the execution
// row instead of leaving it stuck in `running` forever.
func TestManager_MonitorExecution_SilentClose_FinalizesExecution(t *testing.T) {
	engines := map[string]ai.EngineAdapter{ai.EngineClaude: &silentMockEngine{}}
	mgr := newTestManager(t, engines, ai.EngineClaude)

	w, err := mgr.CreateWorker(CreateWorkerParams{Name: "alice"})
	require.NoError(t, err)

	exec, err := mgr.ExecuteWorker(context.Background(), ExecuteRequest{
		WorkerID:     w.ID,
		TriggerInput: "test",
		SessionID:    "session-1",
	})
	require.NoError(t, err)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		got, err := mgr.executionStore.GetByID(exec.ID)
		if err == nil && got.Status == model.ExecStatusFailed && got.CompletedAt != nil {
			require.NotEmpty(t, got.Result, "expected non-empty result on abandoned execution")
			// The worker-status write still follows MarkAbandoned.
			waitForMonitors(t, mgr)
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	got, _ := mgr.executionStore.GetByID(exec.ID)
	t.Fatalf("execution not finalized after 2s; status=%s completedAt=%v", got.Status, got.CompletedAt)
}

// TestManager_ValidateWorkerName_Rejects merges the duplicate/conflict name
// scenarios that must all be rejected with ErrValidation, whether triggered
// via CreateWorker or via a rename through UpdateWorker.
func TestManager_ValidateWorkerName_Rejects(t *testing.T) {
	cases := []struct {
		name          string
		existingNames []string // workers created up front, in order
		botNames      []string
		renameIdx     int // -1: create a new worker named targetName; >=0: rename existingNames[renameIdx]
		targetName    string
	}{
		{name: "ValidateWorkerName_DuplicateName", existingNames: []string{"alice"}, renameIdx: -1, targetName: "alice"},
		{name: "ValidateWorkerName_CaseInsensitiveDuplicate", existingNames: []string{"Alice"}, renameIdx: -1, targetName: "alice"},
		{name: "ValidateWorkerName_WhitespaceTrimmed", existingNames: []string{"alice"}, renameIdx: -1, targetName: " alice "},
		{name: "UpdateWorker_RenameToDifferentName_Duplicate", existingNames: []string{"alice", "bob"}, renameIdx: 0, targetName: "bob"},
		{name: "ValidateWorkerName_BotNameConflict", botNames: []string{"feishu"}, renameIdx: -1, targetName: "feishu"},
		{name: "ValidateWorkerName_BotNameConflict_CaseInsensitive", botNames: []string{"feishu"}, renameIdx: -1, targetName: "FEISHU"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			engines := map[string]ai.EngineAdapter{ai.EngineClaude: &mockEngine{}}
			var mgr *Manager
			if len(tc.botNames) > 0 {
				mgr = newTestManagerWithBotNames(t, engines, ai.EngineClaude, tc.botNames)
			} else {
				mgr = newTestManager(t, engines, ai.EngineClaude)
			}
			ids := make([]string, len(tc.existingNames))
			for i, n := range tc.existingNames {
				w, err := mgr.CreateWorker(CreateWorkerParams{Name: n})
				require.NoError(t, err)
				ids[i] = w.ID
			}

			var err error
			if tc.renameIdx >= 0 {
				newName := tc.targetName
				_, err = mgr.UpdateWorker(ids[tc.renameIdx], UpdateWorkerParams{Name: &newName})
			} else {
				_, err = mgr.CreateWorker(CreateWorkerParams{Name: tc.targetName})
			}

			require.Error(t, err)
			assert.ErrorIs(t, err, ErrValidation)
		})
	}
}

func TestManager_UpdateWorker_RenameToSameName_Succeeds(t *testing.T) {
	engines := map[string]ai.EngineAdapter{ai.EngineClaude: &mockEngine{}}
	mgr := newTestManager(t, engines, ai.EngineClaude)

	w, err := mgr.CreateWorker(CreateWorkerParams{Name: "alice"})
	require.NoError(t, err)

	sameName := "alice"
	_, err = mgr.UpdateWorker(w.ID, UpdateWorkerParams{Name: &sameName})
	assert.NoError(t, err)
}

func TestManager_UpdateWorker_EmptyEngineArgsClearsAll(t *testing.T) {
	engines := map[string]ai.EngineAdapter{ai.EngineClaude: &mockEngine{}, ai.EngineCodex: &mockEngine{}}
	mgr := newTestManager(t, engines, ai.EngineClaude)

	w, err := mgr.CreateWorker(CreateWorkerParams{
		Name:       "alice",
		EngineArgs: `{"claude":"--model sonnet","codex":"--model o3"}`,
	})
	require.NoError(t, err)

	updated, err := mgr.UpdateWorker(w.ID, UpdateWorkerParams{
		EngineArgs: map[string]string{},
	})
	require.NoError(t, err)
	require.Equal(t, "{}", updated.EngineArgs)
}

func TestManager_UpdateWorker_RenameToCaseVariant_Succeeds(t *testing.T) {
	engines := map[string]ai.EngineAdapter{ai.EngineClaude: &mockEngine{}}
	mgr := newTestManager(t, engines, ai.EngineClaude)

	w, err := mgr.CreateWorker(CreateWorkerParams{Name: "alice"})
	require.NoError(t, err)

	upperName := "ALICE"
	_, err = mgr.UpdateWorker(w.ID, UpdateWorkerParams{Name: &upperName})
	assert.NoError(t, err)
}

func TestManager_UpdateWorker_RenameToBotName_Rejected(t *testing.T) {
	engines := map[string]ai.EngineAdapter{ai.EngineClaude: &mockEngine{}}
	mgr := newTestManagerWithBotNames(t, engines, ai.EngineClaude, []string{"feishu"})

	w, err := mgr.CreateWorker(CreateWorkerParams{Name: "alice"})
	require.NoError(t, err)

	botName := "feishu"
	_, err = mgr.UpdateWorker(w.ID, UpdateWorkerParams{Name: &botName})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrValidation)
}

func TestManager_UpdateWorker_RenameToBotNameCaseVariant_Rejected(t *testing.T) {
	engines := map[string]ai.EngineAdapter{ai.EngineClaude: &mockEngine{}}
	mgr := newTestManagerWithBotNames(t, engines, ai.EngineClaude, []string{"feishu"})

	w, err := mgr.CreateWorker(CreateWorkerParams{Name: "alice"})
	require.NoError(t, err)

	upperBotName := "FEISHU"
	_, err = mgr.UpdateWorker(w.ID, UpdateWorkerParams{Name: &upperBotName})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrValidation)
}

func TestManager_UpdateWorker_NoNameChange_Succeeds(t *testing.T) {
	engines := map[string]ai.EngineAdapter{ai.EngineClaude: &mockEngine{}}
	mgr := newTestManager(t, engines, ai.EngineClaude)

	w, err := mgr.CreateWorker(CreateWorkerParams{Name: "alice", Description: "original"})
	require.NoError(t, err)

	newDesc := "updated"
	updated, err := mgr.UpdateWorker(w.ID, UpdateWorkerParams{Description: &newDesc})
	assert.NoError(t, err)
	assert.Equal(t, "updated", updated.Description)
	assert.Equal(t, "alice", updated.Name)
}

// TestUpdateWorker_WorkDir merges the WorkDir update scenarios. Each case
// builds its own worker and performs its own UpdateWorker call; OldDirUntouched
// checks a side effect (the old directory's file survives) instead of the
// returned WorkDir.
func TestUpdateWorker_WorkDir(t *testing.T) {
	type result struct {
		params      UpdateWorkerParams
		wantWorkDir string // ignored when wantOldDirMarker is true
	}
	cases := []struct {
		name             string
		workerName       string
		build            func(t *testing.T, w model.Worker) result
		wantOldDirMarker bool
	}{
		{
			name:       "Success",
			workerName: "wd-success",
			build: func(t *testing.T, w model.Worker) result {
				newDir := filepath.Join(t.TempDir(), "moved")
				return result{params: UpdateWorkerParams{WorkDir: &newDir}, wantWorkDir: newDir}
			},
		},
		{
			name:       "Nil_NoChange",
			workerName: "wd-nil",
			build: func(t *testing.T, w model.Worker) result {
				newName := "wd-nil-renamed"
				return result{params: UpdateWorkerParams{Name: &newName}, wantWorkDir: w.WorkDir}
			},
		},
		{
			name:       "TrimWhitespace",
			workerName: "wd-trim",
			build: func(t *testing.T, w model.Worker) result {
				padded := "   /tmp/openbee-x   "
				return result{params: UpdateWorkerParams{WorkDir: &padded}, wantWorkDir: "/tmp/openbee-x"}
			},
		},
		{
			name:       "OldDirUntouched",
			workerName: "wd-old",
			build: func(t *testing.T, w model.Worker) result {
				require.NoError(t, os.WriteFile(filepath.Join(w.WorkDir, "marker.txt"), []byte("keep me"), 0644))
				newDir := filepath.Join(t.TempDir(), "elsewhere")
				return result{params: UpdateWorkerParams{WorkDir: &newDir}}
			},
			wantOldDirMarker: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			engines := map[string]ai.EngineAdapter{ai.EngineClaude: &mockEngine{}}
			m := newTestManager(t, engines, ai.EngineClaude)
			w, err := m.CreateWorker(CreateWorkerParams{Name: tc.workerName, Engine: ai.EngineClaude})
			require.NoError(t, err)

			r := tc.build(t, w)
			got, err := m.UpdateWorker(w.ID, r.params)
			require.NoError(t, err)

			if tc.wantOldDirMarker {
				_, err := os.Stat(filepath.Join(w.WorkDir, "marker.txt"))
				require.NoError(t, err)
				return
			}
			require.Equal(t, r.wantWorkDir, got.WorkDir)
		})
	}
}

func TestUpdateWorker_WorkDir_EmptyAfterTrim(t *testing.T) {
	engines := map[string]ai.EngineAdapter{ai.EngineClaude: &mockEngine{}}
	m := newTestManager(t, engines, ai.EngineClaude)

	w, err := m.CreateWorker(CreateWorkerParams{Name: "wd-empty", Engine: ai.EngineClaude})
	require.NoError(t, err)

	blank := "   "
	_, err = m.UpdateWorker(w.ID, UpdateWorkerParams{WorkDir: &blank})
	require.Error(t, err)
	require.ErrorIs(t, err, ErrValidation)
}

func TestManager_Execute_CreatesNewWorkDirIfMissing(t *testing.T) {
	engines := map[string]ai.EngineAdapter{ai.EngineClaude: &mockEngine{}}
	m := newTestManager(t, engines, ai.EngineClaude)

	w, err := m.CreateWorker(CreateWorkerParams{Name: "wd-exec-mkdir"})
	require.NoError(t, err)

	missing := filepath.Join(t.TempDir(), "deep", "nested", "missing")
	_, err = m.UpdateWorker(w.ID, UpdateWorkerParams{WorkDir: &missing})
	require.NoError(t, err)
	_, err = os.Stat(missing)
	require.True(t, os.IsNotExist(err), "precondition: missing dir should not exist")

	// ExecuteWorker may fail later (no real engine subprocess), but it MUST create the dir first.
	_, _ = m.ExecuteWorker(context.Background(), ExecuteRequest{
		WorkerID:     w.ID,
		SessionID:    "test-session",
		TriggerInput: "noop",
	})
	waitForMonitors(t, m)

	_, err = os.Stat(missing)
	require.NoError(t, err, "expected execute to create WorkDir")
}

func TestCreateWorker_WorkDir_TrimWhitespace(t *testing.T) {
	engines := map[string]ai.EngineAdapter{ai.EngineClaude: &mockEngine{}}
	m := newTestManager(t, engines, ai.EngineClaude)

	want := filepath.Join(t.TempDir(), "trimmed")
	padded := "   " + want + "   "

	w, err := m.CreateWorker(CreateWorkerParams{Name: "create-wd-trim", WorkDir: padded, Engine: ai.EngineClaude})
	require.NoError(t, err)
	require.Equal(t, want, w.WorkDir)
}

func TestCreateWorker_WorkDir_BlankFallsBackToDefault(t *testing.T) {
	engines := map[string]ai.EngineAdapter{ai.EngineClaude: &mockEngine{}}
	m := newTestManager(t, engines, ai.EngineClaude)

	w, err := m.CreateWorker(CreateWorkerParams{Name: "create-wd-blank", WorkDir: "   ", Engine: ai.EngineClaude})
	require.NoError(t, err)
	require.Equal(t, m.workerBaseDir, filepath.Dir(w.WorkDir))
}
