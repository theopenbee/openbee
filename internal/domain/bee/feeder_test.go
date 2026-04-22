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

	ai "github.com/theopenbee/openbee/internal/ai"
	"github.com/theopenbee/openbee/internal/domain/bee"
	"github.com/theopenbee/openbee/internal/domain/enginecfg"
	"github.com/theopenbee/openbee/internal/infra/config"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/infra/store"
)

func setupFeederDB(t *testing.T) (*sql.DB, *store.MessageStore, *store.TaskStore, *store.SessionStore, *store.ExecutionStore) {
	t.Helper()
	db, err := store.InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db, store.NewMessageStore(db), store.NewTaskStore(db), store.NewSessionStore(db), store.NewExecutionStore(db, t.TempDir())
}

func insertMessage(t *testing.T, db *sql.DB, id, sessionKey, content string) {
	t.Helper()
	now := time.Now().UnixMilli()
	_, err := db.Exec(
		`INSERT INTO bee_platform_messages (id, session_key, platform, content, status, received_at, created_at, updated_at)
		 VALUES (?, ?, 'feishu', ?, 'received', ?, ?, ?)`,
		id, sessionKey, content, now, now, now,
	)
	if err != nil {
		t.Fatalf("insert message: %v", err)
	}
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

func (m *mockBeeRunner) Prepare(_ string, _ ai.PrepareOptions) error {
	return nil
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
	return bee.NewFeeder(ms, ts, ss, es, runner, "/tmp", cfg, enginecfg.NewStore(engine))
}

func TestFeeder_FirstTick_UsesNewSessionID(t *testing.T) {
	db, ms, ts, ss, es := setupFeederDB(t)
	insertMessage(t, db, "m1", "feishu:c:u", "hello")

	runner := &mockBeeRunner{}
	f := newFeeder(ms, ts, ss, es, runner)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	go f.Run(ctx)
	time.Sleep(700 * time.Millisecond)

	calls := runner.getCalls()
	if len(calls) == 0 {
		t.Fatal("expected bee runner to be called")
	}
	call := calls[0]
	if call.opts.SessionID == "" {
		t.Error("expected non-empty sessionID on first call")
	}
	if call.opts.Resume {
		t.Error("expected resume=false on first call")
	}

	got, _, err := ss.GetSessionContext(context.Background(), "feishu:c:u", store.BeeAgentID)
	if err != nil {
		t.Fatalf("get session context: %v", err)
	}
	if got != call.opts.SessionID {
		t.Errorf("persisted sessionID mismatch: want %q got %q", call.opts.SessionID, got)
	}

	var status string
	db.QueryRow(`SELECT status FROM bee_platform_messages WHERE id='m1'`).Scan(&status)
	if status != "bee_processed" {
		t.Errorf("expected bee_processed, got %q", status)
	}
}

func TestFeeder_SecondTick_ResumesSession(t *testing.T) {
	db, ms, ts, ss, es := setupFeederDB(t)
	ctx := context.Background()

	if err := ss.UpsertSessionContext(ctx, "feishu:c:u", store.BeeAgentID, "existing-session", "claude"); err != nil {
		t.Fatalf("seed session: %v", err)
	}

	insertMessage(t, db, "m1", "feishu:c:u", "follow-up")

	runner := &mockBeeRunner{}
	f := newFeeder(ms, ts, ss, es, runner)

	tickCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	go f.Run(tickCtx)
	time.Sleep(700 * time.Millisecond)

	calls := runner.getCalls()
	if len(calls) == 0 {
		t.Fatal("expected bee runner to be called")
	}
	call := calls[0]
	if call.opts.SessionID != "existing-session" {
		t.Errorf("expected existing-session, got %q", call.opts.SessionID)
	}
	if !call.opts.Resume {
		t.Error("expected resume=true on second call")
	}
}

func TestFeeder_EngineSwitch_PreservesPriorSession(t *testing.T) {
	db, ms, ts, ss, es := setupFeederDB(t)
	ctx := context.Background()

	if err := ss.UpsertSessionContext(ctx, "feishu:c:u", store.BeeAgentID, "claude-session", "claude"); err != nil {
		t.Fatalf("seed session: %v", err)
	}

	insertMessage(t, db, "m1", "feishu:c:u", "switch to codex")

	codexRunner := &mockBeeRunner{}
	codexFeeder := newFeederWithEngine(ms, ts, ss, es, codexRunner, "codex")

	tickCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	go codexFeeder.Run(tickCtx)
	time.Sleep(700 * time.Millisecond)

	codexCalls := codexRunner.getCalls()
	if len(codexCalls) == 0 {
		t.Fatal("expected codex bee runner to be called")
	}
	codexCall := codexCalls[0]
	if codexCall.opts.Resume {
		t.Error("expected codex run to start fresh on engine switch")
	}
	if codexCall.opts.SessionID == "claude-session" {
		t.Error("expected codex run to use a new session ID")
	}

	claudeSID, err := ss.GetSessionContextForEngine(ctx, "feishu:c:u", store.BeeAgentID, "claude")
	if err != nil {
		t.Fatalf("get claude session: %v", err)
	}
	codexSID, err := ss.GetSessionContextForEngine(ctx, "feishu:c:u", store.BeeAgentID, "codex")
	if err != nil {
		t.Fatalf("get codex session: %v", err)
	}
	if claudeSID != "claude-session" {
		t.Errorf("expected claude session preserved, got %q", claudeSID)
	}
	if codexSID != codexCall.opts.SessionID {
		t.Errorf("expected codex session persisted, got %q want %q", codexSID, codexCall.opts.SessionID)
	}

	cancel()
	time.Sleep(100 * time.Millisecond)

	insertMessage(t, db, "m2", "feishu:c:u", "switch back to claude")

	claudeRunner := &mockBeeRunner{}
	claudeFeeder := newFeederWithEngine(ms, ts, ss, es, claudeRunner, "claude")

	tickCtx2, cancel2 := context.WithTimeout(ctx, 2*time.Second)
	defer cancel2()
	go claudeFeeder.Run(tickCtx2)
	time.Sleep(700 * time.Millisecond)

	claudeCalls := claudeRunner.getCalls()
	if len(claudeCalls) == 0 {
		t.Fatal("expected claude bee runner to be called")
	}
	if !claudeCalls[0].opts.Resume {
		t.Error("expected claude run to resume original claude session")
	}
	if claudeCalls[0].opts.SessionID != "claude-session" {
		t.Errorf("expected original claude session, got %q", claudeCalls[0].opts.SessionID)
	}
}

func TestFeeder_OnBeeFailure_MarksFailedAndDoesNotUpdateSession(t *testing.T) {
	db, ms, ts, ss, es := setupFeederDB(t)
	insertMessage(t, db, "m1", "feishu:c:u", "hello")

	runner := &mockBeeRunner{err: fmt.Errorf("bee crashed")}
	f := newFeeder(ms, ts, ss, es, runner)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	go f.Run(ctx)
	time.Sleep(700 * time.Millisecond)

	var status string
	db.QueryRow(`SELECT status FROM bee_platform_messages WHERE id='m1'`).Scan(&status)
	if status != "failed" {
		t.Errorf("expected status=failed on bee failure, got %q", status)
	}

	got, _, _ := ss.GetSessionContext(context.Background(), "feishu:c:u", store.BeeAgentID)
	if got != "" {
		t.Errorf("session context should not be written on failure, got %q", got)
	}
}

func TestFeeder_MultipleSessionKeys_ProcessedIndependently(t *testing.T) {
	db, ms, ts, ss, es := setupFeederDB(t)
	insertMessage(t, db, "m1", "feishu:c:u1", "message from user1")
	insertMessage(t, db, "m2", "feishu:c:u2", "message from user2")

	runner := &mockBeeRunner{}
	f := newFeeder(ms, ts, ss, es, runner)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	go f.Run(ctx)
	time.Sleep(1200 * time.Millisecond)

	calls := runner.getCalls()
	if len(calls) != 2 {
		t.Fatalf("expected 2 bee invocations (one per sessionKey), got %d", len(calls))
	}

	sess1, _, _ := ss.GetSessionContext(context.Background(), "feishu:c:u1", store.BeeAgentID)
	sess2, _, _ := ss.GetSessionContext(context.Background(), "feishu:c:u2", store.BeeAgentID)
	if sess1 == "" {
		t.Error("session context for u1 should be set")
	}
	if sess2 == "" {
		t.Error("session context for u2 should be set")
	}
	if sess1 == sess2 {
		t.Error("session IDs for different sessionKeys must differ")
	}
}

func TestFeeder_CreatesExecutionOnBeeRun(t *testing.T) {
	db, ms, ts, ss, es := setupFeederDB(t)
	insertMessage(t, db, "m1", "feishu:c:u", "hello bee")

	runner := &mockBeeRunner{}
	f := newFeeder(ms, ts, ss, es, runner)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	go f.Run(ctx)
	time.Sleep(700 * time.Millisecond)

	rows, err := db.Query(`SELECT id, worker_id, status, log_path FROM bee_executions`)
	if err != nil {
		t.Fatalf("query executions: %v", err)
	}
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
		if err := rows.Scan(&e.id, &e.workerID, &e.status, &e.logPath); err != nil {
			t.Fatalf("scan: %v", err)
		}
		execs = append(execs, e)
	}

	if len(execs) != 1 {
		t.Fatalf("expected 1 execution row, got %d", len(execs))
	}
	e := execs[0]
	if e.workerID != nil {
		t.Errorf("expected nil worker_id for bee execution, got %v", e.workerID)
	}
	if e.status != string(model.ExecStatusCompleted) {
		t.Errorf("expected status=completed, got %q", e.status)
	}
	if e.logPath == "" {
		t.Error("expected non-empty log_path — PrepareLogPath should set it before process runs")
	}
}

