package bee_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ai "github.com/theopenbee/openbee/internal/ai"
	"github.com/theopenbee/openbee/internal/ai/aitest"
	"github.com/theopenbee/openbee/internal/domain/bee"
	"github.com/theopenbee/openbee/internal/domain/delivery"
	"github.com/theopenbee/openbee/internal/domain/enginecfg"
	"github.com/theopenbee/openbee/internal/domain/localchat"
	"github.com/theopenbee/openbee/internal/infra/config"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/infra/store"
	"github.com/theopenbee/openbee/internal/platform"
	"github.com/theopenbee/openbee/internal/platform/local"
)

func setupFeederDB(t *testing.T) (*sql.DB, *store.MessageStore, *store.TaskStore, *store.SessionStore, *store.ExecutionStore) {
	t.Helper()
	db, err := store.InitDB(t.TempDir() + "/test.db")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	return db, store.NewMessageStore(db), store.NewTaskStore(db), store.NewSessionStore(db), store.NewExecutionStore(db, t.TempDir())
}

func insertMessage(t *testing.T, db *sql.DB, id, sessionKey, content string) {
	t.Helper()
	now := time.Now().UnixMilli()
	_, err := db.Exec(
		`INSERT INTO bee_platform_messages (id, session_key, platform, content, status, received_at, created_at, updated_at)
		 VALUES (?, ?, 'beta', ?, 'received', ?, ?, ?)`,
		id, sessionKey, content, now, now, now,
	)
	require.NoError(t, err)
}

type mockProcess struct{}

func (m *mockProcess) PID() int    { return 0 }
func (m *mockProcess) Stop() error { return nil }

// mockBeeRunner records all Run calls.
type mockBeeRunner struct {
	mu          sync.Mutex
	calls       []beeCall
	err         error
	outputLines []ai.Output
}

type beeCall struct {
	prompt  string
	opts    ai.RunOptions
	logPath string
}

func (m *mockBeeRunner) Run(_ context.Context, _, prompt string, opts ai.RunOptions, logPath string) (ai.RunResult, error) {
	m.mu.Lock()
	m.calls = append(m.calls, beeCall{prompt: prompt, opts: opts, logPath: logPath})
	m.mu.Unlock()
	if m.err != nil {
		return ai.RunResult{}, m.err
	}
	var lines []ai.Output
	if len(m.outputLines) > 0 {
		lines = m.outputLines
	} else {
		lines = []ai.Output{{Type: ai.OutputDone}}
	}
	ch := make(chan ai.Output, len(lines))
	for _, l := range lines {
		ch <- l
	}
	close(ch)
	return ai.RunResult{Process: &mockProcess{}, Output: ch, ExtractResult: func(string) string { return "" }}, nil
}

func (m *mockBeeRunner) CollectTokenUsage(_ context.Context, _ []string) (map[string][]ai.TokenUsage, error) {
	return nil, nil
}

func (m *mockBeeRunner) getCalls() []beeCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]beeCall{}, m.calls...)
}

func newFeeder(ms *store.MessageStore, ts *store.TaskStore, ss *store.SessionStore, es *store.ExecutionStore, runner ai.EngineAdapter) *bee.Feeder {
	return newFeederWithEngine(ms, ts, ss, es, runner, "")
}

func newFeederWithEngine(ms *store.MessageStore, ts *store.TaskStore, ss *store.SessionStore, es *store.ExecutionStore, runner ai.EngineAdapter, engine string) *bee.Feeder {
	cfg := config.BeeConfig{}
	cfg.Engine.Timeout.Bee = 5 * time.Second
	cfg.Feeder.MaxConcurrentBee = 5
	return bee.NewFeeder(ms, ts, ss, es, runner, "/tmp", cfg, enginecfg.NewStore(engine, "", aitest.IDs(engine)))
}

func TestFeeder_FirstTick_UsesNewSessionID(t *testing.T) {
	db, ms, ts, ss, es := setupFeederDB(t)
	insertMessage(t, db, "m1", "beta:c:u", "hello")

	runner := &mockBeeRunner{}
	f := newFeeder(ms, ts, ss, es, runner)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	go f.Run(ctx)
	time.Sleep(700 * time.Millisecond)

	calls := runner.getCalls()
	require.NotEmpty(t, calls, "expected bee runner to be called")
	call := calls[0]
	assert.NotEmpty(t, call.opts.SessionID, "expected non-empty sessionID on first call")
	assert.False(t, call.opts.Resume, "expected resume=false on first call")
	assert.Equal(t, ai.RoleBee, call.opts.Role)

	// logPath should already be prepared (PrepareLogPath) before the process runs.
	assert.NotEmpty(t, call.logPath, "logPath passed to runner must be non-empty")
	_, statErr := os.Stat(filepath.Dir(call.logPath))
	assert.NoError(t, statErr, "log directory should exist before process runs")

	got, _, err := ss.GetSessionContext(context.Background(), "beta:c:u", store.BeeAgentID)
	require.NoError(t, err)
	assert.Equal(t, call.opts.SessionID, got)

	var status string
	db.QueryRow(`SELECT status FROM bee_platform_messages WHERE id='m1'`).Scan(&status)
	assert.Equal(t, "bee_processed", status)
}

