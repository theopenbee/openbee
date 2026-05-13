package adapters

import (
	"context"
	"testing"

	ai "github.com/theopenbee/openbee/internal/ai"
	"github.com/theopenbee/openbee/internal/infra/model"
)

type fakeSysStore struct{ values map[string]string }

func (f fakeSysStore) Get(_ context.Context, key string) (model.SystemConfig, bool, error) {
	v, ok := f.values[key]
	return model.SystemConfig{Key: key, Value: v}, ok, nil
}

func TestArgsResolverForWorkerMergesGlobalThenWorker(t *testing.T) {
	store := fakeSysStore{values: map[string]string{
		model.SystemConfigKeyEngineArgsGlobal: `{"claude":"--g"}`,
	}}
	r := NewArgsResolver(store)
	got := r.ForWorker(context.Background(), `{"claude":"--w"}`, ai.EngineClaude)
	if got != "--g --w" {
		t.Fatalf("got %q, want %q", got, "--g --w")
	}
}

func TestArgsResolverForBeeMergesGlobalThenBee(t *testing.T) {
	store := fakeSysStore{values: map[string]string{
		model.SystemConfigKeyEngineArgsGlobal: `{"claude":"--g"}`,
		model.SystemConfigKeyEngineArgsBee:    `{"claude":"--b"}`,
	}}
	r := NewArgsResolver(store)
	got := r.ForBee(context.Background(), ai.EngineClaude)
	if got != "--g --b" {
		t.Fatalf("got %q", got)
	}
}

func TestArgsResolverMissingValuesAreEmpty(t *testing.T) {
	r := NewArgsResolver(fakeSysStore{values: map[string]string{}})
	if got := r.ForWorker(context.Background(), "", ai.EngineClaude); got != "" {
		t.Fatalf("got %q, want empty", got)
	}
}
