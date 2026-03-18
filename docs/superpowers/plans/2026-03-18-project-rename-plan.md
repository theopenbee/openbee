# Project Rename Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rename the project from `github.com/robobee/core` to `github.com/theopenbee/openbee` across all files.

**Architecture:** Sequential find-and-replace across the codebase, ordered from longest match to shortest to avoid partial replacements. Directory renames first, then text replacements, then build verification.

**Tech Stack:** Go, sed, git mv

---

### Task 1: Rename directories and files

**Files:**
- Rename: `cmd/robobee/` → `cmd/openbee/`
- Modify: `CLAUDE.md` (line 1: `@.robobee.md` → `@.openbee.md`)

- [ ] **Step 1: Rename cmd directory**

```bash
git mv cmd/robobee cmd/openbee
```

- [ ] **Step 2: Update CLAUDE.md**

Change `@.robobee.md` to `@.openbee.md` in `CLAUDE.md`.

- [ ] **Step 3: Commit**

```bash
git add -A && git commit -m "refactor: rename cmd/robobee to cmd/openbee and update CLAUDE.md ref"
```

---

### Task 2: Replace `github.com/robobee/core` → `github.com/theopenbee/openbee`

This is the longest string and must be replaced first.

**Files:** `go.mod` (line 1), all Go source files with imports (~51 files), documentation files with import examples.

- [ ] **Step 1: Update go.mod module declaration**

In `go.mod` line 1, change:
```
module github.com/robobee/core
```
to:
```
module github.com/theopenbee/openbee
```

- [ ] **Step 2: Replace all Go import paths**

Run across all `.go` files:
```bash
find . -name '*.go' -exec sed -i '' 's|github.com/robobee/core|github.com/theopenbee/openbee|g' {} +
```

- [ ] **Step 3: Replace in documentation files**

Run across all `.md` files:
```bash
find . -name '*.md' -exec sed -i '' 's|github.com/robobee/core|github.com/theopenbee/openbee|g' {} +
```

- [ ] **Step 4: Verify no remaining references**

```bash
grep -r "github.com/robobee/core" --include='*.go' --include='*.md' --include='*.yml' --include='*.yaml' .
```
Expected: no output.

- [ ] **Step 5: Commit**

```bash
git add -A && git commit -m "refactor: update Go module path to github.com/theopenbee/openbee"
```

---

### Task 3: Replace `cc-download.robobee.dev` → `cc-download.openbee.dev`

Must be done before the general `robobee` → `openbee` replacement.

**Files:**
- Modify: `cmd/openbee/config_claude.go` (line 16)

- [ ] **Step 1: Update the Claude download URL**

In `cmd/openbee/config_claude.go` line 16, change:
```go
const claudeDownloadURL = "https://cc-download.robobee.dev/claude/download"
```
to:
```go
const claudeDownloadURL = "https://cc-download.openbee.dev/claude/download"
```

- [ ] **Step 2: Commit**

```bash
git add cmd/openbee/config_claude.go && git commit -m "refactor: update Claude download URL domain"
```

---

### Task 4: Replace `robobeedev` → `theopenbee`

**Files:**
- Modify: `.goreleaser.yml` (lines 54, 62, 65, 71, 74, 82)
- Modify: `CONTRIBUTING.md` (lines 20, 125)

- [ ] **Step 1: Update .goreleaser.yml**

Replace all `robobeedev` with `theopenbee` in `.goreleaser.yml`:
- `owner: robobeedev` → `owner: theopenbee` (3 occurrences: release, homebrew, scoop)
- `https://github.com/robobeedev/robobee` → `https://github.com/theopenbee/openbee` (3 occurrences)

- [ ] **Step 2: Update CONTRIBUTING.md**

- Line 20: `git clone https://github.com/robobeedev/robobee.git` → `git clone https://github.com/theopenbee/openbee.git`
- Line 125: `https://github.com/robobeedev/robobee/issues` → `https://github.com/theopenbee/openbee/issues`

- [ ] **Step 3: Search for any remaining `robobeedev` references**

```bash
grep -r "robobeedev" --include='*.go' --include='*.md' --include='*.yml' --include='*.yaml' --include='*.sh' .
```
Expected: no output.

- [ ] **Step 4: Commit**

```bash
git add -A && git commit -m "refactor: update GitHub org from robobeedev to theopenbee"
```

---

### Task 5: Replace `~/.robobee` → `~/.openbee` and `.robobee.md` → `.openbee.md`

**Files:**
- Modify: `internal/media/service.go` (lines 20, 23, 28)
- Modify: `internal/config/config.go` (lines 16, 19, 22, 25)
- Modify: `cmd/openbee/config_claude.go` (line 325: `filepath.Join(home, ".robobee", "bin")`)
- Modify: `internal/claudemd/claudemd.go` (line 16: `SystemRulesFile = ".robobee.md"`)

