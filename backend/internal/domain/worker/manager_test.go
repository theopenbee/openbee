package worker

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ai "github.com/theopenbee/openbee/internal/ai"
	"github.com/theopenbee/openbee/internal/ai/aitest"
	"github.com/theopenbee/openbee/internal/domain/enginecfg"
	"github.com/theopenbee/openbee/internal/domain/env"
	"github.com/theopenbee/openbee/internal/infra/auth"
	"github.com/theopenbee/openbee/internal/infra/config"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/infra/store"
)

type mockEngine struct{}

func (e *mockEngine) Run(_ context.Context, _, _ string, _ ai.RunOptions, _ string) (ai.RunResult, error) {
	ch := make(chan ai.Output, 1)
	ch <- ai.Output{Type: ai.OutputDone}
	close(ch)
	return ai.RunResult{Process: &mockProcess{}, Output: ch, ExtractResult: func(string) string { return "" }}, nil
}

func (m *mockEngine) CollectTokenUsage(_ context.Context, _ []string) (map[string][]ai.TokenUsage, error) {
	return nil, nil
}

type mockProcess struct{}

func (p *mockProcess) PID() int    { return 0 }
func (p *mockProcess) Stop() error { return nil }

// silentMockEngine simulates a process whose output channel closes without
// emitting a terminal Done/Error signal — the abandoned-process scenario.
type silentMockEngine struct{}

func (e *silentMockEngine) Run(_ context.Context, _, _ string, _ ai.RunOptions, _ string) (ai.RunResult, error) {
	ch := make(chan ai.Output)
	close(ch)
	return ai.RunResult{Process: &mockProcess{}, Output: ch, ExtractResult: func(string) string { return "" }}, nil
}

func (e *silentMockEngine) CollectTokenUsage(_ context.Context, _ []string) (map[string][]ai.TokenUsage, error) {
	return nil, nil
}

func newTestManager(t *testing.T, engines map[string]ai.EngineAdapter, defaultEngine string) *Manager {
	return newTestManagerWithSource(t, aitest.NewSource(engines), defaultEngine, nil)
}

func newTestManagerWithBotNames(t *testing.T, engines map[string]ai.EngineAdapter, defaultEngine string, botNames []string) *Manager {
	return newTestManagerWithSource(t, aitest.NewSource(engines), defaultEngine, botNames)
}

func newTestManagerWithSource(t *testing.T, src *aitest.Source, defaultEngine string, botNames []string) *Manager {
	t.Helper()
	dir := t.TempDir()
	db, err := store.InitDB(filepath.Join(dir, "test.db"))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	ws := store.NewWorkerStore(db)
	es := store.NewExecutionStore(db, dir)
	const testKey = "0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f20"
	envSvc, err := env.NewService(store.NewEnvConfigStore(db), testKey)
	require.NoError(t, err)
	bc := config.BeeConfig{}
	m := &Manager{
		workerBaseDir:   dir,
		tokenSecret:     bc.RPC.TokenSecret,
		workerTimeout:   30 * time.Minute,
		workerStore:     ws,
		executionStore:  es,
		engines:         src,
		engineCfg:       enginecfg.NewStore(defaultEngine, "", src.IDs),
		envService:      envSvc,
		botNames:        func() []string { return botNames },
		activeProcesses: make(map[string]ai.Process),
		monitorDone:     make(map[string]chan struct{}),
	}
	return m
}

func TestManager_ResolveEngine(t *testing.T) {
	claude := &aitest.Engine{}
	codex := &aitest.Engine{}
	mgr := newTestManager(t, map[string]ai.EngineAdapter{"claude": claude, "codex": codex}, "claude")

	name, got, err := mgr.resolveEngine(model.Worker{Engine: "codex"})
	require.NoError(t, err)
	assert.Equal(t, "codex", name)
	assert.Same(t, codex, got)

	name, got, err = mgr.resolveEngine(model.Worker{})
	require.NoError(t, err)
	assert.Equal(t, "claude", name)
	assert.Same(t, claude, got)
}

func TestResolveEngine_WorkerEngineGone(t *testing.T) {
	pi := &aitest.Engine{RunFunc: (&mockEngine{}).Run}
	codex := &aitest.Engine{RunFunc: (&mockEngine{}).Run}
	src := aitest.NewSource(map[string]ai.EngineAdapter{"pi": pi, "codex": codex})
	mgr := newTestManagerWithSource(t, src, "pi", nil)

	coder, err := mgr.CreateWorker(CreateWorkerParams{Name: "coder", Engine: "codex"})
	require.NoError(t, err)
	helper, err := mgr.CreateWorker(CreateWorkerParams{Name: "helper"})
	require.NoError(t, err)

	src.Remove("codex")

	name, got, err := mgr.resolveEngine(coder)
	require.EqualError(t, err, `engine "codex" is not installed or enabled`)
	assert.Equal(t, "codex", name)
	assert.Nil(t, got)

	exec, err := mgr.ExecuteWorker(context.Background(), ExecuteRequest{WorkerID: coder.ID, TriggerInput: "hi", SessionID: "s1"})
	require.EqualError(t, err, `engine "codex" is not installed or enabled`)
	assert.True(t, ai.IsEngineMissing(err))
	stored, err := mgr.executionStore.GetByID(exec.ID)
	require.NoError(t, err)
	assert.Equal(t, model.ExecStatusFailed, stored.Status)
	assert.Equal(t, `engine "codex" is not installed or enabled`, stored.Result)
	assert.Equal(t, "codex", stored.Engine)
	assert.Empty(t, pi.Runs())
	assert.Empty(t, codex.Runs())

	_, err = mgr.ExecuteWorker(context.Background(), ExecuteRequest{WorkerID: helper.ID, TriggerInput: "hi", SessionID: "s2"})
	require.NoError(t, err)
	require.Len(t, pi.Runs(), 1)
	assert.Equal(t, ai.RoleWorker, pi.Runs()[0].Role)
}