func TestFeeder_SecondTick_ResumesSession(t *testing.T) {
	db, ms, ts, ss, es := setupFeederDB(t)
	ctx := context.Background()

	err := ss.UpsertSessionContext(ctx, "beta:c:u", store.BeeAgentID, "existing-session", "claude")
	require.NoError(t, err)

	insertMessage(t, db, "m1", "beta:c:u", "follow-up")

	runner := &mockBeeRunner{}
	f := newFeeder(ms, ts, ss, es, runner)

	tickCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	go f.Run(tickCtx)
	time.Sleep(700 * time.Millisecond)

	calls := runner.getCalls()
	require.NotEmpty(t, calls, "expected bee runner to be called")
	call := calls[0]
	assert.Equal(t, "existing-session", call.opts.SessionID)
	assert.True(t, call.opts.Resume, "expected resume=true on second call")
}

func TestFeeder_EngineSwitch_PreservesPriorSession(t *testing.T) {
	db, ms, ts, ss, es := setupFeederDB(t)
	ctx := context.Background()

	err := ss.UpsertSessionContext(ctx, "beta:c:u", store.BeeAgentID, "claude-session", "claude")
	require.NoError(t, err)

	insertMessage(t, db, "m1", "beta:c:u", "switch to codex")

	codexRunner := &mockBeeRunner{}
	codexFeeder := newFeederWithEngine(ms, ts, ss, es, codexRunner, "codex")

	tickCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	go codexFeeder.Run(tickCtx)
	time.Sleep(700 * time.Millisecond)

	codexCalls := codexRunner.getCalls()
	require.NotEmpty(t, codexCalls, "expected codex bee runner to be called")
	codexCall := codexCalls[0]
	assert.False(t, codexCall.opts.Resume, "expected codex run to start fresh on engine switch")
	assert.NotEqual(t, "claude-session", codexCall.opts.SessionID, "expected codex run to use a new session ID")

	claudeSID, err := ss.GetSessionContextForEngine(ctx, "beta:c:u", store.BeeAgentID, "claude")
	require.NoError(t, err)
	codexSID, err := ss.GetSessionContextForEngine(ctx, "beta:c:u", store.BeeAgentID, "codex")
	require.NoError(t, err)
	assert.Equal(t, "claude-session", claudeSID)
	assert.Equal(t, codexCall.opts.SessionID, codexSID)

	cancel()
	time.Sleep(100 * time.Millisecond)

	insertMessage(t, db, "m2", "beta:c:u", "switch back to claude")

	claudeRunner := &mockBeeRunner{}
	claudeFeeder := newFeederWithEngine(ms, ts, ss, es, claudeRunner, "claude")

	tickCtx2, cancel2 := context.WithTimeout(ctx, 2*time.Second)
	defer cancel2()
	go claudeFeeder.Run(tickCtx2)
	time.Sleep(700 * time.Millisecond)

	claudeCalls := claudeRunner.getCalls()
	require.NotEmpty(t, claudeCalls, "expected claude bee runner to be called")
	assert.True(t, claudeCalls[0].opts.Resume, "expected claude run to resume original claude session")
	assert.Equal(t, "claude-session", claudeCalls[0].opts.SessionID)
}

func TestFeeder_OnBeeFailure_MarksFailedAndDoesNotUpdateSession(t *testing.T) {
	db, ms, ts, ss, es := setupFeederDB(t)
	insertMessage(t, db, "m1", "beta:c:u", "hello")

	runner := &mockBeeRunner{err: fmt.Errorf("bee crashed")}
	f := newFeeder(ms, ts, ss, es, runner)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	go f.Run(ctx)
	time.Sleep(700 * time.Millisecond)

	var status string
	db.QueryRow(`SELECT status FROM bee_platform_messages WHERE id='m1'`).Scan(&status)
	assert.Equal(t, "failed", status)

	got, _, _ := ss.GetSessionContext(context.Background(), "beta:c:u", store.BeeAgentID)
	assert.Empty(t, got, "session context should not be written on failure")
}

func TestFeeder_MultipleSessionKeys_ProcessedIndependently(t *testing.T) {
	db, ms, ts, ss, es := setupFeederDB(t)
	insertMessage(t, db, "m1", "beta:c:u1", "message from user1")
	insertMessage(t, db, "m2", "beta:c:u2", "message from user2")

	runner := &mockBeeRunner{}
	f := newFeeder(ms, ts, ss, es, runner)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	go f.Run(ctx)
	time.Sleep(1200 * time.Millisecond)

	calls := runner.getCalls()
	require.Len(t, calls, 2, "expected 2 bee invocations (one per sessionKey)")

	sess1, _, _ := ss.GetSessionContext(context.Background(), "beta:c:u1", store.BeeAgentID)
	sess2, _, _ := ss.GetSessionContext(context.Background(), "beta:c:u2", store.BeeAgentID)
	assert.NotEmpty(t, sess1, "session context for u1 should be set")
	assert.NotEmpty(t, sess2, "session context for u2 should be set")
	assert.NotEqual(t, sess1, sess2, "session IDs for different sessionKeys must differ")
}

