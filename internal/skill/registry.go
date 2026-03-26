package skill

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// ErrLastVersion is returned when deleting the only remaining version of a skill.
var ErrLastVersion = errors.New("cannot delete the last version of a skill")

// Registry manages versioned skill directories under a root directory.
// Structure: <root>/<skill-name>/v{N}/SKILL.md
type Registry struct {
	root string
}

// NewRegistry returns a Registry rooted at dir (e.g. ~/.openbee/skills).
func NewRegistry(root string) *Registry {
	return &Registry{root: root}
}

// VersionPath returns the directory path for a specific skill version.
func (r *Registry) VersionPath(name, version string) string {
	return filepath.Join(r.root, name, version)
}

// CreateVersion creates the next version directory for name and writes content to SKILL.md.
// Returns the path of the new version directory.
func (r *Registry) CreateVersion(name, content string) (string, error) {
	next, err := r.nextVersionID(name)
	if err != nil {
		return "", fmt.Errorf("determine version: %w", err)
	}
	dir := r.VersionPath(name, next)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create version dir: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
		return "", fmt.Errorf("write SKILL.md: %w", err)
	}
	return dir, nil
}

// ReadVersion reads SKILL.md content for the given skill and version.
func (r *Registry) ReadVersion(name, version string) (string, error) {
	data, err := os.ReadFile(filepath.Join(r.VersionPath(name, version), "SKILL.md"))
	if err != nil {
		return "", fmt.Errorf("read version %s/%s: %w", name, version, err)
	}
	return string(data), nil
}

// ListVersions returns all version IDs for a skill in ascending order (v1, v2, …).
func (r *Registry) ListVersions(name string) ([]string, error) {
	entries, err := os.ReadDir(filepath.Join(r.root, name))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var versions []string
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), "v") {
			versions = append(versions, e.Name())
		}
	}
	// Sort versions numerically (v1, v2, ..., v10 instead of v1, v10, v2, ...)
	sort.Slice(versions, func(i, j int) bool {
		ni, _ := strconv.Atoi(strings.TrimPrefix(versions[i], "v"))
		nj, _ := strconv.Atoi(strings.TrimPrefix(versions[j], "v"))
		return ni < nj
	})
	return versions, nil
}

// DeleteVersion removes a specific version directory.
// Returns ErrLastVersion if it is the only version.
func (r *Registry) DeleteVersion(name, version string) error {
	versions, err := r.ListVersions(name)
	if err != nil {
		return err
	}
	if len(versions) <= 1 {
		return ErrLastVersion
	}
	// Check if the target version exists
	found := false
	for _, v := range versions {
		if v == version {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("version %q not found for skill %q", version, name)
	}
	return os.RemoveAll(r.VersionPath(name, version))
}

// DeleteSkill removes the entire skill directory including all versions.
func (r *Registry) DeleteSkill(name string) error {
	return os.RemoveAll(filepath.Join(r.root, name))
}

// nextVersionID returns "v1" for a new skill or "v{N+1}" for an existing one.
func (r *Registry) nextVersionID(name string) (string, error) {
	versions, err := r.ListVersions(name)
	if err != nil {
		return "", err
	}
	if len(versions) == 0 {
		return "v1", nil
	}
	// Find the highest version number.
	max := 0
	for _, v := range versions {
		n, err := strconv.Atoi(strings.TrimPrefix(v, "v"))
		if err == nil && n > max {
			max = n
		}
	}
	return fmt.Sprintf("v%d", max+1), nil
}