func TestExecuteWorker_OtherStartErrorsStayWrapped(t *testing.T) {
	pi := &aitest.Engine{RunFunc: func(context.Context, string, string, ai.RunOptions, string) (ai.RunResult, error) {
		return ai.RunResult{}, errors.New("spawn failed")
	}}
	src := aitest.NewSource(map[string]ai.EngineAdapter{"pi": pi})
	mgr := newTestManagerWithSource(t, src, "pi", nil)
	w, err := mgr.CreateWorker(CreateWorkerParams{Name: "helper"})
	require.NoError(t, err)

	_, err = mgr.ExecuteWorker(context.Background(), ExecuteRequest{WorkerID: w.ID, TriggerInput: "hi", SessionID: "s1"})
	require.EqualError(t, err, "start runtime: spawn failed")
	assert.False(t, ai.IsEngineMissing(err))
}

func TestExecuteWorker_TokenOutlivesWorkerTimeout(t *testing.T) {
	pi := &aitest.Engine{RunFunc: (&mockEngine{}).Run}
	src := aitest.NewSource(map[string]ai.EngineAdapter{"pi": pi})
	mgr := newTestManagerWithSource(t, src, "pi", nil)
	mgr.tokenSecret = "test-secret"
	mgr.workerTimeout = 24 * time.Hour
	w, err := mgr.CreateWorker(CreateWorkerParams{Name: "helper"})
	require.NoError(t, err)

	_, err = mgr.ExecuteWorker(context.Background(), ExecuteRequest{WorkerID: w.ID, TriggerInput: "hi", SessionID: "s1"})
	require.NoError(t, err)

	require.Len(t, pi.Runs(), 1)
	claims, err := auth.ValidateToken(pi.Runs()[0].APIKey, "test-secret")
	require.NoError(t, err)
	assert.True(t, claims.ExpiresAt.After(time.Now().Add(24*time.Hour)), "token expires at %s", claims.ExpiresAt)
}

func TestResolveEngine_NoEngine(t *testing.T) {
	mgr := newTestManager(t, nil, "claude")
	w, err := mgr.CreateWorker(CreateWorkerParams{Name: "idle"})
	require.NoError(t, err)

	_, _, err = mgr.resolveEngine(w)
	require.ErrorIs(t, err, ai.ErrNoEngine)

	_, err = mgr.ExecuteWorker(context.Background(), ExecuteRequest{WorkerID: w.ID, TriggerInput: "hi", SessionID: "s1"})
	require.ErrorIs(t, err, ai.ErrNoEngine)
}

func TestValidateEngine_UsesSource(t *testing.T) {
	src := aitest.NewSource(map[string]ai.EngineAdapter{"pi": &aitest.Engine{}})
	mgr := newTestManagerWithSource(t, src, "", nil)

	require.NoError(t, mgr.ValidateEngine(""))
	require.NoError(t, mgr.ValidateEngine("pi"))
	require.EqualError(t, mgr.ValidateEngine("codex"), `engine "codex" is not installed or enabled`)
	assert.Equal(t, []string{"pi"}, mgr.EnabledEngines())

	src.Add("codex", map[string]string{"en": "Codex"}, &aitest.Engine{})
	require.NoError(t, mgr.ValidateEngine("codex"))
	assert.Equal(t, []string{"codex", "pi"}, mgr.EnabledEngines())

	src.Remove("pi")
	require.Error(t, mgr.ValidateEngine("pi"))
	require.Error(t, mgr.ValidateEngineArgs(map[string]string{"pi": "--model x"}))
	assert.Equal(t, []string{"codex"}, mgr.EnabledEngines())
}

func newUninstalledEngineWorker(t *testing.T) (*Manager, model.Worker) {
	t.Helper()
	src := aitest.NewSource(map[string]ai.EngineAdapter{"pi": &aitest.Engine{}, "codex": &aitest.Engine{}})
	mgr := newTestManagerWithSource(t, src, "pi", nil)
	w, err := mgr.CreateWorker(CreateWorkerParams{
		Name:       "coder",
		Engine:     "codex",
		EngineArgs: `{"codex":"--model o3","pi":"--model x"}`,
	})
	require.NoError(t, err)
	src.Remove("codex")
	return mgr, w
}

func strPtr(s string) *string { return &s }

