package skillinstall

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

type Action string

const (
	ActionInstalled Action = "installed"
	ActionUpdated   Action = "updated"
	ActionUpToDate  Action = "up-to-date"
)

// SkillResult holds the install outcome for one skill.
type SkillResult struct {
	Name   string
	Action Action
}

// InstallSkills installs embedded skills to baseDir.
// Pass "" to use the default ~/.claude/skills.
// Returns per-skill results. A non-nil error means a write failed.
func InstallSkills(baseDir string) ([]SkillResult, error) {
	if baseDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("resolve home dir: %w", err)
		}
		baseDir = filepath.Join(home, ".claude", "skills")
	}

	var results []SkillResult
	for _, name := range embeddedSkills {
		result, err := installSkill(baseDir, name)
		if err != nil {
			return results, err
		}
		results = append(results, result)
	}
	return results, nil
}

// InstallSkillsToDefaults installs embedded skills to all default locations:
//
//	~/.claude/skills  (Claude Code)
//	~/.agents/skills  (agents runtime)
func InstallSkillsToDefaults() ([]SkillResult, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolve home dir: %w", err)
	}
	dirs := []string{
		filepath.Join(home, ".claude", "skills"),
		filepath.Join(home, ".agents", "skills"),
	}
	var allResults []SkillResult
	for _, dir := range dirs {
		results, err := InstallSkills(dir)
		allResults = append(allResults, results...)
		if err != nil {
			return allResults, err
		}
	}
	return allResults, nil
}

func installSkill(baseDir string, name string) (SkillResult, error) {
	skillDir := filepath.Join(baseDir, name)
	fsRoot := "skills/" + name

	// Determine whether this is a first install.
	_, err := os.Stat(skillDir)
	firstInstall := errors.Is(err, os.ErrNotExist)

	// Collect all files embedded under this skill (relPath -> content).
	embeddedFiles, err := collectEmbeddedFiles(fsRoot)
	if err != nil {
		return SkillResult{}, fmt.Errorf("read embedded skill %s: %w", name, err)
	}

	// Collect all files currently on disk under skillDir (relPath -> exists).
	diskFiles, err := collectDiskFiles(skillDir)
	if err != nil {
		return SkillResult{}, fmt.Errorf("scan skill dir %s: %w", name, err)
	}

	changed := false

	// Write or update files from embedded FS.
	for relPath, content := range embeddedFiles {
		target := filepath.Join(skillDir, filepath.FromSlash(relPath))
		if diskHashMatches(target, content) {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return SkillResult{}, fmt.Errorf("create dir for %s/%s: %w", name, relPath, err)
		}
		if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
			return SkillResult{}, fmt.Errorf("write %s/%s: %w", name, relPath, err)
		}
		changed = true
	}

	// Remove disk files not present in the embedded FS.
	for relPath := range diskFiles {
		if _, ok := embeddedFiles[relPath]; !ok {
			target := filepath.Join(skillDir, filepath.FromSlash(relPath))
			if err := os.Remove(target); err != nil && !errors.Is(err, os.ErrNotExist) {
				return SkillResult{}, fmt.Errorf("remove stale %s/%s: %w", name, relPath, err)
			}
			changed = true
		}
	}

	action := actionFor(firstInstall, changed)
	return SkillResult{Name: name, Action: action}, nil
}

// collectEmbeddedFiles walks the embedded FS under root and returns
// a map of slash-separated relative paths to file contents.
func collectEmbeddedFiles(root string) (map[string]string, error) {
	files := make(map[string]string)
	err := fs.WalkDir(skillsFS, root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := skillsFS.ReadFile(path)
		if err != nil {
			return err
		}
		// relPath is relative to the skill root, using forward slashes.
		relPath := path[len(root)+1:]
		files[relPath] = string(data)
		return nil
	})
	return files, err
}

// collectDiskFiles walks skillDir and returns a set of slash-separated
// relative file paths. Returns an empty map if the directory does not exist.
func collectDiskFiles(skillDir string) (map[string]struct{}, error) {
	files := make(map[string]struct{})
	err := filepath.WalkDir(skillDir, func(path string, d fs.DirEntry, err error) error {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(skillDir, path)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(rel)] = struct{}{}
		return nil
	})
	if errors.Is(err, os.ErrNotExist) {
		return files, nil
	}
	return files, err
}

// diskHashMatches reports whether the file at path has the same SHA-256 as content.
func diskHashMatches(path string, content string) bool {
	existing, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return sha256.Sum256(existing) == sha256.Sum256([]byte(content))
}

func actionFor(firstInstall, changed bool) Action {
	if firstInstall {
		return ActionInstalled
	}
	if changed {
		return ActionUpdated
	}
	return ActionUpToDate
}
