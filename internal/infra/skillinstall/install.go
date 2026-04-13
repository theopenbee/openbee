package skillinstall

import (
	"crypto/sha256"
	"errors"
	"fmt"
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
	for _, skill := range embeddedSkills {
		result, err := installSkill(baseDir, skill)
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

func installSkill(baseDir string, skill skillDef) (SkillResult, error) {
	skillDir := filepath.Join(baseDir, skill.name)
	targetPath := filepath.Join(skillDir, "SKILL.md")
	newContent := []byte(skill.content)
	newHash := sha256.Sum256(newContent)

	action := ActionInstalled
	existing, err := os.ReadFile(targetPath)
	if err == nil {
		if sha256.Sum256(existing) == newHash && !referencesChanged(skillDir, skill.references) {
			return SkillResult{Name: skill.name, Action: ActionUpToDate}, nil
		}
		action = ActionUpdated
	} else if !errors.Is(err, os.ErrNotExist) {
		return SkillResult{}, fmt.Errorf("read skill %s: %w", skill.name, err)
	}

	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		return SkillResult{}, fmt.Errorf("create skill dir %s: %w", skill.name, err)
	}

	if err := os.WriteFile(targetPath, newContent, 0o644); err != nil {
		return SkillResult{}, fmt.Errorf("%s skill %s: %w", action, skill.name, err)
	}

	if len(skill.references) > 0 {
		refDir := filepath.Join(skillDir, "references")
		if err := os.MkdirAll(refDir, 0o755); err != nil {
			return SkillResult{}, fmt.Errorf("create references dir %s: %w", skill.name, err)
		}
		for name, content := range skill.references {
			refPath := filepath.Join(refDir, name)
			if err := os.WriteFile(refPath, []byte(content), 0o644); err != nil {
				return SkillResult{}, fmt.Errorf("write reference %s/%s: %w", skill.name, name, err)
			}
		}
	}

	return SkillResult{Name: skill.name, Action: action}, nil
}

// referencesChanged reports whether any reference file is missing or has different content.
func referencesChanged(skillDir string, references map[string]string) bool {
	for name, content := range references {
		refPath := filepath.Join(skillDir, "references", name)
		existing, err := os.ReadFile(refPath)
		if err != nil || sha256.Sum256(existing) != sha256.Sum256([]byte(content)) {
			return true
		}
	}
	return false
}
