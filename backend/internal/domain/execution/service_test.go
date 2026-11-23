package execution

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ai "github.com/theopenbee/openbee/internal/ai"
	"github.com/theopenbee/openbee/internal/ai/aitest"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/platform/local"
)

type fakeExecutions struct {
	byID   map[string]model.WorkerExecution
	byTask map[string][]model.WorkerExecution
	limits []int
}

func (f *fakeExecutions) GetByID(id string) (model.WorkerExecution, error) {
	e, ok := f.byID[id]
	if !ok {
		return model.WorkerExecution{}, fmt.Errorf("get execution: %w", sql.ErrNoRows)
	}
	return e, nil
}

func (f *fakeExecutions) ListByTaskIDs(_ context.Context, taskIDs []string, limit int) (map[string][]model.WorkerExecution, error) {
	f.limits = append(f.limits, limit)
	out := map[string][]model.WorkerExecution{}
	for _, id := range taskIDs {
		list := f.byTask[id]
		if limit > 0 && len(list) > limit {
			list = list[:limit]
		}
		if len(list) > 0 {
			out[id] = list
		}
	}
	return out, nil
}

type fakeTasks map[string]model.Task

func (f fakeTasks) GetByID(_ context.Context, id string) (model.Task, error) {
	t, ok := f[id]
	if !ok {
		return model.Task{}, fmt.Errorf("scan task: %w", sql.ErrNoRows)
	}
	return t, nil
}

type fixture struct {
	execs   *fakeExecutions
	tasks   fakeTasks
	source  *aitest.Source
	engine  *aitest.TranscriptEngine
	service *Service
	logPath string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	engine := &aitest.TranscriptEngine{
		TranscriptFunc: func(context.Context, string) ([]ai.TranscriptStep, error) {
			return []ai.TranscriptStep{{Type: "text", Text: "hello"}}, nil
		},
	}
	source := aitest.NewSource(map[string]ai.EngineAdapter{"claude": engine})
	execs := &fakeExecutions{byID: map[string]model.WorkerExecution{}, byTask: map[string][]model.WorkerExecution{}}
	tasks := fakeTasks{}
	logPath := filepath.Join(t.TempDir(), "e1.log")
	require.NoError(t, os.WriteFile(logPath, []byte("line one\n"), 0o644))
	f := &fixture{execs: execs, tasks: tasks, source: source, engine: engine, logPath: logPath}
	f.service = New(execs, tasks, source)
	f.addTask("t1", local.SessionKey("u1"))
	f.addExecution(model.WorkerExecution{ID: "e1", TaskID: "t1", Engine: "claude", Status: model.ExecStatusCompleted, LogPath: logPath})
	return f
}

func (f *fixture) addTask(id, sessionKey string) {
	f.tasks[id] = model.Task{ID: id, SessionKey: sessionKey, Instruction: "do it"}
}

func (f *fixture) addExecution(e model.WorkerExecution) {
	if e.WorkerID == nil {
		e.WorkerID = ptr("w1")
	}
	f.execs.byID[e.ID] = e
	f.execs.byTask[e.TaskID] = append([]model.WorkerExecution{e}, f.execs.byTask[e.TaskID]...)
}

func (f *fixture) detail(t *testing.T, id string) Detail {
	t.Helper()
	d, err := f.service.Detail(context.Background(), "u1", id)
	require.NoError(t, err)
	return d
}

func ptr[T any](v T) *T { return &v }

func TestDetailNotFound(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	_, err := f.service.Detail(ctx, "u1", "missing")
	require.ErrorIs(t, err, ErrNotFound)

	f.execs.byID["bee1"] = model.WorkerExecution{ID: "bee1", Engine: "claude", Status: model.ExecStatusCompleted}
	_, err = f.service.Detail(ctx, "u1", "bee1")
	require.ErrorIs(t, err, ErrNotFound)

	f.execs.byID["noworker"] = model.WorkerExecution{ID: "noworker", TaskID: "t1", Engine: "claude"}
	_, err = f.service.Detail(ctx, "u1", "noworker")
	require.ErrorIs(t, err, ErrNotFound)

	f.addExecution(model.WorkerExecution{ID: "orphan", TaskID: "gone", Engine: "claude"})
	_, err = f.service.Detail(ctx, "u1", "orphan")
	require.ErrorIs(t, err, ErrNotFound)
}

func TestDetailTaskOwnership(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	_, err := f.service.Detail(ctx, "u2", "e1")
	require.ErrorIs(t, err, ErrNotFound)

	f.addTask("t2", local.WorkerSessionKey("u1", "w1"))
	f.addExecution(model.WorkerExecution{ID: "e2", TaskID: "t2", Engine: "claude"})
	_, err = f.service.Detail(ctx, "u2", "e2")
	require.ErrorIs(t, err, ErrNotFound)
	d, err := f.service.Detail(ctx, "u1", "e2")
	require.NoError(t, err)
	assert.Equal(t, "e2", d.ID)

	f.addTask("t3", "telegram:42")
	f.addExecution(model.WorkerExecution{ID: "e3", TaskID: "t3", Engine: "claude"})
	d, err = f.service.Detail(ctx, "u2", "e3")
	require.NoError(t, err)
	assert.Equal(t, "e3", d.ID)
}

