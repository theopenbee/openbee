package env

import (
	"crypto/cipher"
	"fmt"
	"sort"

	"golang.org/x/sync/errgroup"

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

func (s *Service) encryptAndMask(plainValue string) (encValue, masked string, err error) {
	encValue, err = crypto.EncryptGCM(s.gcm, plainValue)
	if err != nil {
		return "", "", fmt.Errorf("encrypt env value: %w", err)
	}
	return encValue, crypto.Mask(plainValue), nil
}

func validateScope(scope string) error {
	switch scope {
	case ScopeGlobal, ScopeBee, ScopeDepartment, ScopeWorker:
		return nil
	default:
		return fmt.Errorf("%w: invalid scope %q: must be one of global, bee, department, worker", ErrValidation, scope)
	}
}

func (s *Service) Create(scope, scopeID, key, plainValue string) (*model.EnvConfig, error) {
	if key == reservedKey {
		return nil, fmt.Errorf("%w: OPENBEE_API_KEY is reserved and cannot be set", ErrValidation)
	}

	if err := validateScope(scope); err != nil {
		return nil, err
	}

	if scope != ScopeGlobal && scopeID == "" {
		return nil, fmt.Errorf("%w: scope_id is required for scope %q", ErrValidation, scope)
	}

	encValue, masked, err := s.encryptAndMask(plainValue)
	if err != nil {
		return nil, err
	}

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

	encValue, masked, err := s.encryptAndMask(plainValue)
	if err != nil {
		return err
	}

	if err := s.store.Update(id, encValue, masked); err != nil {
		return fmt.Errorf("update env config: %w", err)
	}

	return nil
}

func (s *Service) List(scope string, scopeID *string) ([]*model.EnvConfig, error) {
	if err := validateScope(scope); err != nil {
		return nil, err
	}
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
	var globalEnvs, workerEnvs []*model.EnvConfig
	var depts []model.Department

	var g errgroup.Group
	g.Go(func() error {
		var err error
		globalEnvs, err = s.store.List(ScopeGlobal, nil)
		return err
	})
	g.Go(func() error {
		var err error
		depts, err = s.deptStore.GetWorkerDepartments(workerID)
		return err
	})
	g.Go(func() error {
		var err error
		workerEnvs, err = s.store.List(ScopeWorker, &workerID)
		return err
	})
	if err := g.Wait(); err != nil {
		return nil, err
	}

	deptIDs := make([]string, len(depts))
	for i, d := range depts {
		deptIDs[i] = d.ID
	}

	deptEnvs, err := s.store.ListForDepartments(deptIDs)
	if err != nil {
		return nil, err
	}

	return s.decryptMerged(merge(globalEnvs, deptEnvs, workerEnvs))
}

// ResolveBeeEnv returns complete env vars for Bee execution (KEY=VALUE slice).
// Resolution chain: global <- bee
func (s *Service) ResolveBeeEnv(beeID string) ([]string, error) {
	var globalEnvs, beeEnvs []*model.EnvConfig
	var g errgroup.Group
	g.Go(func() error {
		var err error
		globalEnvs, err = s.store.List(ScopeGlobal, nil)
		return err
	})
	g.Go(func() error {
		var err error
		beeEnvs, err = s.store.List(ScopeBee, &beeID)
		return err
	})
	if err := g.Wait(); err != nil {
		return nil, err
	}
	return s.decryptMerged(merge(globalEnvs, beeEnvs))
}

// decryptMerged sorts for deterministic subprocess env ordering.
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

// merge applies layers left-to-right; later layers override earlier ones for the same key.
func merge(layers ...[]*model.EnvConfig) map[string]string {
	result := make(map[string]string)
	for _, layer := range layers {
		for _, cfg := range layer {
			result[cfg.Key] = cfg.EncValue
		}
	}
	return result
}
