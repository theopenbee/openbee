package skillinstall

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
)

// SkillResult holds the install outcome for one skill.
type SkillResult struct {
	Name   string
	Action string // "installed" | "updated" | "up-to-date"
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
	newHash := sha256.Sum256([]byte(skill.content))

	existing, err := os.ReadFile(targetPath)
	if err == nil {
		// File exists — compare hash
		existingHash := sha256.Sum256(existing)
		if newHash == existingHash {
			return SkillResult{Name: skill.name, Action: "up-to-date"}, nil
		}
		// Content differs — overwrite
		if err := os.WriteFile(targetPath, []byte(skill.content), 0o644); err != nil {
			return SkillResult{}, fmt.Errorf("update skill %s: %w", skill.name, err)
		}
		return SkillResult{Name: skill.name, Action: "updated"}, nil
	}

	if !os.IsNotExist(err) {
		return SkillResult{}, fmt.Errorf("read skill %s: %w", skill.name, err)
	}

	// File does not exist — create
	skillDir := filepath.Dir(targetPath)
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		return SkillResult{}, fmt.Errorf("create skill dir %s: %w", skill.name, err)
	}
	if err := os.WriteFile(targetPath, []byte(skill.content), 0o644); err != nil {
		return SkillResult{}, fmt.Errorf("install skill %s: %w", skill.name, err)
	}
	return SkillResult{Name: skill.name, Action: "installed"}, nil
}
