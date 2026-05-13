package bridge

import (
	"context"
	"errors"
	"testing"
	"time"

	ai "github.com/theopenbee/openbee/internal/ai"
)

func newTestBridge(t *testing.T, eng *fakeEngine, selector EngineSelector) *bridgeImpl {
	t.Helper()
	if selector == nil {
		selector = stubSelector{
			worker: func(hint string) string {
				if hint == "" {
					return ai.EngineClaude
				}
				return hint
			},
			bee: func() string { return ai.EngineClaude },
		}
	}
	return &bridgeImpl{
		engines: map[string]ai.EngineAdapter{ai.EngineClaude: eng},
		deps: Deps{
			TokenIssuer:     stubTokens{},
			EnvResolver:     stubEnv{},
			EngineSelector:  selector,
			ArgsResolver:    stubArgs{},
			LogPathProvider: stubLogPath{path: "/tmp/log"},
		},
	}
}

type stubTokens struct{}

func (stubTokens) WorkerToken(_ string, _ []string) (string, error) { return "wtok", nil }
func (stubTokens) BeeToken() (string, error)                        { return "btok", nil }

type stubEnv struct{}

func (stubEnv) WorkerEnv(_ string) ([]string, error) { return []string{"K=V"}, nil }
func (stubEnv) BeeEnv() ([]string, error)            { return []string{"K=B"}, nil }

type stubArgs struct{}

func (stubArgs) ForWorker(context.Context, string, string) string { return "--worker-flag" }
func (stubArgs) ForBee(context.Context, string) string            { return "--bee-flag" }

type stubLogPath struct {
	path string
	err  error
}

func (s stubLogPath) PrepareForWorker(string, time.Time) (string, error) { return s.path, s.err }

func TestRunWorkerHappyPath_CompletedOutcome(t *testing.T) {
	ch := make(chan ai.Output, 1)
	eng := &fakeEngine{pid: 4321, scriptedCh: ch, extract: func() string { return "final" }}
	b := newTestBridge(t, eng, nil)

	h, err := b.RunWorker(context.Background(), WorkerRunRequest{
		WorkerID: "w1", ExecutionID: "e1", StartedAt: time.Now(),
		WorkDir: "/wd", Prompt: "p", SessionID: "s1",
	})
	if err != nil {
		t.Fatalf("RunWorker: %v", err)
	}
	if h.PID() != 4321 {
		t.Fatalf("PID: got %d, want 4321", h.PID())
	}
	if h.EngineName() != ai.EngineClaude {
		t.Fatalf("EngineName: got %q", h.EngineName())
	}

	ch <- ai.Output{Type: ai.OutputDone}
	close(ch)

	got, err := h.Wait(context.Background())
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if got.Status != StatusCompleted || got.Result != "final" {
		t.Fatalf("outcome: %+v", got)
	}

	// Invariant 1: Wait is idempotent.
	got2, _ := h.Wait(context.Background())
	if got2 != got {
		t.Fatalf("second Wait returned different outcome: %+v vs %+v", got2, got)
	}
}

func TestRunWorkerFailedOutcome(t *testing.T) {
	ch := make(chan ai.Output, 1)
	eng := &fakeEngine{pid: 1, scriptedCh: ch, extract: func() string { return "" }}
	b := newTestBridge(t, eng, nil)
	h, _ := b.RunWorker(context.Background(), WorkerRunRequest{})
	ch <- ai.Output{Type: ai.OutputError, Content: "boom"}
	close(ch)
	got, _ := h.Wait(context.Background())
	if got.Status != StatusFailed || got.Result != "boom" {
		t.Fatalf("outcome: %+v", got)
	}
}

func TestRunWorkerAbandonedOutcome(t *testing.T) {
	ch := make(chan ai.Output)
	eng := &fakeEngine{pid: 1, scriptedCh: ch, extract: func() string { return "" }}
	b := newTestBridge(t, eng, nil)
	h, _ := b.RunWorker(context.Background(), WorkerRunRequest{})
	close(ch) // no Done/Error signal
	got, _ := h.Wait(context.Background())
	if got.Status != StatusAbandoned {
		t.Fatalf("status: got %v, want StatusAbandoned", got.Status)
	}
	if got.Result != "process exited without completion signal" {
		t.Fatalf("placeholder result missing: %q", got.Result)
	}
}

