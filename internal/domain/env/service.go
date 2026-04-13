package env

import (
	"fmt"
	"sort"

	"github.com/theopenbee/openbee/internal/infra/crypto"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/infra/store"
)

type Service struct {
	store       *store.EnvConfigStore
	workerStore *store.WorkerStore
	deptStore   *store.DepartmentStore
	encKey      string // config.Advanced.EnvSecret
}

func NewService(envStore *store.EnvConfigStore, ws *store.WorkerStore, ds *store.DepartmentStore, encKey string) *Service {
	return &Service{
		store:       envStore,
		workerStore: ws,
		deptStore:   ds,
		encKey:      encKey,
	}
}

// Create encrypts plainValue and persists the env config.
func (s *Service) Create(scope, scopeID, key, plainValue string) (*model.EnvConfig, error) {
	if key == "OPENBEE_API_KEY" {
		return nil, fmt.Errorf("OPENBEE_API_KEY is reserved and cannot be set")
	}

	switch scope {
	case "global", "bee", "department", "worker":
		// valid
	default:
		return nil, fmt.Errorf("invalid scope %q: must be one of global, bee, department, worker", scope)
	}

	if scope != "global" && scopeID == "" {
		return nil, fmt.Errorf("scope_id is required for scope %q", scope)
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
	if existing.Key == "OPENBEE_API_KEY" {
		return fmt.Errorf("OPENBEE_API_KEY is reserved and cannot be set")
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
// Returns error if the config's key is "OPENBEE_API_KEY".
func (s *Service) Delete(id string) error {
	existing, err := s.store.Get(id)
	if err != nil {
		return fmt.Errorf("get env config: %w", err)
	}
	if existing == nil {
		return fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	if existing.Key == "OPENBEE_API_KEY" {
		return fmt.Errorf("OPENBEE_API_KEY is reserved and cannot be deleted")
	}
	return s.store.Delete(id)
}

// ResolveWorkerEnv returns complete env vars for Worker execution (KEY=VALUE slice).
// Resolution chain: global <- department (sorted by dept_id) <- worker
func (s *Service) ResolveWorkerEnv(workerID string) ([]string, error) {
	// Get worker's departments
	depts, err := s.deptStore.GetWorkerDepartments(workerID)
	if err != nil {
		return nil, fmt.Errorf("get worker departments: %w", err)
	}

	// Extract and sort department IDs for determinism
	deptIDs := make([]string, 0, len(depts))
	for _, d := range depts {
		deptIDs = append(deptIDs, d.ID)
	}
	sort.Strings(deptIDs)

	// Get global envs
	globalEnvs, err := s.store.List("global", nil)
	if err != nil {
		return nil, fmt.Errorf("list global env configs: %w", err)
	}

	// Get department envs
	deptEnvs, err := s.store.ListForDepartments(deptIDs)
	if err != nil {
		return nil, fmt.Errorf("list department env configs: %w", err)
	}

	// Get worker envs
	workerEnvs, err := s.store.List("worker", &workerID)
	if err != nil {
		return nil, fmt.Errorf("list worker env configs: %w", err)
	}

	// Merge: global <- depts <- worker
	merged := merge(globalEnvs, deptEnvs, workerEnvs)

	// Decrypt and format as KEY=VALUE
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

// ResolveBeeEnv returns complete env vars for Bee execution (KEY=VALUE slice).
// Resolution chain: global <- bee
func (s *Service) ResolveBeeEnv(beeID string) ([]string, error) {
	// Get global envs
	globalEnvs, err := s.store.List("global", nil)
	if err != nil {
		return nil, fmt.Errorf("list global env configs: %w", err)
	}

	// Get bee envs
	beeEnvs, err := s.store.List("bee", &beeID)
	if err != nil {
		return nil, fmt.Errorf("list bee env configs: %w", err)
	}

	// Merge: global <- bee
	merged := merge(globalEnvs, beeEnvs)

	// Decrypt and format as KEY=VALUE
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