func TestFeeder_CreatesExecutionOnBeeRun(t *testing.T) {
	db, ms, ts, ss, es := setupFeederDB(t)
	insertMessage(t, db, "m1", "beta:c:u", "hello bee")

	runner := &mockBeeRunner{}
	f := newFeeder(ms, ts, ss, es, runner)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	go f.Run(ctx)
	time.Sleep(700 * time.Millisecond)

	rows, err := db.Query(`SELECT id, worker_id, status, log_path FROM bee_executions`)
	require.NoError(t, err)
	defer rows.Close()

	var execs []struct {
		id       string
		workerID *string
		status   string
		logPath  string
	}
	for rows.Next() {
		var e struct {
			id       string
			workerID *string
			status   string
			logPath  string
		}
		require.NoError(t, rows.Scan(&e.id, &e.workerID, &e.status, &e.logPath))
		execs = append(execs, e)
	}

	require.Len(t, execs, 1)
	e := execs[0]
	assert.Nil(t, e.workerID, "expected nil worker_id for bee execution")
	assert.Equal(t, string(model.ExecStatusCompleted), e.status)
	assert.NotEmpty(t, e.logPath, "expected non-empty log_path — PrepareLogPath should set it before process runs")
}

func TestFeeder_ExecutionFailedOnBeeError(t *testing.T) {
	db, ms, ts, ss, es := setupFeederDB(t)
	insertMessage(t, db, "m1", "beta:c:u", "hello bee")

	runner := &mockBeeRunner{err: fmt.Errorf("bee crashed")}
	f := newFeeder(ms, ts, ss, es, runner)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	go f.Run(ctx)
	time.Sleep(700 * time.Millisecond)

	var status string
	err := db.QueryRow(`SELECT status FROM bee_executions`).Scan(&status)
	require.NoError(t, err)
	assert.Equal(t, "failed", status)
}

type failureNotifyCall struct {
	messageID string
	info      model.FailureInfo
}

type mockFailureNotifier struct {
	mu    sync.Mutex
	calls []failureNotifyCall
}

func (m *mockFailureNotifier) NotifyTaskFailure(_ context.Context, target delivery.Target, info model.FailureInfo) error {
	m.mu.Lock()
	m.calls = append(m.calls, failureNotifyCall{messageID: target.MessageID, info: info})
	m.mu.Unlock()
	return nil
}

func (m *mockFailureNotifier) getCalls() []failureNotifyCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]failureNotifyCall{}, m.calls...)
}

func TestFeeder_ImmediateFailure_MarksFailedAndNotifies(t *testing.T) {
	db, ms, ts, ss, es := setupFeederDB(t)
	insertMessage(t, db, "m1", "beta:c:u", "hello")

	runner := &mockBeeRunner{err: fmt.Errorf("bee crashed")}
	notifier := &mockFailureNotifier{}
	cfg := config.BeeConfig{}
	cfg.Engine.Timeout.Bee = 5 * time.Second
	cfg.Feeder.MaxConcurrentBee = 5
	f := bee.NewFeeder(ms, ts, ss, es, runner, "/tmp", cfg, enginecfg.NewStore("", "", aitest.IDs()), bee.WithFailureNotifier(notifier))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go f.Run(ctx)

	// One poll cycle is enough — failure must be immediate, no retries.
	time.Sleep(bee.PollInterval + 500*time.Millisecond)

	var status string
	db.QueryRow(`SELECT status FROM bee_platform_messages WHERE id='m1'`).Scan(&status)
	assert.Equal(t, "failed", status, "expected status=failed immediately after one failure")

	calls := notifier.getCalls()
	require.Len(t, calls, 1, "expected notifier called once")
	assert.Equal(t, "m1", calls[0].messageID)
	assert.NotEmpty(t, calls[0].info.Reason, "expected non-empty Reason in FailureInfo")
}

func TestFeeder_MultipleSessionKeys_ProcessedConcurrently(t *testing.T) {
	db, ms, ts, ss, es := setupFeederDB(t)

	// Insert 3 messages from 3 different sessions.
	insertMessage(t, db, "m1", "beta:c:u1", "msg1")
	insertMessage(t, db, "m2", "beta:c:u2", "msg2")
	insertMessage(t, db, "m3", "beta:c:u3", "msg3")

	var (
		mu         sync.Mutex
		startTimes []time.Time
	)
	// Runner records when each call starts; simulate 200ms bee execution.
	slowRunner := &callbackBeeRunner{
		fn: func() {
			mu.Lock()
			startTimes = append(startTimes, time.Now())
			mu.Unlock()
			time.Sleep(200 * time.Millisecond)
		},
	}

	cfg := config.BeeConfig{}
	cfg.Engine.Timeout.Bee = 5 * time.Second
	cfg.Feeder.MaxConcurrentBee = 5
	f := bee.NewFeeder(ms, ts, ss, es, slowRunner, "/tmp", cfg, enginecfg.NewStore("", "", aitest.IDs()))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go f.Run(ctx)

	// Wait long enough for one tick + concurrent bee runs to complete.
	time.Sleep(800 * time.Millisecond)

	mu.Lock()
	n := len(startTimes)
	mu.Unlock()

	require.Equal(t, 3, n, "expected 3 concurrent bee invocations")

	// All 3 should have started within a short window (concurrent, not serial).
	mu.Lock()
	first, last := startTimes[0], startTimes[0]
	for _, st := range startTimes[1:] {
		if st.Before(first) {
			first = st
		}
		if st.After(last) {
			last = st
		}
	}
	mu.Unlock()
	assert.LessOrEqual(t, last.Sub(first), 100*time.Millisecond, "bee invocations should start nearly simultaneously (concurrent)")
}