func TestUpdateWorker_UninstalledEngine(t *testing.T) {
	t.Run("UnchangedEngineAndArgsSucceeds", func(t *testing.T) {
		mgr, w := newUninstalledEngineWorker(t)
		updated, err := mgr.UpdateWorker(w.ID, UpdateWorkerParams{
			Description: strPtr("edited"),
			Engine:      strPtr("codex"),
			EngineArgs:  map[string]string{"codex": "--model o3", "pi": "--model x"},
		})
		require.NoError(t, err)
		assert.Equal(t, "edited", updated.Description)
		assert.Equal(t, "codex", updated.Engine)
		assert.JSONEq(t, `{"codex":"--model o3","pi":"--model x"}`, updated.EngineArgs)
	})

	t.Run("RemoveArgsKeySucceeds", func(t *testing.T) {
		mgr, w := newUninstalledEngineWorker(t)
		updated, err := mgr.UpdateWorker(w.ID, UpdateWorkerParams{
			EngineArgs: map[string]string{"codex": "", "pi": "--model x"},
		})
		require.NoError(t, err)
		assert.JSONEq(t, `{"pi":"--model x"}`, updated.EngineArgs)
	})

	t.Run("ChangeToUninstalledEngineFails", func(t *testing.T) {
		mgr, w := newUninstalledEngineWorker(t)
		_, err := mgr.UpdateWorker(w.ID, UpdateWorkerParams{Engine: strPtr("pi")})
		require.NoError(t, err)
		_, err = mgr.UpdateWorker(w.ID, UpdateWorkerParams{Engine: strPtr("codex")})
		require.ErrorIs(t, err, ErrValidation)
		assert.True(t, ai.IsEngineMissing(err))
		assert.Contains(t, err.Error(), `engine "codex" is not installed or enabled`)
	})

	t.Run("AddArgsKeyForUninstalledEngineFails", func(t *testing.T) {
		mgr, w := newUninstalledEngineWorker(t)
		_, err := mgr.UpdateWorker(w.ID, UpdateWorkerParams{EngineArgs: map[string]string{"claude": "--model sonnet"}})
		require.ErrorIs(t, err, ErrValidation)
		assert.Contains(t, err.Error(), `engine "claude" is not installed or enabled`)
	})

	t.Run("ChangeArgsForUninstalledEngineFails", func(t *testing.T) {
		mgr, w := newUninstalledEngineWorker(t)
		_, err := mgr.UpdateWorker(w.ID, UpdateWorkerParams{EngineArgs: map[string]string{"codex": "--model o4"}})
		require.ErrorIs(t, err, ErrValidation)
		assert.Contains(t, err.Error(), `engine "codex" is not installed or enabled`)
	})

	t.Run("ChangedArgsStillParsed", func(t *testing.T) {
		mgr, w := newUninstalledEngineWorker(t)
		_, err := mgr.UpdateWorker(w.ID, UpdateWorkerParams{EngineArgs: map[string]string{"pi": `--model "unterminated`}})
		require.ErrorIs(t, err, ErrValidation)
	})
}

func TestManager_ValidateEngineArgs_RejectsUnknownEngine(t *testing.T) {
	mgr := newTestManager(t, map[string]ai.EngineAdapter{"claude": &mockEngine{}}, "claude")
	err := mgr.ValidateEngineArgs(map[string]string{"unknown": "--model foo"})
	require.Error(t, err)
}

func TestManager_ValidateEngineArgs_RejectsInvalidArgs(t *testing.T) {
	mgr := newTestManager(t, map[string]ai.EngineAdapter{"claude": &mockEngine{}}, "claude")
	err := mgr.ValidateEngineArgs(map[string]string{"claude": `--model "unterminated`})
	require.Error(t, err)
}

func TestManager_CancelExecution_StopsActiveProcess(t *testing.T) {
	// This test verifies CancelExecution returns a sensible error for an unknown execution ID.
	engines := map[string]ai.EngineAdapter{"claude": &mockEngine{}}
	mgr := newTestManager(t, engines, "claude")

	err := mgr.CancelExecution(context.Background(), "nonexistent-exec-id")
	assert.Error(t, err)
}

// Regression: when a worker process exits without emitting Done/Error (killed,
// crashed, signal-terminated), monitorExecution must finalize the execution
// row instead of leaving it stuck in `running` forever.
func TestManager_MonitorExecution_SilentClose_FinalizesExecution(t *testing.T) {
	engines := map[string]ai.EngineAdapter{"claude": &silentMockEngine{}}
	mgr := newTestManager(t, engines, "claude")

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
		{name: "ValidateWorkerName_BotNameConflict", botNames: []string{"beta"}, renameIdx: -1, targetName: "beta"},
		{name: "ValidateWorkerName_BotNameConflict_CaseInsensitive", botNames: []string{"beta"}, renameIdx: -1, targetName: "BETA"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			engines := map[string]ai.EngineAdapter{"claude": &mockEngine{}}
			var mgr *Manager
			if len(tc.botNames) > 0 {
				mgr = newTestManagerWithBotNames(t, engines, "claude", tc.botNames)
			} else {
				mgr = newTestManager(t, engines, "claude")
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
	engines := map[string]ai.EngineAdapter{"claude": &mockEngine{}}
	mgr := newTestManager(t, engines, "claude")

	w, err := mgr.CreateWorker(CreateWorkerParams{Name: "alice"})
	require.NoError(t, err)

	sameName := "alice"
	_, err = mgr.UpdateWorker(w.ID, UpdateWorkerParams{Name: &sameName})
	assert.NoError(t, err)
}

func TestManager_UpdateWorker_EmptyEngineArgsClearsAll(t *testing.T) {
	engines := map[string]ai.EngineAdapter{"claude": &mockEngine{}, "codex": &mockEngine{}}
	mgr := newTestManager(t, engines, "claude")

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
	engines := map[string]ai.EngineAdapter{"claude": &mockEngine{}}
	mgr := newTestManager(t, engines, "claude")

	w, err := mgr.CreateWorker(CreateWorkerParams{Name: "alice"})
	require.NoError(t, err)

	upperName := "ALICE"
	_, err = mgr.UpdateWorker(w.ID, UpdateWorkerParams{Name: &upperName})
	assert.NoError(t, err)
}

func TestManager_UpdateWorker_RenameToBotName_Rejected(t *testing.T) {
	engines := map[string]ai.EngineAdapter{"claude": &mockEngine{}}
	mgr := newTestManagerWithBotNames(t, engines, "claude", []string{"beta"})

	w, err := mgr.CreateWorker(CreateWorkerParams{Name: "alice"})
	require.NoError(t, err)

	botName := "beta"
	_, err = mgr.UpdateWorker(w.ID, UpdateWorkerParams{Name: &botName})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrValidation)
}

func TestManager_UpdateWorker_RenameToBotNameCaseVariant_Rejected(t *testing.T) {
	engines := map[string]ai.EngineAdapter{"claude": &mockEngine{}}
	mgr := newTestManagerWithBotNames(t, engines, "claude", []string{"beta"})

	w, err := mgr.CreateWorker(CreateWorkerParams{Name: "alice"})
	require.NoError(t, err)

	upperBotName := "BETA"
	_, err = mgr.UpdateWorker(w.ID, UpdateWorkerParams{Name: &upperBotName})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrValidation)
}

