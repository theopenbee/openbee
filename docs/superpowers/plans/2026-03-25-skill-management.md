# Skill Management Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add skill management to openbee — create, version, and manage Claude Code skills for global and per-worker use, exposed via CLI, REST API, and integrated with worker lifecycle.

**Architecture:** A new `internal/skill` package owns all skill logic: a JSON store (`~/.openbee/skills.json`) tracks versions and worker overrides; a versioned registry (`~/.openbee/skills/<name>/v{N}/`) holds real files; symlinks in `~/.claude/skills/` (global) and `{WorkDir}/.claude/skills/` (per-worker) point into the registry. External (non-openbee) skills are classified at scan time.

**Tech Stack:** Go stdlib (`os`, `path/filepath`, `encoding/json`), Cobra (CLI), Gin (HTTP), `stretchr/testify` (tests).

---

## File Map

| Action | Path | Responsibility |
|---|---|---|
| Create | `internal/skill/model.go` | Types: `SkillEntry`, `SkillsConfig`, `ScannedSkill`, `SkillSource` |
| Create | `internal/skill/store.go` | Read/write `~/.openbee/skills.json` with file lock |
| Create | `internal/skill/store_test.go` | Tests for store |
| Create | `internal/skill/registry.go` | Manage versioned dirs in `~/.openbee/skills/<name>/v{N}/` |
| Create | `internal/skill/registry_test.go` | Tests for registry |
| Create | `internal/skill/links.go` | Create/update/remove symlinks for global and worker skill dirs |
| Create | `internal/skill/links_test.go` | Tests for link management |
| Create | `internal/skill/scanner.go` | Scan and classify skills (managed vs external) |
| Create | `internal/skill/scanner_test.go` | Tests for scanner |
| Create | `internal/skill/manager.go` | Orchestrate: Create, Edit, Delete, Use, Adopt, List, Versions |
| Create | `internal/skill/manager_test.go` | Integration tests for manager |
| Create | `internal/api/skill_handler.go` | HTTP handlers for skill endpoints |
| Modify | `internal/api/router.go` | Add `SkillManager` to `ServerParams`, register skill routes |
| Create | `cmd/openbee/skill.go` | CLI: `openbee skill list/create/edit/delete/versions/use/adopt` |
| Modify | `internal/worker/manager.go` | Clean up worker skill symlinks on `DeleteWorker` |

---

## Task 1: Core Types

**Files:**
- Create: `internal/skill/model.go`

- [ ] **Step 1: Write the file**

```go
package skill

import "time"

// SkillSource identifies whether a skill is openbee-managed or externally placed.
type SkillSource string

const (
    SkillSourceManaged  SkillSource = "managed"
    SkillSourceExternal SkillSource = "external"
)

// VersionEntry holds metadata for one version of a skill.
type VersionEntry struct {
    CreatedAt time.Time `json:"created_at"`
}

// SkillEntry is the registry record for one managed skill.
type SkillEntry struct {
    Description   string                  `json:"description"`
    LatestVersion string                  `json:"latest_version"`
    GlobalVersion string                  `json:"global_version"`
    Versions      map[string]VersionEntry `json:"versions"`
}

// SkillsConfig is the full contents of skills.json.
type SkillsConfig struct {
    Version         int                          `json:"version"`
    Skills          map[string]SkillEntry        `json:"skills"`
    // WorkerOverrides maps workerID -> skillName -> version.
    // Absent entry means the worker inherits the global version.
    WorkerOverrides map[string]map[string]string `json:"worker_overrides"`
}

// ScannedSkill represents one skill found during a directory scan.
type ScannedSkill struct {
    Name       string
    Source     SkillSource
    ActiveVersion string  // set for managed skills; empty for external
    IsOverride bool       // true when a worker skill overrides a global one
    Scope      string     // "global" or worker ID
    LinkTarget string     // resolved symlink target (empty if real directory)
}
```

- [ ] **Step 2: Confirm it compiles**

```bash
cd /path/to/openbee && go build ./internal/skill/...
```

Expected: no output (no errors).

- [ ] **Step 3: Commit**

```bash
git add internal/skill/model.go
git commit -m "feat(skill): add core model types"
```

---

## Task 2: Skills JSON Store

**Files:**
- Create: `internal/skill/store.go`
- Create: `internal/skill/store_test.go`

- [ ] **Step 1: Write failing test**

```go
// internal/skill/store_test.go
package skill_test

import (
    "os"
    "path/filepath"
    "testing"
    "time"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
    "github.com/theopenbee/openbee/internal/skill"
)

func TestStore_LoadEmpty(t *testing.T) {
    dir := t.TempDir()
    s := skill.NewStore(filepath.Join(dir, "skills.json"))
    cfg, err := s.Load()
    require.NoError(t, err)
    assert.Equal(t, 1, cfg.Version)
    assert.NotNil(t, cfg.Skills)
    assert.NotNil(t, cfg.WorkerOverrides)
}

func TestStore_SaveLoad(t *testing.T) {
    dir := t.TempDir()
    s := skill.NewStore(filepath.Join(dir, "skills.json"))

    cfg := skill.SkillsConfig{
        Version: 1,
        Skills: map[string]skill.SkillEntry{
            "brainstorming": {
                Description:   "Brainstorm",
                LatestVersion: "v1",
                GlobalVersion: "v1",
                Versions: map[string]skill.VersionEntry{
                    "v1": {CreatedAt: time.Now().UTC().Truncate(time.Second)},
                },
            },
        },
        WorkerOverrides: map[string]map[string]string{},
    }

    require.NoError(t, s.Save(cfg))

    loaded, err := s.Load()
    require.NoError(t, err)
    assert.Equal(t, "Brainstorm", loaded.Skills["brainstorming"].Description)
    assert.Equal(t, "v1", loaded.Skills["brainstorming"].GlobalVersion)
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/skill/... -run TestStore -v
```

