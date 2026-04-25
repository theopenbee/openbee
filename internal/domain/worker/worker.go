package worker

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/google/uuid"
	ai "github.com/theopenbee/openbee/internal/ai"
	"github.com/theopenbee/openbee/internal/infra/auth"
	"github.com/theopenbee/openbee/internal/infra/model"
)

// CreateWorkerParams holds the inputs for creating a new worker.
type CreateWorkerParams struct {
	Name             string
	Description      string
	Constraints      string
	WorkDir          string
	PermissionScopes string
	Engine           string
}

// UpdateWorkerParams holds the inputs for a partial worker update.
type UpdateWorkerParams struct {
	Name             *string `json:"name"`
	Description      *string `json:"description"`
	Constraints      *string `json:"constraints"`
	PermissionScopes *string `json:"permission_scopes"`
	Engine           *string `json:"engine"`
}

func (p UpdateWorkerParams) HasChanges() bool {
	return p.Name != nil || p.Description != nil || p.Constraints != nil ||
		p.PermissionScopes != nil || p.Engine != nil
}

func (p UpdateWorkerParams) Validate(m *Manager) error {
	if p.PermissionScopes != nil {
		if err := auth.ValidatePermissionScopes(*p.PermissionScopes); err != nil {
			return err
		}
	}
	if p.Engine != nil {
		return m.ValidateEngine(*p.Engine)
	}
	return nil
}

func (p UpdateWorkerParams) ApplyTo(w *model.Worker) {
	if p.Name != nil {
		w.Name = *p.Name
	}
	if p.Description != nil {
		w.Description = *p.Description
	}
	if p.Constraints != nil {
		w.Constraints = *p.Constraints
	}
	if p.PermissionScopes != nil {
		w.PermissionScopes = *p.PermissionScopes
	}
	if p.Engine != nil {
		w.Engine = *p.Engine
	}
}

func (m *Manager) validateWorkerName(name, excludeID string) error {
	if name == "" {
		return fmt.Errorf("worker name cannot be empty: %w", ErrValidation)
	}
	lower := strings.ToLower(name)
	if slices.Contains(m.botNamesLower, lower) {
		return fmt.Errorf("worker name %q conflicts with bot name: %w", name, ErrValidation)
	}
	exists, err := m.workerStore.ExistsByName(name, excludeID)
	if err != nil {
		return fmt.Errorf("check worker name: %w", err)
	}
	if exists {
		return fmt.Errorf("worker name %q is already taken: %w", name, ErrValidation)
	}
	return nil
}

func (m *Manager) UpdateWorker(id string, p UpdateWorkerParams) (model.Worker, error) {
	if err := p.Validate(m); err != nil {
		return model.Worker{}, err
	}
	w, err := m.workerStore.GetByID(id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.Worker{}, ErrNotFound
		}
		return model.Worker{}, fmt.Errorf("get worker: %w", err)
	}
	if p.Name != nil {
		trimmed := strings.TrimSpace(*p.Name)
		if trimmed == w.Name {
			p.Name = nil
		} else {
			p.Name = &trimmed
			if err := m.validateWorkerName(trimmed, id); err != nil {
				return model.Worker{}, err
			}
		}
	}
	if !p.HasChanges() {
		return w, nil
	}
	p.ApplyTo(&w)
	return m.workerStore.Update(w)
}

func (m *Manager) CreateWorker(p CreateWorkerParams) (model.Worker, error) {
	p.Name = strings.TrimSpace(p.Name)
	if err := m.validateWorkerName(p.Name, ""); err != nil {
		return model.Worker{}, err
	}
	id := uuid.New().String()
	if p.WorkDir == "" {
		p.WorkDir = filepath.Join(m.workerBaseDir, id)
	}

	if err := os.MkdirAll(p.WorkDir, 0755); err != nil {
		return model.Worker{}, fmt.Errorf("create work dir: %w", err)
	}

	workerModel := model.Worker{
		ID:               id,
		Name:             p.Name,
		Description:      p.Description,
		Constraints:      p.Constraints,
		WorkDir:          p.WorkDir,
		Engine:           p.Engine,
		PermissionScopes: p.PermissionScopes,
	}
	_, engine := m.resolveEngine(workerModel)
	if err := engine.Prepare(p.WorkDir, ai.PrepareOptions{Role: ai.RoleWorker}); err != nil {
		return model.Worker{}, fmt.Errorf("prepare worker workspace: %w", err)
	}

	return m.workerStore.Create(workerModel)
}

func (m *Manager) DeleteWorker(id string, deleteWorkDir bool) error {
	if deleteWorkDir {
		worker, err := m.workerStore.GetByID(id)
		if err != nil {
			return fmt.Errorf("get worker: %w", err)
		}
		if worker.WorkDir != "" {
			if err := os.RemoveAll(worker.WorkDir); err != nil {
				return fmt.Errorf("remove work dir: %w", err)
			}
		}
	}
	return m.workerStore.Delete(id)
}
