package env

import (
	"crypto/cipher"
	"fmt"
	"sort"
	"sync"

	"github.com/theopenbee/openbee/internal/infra/crypto"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/infra/store"
)

// reservedKey is the env var name that openbee manages internally and must
// never be overridden by user-supplied env configs.
const reservedKey = "OPENBEE_API_KEY"

type Service struct {
	store     *store.EnvConfigStore
	deptStore *store.DepartmentStore
	gcm       cipher.AEAD
}

func NewService(envStore *store.EnvConfigStore, ds *store.DepartmentStore, encKey string) (*Service, error) {
	gcm, err := crypto.NewGCM(encKey)
	if err != nil {
		return nil, fmt.Errorf("init env service: %w", err)
	}
	return &Service{
		store:     envStore,
		deptStore: ds,
		gcm:       gcm,
	}, nil
}

func (s *Service) Create(scope, scopeID, key, plainValue string) (*model.EnvConfig, error) {
	if key == reservedKey {
		return nil, fmt.Errorf("%w: OPENBEE_API_KEY is reserved and cannot be set", ErrValidation)
	}

	switch scope {
	case ScopeGlobal, ScopeBee, ScopeDepartment, ScopeWorker:
	default:
		return nil, fmt.Errorf("%w: invalid scope %q: must be one of global, bee, department, worker", ErrValidation, scope)
	}

	if scope != ScopeGlobal && scopeID == "" {
		return nil, fmt.Errorf("%w: scope_id is required for scope %q", ErrValidation, scope)
	}

	encValue, err := crypto.EncryptGCM(s.gcm, plainValue)
	if err != nil {
		return nil, fmt.Errorf("encrypt env value: %w", err)
	}

	masked := crypto.Mask(plainValue)

	var scopeIDPtr *string
	if scope != ScopeGlobal {
		scopeIDPtr = &scopeID
	}

	cfg := &model.EnvConfig{
		Scope:    scope,
		ScopeID:  scopeIDPtr,
		Key:      key,
		EncValue: encValue,
		Masked:   masked,
	}

	if err := s.store.Create(cfg); err != nil {
		return nil, fmt.Errorf("create env config: %w", err)
	}

	return cfg, nil
}

func (s *Service) getEditable(id string) (*model.EnvConfig, error) {
	existing, err := s.store.Get(id)
	if err != nil {
		return nil, fmt.Errorf("get env config: %w", err)
	}
	if existing == nil {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	if existing.Key == reservedKey {
		return nil, fmt.Errorf("%w: %s is reserved", ErrValidation, reservedKey)
	}
	return existing, nil
}

func (s *Service) UpdateValue(id, plainValue string) error {
	if _, err := s.getEditable(id); err != nil {
		return err
	}

	encValue, err := crypto.EncryptGCM(s.gcm, plainValue)
	if err != nil {
		return fmt.Errorf("encrypt env value: %w", err)
	}

	masked := crypto.Mask(plainValue)

	if err := s.store.Update(id, encValue, masked); err != nil {
		return fmt.Errorf("update env config: %w", err)
	}

	return nil
}

func (s *Service) List(scope string, scopeID *string) ([]*model.EnvConfig, error) {
	return s.store.List(scope, scopeID)
}

func (s *Service) Delete(id string) error {
	if _, err := s.getEditable(id); err != nil {
		return err
	}
	return s.store.Delete(id)
}

// ResolveWorkerEnv returns complete env vars for Worker execution (KEY=VALUE slice).
// Resolution chain: global <- department (last dept alphabetically by ID wins) <- worker
func (s *Service) ResolveWorkerEnv(workerID string) ([]string, error) {
	depts, err := s.deptStore.GetWorkerDepartments(workerID)
	if err != nil {
		return nil, fmt.Errorf("get worker departments: %w", err)
	}

	deptIDs := make([]string, 0, len(depts))
	for _, d := range depts {
		deptIDs = append(deptIDs, d.ID)
	}
	sort.Strings(deptIDs)

	layers, err := fetchParallel(
		func() ([]*model.EnvConfig, error) { return s.store.List(ScopeGlobal, nil) },
		func() ([]*model.EnvConfig, error) { return s.store.ListForDepartments(deptIDs) },
		func() ([]*model.EnvConfig, error) { return s.store.List(ScopeWorker, &workerID) },
	)
	if err != nil {
		return nil, err
	}
	return s.decryptMerged(merge(layers...))
}

// ResolveBeeEnv returns complete env vars for Bee execution (KEY=VALUE slice).
// Resolution chain: global <- bee
func (s *Service) ResolveBeeEnv(beeID string) ([]string, error) {
	layers, err := fetchParallel(
		func() ([]*model.EnvConfig, error) { return s.store.List(ScopeGlobal, nil) },
		func() ([]*model.EnvConfig, error) { return s.store.List(ScopeBee, &beeID) },
	)
	if err != nil {
		return nil, err
	}
	return s.decryptMerged(merge(layers...))
}

// decryptMerged decrypts a merged map and returns sorted KEY=VALUE strings.
// Sorted for deterministic ordering so subprocess env is predictable.
func (s *Service) decryptMerged(merged map[string]string) ([]string, error) {
	result := make([]string, 0, len(merged))
	for k, encVal := range merged {
		plainVal, err := crypto.DecryptGCM(s.gcm, encVal)
		if err != nil {
			return nil, fmt.Errorf("decrypt env value for key %s: %w", k, err)
		}
		result = append(result, k+"="+plainVal)
	}
	sort.Strings(result)
	return result, nil
}

// fetchParallel runs each fetcher concurrently and returns results in input order.
// Returns on the first error (in input order).
func fetchParallel(fetchers ...func() ([]*model.EnvConfig, error)) ([][]*model.EnvConfig, error) {
	type result struct {
		envs []*model.EnvConfig
		err  error
	}
	chs := make([]chan result, len(fetchers))
	var wg sync.WaitGroup
	wg.Add(len(fetchers))
	for i, f := range fetchers {
		chs[i] = make(chan result, 1)
		ch, fn := chs[i], f
		go func() { defer wg.Done(); e, err := fn(); ch <- result{e, err} }()
	}
	wg.Wait()

	out := make([][]*model.EnvConfig, len(fetchers))
	for i, ch := range chs {
		r := <-ch
		if r.err != nil {
			return nil, r.err
		}
		out[i] = r.envs
	}
	return out, nil
}

// merge merges multiple layers of env configs. Later layers override earlier ones for the same key.
func merge(layers ...[]*model.EnvConfig) map[string]string {
	result := make(map[string]string)
	for _, layer := range layers {
		for _, cfg := range layer {
			result[cfg.Key] = cfg.EncValue
		}
	}
	return result
}