func TestFeeder_SemaphoreLimit_CapsActiveBee(t *testing.T) {
	db, ms, ts, ss, es := setupFeederDB(t)

	// Insert 6 messages from 6 different sessions — more than MaxConcurrentBee.
	for i := range 6 {
		insertMessage(t, db, fmt.Sprintf("m%d", i), fmt.Sprintf("beta:c:u%d", i), "msg")
	}

	var (
		mu            sync.Mutex
		maxActive     int
		currentActive int
	)
	slowRunner := &callbackBeeRunner{
		fn: func() {
			mu.Lock()
			currentActive++
			if currentActive > maxActive {
				maxActive = currentActive
			}
			mu.Unlock()
			time.Sleep(300 * time.Millisecond)
			mu.Lock()
			currentActive--
			mu.Unlock()
		},
	}

	cfg := config.BeeConfig{}
	cfg.Engine.Timeout.Bee = 5 * time.Second
	cfg.Feeder.MaxConcurrentBee = 3 // deliberately small
	f := bee.NewFeeder(ms, ts, ss, es, slowRunner, "/tmp", cfg, enginecfg.NewStore("", "", aitest.IDs()))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go f.Run(ctx)
	time.Sleep(2 * time.Second)

	mu.Lock()
	peak := maxActive
	mu.Unlock()

	assert.LessOrEqual(t, peak, 3, "semaphore should cap concurrent bee at 3")
	// All 6 messages should eventually be processed.
	var processed int
	db.QueryRow(`SELECT COUNT(*) FROM bee_platform_messages WHERE status = 'bee_processed'`).Scan(&processed)
	assert.Equal(t, 6, processed, "expected all 6 messages processed")
}

// callbackBeeRunner invokes fn synchronously inside Run, then signals done.
type callbackBeeRunner struct {
	fn   func()
	done chan struct{}
}

func (r *callbackBeeRunner) CollectTokenUsage(_ context.Context, _ []string) (map[string][]ai.TokenUsage, error) {
	return nil, nil
}

func (r *callbackBeeRunner) Run(_ context.Context, _, _ string, _ ai.RunOptions, _ string) (ai.RunResult, error) {
	ch := make(chan ai.Output, 1)
	go func() {
		r.fn()
		if r.done != nil {
			close(r.done)
		}
		ch <- ai.Output{Type: ai.OutputDone}
		close(ch)
	}()
	return ai.RunResult{Process: &mockProcess{}, Output: ch, ExtractResult: func(string) string { return "" }}, nil
}

func TestFeeder_DirectDispatch_NoPrefix_FallsBackToBee(t *testing.T) {
	db, ms, ts, ss, es := setupFeederDB(t)
	insertMessage(t, db, "m1", "sk1", "hello world")

	runner := &mockBeeRunner{}
	ws := store.NewWorkerStore(db)

	f := bee.NewFeeder(ms, ts, ss, es, runner, "/tmp", config.BeeConfig{
		Engine: config.EngineDefaultConfig{Timeout: config.EngineTimeoutConfig{Bee: 5 * time.Second}},
		Feeder: config.FeederConfig{MaxConcurrentBee: 5},
	}, enginecfg.NewStore("", "", aitest.IDs()), bee.WithWorkerDispatch(ws))

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	go f.Run(ctx)
	time.Sleep(700 * time.Millisecond)

	// Bee must have been called (normal flow)
	assert.NotEmpty(t, runner.getCalls(), "expected bee runner to be called for non-direct-dispatch message")
}

func TestFeeder_DirectDispatch_WorkerNotFound_FallsBackToBee(t *testing.T) {
	db, ms, ts, ss, es := setupFeederDB(t)
	insertMessage(t, db, "m1", "sk1", "unknown do something")

	runner := &mockBeeRunner{}
	ws := store.NewWorkerStore(db) // empty store: "unknown" worker does not exist

	cfg := config.BeeConfig{}
	cfg.Engine.Timeout.Bee = 5 * time.Second
	cfg.Feeder.MaxConcurrentBee = 5
	f := bee.NewFeeder(ms, ts, ss, es, runner, "/tmp", cfg, enginecfg.NewStore("", "", aitest.IDs()),
		bee.WithWorkerDispatch(ws))

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	go f.Run(ctx)
	time.Sleep(700 * time.Millisecond)

	assert.NotEmpty(t, runner.getCalls(), "expected bee runner to be called when worker not found")
}

