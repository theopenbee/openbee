# Design: Auto Install and Update Built-in Skills

**Date:** 2026-03-31
**Status:** Approved

## Summary

When `openbee config` completes Claude executable configuration, automatically install or update the built-in `bee` and `worker` skills into the Claude Code global skills directory (`~/.claude/skills/`). Skill content is embedded in the binary as string constants. A SHA-256 hash comparison determines whether each skill needs to be written, producing one of three outcomes: `installed`, `updated`, or `up-to-date`.

## Goals

- Workers running in any work directory have access to the `bee` and `worker` skills without manual setup
- Skill content stays in sync with the installed openbee binary version
- Hash comparison avoids unnecessary writes when skills are already current
- Non-fatal: skill install failure does not abort the `config` flow

## Non-Goals

- Installing externally-sourced skills (those in `skills-lock.json`)
- A standalone `openbee ctl skill` command
- Watch-mode or server-startup auto-sync
- Tracking installed skill versions in a lock file

## Architecture

### Flow

```
openbee config
       │
       ▼ (after configureClaudeExecutable + configureClaudeProvider succeed)
installBuiltinSkills()  [cmd/openbee/config.go]
       │
       ▼
skillinstall.InstallSkills("")  [internal/skillinstall]
       │
       ├── bee/SKILL.md   → SHA-256 compare → installed / updated / up-to-date
       └── worker/SKILL.md → SHA-256 compare → installed / updated / up-to-date
       │
       ▼
~/.claude/skills/bee/SKILL.md
~/.claude/skills/worker/SKILL.md
```

### Install Directory Resolution

`InstallSkills(baseDir string)` resolves the target directory as follows:
- If `baseDir` is non-empty: use it directly (for tests)
- If `baseDir` is empty: `filepath.Join(os.UserHomeDir(), ".claude", "skills")`

## Components

### `internal/skillinstall` Package

**New files:**

```
internal/skillinstall/
  install.go       ← InstallSkills() implementation and hash comparison logic
  skills_data.go   ← beeSkillMD / workerSkillMD string constants
  install_test.go  ← unit tests using t.TempDir()
```

**API:**

```go
// SkillResult holds the outcome for a single skill.
type SkillResult struct {
    Name   string
    Action string // "installed" | "updated" | "up-to-date"
}

// InstallSkills installs embedded skills to baseDir.
// Pass "" to use the default ~/.claude/skills.
// Returns per-skill results for display. Error means a write failed.
func InstallSkills(baseDir string) ([]SkillResult, error)
```

**skills_data.go structure:**

```go
package skillinstall

type skillDef struct {
    name    string
    content string
}

var embeddedSkills = []skillDef{
    {name: "bee",    content: beeSkillMD},
    {name: "worker", content: workerSkillMD},
}

const beeSkillMD = `---
name: bee
...
`

const workerSkillMD = `---
name: worker
...
`
```

Skill content is copied verbatim from `skills/bee/SKILL.md` and `skills/worker/SKILL.md` at the time of implementation. Updates to skill content require updating these constants and cutting a new binary release.

**Hash comparison logic (install.go):**

```
for each skill in embeddedSkills:
  targetPath = filepath.Join(baseDir, skill.name, "SKILL.md")
  newHash    = sha256([]byte(skill.content))

  if targetPath exists:
    existingData, err = os.ReadFile(targetPath)
    if err → return error
    existingHash = sha256(existingData)
    if newHash == existingHash:
      results = append(results, {skill.name, "up-to-date"})
      continue
    // content differs
    os.WriteFile(targetPath, ...)
    results = append(results, {skill.name, "updated"})
  else:
    os.MkdirAll(dir, 0o755)
    os.WriteFile(targetPath, ...)
    results = append(results, {skill.name, "installed"})
```

A write error on any skill terminates the loop and returns the error immediately.

### Integration with `config` Command

In `cmd/openbee/config.go`, after both Claude configuration steps succeed:

```go
// Step 1 — Claude config
fmt.Println(i18n.M.Output.Config.SectionClaude)

if err := configureClaudeExecutable(&vals); err != nil {
    return err
}
if err := configureClaudeProvider(&vals); err != nil {
    return err
}

// Install/update built-in skills (non-fatal on error)
installBuiltinSkills()
```

**`installBuiltinSkills()` helper:**

```go
func installBuiltinSkills() {
    results, err := skillinstall.InstallSkills("")
    if err != nil {
        fmt.Printf(i18n.M.Output.Config.SkillsInstallWarning+"\n", err)
        return
    }
    for _, r := range results {
        switch r.Action {
        case "installed":
            fmt.Printf(i18n.M.Output.Config.SkillInstalled+"\n", r.Name)
        case "updated":
            fmt.Printf(i18n.M.Output.Config.SkillUpdated+"\n", r.Name)
        // "up-to-date": silent
        }
    }
}
```

## Error Handling

| Scenario | Behavior |
|---|---|
| `~/.claude/skills/` does not exist | `os.MkdirAll` creates it; proceed normally |
| Individual skill write fails (e.g. permission denied) | Print `SkillsInstallWarning` to stdout; config continues |
| `os.UserHomeDir()` fails | `InstallSkills` returns error; warning printed; config continues |

Skill installation is non-fatal. A failure here should not prevent the user from completing the config wizard.

## i18n Keys

Three new keys added to both English and Chinese message structs:

| Key | English | Chinese |
|---|---|---|
| `Output.Config.SkillInstalled` | `skill %s installed` | `skill %s 已安装` |
| `Output.Config.SkillUpdated` | `skill %s updated` | `skill %s 已更新` |
| `Output.Config.SkillsInstallWarning` | `skills install warning (skipped): %v` | `安装 skills 时出错（已跳过）: %v` |

## Testing

**`internal/skillinstall/install_test.go`** — all tests use `t.TempDir()` as `baseDir`:

| Test case | What is asserted |
|---|---|
| First install (dir absent) | Dir created, file written, action = `"installed"` |
| Content unchanged | File not re-written (mtime unchanged), action = `"up-to-date"` |
| Content changed | File overwritten with new content, action = `"updated"` |
| Write permission error | `InstallSkills` returns non-nil error |

`installBuiltinSkills()` in `config.go` is not unit-tested (thin wrapper).

## File Changes

**New files:**
```
internal/skillinstall/install.go
internal/skillinstall/skills_data.go
internal/skillinstall/install_test.go
```

**Modified files:**
```
cmd/openbee/config.go        ← call installBuiltinSkills() after claude config steps
internal/i18n/               ← add SkillInstalled, SkillUpdated, SkillsInstallWarning keys
```