func TestManager_UpdateWorker_NoNameChange_Succeeds(t *testing.T) {
	engines := map[string]ai.EngineAdapter{"claude": &mockEngine{}}
	mgr := newTestManager(t, engines, "claude")

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
			engines := map[string]ai.EngineAdapter{"claude": &mockEngine{}}
			m := newTestManager(t, engines, "claude")
			w, err := m.CreateWorker(CreateWorkerParams{Name: tc.workerName, Engine: "claude"})
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
	engines := map[string]ai.EngineAdapter{"claude": &mockEngine{}}
	m := newTestManager(t, engines, "claude")

	w, err := m.CreateWorker(CreateWorkerParams{Name: "wd-empty", Engine: "claude"})
	require.NoError(t, err)

	blank := "   "
	_, err = m.UpdateWorker(w.ID, UpdateWorkerParams{WorkDir: &blank})
	require.Error(t, err)
	require.ErrorIs(t, err, ErrValidation)
}

func TestManager_Execute_CreatesNewWorkDirIfMissing(t *testing.T) {
	engines := map[string]ai.EngineAdapter{"claude": &mockEngine{}}
	m := newTestManager(t, engines, "claude")

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

	_, err = os.Stat(missing)
	require.NoError(t, err, "expected execute to create WorkDir")
}

func TestCreateWorker_WorkDir_TrimWhitespace(t *testing.T) {
	engines := map[string]ai.EngineAdapter{"claude": &mockEngine{}}
	m := newTestManager(t, engines, "claude")

	want := filepath.Join(t.TempDir(), "trimmed")
	padded := "   " + want + "   "

	w, err := m.CreateWorker(CreateWorkerParams{Name: "create-wd-trim", WorkDir: padded, Engine: "claude"})
	require.NoError(t, err)
	require.Equal(t, want, w.WorkDir)
}

func TestCreateWorker_WorkDir_BlankFallsBackToDefault(t *testing.T) {
	engines := map[string]ai.EngineAdapter{"claude": &mockEngine{}}
	m := newTestManager(t, engines, "claude")

	w, err := m.CreateWorker(CreateWorkerParams{Name: "create-wd-blank", WorkDir: "   ", Engine: "claude"})
	require.NoError(t, err)
	require.Equal(t, m.workerBaseDir, filepath.Dir(w.WorkDir))
}

func TestManager_ValidateWorkerNameUsesBotNameSource(t *testing.T) {
	engines := map[string]ai.EngineAdapter{"claude": &mockEngine{}}
	mgr := newTestManager(t, engines, "claude")
	_, err := mgr.CreateWorker(CreateWorkerParams{Name: "gamma"})
	require.NoError(t, err)

	names := []string{" Delta "}
	mgr.SetBotNameSource(func() []string { return names })
	_, err = mgr.CreateWorker(CreateWorkerParams{Name: "delta"})
	require.ErrorIs(t, err, ErrValidation)

	names = nil
	_, err = mgr.CreateWorker(CreateWorkerParams{Name: "delta"})
	require.NoError(t, err)
}

func TestManagerStatusObserver(t *testing.T) {
	t.Run("ReportsExecutionStatuses", func(t *testing.T) {
		engines := map[string]ai.EngineAdapter{"claude": &mockEngine{}}
		mgr := newTestManager(t, engines, "claude")
		w, err := mgr.CreateWorker(CreateWorkerParams{Name: "observed"})
		require.NoError(t, err)

		var mu sync.Mutex
		var got []model.WorkerStatus
		mgr.SetStatusObserver(func(workerID string, status model.WorkerStatus) {
			mu.Lock()
			defer mu.Unlock()
			assert.Equal(t, w.ID, workerID)
			got = append(got, status)
		})

		_, err = mgr.ExecuteWorker(context.Background(), ExecuteRequest{
			WorkerID:     w.ID,
			TriggerInput: "hello",
			SessionID:    "session-1",
		})
		require.NoError(t, err)

		require.Eventually(t, func() bool {
			mu.Lock()
			defer mu.Unlock()
			return len(got) == 2
		}, 2*time.Second, 10*time.Millisecond)
		mu.Lock()
		defer mu.Unlock()
		assert.Equal(t, []model.WorkerStatus{model.WorkerStatusWorking, model.WorkerStatusIdle}, got)
	})

	t.Run("SkipsFailedUpdate", func(t *testing.T) {
		mgr := newTestManager(t, map[string]ai.EngineAdapter{"claude": &mockEngine{}}, "claude")
		db, err := store.InitDB(filepath.Join(t.TempDir(), "closed.db"))
		require.NoError(t, err)
		require.NoError(t, db.Close())
		mgr.workerStore = store.NewWorkerStore(db)

		calls := 0
		mgr.SetStatusObserver(func(string, model.WorkerStatus) { calls++ })

		require.Error(t, mgr.setStatus("w1", model.WorkerStatusIdle))
		assert.Zero(t, calls)
	})
}

