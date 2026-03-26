package skill

import (
	"os"
	"path/filepath"
	"strings"
)

// Scanner classifies skills found in Claude Code skill directories.
type Scanner struct {
	registryRoot    string
	globalSkillsDir string
}

// NewScanner returns a Scanner.
// registryRoot is where versioned skill dirs live (e.g. ~/.openbee/skills).
// globalSkillsDir is ~/.claude/skills.
func NewScanner(registryRoot, globalSkillsDir string) *Scanner {
	// Absolutize paths defensively
	if abs, err := filepath.Abs(registryRoot); err == nil {
		registryRoot = abs
	}
	if abs, err := filepath.Abs(globalSkillsDir); err == nil {
		globalSkillsDir = abs
	}
	return &Scanner{registryRoot: registryRoot, globalSkillsDir: globalSkillsDir}
}

// ScanGlobal scans ~/.claude/skills and classifies each entry.
func (s *Scanner) ScanGlobal() ([]ScannedSkill, error) {
	return s.scanDir("global", s.globalSkillsDir)
}

// ScanWorker scans {workDir}/.claude/skills and classifies each entry.
// workerID is stored in ScannedSkill.Scope.
func (s *Scanner) ScanWorker(workerID, workDir string) ([]ScannedSkill, error) {
	return s.scanDir(workerID, filepath.Join(workDir, ".claude", "skills"))
}

func (s *Scanner) scanDir(scope, dir string) ([]ScannedSkill, error) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var results []ScannedSkill
	for _, e := range entries {
		link := filepath.Join(dir, e.Name())
		sk := ScannedSkill{Name: e.Name(), Scope: scope}

		target, linkErr := os.Readlink(link)
		if linkErr != nil {
			// Not a symlink — real directory.
			sk.Source = SkillSourceExternal
		} else {
			sk.LinkTarget = target
			if isManagedLink(link, s.registryRoot) {
				sk.Source = SkillSourceManaged
				// Extract version from target path: <registry>/<name>/<version>
				rel, _ := filepath.Rel(filepath.Join(s.registryRoot, e.Name()), target)
				parts := strings.Split(rel, string(filepath.Separator))
				if len(parts) == 1 {
					sk.ActiveVersion = parts[0]
				}
			} else {
				sk.Source = SkillSourceExternal
			}
		}
		results = append(results, sk)
	}
	return results, nil
}