Expected: FAIL with "undefined: skill.NewStore".

- [ ] **Step 3: Implement store.go**

```go
// internal/skill/store.go
package skill

import (
    "encoding/json"
    "os"
    "path/filepath"
)

// Store reads and writes the skills.json state file.
type Store struct {
    path string
}

// NewStore returns a Store for the given JSON file path.
func NewStore(path string) *Store {
    return &Store{path: path}
}

// Load reads the config from disk. Returns an empty config if the file does not exist.
func (s *Store) Load() (SkillsConfig, error) {
    data, err := os.ReadFile(s.path)
    if os.IsNotExist(err) {
        return SkillsConfig{
            Version:         1,
            Skills:          make(map[string]SkillEntry),
            WorkerOverrides: make(map[string]map[string]string),
        }, nil
    }
    if err != nil {
        return SkillsConfig{}, err
    }

    var cfg SkillsConfig
    if err := json.Unmarshal(data, &cfg); err != nil {
        return SkillsConfig{}, err
    }
    if cfg.Skills == nil {
        cfg.Skills = make(map[string]SkillEntry)
    }
    if cfg.WorkerOverrides == nil {
        cfg.WorkerOverrides = make(map[string]map[string]string)
    }
    return cfg, nil
}

// Save writes the config atomically (write to temp file, then rename).
func (s *Store) Save(cfg SkillsConfig) error {
    if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
        return err
    }
    data, err := json.MarshalIndent(cfg, "", "  ")
    if err != nil {
        return err
    }
    tmp := s.path + ".tmp"
    if err := os.WriteFile(tmp, data, 0o644); err != nil {
        return err
    }
    return os.Rename(tmp, s.path)
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/skill/... -run TestStore -v
```

Expected: PASS for both TestStore_LoadEmpty and TestStore_SaveLoad.

- [ ] **Step 5: Commit**

```bash
git add internal/skill/store.go internal/skill/store_test.go
git commit -m "feat(skill): add skills.json store"
```

---

## Task 3: Versioned Registry

**Files:**
- Create: `internal/skill/registry.go`
- Create: `internal/skill/registry_test.go`

- [ ] **Step 1: Write failing tests**

```go
// internal/skill/registry_test.go
package skill_test

import (
    "os"
    "path/filepath"
    "testing"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
    "github.com/theopenbee/openbee/internal/skill"
)

func TestRegistry_CreateAndRead(t *testing.T) {
    dir := t.TempDir()
    r := skill.NewRegistry(dir)

    content := "---\nname: test\n---\nHello"
    versionDir, err := r.CreateVersion("myskill", content)
    require.NoError(t, err)
    assert.DirExists(t, versionDir)

    data, err := os.ReadFile(filepath.Join(versionDir, "SKILL.md"))
    require.NoError(t, err)
    assert.Equal(t, content, string(data))
}

func TestRegistry_VersionSequence(t *testing.T) {
    dir := t.TempDir()
    r := skill.NewRegistry(dir)

    v1, err := r.CreateVersion("seq", "content v1")
    require.NoError(t, err)
    assert.Contains(t, v1, "v1")

    v2, err := r.CreateVersion("seq", "content v2")
    require.NoError(t, err)
    assert.Contains(t, v2, "v2")
}

func TestRegistry_ReadVersion(t *testing.T) {
    dir := t.TempDir()
    r := skill.NewRegistry(dir)

    _, err := r.CreateVersion("skill1", "initial content")
    require.NoError(t, err)

    content, err := r.ReadVersion("skill1", "v1")
    require.NoError(t, err)
    assert.Equal(t, "initial content", content)
}

func TestRegistry_VersionPath(t *testing.T) {
    dir := t.TempDir()
    r := skill.NewRegistry(dir)

    path := r.VersionPath("myskill", "v2")
    assert.Equal(t, filepath.Join(dir, "myskill", "v2"), path)
}

func TestRegistry_DeleteVersion_RefusedWhenLast(t *testing.T) {
    dir := t.TempDir()
    r := skill.NewRegistry(dir)

    _, err := r.CreateVersion("solo", "only version")
    require.NoError(t, err)

    err = r.DeleteVersion("solo", "v1")
    assert.ErrorIs(t, err, skill.ErrLastVersion)
}

func TestRegistry_DeleteSkill(t *testing.T) {
    dir := t.TempDir()
    r := skill.NewRegistry(dir)

    _, err := r.CreateVersion("todelete", "content")
    require.NoError(t, err)

    require.NoError(t, r.DeleteSkill("todelete"))
    assert.NoDirExists(t, filepath.Join(dir, "todelete"))
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/skill/... -run TestRegistry -v
```

Expected: FAIL with "undefined: skill.NewRegistry".

- [ ] **Step 3: Implement registry.go**

```go
// internal/skill/registry.go
package skill

import (
    "errors"
    "fmt"
    "os"
    "path/filepath"
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
        return "", err
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
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/skill/... -run TestRegistry -v
```

Expected: all 6 registry tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/skill/registry.go internal/skill/registry_test.go
git commit -m "feat(skill): add versioned registry"
```

---

## Task 4: Symlink Manager

**Files:**
- Create: `internal/skill/links.go`
- Create: `internal/skill/links_test.go`

- [ ] **Step 1: Write failing tests**

```go
// internal/skill/links_test.go
package skill_test

import (
    "os"
    "path/filepath"
    "testing"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
    "github.com/theopenbee/openbee/internal/skill"
)

