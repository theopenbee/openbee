package adapters

import bridge "github.com/theopenbee/openbee/internal/ai/bridge"

// envService is the subset of *env.Service consumed here, declared locally
// so tests can fake it without depending on env.Service internals.
type envService interface {
	ResolveWorkerEnv(workerID string) ([]string, error)
	ResolveBeeEnv(beeID string) ([]string, error)
}

const defaultBeeID = "default"

type envResolver struct{ svc envService }

func NewEnvResolver(svc envService) bridge.EnvResolver { return envResolver{svc: svc} }

func (e envResolver) WorkerEnv(workerID string) ([]string, error) {
	return e.svc.ResolveWorkerEnv(workerID)
}
func (e envResolver) BeeEnv() ([]string, error) { return e.svc.ResolveBeeEnv(defaultBeeID) }