type heldEngine struct {
	out chan ai.Output
}

func (e *heldEngine) Run(_ context.Context, _, _ string, _ ai.RunOptions, _ string) (ai.RunResult, error) {
	return ai.RunResult{Process: &mockProcess{}, Output: e.out, ExtractResult: func(string) string { return "" }}, nil
}

func (e *heldEngine) CollectTokenUsage(_ context.Context, _ []string) (map[string][]ai.TokenUsage, error) {
	return nil, nil
}

type statusLog struct {
	mu       sync.Mutex
	statuses []model.WorkerStatus
}

func (l *statusLog) record(_ string, status model.WorkerStatus) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.statuses = append(l.statuses, status)
}

func (l *statusLog) list() []model.WorkerStatus {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]model.WorkerStatus(nil), l.statuses...)
}

type stoppedRun struct {
	mgr    *Manager
	engine *heldEngine
	worker model.Worker
	exec   model.WorkerExecution
	log    *statusLog
}

func startStoppedRun(t *testing.T) *stoppedRun {
	t.Helper()
	engine := &heldEngine{out: make(chan ai.Output, 1)}
	mgr := newTestManager(t, map[string]ai.EngineAdapter{"claude": engine}, "claude")
	w, err := mgr.CreateWorker(CreateWorkerParams{Name: "stopped"})
	require.NoError(t, err)
	log := &statusLog{}
	mgr.SetStatusObserver(log.record)
	exec, err := mgr.ExecuteWorker(context.Background(), ExecuteRequest{WorkerID: w.ID, TriggerInput: "sleep", SessionID: "s1"})
	require.NoError(t, err)
	return &stoppedRun{mgr: mgr, engine: engine, worker: w, exec: exec, log: log}
}

func (r *stoppedRun) kill() {
	r.engine.out <- ai.Output{Type: ai.OutputError, Content: "signal: killed"}
	close(r.engine.out)
}

func (r *stoppedRun) storedStatus(t *testing.T) model.WorkerStatus {
	t.Helper()
	w, err := r.mgr.workerStore.GetByID(r.worker.ID)
	require.NoError(t, err)
	return w.Status
}

func (r *stoppedRun) waitForMonitor(t *testing.T) {
	t.Helper()
	require.Eventually(t, func() bool {
		r.mgr.mu.RLock()
		defer r.mgr.mu.RUnlock()
		_, live := r.mgr.activeProcesses[r.exec.ID]
		return !live
	}, 2*time.Second, 5*time.Millisecond)
}

func TestManagerSettleStopped(t *testing.T) {
	t.Run("AfterMonitorFinished", func(t *testing.T) {
		run := startStoppedRun(t)
		run.kill()
		run.waitForMonitor(t)

		run.mgr.SettleStopped(context.Background(), run.exec.ID, run.worker.ID)

		assert.Equal(t, []model.WorkerStatus{model.WorkerStatusWorking, model.WorkerStatusError, model.WorkerStatusIdle}, run.log.list())
		assert.Equal(t, model.WorkerStatusIdle, run.storedStatus(t))
	})

	t.Run("WaitsForMonitor", func(t *testing.T) {
		run := startStoppedRun(t)
		settled := make(chan struct{})
		go func() {
			run.mgr.SettleStopped(context.Background(), run.exec.ID, run.worker.ID)
			close(settled)
		}()
		assert.Never(t, func() bool {
			select {
			case <-settled:
				return true
			default:
				return false
			}
		}, 50*time.Millisecond, 5*time.Millisecond)

		run.kill()
		select {
		case <-settled:
		case <-time.After(2 * time.Second):
			require.Fail(t, "SettleStopped did not return after the run ended")
		}

		assert.Equal(t, []model.WorkerStatus{model.WorkerStatusWorking, model.WorkerStatusError, model.WorkerStatusIdle}, run.log.list())
		assert.Equal(t, model.WorkerStatusIdle, run.storedStatus(t))
	})

	t.Run("KeepsStatusWhileAnotherRunIsActive", func(t *testing.T) {
		run := startStoppedRun(t)
		run.kill()
		run.waitForMonitor(t)
		_, err := run.mgr.executionStore.Create(store.ExecutionCreate{WorkerID: run.worker.ID, TriggerInput: "next", SessionID: "s2"})
		require.NoError(t, err)

		run.mgr.SettleStopped(context.Background(), run.exec.ID, run.worker.ID)

		assert.Equal(t, []model.WorkerStatus{model.WorkerStatusWorking, model.WorkerStatusError}, run.log.list())
		assert.Equal(t, model.WorkerStatusError, run.storedStatus(t))
	})

	t.Run("GivesUpWhenContextEnds", func(t *testing.T) {
		run := startStoppedRun(t)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		run.mgr.SettleStopped(ctx, run.exec.ID, run.worker.ID)
		run.kill()
		run.waitForMonitor(t)

		assert.Equal(t, []model.WorkerStatus{model.WorkerStatusWorking, model.WorkerStatusError}, run.log.list())
	})
}