func TestLinks_SetGlobal(t *testing.T) {
    registryDir := t.TempDir()
    globalSkillsDir := t.TempDir()

    // Create a fake version directory in registry.
    versionDir := filepath.Join(registryDir, "brainstorming", "v1")
    require.NoError(t, os.MkdirAll(versionDir, 0o755))

    lm := skill.NewLinkManager(registryDir, globalSkillsDir)
    require.NoError(t, lm.SetGlobal("brainstorming", "v1"))

    link := filepath.Join(globalSkillsDir, "brainstorming")
    target, err := os.Readlink(link)
    require.NoError(t, err)
    assert.Equal(t, versionDir, target)
}

func TestLinks_SetWorker(t *testing.T) {
    registryDir := t.TempDir()
    workDir := t.TempDir()
    globalSkillsDir := t.TempDir()

    versionDir := filepath.Join(registryDir, "commit", "v2")
    require.NoError(t, os.MkdirAll(versionDir, 0o755))

    lm := skill.NewLinkManager(registryDir, globalSkillsDir)
    require.NoError(t, lm.SetWorker(workDir, "commit", "v2"))

    link := filepath.Join(workDir, ".claude", "skills", "commit")
    target, err := os.Readlink(link)
    require.NoError(t, err)
    assert.Equal(t, versionDir, target)
}

func TestLinks_RemoveGlobal(t *testing.T) {
    registryDir := t.TempDir()
    globalSkillsDir := t.TempDir()

    versionDir := filepath.Join(registryDir, "myskill", "v1")
    require.NoError(t, os.MkdirAll(versionDir, 0o755))

    lm := skill.NewLinkManager(registryDir, globalSkillsDir)
    require.NoError(t, lm.SetGlobal("myskill", "v1"))
    require.NoError(t, lm.RemoveGlobal("myskill"))

    assert.NoFileExists(t, filepath.Join(globalSkillsDir, "myskill"))
}

func TestLinks_RemoveWorker(t *testing.T) {
    registryDir := t.TempDir()
    workDir := t.TempDir()
    globalSkillsDir := t.TempDir()

    versionDir := filepath.Join(registryDir, "myskill", "v1")
    require.NoError(t, os.MkdirAll(versionDir, 0o755))

    lm := skill.NewLinkManager(registryDir, globalSkillsDir)
    require.NoError(t, lm.SetWorker(workDir, "myskill", "v1"))
    require.NoError(t, lm.RemoveWorker(workDir, "myskill"))

    assert.NoFileExists(t, filepath.Join(workDir, ".claude", "skills", "myskill"))
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/skill/... -run TestLinks -v
```

Expected: FAIL with "undefined: skill.NewLinkManager".

- [ ] **Step 3: Implement links.go**

```go
// internal/skill/links.go
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
    return os.Remove(filepath.Join(lm.globalSkillsDir, skillName))
}

