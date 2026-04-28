package group

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/google/uuid"
	ai "github.com/theopenbee/openbee/internal/ai"
	"github.com/theopenbee/openbee/internal/domain/enginecfg"
	"github.com/theopenbee/openbee/internal/infra/auth"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/infra/store"
)

var (
	ErrValidation = errors.New("validation")
	ErrNotFound   = errors.New("group not found")
)

type Manager struct {
	groupBaseDir  string
	groupStore    *store.GroupStore
	workerStore   *store.WorkerStore
	taskStore     *store.TaskStore
	engines       map[string]ai.EngineAdapter
	engineCfg     *enginecfg.Store
	botNamesLower []string
}

func NewManager(
	baseDir string,
	gs *store.GroupStore, ws *store.WorkerStore, ts *store.TaskStore,
	engines map[string]ai.EngineAdapter, engineCfg *enginecfg.Store,
	botNames []string,
) *Manager {
	lower := make([]string, len(botNames))
	for i, n := range botNames {
		lower[i] = strings.ToLower(strings.TrimSpace(n))
	}
	return &Manager{
		groupBaseDir:  baseDir,
		groupStore:    gs,
		workerStore:   ws,
		taskStore:     ts,
		engines:       engines,
		engineCfg:     engineCfg,
		botNamesLower: lower,
	}
}

type CreateGroupParams struct {
	Name             string
	Description      string
	Constraints      string
	WorkDir          string
	PermissionScopes string
	Engine           string
	EngineArgs       string
}

func (m *Manager) CreateGroup(p CreateGroupParams) (model.Group, error) {
	p.Name = strings.TrimSpace(p.Name)
	if err := m.validateName(p.Name, ""); err != nil {
		return model.Group{}, err
	}
	if err := m.ValidateEngine(p.Engine); err != nil {
		return model.Group{}, err
	}
	if err := auth.ValidatePermissionScopes(p.PermissionScopes); err != nil {
		return model.Group{}, fmt.Errorf("%w: %v", ErrValidation, err)
	}
	id := uuid.New().String()
	if p.WorkDir == "" {
		p.WorkDir = filepath.Join(m.groupBaseDir, id)
	}
	if err := os.MkdirAll(p.WorkDir, 0o755); err != nil {
		return model.Group{}, fmt.Errorf("create work dir: %w", err)
	}
	engineArgs := p.EngineArgs
	if engineArgs == "" {
		engineArgs = "{}"
	}
	if err := m.ValidateEngineArgsJSON(engineArgs); err != nil {
		return model.Group{}, err
	}
	g := model.Group{
		ID:               id,
		Name:             p.Name,
		Description:      p.Description,
		Constraints:      p.Constraints,
		WorkDir:          p.WorkDir,
		Engine:           p.Engine,
		EngineArgs:       engineArgs,
		PermissionScopes: p.PermissionScopes,
	}
	// Optional engine prepare (skipped in tests where engines == nil).
	if m.engines != nil && m.engineCfg != nil {
		if _, engine, err := m.resolveEngine(g); err == nil {
			if err := engine.Prepare(p.WorkDir, ai.PrepareOptions{Role: ai.RoleWorker}); err != nil {
				return model.Group{}, fmt.Errorf("prepare group workspace: %w", err)
			}
		}
	}
	return m.groupStore.Create(g)
}

func (m *Manager) GetGroup(id string) (model.Group, error) {
	return m.groupStore.GetByID(id)
}

func (m *Manager) ListGroups() ([]model.Group, error) {
	return m.groupStore.List()
}

func (m *Manager) UpdateGroup(g model.Group) (model.Group, error) {
	if err := m.validateName(g.Name, g.ID); err != nil {
		return model.Group{}, err
	}
	if err := m.ValidateEngine(g.Engine); err != nil {
		return model.Group{}, err
	}
	if err := auth.ValidatePermissionScopes(g.PermissionScopes); err != nil {
		return model.Group{}, fmt.Errorf("%w: %v", ErrValidation, err)
	}
	if err := m.ValidateEngineArgsJSON(g.EngineArgs); err != nil {
		return model.Group{}, err
	}
	return m.groupStore.Update(g)
}

