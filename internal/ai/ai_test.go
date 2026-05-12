package ai_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	ai "github.com/theopenbee/openbee/internal/ai"
	"github.com/theopenbee/openbee/internal/domain/enginecfg"
)

// stubEngine is a minimal EngineAdapter for testing.
type stubEngine struct {
	name     string
	prepared []string // workDirs seen
}

func (s *stubEngine) Prepare(workDir string, _ ai.PrepareOptions) error {
	s.prepared = append(s.prepared, workDir)
	return nil
}
func (s *stubEngine) Run(_ context.Context, _, _ string, _ ai.RunOptions, _ string) (ai.RunResult, error) {
	name := s.name
	return ai.RunResult{
		ExtractResult: func() string { return name + "-result" },
	}, errors.New(s.name + " run called")
}
func (s *stubEngine) CollectTokenUsage(_ context.Context, _ string) ([]ai.TokenUsage, error) {
	return nil, ai.ErrSessionDataNotFound
}

func TestNewRunResult_MemoizesExtract(t *testing.T) {
	calls := 0
	res, err := ai.NewRunResult(nil, nil, nil, func() string {
		calls++
		return "value"
	})
	if err != nil {
		t.Fatalf("NewRunResult: %v", err)
	}
	for i := range 3 {
		if got := res.ExtractResult(); got != "value" {
			t.Fatalf("call %d: got %q want %q", i, got, "value")
		}
	}
	if calls != 1 {
		t.Errorf("extract should run once; got %d calls", calls)
	}
}

func TestNewRunResult_PropagatesError(t *testing.T) {
	wantErr := errors.New("boom")
	_, err := ai.NewRunResult(nil, nil, wantErr, func() string { return "" })
	if !errors.Is(err, wantErr) {
		t.Errorf("want %v, got %v", wantErr, err)
	}
}

func TestDynamicAdapter_PrepareCallsAll(t *testing.T) {
	a := &stubEngine{name: "a"}
	b := &stubEngine{name: "b"}
	cfg := enginecfg.NewStore("a")
	d := ai.NewDynamicAdapter(map[string]ai.EngineAdapter{"a": a, "b": b}, cfg)
	if err := d.Prepare("/work", ai.PrepareOptions{}); err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if len(a.prepared) != 1 || len(b.prepared) != 1 {
		t.Errorf("expected each engine prepared once; a=%d b=%d", len(a.prepared), len(b.prepared))
	}
}

func TestDynamicAdapter_RunRoutesToCurrentEngine(t *testing.T) {
	cfg := enginecfg.NewStore("a")
	a := &stubEngine{name: "a"}
	b := &stubEngine{name: "b"}
	d := ai.NewDynamicAdapter(map[string]ai.EngineAdapter{"a": a, "b": b}, cfg)

	_, err := d.Run(context.Background(), "/w", "prompt", ai.RunOptions{}, "/log")
	if err == nil || err.Error() != "a run called" {
		t.Errorf("expected 'a run called', got %v", err)
	}

	cfg.Set("b")
	_, err = d.Run(context.Background(), "/w", "prompt", ai.RunOptions{}, "/log")
	if err == nil || err.Error() != "b run called" {
		t.Errorf("expected 'b run called', got %v", err)
	}
}

func TestDynamicAdapter_RunBindsExtractResultToEngine(t *testing.T) {
	cfg := enginecfg.NewStore("a")
	a := &stubEngine{name: "a"}
	b := &stubEngine{name: "b"}
	d := ai.NewDynamicAdapter(map[string]ai.EngineAdapter{"a": a, "b": b}, cfg)

	res, _ := d.Run(context.Background(), "/w", "prompt", ai.RunOptions{}, "/log")

	// Simulate /engine switch mid-execution.
	cfg.Set("b")

	if got := res.ExtractResult(); got != "a-result" {
		t.Errorf("expected Run-time engine 'a' extractor; got %s", got)
	}
}