// SetWorker creates or updates the worker-scoped symlink for skillName.
// workDir is the worker's working directory (the one stored in model.Worker.WorkDir).
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
    return os.Remove(filepath.Join(workDir, ".claude", "skills", skillName))
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
            if err := os.Remove(link); err != nil && !os.IsNotExist(err) {
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
    // Remove stale tmp if any.
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
    rel, err := filepath.Rel(registryRoot, target)
    if err != nil {
        return false
    }
    return len(rel) > 0 && rel[0] != '.'
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/skill/... -run TestLinks -v
```

Expected: all 4 link tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/skill/links.go internal/skill/links_test.go
git commit -m "feat(skill): add symlink manager"
```

---

## Task 5: Scanner

**Files:**
- Create: `internal/skill/scanner.go`
- Create: `internal/skill/scanner_test.go`

- [ ] **Step 1: Write failing tests**

```go
// internal/skill/scanner_test.go
package skill_test

import (
    "os"
    "path/filepath"
    "testing"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
    "github.com/theopenbee/openbee/internal/skill"
)

func TestScanner_ManagedGlobal(t *testing.T) {
    registryDir := t.TempDir()
    globalSkillsDir := t.TempDir()

    // Create registry version.
    vDir := filepath.Join(registryDir, "brainstorm", "v1")
    require.NoError(t, os.MkdirAll(vDir, 0o755))

    // Create managed symlink.
    lm := skill.NewLinkManager(registryDir, globalSkillsDir)
    require.NoError(t, lm.SetGlobal("brainstorm", "v1"))

    sc := skill.NewScanner(registryDir, globalSkillsDir)
    results, err := sc.ScanGlobal()
    require.NoError(t, err)

    require.Len(t, results, 1)
    assert.Equal(t, "brainstorm", results[0].Name)
    assert.Equal(t, skill.SkillSourceManaged, results[0].Source)
    assert.Equal(t, "v1", results[0].ActiveVersion)
}

func TestScanner_ExternalGlobal(t *testing.T) {
    registryDir := t.TempDir()
    globalSkillsDir := t.TempDir()

    // Place a real (non-symlink) skill directory.
    extSkill := filepath.Join(globalSkillsDir, "external-skill")
    require.NoError(t, os.MkdirAll(extSkill, 0o755))
    require.NoError(t, os.WriteFile(filepath.Join(extSkill, "SKILL.md"), []byte("---\n---\nhello"), 0o644))

    sc := skill.NewScanner(registryDir, globalSkillsDir)
    results, err := sc.ScanGlobal()
    require.NoError(t, err)

    require.Len(t, results, 1)
    assert.Equal(t, "external-skill", results[0].Name)
    assert.Equal(t, skill.SkillSourceExternal, results[0].Source)
    assert.Empty(t, results[0].ActiveVersion)
}

func TestScanner_WorkerSkill(t *testing.T) {
    registryDir := t.TempDir()
    globalSkillsDir := t.TempDir()
    workDir := t.TempDir()

    vDir := filepath.Join(registryDir, "commit", "v2")
    require.NoError(t, os.MkdirAll(vDir, 0o755))

    lm := skill.NewLinkManager(registryDir, globalSkillsDir)
    require.NoError(t, lm.SetWorker(workDir, "commit", "v2"))

    sc := skill.NewScanner(registryDir, globalSkillsDir)
    results, err := sc.ScanWorker("worker1", workDir)
    require.NoError(t, err)

    require.Len(t, results, 1)
    assert.Equal(t, "commit", results[0].Name)
    assert.Equal(t, skill.SkillSourceManaged, results[0].Source)
    assert.Equal(t, "v2", results[0].ActiveVersion)
    assert.Equal(t, "worker1", results[0].Scope)
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/skill/... -run TestScanner -v
```

Expected: FAIL with "undefined: skill.NewScanner".

- [ ] **Step 3: Implement scanner.go**

```go
// internal/skill/scanner.go
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
func NewScanner(registryRoot, globalSkillsDir string) *Scanner {
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
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/skill/... -run TestScanner -v
```

Expected: all 3 scanner tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/skill/scanner.go internal/skill/scanner_test.go
git commit -m "feat(skill): add skill scanner"
```

---

## Task 6: Manager (Orchestrator)

**Files:**
- Create: `internal/skill/manager.go`
- Create: `internal/skill/manager_test.go`

- [ ] **Step 1: Write failing tests**

```go
// internal/skill/manager_test.go
package skill_test

import (
    "os"
    "path/filepath"
    "testing"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
    "github.com/theopenbee/openbee/internal/skill"
)

func newTestManager(t *testing.T) (*skill.Manager, string, string) {
    t.Helper()
    stateDir := t.TempDir()
    globalSkillsDir := t.TempDir()
    m := skill.NewManager(stateDir, globalSkillsDir)
    return m, stateDir, globalSkillsDir
}

func TestManager_CreateAndList(t *testing.T) {
    m, _, globalDir := newTestManager(t)

    err := m.Create("brainstorm", "Brainstorm discussions", "---\nname: brainstorm\n---\nThink!")
    require.NoError(t, err)

    // Global symlink should exist.
    link := filepath.Join(globalDir, "brainstorm")
    _, err = os.Lstat(link)
    require.NoError(t, err)

    skills, err := m.List()
    require.NoError(t, err)
    require.Len(t, skills, 1)
    assert.Equal(t, "brainstorm", skills[0].Name)
    assert.Equal(t, "v1", skills[0].ActiveVersion)
    assert.Equal(t, skill.SkillSourceManaged, skills[0].Source)
}

func TestManager_EditCreatesNewVersion(t *testing.T) {
    m, stateDir, _ := newTestManager(t)
    require.NoError(t, m.Create("commit", "Commit", "v1 content"))

    require.NoError(t, m.Edit("commit", "v2 content"))

    // v2 should exist in registry.
    v2Dir := filepath.Join(stateDir, "skills", "commit", "v2")
    assert.DirExists(t, v2Dir)

    // Global version should still be v1 (edit ≠ publish).
    cfg, err := m.LoadConfig()
    require.NoError(t, err)
    assert.Equal(t, "v1", cfg.Skills["commit"].GlobalVersion)
    assert.Equal(t, "v2", cfg.Skills["commit"].LatestVersion)
}

func TestManager_UseGlobal(t *testing.T) {
    m, _, globalDir := newTestManager(t)
    require.NoError(t, m.Create("deploy", "Deploy", "deploy v1"))
    require.NoError(t, m.Edit("deploy", "deploy v2"))

    require.NoError(t, m.UseGlobal("deploy", "v2"))

    target, err := os.Readlink(filepath.Join(globalDir, "deploy"))
    require.NoError(t, err)
    assert.Contains(t, target, "v2")

    cfg, err := m.LoadConfig()
    require.NoError(t, err)
    assert.Equal(t, "v2", cfg.Skills["deploy"].GlobalVersion)
}

func TestManager_UseWorker(t *testing.T) {
    m, _, _ := newTestManager(t)
    workDir := t.TempDir()

    require.NoError(t, m.Create("review", "Review", "review v1"))
    require.NoError(t, m.Edit("review", "review v2"))
    require.NoError(t, m.UseWorker("worker1", workDir, "review", "v1"))

    link := filepath.Join(workDir, ".claude", "skills", "review")
    target, err := os.Readlink(link)
    require.NoError(t, err)
    assert.Contains(t, target, "v1")
}

func TestManager_DeleteRefusedWithReferences(t *testing.T) {
    m, _, _ := newTestManager(t)
    workDir := t.TempDir()

    require.NoError(t, m.Create("shared", "Shared", "content"))
    require.NoError(t, m.UseWorker("worker1", workDir, "shared", "v1"))

    err := m.Delete("shared")
    assert.Error(t, err) // must refuse because worker1 references it
}

func TestManager_DeleteSucceeds(t *testing.T) {
    m, _, globalDir := newTestManager(t)
    require.NoError(t, m.Create("temp", "Temp", "content"))

    require.NoError(t, m.Delete("temp"))

    assert.NoFileExists(t, filepath.Join(globalDir, "temp"))
}

func TestManager_Adopt(t *testing.T) {
    m, stateDir, globalDir := newTestManager(t)

    // Simulate an externally-placed skill.
    extSkill := filepath.Join(globalDir, "legacy")
    require.NoError(t, os.MkdirAll(extSkill, 0o755))
    require.NoError(t, os.WriteFile(filepath.Join(extSkill, "SKILL.md"), []byte("legacy content"), 0o644))

    require.NoError(t, m.AdoptGlobal("legacy"))

    // Original path should now be a symlink.
    target, err := os.Readlink(filepath.Join(globalDir, "legacy"))
    require.NoError(t, err)
    assert.Contains(t, target, "v1")

    // Registry should have the content.
    v1Dir := filepath.Join(stateDir, "skills", "legacy", "v1")
    assert.DirExists(t, v1Dir)
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/skill/... -run TestManager -v
```

Expected: FAIL with "undefined: skill.NewManager".

- [ ] **Step 3: Implement manager.go**

```go
// internal/skill/manager.go
package skill

import (
    "errors"
    "fmt"
    "os"
    "path/filepath"
)

// Manager orchestrates all skill operations.
type Manager struct {
    store    *Store
    registry *Registry
    links    *LinkManager
    scanner  *Scanner
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
func (m *Manager) Edit(name, content string) error {
    cfg, err := m.store.Load()
    if err != nil {
        return err
    }
    entry, ok := cfg.Skills[name]
    if !ok {
        return fmt.Errorf("skill %q not found", name)
    }

    versionDir, err := m.registry.CreateVersion(name, content)
    if err != nil {
        return fmt.Errorf("create version: %w", err)
    }
    version := filepath.Base(versionDir)

    entry.LatestVersion = version
    entry.Versions[version] = VersionEntry{CreatedAt: now()}
    cfg.Skills[name] = entry
    return m.store.Save(cfg)
}

// UseGlobal switches the global symlink to the given version.
func (m *Manager) UseGlobal(name, version string) error {
    cfg, err := m.store.Load()
    if err != nil {
        return err
    }
    entry, ok := cfg.Skills[name]
    if !ok {
        return fmt.Errorf("skill %q not found", name)
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
    cfg, err := m.store.Load()
    if err != nil {
        return err
    }
    entry, ok := cfg.Skills[name]
    if !ok {
        return fmt.Errorf("skill %q not found", name)
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
    cfg, err := m.store.Load()
    if err != nil {
        return err
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
        return nil, fmt.Errorf("skill %q not found", name)
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
    return m.adopt(name, m.links.globalSkillsDir, func(version string) error {
        return m.links.SetGlobal(name, version)
    }, func(cfg *SkillsConfig, entry SkillEntry) {
        cfg.Skills[name] = entry
    })
}

// AdoptWorker converts an externally-placed worker skill into an openbee-managed one.
func (m *Manager) AdoptWorker(workerID, workDir, name string) error {
    skillsDir := filepath.Join(workDir, ".claude", "skills")
    return m.adopt(name, skillsDir, func(version string) error {
        return m.links.SetWorker(workDir, name, version)
    }, func(cfg *SkillsConfig, entry SkillEntry) {
        cfg.Skills[name] = entry
        if cfg.WorkerOverrides[workerID] == nil {
            cfg.WorkerOverrides[workerID] = make(map[string]string)
        }
        cfg.WorkerOverrides[workerID][name] = version
    })
}

// CleanupWorkerLinks removes all openbee-managed symlinks from a worker's skill dir.
// Called during worker deletion when deleteWorkDir is false.
func (m *Manager) CleanupWorkerLinks(workerID, workDir string) error {
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

    // Remove the original and create managed symlink.
    if err := os.RemoveAll(target); err != nil {
        return fmt.Errorf("remove original: %w", err)
    }
    if err := setLink(version); err != nil {
        return fmt.Errorf("set link: %w", err)
    }

    // Update state.
    cfg, err := m.store.Load()
    if err != nil {
        return err
    }
    entry := newSkillEntry("", version, version)
    updateCfg(&cfg, entry)
    return m.store.Save(cfg)
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
```

Add the `now()` helper at the bottom of `model.go` (or a separate `helpers.go`):

```go
// Add to internal/skill/model.go
import "time"

func now() time.Time { return time.Now().UTC().Truncate(time.Second) }
```

- [ ] **Step 4: Run all skill tests**

```bash
go test ./internal/skill/... -v
```

Expected: all tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/skill/manager.go internal/skill/manager_test.go internal/skill/model.go
git commit -m "feat(skill): add skill manager"
```

---

## Task 7: CLI Command

**Files:**
- Create: `cmd/openbee/skill.go`

No unit tests for CLI (cobra commands are tested via integration); verify manually.

- [ ] **Step 1: Write skill.go**

```go
// cmd/openbee/skill.go
package main

import (
    "fmt"
    "os"
    "path/filepath"
    "text/tabwriter"

    "github.com/spf13/cobra"
    "github.com/theopenbee/openbee/internal/skill"
)

func globalSkillsDir() string {
    home, err := os.UserHomeDir()
    if err != nil {
        fmt.Fprintln(os.Stderr, "cannot determine home dir:", err)
        os.Exit(1)
    }
    return filepath.Join(home, ".claude", "skills")
}

func newSkillManager() *skill.Manager {
    return skill.NewManager(openbeeStateDir(), globalSkillsDir())
}

var skillCmd = &cobra.Command{
    Use:   "skill",
    Short: "Manage Claude Code skills",
}

var skillListCmd = &cobra.Command{
    Use:   "list",
    Short: "List all skills (global view)",
    RunE: func(cmd *cobra.Command, args []string) error {
        m := newSkillManager()
        skills, err := m.List()
        if err != nil {
            return err
        }
        w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
        fmt.Fprintln(w, "NAME\tSOURCE\tVERSION")
        for _, s := range skills {
            fmt.Fprintf(w, "%s\t%s\t%s\n", s.Name, s.Source, s.ActiveVersion)
        }
        return w.Flush()
    },
}

var skillCreateName string
var skillCreateDesc string
var skillCreateContent string

var skillCreateCmd = &cobra.Command{
    Use:   "create <name>",
    Short: "Create a new managed skill",
    Args:  cobra.ExactArgs(1),
    RunE: func(cmd *cobra.Command, args []string) error {
        name := args[0]
        content := skillCreateContent
        if content == "" {
            content = fmt.Sprintf("---\nname: %s\ndescription: %s\n---\n\n# %s\n", name, skillCreateDesc, name)
        }
        m := newSkillManager()
        if err := m.Create(name, skillCreateDesc, content); err != nil {
            return err
        }
        fmt.Printf("Skill %q created (v1).\n", name)
        return nil
    },
}

var skillEditCmd = &cobra.Command{
    Use:   "edit <name>",
    Short: "Edit skill content (opens $EDITOR, saves as new version)",
    Args:  cobra.ExactArgs(1),
    RunE: func(cmd *cobra.Command, args []string) error {
        name := args[0]
        m := newSkillManager()

        cfg, err := m.LoadConfig()
        if err != nil {
            return err
        }
        entry, ok := cfg.Skills[name]
        if !ok {
            return fmt.Errorf("skill %q not found", name)
        }

        // Read current latest version content.
        registryRoot := filepath.Join(openbeeStateDir(), "skills")
        current, err := os.ReadFile(filepath.Join(registryRoot, name, entry.LatestVersion, "SKILL.md"))
        if err != nil {
            return fmt.Errorf("read current content: %w", err)
        }

        newContent, err := openInEditor(current)
        if err != nil {
            return err
        }
        if string(newContent) == string(current) {
            fmt.Println("No changes made.")
            return nil
        }

        if err := m.Edit(name, string(newContent)); err != nil {
            return err
        }
        newCfg, _ := m.LoadConfig()
        fmt.Printf("Skill %q saved as %s. Global still uses %s.\n",
            name, newCfg.Skills[name].LatestVersion, newCfg.Skills[name].GlobalVersion)
        fmt.Printf("To promote: openbee skill use %s %s --global\n", name, newCfg.Skills[name].LatestVersion)
        return nil
    },
}

var skillDeleteCmd = &cobra.Command{
    Use:   "delete <name>",
    Short: "Delete a managed skill",
    Args:  cobra.ExactArgs(1),
    RunE: func(cmd *cobra.Command, args []string) error {
        m := newSkillManager()
        if err := m.Delete(args[0]); err != nil {
            return err
        }
        fmt.Printf("Skill %q deleted.\n", args[0])
        return nil
    },
}

var skillVersionsCmd = &cobra.Command{
    Use:   "versions <name>",
    Short: "List version history for a skill",
    Args:  cobra.ExactArgs(1),
    RunE: func(cmd *cobra.Command, args []string) error {
        m := newSkillManager()
        versions, err := m.Versions(args[0])
        if err != nil {
            return err
        }
        w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
        fmt.Fprintln(w, "VERSION\tCREATED AT")
        for v, e := range versions {
            fmt.Fprintf(w, "%s\t%s\n", v, e.CreatedAt.Format("2006-01-02 15:04:05"))
        }
        return w.Flush()
    },
}

var skillUseGlobal bool
var skillUseWorker string

var skillUseCmd = &cobra.Command{
    Use:   "use <name> <version>",
    Short: "Switch active version (default: global)",
    Args:  cobra.ExactArgs(2),
    RunE: func(cmd *cobra.Command, args []string) error {
        name, version := args[0], args[1]
        m := newSkillManager()
        if skillUseWorker != "" {
            // Need workDir — look up from DB config path.
            return fmt.Errorf("--worker requires running with a config file; use the API instead")
        }
        if err := m.UseGlobal(name, version); err != nil {
            return err
        }
        fmt.Printf("Global skill %q now uses %s.\n", name, version)
        return nil
    },
}

var skillAdoptGlobal bool

var skillAdoptCmd = &cobra.Command{
    Use:   "adopt <name>",
    Short: "Adopt an externally-placed skill into openbee management",
    Args:  cobra.ExactArgs(1),
    RunE: func(cmd *cobra.Command, args []string) error {
        m := newSkillManager()
        if err := m.AdoptGlobal(args[0]); err != nil {
            return err
        }
        fmt.Printf("Skill %q is now managed by openbee (v1).\n", args[0])
        return nil
    },
}

func init() {
    skillCreateCmd.Flags().StringVar(&skillCreateDesc, "description", "", "Skill description")
    skillCreateCmd.Flags().StringVar(&skillCreateContent, "content", "", "Initial SKILL.md content (default: generated template)")
    skillUseCmd.Flags().BoolVar(&skillUseGlobal, "global", true, "Switch global version (default)")
    skillUseCmd.Flags().StringVar(&skillUseWorker, "worker", "", "Worker ID for worker-scoped version switch")

    skillCmd.AddCommand(skillListCmd)
    skillCmd.AddCommand(skillCreateCmd)
    skillCmd.AddCommand(skillEditCmd)
    skillCmd.AddCommand(skillDeleteCmd)
    skillCmd.AddCommand(skillVersionsCmd)
    skillCmd.AddCommand(skillUseCmd)
    skillCmd.AddCommand(skillAdoptCmd)
    rootCmd.AddCommand(skillCmd)
}

// openInEditor writes content to a temp file, opens $EDITOR, and returns the modified content.
func openInEditor(content []byte) ([]byte, error) {
    f, err := os.CreateTemp("", "openbee-skill-*.md")
    if err != nil {
        return nil, err
    }
    defer os.Remove(f.Name())
    if _, err := f.Write(content); err != nil {
        f.Close()
        return nil, err
    }
    f.Close()

    editor := os.Getenv("EDITOR")
    if editor == "" {
        editor = "vi"
    }

    cmd := &cobra.Command{}
    _ = cmd // use exec directly
    // exec.Command is needed here — import "os/exec" at top of file.
    // Add: import "os/exec" to the import block.
    return runEditor(editor, f.Name())
}
```

Add a separate `skill_editor.go` to avoid circular imports:

```go
// cmd/openbee/skill_editor.go
package main

import (
    "os"
    "os/exec"
)

func runEditor(editor, path string) ([]byte, error) {
    cmd := exec.Command(editor, path) //nolint:gosec
    cmd.Stdin = os.Stdin
    cmd.Stdout = os.Stdout
    cmd.Stderr = os.Stderr
    if err := cmd.Run(); err != nil {
        return nil, err
    }
    return os.ReadFile(path)
}
```

- [ ] **Step 2: Build to verify compilation**

```bash
go build ./cmd/openbee/...
```

Expected: binary builds with no errors.

- [ ] **Step 3: Smoke test**

```bash
./openbee skill --help
./openbee skill create test-skill --description "Test"
./openbee skill list
./openbee skill versions test-skill
./openbee skill delete test-skill
./openbee skill list
```

Expected: commands run without errors; list shows then hides `test-skill`.

- [ ] **Step 4: Commit**

```bash
git add cmd/openbee/skill.go cmd/openbee/skill_editor.go
git commit -m "feat(skill): add CLI commands"
```

---

## Task 8: REST API Handler

**Files:**
- Create: `internal/api/skill_handler.go`
- Modify: `internal/api/router.go`

- [ ] **Step 1: Write skill_handler.go**

```go
// internal/api/skill_handler.go
package api

import (
    "net/http"

    "github.com/gin-gonic/gin"
    "github.com/theopenbee/openbee/internal/skill"
)

// listSkills GET /api/skills
func (s *Server) listSkills(c *gin.Context) {
    skills, err := s.SkillManager.List()
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    c.JSON(http.StatusOK, skills)
}

type createSkillRequest struct {
    Name        string `json:"name" binding:"required"`
    Description string `json:"description"`
    Content     string `json:"content" binding:"required"`
}

// createSkill POST /api/skills
func (s *Server) createSkill(c *gin.Context) {
    var req createSkillRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }
    if err := s.SkillManager.Create(req.Name, req.Description, req.Content); err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    c.JSON(http.StatusCreated, gin.H{"name": req.Name, "version": "v1"})
}

// getSkill GET /api/skills/:name
func (s *Server) getSkill(c *gin.Context) {
    name := c.Param("name")
    cfg, err := s.SkillManager.LoadConfig()
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    entry, ok := cfg.Skills[name]
    if !ok {
        c.JSON(http.StatusNotFound, gin.H{"error": "skill not found"})
        return
    }
    c.JSON(http.StatusOK, entry)
}

// deleteSkill DELETE /api/skills/:name
func (s *Server) deleteSkill(c *gin.Context) {
    if err := s.SkillManager.Delete(c.Param("name")); err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}

type createVersionRequest struct {
    Content string `json:"content" binding:"required"`
}

// createSkillVersion POST /api/skills/:name/versions
func (s *Server) createSkillVersion(c *gin.Context) {
    var req createVersionRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }
    if err := s.SkillManager.Edit(c.Param("name"), req.Content); err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    cfg, _ := s.SkillManager.LoadConfig()
    entry := cfg.Skills[c.Param("name")]
    c.JSON(http.StatusCreated, gin.H{"latest_version": entry.LatestVersion})
}

type setVersionRequest struct {
    Version string `json:"version" binding:"required"`
}

// setGlobalVersion PUT /api/skills/:name/global-version
func (s *Server) setGlobalVersion(c *gin.Context) {
    var req setVersionRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }
    if err := s.SkillManager.UseGlobal(c.Param("name"), req.Version); err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    c.JSON(http.StatusOK, gin.H{"global_version": req.Version})
}

// adoptSkill POST /api/skills/:name/adopt
func (s *Server) adoptSkill(c *gin.Context) {
    if err := s.SkillManager.AdoptGlobal(c.Param("name")); err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    c.JSON(http.StatusOK, gin.H{"status": "adopted", "version": "v1"})
}

// listWorkerSkills GET /api/workers/:id/skills
func (s *Server) listWorkerSkills(c *gin.Context) {
    workerID := c.Param("id")
    w, err := s.WorkerStore.GetByID(workerID)
    if err != nil {
        c.JSON(http.StatusNotFound, gin.H{"error": "worker not found"})
        return
    }
    skills, err := s.SkillManager.ListWorker(workerID, w.WorkDir)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    c.JSON(http.StatusOK, skills)
}

// setWorkerSkillVersion PUT /api/workers/:id/skills/:name
func (s *Server) setWorkerSkillVersion(c *gin.Context) {
    var req setVersionRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }
    workerID := c.Param("id")
    w, err := s.WorkerStore.GetByID(workerID)
    if err != nil {
        c.JSON(http.StatusNotFound, gin.H{"error": "worker not found"})
        return
    }
    if err := s.SkillManager.UseWorker(workerID, w.WorkDir, c.Param("name"), req.Version); err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    c.JSON(http.StatusOK, gin.H{"version": req.Version})
}

// deleteWorkerSkillOverride DELETE /api/workers/:id/skills/:name
func (s *Server) deleteWorkerSkillOverride(c *gin.Context) {
    workerID := c.Param("id")
    w, err := s.WorkerStore.GetByID(workerID)
    if err != nil {
        c.JSON(http.StatusNotFound, gin.H{"error": "worker not found"})
        return
    }
    if err := s.SkillManager.RemoveWorkerOverride(workerID, w.WorkDir, c.Param("name")); err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    c.JSON(http.StatusOK, gin.H{"status": "override removed"})
}
```

- [ ] **Step 2: Modify router.go — add SkillManager to ServerParams and register routes**

In `internal/api/router.go`, add to `ServerParams`:
```go
SkillManager *skill.Manager
```

Add the import: `"github.com/theopenbee/openbee/internal/skill"`

Add a new registration method and call it in `setupRoutes`:
```go
func (s *Server) registerSkillRoutes(api *gin.RouterGroup) {
    api.GET("/skills", s.listSkills)
    api.POST("/skills", s.createSkill)
    api.GET("/skills/:name", s.getSkill)
    api.DELETE("/skills/:name", s.deleteSkill)
    api.POST("/skills/:name/versions", s.createSkillVersion)
    api.PUT("/skills/:name/global-version", s.setGlobalVersion)
    api.POST("/skills/:name/adopt", s.adoptSkill)

    api.GET("/workers/:id/skills", s.listWorkerSkills)
    api.PUT("/workers/:id/skills/:name", s.setWorkerSkillVersion)
    api.DELETE("/workers/:id/skills/:name", s.deleteWorkerSkillOverride)
}
```

In `setupRoutes`, inside the `api` group, add:
```go
s.registerSkillRoutes(api)
```

- [ ] **Step 3: Wire SkillManager in server.go**

Find where `api.NewServer(p)` is called (in `cmd/openbee/server.go`). Add `SkillManager` initialization:

```go
// In the file that constructs ServerParams (server.go or daemon.go):
stateDir := openbeeStateDir()
home, _ := os.UserHomeDir()
globalSkillsDir := filepath.Join(home, ".claude", "skills")
skillMgr := skill.NewManager(stateDir, globalSkillsDir)

// Then in ServerParams:
SkillManager: skillMgr,
```

- [ ] **Step 4: Build to verify compilation**

```bash
go build ./...
```

Expected: no errors.

- [ ] **Step 5: Commit**

```bash
git add internal/api/skill_handler.go internal/api/router.go cmd/openbee/server.go
git commit -m "feat(skill): add REST API handlers"
```

---

## Task 9: Worker Teardown Integration

**Files:**
- Modify: `internal/worker/manager.go`

- [ ] **Step 1: Add skill manager dependency to worker Manager**

In `internal/worker/manager.go`, the `Manager` struct needs a reference to the skill manager to clean up symlinks during worker deletion (only when `deleteWorkDir=false`).

Add field to Manager struct:
```go
type Manager struct {
    workerBaseDir  string
    beeCfg         config.BeeConfig
    workerStore    *store.WorkerStore
    executionStore *store.ExecutionStore
    invoker        *claude.Invoker
    skillManager   skillCleaner  // interface to avoid circular import

    activeProcesses map[string]*claude.Process
    mu              sync.RWMutex
}

// skillCleaner is the subset of skill.Manager used by worker teardown.
type skillCleaner interface {
    CleanupWorkerLinks(workerID, workDir string) error
}
```

Update `NewManager` signature:
```go
func NewManager(
    workerBaseDir string,
    bc config.BeeConfig,
    ws *store.WorkerStore,
    es *store.ExecutionStore,
    sc skillCleaner,
) *Manager {
    return &Manager{
        workerBaseDir:   workerBaseDir,
        beeCfg:          bc,
        workerStore:     ws,
        executionStore:  es,
        invoker:         claude.NewInvoker(bc.Claude.Path, bc.MCPBaseURL+config.MCPWorkerBasePath, bc.MCP.WorkerAPIKey),
        activeProcesses: make(map[string]*claude.Process),
        skillManager:    sc,
    }
}
```

Update `DeleteWorker`:
```go
func (m *Manager) DeleteWorker(id string, deleteWorkDir bool) error {
    worker, err := m.workerStore.GetByID(id)
    if err != nil {
        return fmt.Errorf("get worker: %w", err)
    }
    if deleteWorkDir {
        if worker.WorkDir != "" {
            if err := os.RemoveAll(worker.WorkDir); err != nil {
                return fmt.Errorf("remove work dir: %w", err)
            }
        }
    } else if worker.WorkDir != "" && m.skillManager != nil {
        // Clean up managed symlinks without deleting the workdir.
        if err := m.skillManager.CleanupWorkerLinks(id, worker.WorkDir); err != nil {
            log.Warn("cleanup worker skill links", zap.Error(err))
        }
    }
    return m.workerStore.Delete(id)
}
```

- [ ] **Step 2: Update caller in server.go to pass skill manager**

In the file constructing `worker.NewManager(...)`, add the `skillMgr` argument:
```go
workerMgr := worker.NewManager(workerBaseDir, beeCfg, workerStore, executionStore, skillMgr)
```

- [ ] **Step 3: Build to verify compilation**

```bash
go build ./...
```

Expected: no errors.

- [ ] **Step 4: Run all tests**

```bash
go test ./...
```

Expected: all tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/worker/manager.go cmd/openbee/server.go
git commit -m "feat(skill): clean up worker skill links on worker deletion"
```

---

## Spec Coverage Check

| Spec requirement | Implemented in |
|---|---|
| Scan global skills | `Scanner.ScanGlobal` (Task 5), `Manager.List` (Task 6) |
| Scan worker skills | `Scanner.ScanWorker` (Task 5), `Manager.ListWorker` (Task 6) |
| Classify managed vs external | `Scanner.scanDir` + `isManagedLink` (Tasks 4, 5) |
| Delete skill | `Manager.Delete` (Task 6), CLI + API (Tasks 7, 8) |
| Edit skill content → new version | `Manager.Edit` (Task 6), CLI + API (Tasks 7, 8) |
| Version switching (use) | `Manager.UseGlobal`, `Manager.UseWorker` (Task 6) |
| Adopt external skill | `Manager.AdoptGlobal`, `Manager.AdoptWorker` (Task 6) |
| `.openbee/skills.json` state file | `Store` (Task 2) |
| Worker override cleanup on delete | `Manager.CleanupWorkerLinks` (Task 9) |
| Refuse delete with references | `Manager.Delete` check (Task 6) |
| CLI `openbee skill …` | Task 7 |
| REST API | Task 8 |
