package pluginmgr

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/theopenbee/openbee/internal/platform"
	"github.com/theopenbee/openbee/internal/plugin"
)

const maxOperations = 50

const (
	OpQueued    = "queued"
	OpRunning   = "running"
	OpSucceeded = "succeeded"
	OpFailed    = "failed"
)

type Operation struct {
	ID         string
	Kind       string
	Spec       string
	PluginID   string
	Status     string
	Error      string
	CreatedAt  time.Time
	FinishedAt time.Time
}

func (m *Manager) newOp(kind, spec, pluginID string) Operation {
	op := &Operation{
		ID:        uuid.NewString(),
		Kind:      kind,
		Spec:      spec,
		PluginID:  pluginID,
		Status:    OpQueued,
		CreatedAt: time.Now(),
	}
	m.mu.Lock()
	m.ops = append(m.ops, op)
	if len(m.ops) > maxOperations {
		m.ops = m.ops[len(m.ops)-maxOperations:]
	}
	m.mu.Unlock()
	return *op
}

func (m *Manager) updateOp(id string, fn func(op *Operation)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, op := range m.ops {
		if op.ID == id {
			fn(op)
			return
		}
	}
}

func (m *Manager) finishOp(id string, err error) {
	m.updateOp(id, func(op *Operation) {
		op.FinishedAt = time.Now()
		if err != nil {
			op.Status = OpFailed
			op.Error = err.Error()
			return
		}
		op.Status = OpSucceeded
	})
}

func (m *Manager) Operations() []Operation {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Operation, 0, len(m.ops))
	for i := len(m.ops) - 1; i >= 0; i-- {
		out = append(out, *m.ops[i])
	}
	return out
}

func (m *Manager) Operation(id string) (Operation, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, op := range m.ops {
		if op.ID == id {
			return *op, true
		}
	}
	return Operation{}, false
}

func (m *Manager) opContext() context.Context {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.rootCtx != nil {
		return context.WithoutCancel(m.rootCtx)
	}
	return context.Background()
}

func (m *Manager) Install(spec string) Operation {
	op := m.newOp("install", spec, "")
	m.opsWG.Add(1)
	go func() {
		defer m.opsWG.Done()
		m.runInstall(op.ID, spec)
	}()
	return op
}

func (m *Manager) runInstall(opID, spec string) {
	m.pkgMu.Lock()
	defer m.pkgMu.Unlock()
	m.updateOp(opID, func(op *Operation) { op.Status = OpRunning })
	ctx := m.opContext()

	prevID := m.matchInstalled(spec)
	var unlockPrev func()
	stoppedPrev := false
	if prevID != "" {
		unlockPrev = m.lockID(prevID)
		m.mu.Lock()
		_, stoppedPrev = m.instances[prevID]
		m.upgrading[prevID] = true
		m.mu.Unlock()
		defer m.clearUpgrading(prevID)
		m.stopLocked(prevID)
	}

	entry, _, err := m.deps.Installer.Install(ctx, spec)
	if err != nil {
		if prevID != "" {
			if stoppedPrev {
				if serr := m.startLocked(ctx, prevID); serr != nil {
					log.Error("restart plugin after failed install", zap.String("plugin", prevID), zap.Error(serr))
				}
			}
			unlockPrev()
		}
		log.Warn("plugin install failed", zap.String("spec", spec), zap.Error(err))
		m.finishOp(opID, err)
		return
	}
	id := entry.ID
	m.updateOp(opID, func(op *Operation) { op.PluginID = id })
	if prevID != "" && prevID != id {
		if stoppedPrev {
			if serr := m.startLocked(ctx, prevID); serr != nil {
				log.Error("restart plugin", zap.String("plugin", prevID), zap.Error(serr))
			}
		}
		unlockPrev()
		prevID = ""
	}
	unlock := unlockPrev
	if prevID == "" {
		unlock = m.lockID(id)
	}
	defer unlock()

	row, found, err := m.deps.Store.Get(ctx, id)
	if err == nil && found && row.Enabled {
		m.stopLocked(id)
		if serr := m.startLocked(ctx, id); serr != nil {
			log.Error("start plugin after install", zap.String("plugin", id), zap.Error(serr))
		}
	}
	log.Info("plugin installed", zap.String("plugin", id), zap.String("version", entry.Version))
	m.finishOp(opID, nil)
}

func (m *Manager) matchInstalled(spec string) string {
	entries, err := m.deps.Registry.List()
	if err != nil {
		return ""
	}
	if info, err := os.Stat(spec); err == nil && info.IsDir() {
		abs, err := filepath.Abs(spec)
		if err != nil {
			return ""
		}
		for _, e := range entries {
			if e.Source == plugin.SourcePath && e.Path == abs {
				return e.ID
			}
		}
		return ""
	}
	name := packageNameFromSpec(spec)
	if name == "" {
		return ""
	}
	for _, e := range entries {
		if e.Source == plugin.SourceNpm && e.Package == name {
			return e.ID
		}
	}
	return ""
}

func packageNameFromSpec(spec string) string {
	if spec == "" || strings.Contains(spec, ":") || strings.HasPrefix(spec, ".") || strings.HasPrefix(spec, "/") || strings.HasPrefix(spec, "~") {
		return ""
	}
	if strings.HasPrefix(spec, "@") {
		name := spec
		if i := strings.Index(spec[1:], "@"); i >= 0 {
			name = spec[:i+1]
		}
		if strings.Count(name, "/") != 1 || strings.HasSuffix(name, "/") {
			return ""
		}
		return name
	}
	name, _, _ := strings.Cut(spec, "@")
	if strings.Contains(name, "/") {
		return ""
	}
	return name
}

func (m *Manager) Uninstall(ctx context.Context, id string) (Operation, error) {
	if _, err := m.entry(id); err != nil {
		return Operation{}, err
	}
	op := m.newOp("uninstall", "", id)
	m.opsWG.Add(1)
	go func() {
		defer m.opsWG.Done()
		m.runUninstall(op.ID, id)
	}()
	return op, nil
}

func (m *Manager) runUninstall(opID, id string) {
	m.pkgMu.Lock()
	defer m.pkgMu.Unlock()
	m.updateOp(opID, func(op *Operation) { op.Status = OpRunning })
	ctx := m.opContext()
	m.cancelSetup(id)
	unlock := m.lockID(id)
	defer unlock()

	m.stopLocked(id)
	if _, err := m.deps.Installer.Remove(ctx, id); err != nil && !errors.Is(err, plugin.ErrNotInstalled) {
		if row, found, gerr := m.deps.Store.Get(ctx, id); gerr == nil && found && row.Enabled {
			if serr := m.startLocked(ctx, id); serr != nil {
				log.Error("restart plugin after failed uninstall", zap.String("plugin", id), zap.Error(serr))
			}
		}
		m.finishOp(opID, err)
		return
	}
	platform.UnregisterExtractor(id)
	var errs []error
	if err := m.deps.Store.Delete(ctx, id); err != nil {
		errs = append(errs, err)
	}
	if err := os.RemoveAll(m.deps.Registry.DataDir(id)); err != nil {
		errs = append(errs, err)
	}
	m.refreshBotNames(ctx)
	log.Info("plugin uninstalled", zap.String("plugin", id))
	m.finishOp(opID, errors.Join(errs...))
}

func (m *Manager) clearUpgrading(id string) {
	m.mu.Lock()
	delete(m.upgrading, id)
	m.mu.Unlock()
}
