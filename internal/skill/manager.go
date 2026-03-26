package skill

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sync"
)

var validSkillName = regexp.MustCompile(`^[a-zA-Z0-9_:-]+$`)

// validateName returns an error if name contains characters that are unsafe
// for use as a filesystem directory name within the registry.
func validateName(name string) error {
	if name == "" {
		return fmt.Errorf("skill name cannot be empty")
	}
	if !validSkillName.MatchString(name) {
		return fmt.Errorf("skill name %q contains invalid characters (allowed: a-z, A-Z, 0-9, _, :, -)", name)
	}
	return nil
}

// ErrSkillNotFound is returned when a requested skill does not exist.
var ErrSkillNotFound = fmt.Errorf("skill not found")

// Manager orchestrates all skill operations.
type Manager struct {
	store    *Store
	registry *Registry
	links    *LinkManager
	scanner  *Scanner
	mu       sync.Mutex
}

// NewManager creates a Manager.
// stateDir is ~/.openbee (skills.json lives at stateDir/skills.json, registry at stateDir/skills/).
// globalSkillsDir is ~/.claude/skills.
func NewManager(stateDir, globalSkillsDir string) *Manager {
	registryRoot := filepath.Join(stateDir, "skills")
	return &Manager{
		store:    NewStore(filepath.Join(stateDir, "skills.json")),
		registry: NewRegistry(registryRoot),
		links:    NewLinkManager(registryRoot, globalSkillsDir),
		scanner:  NewScanner(registryRoot, globalSkillsDir),
	}
}

// LoadConfig returns the current SkillsConfig.
func (m *Manager) LoadConfig() (SkillsConfig, error) {
	return m.store.Load()
}

// Create creates a new managed skill at v1 and sets it as the global version.
func (m *Manager) Create(name, description, content string) error {
	if err := validateName(name); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	cfg, err := m.store.Load()
	if err != nil {
		return err
	}
	if _, exists := cfg.Skills[name]; exists {
		return fmt.Errorf("skill %q already exists", name)
	}

	versionDir, err := m.registry.CreateVersion(name, content)
	if err != nil {
		return fmt.Errorf("create version: %w", err)
	}
	version := filepath.Base(versionDir)

	if err := m.links.SetGlobal(name, version); err != nil {
		return fmt.Errorf("set global link: %w", err)
	}

	entry := newSkillEntry(description, version, version)
	cfg.Skills[name] = entry
	return m.store.Save(cfg)
}

// Edit saves new content as the next version. Does NOT update global_version.
// Returns the new version ID (e.g. "v2").
func (m *Manager) Edit(name, content string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cfg, err := m.store.Load()
	if err != nil {
		return "", err
	}
	entry, ok := cfg.Skills[name]
	if !ok {
		return "", fmt.Errorf("skill %q: %w", name, ErrSkillNotFound)
	}

	versionDir, err := m.registry.CreateVersion(name, content)
	if err != nil {
		return "", fmt.Errorf("create version: %w", err)
	}
	version := filepath.Base(versionDir)

	entry.LatestVersion = version
	entry.Versions[version] = VersionEntry{CreatedAt: now()}
	cfg.Skills[name] = entry
	return version, m.store.Save(cfg)
}

// ReadVersion returns the SKILL.md content for the given skill and version.
func (m *Manager) ReadVersion(name, version string) (string, error) {
	return m.registry.ReadVersion(name, version)
}

// UseGlobal switches the global symlink to the given version.
func (m *Manager) UseGlobal(name, version string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cfg, err := m.store.Load()
	if err != nil {
		return err
	}
	entry, ok := cfg.Skills[name]
	if !ok {
		return fmt.Errorf("skill %q: %w", name, ErrSkillNotFound)
	}
	if _, ok := entry.Versions[version]; !ok {
		return fmt.Errorf("version %q not found for skill %q", version, name)
	}
	if err := m.links.SetGlobal(name, version); err != nil {
		return err
	}
	entry.GlobalVersion = version
	cfg.Skills[name] = entry
	return m.store.Save(cfg)
}

// UseWorker sets a worker-scoped version override.
func (m *Manager) UseWorker(workerID, workDir, name, version string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cfg, err := m.store.Load()
	if err != nil {
		return err
	}
	entry, ok := cfg.Skills[name]
	if !ok {
		return fmt.Errorf("skill %q: %w", name, ErrSkillNotFound)
	}
	if _, ok := entry.Versions[version]; !ok {
		return fmt.Errorf("version %q not found for skill %q", version, name)
	}
	if err := m.links.SetWorker(workDir, name, version); err != nil {
		return err
	}
	if cfg.WorkerOverrides[workerID] == nil {
		cfg.WorkerOverrides[workerID] = make(map[string]string)
	}
	cfg.WorkerOverrides[workerID][name] = version
	return m.store.Save(cfg)
}

// RemoveWorkerOverride removes a worker-scoped version override (reverts to global).
func (m *Manager) RemoveWorkerOverride(workerID, workDir, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cfg, err := m.store.Load()
	if err != nil {
		return err
	}
	if err := m.links.RemoveWorker(workDir, name); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if overrides, ok := cfg.WorkerOverrides[workerID]; ok {
		delete(overrides, name)
		if len(overrides) == 0 {
			delete(cfg.WorkerOverrides, workerID)
		}
	}
	return m.store.Save(cfg)
}