func TestChangedEngineArgs(t *testing.T) {
	changed, err := ChangedEngineArgs(`{"codex":"--model o3","pi":"--model x"}`, map[string]string{
		"codex":  "--model o3",
		"pi":     "--model y",
		"claude": "--model sonnet",
		"gone":   "",
	})
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"pi": "--model y", "claude": "--model sonnet"}, changed)

	changed, err = ChangedEngineArgs("", map[string]string{"pi": "--model x"})
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"pi": "--model x"}, changed)

	_, err = ChangedEngineArgs("{", map[string]string{"pi": "--model x"})
	require.Error(t, err)
}

func TestCreateWorker_EngineModels(t *testing.T) {
	src := aitest.NewSource(map[string]ai.EngineAdapter{"pi": &aitest.Engine{}, "opencode": &aitest.Engine{}})

	t.Run("TrimsAndDropsEmpty", func(t *testing.T) {
		mgr := newTestManagerWithSource(t, src, "pi", nil)
		w, err := mgr.CreateWorker(CreateWorkerParams{
			Name:         "coder",
			EngineModels: map[string]string{"opencode": "  deepseek/deepseek-flash  ", "pi": "   "},
		})
		require.NoError(t, err)
		assert.JSONEq(t, `{"opencode":"deepseek/deepseek-flash"}`, w.EngineModels)
		stored, err := mgr.workerStore.GetByID(w.ID)
		require.NoError(t, err)
		assert.JSONEq(t, `{"opencode":"deepseek/deepseek-flash"}`, stored.EngineModels)
	})

	t.Run("NilStoresEmptyObject", func(t *testing.T) {
		mgr := newTestManagerWithSource(t, src, "pi", nil)
		w, err := mgr.CreateWorker(CreateWorkerParams{Name: "plain"})
		require.NoError(t, err)
		assert.Equal(t, "{}", w.EngineModels)
	})

	cases := []struct {
		name   string
		models map[string]string
		want   string
	}{
		{name: "UnknownEngine", models: map[string]string{"codex": "o3"}, want: `engine "codex" is not installed or enabled`},
		{name: "EmptyEngineName", models: map[string]string{"": "o3"}, want: "empty engine name"},
		{name: "InnerWhitespace", models: map[string]string{"pi": "openai/gpt 5"}, want: "whitespace or control characters"},
		{name: "ControlCharacter", models: map[string]string{"pi": "openai/gpt\x005"}, want: "whitespace or control characters"},
		{name: "LeadingDash", models: map[string]string{"pi": "--yolo"}, want: `must not start with "-"`},
		{name: "TooLong", models: map[string]string{"pi": strings.Repeat("m", 201)}, want: "longer than 200 characters"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mgr := newTestManagerWithSource(t, src, "pi", nil)
			workDir := filepath.Join(t.TempDir(), "never-created")
			_, err := mgr.CreateWorker(CreateWorkerParams{Name: "bad", WorkDir: workDir, EngineModels: tc.models})
			require.ErrorIs(t, err, ErrValidation)
			assert.Contains(t, err.Error(), tc.want)
			_, statErr := os.Stat(workDir)
			assert.True(t, os.IsNotExist(statErr))
			exists, err := mgr.workerStore.ExistsByName("bad", "")
			require.NoError(t, err)
			assert.False(t, exists)
		})
	}

	t.Run("MaxLengthAccepted", func(t *testing.T) {
		mgr := newTestManagerWithSource(t, src, "pi", nil)
		long := strings.Repeat("模", 200)
		w, err := mgr.CreateWorker(CreateWorkerParams{Name: "long", EngineModels: map[string]string{"pi": long}})
		require.NoError(t, err)
		assert.Equal(t, long, mgr.resolveModel(context.Background(), w, "pi"))
	})
}