func TestRunWorkerExtractCalledOncePerTerminal(t *testing.T) {
	ch := make(chan ai.Output, 1)
	var calls int32
	eng := &fakeEngine{pid: 1, scriptedCh: ch, extract: func() string {
		calls++
		return "x"
	}}
	b := newTestBridge(t, eng, nil)
	h, _ := b.RunWorker(context.Background(), WorkerRunRequest{})
	ch <- ai.Output{Type: ai.OutputDone}
	close(ch)
	for i := 0; i < 5; i++ {
		_, _ = h.Wait(context.Background())
	}
	if calls != 1 {
		t.Fatalf("ExtractResult call count: got %d, want 1", calls)
	}
}

func TestRunWorkerStopIsIdempotentAndWaitStillReturns(t *testing.T) {
	ch := make(chan ai.Output, 1)
	eng := &fakeEngine{pid: 1, scriptedCh: ch, extract: func() string { return "x" }}
	b := newTestBridge(t, eng, nil)
	h, _ := b.RunWorker(context.Background(), WorkerRunRequest{})
	if err := h.Stop(); err != nil {
		t.Fatalf("first Stop: %v", err)
	}
	if err := h.Stop(); err != nil {
		t.Fatalf("second Stop: %v", err)
	}
	ch <- ai.Output{Type: ai.OutputDone}
	close(ch)
	if _, err := h.Wait(context.Background()); err != nil {
		t.Fatalf("Wait after Stop: %v", err)
	}
}

func TestRunWorkerWaitContextCancelled(t *testing.T) {
	ch := make(chan ai.Output)
	eng := &fakeEngine{pid: 1, scriptedCh: ch, extract: func() string { return "" }}
	b := newTestBridge(t, eng, nil)
	h, _ := b.RunWorker(context.Background(), WorkerRunRequest{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := h.Wait(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestRunWorkerPassesAssembledOptionsToEngine(t *testing.T) {
	ch := make(chan ai.Output, 1)
	var captured ai.RunOptions
	eng := &fakeEngine{pid: 1, scriptedCh: ch, extract: func() string { return "x" }, onRun: func(o ai.RunOptions) { captured = o }}
	b := newTestBridge(t, eng, nil)
	_, err := b.RunWorker(context.Background(), WorkerRunRequest{
		WorkerID: "w1", PermissionScopes: []string{"a", "b"},
		SessionID: "sess", Resume: true,
	})
	if err != nil {
		t.Fatalf("RunWorker: %v", err)
	}
	if captured.APIKey != "wtok" {
		t.Fatalf("APIKey: got %q, want wtok", captured.APIKey)
	}
	if len(captured.ExtraEnv) != 1 || captured.ExtraEnv[0] != "K=V" {
		t.Fatalf("ExtraEnv: %v", captured.ExtraEnv)
	}
	if captured.ExtraArgs != "--worker-flag" {
		t.Fatalf("ExtraArgs: %q", captured.ExtraArgs)
	}
	if captured.SessionID != "sess" || !captured.Resume {
		t.Fatalf("session/resume not propagated: %+v", captured)
	}
	close(ch)
}

func TestRunWorkerStartupFailures(t *testing.T) {
	// Unknown engine.
	b := newTestBridge(t, &fakeEngine{scriptedCh: make(chan ai.Output)}, stubSelector{
		worker: func(string) string { return "nonexistent" },
	})
	if _, err := b.RunWorker(context.Background(), WorkerRunRequest{}); err == nil {
		t.Fatalf("expected error for unknown engine")
	}

	// Engine.Run propagates failure.
	failing := &fakeEngine{runErr: errors.New("engine boom")}
	b2 := newTestBridge(t, failing, nil)
	if _, err := b2.RunWorker(context.Background(), WorkerRunRequest{}); err == nil {
		t.Fatalf("expected error from engine.Run")
	}
}

func TestRunBeeHappyPath(t *testing.T) {
	ch := make(chan ai.Output, 1)
	var captured ai.RunOptions
	eng := &fakeEngine{pid: 9, scriptedCh: ch, extract: func() string { return "bee-final" }, onRun: func(o ai.RunOptions) { captured = o }}
	b := newTestBridge(t, eng, nil)
	h, err := b.RunBee(context.Background(), BeeRunRequest{WorkDir: "/wd", Prompt: "p", SessionID: "bs", LogPath: "/tmp/bee.log"})
	if err != nil {
		t.Fatalf("RunBee: %v", err)
	}
	if captured.APIKey != "btok" || captured.ExtraEnv[0] != "K=B" || captured.ExtraArgs != "--bee-flag" {
		t.Fatalf("bee options wrong: %+v", captured)
	}
	ch <- ai.Output{Type: ai.OutputDone}
	close(ch)
	got, _ := h.Wait(context.Background())
	if got.Result != "bee-final" {
		t.Fatalf("result: %q", got.Result)
	}
}
