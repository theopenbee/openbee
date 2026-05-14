package bridge_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBusinessPackagesDoNotImportInternalAI(t *testing.T) {
	root := repoRoot(t)
	var offenders []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", ".worktrees", "data", "node_modules", "web":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if allowedAIImport(rel) {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(b), `github.com/theopenbee/openbee/internal/ai`) {
			offenders = append(offenders, rel)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(offenders) > 0 {
		t.Fatalf("business packages must import internal/bridge instead of internal/ai: %s", strings.Join(offenders, ", "))
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		next := filepath.Dir(dir)
		if next == dir {
			t.Fatal("go.mod not found")
		}
		dir = next
	}
}

func allowedAIImport(rel string) bool {
	return strings.HasPrefix(rel, "internal/ai/") ||
		strings.HasPrefix(rel, "internal/bridge/")
}
