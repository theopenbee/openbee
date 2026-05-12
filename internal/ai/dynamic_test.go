package ai_test

import (
	"context"
	"errors"
	"testing"

	"github.com/theopenbee/openbee/internal/ai"
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