func TestUpdateWorker_EngineModels(t *testing.T) {
	newWorker := func(t *testing.T) (*Manager, *aitest.Source, model.Worker) {
		t.Helper()
		src := aitest.NewSource(map[string]ai.EngineAdapter{"pi": &aitest.Engine{}, "opencode": &aitest.Engine{}, "codex": &aitest.Engine{}})
		mgr := newTestManagerWithSource(t, src, "pi", nil)
		w, err := mgr.CreateWorker(CreateWorkerParams{
			Name:         "coder",
			EngineModels: map[string]string{"opencode": "deepseek/deepseek-flash", "codex": "o3"},
		})
		require.NoError(t, err)
		return mgr, src, w
	}

	t.Run("NilLeavesModels", func(t *testing.T) {
		mgr, _, w := newWorker(t)
		updated, err := mgr.UpdateWorker(w.ID, UpdateWorkerParams{Description: strPtr("edited")})
		require.NoError(t, err)
		assert.JSONEq(t, `{"opencode":"deepseek/deepseek-flash","codex":"o3"}`, updated.EngineModels)
	})

	t.Run("SetTrimmedAndRemove", func(t *testing.T) {
		mgr, _, w := newWorker(t)
		updated, err := mgr.UpdateWorker(w.ID, UpdateWorkerParams{
			EngineModels: map[string]string{"pi": " openai/gpt-5 ", "codex": ""},
		})
		require.NoError(t, err)
		assert.JSONEq(t, `{"opencode":"deepseek/deepseek-flash","pi":"openai/gpt-5"}`, updated.EngineModels)
		stored, err := mgr.workerStore.GetByID(w.ID)
		require.NoError(t, err)
		assert.Equal(t, updated.EngineModels, stored.EngineModels)
	})

	t.Run("WhitespaceOnlyRemoves", func(t *testing.T) {
		mgr, _, w := newWorker(t)
		updated, err := mgr.UpdateWorker(w.ID, UpdateWorkerParams{EngineModels: map[string]string{"codex": "  "}})
		require.NoError(t, err)
		assert.JSONEq(t, `{"opencode":"deepseek/deepseek-flash"}`, updated.EngineModels)
	})

	t.Run("EmptyMapClearsAll", func(t *testing.T) {
		mgr, _, w := newWorker(t)
		updated, err := mgr.UpdateWorker(w.ID, UpdateWorkerParams{EngineModels: map[string]string{}})
		require.NoError(t, err)
		assert.Equal(t, "{}", updated.EngineModels)
	})

	t.Run("UnchangedModelForUninstalledEngineSucceeds", func(t *testing.T) {
		mgr, src, w := newWorker(t)
		src.Remove("codex")
		updated, err := mgr.UpdateWorker(w.ID, UpdateWorkerParams{
			EngineModels: map[string]string{"codex": "o3", "opencode": "deepseek/deepseek-pro"},
		})
		require.NoError(t, err)
		assert.JSONEq(t, `{"opencode":"deepseek/deepseek-pro","codex":"o3"}`, updated.EngineModels)
	})

	t.Run("ChangedModelForUninstalledEngineFails", func(t *testing.T) {
		mgr, src, w := newWorker(t)
		src.Remove("codex")
		_, err := mgr.UpdateWorker(w.ID, UpdateWorkerParams{EngineModels: map[string]string{"codex": "o4"}})
		require.ErrorIs(t, err, ErrValidation)
		assert.True(t, ai.IsEngineMissing(err))
	})

	t.Run("InvalidModelFails", func(t *testing.T) {
		mgr, _, w := newWorker(t)
		for _, bad := range []string{"-m", "a b", "a\tb", strings.Repeat("x", 201)} {
			_, err := mgr.UpdateWorker(w.ID, UpdateWorkerParams{EngineModels: map[string]string{"pi": bad}})
			require.ErrorIs(t, err, ErrValidation, bad)
		}
		_, err := mgr.UpdateWorker(w.ID, UpdateWorkerParams{EngineModels: map[string]string{"": "x"}})
		require.ErrorIs(t, err, ErrValidation)
		stored, err := mgr.workerStore.GetByID(w.ID)
		require.NoError(t, err)
		assert.JSONEq(t, `{"opencode":"deepseek/deepseek-flash","codex":"o3"}`, stored.EngineModels)
	})
}

func TestExecuteWorker_PassesModelForEffectiveEngine(t *testing.T) {
	pi := &aitest.Engine{RunFunc: (&mockEngine{}).Run}
	opencode := &aitest.Engine{RunFunc: (&mockEngine{}).Run}
	src := aitest.NewSource(map[string]ai.EngineAdapter{"pi": pi, "opencode": opencode})
	mgr := newTestManagerWithSource(t, src, "pi", nil)
	models := map[string]string{"pi": "openai/gpt-5", "opencode": "deepseek/deepseek-flash"}

	followsDefault, err := mgr.CreateWorker(CreateWorkerParams{Name: "default", EngineModels: models})
	require.NoError(t, err)
	pinned, err := mgr.CreateWorker(CreateWorkerParams{Name: "pinned", Engine: "opencode", EngineModels: models})
	require.NoError(t, err)
	bare, err := mgr.CreateWorker(CreateWorkerParams{Name: "bare", Engine: "opencode", EngineModels: map[string]string{"pi": "openai/gpt-5"}})
	require.NoError(t, err)

	for i, w := range []model.Worker{followsDefault, pinned, bare} {
		_, err := mgr.ExecuteWorker(context.Background(), ExecuteRequest{WorkerID: w.ID, TriggerInput: "hi", SessionID: "s" + strconv.Itoa(i)})
		require.NoError(t, err)
	}

	require.Len(t, pi.Runs(), 1)
	assert.Equal(t, "openai/gpt-5", pi.Runs()[0].Model)
	require.Len(t, opencode.Runs(), 2)
	assert.Equal(t, "deepseek/deepseek-flash", opencode.Runs()[0].Model)
	assert.Equal(t, "", opencode.Runs()[1].Model)
}

type mapSystemConfigs map[string]string

func (m mapSystemConfigs) Get(_ context.Context, key string) (model.SystemConfig, bool, error) {
	value, ok := m[key]
	return model.SystemConfig{Key: key, Value: value}, ok, nil
}

