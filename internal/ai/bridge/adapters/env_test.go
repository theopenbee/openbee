package adapters

import (
	"reflect"
	"testing"

	"github.com/theopenbee/openbee/internal/domain/env"
)

func TestEnvResolverDelegates(t *testing.T) {
	svc := &fakeEnvService{
		worker: map[string][]string{"w1": {"A=1"}},
		bee:    []string{"B=2"},
	}
	r := NewEnvResolver(svc)

	got, err := r.WorkerEnv("w1")
	if err != nil || !reflect.DeepEqual(got, []string{"A=1"}) {
		t.Fatalf("WorkerEnv: got %v, err %v", got, err)
	}
	got, err = r.BeeEnv()
	if err != nil || !reflect.DeepEqual(got, []string{"B=2"}) {
		t.Fatalf("BeeEnv: got %v, err %v", got, err)
	}
}

// fakeEnvService satisfies the subset of *env.Service used by the adapter.
type fakeEnvService struct {
	worker map[string][]string
	bee    []string
}

func (f *fakeEnvService) ResolveWorkerEnv(id string) ([]string, error) { return f.worker[id], nil }
func (f *fakeEnvService) ResolveBeeEnv(string) ([]string, error)       { return f.bee, nil }

// Compile-time check: ensure the adapter accepts our fake via the interface
// declared in env.go.
var _ envService = (*fakeEnvService)(nil)
var _ envService = (*env.Service)(nil)
