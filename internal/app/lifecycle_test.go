package app

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/theopenbee/openbee/internal/infra/store"
)

// stubServer stands in for *routes.Server so the lifecycle can be exercised
// without building the full router. Run blocks until Shutdown is called,
// mirroring http.Server semantics.
type stubServer struct {
	started  chan struct{}
	stopped  chan struct{}
	stopOnce sync.Once
	runErr   error
}

func newStubServer(runErr error) *stubServer {
	return &stubServer{
		started: make(chan struct{}),
		stopped: make(chan struct{}),
		runErr:  runErr,
	}
}

func (s *stubServer) Run(string) error {
	close(s.started)
	if s.runErr != nil {
		return s.runErr
	}
	<-s.stopped
	return nil
}

func (s *stubServer) Shutdown(context.Context) error {
	s.stopOnce.Do(func() { close(s.stopped) })
	return nil
}

func (s *stubServer) waitStarted(t *testing.T) {
	t.Helper()
	select {
	case <-s.started:
	case <-time.After(5 * time.Second):
		t.Fatal("server never started")
	}
}

func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := store.InitDB(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("init test database: %v", err)
	}
	return db
}

// runInBackground starts a.run and returns a channel carrying its result.
func runInBackground(a *App, ctx context.Context) <-chan error {
	done := make(chan error, 1)
	go func() { done <- a.run(ctx) }()
	return done
}

func awaitRun(t *testing.T, done <-chan error) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(10 * time.Second):
		t.Fatal("run did not return after shutdown")
		return nil
	}
}

// The regression this whole change exists for: the database must stay open
// until every runner has returned. The probe deliberately lingers after
// cancellation and then touches the database — a premature Close surfaces
// here as "sql: database is closed".
func TestRunClosesDatabaseAfterRunnersDrain(t *testing.T) {
	db := newTestDB(t)
	var probeErr atomic.Value
	// probeDone is what makes this a real regression test: without it, run()
	// returning early would leave probeErr unset and the assertions would pass
	// vacuously.
	var probeDone atomic.Bool

	probe := runner{name: "db-probe", run: func(ctx context.Context) {
		<-ctx.Done()
		time.Sleep(100 * time.Millisecond)
		var n int
		if err := db.QueryRow("SELECT 1").Scan(&n); err != nil {
			probeErr.Store(err)
		}
		probeDone.Store(true)
	}}

	srv := newStubServer(nil)
	a := &App{
		db:              db,
		server:          srv,
		runners:         []runner{probe},
		addr:            "127.0.0.1:0",
		recoverInflight: func(context.Context) {},
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := runInBackground(a, ctx)
	srv.waitStarted(t)
	cancel()

	if err := awaitRun(t, done); err != nil {
		t.Fatalf("run returned an error: %v", err)
	}
	if !probeDone.Load() {
		t.Fatal("run returned before the runner finished; the database was closed mid-flight")
	}
	if err, ok := probeErr.Load().(error); ok && err != nil {
		t.Fatalf("database was closed before runners drained: %v", err)
	}
}

func TestRunStartsEveryRunnerAndWaitsForAll(t *testing.T) {
	db := newTestDB(t)

	const count = 5
	var started, finished atomic.Int32
	runners := make([]runner, 0, count)
	for i := range count {
		runners = append(runners, runner{
			name: "r",
			run: func(ctx context.Context) {
				started.Add(1)
				<-ctx.Done()
				// Stagger the exits so a missing Wait shows up reliably.
				time.Sleep(time.Duration(i+1) * 20 * time.Millisecond)
				finished.Add(1)
			},
		})
	}

	srv := newStubServer(nil)
	a := &App{
		db:              db,
		server:          srv,
		runners:         runners,
		addr:            "127.0.0.1:0",
		recoverInflight: func(context.Context) {},
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := runInBackground(a, ctx)
	srv.waitStarted(t)
	cancel()

	if err := awaitRun(t, done); err != nil {
		t.Fatalf("run returned an error: %v", err)
	}
	if got := started.Load(); got != count {
		t.Errorf("started %d runners, want %d", got, count)
	}
	if got := finished.Load(); got != count {
		t.Errorf("run returned with only %d of %d runners drained", got, count)
	}
}

// A server failure must tear the whole app down and surface as a non-nil
// error, so `openbee server` exits non-zero.
func TestRunReturnsServerErrorAndDrainsRunners(t *testing.T) {
	db := newTestDB(t)
	wantErr := errors.New("listen tcp: address already in use")

	var drained atomic.Bool
	probe := runner{name: "probe", run: func(ctx context.Context) {
		<-ctx.Done()
		drained.Store(true)
	}}

	srv := newStubServer(wantErr)
	a := &App{
		db:              db,
		server:          srv,
		runners:         []runner{probe},
		addr:            "127.0.0.1:0",
		recoverInflight: func(context.Context) {},
	}

	err := awaitRun(t, runInBackground(a, context.Background()))
	if !errors.Is(err, wantErr) {
		t.Fatalf("run error = %v, want it to wrap %v", err, wantErr)
	}
	if !drained.Load() {
		t.Error("runners were not drained after the server failed")
	}
}

// Recovery must complete before any runner observes the world.
func TestRunRecoversBeforeStartingRunners(t *testing.T) {
	db := newTestDB(t)

	var recovered, ranBeforeRecovery atomic.Bool
	probe := runner{name: "probe", run: func(ctx context.Context) {
		if !recovered.Load() {
			ranBeforeRecovery.Store(true)
		}
		<-ctx.Done()
	}}

	srv := newStubServer(nil)
	a := &App{
		db:              db,
		server:          srv,
		runners:         []runner{probe},
		addr:            "127.0.0.1:0",
		recoverInflight: func(context.Context) {
			time.Sleep(50 * time.Millisecond)
			recovered.Store(true)
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := runInBackground(a, ctx)
	srv.waitStarted(t)
	cancel()

	if err := awaitRun(t, done); err != nil {
		t.Fatalf("run returned an error: %v", err)
	}
	if !recovered.Load() {
		t.Fatal("recovery never ran")
	}
	if ranBeforeRecovery.Load() {
		t.Error("a runner started before startup recovery completed")
	}
}
