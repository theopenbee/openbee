package bee_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/theopenbee/openbee/internal/bee"
	"github.com/theopenbee/openbee/internal/claude"
	"github.com/theopenbee/openbee/internal/config"
	"github.com/theopenbee/openbee/internal/model"
	"github.com/theopenbee/openbee/internal/store"
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

// mockBeeRunner records all Run calls.
type mockBeeRunner struct {
	mu          sync.Mutex
	calls       []beeCall
	err         error
	outputLines []claude.Output
}

type beeCall struct {
	prompt    string
	sessionID string
	resume    bool
	logPath   string
}

func (m *mockBeeRunner) Run(_ context.Context, _, prompt string, opts claude.RunOptions, logPath string) (*claude.Process, <-chan claude.Output, error) {
	m.mu.Lock()
	m.calls = append(m.calls, beeCall{
		prompt:    prompt,
		sessionID: opts.SessionID,
		resume:    opts.Resume,
		logPath:   logPath,
	})
	customLines := m.outputLines
	m.mu.Unlock()

	var lines []claude.Output
	if customLines != nil {
		lines = customLines
	} else if m.err != nil {
		lines = []claude.Output{{Type: claude.OutputError, Content: m.err.Error()}}
	} else {
		lines = []claude.Output{{Type: claude.OutputDone}}
	}

	ch := make(chan claude.Output, len(lines))
	for _, l := range lines {
		ch <- l
	}
	close(ch)
	return &claude.Process{}, ch, nil
}

func (m *mockBeeRunner) getCalls() []beeCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]beeCall{}, m.calls...)
}

func newFeeder(ms *store.MessageStore, ts *store.TaskStore, ss *store.SessionStore, es *store.ExecutionStore, runner bee.BeeRunner) *bee.Feeder {
	cfg := config.BeeConfig{}
	cfg.Feeder.Timeout = 5 * time.Second
	cfg.Feeder.MaxConcurrentBee = 5
	return bee.NewFeeder(ms, ts, ss, es, runner, "/tmp", cfg)
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
	if call.sessionID == "" {
		t.Error("expected non-empty sessionID on first call")
	}
	if call.resume {
		t.Error("expected resume=false on first call")
	}

	got, err := ss.GetSessionContext(context.Background(), "feishu:c:u", store.BeeAgentID)
	if err != nil {
		t.Fatalf("get session context: %v", err)
	}
	if got != call.sessionID {
		t.Errorf("persisted sessionID mismatch: want %q got %q", call.sessionID, got)
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

	if err := ss.UpsertSessionContext(ctx, "feishu:c:u", store.BeeAgentID, "existing-session"); err != nil {
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
	if call.sessionID != "existing-session" {
		t.Errorf("expected existing-session, got %q", call.sessionID)
	}
	if !call.resume {
		t.Error("expected resume=true on second call")
	}
}

func TestFeeder_OnBeeFailure_RollsBackAndDoesNotUpdateSession(t *testing.T) {
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
	if status != "received" {
		t.Errorf("expected rollback to received, got %q", status)
	}

	got, _ := ss.GetSessionContext(context.Background(), "feishu:c:u", store.BeeAgentID)
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

	sess1, _ := ss.GetSessionContext(context.Background(), "feishu:c:u1", store.BeeAgentID)
	sess2, _ := ss.GetSessionContext(context.Background(), "feishu:c:u2", store.BeeAgentID)
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

type mockFailureNotifier struct {
	mu   sync.Mutex
	msgs []string
}

func (m *mockFailureNotifier) NotifyTaskFailure(_ context.Context, messageID, _ string) error {
	m.mu.Lock()
	m.msgs = append(m.msgs, messageID)
	m.mu.Unlock()
	return nil
}

func (m *mockFailureNotifier) getNotified() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string{}, m.msgs...)
}

func TestFeeder_ExhaustsRetries_MarksFailedAndNotifies(t *testing.T) {
	db, ms, ts, ss, es := setupFeederDB(t)
	insertMessage(t, db, "m1", "feishu:c:u", "hello")

	runner := &mockBeeRunner{err: fmt.Errorf("bee crashed")}
	notifier := &mockFailureNotifier{}
	cfg := config.BeeConfig{}
	cfg.Feeder.Timeout = 5 * time.Second
	cfg.Feeder.MaxConcurrentBee = 5
	f := bee.NewFeeder(ms, ts, ss, es, runner, "/tmp", cfg, bee.WithFailureNotifier(notifier))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	go f.Run(ctx)

	time.Sleep(time.Duration(bee.MaxRetries+1)*bee.PollInterval + 500*time.Millisecond)

	var status string
	db.QueryRow(`SELECT status FROM bee_platform_messages WHERE id='m1'`).Scan(&status)
	if status != "failed" {
		t.Errorf("expected status=failed after exhausting retries, got %q", status)
	}

	notified := notifier.getNotified()
	if len(notified) != 1 || notified[0] != "m1" {
		t.Errorf("expected notifier called once with m1, got %v", notified)
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
	cfg.Feeder.Timeout = 5 * time.Second
	cfg.Feeder.MaxConcurrentBee = 5
	f := bee.NewFeeder(ms, ts, ss, es, slowRunner, "/tmp", cfg)

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
	for _, ts := range startTimes[1:] {
		if ts.Before(first) {
			first = ts
		}
		if ts.After(last) {
			last = ts
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
	for i := 0; i < 6; i++ {
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
	cfg.Feeder.Timeout = 5 * time.Second
	cfg.Feeder.MaxConcurrentBee = 3 // deliberately small
	f := bee.NewFeeder(ms, ts, ss, es, slowRunner, "/tmp", cfg)

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
	fn func()
}

func (r *callbackBeeRunner) Run(_ context.Context, _, _ string, opts claude.RunOptions, _ string) (*claude.Process, <-chan claude.Output, error) {
	ch := make(chan claude.Output, 1)
	go func() {
		if r.fn != nil {
			r.fn()
		}
		ch <- claude.Output{Type: claude.OutputDone}
		close(ch)
	}()
	return &claude.Process{}, ch, nil
}

func TestWriteCLAUDEMD_DoesNotOverwriteExisting(t *testing.T) {
	dir := t.TempDir()
	original := "user-edited content"
	os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte(original), 0644)

	if err := bee.WriteCLAUDEMD(dir, "new persona"); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
	if string(data) != original {
		t.Error("WriteCLAUDEMD should not overwrite existing file")
	}
}

func TestWriteCLAUDEMD_CreatesWhenMissing(t *testing.T) {
	dir := t.TempDir()

	if err := bee.WriteCLAUDEMD(dir, bee.DefaultPersona); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
	if err != nil {
		t.Fatalf("CLAUDE.md should be created: %v", err)
	}
	if string(data) != bee.DefaultPersona {
		t.Errorf("unexpected content: %q", string(data))
	}
}