func TestFeeder_PreflightSessionContextWrittenBeforeRun(t *testing.T) {
	db, ms, ts, ss, es := setupFeederDB(t)
	insertMessage(t, db, "m1", "beta:c:u", "hello")

	var upsertCalledBeforeRun atomic.Bool
	done := make(chan struct{})

	// Verify the session context row exists in DB when runner.Run() is called.
	runner := &callbackBeeRunner{
		fn: func() {
			ctx := context.Background()
			sid, _, err := ss.GetSessionContext(ctx, "beta:c:u", store.BeeAgentID)
			if err == nil && sid != "" {
				upsertCalledBeforeRun.Store(true)
			}
		},
		done: done,
	}

	f := newFeeder(ms, ts, ss, es, runner)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	go f.Run(ctx)

	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("timed out waiting for runner.Run() to be called")
	}

	assert.True(t, upsertCalledBeforeRun.Load(), "expected session context to be written before runner.Run() is called")
}

func TestFeeder_DirectDispatch_SkipsBee(t *testing.T) {
	for _, tc := range []struct {
		name string
		msg  string
	}{
		{"at-prefix", "@天天 write a report"},
		{"at-prefix-newline", "@天天\nwrite a report"},
		{"no-prefix-space", "天天 write a report"},
		{"no-prefix-newline", "天天\nwrite a report"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, ms, ts, ss, es := setupFeederDB(t)
			insertMessage(t, db, "m1", "sk1", tc.msg)

			runner := &mockBeeRunner{}
			ws := store.NewWorkerStore(db)
			w, err := ws.Create(model.Worker{Name: "天天", WorkDir: "/tmp/tt"})
			require.NoError(t, err)

			cfg := config.BeeConfig{}
			cfg.Engine.Timeout.Bee = 5 * time.Second
			cfg.Feeder.MaxConcurrentBee = 5
			f := bee.NewFeeder(ms, ts, ss, es, runner, "/tmp", cfg, enginecfg.NewStore("", "", aitest.IDs()),
				bee.WithWorkerDispatch(ws))

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			go f.Run(ctx)
			time.Sleep(700 * time.Millisecond)

			assert.Empty(t, runner.getCalls(), "expected bee runner NOT to be called for direct dispatch")

			var workerID, instruction, status string
			db.QueryRow(`SELECT worker_id, instruction, status FROM bee_tasks WHERE message_id='m1'`).Scan(&workerID, &instruction, &status)
			assert.Equal(t, w.ID, workerID)
			assert.Equal(t, "write a report", instruction)
			assert.Equal(t, model.TaskStatusPending, status)

			var msgStatus string
			db.QueryRow(`SELECT status FROM bee_platform_messages WHERE id='m1'`).Scan(&msgStatus)
			assert.Equal(t, store.MsgStatusBeeProcessed, msgStatus)
		})
	}
}

type workerChatEnv struct {
	db       *sql.DB
	ms       *store.MessageStore
	ws       *store.WorkerStore
	runner   *mockBeeRunner
	notifier *mockFailureNotifier
	root     string
}

func startWorkerChatFeeder(t *testing.T, opts ...bee.Option) *workerChatEnv {
	t.Helper()
	db, ms, ts, ss, es := setupFeederDB(t)
	env := &workerChatEnv{
		db:       db,
		ms:       ms,
		ws:       store.NewWorkerStore(db),
		runner:   &mockBeeRunner{},
		notifier: &mockFailureNotifier{},
		root:     t.TempDir(),
	}
	cfg := config.BeeConfig{}
	cfg.Engine.Timeout.Bee = 5 * time.Second
	cfg.Feeder.MaxConcurrentBee = 5
	all := append([]bee.Option{
		bee.WithWorkerDispatch(env.ws),
		bee.WithFailureNotifier(env.notifier),
		bee.WithLocalMediaRoot(env.root),
	}, opts...)
	f := bee.NewFeeder(ms, ts, ss, es, env.runner, "/tmp", cfg, enginecfg.NewStore("", "", aitest.IDs()), all...)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go f.Run(ctx)
	return env
}

func (e *workerChatEnv) createWorker(t *testing.T, name string) model.Worker {
	t.Helper()
	w, err := e.ws.Create(model.Worker{Name: name, WorkDir: t.TempDir()})
	require.NoError(t, err)
	return w
}

func (e *workerChatEnv) awaitStatus(t *testing.T, id, want string) {
	t.Helper()
	require.Eventually(t, func() bool {
		var status string
		if err := e.db.QueryRow(`SELECT status FROM bee_platform_messages WHERE id=?`, id).Scan(&status); err != nil {
			return false
		}
		return status == want
	}, 5*time.Second, 50*time.Millisecond, "message %s did not reach status %s", id, want)
}

func (e *workerChatEnv) taskCount(t *testing.T) int {
	t.Helper()
	var n int
	require.NoError(t, e.db.QueryRow(`SELECT COUNT(*) FROM bee_tasks`).Scan(&n))
	return n
}

type taskRow struct {
	messageID, sessionKey, platform, workerID, instruction, taskType, status string
}

func (e *workerChatEnv) onlyTask(t *testing.T) taskRow {
	t.Helper()
	require.Equal(t, 1, e.taskCount(t))
	var r taskRow
	require.NoError(t, e.db.QueryRow(
		`SELECT message_id, session_key, platform, worker_id, instruction, type, status FROM bee_tasks`,
	).Scan(&r.messageID, &r.sessionKey, &r.platform, &r.workerID, &r.instruction, &r.taskType, &r.status))
	return r
}