// Delete removes a skill entirely. Fails if any worker has an override referencing it.
func (m *Manager) Delete(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cfg, err := m.store.Load()
	if err != nil {
		return err
	}
	if _, ok := cfg.Skills[name]; !ok {
		return fmt.Errorf("skill %q: %w", name, ErrSkillNotFound)
	}
	// Check for worker references.
	var refWorkers []string
	for wid, overrides := range cfg.WorkerOverrides {
		if _, ok := overrides[name]; ok {
			refWorkers = append(refWorkers, wid)
		}
	}
	if len(refWorkers) > 0 {
		return fmt.Errorf("skill %q is referenced by workers: %v; remove overrides first", name, refWorkers)
	}

	if err := m.links.RemoveGlobal(name); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := m.registry.DeleteSkill(name); err != nil {
		return err
	}
	delete(cfg.Skills, name)
	return m.store.Save(cfg)
}

// Versions returns the version history for a skill.
func (m *Manager) Versions(name string) (map[string]VersionEntry, error) {
	cfg, err := m.store.Load()
	if err != nil {
		return nil, err
	}
	entry, ok := cfg.Skills[name]
	if !ok {
		return nil, fmt.Errorf("skill %q: %w", name, ErrSkillNotFound)
	}
	return entry.Versions, nil
}

// List scans global skills and returns all of them (managed + external).
func (m *Manager) List() ([]ScannedSkill, error) {
	return m.scanner.ScanGlobal()
}

// ListWorker scans a worker's skill directory.
func (m *Manager) ListWorker(workerID, workDir string) ([]ScannedSkill, error) {
	return m.scanner.ScanWorker(workerID, workDir)
}

// AdoptGlobal converts an externally-placed global skill into an openbee-managed one.
func (m *Manager) AdoptGlobal(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.adopt(name, m.links.globalSkillsDir, func(version string) error {
		return m.links.SetGlobal(name, version)
	}, func(cfg *SkillsConfig, entry SkillEntry) {
		cfg.Skills[name] = entry
	})
}

// AdoptWorker converts an externally-placed worker skill into an openbee-managed one.
func (m *Manager) AdoptWorker(workerID, workDir, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	skillsDir := filepath.Join(workDir, ".claude", "skills")
	return m.adopt(name, skillsDir, func(version string) error {
		return m.links.SetWorker(workDir, name, version)
	}, func(cfg *SkillsConfig, entry SkillEntry) {
		cfg.Skills[name] = entry
		if cfg.WorkerOverrides[workerID] == nil {
			cfg.WorkerOverrides[workerID] = make(map[string]string)
		}
		cfg.WorkerOverrides[workerID][name] = entry.LatestVersion
	})
}

// CleanupWorkerLinks removes all openbee-managed symlinks from a worker's skill dir.
// Called during worker deletion when deleteWorkDir is false.
func (m *Manager) CleanupWorkerLinks(workerID, workDir string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cfg, err := m.store.Load()
	if err != nil {
		return err
	}
	if err := m.links.RemoveAllWorkerLinks(workDir); err != nil {
		return err
	}
	delete(cfg.WorkerOverrides, workerID)
	return m.store.Save(cfg)
}

// adopt is the shared logic for AdoptGlobal and AdoptWorker.
func (m *Manager) adopt(name, skillsDir string, setLink func(string) error, updateCfg func(*SkillsConfig, SkillEntry)) error {
	if err := validateName(name); err != nil {
		return err
	}
	target := filepath.Join(skillsDir, name)
	info, err := os.Lstat(target)
	if err != nil {
		return fmt.Errorf("skill %q not found at %s: %w", name, skillsDir, err)
	}

	// Resolve real content path.
	var contentDir string
	if info.Mode()&os.ModeSymlink != 0 {
		resolved, err := filepath.EvalSymlinks(target)
		if err != nil {
			return fmt.Errorf("resolve symlink: %w", err)
		}
		// If it already points into our registry, it's already managed.
		if isManagedLink(target, m.registry.root) {
			return fmt.Errorf("skill %q is already managed by openbee", name)
		}
		contentDir = resolved
	} else {
		contentDir = target
	}

	// Read SKILL.md.
	content, err := os.ReadFile(filepath.Join(contentDir, "SKILL.md"))
	if err != nil {
		return fmt.Errorf("read SKILL.md: %w", err)
	}

	// Write to registry as v1.
	versionDir, err := m.registry.CreateVersion(name, string(content))
	if err != nil {
		return fmt.Errorf("create registry version: %w", err)
	}
	version := filepath.Base(versionDir)

	// Move original aside (backup) before replacing with symlink.
	backup := target + ".backup"
	if err := os.Rename(target, backup); err != nil {
		return fmt.Errorf("backup original: %w", err)
	}
	if err := setLink(version); err != nil {
		// Restore backup on failure.
		_ = os.Rename(backup, target)
		return fmt.Errorf("set link: %w", err)
	}
	// setLink succeeded — now persist state.
	cfg, err := m.store.Load()
	if err != nil {
		// Restore backup.
		_ = os.Rename(backup, target)
		return err
	}
	entry := newSkillEntry("", version, version)
	updateCfg(&cfg, entry)
	if err := m.store.Save(cfg); err != nil {
		// Restore backup.
		_ = os.Rename(backup, target)
		return err
	}
	// All succeeded — remove backup.
	if err := os.RemoveAll(backup); err != nil {
		// Non-fatal: backup can be cleaned up manually.
		_ = err
	}
	return nil
}

// --- helpers ---

func newSkillEntry(description, latestVersion, globalVersion string) SkillEntry {
	return SkillEntry{
		Description:   description,
		LatestVersion: latestVersion,
		GlobalVersion: globalVersion,
		Versions: map[string]VersionEntry{
			latestVersion: {CreatedAt: now()},
		},
	}
}