func TestDynamicAdapter_RunUnknownEngine(t *testing.T) {
	cfg := enginecfg.NewStore("missing")
	d := ai.NewDynamicAdapter(map[string]ai.EngineAdapter{"a": &stubEngine{name: "a"}}, cfg)
	_, err := d.Run(context.Background(), "/w", "p", ai.RunOptions{}, "/log")
	if err == nil {
		t.Error("expected error for unknown engine")
	}
}

func TestParseEngineArgs_PreservesOrderAndQuotedValues(t *testing.T) {
	raw := map[string]string{
		"claude": `--model claude-sonnet-4-5 --append-system-prompt "be terse" --verbose`,
	}
	got, err := ai.ParseEngineArgs(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []string{"--model", "claude-sonnet-4-5", "--append-system-prompt", "be terse", "--verbose"}
	if !slices.Equal(got["claude"], want) {
		t.Fatalf("got %v, want %v", got["claude"], want)
	}
}

func TestParseEngineArgs_PreservesDuplicateFlags(t *testing.T) {
	raw := map[string]string{
		"codex": `--include src --include test`,
	}
	got, err := ai.ParseEngineArgs(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []string{"--include", "src", "--include", "test"}
	if !slices.Equal(got["codex"], want) {
		t.Fatalf("got %v, want %v", got["codex"], want)
	}
}

func TestParseEngineArgs_PreservesEmptyQuotedValue(t *testing.T) {
	raw := map[string]string{
		"claude": `--append-system-prompt "" --verbose`,
	}
	got, err := ai.ParseEngineArgs(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []string{"--append-system-prompt", "", "--verbose"}
	if !slices.Equal(got["claude"], want) {
		t.Fatalf("got %v, want %v", got["claude"], want)
	}
}

func TestParseEngineArgs_UnterminatedQuote(t *testing.T) {
	_, err := ai.ParseEngineArgs(map[string]string{
		"claude": `--model "unterminated`,
	})
	if err == nil {
		t.Fatal("expected parse error, got nil")
	}
}

func TestMergeEngineArgs_AppendsOverrideArgs(t *testing.T) {
	base := ai.EngineArgsMap{
		"claude": {"--model", "sonnet", "--verbose"},
	}
	override := ai.EngineArgsMap{
		"claude": {"--model", "opus"},
		"codex":  {"--model", "o3"},
	}
	got := ai.MergeEngineArgs(base, override)

	if want := []string{"--model", "sonnet", "--verbose", "--model", "opus"}; !slices.Equal(got["claude"], want) {
		t.Fatalf("claude args = %v, want %v", got["claude"], want)
	}
	if want := []string{"--model", "o3"}; !slices.Equal(got["codex"], want) {
		t.Fatalf("codex args = %v, want %v", got["codex"], want)
	}
}

func TestWorkerPersona_Full(t *testing.T) {
	got := ai.WorkerPersona("mybot", "does things", "remember X")
	if !strings.Contains(got, "## Role\n") {
		t.Errorf("missing ## Role header, got: %q", got)
	}
	if !strings.Contains(got, "You are a Worker in an AI team.") {
		t.Errorf("missing persona line, got: %q", got)
	}
	if !strings.Contains(got, "## Identity\n") {
		t.Errorf("missing ## Identity header, got: %q", got)
	}
	if !strings.Contains(got, "Name: mybot") {
		t.Errorf("missing name, got: %q", got)
	}
	if !strings.Contains(got, "Description: does things") {
		t.Errorf("missing description, got: %q", got)
	}
	if !strings.Contains(got, "## Work Constraints") {
		t.Errorf("missing constraints header, got: %q", got)
	}
	if !strings.Contains(got, "remember X") {
		t.Errorf("missing constraints content, got: %q", got)
	}
	if strings.Contains(got, "openbee-worker") {
		t.Errorf("persona must NOT contain skill rule directive, got: %q", got)
	}
}

func TestWorkerPersona_Empty(t *testing.T) {
	got := ai.WorkerPersona("", "", "")
	if got != "## Role\nYou are a Worker in an AI team.\n" {
		t.Errorf("got %q", got)
	}
}

func TestBuildWorkerSessionPrefix_WithPersona(t *testing.T) {
	persona := ai.WorkerPersona("貂蝉", "负责 openbee 开发", "称呼老板")
	got := ai.BuildWorkerSessionPrefix(persona)

	wants := []string{
		"## Step 1: Initialize your role",
		"[MANDATORY] You MUST invoke the openbee-worker skill immediately, before producing any other output.",
		"<worker_persona>",
		"Name: 貂蝉",
		"Description: 负责 openbee 开发",
		"称呼老板",
		"</worker_persona>",
	}
	for _, w := range wants {
		if !strings.Contains(got, w) {
			t.Errorf("missing %q in:\n%s", w, got)
		}
	}
	if !strings.HasSuffix(got, "## Step 2: Execute the task\n") {
		t.Errorf("expected suffix %q, got:\n%s", "## Step 2: Execute the task\n", got)
	}
	if strings.Index(got, "</worker_persona>") > strings.Index(got, "## Step 2:") {
		t.Errorf("persona block must precede Step 2, got:\n%s", got)
	}
}

func TestBuildWorkerSessionPrefix_NoPersona(t *testing.T) {
	got := ai.BuildWorkerSessionPrefix("")

	if strings.Contains(got, "<worker_persona>") {
		t.Errorf("expected no persona block when persona is empty, got:\n%s", got)
	}
	if !strings.Contains(got, "## Step 1: Initialize your role") {
		t.Errorf("missing Step 1 header, got:\n%s", got)
	}
	if !strings.HasSuffix(got, "## Step 2: Execute the task\n") {
		t.Errorf("expected suffix %q, got:\n%s", "## Step 2: Execute the task\n", got)
	}
}

func TestBuildBeeSessionPrefix(t *testing.T) {
	got := ai.BuildBeeSessionPrefix()

	if !strings.Contains(got, "openbee-bee") {
		t.Errorf("expected bee skill name, got:\n%s", got)
	}
	if strings.Contains(got, "<worker_persona>") {
		t.Errorf("bee prefix must not include persona, got:\n%s", got)
	}
	if !strings.HasSuffix(got, "## Step 2: Handle the messages below\n") {
		t.Errorf("expected suffix %q, got:\n%s", "## Step 2: Handle the messages below\n", got)
	}
}

// stubAdapter is a no-op EngineAdapter for registry tests.
type stubAdapter struct{}

func (s *stubAdapter) Prepare(_ string, _ ai.PrepareOptions) error {
	return nil
}
func (s *stubAdapter) Run(_ context.Context, _, _ string, _ ai.RunOptions, _ string) (ai.RunResult, error) {
	return ai.RunResult{ExtractResult: func() string { return "" }}, nil
}
func (s *stubAdapter) CollectTokenUsage(_ context.Context, _ string) ([]ai.TokenUsage, error) {
	return nil, ai.ErrSessionDataNotFound
}

func TestRegistry_NewReturnsRegisteredEngine(t *testing.T) {
	r := ai.NewRegistry()
	r.Register("stub", func(_ ai.EngineConfig) (ai.EngineAdapter, error) {
		return &stubAdapter{}, nil
	})
	eng, err := r.New("stub", ai.EngineConfig{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if eng == nil {
		t.Error("expected non-nil adapter")
	}
}

func TestRegistry_NewUnknownEngineReturnsError(t *testing.T) {
	r := ai.NewRegistry()
	_, err := r.New("unknown", ai.EngineConfig{})
	if err == nil {
		t.Fatal("expected error for unknown engine")
	}
	if !errors.Is(err, ai.ErrUnknownEngine) {
		t.Errorf("expected ErrUnknownEngine, got: %v", err)
	}
}

func TestRegistry_NewCallsFactory(t *testing.T) {
	r := ai.NewRegistry()
	called := false
	r.Register("called", func(cfg ai.EngineConfig) (ai.EngineAdapter, error) {
		called = true
		return &stubAdapter{}, nil
	})
	r.New("called", ai.EngineConfig{})
	if !called {
		t.Error("factory was not called")
	}
}