func insertLocalMessage(t *testing.T, db *sql.DB, id, sessionKey, content string) {
	t.Helper()
	now := time.Now().UnixMilli()
	_, err := db.Exec(
		`INSERT INTO bee_platform_messages (id, session_key, platform, content, status, received_at, created_at, updated_at)
		 VALUES (?, ?, 'local', ?, 'received', ?, ?, ?)`,
		id, sessionKey, content, now, now, now,
	)
	require.NoError(t, err)
}

func TestFeederWorkerChatCreatesTaskWithoutBee(t *testing.T) {
	env := startWorkerChatFeeder(t)
	w := env.createWorker(t, "writer")
	key := local.WorkerSessionKey("u1", w.ID)
	insertLocalMessage(t, env.db, "m1", key, "draft the release notes")

	env.awaitStatus(t, "m1", store.MsgStatusBeeProcessed)

	task := env.onlyTask(t)
	assert.Equal(t, "m1", task.messageID)
	assert.Equal(t, key, task.sessionKey)
	assert.Equal(t, local.PlatformID, task.platform)
	assert.Equal(t, w.ID, task.workerID)
	assert.Equal(t, "draft the release notes", task.instruction)
	assert.Equal(t, model.TaskTypeImmediate, task.taskType)
	assert.Equal(t, model.TaskStatusPending, task.status)
	assert.Empty(t, env.runner.getCalls(), "bee must never run for a worker conversation")
	assert.Empty(t, env.notifier.getCalls())
}

func TestFeederWorkerChatIgnoresDirectMention(t *testing.T) {
	env := startWorkerChatFeeder(t)
	w := env.createWorker(t, "writer")
	other := env.createWorker(t, "editor")
	key := local.WorkerSessionKey("u1", w.ID)
	insertLocalMessage(t, env.db, "m1", key, "@editor please review this")

	env.awaitStatus(t, "m1", store.MsgStatusBeeProcessed)

	task := env.onlyTask(t)
	assert.Equal(t, w.ID, task.workerID)
	assert.NotEqual(t, other.ID, task.workerID)
	assert.Equal(t, "@editor please review this", task.instruction)
	assert.Empty(t, env.runner.getCalls())
}

func TestFeederWorkerChatMergesBurst(t *testing.T) {
	env := startWorkerChatFeeder(t)
	w := env.createWorker(t, "writer")
	key := local.WorkerSessionKey("u1", w.ID)
	base := time.Now().Add(-time.Minute).UnixMilli()
	_, err := env.ms.CreateBatch(context.Background(), []store.BatchMsg{
		{ID: "m1", SessionKey: key, Platform: local.PlatformID, Content: "first", MessageTime: base, Status: store.MsgStatusMerged, MergedInto: "m3"},
		{ID: "m2", SessionKey: key, Platform: local.PlatformID, Content: "second", MessageTime: base + 1, Status: store.MsgStatusMerged, MergedInto: "m3"},
		{ID: "m3", SessionKey: key, Platform: local.PlatformID, Content: "third", MessageTime: base + 2, Status: store.MsgStatusReceived},
	})
	require.NoError(t, err)

	env.awaitStatus(t, "m3", store.MsgStatusBeeProcessed)

	task := env.onlyTask(t)
	assert.Equal(t, "m3", task.messageID)
	assert.Equal(t, "first\n\nsecond\n\nthird", task.instruction)
	assert.Empty(t, env.runner.getCalls())
}

func TestFeederWorkerChatRendersAttachments(t *testing.T) {
	env := startWorkerChatFeeder(t)
	w := env.createWorker(t, "writer")
	key := local.WorkerSessionKey("u1", w.ID)
	insertLocalMessage(t, env.db, "m1", key, local.EncodeMediaPaths([]string{"a_x.png"}, "see"))

	env.awaitStatus(t, "m1", store.MsgStatusBeeProcessed)

	task := env.onlyTask(t)
	assert.Contains(t, task.instruction, "[attachment] "+filepath.Join(env.root, "u1", "a_x.png"))
	assert.Contains(t, task.instruction, "see")
	assert.NotContains(t, task.instruction, "\x00")
}

func TestFeederWorkerChatRendersAttachmentsInMergedMessages(t *testing.T) {
	env := startWorkerChatFeeder(t)
	w := env.createWorker(t, "writer")
	key := local.WorkerSessionKey("u1", w.ID)
	base := time.Now().Add(-time.Minute).UnixMilli()
	_, err := env.ms.CreateBatch(context.Background(), []store.BatchMsg{
		{ID: "m1", SessionKey: key, Platform: local.PlatformID, Content: local.EncodeMediaPaths([]string{"a_x.png"}, "first"), MessageTime: base, Status: store.MsgStatusMerged, MergedInto: "m2"},
		{ID: "m2", SessionKey: key, Platform: local.PlatformID, Content: local.EncodeMediaPaths([]string{"b_y.pdf"}, "second"), MessageTime: base + 1, Status: store.MsgStatusReceived},
	})
	require.NoError(t, err)

	env.awaitStatus(t, "m2", store.MsgStatusBeeProcessed)

	dir := filepath.Join(env.root, "u1")
	want := "[attachment] " + filepath.Join(dir, "a_x.png") + "\nfirst\n\n[attachment] " + filepath.Join(dir, "b_y.pdf") + "\nsecond"
	assert.Equal(t, want, env.onlyTask(t).instruction)
}