- [ ] **Step 1: Update internal/config/config.go**

In `internal/config/config.go`:
- Line 16 comment: `~/.robobee/bee` → `~/.openbee/bee`
- Line 19: `filepath.Join(home, ".robobee", "bee")` → `filepath.Join(home, ".openbee", "bee")`
- Line 22 comment: `~/.robobee/worker` → `~/.openbee/worker`
- Line 25: `filepath.Join(home, ".robobee", "worker")` → `filepath.Join(home, ".openbee", "worker")`

- [ ] **Step 2: Update media service**

In `internal/media/service.go`:
- Line 20 comment: `~/.robobee/media` → `~/.openbee/media`
- Line 23: `filepath.Join(home, ".robobee", "media")` → `filepath.Join(home, ".openbee", "media")`
- Line 28 comment: `~/.robobee/media/inbound/` → `~/.openbee/media/inbound/`

- [ ] **Step 3: Update config_claude.go**

In `cmd/openbee/config_claude.go`:
- Line 325: `filepath.Join(home, ".robobee", "bin")` → `filepath.Join(home, ".openbee", "bin")`

- [ ] **Step 4: Update internal/claudemd/claudemd.go**

In `internal/claudemd/claudemd.go`:
- Line 16: `SystemRulesFile = ".robobee.md"` → `SystemRulesFile = ".openbee.md"`

(Note: `ImportLine` on line 17 is derived from `SystemRulesFile` so it updates automatically.)

- [ ] **Step 5: Search for remaining `.robobee` references in Go files**

```bash
grep -r '\.robobee' --include='*.go' .
```
Expected: no output.

- [ ] **Step 6: Commit**

```bash
git add -A && git commit -m "refactor: update user data directory from .robobee to .openbee"
```

---

### Task 6: Replace `robobee.db` → `openbee.db`

**Files:**
- Modify: `config.example.yaml` (line 7)
- Modify: `internal/config/config.yaml.tmpl` (no direct reference, but check)
- Modify: `cmd/openbee/config.go` (line 105: default DBPath `"./data/robobee.db"`)

- [ ] **Step 1: Update config.example.yaml**

Line 7: `path: ./data/robobee.db` → `path: ./data/openbee.db`

- [ ] **Step 2: Update config.go default**

In `cmd/openbee/config.go` line 105: `"./data/robobee.db"` → `"./data/openbee.db"`

- [ ] **Step 3: Commit**

```bash
git add -A && git commit -m "refactor: rename default database file from robobee.db to openbee.db"
```

---

### Task 7: Replace remaining `robobee` → `openbee` (binary name, CLI, scripts, configs)

**Files:**
- Modify: `cmd/openbee/main.go` (lines 18, 19, 26)
- Modify: `cmd/openbee/server.go` (line 16)
- Modify: `Makefile` (lines 1, 26, 36)
- Modify: `.goreleaser.yml` (lines 9-11, 54-55, 79-80, 92)
- Modify: `install.sh` (all references)
- Modify: `CONTRIBUTING.md` (lines 21, 30, 94-95)
- Modify: `internal/config/config.yaml.tmpl` (lines 1-2)
- Modify: `.gitignore` (if contains robobee references)

- [ ] **Step 1: Update cmd/openbee/main.go**

- Line 18: `Use: "robobee"` → `Use: "openbee"`
- Line 19: `Short: "RoboBee 核心服务"` → `Short: "OpenBee 核心服务"`
- Line 26: `robobee %s (commit: %s, built: %s)` → `openbee %s (commit: %s, built: %s)`

- [ ] **Step 2: Update cmd/openbee/server.go**

- Line 16: `Short: "启动 RoboBee 服务"` → `Short: "启动 OpenBee 服务"`

- [ ] **Step 3: Update Makefile**

- Line 1: `BINARY := robobee` → `BINARY := openbee`
- Line 26: `./cmd/robobee/` → `./cmd/openbee/`
- Line 36: `./cmd/robobee/` → `./cmd/openbee/`

- [ ] **Step 4: Update .goreleaser.yml**

- Line 9: `id: robobee` → `id: openbee`
- Line 10: `main: ./cmd/robobee/` → `main: ./cmd/openbee/`
- Line 11: `binary: robobee` → `binary: openbee`
- Line 55: `name: robobee` → `name: openbee`
- Line 79: `id: robobee` → `id: openbee`
- Line 80: `package_name: robobee` → `package_name: openbee`
- Line 92: `dst: /etc/robobee/config.example.yaml` → `dst: /etc/openbee/config.example.yaml`

- [ ] **Step 5: Update install.sh**

