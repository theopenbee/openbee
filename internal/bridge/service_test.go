package bridge_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	ai "github.com/theopenbee/openbee/internal/ai"
	"github.com/theopenbee/openbee/internal/bridge"
	"github.com/theopenbee/openbee/internal/domain/enginecfg"
)

type fakeAdapter struct {
	prepareRole ai.Role
	runOpts     ai.RunOptions
	runPrompt   string
	runLogPath  string
	outputs     []ai.Output
	usage       []ai.TokenUsage
	usageErr    error
}

func (f *fakeAdapter) Prepare(_ string, opts ai.PrepareOptions) error {
	f.prepareRole = opts.Role
	return nil
}

func (f *fakeAdapter) Run(_ context.Context, _ string, prompt string, opts ai.RunOptions, logPath string) (ai.RunResult, error) {
	f.runPrompt = prompt
	f.runOpts = opts
	f.runLogPath = logPath
	ch := make(chan ai.Output, len(f.outputs))
	for _, out := range f.outputs {
		ch <- out
	}
	close(ch)
	return ai.RunResult{Process: fakeProcess{pid: 42}, Output: ch, ExtractResult: func(string) string { return "result" }}, nil
}

func (f *fakeAdapter) CollectTokenUsage(_ context.Context, _ string) ([]ai.TokenUsage, error) {
	if f.usageErr != nil {
		return nil, f.usageErr
	}
	return f.usage, nil
}

type fakeProcess struct{ pid int }

func (p fakeProcess) PID() int    { return p.pid }
func (p fakeProcess) Stop() error { return nil }

type fakeEnv struct{}

func (fakeEnv) ResolveBeeEnv(string) ([]string, error) {
	return []string{"BEE_ENV=1"}, nil
}

func (fakeEnv) ResolveWorkerEnv(string) ([]string, error) {
	return []string{"WORKER_ENV=1"}, nil
}

func TestAllEnginesMatchesCanonicalOrder(t *testing.T) {
	got := bridge.AllEngines()
	want := []string{bridge.EngineClaude, bridge.EngineCodex, bridge.EnginePi, bridge.EngineKimi}
	if len(got) != len(want) {
		t.Fatalf("len=%d want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("engine[%d]=%q want %q", i, got[i], want[i])
		}
	}
}

func TestWorkerPersonaAndPrefix(t *testing.T) {
	persona := bridge.WorkerPersona{
		Name:        "xiao-qiao",
		Description: "openbee development",
		Constraints: "call the user boss",
	}
	prefix := bridge.BuildWorkerSessionPrefix(persona)
	for _, want := range []string{
		"## Step 1: Initialize your role",
		"openbee-worker",
		"<worker_persona>",
		"Name: xiao-qiao",
		"Description: openbee development",
		"call the user boss",
		"## Step 2: Execute the task",
	} {
		if !strings.Contains(prefix, want) {
			t.Fatalf("prefix missing %q:\n%s", want, prefix)
		}
	}
}

func TestParseEngineArgs(t *testing.T) {
	got, err := bridge.ParseEngineArgs(map[string]string{
		bridge.EngineClaude: `--model "sonnet 4" --verbose`,
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"--model", "sonnet 4", "--verbose"}
	args := got[bridge.EngineClaude]
	if len(args) != len(want) {
		t.Fatalf("len=%d want %d: %v", len(args), len(want), args)
	}
	for i := range want {
		if args[i] != want[i] {
			t.Fatalf("arg[%d]=%q want %q", i, args[i], want[i])
		}
	}
}

func TestServiceResolveEngineFallback(t *testing.T) {
	svc := bridge.NewService(bridge.ServiceOptions{
		Engines:   bridge.EngineSetForTest(map[string]ai.EngineAdapter{bridge.EngineClaude: &fakeAdapter{}}),
		EngineCfg: enginecfg.NewStore(bridge.EngineClaude),
	})
	got, err := svc.ResolveEngine("unknown")
	if err != nil {
		t.Fatal(err)
	}
	if got != bridge.EngineClaude {
		t.Fatalf("got %q want %q", got, bridge.EngineClaude)
	}
}

func TestRunWorkerMapsRequestAndLifecycle(t *testing.T) {
	adapter := &fakeAdapter{outputs: []ai.Output{{Type: ai.OutputDone}}}
	svc := bridge.NewService(bridge.ServiceOptions{
		Engines:     bridge.EngineSetForTest(map[string]ai.EngineAdapter{bridge.EngineClaude: adapter}),
		EngineCfg:   enginecfg.NewStore(bridge.EngineClaude),
		TokenSecret: "0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f20",
		TokenTTL:    time.Minute,
		Env:         fakeEnv{},
	})
	handle, err := svc.RunWorker(context.Background(), bridge.WorkerRunRequest{
		WorkerID:         "worker-1",
		WorkDir:          "/tmp/worker",
		PermissionScopes: []string{"messages:write"},
		Prompt:           "do work",
		SessionID:        "session-1",
		LogPath:          "/tmp/log",
	})
	if err != nil {
		t.Fatal(err)
	}
	if handle.Engine != bridge.EngineClaude {
		t.Fatalf("engine=%q", handle.Engine)
	}
	if adapter.runPrompt != "do work" || adapter.runLogPath != "/tmp/log" {
		t.Fatalf("run mapping failed prompt=%q log=%q", adapter.runPrompt, adapter.runLogPath)
	}
	if adapter.runOpts.SessionID != "session-1" || adapter.runOpts.APIKey == "" {
		t.Fatalf("opts not populated: %+v", adapter.runOpts)
	}
	event := <-handle.Events
	if event.Type != bridge.LifecycleDone {
		t.Fatalf("event=%+v", event)
	}
}

func TestCollectTokenUsageMapsEngineName(t *testing.T) {
	adapter := &fakeAdapter{usage: []ai.TokenUsage{{Model: "sonnet", InputTokens: 10}}}
	svc := bridge.NewService(bridge.ServiceOptions{
		Engines:   bridge.EngineSetForTest(map[string]ai.EngineAdapter{bridge.EngineClaude: adapter}),
		EngineCfg: enginecfg.NewStore(bridge.EngineClaude),
	})
	result, err := svc.CollectTokenUsage(context.Background(), "session-1", bridge.EngineClaude)
	if err != nil {
		t.Fatal(err)
	}
	if result.Engine != bridge.EngineClaude {
		t.Fatalf("engine=%q want %q", result.Engine, bridge.EngineClaude)
	}
	if len(result.Usages) != 1 || result.Usages[0].Model != "sonnet" {
		t.Fatalf("usages=%+v", result.Usages)
	}
}

func TestCollectTokenUsageMapsNotFound(t *testing.T) {
	adapter := &fakeAdapter{usageErr: ai.ErrSessionDataNotFound}
	svc := bridge.NewService(bridge.ServiceOptions{
		Engines:   bridge.EngineSetForTest(map[string]ai.EngineAdapter{bridge.EngineClaude: adapter}),
		EngineCfg: enginecfg.NewStore(bridge.EngineClaude),
	})
	_, err := svc.CollectTokenUsage(context.Background(), "missing", bridge.EngineClaude)
	if !errors.Is(err, bridge.ErrSessionDataNotFound) {
		t.Fatalf("err=%v", err)
	}
}