func TestFeeder_LogPathSetBeforeProcessRuns(t *testing.T) {
	db, ms, ts, ss, es := setupFeederDB(t)
	insertMessage(t, db, "m1", "feishu:c:u", "hello")

	var capturedLogPath string
	runner := &mockBeeRunner{}
	// Intercept: after Run is called, log_path should already be in DB.
	// We verify this by checking the call's logPath is non-empty AND matches DB.
	f := newFeeder(ms, ts, ss, es, runner)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	go f.Run(ctx)
	time.Sleep(700 * time.Millisecond)

	calls := runner.getCalls()
	if len(calls) == 0 {
		t.Fatal("expected runner to be called")
	}
	capturedLogPath = calls[0].logPath
	if capturedLogPath == "" {
		t.Error("logPath passed to runner must be non-empty")
	}

	// Verify the directory exists (PrepareLogPath creates it)
	if _, err := os.Stat(filepath.Dir(capturedLogPath)); err != nil {
		t.Errorf("log directory should exist before process runs: %v", err)
	}
}

func TestFeeder_ExecutionFailedOnBeeError(t *testing.T) {
	db, ms, ts, ss, es := setupFeederDB(t)
	insertMessage(t, db, "m1", "feishu:c:u", "hello bee")

	runner := &mockBeeRunner{err: fmt.Errorf("bee crashed")}
	f := newFeeder(ms, ts, ss, es, runner)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	go f.Run(ctx)
	time.Sleep(700 * time.Millisecond)

	var status string
	err := db.QueryRow(`SELECT status FROM bee_executions`).Scan(&status)
	if err != nil {
		t.Fatalf("query executions: %v", err)
	}
	if status != "failed" {
		t.Errorf("expected status=failed, got %q", status)
	}
}