func (m *Manager) ValidateEngine(name string) error {
	if name == "" || m.engines == nil {
		return nil
	}
	if _, ok := m.engines[name]; !ok {
		return fmt.Errorf("engine %q is not enabled: %w", name, ErrValidation)
	}
	return nil
}

func (m *Manager) ValidateEngineArgsJSON(raw string) error {
	if raw == "" || raw == "{}" {
		return nil
	}
	var parsed map[string]string
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return fmt.Errorf("invalid engine_args JSON: %w: %v", ErrValidation, err)
	}
	for engine := range parsed {
		if engine == "" {
			return fmt.Errorf("engine_args contains an empty engine name: %w", ErrValidation)
		}
		if err := m.ValidateEngine(engine); err != nil {
			return fmt.Errorf("engine_args[%q]: %w", engine, err)
		}
	}
	if _, err := ai.ParseEngineArgs(parsed); err != nil {
		return fmt.Errorf("invalid engine_args: %w: %v", ErrValidation, err)
	}
	return nil
}

func (m *Manager) DeleteGroup(id string, deleteWorkDir bool) error {
	// Refuse if there are non-terminal root tasks for this group.
	active, err := m.hasActiveRootTask(id)
	if err != nil {
		return err
	}
	if active {
		return fmt.Errorf("group has active root task: %w", ErrValidation)
	}
	if deleteWorkDir {
		g, err := m.groupStore.GetByID(id)
		if err != nil {
			return err
		}
		if g.WorkDir != "" {
			if err := os.RemoveAll(g.WorkDir); err != nil {
				return fmt.Errorf("remove work dir: %w", err)
			}
		}
	}
	return m.groupStore.Delete(id)
}

func (m *Manager) AddMember(groupID, workerID string) error {
	if _, err := m.groupStore.GetByID(groupID); err != nil {
		return ErrNotFound
	}
	if _, err := m.workerStore.GetByID(workerID); err != nil {
		return fmt.Errorf("worker not found: %w", err)
	}
	return m.groupStore.AddMember(groupID, workerID, "member")
}

func (m *Manager) RemoveMember(groupID, workerID string) error {
	return m.groupStore.RemoveMember(groupID, workerID)
}

func (m *Manager) validateName(name, excludeID string) error {
	if name == "" {
		return fmt.Errorf("group name cannot be empty: %w", ErrValidation)
	}
	lower := strings.ToLower(name)
	if slices.Contains(m.botNamesLower, lower) {
		return fmt.Errorf("group name %q conflicts with bot name: %w", name, ErrValidation)
	}
	// Check both group and worker namespaces.
	if exists, err := m.groupStore.ExistsByName(name, excludeID); err != nil {
		return err
	} else if exists {
		return fmt.Errorf("group name %q already taken: %w", name, ErrValidation)
	}
	if exists, err := m.workerStore.ExistsByName(name, ""); err != nil {
		return err
	} else if exists {
		return fmt.Errorf("group name %q conflicts with existing worker: %w", name, ErrValidation)
	}
	return nil
}

func (m *Manager) hasActiveRootTask(groupID string) (bool, error) {
	tasks, err := m.taskStore.List(context.Background(), store.TaskFilter{
		WorkerID: groupID,
		Status:   "pending,running,waiting_subtasks",
		Limit:    1,
	})
	if err != nil {
		return false, fmt.Errorf("check active tasks: %w", err)
	}
	return len(tasks) > 0, nil
}

func (m *Manager) resolveEngine(g model.Group) (string, ai.EngineAdapter, error) {
	if g.Engine != "" {
		if e, ok := m.engines[g.Engine]; ok {
			return g.Engine, e, nil
		}
	}
	name := m.engineCfg.Get()
	e, ok := m.engines[name]
	if !ok {
		return "", nil, fmt.Errorf("no engine adapter for default %q", name)
	}
	return name, e, nil
}