func TestFeederWorkerChatWithoutMediaRootKeepsMarkers(t *testing.T) {
	env := startWorkerChatFeeder(t, bee.WithLocalMediaRoot(""))
	w := env.createWorker(t, "writer")
	key := local.WorkerSessionKey("u1", w.ID)
	content := local.EncodeMediaPaths([]string{"a_x.png"}, "see")
	insertLocalMessage(t, env.db, "m1", key, content)

	env.awaitStatus(t, "m1", store.MsgStatusBeeProcessed)

	assert.Equal(t, content, env.onlyTask(t).instruction)
	assert.Empty(t, env.runner.getCalls())
}

func TestFeederWorkerChatDeletedWorker(t *testing.T) {
	env := startWorkerChatFeeder(t)
	key := local.WorkerSessionKey("u1", "missing-worker")
	insertLocalMessage(t, env.db, "m1", key, "anyone there?")

	env.awaitStatus(t, "m1", store.MsgStatusFailed)

	assert.Equal(t, 0, env.taskCount(t))
	assert.Empty(t, env.runner.getCalls(), "bee must never run for a worker conversation")
	calls := env.notifier.getCalls()
	require.Len(t, calls, 1)
	assert.Equal(t, "m1", calls[0].messageID)
	assert.Equal(t, "worker not found", calls[0].info.Reason)
}

func TestFeederWorkerChatWithoutWorkerLookupFails(t *testing.T) {
	db, ms, ts, ss, es := setupFeederDB(t)
	runner := &mockBeeRunner{}
	notifier := &mockFailureNotifier{}
	cfg := config.BeeConfig{}
	cfg.Engine.Timeout.Bee = 5 * time.Second
	cfg.Feeder.MaxConcurrentBee = 5
	f := bee.NewFeeder(ms, ts, ss, es, runner, "/tmp", cfg, enginecfg.NewStore("", "", aitest.IDs()), bee.WithFailureNotifier(notifier))
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go f.Run(ctx)

	insertLocalMessage(t, db, "m1", local.WorkerSessionKey("u1", "w1"), "hello")

	require.Eventually(t, func() bool {
		var status string
		db.QueryRow(`SELECT status FROM bee_platform_messages WHERE id='m1'`).Scan(&status)
		return status == store.MsgStatusFailed
	}, 5*time.Second, 50*time.Millisecond)
	assert.Empty(t, runner.getCalls())
	assert.Len(t, notifier.getCalls(), 1)
}

func TestFeederBeeChatStillRunsBee(t *testing.T) {
	env := startWorkerChatFeeder(t)
	insertLocalMessage(t, env.db, "m1", local.SessionKey("u1"), "hello bee")

	env.awaitStatus(t, "m1", store.MsgStatusBeeProcessed)

	assert.NotEmpty(t, env.runner.getCalls())
	assert.Equal(t, 0, env.taskCount(t))
}

func TestFeederBeeChatRendersAttachments(t *testing.T) {
	env := startWorkerChatFeeder(t)
	insertLocalMessage(t, env.db, "m1", local.SessionKey("u1"), local.EncodeMediaPaths([]string{"a_x.png"}, "what is this?"))

	env.awaitStatus(t, "m1", store.MsgStatusBeeProcessed)

	calls := env.runner.getCalls()
	require.Len(t, calls, 1)
	assert.Contains(t, calls[0].prompt, "[attachment] "+filepath.Join(env.root, "u1", "a_x.png")+"\nwhat is this?")
	assert.NotContains(t, calls[0].prompt, "\x00")
}

func TestFeederBeeChatRendersAttachmentsInMergedMessages(t *testing.T) {
	env := startWorkerChatFeeder(t)
	key := local.SessionKey("u1")
	base := time.Now().Add(-time.Minute).UnixMilli()
	_, err := env.ms.CreateBatch(context.Background(), []store.BatchMsg{
		{ID: "m1", SessionKey: key, Platform: local.PlatformID, Content: local.EncodeMediaPaths([]string{"a_x.png"}, "first"), MessageTime: base, Status: store.MsgStatusMerged, MergedInto: "m2"},
		{ID: "m2", SessionKey: key, Platform: local.PlatformID, Content: local.EncodeMediaPaths([]string{"b_y.pdf"}, "second"), MessageTime: base + 1, Status: store.MsgStatusReceived},
	})
	require.NoError(t, err)

	env.awaitStatus(t, "m2", store.MsgStatusBeeProcessed)

	calls := env.runner.getCalls()
	require.Len(t, calls, 1)
	dir := filepath.Join(env.root, "u1")
	want := "[attachment] " + filepath.Join(dir, "a_x.png") + "\nfirst\n\n---\n\n[attachment] " + filepath.Join(dir, "b_y.pdf") + "\nsecond"
	assert.Contains(t, calls[0].prompt, want)
	assert.NotContains(t, calls[0].prompt, "\x00")
}