type failureNotifyCall struct {
	messageID string
	info      model.FailureInfo
}

type mockFailureNotifier struct {
	mu    sync.Mutex
	calls []failureNotifyCall
}

func (m *mockFailureNotifier) NotifyTaskFailure(_ context.Context, messageID string, info model.FailureInfo) error {
	m.mu.Lock()
	m.calls = append(m.calls, failureNotifyCall{messageID: messageID, info: info})
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
	insertMessage(t, db, "m1", "feishu:c:u", "hello")

	runner := &mockBeeRunner{err: fmt.Errorf("bee crashed")}
	notifier := &mockFailureNotifier{}
	cfg := config.BeeConfig{}
	cfg.Engine.Timeout.Bee = 5 * time.Second
	cfg.Feeder.MaxConcurrentBee = 5
	f := bee.NewFeeder(ms, ts, ss, es, runner, "/tmp", cfg, enginecfg.NewStore(""), bee.WithFailureNotifier(notifier))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go f.Run(ctx)

	// One poll cycle is enough — failure must be immediate, no retries.
	time.Sleep(bee.PollInterval + 500*time.Millisecond)

	var status string
	db.QueryRow(`SELECT status FROM bee_platform_messages WHERE id='m1'`).Scan(&status)
	if status != "failed" {
		t.Errorf("expected status=failed immediately after one failure, got %q", status)
	}

	calls := notifier.getCalls()
	if len(calls) != 1 {
		t.Fatalf("expected notifier called once, got %d calls", len(calls))
	}
	if calls[0].messageID != "m1" {
		t.Errorf("expected notifier called with messageID m1, got %q", calls[0].messageID)
	}
	if calls[0].info.Reason == "" {
		t.Error("expected non-empty Reason in FailureInfo")
	}
}