func TestDetailFields(t *testing.T) {
	f := newFixture(t)
	f.addExecution(model.WorkerExecution{
		ID: "e9", TaskID: "t1", WorkerID: ptr("w9"), Engine: "claude",
		Status: model.ExecStatusCompleted, LogPath: f.logPath,
		StartedAt: ptr(int64(1000)), CompletedAt: ptr(int64(2000)),
	})
	d := f.detail(t, "e9")
	assert.Equal(t, "e9", d.ID)
	assert.Equal(t, "t1", d.TaskID)
	assert.Equal(t, "w9", d.WorkerID)
	assert.Equal(t, "claude", d.Engine)
	assert.Equal(t, "completed", d.Status)
	assert.Equal(t, "do it", d.Instruction)
	assert.Equal(t, int64(1000), d.StartedAt)
	require.NotNil(t, d.CompletedAt)
	assert.Equal(t, int64(2000), *d.CompletedAt)
	assert.Empty(t, d.Error)
}

func TestDetailStatusAndError(t *testing.T) {
	f := newFixture(t)
	f.addExecution(model.WorkerExecution{ID: "pend", TaskID: "t1", Engine: "claude", Status: model.ExecStatusPending})
	f.addExecution(model.WorkerExecution{ID: "run", TaskID: "t1", Engine: "claude", Status: model.ExecStatusRunning, Result: "partial"})
	f.addExecution(model.WorkerExecution{ID: "fail", TaskID: "t1", Engine: "claude", Status: model.ExecStatusFailed, Result: "boom"})

	pending := f.detail(t, "pend")
	assert.Equal(t, "running", pending.Status)
	assert.Nil(t, pending.CompletedAt)
	assert.Equal(t, int64(0), pending.StartedAt)

	running := f.detail(t, "run")
	assert.Equal(t, "running", running.Status)
	assert.Empty(t, running.Error)

	failed := f.detail(t, "fail")
	assert.Equal(t, "failed", failed.Status)
	assert.Equal(t, "boom", failed.Error)
	assert.Empty(t, f.detail(t, "e1").Error)
}

func TestDetailRuns(t *testing.T) {
	f := newFixture(t)
	assert.Nil(t, f.detail(t, "e1").Runs)

	f.addTask("t2", local.SessionKey("u1"))
	for i := 1; i <= 25; i++ {
		status := model.ExecStatusCompleted
		if i == 25 {
			status = model.ExecStatusPending
		}
		f.addExecution(model.WorkerExecution{
			ID: fmt.Sprintf("r%02d", i), TaskID: "t2", Engine: "claude", Status: status,
			StartedAt: ptr(int64(i * 100)), CompletedAt: ptr(int64(i*100 + 50)),
		})
	}
	d := f.detail(t, "r01")
	require.Len(t, d.Runs, 20)
	assert.Equal(t, "r25", d.Runs[0].ID)
	assert.Equal(t, "running", d.Runs[0].Status)
	assert.Equal(t, int64(2500), d.Runs[0].StartedAt)
	require.NotNil(t, d.Runs[0].CompletedAt)
	assert.Equal(t, int64(2550), *d.Runs[0].CompletedAt)
	assert.Equal(t, "r06", d.Runs[19].ID)
	assert.Equal(t, "completed", d.Runs[19].Status)
}

func TestDetailRunsTwoExecutions(t *testing.T) {
	f := newFixture(t)
	f.addExecution(model.WorkerExecution{ID: "e2", TaskID: "t1", Engine: "claude", Status: model.ExecStatusRunning})
	d := f.detail(t, "e1")
	require.Len(t, d.Runs, 2)
	assert.Equal(t, "e2", d.Runs[0].ID)
	assert.Equal(t, "e1", d.Runs[1].ID)
}

func TestDetailSteps(t *testing.T) {
	f := newFixture(t)
	d := f.detail(t, "e1")
	assert.Equal(t, []ai.TranscriptStep{{Type: "text", Text: "hello"}}, d.Steps)
	assert.Empty(t, d.StepsError)
	assert.Equal(t, []string{f.logPath}, f.engine.TranscriptCalls())
}

func TestDetailStepsUseRequestContext(t *testing.T) {
	f := newFixture(t)
	type ctxKey struct{}
	var got any
	f.engine.TranscriptFunc = func(ctx context.Context, _ string) ([]ai.TranscriptStep, error) {
		got = ctx.Value(ctxKey{})
		return nil, nil
	}
	_, err := f.service.Detail(context.WithValue(context.Background(), ctxKey{}, "req"), "u1", "e1")
	require.NoError(t, err)
	assert.Equal(t, "req", got)
}

