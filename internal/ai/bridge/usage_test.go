package bridge

import (
	"context"
	"errors"
	"reflect"
	"testing"

	ai "github.com/theopenbee/openbee/internal/ai"
)

type usageFakeEngine struct {
	usages []ai.TokenUsage
	err    error
}

func (f *usageFakeEngine) Run(context.Context, string, string, ai.RunOptions, string) (ai.RunResult, error) {
	return ai.RunResult{}, nil
}
func (f *usageFakeEngine) CollectTokenUsage(context.Context, string) ([]ai.TokenUsage, error) {
	return f.usages, f.err
}

func TestCollectUsageTranslatesValues(t *testing.T) {
	fe := &usageFakeEngine{usages: []ai.TokenUsage{{Model: "m", InputTokens: 1, OutputTokens: 2, CacheCreationTokens: 3, CacheReadTokens: 4}}}
	b := &bridgeImpl{engines: map[string]ai.EngineAdapter{ai.EngineClaude: fe}}
	got, err := b.CollectUsage(context.Background(), ai.EngineClaude, "sid")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []Usage{{Model: "m", InputTokens: 1, OutputTokens: 2, CacheCreationTokens: 3, CacheReadTokens: 4}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("CollectUsage: got %+v, want %+v", got, want)
	}
}

func TestCollectUsageTranslatesNotFound(t *testing.T) {
	fe := &usageFakeEngine{err: ai.ErrSessionDataNotFound}
	b := &bridgeImpl{engines: map[string]ai.EngineAdapter{ai.EngineClaude: fe}}
	_, err := b.CollectUsage(context.Background(), ai.EngineClaude, "sid")
	if !errors.Is(err, ErrSessionDataNotFound) {
		t.Fatalf("expected ErrSessionDataNotFound, got %v", err)
	}
}

func TestCollectUsageUnknownEngine(t *testing.T) {
	b := &bridgeImpl{engines: map[string]ai.EngineAdapter{}}
	_, err := b.CollectUsage(context.Background(), ai.EngineClaude, "sid")
	if !errors.Is(err, ErrEngineNotEnabled) {
		t.Fatalf("expected ErrEngineNotEnabled, got %v", err)
	}
}
