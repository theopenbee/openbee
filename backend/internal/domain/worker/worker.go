package worker

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
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
	EngineArgs       string // JSON: map[engine]rawCLIString
}

// UpdateWorkerParams holds the inputs for a partial worker update.
type UpdateWorkerParams struct {
	Name             *string `json:"name"`
	Description      *string `json:"description"`
	Constraints      *string `json:"constraints"`
	WorkDir          *string `json:"work_dir"`
	PermissionScopes *string `json:"permission_scopes"`
	Engine           *string `json:"engine"`
	// nil = no change; empty map clears all; per-engine empty string deletes that entry.
	EngineArgs map[string]string `json:"engine_args"`
}

func (p UpdateWorkerParams) HasChanges() bool {
	return p.Name != nil || p.Description != nil || p.Constraints != nil ||
		p.WorkDir != nil || p.PermissionScopes != nil || p.Engine != nil || p.EngineArgs != nil
}

func (p UpdateWorkerParams) Validate() error {
	if p.WorkDir != nil {
		if strings.TrimSpace(*p.WorkDir) == "" {
			return fmt.Errorf("work_dir cannot be empty: %w", ErrValidation)
		}
	}
	if p.PermissionScopes != nil {
		if err := auth.ValidatePermissionScopes(*p.PermissionScopes); err != nil {
			return err
		}
	}
	return nil
}

func (p UpdateWorkerParams) validateEngineChanges(m *Manager, current model.Worker) error {
	if p.Engine != nil && *p.Engine != current.Engine {
		if err := m.ValidateEngine(*p.Engine); err != nil {
			return fmt.Errorf("%w: %w", err, ErrValidation)
		}
	}
	if len(p.EngineArgs) == 0 {
		return nil
	}
	changed, err := ChangedEngineArgs(current.EngineArgs, p.EngineArgs)
	if err != nil {
		return err
	}
	if err := m.ValidateEngineArgs(changed); err != nil {
		return fmt.Errorf("%w: %w", err, ErrValidation)
	}
	return nil
}

func ChangedEngineArgs(stored string, submitted map[string]string) (map[string]string, error) {
	existing, err := storedEngineArgs(stored)
	if err != nil {
		return nil, err
	}
	changed := make(map[string]string)
	for engine, args := range submitted {
		if args == "" {
			continue
		}
		if prev, ok := existing[engine]; ok && prev == args {
			continue
		}
		changed[engine] = args
	}
	return changed, nil
}

func storedEngineArgs(raw string) (map[string]string, error) {
	existing := make(map[string]string)
	if raw == "" || raw == "{}" {
		return existing, nil
	}
	if err := json.Unmarshal([]byte(raw), &existing); err != nil {
		return nil, fmt.Errorf("parse existing engine_args: %w", err)
	}
	return existing, nil
}

func (p UpdateWorkerParams) ApplyTo(w *model.Worker) error {
	if p.Name != nil {
		w.Name = *p.Name
	}
	if p.Description != nil {
		w.Description = *p.Description
	}
	if p.Constraints != nil {
		w.Constraints = *p.Constraints
	}
	if p.WorkDir != nil {
		w.WorkDir = strings.TrimSpace(*p.WorkDir)
	}
	if p.PermissionScopes != nil {
		w.PermissionScopes = *p.PermissionScopes
	}
	if p.Engine != nil {
		w.Engine = *p.Engine
	}
	if p.EngineArgs == nil {
		return nil
	}
	if len(p.EngineArgs) == 0 {
		w.EngineArgs = "{}"
		return nil
	}
	existing, err := storedEngineArgs(w.EngineArgs)
	if err != nil {
		return err
	}
	for engine, args := range p.EngineArgs {
		if args == "" {
			delete(existing, engine)
		} else {
			existing[engine] = args
		}
	}
	b, _ := json.Marshal(existing)
	w.EngineArgs = string(b)
	return nil
}

func (m *Manager) validateWorkerName(name, excludeID string) error {
	if name == "" {
		return fmt.Errorf("worker name cannot be empty: %w", ErrValidation)
	}
	if m.conflictsWithBotName(name) {
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
	if err := p.Validate(); err != nil {
		return model.Worker{}, err
	}
	w, err := m.workerStore.GetByID(id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.Worker{}, ErrNotFound
		}
		return model.Worker{}, fmt.Errorf("get worker: %w", err)
	}
	if err := p.validateEngineChanges(m, w); err != nil {
		return model.Worker{}, err
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
	if err := p.ApplyTo(&w); err != nil {
		return model.Worker{}, err
	}
	return m.workerStore.Update(w)
}

func (m *Manager) CreateWorker(p CreateWorkerParams) (model.Worker, error) {
	p.Name = strings.TrimSpace(p.Name)
	p.WorkDir = strings.TrimSpace(p.WorkDir)
	if err := m.validateWorkerName(p.Name, ""); err != nil {
		return model.Worker{}, err
	}
	id := uuid.New().String()
	if p.WorkDir == "" {
		p.WorkDir = filepath.Join(m.workerBaseDir, id)
	}

	if err := os.MkdirAll(p.WorkDir, 0o755); err != nil {
		return model.Worker{}, fmt.Errorf("create work dir: %w", err)
	}

	engineArgs := p.EngineArgs
	if engineArgs == "" {
		engineArgs = "{}"
	}
	workerModel := model.Worker{
		ID:               id,
		Name:             p.Name,
		Description:      p.Description,
		Constraints:      p.Constraints,
		WorkDir:          p.WorkDir,
		Engine:           p.Engine,
		EngineArgs:       engineArgs,
		PermissionScopes: p.PermissionScopes,
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