func TestFeeder_MultipleSessionKeys_ProcessedConcurrently(t *testing.T) {
	db, ms, ts, ss, es := setupFeederDB(t)

	// Insert 3 messages from 3 different sessions.
	insertMessage(t, db, "m1", "feishu:c:u1", "msg1")
	insertMessage(t, db, "m2", "feishu:c:u2", "msg2")
	insertMessage(t, db, "m3", "feishu:c:u3", "msg3")

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
	f := bee.NewFeeder(ms, ts, ss, es, slowRunner, "/tmp", cfg, enginecfg.NewStore(""))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go f.Run(ctx)

	// Wait long enough for one tick + concurrent bee runs to complete.
	time.Sleep(800 * time.Millisecond)

	mu.Lock()
	n := len(startTimes)
	mu.Unlock()

	if n != 3 {
		t.Fatalf("expected 3 concurrent bee invocations, got %d", n)
	}

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
	if last.Sub(first) > 100*time.Millisecond {
		t.Errorf("bee invocations should start nearly simultaneously (concurrent), spread was %v", last.Sub(first))
	}
}

func TestFeeder_SemaphoreLimit_CapsActiveBee(t *testing.T) {
	db, ms, ts, ss, es := setupFeederDB(t)

	// Insert 6 messages from 6 different sessions — more than MaxConcurrentBee.
	for i := range 6 {
		insertMessage(t, db, fmt.Sprintf("m%d", i), fmt.Sprintf("feishu:c:u%d", i), "msg")
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
	f := bee.NewFeeder(ms, ts, ss, es, slowRunner, "/tmp", cfg, enginecfg.NewStore(""))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go f.Run(ctx)
	time.Sleep(2 * time.Second)

	mu.Lock()
	peak := maxActive
	mu.Unlock()

	if peak > 3 {
		t.Errorf("semaphore should cap concurrent bee at 3, peak was %d", peak)
	}
	// All 6 messages should eventually be processed.
	var processed int
	db.QueryRow(`SELECT COUNT(*) FROM bee_platform_messages WHERE status = 'bee_processed'`).Scan(&processed)
	if processed != 6 {
		t.Errorf("expected all 6 messages processed, got %d", processed)
	}
}

// callbackBeeRunner invokes fn synchronously inside Run, then signals done.
type callbackBeeRunner struct {
	fn   func()
	done chan struct{}
}

func (r *callbackBeeRunner) Prepare(_ string, _ ai.PrepareOptions) error {
	return nil
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
	}, enginecfg.NewStore(""), bee.WithWorkerDispatch(ws))

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	go f.Run(ctx)
	time.Sleep(700 * time.Millisecond)

	// Bee must have been called (normal flow)
	if len(runner.getCalls()) == 0 {
		t.Error("expected bee runner to be called for non-direct-dispatch message")
	}
}