func TestFeederIMMessageKeepsFileMarkerText(t *testing.T) {
	env := startWorkerChatFeeder(t)
	insertMessage(t, env.db, "m1", "beta:chat-1", "[file] notes.txt\nplain text")

	env.awaitStatus(t, "m1", store.MsgStatusBeeProcessed)

	calls := env.runner.getCalls()
	require.Len(t, calls, 1)
	assert.Contains(t, calls[0].prompt, "[file] notes.txt\nplain text")
	assert.NotContains(t, calls[0].prompt, "[attachment]")
}

type cardRecorder struct {
	mu   sync.Mutex
	sent []platform.OutboundMessage
}

func (c *cardRecorder) Send(_ context.Context, msg platform.OutboundMessage) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sent = append(c.sent, msg)
	return nil
}

func (c *cardRecorder) messages() []platform.OutboundMessage {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]platform.OutboundMessage(nil), c.sent...)
}

func startHandoffFeeder(t *testing.T, db *sql.DB, ms *store.MessageStore, ts *store.TaskStore, ss *store.SessionStore, es *store.ExecutionStore, runner *mockBeeRunner) (*store.WorkerStore, *cardRecorder) {
	t.Helper()
	ws := store.NewWorkerStore(db)
	cards := &cardRecorder{}
	handoff := localchat.NewHandoff(ms, ts, cards, local.NewEventPublisher(local.NewSSEHub()))
	cfg := config.BeeConfig{}
	cfg.Engine.Timeout.Bee = 5 * time.Second
	cfg.Feeder.MaxConcurrentBee = 5
	f := bee.NewFeeder(ms, ts, ss, es, runner, "/tmp", cfg, enginecfg.NewStore("", "", aitest.IDs()),
		bee.WithWorkerDispatch(ws), bee.WithHandoff(handoff))
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go f.Run(ctx)
	return ws, cards
}

func TestDirectMentionInBeeChatHandsOff(t *testing.T) {
	db, ms, ts, ss, es := setupFeederDB(t)
	runner := &mockBeeRunner{}
	ws, cards := startHandoffFeeder(t, db, ms, ts, ss, es, runner)
	w, err := ws.Create(model.Worker{Name: "writer", WorkDir: t.TempDir()})
	require.NoError(t, err)
	insertLocalMessage(t, db, "m1", local.SessionKey("u1"), "@writer draft the release notes")

	require.Eventually(t, func() bool {
		var status string
		if err := db.QueryRow(`SELECT status FROM bee_platform_messages WHERE id='m1'`).Scan(&status); err != nil {
			return false
		}
		return status == store.MsgStatusBeeProcessed
	}, 5*time.Second, 50*time.Millisecond)

	assert.Empty(t, runner.getCalls(), "bee must not run for a direct mention")

	workerKey := local.WorkerSessionKey("u1", w.ID)
	var messageID, sessionKey, workerID, instruction string
	require.NoError(t, db.QueryRow(
		`SELECT message_id, session_key, worker_id, instruction FROM bee_tasks`,
	).Scan(&messageID, &sessionKey, &workerID, &instruction))
	assert.Equal(t, workerKey, sessionKey)
	assert.Equal(t, w.ID, workerID)
	assert.Equal(t, "draft the release notes", instruction)
	assert.NotEqual(t, "m1", messageID)

	var rowKey, senderType, content string
	require.NoError(t, db.QueryRow(
		`SELECT session_key, sender_type, content FROM bee_platform_messages WHERE id=?`, messageID,
	).Scan(&rowKey, &senderType, &content))
	assert.Equal(t, workerKey, rowKey)
	assert.Equal(t, store.SenderTypeBee, senderType)
	assert.Equal(t, "draft the release notes", content)

	sent := cards.messages()
	require.Len(t, sent, 1)
	assert.Equal(t, local.SessionKey("u1"), sent[0].SessionKey)
	assert.Equal(t, store.OutboundKindHandoff, sent[0].Kind)
	assert.Equal(t, "m1", sent[0].InboundMsgID)
}

func TestDirectMentionOnIMChannelIgnoresHandoff(t *testing.T) {
	db, ms, ts, ss, es := setupFeederDB(t)
	runner := &mockBeeRunner{}
	ws, cards := startHandoffFeeder(t, db, ms, ts, ss, es, runner)
	w, err := ws.Create(model.Worker{Name: "writer", WorkDir: t.TempDir()})
	require.NoError(t, err)
	insertMessage(t, db, "m1", "beta:c1:u1", "@writer draft the release notes")

	require.Eventually(t, func() bool {
		var n int
		return db.QueryRow(`SELECT COUNT(*) FROM bee_tasks`).Scan(&n) == nil && n == 1
	}, 5*time.Second, 50*time.Millisecond)

	var messageID, sessionKey, workerID string
	require.NoError(t, db.QueryRow(`SELECT message_id, session_key, worker_id FROM bee_tasks`).Scan(&messageID, &sessionKey, &workerID))
	assert.Equal(t, "m1", messageID)
	assert.Equal(t, "beta:c1:u1", sessionKey)
	assert.Equal(t, w.ID, workerID)
	assert.Empty(t, cards.messages())
	var total int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM bee_platform_messages`).Scan(&total))
	assert.Equal(t, 1, total)
}
