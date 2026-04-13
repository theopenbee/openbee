package env

import (
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
	encKey    string
}

func NewService(envStore *store.EnvConfigStore, ds *store.DepartmentStore, encKey string) *Service {
	return &Service{
		store:     envStore,
		deptStore: ds,
		encKey:    encKey,
	}
}

// Create encrypts plainValue and persists the env config.
func (s *Service) Create(scope, scopeID, key, plainValue string) (*model.EnvConfig, error) {
	if key == reservedKey {
		return nil, fmt.Errorf("%w: OPENBEE_API_KEY is reserved and cannot be set", ErrValidation)
	}

	switch scope {
	case "global", "bee", "department", "worker":
	default:
		return nil, fmt.Errorf("%w: invalid scope %q: must be one of global, bee, department, worker", ErrValidation, scope)
	}

	if scope != "global" && scopeID == "" {
		return nil, fmt.Errorf("%w: scope_id is required for scope %q", ErrValidation, scope)
	}

	encValue, err := crypto.Encrypt(s.encKey, plainValue)
	if err != nil {
		return nil, fmt.Errorf("encrypt env value: %w", err)
	}

	masked := crypto.Mask(plainValue)

	var scopeIDPtr *string
	if scope != "global" {
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

// UpdateValue updates the encrypted value for an existing env config.
func (s *Service) UpdateValue(id, plainValue string) error {
	existing, err := s.store.Get(id)
	if err != nil {
		return fmt.Errorf("get env config: %w", err)
	}
	if existing == nil {
		return fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	if existing.Key == reservedKey {
		return fmt.Errorf("%w: %s is reserved and cannot be set", ErrValidation, reservedKey)
	}

	encValue, err := crypto.Encrypt(s.encKey, plainValue)
	if err != nil {
		return fmt.Errorf("encrypt env value: %w", err)
	}

	masked := crypto.Mask(plainValue)

	if err := s.store.Update(id, encValue, masked); err != nil {
		return fmt.Errorf("update env config: %w", err)
	}

	return nil
}

// List returns env configs for the given scope/scopeID.
// scopeID can be nil (for global scope).
func (s *Service) List(scope string, scopeID *string) ([]*model.EnvConfig, error) {
	return s.store.List(scope, scopeID)
}

// Delete removes an env config by ID.
// Returns error if the config's key is reservedKey.
func (s *Service) Delete(id string) error {
	existing, err := s.store.Get(id)
	if err != nil {
		return fmt.Errorf("get env config: %w", err)
	}
	if existing == nil {
		return fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	if existing.Key == reservedKey {
		return fmt.Errorf("%w: %s is reserved and cannot be deleted", ErrValidation, reservedKey)
	}
	return s.store.Delete(id)
}

// ResolveWorkerEnv returns complete env vars for Worker execution (KEY=VALUE slice).
// Resolution chain: global <- department (sorted by dept_id) <- worker
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

	type result struct {
		envs []*model.EnvConfig
		err  error
	}
	globalCh := make(chan result, 1)
	deptCh := make(chan result, 1)
	workerCh := make(chan result, 1)

	var wg sync.WaitGroup
	wg.Add(3)
	go func() { defer wg.Done(); e, err := s.store.List("global", nil); globalCh <- result{e, err} }()
	go func() { defer wg.Done(); e, err := s.store.ListForDepartments(deptIDs); deptCh <- result{e, err} }()
	go func() { defer wg.Done(); e, err := s.store.List("worker", &workerID); workerCh <- result{e, err} }()
	wg.Wait()

	gr, dr, wr := <-globalCh, <-deptCh, <-workerCh
	if gr.err != nil {
		return nil, fmt.Errorf("list global env configs: %w", gr.err)
	}
	if dr.err != nil {
		return nil, fmt.Errorf("list department env configs: %w", dr.err)
	}
	if wr.err != nil {
		return nil, fmt.Errorf("list worker env configs: %w", wr.err)
	}

	return s.decryptMerged(merge(gr.envs, dr.envs, wr.envs))
}

// ResolveBeeEnv returns complete env vars for Bee execution (KEY=VALUE slice).
// Resolution chain: global <- bee
func (s *Service) ResolveBeeEnv(beeID string) ([]string, error) {
	globalEnvs, err := s.store.List("global", nil)
	if err != nil {
		return nil, fmt.Errorf("list global env configs: %w", err)
	}

	beeEnvs, err := s.store.List("bee", &beeID)
	if err != nil {
		return nil, fmt.Errorf("list bee env configs: %w", err)
	}

	return s.decryptMerged(merge(globalEnvs, beeEnvs))
}

// decryptMerged decrypts a merged map of key -> encValue and returns KEY=VALUE strings.
func (s *Service) decryptMerged(merged map[string]string) ([]string, error) {
	result := make([]string, 0, len(merged))
	for k, encVal := range merged {
		plainVal, err := crypto.Decrypt(s.encKey, encVal)
		if err != nil {
			return nil, fmt.Errorf("decrypt env value for key %s: %w", k, err)
		}
		result = append(result, k+"="+plainVal)
	}
	return result, nil
}

// merge merges multiple layers of env configs. Later layers override earlier ones for the same key.
// Stores EncValue; callers are responsible for decrypting.
func merge(layers ...[]*model.EnvConfig) map[string]string {
	result := make(map[string]string)
	for _, layer := range layers {
		for _, cfg := range layer {
			result[cfg.Key] = cfg.EncValue
		}
	}
	return result
}