func TestDetailStepsNilBecomesEmpty(t *testing.T) {
	f := newFixture(t)
	f.engine.TranscriptFunc = func(context.Context, string) ([]ai.TranscriptStep, error) { return nil, nil }
	d := f.detail(t, "e1")
	require.NotNil(t, d.Steps)
	raw, err := json.Marshal(d)
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"steps":[]`)
}

func TestDetailStepsError(t *testing.T) {
	f := newFixture(t)
	f.engine.TranscriptFunc = func(context.Context, string) ([]ai.TranscriptStep, error) {
		return nil, fmt.Errorf("wrapped: %w", ai.ErrTranscriptUnsupported)
	}
	d := f.detail(t, "e1")
	assert.Equal(t, "transcript_unsupported", d.StepsError)
	assert.Equal(t, []ai.TranscriptStep{}, d.Steps)

	f.engine.TranscriptFunc = func(context.Context, string) ([]ai.TranscriptStep, error) {
		return nil, errors.New("plugin crashed")
	}
	d = f.detail(t, "e1")
	assert.Equal(t, "transcript_failed", d.StepsError)
	assert.Equal(t, []ai.TranscriptStep{}, d.Steps)
}

func TestDetailStepsEngineNotReader(t *testing.T) {
	f := newFixture(t)
	f.source.Add("plain", map[string]string{"en": "plain"}, &aitest.Engine{})
	f.addExecution(model.WorkerExecution{ID: "e2", TaskID: "t1", Engine: "plain", LogPath: f.logPath})
	d := f.detail(t, "e2")
	assert.Equal(t, "transcript_unsupported", d.StepsError)
	assert.Equal(t, []ai.TranscriptStep{}, d.Steps)
}

func TestDetailStepsEngineUnavailable(t *testing.T) {
	f := newFixture(t)
	f.addExecution(model.WorkerExecution{ID: "e2", TaskID: "t1", Engine: "gone", LogPath: f.logPath})
	d := f.detail(t, "e2")
	assert.Equal(t, "engine_unavailable", d.StepsError)
	assert.Equal(t, []ai.TranscriptStep{}, d.Steps)
}

func TestDetailStepsMissingLog(t *testing.T) {
	f := newFixture(t)
	f.addExecution(model.WorkerExecution{ID: "nolog", TaskID: "t1", Engine: "claude"})
	f.addExecution(model.WorkerExecution{ID: "nofile", TaskID: "t1", Engine: "claude", LogPath: filepath.Join(t.TempDir(), "absent.log")})
	f.addExecution(model.WorkerExecution{ID: "noengine", TaskID: "t1", Engine: "gone"})
	for _, id := range []string{"nolog", "nofile", "noengine"} {
		d := f.detail(t, id)
		assert.Equal(t, []ai.TranscriptStep{}, d.Steps, id)
		assert.Empty(t, d.StepsError, id)
	}
	assert.Empty(t, f.engine.TranscriptCalls())
}

func TestDetailStepsCache(t *testing.T) {
	f := newFixture(t)
	f.detail(t, "e1")
	f.detail(t, "e1")
	assert.Len(t, f.engine.TranscriptCalls(), 1)

	file, err := os.OpenFile(f.logPath, os.O_APPEND|os.O_WRONLY, 0o644)
	require.NoError(t, err)
	_, err = file.WriteString("line two\n")
	require.NoError(t, err)
	require.NoError(t, file.Close())

	f.detail(t, "e1")
	assert.Len(t, f.engine.TranscriptCalls(), 2)
	f.detail(t, "e1")
	assert.Len(t, f.engine.TranscriptCalls(), 2)
}

func TestDetailStepsCacheSkipsErrors(t *testing.T) {
	f := newFixture(t)
	f.engine.TranscriptFunc = func(context.Context, string) ([]ai.TranscriptStep, error) {
		return nil, errors.New("flaky")
	}
	f.detail(t, "e1")
	f.detail(t, "e1")
	assert.Len(t, f.engine.TranscriptCalls(), 2)
}

func TestDetailStepsCacheEvictsOldest(t *testing.T) {
	f := newFixture(t)
	dir := t.TempDir()
	var paths []string
	for i := 0; i < maxCacheEntries+1; i++ {
		path := filepath.Join(dir, fmt.Sprintf("x%02d.log", i))
		require.NoError(t, os.WriteFile(path, []byte("x"), 0o644))
		paths = append(paths, path)
		f.addExecution(model.WorkerExecution{ID: fmt.Sprintf("x%02d", i), TaskID: "t1", Engine: "claude", LogPath: path})
	}
	for i := range paths {
		f.detail(t, fmt.Sprintf("x%02d", i))
	}
	require.Len(t, f.engine.TranscriptCalls(), maxCacheEntries+1)

	f.detail(t, fmt.Sprintf("x%02d", maxCacheEntries))
	assert.Len(t, f.engine.TranscriptCalls(), maxCacheEntries+1)

	f.detail(t, "x00")
	calls := f.engine.TranscriptCalls()
	require.Len(t, calls, maxCacheEntries+2)
	assert.Equal(t, paths[0], calls[len(calls)-1])
}
