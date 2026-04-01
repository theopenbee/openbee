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

func installSkill(baseDir string, skill skillDef) (SkillResult, error) {
	targetPath := filepath.Join(baseDir, skill.name, "SKILL.md")
	newContent := []byte(skill.content)

	action := ActionInstalled
	existing, err := os.ReadFile(targetPath)
	if err == nil {
		if sha256.Sum256(existing) == sha256.Sum256(newContent) {
			return SkillResult{Name: skill.name, Action: ActionUpToDate}, nil
		}
		action = ActionUpdated
	} else if !errors.Is(err, os.ErrNotExist) {
		return SkillResult{}, fmt.Errorf("read skill %s: %w", skill.name, err)
	} else {
		if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
			return SkillResult{}, fmt.Errorf("create skill dir %s: %w", skill.name, err)
		}
	}

	if err := os.WriteFile(targetPath, newContent, 0o644); err != nil {
		return SkillResult{}, fmt.Errorf("%s skill %s: %w", action, skill.name, err)
	}
	return SkillResult{Name: skill.name, Action: action}, nil
}
