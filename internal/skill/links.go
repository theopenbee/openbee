package skill

import (
	"os"
	"path/filepath"
)

// LinkManager creates and removes symlinks in Claude Code skill directories.
type LinkManager struct {
	registryRoot    string
	globalSkillsDir string // ~/.claude/skills
}

// NewLinkManager returns a LinkManager.
// registryRoot is where versioned skill dirs live (e.g. ~/.openbee/skills).
// globalSkillsDir is ~/.claude/skills.
func NewLinkManager(registryRoot, globalSkillsDir string) *LinkManager {
	if abs, err := filepath.Abs(registryRoot); err == nil {
		registryRoot = abs
	}
	if abs, err := filepath.Abs(globalSkillsDir); err == nil {
		globalSkillsDir = abs
	}
	return &LinkManager{
		registryRoot:    registryRoot,
		globalSkillsDir: globalSkillsDir,
	}
}

// SetGlobal creates or updates the global symlink for skillName pointing to version.
func (lm *LinkManager) SetGlobal(skillName, version string) error {
	target := filepath.Join(lm.registryRoot, skillName, version)
	link := filepath.Join(lm.globalSkillsDir, skillName)
	return atomicSymlink(target, link)
}

// RemoveGlobal removes the global symlink for skillName.
func (lm *LinkManager) RemoveGlobal(skillName string) error {
	return removeLink(filepath.Join(lm.globalSkillsDir, skillName))
}

// SetWorker creates or updates the worker-scoped symlink for skillName.
// workDir is the worker's working directory.
func (lm *LinkManager) SetWorker(workDir, skillName, version string) error {
	target := filepath.Join(lm.registryRoot, skillName, version)
	skillsDir := filepath.Join(workDir, ".claude", "skills")
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		return err
	}
	return atomicSymlink(target, filepath.Join(skillsDir, skillName))
}

// RemoveWorker removes the worker-scoped symlink for skillName.
func (lm *LinkManager) RemoveWorker(workDir, skillName string) error {
	return removeLink(filepath.Join(workDir, ".claude", "skills", skillName))
}

// removeLink removes a symlink, ignoring not-exist errors.
func removeLink(path string) error {
	err := os.Remove(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// RemoveAllWorkerLinks removes all openbee-managed symlinks from a worker's skill dir.
// Used during worker deletion to clean up without deleting the whole workdir.
func (lm *LinkManager) RemoveAllWorkerLinks(workDir string) error {
	skillsDir := filepath.Join(workDir, ".claude", "skills")
	entries, err := os.ReadDir(skillsDir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, e := range entries {
		link := filepath.Join(skillsDir, e.Name())
		if isManagedLink(link, lm.registryRoot) {
			if err := removeLink(link); err != nil {
				return err
			}
		}
	}
	return nil
}

// atomicSymlink creates a symlink from link → target, replacing any existing entry atomically.
func atomicSymlink(target, link string) error {
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		return err
	}
	tmp := link + ".tmp"
	_ = os.Remove(tmp)
	if err := os.Symlink(target, tmp); err != nil {
		return err
	}
	return os.Rename(tmp, link)
}

// isManagedLink returns true if link is a symlink whose target is under registryRoot.
func isManagedLink(link, registryRoot string) bool {
	target, err := os.Readlink(link)
	if err != nil {
		return false
	}
	return isManagedTarget(target, registryRoot)
}

// isManagedTarget returns true if a resolved symlink target is under registryRoot.
// Use this when the target has already been read via os.Readlink to avoid a second syscall.
func isManagedTarget(target, registryRoot string) bool {
	rel, err := filepath.Rel(registryRoot, target)
	if err != nil {
		return false
	}
	return len(rel) > 0 && rel[0] != '.'
}