func TestExecuteWorker_FallsBackToGlobalModel(t *testing.T) {
	pi := &aitest.Engine{RunFunc: (&mockEngine{}).Run}
	opencode := &aitest.Engine{RunFunc: (&mockEngine{}).Run}
	src := aitest.NewSource(map[string]ai.EngineAdapter{"pi": pi, "opencode": opencode})
	mgr := newTestManagerWithSource(t, src, "pi", nil)
	mgr.sysConfigStore = mapSystemConfigs{model.SystemConfigKeyEngineModelsGlobal: `{"pi":"deepseek/deepseek-flash"}`}

	own, err := mgr.CreateWorker(CreateWorkerParams{Name: "own", EngineModels: map[string]string{"pi": "openai/gpt-5"}})
	require.NoError(t, err)
	inherits, err := mgr.CreateWorker(CreateWorkerParams{Name: "inherits"})
	require.NoError(t, err)
	other, err := mgr.CreateWorker(CreateWorkerParams{Name: "other", Engine: "opencode"})
	require.NoError(t, err)

	for i, w := range []model.Worker{own, inherits, other} {
		_, err := mgr.ExecuteWorker(context.Background(), ExecuteRequest{WorkerID: w.ID, TriggerInput: "hi", SessionID: "s" + strconv.Itoa(i)})
		require.NoError(t, err)
	}

	require.Len(t, pi.Runs(), 2)
	assert.Equal(t, "openai/gpt-5", pi.Runs()[0].Model)
	assert.Equal(t, "deepseek/deepseek-flash", pi.Runs()[1].Model)
	require.Len(t, opencode.Runs(), 1)
	assert.Equal(t, "", opencode.Runs()[0].Model)
}

func TestManager_EngineModels(t *testing.T) {
	newModelsManager := func(t *testing.T) (*Manager, *aitest.ModelEngine, *aitest.Source) {
		t.Helper()
		lister := &aitest.ModelEngine{ModelsFunc: func(context.Context, []string) ([]ai.ModelInfo, error) {
			return []ai.ModelInfo{{ID: "deepseek/deepseek-flash", Name: "DeepSeek Flash"}}, nil
		}}
		src := aitest.NewSource(map[string]ai.EngineAdapter{"opencode": lister, "pi": &aitest.Engine{}})
		mgr := newTestManagerWithSource(t, src, "pi", nil)
		_, err := mgr.envService.Create(env.ScopeGlobal, "", "FAKE_GLOBAL", "g")
		require.NoError(t, err)
		_, err = mgr.envService.Create(env.ScopeGlobal, "", "FAKE_SHARED", "global")
		require.NoError(t, err)
		return mgr, lister, src
	}

	t.Run("GlobalEnv", func(t *testing.T) {
		mgr, lister, _ := newModelsManager(t)
		got, err := mgr.EngineModels(context.Background(), "opencode", "")
		require.NoError(t, err)
		assert.Equal(t, []ai.ModelInfo{{ID: "deepseek/deepseek-flash", Name: "DeepSeek Flash"}}, got)
		assert.Equal(t, [][]string{{"FAKE_GLOBAL=g", "FAKE_SHARED=global"}}, lister.ModelCalls())
	})

	t.Run("WorkerEnv", func(t *testing.T) {
		mgr, lister, _ := newModelsManager(t)
		w, err := mgr.CreateWorker(CreateWorkerParams{Name: "coder"})
		require.NoError(t, err)
		_, err = mgr.envService.Create(env.ScopeWorker, w.ID, "FAKE_SHARED", "worker")
		require.NoError(t, err)

		_, err = mgr.EngineModels(context.Background(), "opencode", w.ID)
		require.NoError(t, err)
		assert.Equal(t, [][]string{{"FAKE_GLOBAL=g", "FAKE_SHARED=worker"}}, lister.ModelCalls())
	})

	t.Run("UnknownEngine", func(t *testing.T) {
		mgr, lister, src := newModelsManager(t)
		src.Remove("opencode")
		_, err := mgr.EngineModels(context.Background(), "opencode", "")
		require.EqualError(t, err, `engine "opencode" is not installed or enabled`)
		assert.True(t, ai.IsEngineMissing(err))
		assert.Empty(t, lister.ModelCalls())
	})

	t.Run("UnknownWorker", func(t *testing.T) {
		mgr, lister, _ := newModelsManager(t)
		_, err := mgr.EngineModels(context.Background(), "opencode", "missing")
		require.ErrorIs(t, err, ErrNotFound)
		assert.Empty(t, lister.ModelCalls())
	})

	t.Run("EngineWithoutList", func(t *testing.T) {
		mgr, _, _ := newModelsManager(t)
		got, err := mgr.EngineModels(context.Background(), "pi", "")
		require.NoError(t, err)
		assert.NotNil(t, got)
		assert.Empty(t, got)
	})

	t.Run("NilListBecomesEmpty", func(t *testing.T) {
		mgr, lister, _ := newModelsManager(t)
		lister.ModelsFunc = nil
		got, err := mgr.EngineModels(context.Background(), "opencode", "")
		require.NoError(t, err)
		assert.NotNil(t, got)
		assert.Empty(t, got)
	})

	t.Run("PluginError", func(t *testing.T) {
		mgr, lister, _ := newModelsManager(t)
		lister.ModelsFunc = func(context.Context, []string) ([]ai.ModelInfo, error) {
			return nil, errors.New(`unknown method "models"`)
		}
		_, err := mgr.EngineModels(context.Background(), "opencode", "")
		var modelsErr *EngineModelsError
		require.ErrorAs(t, err, &modelsErr)
		assert.EqualError(t, err, `unknown method "models"`)
	})
}