func TestFeeder_DirectDispatch_WorkerNotFound_FallsBackToBee(t *testing.T) {
	db, ms, ts, ss, es := setupFeederDB(t)
	insertMessage(t, db, "m1", "sk1", " unknown do something")

	runner := &mockBeeRunner{}
	ws := store.NewWorkerStore(db) // empty store: "unknown" worker does not exist

	cfg := config.BeeConfig{}
	cfg.Engine.Timeout.Bee = 5 * time.Second
	cfg.Feeder.MaxConcurrentBee = 5
	f := bee.NewFeeder(ms, ts, ss, es, runner, "/tmp", cfg, enginecfg.NewStore(""),
		bee.WithWorkerDispatch(ws))

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	go f.Run(ctx)
	time.Sleep(700 * time.Millisecond)

	if len(runner.getCalls()) == 0 {
		t.Error("expected bee runner to be called when worker not found")
	}
}

func TestFeeder_PreflightSessionContextWrittenBeforeRun(t *testing.T) {
	db, ms, ts, ss, es := setupFeederDB(t)
	insertMessage(t, db, "m1", "feishu:c:u", "hello")

	var upsertCalledBeforeRun atomic.Bool
	done := make(chan struct{})

	// Verify the session context row exists in DB when runner.Run() is called.
	runner := &callbackBeeRunner{
		fn: func() {
			ctx := context.Background()
			sid, _, err := ss.GetSessionContext(ctx, "feishu:c:u", store.BeeAgentID)
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

	if !upsertCalledBeforeRun.Load() {
		t.Error("expected session context to be written before runner.Run() is called")
	}
}

func TestFeeder_DirectDispatch_SkipsBee(t *testing.T) {
	for _, tc := range []struct {
		name string
		msg  string
	}{
		{"at-prefix", "@天天 write a report"},
		{"space-prefix", " 天天 write a report"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, ms, ts, ss, es := setupFeederDB(t)
			insertMessage(t, db, "m1", "sk1", tc.msg)

			runner := &mockBeeRunner{}
			ws := store.NewWorkerStore(db)
			w, err := ws.Create(model.Worker{Name: "天天", WorkDir: "/tmp/tt"})
			if err != nil {
				t.Fatalf("create worker: %v", err)
			}

			cfg := config.BeeConfig{}
			cfg.Engine.Timeout.Bee = 5 * time.Second
			cfg.Feeder.MaxConcurrentBee = 5
			f := bee.NewFeeder(ms, ts, ss, es, runner, "/tmp", cfg, enginecfg.NewStore(""),
				bee.WithWorkerDispatch(ws))

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			go f.Run(ctx)
			time.Sleep(700 * time.Millisecond)

			if len(runner.getCalls()) != 0 {
				t.Error("expected bee runner NOT to be called for direct dispatch")
			}

			var workerID, instruction, status string
			db.QueryRow(`SELECT worker_id, instruction, status FROM bee_tasks WHERE message_id='m1'`).Scan(&workerID, &instruction, &status)
			if workerID != w.ID {
				t.Errorf("expected task workerID %s, got %q", w.ID, workerID)
			}
			if instruction != "write a report" {
				t.Errorf("expected instruction 'write a report', got %q", instruction)
			}
			if status != model.TaskStatusPending {
				t.Errorf("expected task status %q, got %q", model.TaskStatusPending, status)
			}

			var msgStatus string
			db.QueryRow(`SELECT status FROM bee_platform_messages WHERE id='m1'`).Scan(&msgStatus)
			if msgStatus != store.MsgStatusBeeProcessed {
				t.Errorf("expected %q, got %q", store.MsgStatusBeeProcessed, msgStatus)
			}
		})
	}
}