Replace all `robobee` → `openbee` and `RoboBee` → `OpenBee`:
```bash
sed -i '' 's/robobee/openbee/g; s/RoboBee/OpenBee/g' install.sh
```

- [ ] **Step 6: Update CONTRIBUTING.md**

- Line 21: `cd robobee` → `cd openbee`
- Line 30: `dist/robobee` → `dist/openbee`
- Line 94: `robobee/` → `openbee/`
- Line 95: `cmd/robobee/` → `cmd/openbee/`
- Title and other brand references: `RoboBee` → `OpenBee`

- [ ] **Step 7: Update internal/config/config.yaml.tmpl**

- Line 1: `# RoboBee 配置文件` → `# OpenBee 配置文件`
- Line 2: `# 由 robobee config 命令生成` → `# 由 openbee config 命令生成`

- [ ] **Step 8: Update .goreleaser.yml brand names**

Replace remaining `RoboBee` with `OpenBee`:
- Line 66: `description: "RoboBee 核心服务"` → `description: "OpenBee 核心服务"` (3 occurrences)
- Line 81: `vendor: RoboBee` → `vendor: OpenBee`
- Line 83: `maintainer: "RoboBee Team"` → `maintainer: "OpenBee Team"`

- [ ] **Step 9: Update .gitignore**

In `.gitignore`:
- Line 1: `data/robobee.db` → `data/openbee.db`
- Line 8: `.robobee.md` → `.openbee.md`
- Line 12: `/robobee` → `/openbee`

- [ ] **Step 10: Update remaining doc files**

Search all `.md` files in `docs/` for `robobee` or `RoboBee` and replace:
```bash
find docs/ -name '*.md' -exec sed -i '' 's/robobee/openbee/g; s/RoboBee/OpenBee/g' {} +
```

- [ ] **Step 11: Update CLA.md and CLA_zh.md**

These files use `Robobee` (not `RoboBee`), so need a separate replacement:
```bash
sed -i '' 's/Robobee/Openbee/g; s/RoboBee/OpenBee/g; s/robobee/openbee/g' CLA.md CLA_zh.md
```

- [ ] **Step 12: Update web/index.html and web/src/components/nav.tsx**

In `web/index.html`: update `<title>RoboBee - Digital Worker Dispatch</title>` → `<title>OpenBee - Digital Worker Dispatch</title>`

In `web/src/components/nav.tsx`: replace any `RoboBee`/`robobee` references with `OpenBee`/`openbee`.

- [ ] **Step 13: Verify no remaining references**

```bash
grep -ri "robobee" --include='*.go' --include='*.md' --include='*.yml' --include='*.yaml' --include='*.sh' --include='*.tsx' --include='*.html' --include='*.json' --include='*.tmpl' . | grep -v 'go\.sum'
```
Also check extensionless files:
```bash
grep -ri "robobee" .gitignore
```
Expected: no output (except possibly `go.sum` which will be regenerated).

- [ ] **Step 13: Commit**

```bash
git add -A && git commit -m "refactor: rename all remaining robobee references to openbee"
```

---

### Task 8: Regenerate go.sum and verify build

- [ ] **Step 1: Delete go.sum and regenerate**

```bash
rm go.sum
go mod tidy
```

- [ ] **Step 2: Verify build**

```bash
go build ./...
```
Expected: successful build with no errors.

- [ ] **Step 3: Run tests**

```bash
go test ./...
```
Expected: all tests pass.

- [ ] **Step 4: Commit**

```bash
git add go.sum && git commit -m "chore: regenerate go.sum after module rename"
```

---

### Task 9: Final sweep and verification

- [ ] **Step 1: Verify old directory no longer exists**

```bash
[ ! -d cmd/robobee ] || echo "ERROR: cmd/robobee still exists"
```
Expected: no output.

- [ ] **Step 2: Full case-insensitive search**

```bash
grep -ri "robobee" . --include='*.go' --include='*.md' --include='*.yml' --include='*.yaml' --include='*.sh' --include='*.tsx' --include='*.ts' --include='*.html' --include='*.json' --include='*.tmpl' | grep -v 'go\.sum' | grep -v '\.git/' | grep -v 'docs/superpowers/specs/2026-03-18-project-rename-design.md' | grep -v 'docs/superpowers/plans/2026-03-18-project-rename-plan.md'
```
Also check extensionless files:
```bash
grep -ri "robobee" .gitignore CLAUDE.md
```
Expected: no output. The rename design/plan docs are excluded as they document the old name by nature.

- [ ] **Step 3: Verify binary builds with correct name**

```bash
go build -o /dev/null ./cmd/openbee/
```
Expected: successful build.

- [ ] **Step 4: Run all tests**

```bash
go test ./...
```
Expected: all tests pass.
