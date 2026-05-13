package bridge

import (
	"errors"
	"testing"
)

type stubSelector struct {
	worker func(hint string) string
	bee    func() string
}

func (s stubSelector) ForWorker(h string) string { return s.worker(h) }
func (s stubSelector) ForBee() string            { return s.bee() }

func TestResolveEngineForWorkerDelegates(t *testing.T) {
	b := &bridgeImpl{deps: Deps{EngineSelector: stubSelector{
		worker: func(h string) string {
			if h == "" {
				return "default"
			}
			return h
		},
	}}}
	if got := b.ResolveEngineForWorker("w1", ""); got != "default" {
		t.Fatalf("empty hint: got %q, want default", got)
	}
	if got := b.ResolveEngineForWorker("w1", "claude"); got != "claude" {
		t.Fatalf("hint: got %q, want claude", got)
	}
}

func TestResolveEngineForBeeDelegates(t *testing.T) {
	b := &bridgeImpl{deps: Deps{EngineSelector: stubSelector{
		bee: func() string { return "kimi" },
	}}}
	if got := b.ResolveEngineForBee(); got != "kimi" {
		t.Fatalf("got %q, want kimi", got)
	}
}

func TestNewValidatesConfig(t *testing.T) {
	_, err := New(Config{})
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("expected ErrInvalidConfig, got %v", err)
	}
}
